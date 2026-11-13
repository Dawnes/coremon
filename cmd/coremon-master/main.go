// Command coremon-master serves the Coremon website and the agent hub.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"coremon/internal/master"
)

var (
	version   = "dev"
	buildDate = ""
)

func main() {
	if strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") == "coremon-reset" {
		os.Exit(resetMain(os.Args[1:]))
	}
	cmd := "serve"
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	listen := fs.String("listen", ":8443", "listen address (HTTPS, used by agents and the UI)")
	httpListen := fs.String("http", "", "optional plain-HTTP UI listener for a local reverse proxy; loopback only, e.g. 127.0.0.1:8080")
	data := fs.String("data", "/var/lib/coremon-master", "data directory")
	release := fs.String("release", "", "release directory (default <data>/release)")
	sans := fs.String("san", "", "extra comma-separated DNS names/IPs for the TLS certificate (first start only)")
	key := fs.String("signing-key", "/etc/coremon-master/signing.pub", "release signing public key (root-owned)")
	github := fs.String("github", os.Getenv("COREMON_GITHUB_REPO"), "GitHub repository (owner/name) publishing the version index; first start only, then editable in Settings")
	indexURL := fs.String("index-url", "", "override the version index URL (testing)")
	target := fs.String("target", "/usr/local/bin/coremon-master", "apply-update: binary to replace")
	user := fs.String("user", "admin", "reset-apply: user name")
	fs.Parse(args)
	log.SetFlags(0)

	switch cmd {
	case "version":
		fmt.Println(version)
	case "reset-info":
		info, err := master.ReadAccountInfo(*data, *user)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		json.NewEncoder(os.Stdout).Encode(info)
	case "reset-apply":
		pw, err := master.ResetPassword(*data, *user)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(pw)
	case "show-state":
		if !isRoot() {
			log.Fatal("show-state: run as root")
		}
		out, err := master.ShowState(*data)
		if err != nil {
			log.Fatalf("show-state: %v", err)
		}
		os.Stdout.Write(out)
	case "apply-update":
		if err := master.ApplyUpdate(*data, *key, *target, version); err != nil {
			log.Fatalf("apply-update: %v", err)
		}
	case "serve":
		var sanList []string
		if *sans != "" {
			sanList = strings.Split(*sans, ",")
		}
		s, err := master.New(master.Config{
			Listen: *listen, HTTPListen: *httpListen, DataDir: *data, ReleaseDir: *release,
			SANs: sanList, Version: version, BuildDate: buildDate, SigningKeyPath: *key, GitHubRepo: *github, IndexURL: *indexURL,
		})
		if err != nil {
			log.Fatal(err)
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		if err := s.Run(ctx); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Fprintf(os.Stderr, "usage: coremon-master [serve|apply-update|show-state|version] [flags]\n(password reset: coremon-reset)\n")
		os.Exit(2)
	}
}
