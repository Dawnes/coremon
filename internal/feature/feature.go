// Package feature is the framework for the agent's built-in functions.
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

type Feature interface {
	Info() proto.FeatureInfo

	Call(c *Ctx, action string) (any, error)
}

type Ctx struct {
	context.Context
	Params json.RawMessage
	logf   func(string)
}

func NewCtx(ctx context.Context, params json.RawMessage, logf func(string)) *Ctx {
	if len(strings.TrimSpace(string(params))) == 0 {
		params = json.RawMessage("{}")
	}
	return &Ctx{Context: ctx, Params: params, logf: logf}
}

func (c *Ctx) Log(format string, args ...any) { c.logf(fmt.Sprintf(format, args...)) }

const sysPath = "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

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

func (c *Ctx) Output(env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(c, name, args...)
	cmd.Env = append(append(os.Environ(), sysPath), env...)
	out, err := cmd.Output()
	return string(out), err
}

func CollapseCR(line string) string {
	line = strings.TrimRight(line, "\r")
	if i := strings.LastIndexByte(line, '\r'); i >= 0 {
		line = line[i+1:]
	}
	return line
}
