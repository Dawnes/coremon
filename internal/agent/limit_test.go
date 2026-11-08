package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"coremon/internal/proto"
)

func TestRunningCommandLimit(t *testing.T) {
	a := &Agent{running: map[string]bool{}}
	for i := 0; i < maxRunning; i++ {
		if r := a.claimLimited(fmt.Sprint("c", i)); r != claimed {
			t.Fatalf("command %d: %v, want claimed", i, r)
		}
	}
	if r := a.claimLimited("c0"); r != claimDuplicate {
		t.Errorf("a duplicate must be ignored, got %v", r)
	}
	if r := a.claimLimited("one-too-many"); r != claimBusy {
		t.Errorf("command %d must be refused, got %v", maxRunning+1, r)
	}
	a.mu.Lock()
	delete(a.running, "c3") // one finishes
	a.mu.Unlock()
	if r := a.claimLimited("next"); r != claimed {
		t.Errorf("after one finished a new command must be accepted, got %v", r)
	}
}

func TestBusyRefusalIsReported(t *testing.T) {
	var mu sync.Mutex
	var got proto.Result
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		json.Unmarshal(b, &got)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	a := &Agent{cl: &client{base: srv.URL, hc: srv.Client(), plain: true}}
	a.reject(context.Background(), proto.Command{ID: "j9"}, errBusy)
	mu.Lock()
	defer mu.Unlock()
	if got.CommandID != "j9" || got.OK || !strings.Contains(got.Error, "agent busy") {
		t.Fatalf("refusal not reported: %+v", got)
	}
}
