package main

// coremon-reset: gives a user a new password (as root, on the master).

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const service = "coremon-master.service"

func resetMain(args []string) int {
	fs := flag.NewFlagSet("coremon-reset", flag.ExitOnError)
	user := fs.String("user", "admin", "user whose password is reset (must exist)")
	data := fs.String("data", "/var/lib/coremon-master", "master data directory")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: coremon-reset [-user name]\n\nGives the user (default: admin) a new random password, shown here only.\nIt must be changed at the next sign-in. The master restarts briefly.\n")
	}
	fs.Parse(args)
	fail := func(f string, a ...any) int { fmt.Fprintf(os.Stderr, "coremon-reset: "+f+"\n", a...); return 1 }
	if !isRoot() {
		return fail("run as root")
	}
	exe, err := os.Executable()
	if err != nil {
		return fail("%v", err)
	}
	child := exec.Command(exe, "reset-apply", "-user", *user, "-data", *data)
	// Files are changed as the master's user, never as root.
	if err := runAsOwner(child, *data); err != nil {
		return fail("%v", err)
	}
	var out, errOut bytes.Buffer
	child.Stdout, child.Stderr = &out, &errOut

	wasRunning := exec.Command("systemctl", "is-active", "--quiet", service).Run() == nil
	if wasRunning {
		fmt.Println("Stopping the master…")
		if err := exec.Command("systemctl", "stop", service).Run(); err != nil {
			return fail("stopping %s: %v", service, err)
		}
	}
	runErr := child.Run()
	if wasRunning {
		fmt.Println("Starting the master…")
		if err := exec.Command("systemctl", "start", service).Run(); err != nil {
			fmt.Fprintf(os.Stderr, "coremon-reset: starting %s failed: %v\n", service, err)
		}
	}
	if runErr != nil {
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = runErr.Error()
		}
		return fail("%s", msg)
	}
	pw := strings.TrimSpace(out.String())
	fmt.Printf("\nNew password for %s: %s\n\nIt must be changed at the next sign-in. All of %s's sessions have ended.\n", *user, pw, *user)
	return 0
}
