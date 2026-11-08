// Package modkit is the tiny SDK for writing coremon modules.
//
// A module is a standalone executable. agent-core invokes it as:
//
//	<module> manifest           -> prints the Manifest as JSON
//	<module> run <action>       -> params JSON on stdin, JSON-line events on stdout
//
// Each call is a fresh process, so modules hold no long-running state and
// cost nothing while idle.
package modkit

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"coremon/internal/proto"
)

// Ctx is passed to action handlers.
type Ctx struct {
	context.Context
	Params json.RawMessage
	mu     sync.Mutex
	out    *json.Encoder
}

// Log emits one log line to the job output.
func (c *Ctx) Log(format string, args ...any) {
	c.emit(proto.ModuleEvent{T: "log", D: fmt.Sprintf(format, args...)})
}

func (c *Ctx) emit(ev proto.ModuleEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.out.Encode(ev)
}

// Handler implements one action. The returned value is JSON encoded as the
// result data. By convention, a "state" key in the result is stored by the
// master as the module's current state for that system.
type Handler func(c *Ctx) (any, error)

// Main runs the module. It never returns.
func Main(m proto.Manifest, handlers map[string]Handler) {
	if len(os.Args) >= 2 && os.Args[1] == "manifest" {
		_ = json.NewEncoder(os.Stdout).Encode(m)
		os.Exit(0)
	}
	if len(os.Args) < 3 || os.Args[1] != "run" {
		fmt.Fprintf(os.Stderr, "usage: %s manifest | run <action>\n", m.Name)
		os.Exit(2)
	}
	action := os.Args[2]
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	params, _ := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	if len(strings.TrimSpace(string(params))) == 0 {
		params = []byte("{}")
	}
	c := &Ctx{Context: ctx, Params: params, out: json.NewEncoder(os.Stdout)}

	h, ok := handlers[action]
	if !ok {
		c.emit(proto.ModuleEvent{T: "result", OK: false, Error: "unknown action: " + action})
		os.Exit(1)
	}
	data, err := h(c)
	if err != nil {
		c.emit(proto.ModuleEvent{T: "result", OK: false, Error: err.Error()})
		os.Exit(1)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		c.emit(proto.ModuleEvent{T: "result", OK: false, Error: "encode result: " + err.Error()})
		os.Exit(1)
	}
	c.emit(proto.ModuleEvent{T: "result", OK: true, Data: raw})
	os.Exit(0)
}

// Run executes a command, streaming its combined output to the job log,
// and returns the combined output.
func (c *Ctx) Run(env []string, name string, args ...string) (string, error) {
	c.Log("$ %s %s", name, strings.Join(args, " "))
	cmd := exec.CommandContext(c, name, args...)
	cmd.Env = append(os.Environ(), env...)
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	var buf strings.Builder
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			buf.WriteString(line)
			buf.WriteByte('\n')
			c.Log("%s", line)
		}
	}()
	err := cmd.Run()
	pw.Close()
	<-done
	return buf.String(), err
}

// Output executes a command quietly and returns stdout.
func (c *Ctx) Output(env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(c, name, args...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	return string(out), err
}
