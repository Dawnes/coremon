// Command coremon-sign manages the release signing key and signs artifacts.
//
//	coremon-sign keygen  -key keys/signing.key          # create a new key pair
//	coremon-sign pubkey  -key keys/signing.key          # print the public key
//	coremon-sign release -key keys/signing.key -dir dist/coremon-X/release
//
// Instead of -key, the private key can be passed in the COREMON_SIGNING_KEY
// environment variable (used by the GitHub Actions release workflow).
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"coremon/internal/sign"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	log.SetFlags(0)
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	key := fs.String("key", "keys/signing.key", "private key file (or set COREMON_SIGNING_KEY)")
	pub := fs.String("pub", "keys/signing.pub", "public key file (keygen)")
	dir := fs.String("dir", "", "release directory to sign")
	fs.Parse(os.Args[2:])

	switch os.Args[1] {
	case "keygen":
		if _, err := os.Stat(*key); err == nil {
			log.Fatalf("%s already exists; refusing to overwrite", *key)
		}
		p, k, err := sign.GenerateKey()
		if err != nil {
			log.Fatal(err)
		}
		os.MkdirAll(filepath.Dir(*key), 0o700)
		if err := os.WriteFile(*key, []byte(k+"\n"), 0o600); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(*pub, []byte(p+"\n"), 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("wrote %s and %s\nKEEP %s SECRET AND BACKED UP - agents only accept code signed with it.\n", *key, *pub, *key)
	case "pubkey":
		priv := loadKey(*key)
		fmt.Println(base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey)))
	case "release":
		if *dir == "" {
			log.Fatal("-dir is required")
		}
		priv := loadKey(*key)
		n := 0
		glob := func(parts ...string) []string {
			m, _ := filepath.Glob(filepath.Join(append([]string{*dir}, parts...)...))
			return m
		}
		for _, f := range glob("core", "*", "*", "coremon-agent") {
			signFile(priv, f, "core", "coremon-agent")
			n++
		}
		for _, f := range glob("master", "*", "*", "coremon-master") {
			signFile(priv, f, "master", "coremon-master")
			n++
		}
		for _, f := range glob("modules", "*", "*", "*", "*") {
			name := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(f))))
			if filepath.Base(f) != name {
				continue
			}
			signFile(priv, f, "module", name)
			n++
		}
		p := base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
		if err := os.WriteFile(filepath.Join(*dir, "signing.pub"), []byte(p+"\n"), 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("signed %d artifact(s); public key %s\n", n, p)
	default:
		usage()
	}
}

func loadKey(path string) ed25519.PrivateKey {
	s := os.Getenv("COREMON_SIGNING_KEY")
	if s == "" {
		b, err := os.ReadFile(path)
		if err != nil {
			log.Fatalf("no signing key: set COREMON_SIGNING_KEY or create %s (coremon-sign keygen)", path)
		}
		s = string(b)
	}
	k, err := sign.ParsePrivate(s)
	if err != nil {
		log.Fatal(err)
	}
	return k
}

// signFile signs <dir>/<ver>/<os>-<arch>/<file>.
func signFile(priv ed25519.PrivateKey, file, kind, name string) {
	plat := filepath.Base(filepath.Dir(file))
	ver := filepath.Base(filepath.Dir(filepath.Dir(file)))
	goos, goarch, ok := strings.Cut(plat, "-")
	if !ok {
		log.Fatalf("bad platform dir %q", plat)
	}
	sum, err := sign.FileSHA256(file)
	if err != nil {
		log.Fatal(err)
	}
	sig := sign.Sign(priv, sign.Message(kind, name, ver, goos, goarch, sum))
	if err := os.WriteFile(file+".sig", []byte(sig+"\n"), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  %-7s %-15s %-10s %s\n", kind, name, ver, plat)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: coremon-sign keygen|pubkey|release [-key file] [-dir release-dir]")
	os.Exit(2)
}
