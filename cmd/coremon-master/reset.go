package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"coremon/internal/master"
)

const service = "coremon-master.service"

// resetMain is coremon-reset: gives an existing user (default admin) a new
// random password that must be changed at the next sign-in. Briefly stops
// and starts the master; the password is only shown on screen.
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
	if _, err := os.Stat(*data); err != nil {
		return fail("%v", err)
	}
	wasRunning := exec.Command("systemctl", "is-active", "--quiet", service).Run() == nil
	if wasRunning {
		fmt.Println("Stopping the master…")
		if err := exec.Command("systemctl", "stop", service).Run(); err != nil {
			return fail("stopping %s: %v", service, err)
		}
	}
	start := func() {
		if wasRunning {
			fmt.Println("Starting the master…")
			if err := exec.Command("systemctl", "start", service).Run(); err != nil {
				fmt.Fprintf(os.Stderr, "coremon-reset: starting %s failed: %v\n", service, err)
			}
		}
	}
	pw, err := master.ResetPassword(*data, *user)
	if err != nil {
		start()
		return fail("%v", err)
	}
	// Written as root: give the files back to the master's own user.
	for _, f := range []string{"state.dat", "audit.jsonl"} {
		chownLike(filepath.Join(*data, f), *data)
	}
	start()
	fmt.Printf("\nNew password for %s: %s\n\nIt must be changed at the next sign-in. All of %s's sessions have ended.\n", *user, pw, *user)
	return 0
}
