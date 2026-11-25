package master

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSupportVerdict(t *testing.T) {
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		o     *osSupport
		days  int
		known bool
		fail  string
	}{
		{nil, 0, false, ""},
		{&osSupport{Phase: "open"}, 0, true, ""},
		{&osSupport{End: "2026-11-25", Phase: "lts"}, 0, true, ""}, // the whole last day still counts
		{&osSupport{End: "2026-11-24", Phase: "ended"}, 0, true, "support ended"},
		{&osSupport{End: "2027-01-31", Phase: "lts"}, 0, true, ""},
		{&osSupport{End: "2027-01-31", Phase: "lts"}, 90, true, "in 67 days"},
	}
	for i, c := range cases {
		_, fail, known := supportVerdict(c.o, c.days, now)
		if known != c.known || (c.fail == "") != (fail == "") || !strings.Contains(fail, c.fail) {
			t.Errorf("%d: %q known=%v", i, fail, known)
		}
	}
}

// Every approved agent device gets OS support: decided from the dates, it
// cannot be removed, but it can be paused and given days before the end.
func TestOSSupportService(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	s.osi.f.Data = &osInfoData{Debian: []osRelease{{Version: "11", EOL: "2024-08-14", EOLLTS: "2026-08-31"}, {Version: "13", EOL: time.Now().AddDate(0, 6, 0).Format("2006-01-02")}}}
	old := &Agent{ID: "a_old", Name: "old", Status: StatusActive}
	old.Facts.OSID, old.Facts.OSVersion = "debian", "11" // LTS ended 2026-08-31
	fresh := &Agent{ID: "a_new", Name: "new", Status: StatusActive}
	fresh.Facts.OSID, fresh.Facts.OSVersion = "debian", "13"
	pend := &Agent{ID: "a_pend", Name: "pend", Status: StatusPending}
	s.st.st.Agents[old.ID], s.st.st.Agents[fresh.ID], s.st.st.Agents[pend.ID] = old, fresh, pend
	s.st.mu.Lock()
	for _, a := range s.st.st.Agents {
		s.osSupportCheck(a, time.Now())
	}
	s.st.mu.Unlock()
	if pend.Checks != nil {
		t.Fatal("a pending agent got checks")
	}
	so, sn := old.Checks.osSupportSvc(), fresh.Checks.osSupportSvc()
	if so == nil || sn == nil || !so.Checked || so.Up || !strings.Contains(so.LastError, "2026-08-31") || !sn.Up {
		t.Fatalf("old %+v / new %+v", so, sn)
	}
	if len(old.Checks.outside()) != 0 || old.Checks.activeChecks() != 0 {
		t.Error("OS support is not checked from outside")
	}

	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)
	base := "/api/ui/locals/a_new/services/" + sn.ID
	if code, b := c.do("POST", base+"/delete", ""); code != 409 || !strings.Contains(b, "pause") {
		t.Errorf("delete: %d %s", code, b)
	}
	if code, b := c.do("POST", base, `{"method":"tcp","port":22}`); code != 400 {
		t.Errorf("change method: %d %s", code, b)
	}
	if code, b := c.do("POST", base, `{"days":400}`); code != 400 {
		t.Errorf("days 400: %d %s", code, b)
	}
	if code, b := c.do("POST", base, `{"days":365}`); code != 200 || sn.Days != 365 {
		t.Errorf("days: %d %s", code, b)
	}
	if code, _ := c.do("POST", base+"/check", ""); code != 200 || sn.Up {
		t.Errorf("check now with 365 days: %d up=%v %s", code, sn.Up, sn.LastError)
	}
	if code, b := c.do("POST", base+"/pause", `{"paused":true}`); code != 200 || !sn.Paused {
		t.Errorf("pause: %d %s", code, b)
	}
}
