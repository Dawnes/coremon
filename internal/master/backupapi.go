package master

// Settings > Backups: the API, and restoring.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"coremon/internal/seal"
)

const restoreTTL = 15 * time.Minute

type preparedRestore struct {
	files  map[string][]byte
	m      backupManifest
	source string
	exp    time.Time
}

func (s *Server) handleBackups(w http.ResponseWriter, r *http.Request) {
	s.bk.idxMu.Lock()
	idx := s.loadBackupIndex()
	s.bk.idxMu.Unlock()
	slots := make([]*slotView, backupSlots)
	for i, n := range idx.Slots {
		if n != "" {
			slots[i] = s.describe(i+1, n)
		}
	}
	writeJSON(w, map[string]any{
		"dir": s.backupDir(), "schedule": s.backupSchedule(), "schedules": backupSchedules,
		"slots": slots, "before": s.describe(0, backupBefore), "has_2fa": userFrom(r).TOTPSecret != "",
		"min_password": minBackupPW, "max_password": maxBackupPW,
	})
}

func (s *Server) handleBackupRun(w http.ResponseWriter, r *http.Request) {
	m, err := s.runBackup(false)
	if err != nil {
		s.audit.write(actor(r), clientIP(r), "backup.failed", "", err.Error())
		http.Error(w, "backup failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.audit.write(actor(r), clientIP(r), "backup.created", "", fmt.Sprintf("manual backup into slot 1 (%d users, %d systems)", m.Users, m.Systems))
	s.handleBackups(w, r)
}

func (s *Server) handleBackupDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slot int `json:"slot"`
	}
	if !readJSON(w, r, &req, 1024) {
		return
	}
	if _, err := s.deleteBackup(req.Slot); err != nil && !os.IsNotExist(err) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	what := fmt.Sprintf("slot %d", req.Slot)
	if req.Slot == 0 {
		what = "the before-restore backup"
	}
	s.audit.write(actor(r), clientIP(r), "backup.deleted", "", what)
	s.handleBackups(w, r)
}

func (s *Server) handleBackupSchedule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Schedule string `json:"schedule"`
	}
	if !readJSON(w, r, &req, 1024) {
		return
	}
	if !validSchedule(req.Schedule) {
		http.Error(w, "schedule must be off, daily, weekly or monthly", http.StatusBadRequest)
		return
	}
	s.st.mu.Lock()
	old := s.st.st.Settings.BackupSchedule
	s.st.st.Settings.BackupSchedule = req.Schedule
	s.st.markDirty()
	s.st.mu.Unlock()
	if old != req.Schedule {
		s.audit.write(actor(r), clientIP(r), "backup.schedule", "", old+" → "+req.Schedule)
	}
	s.handleBackups(w, r)
}

// handleBackupDownload: only with 2FA on, a fresh code, and an encryption
// password that really encrypts the file.
func (s *Server) handleBackupDownload(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	var req struct {
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}
	if me.TOTPSecret == "" {
		http.Error(w, "turn on 2FA in My Account first", http.StatusForbidden)
		return
	}
	if msg := backupPasswordProblem(req.Password); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	if !s.checkCode(w, r, me.ID, "", req.Code) {
		s.audit.write(me.Name, clientIP(r), "backup.download_refused", "", "wrong 2FA code")
		return
	}
	s.bk.mu.Lock()
	files, m, err := s.snapshot()
	var archive, enc []byte
	if err == nil {
		archive, err = packBackup(files, m)
	}
	if err == nil {
		enc, err = encryptBackup(archive, req.Password)
	}
	s.bk.mu.Unlock()
	if err != nil {
		http.Error(w, "backup failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.audit.write(me.Name, clientIP(r), "backup.downloaded", "", fmt.Sprintf("encrypted backup (%d users, %d systems)", m.Users, m.Systems))
	name := "coremon-backup-" + time.Unix(m.Created, 0).Local().Format("2006-01-02-1504") + ".cmbak"
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Write(enc)
}

// ---- restore ----

// prepareRestore checks a backup completely before anything changes.
func (s *Server) prepareRestore(archive []byte, source string) (map[string]any, error) {
	files, m, err := unpackBackup(archive)
	if err != nil {
		return nil, err
	}
	if versionLess(s.cfg.Version, m.Version) {
		return nil, fmt.Errorf("this backup was made by master %s, newer than this master (%s): update the master first", m.Version, s.cfg.Version)
	}
	js, _ := decodeState(files["state.dat"])
	var bst State
	json.Unmarshal(js, &bst)
	trusted := map[string]bool{}
	for _, n := range []string{"payload.key", "payload-next.key"} {
		if b := files[n]; b != nil {
			if k, err := seal.ParsePrivate(b); err == nil {
				trusted[k.Public().String()] = true
			}
		}
	}
	newerKey, added := 0, 0
	s.st.mu.Lock()
	for id, a := range s.st.st.Agents {
		if _, known := bst.Agents[id]; !known {
			added++
			continue
		}
		reported := a.Pins[0] != "" || a.Pins[1] != ""
		if reported && !trusted[a.Pins[0]] && !trusted[a.Pins[1]] {
			newerKey++
		}
	}
	s.st.mu.Unlock()
	tok := randHex(16)
	s.bk.pendMu.Lock()
	if s.bk.pending == nil {
		s.bk.pending = map[string]*preparedRestore{}
	}
	for k, p := range s.bk.pending {
		if time.Now().After(p.exp) {
			delete(s.bk.pending, k)
		}
	}
	s.bk.pending[tok] = &preparedRestore{files: files, m: m, source: source, exp: time.Now().Add(restoreTTL)}
	s.bk.pendMu.Unlock()
	return map[string]any{"token": tok, "created": m.Created, "version": m.Version, "users": m.Users, "systems": m.Systems,
		"newer_key": newerKey, "added_since": added, "source": source}, nil
}

func (s *Server) handleRestoreCheck(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slot int `json:"slot"`
	}
	if !readJSON(w, r, &req, 1024) {
		return
	}
	s.bk.idxMu.Lock()
	idx := s.loadBackupIndex()
	s.bk.idxMu.Unlock()
	name, source := backupBefore, "the before-restore backup"
	if req.Slot != 0 {
		if req.Slot < 1 || req.Slot > backupSlots || idx.Slots[req.Slot-1] == "" {
			http.Error(w, "that slot is empty", http.StatusBadRequest)
			return
		}
		name, source = idx.Slots[req.Slot-1], fmt.Sprintf("slot %d", req.Slot)
	}
	b, err := os.ReadFile(filepath.Join(s.backupDir(), name))
	if err != nil {
		http.Error(w, "the backup file cannot be read", http.StatusBadRequest)
		return
	}
	info, err := s.prepareRestore(b, source)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, info)
}

