package master

// Backups of the master's data: three local slots, a schedule, encrypted
// downloads, and restoring.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"coremon/internal/seal"
)

const (
	backupFormat    = "coremon-backup-1"
	backupDirName   = "backups"
	backupSlots     = 3
	backupBefore    = "before-restore.tar.gz"
	backupEncMagic  = "COREMON-BACKUP-ENC-1\n"
	backupKDFIter   = 600000
	maxBackupFile   = 256 << 20
	minBackupPW     = 3
	maxBackupPW     = 40
	defaultSchedule = "weekly"
)

// Files a backup carries (optional ones may be missing).
var backupFiles = []struct {
	name     string
	required bool
}{
	{"state.dat", true},
	{"payload.key", true},
	{"payload-next.key", false},
	{"tls.crt", false},
	{"tls.key", false},
	{"tls-next.crt", false},
	{"tls-next.key", false},
	{"audit.jsonl", false},
}

var backupSchedules = []string{"off", "daily", "weekly", "monthly"}

func validSchedule(s string) bool {
	for _, x := range backupSchedules {
		if x == s {
			return true
		}
	}
	return false
}

type backupManifest struct {
	Format  string            `json:"format"`
	Version string            `json:"version"`
	Created int64             `json:"created"`
	Users   int               `json:"users"`
	Systems int               `json:"systems"`
	Files   map[string]string `json:"files"` // name -> sha256
}

type backupIndex struct {
	Slots    [backupSlots]string `json:"slots"` // file names, "" = empty
	LastAuto int64               `json:"last_auto"`
}

type backupState struct {
	mu      sync.Mutex // one backup or restore at a time
	idxMu   sync.Mutex
	pending map[string]*preparedRestore
	pendMu  sync.Mutex
}

func (s *Server) backupDir() string { return filepath.Join(s.cfg.DataDir, backupDirName) }

// ---- the archive ----

// snapshot collects the current data: the state from memory (consistent),
// the key files, the certificate and the audit log.
func (s *Server) snapshot() (map[string][]byte, backupManifest, error) {
	files := map[string][]byte{}
	s.st.mu.Lock()
	js, err := json.Marshal(&s.st.st)
	users, systems := len(s.st.st.Users), len(s.st.st.Agents)
	s.st.mu.Unlock()
	if err != nil {
		return nil, backupManifest{}, err
	}
	enc, err := encodeState(js)
	if err != nil {
		return nil, backupManifest{}, err
	}
	files["state.dat"] = enc
	s.keyMu.Lock()
	for _, f := range backupFiles[1:7] {
		if b, err := os.ReadFile(filepath.Join(s.cfg.DataDir, f.name)); err == nil {
			files[f.name] = b
		}
	}
	s.keyMu.Unlock()
	if b, err := s.audit.raw(); err == nil {
		files["audit.jsonl"] = b
	}
	if files["payload.key"] == nil {
		return nil, backupManifest{}, errors.New("payload.key missing")
	}
	m := backupManifest{Format: backupFormat, Version: s.cfg.Version, Created: time.Now().Unix(), Users: users, Systems: systems, Files: map[string]string{}}
	for n, b := range files {
		h := sha256.Sum256(b)
		m.Files[n] = hex.EncodeToString(h[:])
	}
	return files, m, nil
}

func packBackup(files map[string][]byte, m backupManifest) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	mj, _ := json.MarshalIndent(m, "", "  ")
	put := func(name string, b []byte) error {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(b)), ModTime: time.Unix(m.Created, 0)}); err != nil {
			return err
		}
		_, err := tw.Write(b)
		return err
	}
	if err := put("manifest.json", mj); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if err := put(n, files[n]); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// unpackBackup reads and checks an archive: known files only, checksums,
