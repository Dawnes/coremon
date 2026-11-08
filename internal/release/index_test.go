package release

import (
	"encoding/json"
	"testing"

	"coremon/internal/sign"
)

func TestAddKeepsNewestThree(t *testing.T) {
	var idx Index
	for _, v := range []string{"0.2.0", "0.10.0", "0.3.0", "0.2.1", "0.3.0"} {
		if err := idx.Add("agent", Release{Version: v}); err != nil {
			t.Fatal(err)
		}
	}
	got := []string{}
	for _, r := range idx.Agent {
		got = append(got, r.Version)
	}
	want := []string{"0.10.0", "0.3.0", "0.2.1"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("got %v, want %v", got, want)
	}
	if err := idx.Add("agent", Release{Version: "1.0"}); err == nil {
		t.Fatal("invalid version must be rejected")
	}
}

func TestIndexSignature(t *testing.T) {
	pub, priv, _ := sign.GenerateKey()
	_, otherPriv, _ := sign.GenerateKey()
	k, _ := sign.ParsePrivate(priv)
	ok, _ := sign.ParsePublic(pub)
	other, _ := sign.ParsePrivate(otherPriv)
	idx := Index{}
	idx.Add("master", Release{Version: "0.2.0"})
	b, _ := json.Marshal(idx)
	if _, err := VerifyIndex(ok, b, SignIndex(k, b)); err != nil {
		t.Fatalf("valid index rejected: %v", err)
	}
	if _, err := VerifyIndex(ok, b, SignIndex(other, b)); err == nil {
		t.Fatal("index signed with another key must be rejected")
	}
	tampered := append([]byte(nil), b...)
	tampered[len(tampered)-2] = ' '
	if _, err := VerifyIndex(ok, tampered, SignIndex(k, b)); err == nil {
		t.Fatal("tampered index must be rejected")
	}
}