func (s *Server) handleRestoreCheckFile(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBackupFile+(1<<20))
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "the upload failed or is too large", http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()
	f, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "no file", http.StatusBadRequest)
		return
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxBackupFile+1))
	if err != nil || len(b) > maxBackupFile {
		http.Error(w, "the file is too large", http.StatusBadRequest)
		return
	}
	plain, err := decryptBackup(b, r.FormValue("password"))
	if err != nil {
		time.Sleep(400 * time.Millisecond)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	info, err := s.prepareRestore(plain, "an uploaded file")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, info)
}

// mergeAudit joins two audit logs: every entry once, oldest first.
func mergeAudit(a, b []byte) []byte {
	type line struct {
		t int64
		s string
	}
	seen := map[string]bool{}
	var all []line
	for _, src := range [][]byte{a, b} {
		for _, l := range bytes.Split(src, []byte("\n")) {
			l = bytes.TrimSpace(l)
			if len(l) == 0 || seen[string(l)] {
				continue
			}
			var e auditEntry
			if json.Unmarshal(l, &e) != nil {
				continue
			}
			seen[string(l)] = true
			all = append(all, line{e.Time, string(l)})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].t < all[j].t })
	var out bytes.Buffer
	for _, l := range all {
		out.WriteString(l.s)
		out.WriteByte('\n')
	}
	return out.Bytes()
}

func writeAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// handleRestore applies a checked backup: a safety backup first, then the
// files, then the master restarts.
func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if !readJSON(w, r, &req, 1024) {
		return
	}
	s.bk.pendMu.Lock()
	p := s.bk.pending[req.Token]
	delete(s.bk.pending, req.Token)
	s.bk.pendMu.Unlock()
	if p == nil || time.Now().After(p.exp) {
		http.Error(w, "this check has expired; check the backup again", http.StatusConflict)
		return
	}
	if err := s.restore(p, actor(r), clientIP(r)); err != nil {
		s.audit.write(actor(r), clientIP(r), "backup.restore_failed", "", err.Error())
		http.Error(w, "restore failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go func() {
		time.Sleep(time.Second)
		log.Printf("restarting after a restore")
		s.exit(3) // the service restarts after a non-zero exit
	}()
}

func (s *Server) restore(p *preparedRestore, who, ip string) error {
	s.bk.mu.Lock()
	defer s.bk.mu.Unlock()
	// Safety net: the current situation, so this restore can be undone.
	files, m, err := s.snapshot()
	if err != nil {
		return fmt.Errorf("before-restore backup: %w", err)
	}
	archive, err := packBackup(files, m)
	if err == nil {
		err = s.writeVerified(backupBefore, archive)
	}
	if err != nil {
		return fmt.Errorf("before-restore backup: %w", err)
	}
	merged := mergeAudit(p.files["audit.jsonl"], files["audit.jsonl"])
	// From here on the in-memory state must not overwrite the restored file.
	s.st.mu.Lock()
	s.st.frozen = true
	s.st.mu.Unlock()
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	for _, f := range backupFiles {
		path := filepath.Join(s.cfg.DataDir, f.name)
		b, ok := p.files[f.name]
		switch {
		case f.name == "audit.jsonl":
			continue
		case ok:
			if err := writeAtomic(path, b); err != nil {
				return err
			}
		case f.name == "payload-next.key" || f.name == "tls-next.crt" || f.name == "tls-next.key":
			os.Remove(path)
		}
	}
	s.audit.mu.Lock()
	err = writeAtomic(s.audit.path, merged)
	s.audit.mu.Unlock()
	if err != nil {
		return err
	}
	s.audit.write(who, ip, "backup.restored", "", fmt.Sprintf("restored from %s, made %s by master %s; the previous data is kept as the before-restore backup",
		p.source, time.Unix(p.m.Created, 0).Format("2006-01-02 15:04"), p.m.Version))
	return nil
}