// format, and a readable state.
func unpackBackup(b []byte) (map[string][]byte, backupManifest, error) {
	var m backupManifest
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, m, errors.New("not a Coremon backup")
	}
	tr := tar.NewReader(zr)
	known := map[string]bool{"manifest.json": true}
	for _, f := range backupFiles {
		known[f.name] = true
	}
	files := map[string][]byte{}
	var mj []byte
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, m, errors.New("the backup is damaged")
		}
		if !known[h.Name] || h.Typeflag != tar.TypeReg || h.Size > maxBackupFile {
			return nil, m, fmt.Errorf("unexpected file %q in the backup", h.Name)
		}
		data, err := io.ReadAll(io.LimitReader(tr, maxBackupFile))
		if err != nil {
			return nil, m, errors.New("the backup is damaged")
		}
		if h.Name == "manifest.json" {
			mj = data
		} else {
			files[h.Name] = data
		}
	}
	if mj == nil || json.Unmarshal(mj, &m) != nil || m.Format != backupFormat {
		return nil, m, errors.New("not a Coremon backup")
	}
	if len(m.Files) != len(files) {
		return nil, m, errors.New("the backup is incomplete")
	}
	for n, sum := range m.Files {
		b, ok := files[n]
		h := sha256.Sum256(b)
		if !ok || hex.EncodeToString(h[:]) != sum {
			return nil, m, fmt.Errorf("%s in the backup is damaged (checksum)", n)
		}
	}
	for _, f := range backupFiles {
		if f.required && files[f.name] == nil {
			return nil, m, fmt.Errorf("%s is missing from the backup", f.name)
		}
	}
	js, err := decodeState(files["state.dat"])
	var st State
	if err != nil || json.Unmarshal(js, &st) != nil {
		return nil, m, errors.New("the state in the backup cannot be read")
	}
	if _, err := seal.ParsePrivate(files["payload.key"]); err != nil {
		return nil, m, errors.New("the key in the backup cannot be read")
	}
	return files, m, nil
}

// ---- encryption for downloads ----

func backupPasswordProblem(pw string) string {
	if n := utf8.RuneCountInString(pw); n < minBackupPW || n > maxBackupPW || !utf8.ValidString(pw) {
		return fmt.Sprintf("the encryption password must be %d-%d characters", minBackupPW, maxBackupPW)
	}
	return ""
}

func backupKey(pw string, salt []byte) ([]byte, error) {
	return pbkdf2.Key(sha256.New, pw, salt, backupKDFIter, 32)
}

func encryptBackup(plain []byte, pw string) ([]byte, error) {
	salt := make([]byte, 16)
	rand.Read(salt)
	key, err := backupKey(pw, salt)
	if err != nil {
		return nil, err
	}
	blk, _ := aes.NewCipher(key)
	g, _ := cipher.NewGCM(blk)
	nonce := make([]byte, g.NonceSize())
	rand.Read(nonce)
	out := append([]byte(backupEncMagic), salt...)
	out = append(out, nonce...)
	return g.Seal(out, nonce, plain, []byte(backupEncMagic)), nil
}

func decryptBackup(b []byte, pw string) ([]byte, error) {
	if !bytes.HasPrefix(b, []byte(backupEncMagic)) {
		return nil, errors.New("not an encrypted Coremon backup")
	}
	rest := b[len(backupEncMagic):]
	if len(rest) < 16+12+16 {
		return nil, errors.New("the backup file is damaged")
	}
	key, err := backupKey(pw, rest[:16])
	if err != nil {
		return nil, err
	}
	blk, _ := aes.NewCipher(key)
	g, _ := cipher.NewGCM(blk)
	plain, err := g.Open(nil, rest[16:28], rest[28:], []byte(backupEncMagic))
	if err != nil {
		return nil, errors.New("wrong encryption password, or the file is damaged")
	}
	return plain, nil
}

// ---- local slots ----

func (s *Server) loadBackupIndex() backupIndex {
	var idx backupIndex
	if b, err := os.ReadFile(filepath.Join(s.backupDir(), "index.json")); err == nil {
		json.Unmarshal(b, &idx)
	}
	for i, n := range idx.Slots {
		if n != "" && !validBackupName(n) {
			idx.Slots[i] = ""
		}
	}
	return idx
}

