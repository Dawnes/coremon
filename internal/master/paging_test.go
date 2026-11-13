package master

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestAuditPaging(t *testing.T) {
	a := &auditLog{path: t.TempDir() + "/audit.jsonl"}
	put := func(e auditEntry) { // with a fixed time (add stamps the current one)
		b, _ := json.Marshal(e)
		f, _ := os.OpenFile(a.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		f.Write(append(b, '\n'))
		f.Close()
	}
	// 7 entries, several in the same second.
	times := []int64{100, 100, 101, 102, 102, 102, 103}
	for i, ts := range times {
		put(auditEntry{Time: ts, Actor: "u", Action: fmt.Sprint("a", i)})
	}
	var got []string
	var before int64
	skip := 0
	for round := 0; ; round++ {
		page, more := a.page("", "", before, skip, 3)
		for _, e := range page {
			got = append(got, e.Action)
		}
		if !more {
			break
		}
		last := page[len(page)-1].Time
		if last != before {
			before, skip = last, 0
		}
		for _, e := range page {
			if e.Time == last {
				skip++
			}
		}
		if round > 5 {
			t.Fatal("no end")
		}
	}
	want := "[a6 a5 a4 a3 a2 a1 a0]"
	if fmt.Sprint(got) != want {
		t.Errorf("pages: %v, want %s", got, want)
	}
	// A new entry does not shift the next page.
	page, _ := a.page("", "", 0, 0, 2) // a6 a5
	put(auditEntry{Time: 104, Actor: "u", Action: "new"})
	if page[1].Action != "a5" {
		t.Fatalf("first page: %v", page)
	}
	next, _ := a.page("", "", 102, 1, 2) // after a5, the first of three at 102
	if next[0].Action+" "+next[1].Action != "a4 a3" {
		t.Errorf("after a new entry: %v", next)
	}
}

func TestJobsPaging(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	s.st.st.Jobs = nil
	for i := 0; i < 7; i++ {
		s.st.st.Jobs = append(s.st.st.Jobs, &Job{ID: fmt.Sprint("j", i), AgentID: "a", Actor: "u", Created: int64(100 + i), Status: JobSuccess})
	}
	ids := func(l []*Job) string {
		out := ""
		for _, j := range l {
			out += j.ID + " "
		}
		return out
	}
	p1, more := s.jobsPage("", "", false, "", 0, 3)
	if ids(p1) != "j6 j5 j4 " || !more {
		t.Fatalf("page 1: %s %v", ids(p1), more)
	}
	p2, more := s.jobsPage("", "", false, "j4", 104, 3)
	if ids(p2) != "j3 j2 j1 " || !more {
		t.Fatalf("page 2: %s %v", ids(p2), more)
	}
	// j1 is cleaned up before the next page: continue by its time.
	s.st.st.Jobs = s.st.st.Jobs[2:] // j0 and j1 gone
	p3, more := s.jobsPage("", "", false, "j1", 101, 3)
	if ids(p3) != "" || more {
		t.Fatalf("page 3: %s %v", ids(p3), more)
	}
}
