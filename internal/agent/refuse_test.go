package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"coremon/internal/feature"
	"coremon/internal/proto"
)

// spyFeature records whether it was called.
type spyFeature struct{ called bool }

func (s *spyFeature) Info() proto.FeatureInfo {
	return proto.FeatureInfo{Name: "updates", Actions: []proto.ActionSpec{{Name: "install", Mutating: true}}}
}
func (s *spyFeature) Call(c *feature.Ctx, action string) (any, error) {
	s.called = true
	return map[string]any{}, nil
}

func TestAgentRefusesActionsInReportOnly(t *testing.T) {
	var mu sync.Mutex
	results := []proto.Result{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/agent/result" {
			b, _ := io.ReadAll(r.Body)
			var res proto.Result
			json.Unmarshal(b, &res)
			mu.Lock()
			results = append(results, res)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	spy := &spyFeature{}
	cl := &client{base: srv.URL, hc: srv.Client(), plain: true}
	a := &Agent{opt: Options{StateDir: t.TempDir()}, cl: cl, out: newOutputBuffer(cl), running: map[string]bool{},
		feats: map[string]feature.Feature{"updates": spy}, locks: map[string]*sync.Mutex{"updates": {}}}
	ctx := context.Background()

	// Report only: an install sent by mistake is refused by the agent itself.
	mode := func(on bool) proto.Command {
		p, _ := json.Marshal(proto.ModeParams{ReportOnly: on})
		return proto.Command{ID: "m", Type: proto.CmdSetMode, Params: p}
	}
	a.execute(ctx, mode(true))
	a.execute(ctx, proto.Command{ID: "j1", Type: proto.CmdAction, Feature: "updates", Action: "install", Params: json.RawMessage(`{"packages":["curl"]}`)})
	if spy.called {
		t.Fatal("the install ran although the agent is in report-only mode")
	}
	mu.Lock()
	last := results[len(results)-1]
	mu.Unlock()
	if last.OK || !strings.Contains(last.Error, "report-only mode") {
		t.Fatalf("expected a refusal, got %+v", last)
	}

	// After "allow changes" the same command runs.
	a.execute(ctx, mode(false))
	a.execute(ctx, proto.Command{ID: "j2", Type: proto.CmdAction, Feature: "updates", Action: "install", Params: json.RawMessage(`{"packages":["curl"]}`)})
	if !spy.called {
		t.Fatal("the install did not run after changes were allowed")
	}

	// Ping is answered in report-only mode too, and does nothing else.
	spy.called = false
	a.opt.Version = "9.9.9"
	cl.lastPin = "K1"
	cl.pins = [2]string{"K1", "K2"}
	a.execute(ctx, mode(true))
	a.execute(ctx, proto.Command{ID: "p", Type: proto.CmdPing})
	mu.Lock()
	last = results[len(results)-1]
	mu.Unlock()
	var pr proto.PingReply
	json.Unmarshal(last.Data, &pr)
	if !last.OK || last.CommandID != "p" || pr.Version != "9.9.9" || pr.KeyUsed != "K1" || pr.Keys != [2]string{"K1", "K2"} || pr.TimeMs == 0 || spy.called {
		t.Fatalf("ping reply: %+v %+v", last, pr)
	}
	if !immediate(proto.CmdPing) || immediate(proto.CmdAction) {
		t.Fatal("ping must be handled at once, actions not")
	}
}
