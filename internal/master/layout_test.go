package master

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Reordering systems when there are no groups at all must keep the group
// list an empty list: the dashboard broke on null (master 0.3.5 and older).
func TestLayoutWithoutGroups(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	for _, id := range []string{"a_1", "a_2"} {
		s.st.st.Agents[id] = &Agent{ID: id, Name: id, Status: StatusActive}
	}
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)
	if code, body := c.do("POST", "/api/ui/layout", `{"groups":[{"id":"","agents":["a_2","a_1"]}]}`); code != 200 {
		t.Fatalf("layout: %d %s", code, body)
	}
	_, body := c.do("GET", "/api/ui/overview", "")
	var ov struct {
		Groups json.RawMessage       `json:"groups"`
		Agents []struct{ ID string } `json:"agents"`
	}
	json.Unmarshal([]byte(body), &ov)
	if string(ov.Groups) != "[]" {
		t.Fatalf("groups = %s, want []", ov.Groups)
	}
	if len(ov.Agents) != 2 || ov.Agents[0].ID != "a_2" {
		t.Fatalf("order not saved: %+v", ov.Agents)
	}
}
