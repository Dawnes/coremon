package master

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTokenValidityAndJobFilters(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.2.12")
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)

	// Install commands: 1 hour by default, 3 at most.
	_, body := c.do("POST", "/api/ui/tokens", `{}`)
	var tok struct{ Expires int64 }
	json.Unmarshal([]byte(body), &tok)
	if d := time.Until(time.Unix(tok.Expires, 0)); d < 59*time.Minute || d > 61*time.Minute {
		t.Errorf("default validity %s, want 1 hour", d)
	}
	if code, _ := c.do("POST", "/api/ui/tokens", `{"ttl_hours":3}`); code != 200 {
		t.Errorf("3 hours refused: %d", code)
	}
	if code, body := c.do("POST", "/api/ui/tokens", `{"ttl_hours":24}`); code != 400 || !strings.Contains(body, "at most 3 hours") {
		t.Errorf("24 hours must be refused: %d %s", code, body)
	}

	// Jobs: filters by user and system are applied before the limit.
	now := time.Now().Unix()
	s.st.mu.Lock()
	s.st.st.Agents["a_1"] = &Agent{ID: "a_1", Name: "web-01"}
	s.st.st.Agents["a_2"] = &Agent{ID: "a_2", Name: "web-02"}
	s.st.st.Jobs = []*Job{
		{ID: "j1", AgentID: "a_1", Actor: "admin", Status: JobSuccess, Created: now},
		{ID: "j2", AgentID: "a_1", Actor: "scheduler", Status: JobSuccess, Created: now},
		{ID: "j3", AgentID: "a_2", Actor: "admin", Status: JobQueued, Created: now},
	}
	s.st.mu.Unlock()
	ids := func(q string) string {
		_, body := c.do("GET", "/api/ui/jobs?"+q, "")
		var d struct{ Jobs []Job }
		json.Unmarshal([]byte(body), &d)
		out := []string{}
		for _, j := range d.Jobs {
			out = append(out, j.ID)
		}
		return strings.Join(out, ",")
	}
	for q, want := range map[string]string{
		"user=admin": "j3,j1", "user=scheduler": "j2", "agent=a_1": "j2,j1",
		"agent=a_1&user=admin": "j1", "user=admin&active=1": "j3", "user=admin&limit=1": "j3",
	} {
		if got := ids(q); got != want {
			t.Errorf("jobs?%s = %s, want %s", q, got, want)
		}
	}
}
