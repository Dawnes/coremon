package master

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTimeZone(t *testing.T) {
	var set Settings
	set.normalize()
	if set.TimeZone != "Europe/Amsterdam" {
		t.Fatalf("default zone %q", set.TimeZone)
	}
	// With the built-in zone data (not with -tags notzdata): summer time.
	if l, err := time.LoadLocation("Europe/Amsterdam"); err == nil {
		_, w := time.Date(2026, 1, 15, 12, 0, 0, 0, l).Zone()
		_, s := time.Date(2026, 7, 15, 12, 0, 0, 0, l).Zone()
		if w != 3600 || s != 7200 || zoneOf("Europe/Amsterdam") != zoneOf("Europe/Amsterdam") {
			t.Errorf("Amsterdam offsets %d %d", w, s)
		}
		// Backups at 01:00 Amsterdam time, also across the summer time change.
		got := nextBackupAfter("daily", time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC), l)
		if got.In(l).Hour() != 1 || got.UTC().Hour() != 0 {
			t.Errorf("backup %v", got)
		}
	}
	if zoneOf("Mars/Olympus") != time.UTC {
		t.Error("unknown zone not UTC")
	}

	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)
	if _, body := c.do("GET", "/api/ui/session", ""); !strings.Contains(body, `"time_zone":"Europe/Amsterdam"`) {
		t.Errorf("session: %s", body)
	}
	if code, _ := c.do("POST", "/api/ui/settings/time", `{"time_zone":"Mars/Olympus"}`); code != 400 {
		t.Error("unknown zone accepted")
	}
	if code, _ := c.do("POST", "/api/ui/settings/time", `{"time_zone":"../etc"}`); code != 400 {
		t.Error("bad name accepted")
	}
	if code, body := c.do("POST", "/api/ui/settings/time", `{"time_zone":"UTC"}`); code != 200 || !strings.Contains(body, `"time_zone":"UTC"`) {
		t.Fatalf("save: %d %s", code, body)
	}
	if s.zoneNow() != time.UTC {
		t.Error("zone not applied")
	}
	if e := s.audit.list("", "", 1); len(e) == 0 || e[0].Action != "settings.time" || !strings.Contains(e[0].Detail, "Europe/Amsterdam → UTC") {
		t.Errorf("audit: %+v", e)
	}
}
