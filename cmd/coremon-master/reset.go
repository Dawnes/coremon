package main

// coremon-reset: gives a user a new password (as root, on the master).

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"coremon/internal/master"
)

const service = "coremon-master.service"

const resetHelp = `Options:
  -user <name>   reset another user (default: admin)
  -y             do not ask; confirm in advance (for scripts)
  -data <dir>    data folder (default: /var/lib/coremon-master)

Examples:
  coremon-reset                 new password for admin
  coremon-reset -user john      new password for john
  coremon-reset -user john -y   the same, without the question
`

func resetMain(args []string) int {
	return resetRun(args, os.Stdin, os.Stdout, stdinIsTerminal())
}

func stdinIsTerminal() bool {
	st, err := os.Stdin.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func resetRun(args []string, in io.Reader, out io.Writer, terminal bool) int {
	fs := flag.NewFlagSet("coremon-reset", flag.ContinueOnError)
	fs.SetOutput(out)
	user := fs.String("user", "admin", "user whose password is reset")
	data := fs.String("data", "/var/lib/coremon-master", "data folder")
	yes := fs.Bool("y", false, "confirm in advance")
	fs.Usage = func() {
		fmt.Fprintf(out, "coremon-reset gives a user of the Coremon website a new password.\n\n%s", resetHelp)
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	fail := func(f string, a ...any) int { fmt.Fprintf(os.Stderr, "coremon-reset: "+f+"\n", a...); return 1 }
	if !isRoot() {
		return fail("run as root")
	}
	exe, err := os.Executable()
	if err != nil {
		return fail("%v", err)
	}
	// Files are read and changed as the master's user, never as root.
	child := func(cmd string) (string, string, error) {
		c := exec.Command(exe, cmd, "-user", *user, "-data", *data)
		if err := runAsOwner(c, *data); err != nil {
			return "", "", err
		}
		var o, e bytes.Buffer
		c.Stdout, c.Stderr = &o, &e
		err := c.Run()
		return o.String(), strings.TrimSpace(e.String()), err
	}
	o, e, err := child("reset-info")
	if err != nil {
		if e == "" {
			e = err.Error()
		}
		return fail("%s", e)
	}
	var info master.AccountInfo
	if err := json.Unmarshal([]byte(o), &info); err != nil {
		return fail("cannot read the account: %v", err)
	}
	if !info.Exists {
		return fail("there is no user %q; administrators: %s\n\n%s", *user, strings.Join(info.Admins, ", "), resetHelp)
	}

	fmt.Fprint(out, explainReset(info))
	fmt.Fprintf(out, "\n%s\n", resetHelp)
	if !*yes {
		if !terminal {
			fmt.Fprintln(out, "Nothing changed: there is no one to answer the question. Run it in a terminal, or add -y to confirm in advance.")
			return 1
		}
		fmt.Fprint(out, "Continue? [y/N] ")
		answer, _ := bufio.NewReader(in).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
			fmt.Fprintln(out, "Nothing changed.")
			return 1
		}
	}

	wasRunning := exec.Command("systemctl", "is-active", "--quiet", service).Run() == nil
	if wasRunning {
		fmt.Fprintln(out, "Stopping the master…")
		if err := exec.Command("systemctl", "stop", service).Run(); err != nil {
			return fail("stopping %s: %v", service, err)
		}
	}
	o, e, runErr := child("reset-apply")
	if wasRunning {
		fmt.Fprintln(out, "Starting the master…")
		if err := exec.Command("systemctl", "start", service).Run(); err != nil {
			fmt.Fprintf(os.Stderr, "coremon-reset: starting %s failed: %v\n", service, err)
		}
	}
	if runErr != nil {
		if e == "" {
			e = runErr.Error()
		}
		return fail("%s", e)
	}
	fmt.Fprintf(out, "\nNew password for %s: %s\n\nIt must be changed at the next sign-in.\n", info.Name, strings.TrimSpace(o))
	return 0
}

func explainReset(info master.AccountInfo) string {
	role := "read-only user"
	if info.Role == "admin" {
		role = "administrator"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "coremon-reset will do the following for %s (%s):\n\n", info.Name, role)
	b.WriteString("  • give it a new random password, shown here once;\n    it must be changed at the next sign-in\n")
	b.WriteString("  • sign it out in every browser\n")
	if info.Locked {
		b.WriteString("  • unlock the account (it is locked now)\n")
	}
	if info.TwoFA {
		b.WriteString("  • turn off two-factor authentication (it is on now)\n")
	}
	b.WriteString("  • restart the master; the website is unavailable for a few seconds\n")
	return b.String()
}
