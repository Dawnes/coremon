package main

import (
	"strings"
	"testing"

	"coremon/internal/master"
)

func TestExplainReset(t *testing.T) {
	plain := explainReset(master.AccountInfo{Exists: true, Name: "john", Role: "viewer"})
	if !strings.Contains(plain, "for john (read-only user)") || strings.Contains(plain, "unlock") || strings.Contains(plain, "two-factor") {
		t.Errorf("plain account:\n%s", plain)
	}
	full := explainReset(master.AccountInfo{Exists: true, Name: "admin", Role: "admin", Locked: true, TwoFA: true})
	if !strings.Contains(full, "unlock the account (it is locked now)") || !strings.Contains(full, "turn off two-factor authentication (it is on now)") {
		t.Errorf("locked account with 2FA:\n%s", full)
	}
	if !strings.Contains(resetHelp, "-user <name>") || !strings.Contains(resetHelp, "-y ") || !strings.Contains(resetHelp, "-data <dir>") {
		t.Error("options missing from the help")
	}
}
