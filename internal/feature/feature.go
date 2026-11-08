// Package feature is the framework for the agent's built-in functions.
//
// Everything the agent can do ships inside the one agent binary. Each
// function lives in its own package (for example feature/updates) and
// implements Feature; the agent lists them in agent.features.
package feature

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"coremon/internal/proto"
)

// Feature is one built-in function of the agent.
type Feature interface {
	Info() proto.FeatureInfo
	// Call runs one action. The returned value is JSON encoded as the
	// result data; a "state" key in it is stored by the master as this
	// feature's current state for the system.
	Call(c *Ctx, action string) (any, error)
}

// Ctx is passed to actions.
type Ctx struct {
	context.Context
	Params json.RawMessage
	logf   func(string)
}

// NewCtx creates a context whose log lines go to logf.
func NewCtx(ctx context.Context, params json.RawMessage, logf func(string)) *Ctx {
	if len(strings.TrimSpace(string(params))) == 0 {
		params = json.RawMessage("{}")
	}
	return &Ctx{Context: ctx, Params: params, logf: logf}
}

// Log emits one line to the job output.
func (c *Ctx) Log(format string, args ...any) { c.logf(fmt.Sprintf(format, args...)) }

// sysPath is used for commands so they behave the same under systemd.
const sysPath = "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

// Run executes a command, streaming its combined output to the job log,
// and returns the combined output.
func (c *Ctx) Run(env []string, name string, args ...string) (string, error) {
	c.Log("$ %s %s", name, strings.Join(args, " "))
	cmd := exec.CommandContext(c, name, args...)
	cmd.Env = append(append(os.Environ(), sysPath), env...)
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
			line := CollapseCR(sc.Text())
			buf.WriteString(line)
			buf.WriteByte('\n')
			c.Log("%s", line)
		}
		io.Copy(io.Discard, pr)
	}()
	err := cmd.Run()
	pw.Close()
	<-done
	return buf.String(), err
}

// Output executes a command quietly and returns stdout.
func (c *Ctx) Output(env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(c, name, args...)
	cmd.Env = append(append(os.Environ(), sysPath), env...)
	out, err := cmd.Output()
	return string(out), err
}

// CollapseCR keeps only the final state of a line that progress output
// (apt's "Reading database ... 5%\r...") rewrites with \r, like a terminal.
func CollapseCR(line string) string {
	line = strings.TrimRight(line, "\r")
	if i := strings.LastIndexByte(line, '\r'); i >= 0 {
		line = line[i+1:]
	}
	return line
}
