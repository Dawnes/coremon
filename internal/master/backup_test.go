package master

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupSlots(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.25")
	var made []string
	for i := 0; i < 4; i++ {
		if _, err := s.runBackup(false); err != nil {
			t.Fatal(err)
		}
		made = append(made, s.loadBackupIndex().Slots[0])
	}
	idx := s.loadBackupIndex()
	if idx.Slots != [3]string{made[3], made[2], made[1]} {
		t.Fatalf("slots %v, made %v", idx.Slots, made)
	}
	if _, err := os.Stat(filepath.Join(s.backupDir(), made[0])); err == nil {
		t.Error("the oldest backup was not removed")
	}
	st, _ := os.Stat(s.backupDir())
	if st.Mode().Perm() != 0o700 && os.PathSeparator == '/' {
		t.Errorf("folder mode %v", st.Mode().Perm())
	}
	// Deleting slot 2 leaves it empty until the next backup; then the slots
	// fill up in date order again.
	if _, err := s.deleteBackup(2); err != nil {
		t.Fatal(err)
	}
	if idx := s.loadBackupIndex(); idx.Slots[1] != "" || idx.Slots[2] != made[1] {
		t.Fatalf("after delete: %v", idx.Slots)
	}
	s.runBackup(false)
	if idx := s.loadBackupIndex(); idx.Slots[1] != made[3] || idx.Slots[2] != made[1] {
		t.Fatalf("after the next backup: %v", idx.Slots)
	}
	if s.describe(1, s.loadBackupIndex().Slots[0]) == nil {
		t.Error("slot 1 not readable")
	}
}

func TestBackupSchedule(t *testing.T) {
	loc := time.Local
	tue := time.Date(2026, 11, 10, 14, 0, 0, 0, loc) // a Tuesday
	cases := map[string]time.Time{
		"daily":   time.Date(2026, 11, 11, 1, 0, 0, 0, loc),
		"weekly":  time.Date(2026, 11, 16, 1, 0, 0, 0, loc), // Monday
		"monthly": time.Date(2026, 12, 1, 1, 0, 0, 0, loc),
	}
	for sched, want := range cases {
		if got := nextBackupAfter(sched, tue); !got.Equal(want) {
			t.Errorf("%s: %v, want %v", sched, got, want)
		}
	}
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.25")
	if s.backupSchedule() != "weekly" {
		t.Fatalf("default schedule %q", s.backupSchedule())
	}
	// A missed run (master off at 01:00) happens at the next look.
	s.ensureBackupDir()
	s.saveBackupIndex(backupIndex{LastAuto: time.Now().Add(-8 * 24 * time.Hour).Unix()})
	s.backupTick(time.Now())
	if s.loadBackupIndex().Slots[0] == "" {
		t.Fatal("missed weekly backup not made")
	}
	s.backupTick(time.Now())
	if s.loadBackupIndex().Slots[1] != "" {
		t.Error("backup made twice")
	}
	s.st.mu.Lock()
	s.st.st.Settings.BackupSchedule = "off"
	s.st.mu.Unlock()
	s.saveBackupIndex(backupIndex{Slots: s.loadBackupIndex().Slots, LastAuto: 1})
	s.backupTick(time.Now())
	if s.loadBackupIndex().Slots[1] != "" {
		t.Error("backup made while off")
	}
}

