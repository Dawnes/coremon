// Command coremon-sign manages the release signing key, signs binaries and
// maintains the signed version index.
//
//	coremon-sign keygen  -key keys/signing.key
//	coremon-sign pubkey  -key keys/signing.key
//	coremon-sign release -dir dist/.../release        # sign every binary below dir
//	coremon-sign index   -track agent -version 0.2.0 -dir dist/.../release \
//	                     -base https://github.com/o/r/releases/download/agent-v0.2.0 \
//	                     -in versions.json -out versions.json
//
// Instead of -key, the private key can be passed in the COREMON_SIGNING_KEY
// environment variable (used by the GitHub Actions release workflows).
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"coremon/internal/release"
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
	dir := fs.String("dir", "", "release directory")
	track := fs.String("track", "", "index: agent or master")
	version := fs.String("version", "", "index: version to add")
	base := fs.String("base", "", "index: download URL prefix of the release assets")
	in := fs.String("in", "versions.json", "index: current index (may be missing)")
	out := fs.String("out", "versions.json", "index: output file (a .sig is written next to it)")
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
		for _, tr := range []string{"agent", "master"} {
			kind, name, _ := release.TrackInfo(tr)
			files, _ := filepath.Glob(filepath.Join(*dir, kind, "*", "*", name))
			for _, f := range files {
				signFile(priv, f, kind, name)
				n++
			}
		}
		p := base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
		if err := os.WriteFile(filepath.Join(*dir, "signing.pub"), []byte(p+"\n"), 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("signed %d binaries; public key %s\n", n, p)
	case "index":
		if err := index(loadKey(*key), *track, *version, *dir, *base, *in, *out); err != nil {
			log.Fatal(err)
		}
	default:
		usage()
	}
}

// AssetName is the file name of a binary as a release asset.
func AssetName(track, version, plat string) string {
	_, name, _ := release.TrackInfo(track)
	return fmt.Sprintf("%s-%s-%s", name, version, plat)
}

func index(priv ed25519.PrivateKey, track, version, dir, base, in, out string) error {
	kind, name, err := release.TrackInfo(track)
	if err != nil {
		return err
	}
	local := strings.HasPrefix(base, "http://127.0.0.1:") // local testing only
	if !release.VersionRe.MatchString(version) || dir == "" || !(strings.HasPrefix(base, "https://") || local) {
		return errors.New("index needs -track, -version, -dir and an https -base")
	}
	pub := priv.Public().(ed25519.PublicKey)
	var idx release.Index
	if b, err := os.ReadFile(in); err == nil {
		// Only ever build on an index we signed ourselves.
		sig, err := os.ReadFile(in + ".sig")
		if err != nil {
			return fmt.Errorf("%s exists but %s.sig is missing", in, in)
		}
		cur, err := release.VerifyIndex(pub, b, string(sig))
		if err != nil {
			return fmt.Errorf("current index: %w", err)
		}
		idx = *cur
	} else if !os.IsNotExist(err) {
		return err
	}
	r := release.Release{Version: version, Published: time.Now().UTC().Format(time.RFC3339), Assets: map[string]release.Asset{}}
	files, _ := filepath.Glob(filepath.Join(dir, kind, version, "*", name))
	for _, f := range files {
		plat := filepath.Base(filepath.Dir(f))
		if !release.PlatRe.MatchString(plat) {
			continue
		}
		sum, err := sign.FileSHA256(f)
		if err != nil {
			return err
		}
		sig, err := os.ReadFile(f + ".sig")
		if err != nil {
			return fmt.Errorf("%s is not signed", f)
		}
		st, _ := os.Stat(f)
		r.Assets[plat] = release.Asset{URL: strings.TrimRight(base, "/") + "/" + AssetName(track, version, plat),
			Size: st.Size(), SHA256: sum, Signature: strings.TrimSpace(string(sig))}
	}
	if len(r.Assets) == 0 {
		return fmt.Errorf("no %s %s binaries below %s", track, version, dir)
	}
	if err := idx.Add(track, r); err != nil {
		return err
	}
	idx.Generated = r.Published
	b, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.WriteFile(out, b, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(out+".sig", []byte(release.SignIndex(priv, b)+"\n"), 0o644); err != nil {
		return err
	}
	list := idx.Agent
	if track == "master" {
		list = idx.Master
	}
	vs := []string{}
	for _, e := range list {
		vs = append(vs, e.Version)
	}
	plats := []string{}
	for p := range r.Assets {
		plats = append(plats, p)
	}
	sort.Strings(plats)
	fmt.Printf("index: added %s %s (%s); %s versions listed: %s\n", track, version, strings.Join(plats, ", "), track, strings.Join(vs, ", "))
	return nil
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
	fmt.Fprintln(os.Stderr, "usage: coremon-sign keygen|pubkey|release|index [flags]")
	os.Exit(2)
}