func (s *Server) saveBackupIndex(idx backupIndex) error {
	b, _ := json.MarshalIndent(idx, "", "  ")
	tmp := filepath.Join(s.backupDir(), "index.json.tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.backupDir(), "index.json"))
}

func validBackupName(n string) bool {
	return n == backupBefore || (strings.HasPrefix(n, "backup-") && strings.HasSuffix(n, ".tar.gz") && !strings.ContainsAny(n, `/\`) && len(n) < 64)
}

func (s *Server) ensureBackupDir() error {
	if err := os.MkdirAll(s.backupDir(), 0o700); err != nil {
		return err
	}
	return os.Chmod(s.backupDir(), 0o700)
}

// writeVerified writes an archive and reads it back before it counts.
func (s *Server) writeVerified(name string, archive []byte) error {
	if err := s.ensureBackupDir(); err != nil {
		return err
	}
	final := filepath.Join(s.backupDir(), name)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, archive, 0o600); err != nil {
		return err
	}
	back, err := os.ReadFile(tmp)
	if err == nil {
		_, _, err = unpackBackup(back)
	}
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("the new backup did not verify: %w", err)
	}
	return os.Rename(tmp, final)
}

// runBackup makes a backup into slot 1; the others shift, the oldest goes.
func (s *Server) runBackup(auto bool) (backupManifest, error) {
	s.bk.mu.Lock()
	defer s.bk.mu.Unlock()
	files, m, err := s.snapshot()
	if err != nil {
		return m, err
	}
	archive, err := packBackup(files, m)
	if err != nil {
		return m, err
	}
	name := "backup-" + time.Unix(m.Created, 0).UTC().Format("20060102-150405") + "-" + randHex(3) + ".tar.gz"
	if err := s.writeVerified(name, archive); err != nil {
		return m, err
	}
	s.bk.idxMu.Lock()
	defer s.bk.idxMu.Unlock()
	idx := s.loadBackupIndex()
	list := []string{name}
	for _, n := range idx.Slots {
		if n != "" && n != name {
			list = append(list, n)
		}
	}
	var next [backupSlots]string
	for i, n := range list {
		if i < backupSlots {
			next[i] = n
		} else {
			os.Remove(filepath.Join(s.backupDir(), n))
		}
	}
	idx.Slots = next
	if auto {
		idx.LastAuto = m.Created
	}
	return m, s.saveBackupIndex(idx)
}

func (s *Server) deleteBackup(slot int) (string, error) {
	s.bk.idxMu.Lock()
	defer s.bk.idxMu.Unlock()
	idx := s.loadBackupIndex()
	if slot == 0 {
		return backupBefore, os.Remove(filepath.Join(s.backupDir(), backupBefore))
	}
	if slot < 1 || slot > backupSlots || idx.Slots[slot-1] == "" {
		return "", errors.New("that slot is empty")
	}
	n := idx.Slots[slot-1]
	os.Remove(filepath.Join(s.backupDir(), n))
	idx.Slots[slot-1] = ""
	return n, s.saveBackupIndex(idx)
}

type slotView struct {
	Slot    int    `json:"slot"` // 0 = before restore
	Created int64  `json:"created"`
	Version string `json:"version"`
	Size    int64  `json:"size"`
	Users   int    `json:"users"`
	Systems int    `json:"systems"`
}

// describe reads a stored backup's manifest; nil if it is missing or broken.
func (s *Server) describe(slot int, name string) *slotView {
	p := filepath.Join(s.backupDir(), name)
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	_, m, err := unpackBackup(b)
	if err != nil {
		return nil
	}
	return &slotView{Slot: slot, Created: m.Created, Version: m.Version, Size: int64(len(b)), Users: m.Users, Systems: m.Systems}
}

// ---- schedule ----

// nextBackupAfter: the first scheduled time (01:00 local) after t.
func nextBackupAfter(schedule string, t time.Time) time.Time {
	t = t.Local()
	day := time.Date(t.Year(), t.Month(), t.Day(), 1, 0, 0, 0, time.Local)
	for i := 0; i < 400; i++ {
		c := day.AddDate(0, 0, i)
		if !c.After(t) {
			continue
		}
		switch schedule {
		case "daily":
			return c
		case "weekly":
			if c.Weekday() == time.Monday {
				return c
			}
		case "monthly":
			if c.Day() == 1 {
				return c
			}
		}
	}
	return time.Time{}
}

func (s *Server) backupSchedule() string {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	return s.st.st.Settings.BackupSchedule
}

func (s *Server) backupLoop(stop <-chan struct{}) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		s.backupTick(time.Now())
		select {
		case <-stop:
			return
		case <-t.C:
		}
	}
}

// backupTick runs a scheduled backup when one is due (also one missed while
// the master was off).
func (s *Server) backupTick(now time.Time) {
	sched := s.backupSchedule()
	if sched == "off" {
		return
	}
	s.bk.idxMu.Lock()
	idx := s.loadBackupIndex()
	if idx.LastAuto == 0 { // first run: from now on
		idx.LastAuto = now.Unix()
		s.ensureBackupDir()
		s.saveBackupIndex(idx)
	}
	s.bk.idxMu.Unlock()
	next := nextBackupAfter(sched, time.Unix(idx.LastAuto, 0))
	if next.IsZero() || now.Before(next) {
		return
	}
	if m, err := s.runBackup(true); err != nil {
		s.audit.write("scheduler", "", "backup.failed", "", err.Error())
	} else {
		s.audit.write("scheduler", "", "backup.created", "", fmt.Sprintf("%s backup into slot 1 (%d users, %d systems)", sched, m.Users, m.Systems))
	}
}