func TestBackupDownloadAndRestore(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.25")
	exited := make(chan int, 1)
	s.exit = func(code int) { exited <- code }
	s.st.mu.Lock()
	s.st.st.Agents["a_1"] = &Agent{ID: "a_1", Name: "web-01", Status: StatusActive}
	s.st.mu.Unlock()
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pwb, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	admin := newUIClient(t, srv.URL)
	admin.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pwb))+`"}`)
	step := func() int64 { return time.Now().Unix() / 30 }

	// Without 2FA: no download.
	if code, body := admin.do("POST", "/api/ui/backups/download", `{"code":"123456","password":"secret"}`); code != 403 || !strings.Contains(body, "2FA") {
		t.Fatalf("download without 2FA: %d %s", code, body)
	}
	_, body := admin.do("POST", "/api/ui/account/2fa/start", `{}`)
	var en struct{ Secret string }
	json.Unmarshal([]byte(body), &en)
	admin.do("POST", "/api/ui/account/2fa/enable", `{"code":"`+totpCode(en.Secret, step())+`"}`)
	if code, _ := admin.do("POST", "/api/ui/backups/download", `{"code":"`+totpCode(en.Secret, step()+1)+`","password":"ab"}`); code != 400 {
		t.Errorf("2-character password accepted: %d", code)
	}
	if code, _ := admin.do("POST", "/api/ui/backups/download", `{"code":"000000","password":"secret"}`); code != 401 {
		t.Fatalf("wrong code: %d", code)
	}
	req, _ := http.NewRequest("POST", srv.URL+"/api/ui/backups/download", strings.NewReader(`{"code":"`+totpCode(en.Secret, step()+1)+`","password":"secret"}`))
	req.Header.Set("X-Coremon", "1")
	resp, err := admin.hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	file, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Disposition"), ".cmbak") || !bytes.HasPrefix(file, []byte(backupEncMagic)) {
		t.Fatalf("download: %d %q", resp.StatusCode, resp.Header.Get("Content-Disposition"))
	}
	if bytes.Contains(file, []byte("web-01")) {
		t.Fatal("downloaded file is not encrypted")
	}

	// Changes after the backup.
	admin.do("POST", "/api/ui/users", `{"name":"later","role":"viewer","password":"secret1"}`)
	s.st.mu.Lock()
	s.st.st.Agents["a_2"] = &Agent{ID: "a_2", Name: "added-later", Status: StatusActive}
	s.st.st.Agents["a_1"].Pins = [2]string{"some-newer-key", ""}
	s.st.mu.Unlock()

	upload := func(pw string) (int, string) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", "backup.cmbak")
		fw.Write(file)
		mw.WriteField("password", pw)
		mw.Close()
		req, _ := http.NewRequest("POST", srv.URL+"/api/ui/backups/check-file", &buf)
		req.Header.Set("X-Coremon", "1")
		req.Header.Set("Content-Type", mw.FormDataContentType())
		resp, err := admin.hc.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, body := upload("wrong"); code != 400 || !strings.Contains(body, "wrong password") {
		t.Fatalf("wrong password: %d %s", code, body)
	}
	code, body := upload("secret")
	var info struct {
		Token      string `json:"token"`
		Users      int    `json:"users"`
		Systems    int    `json:"systems"`
		NewerKey   int    `json:"newer_key"`
		AddedSince int    `json:"added_since"`
	}
	json.Unmarshal([]byte(body), &info)
	if code != 200 || info.Token == "" || info.Users != 1 || info.Systems != 1 || info.NewerKey != 1 || info.AddedSince != 1 {
		t.Fatalf("check: %d %s", code, body)
	}
	// Checking changes nothing.
	s.st.mu.Lock()
	n := len(s.st.st.Users)
	s.st.mu.Unlock()
	if n != 2 {
		t.Fatal("check changed the users")
	}
	if code, body := admin.do("POST", "/api/ui/backups/restore", `{"token":"`+info.Token+`"}`); code != 200 {
		t.Fatalf("restore: %d %s", code, body)
	}
	select {
	case c := <-exited:
		if c == 0 {
			t.Error("restart must use a non-zero exit")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("master did not restart")
	}
	// After the restart: the backup's data, a before-restore backup, the
	// merged audit log.
	st, err := openStore(s.cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if st.userByName("later") != nil || st.st.Agents["a_2"] != nil || st.st.Agents["a_1"] == nil {
		t.Error("restored state is not the backup's")
	}
	if s.describe(0, backupBefore) == nil {
		t.Error("no before-restore backup")
	}
	audit, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "audit.jsonl"))
	for _, want := range []string{"backup.downloaded", "user.created", "backup.restored"} {
		if !strings.Contains(string(audit), want) {
			t.Errorf("merged audit log lacks %s", want)
		}
	}
	if code, _ := admin.do("POST", "/api/ui/backups/restore", `{"token":"`+info.Token+`"}`); code != 409 {
		t.Error("a check can be used twice")
	}
}

func TestBackupRefusesNewerAndDamaged(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.25")
	files, m, err := s.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	m.Version = "9.9.9"
	a, _ := packBackup(files, m)
	if _, err := s.prepareRestore(a, "x"); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Errorf("newer backup: %v", err)
	}
	m.Version = "0.3.25"
	files["state.dat"] = append(files["state.dat"], 'x')
	a, _ = packBackup(files, m)
	if _, err := s.prepareRestore(a, "x"); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Errorf("damaged backup: %v", err)
	}
	if _, err := s.prepareRestore([]byte("hello"), "x"); err == nil {
		t.Error("garbage accepted")
	}
}
