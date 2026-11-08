// Command coremon-master serves the Coremon web UI and agent hub.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"coremon/internal/master"
)

var (
	version   = "dev"
	buildDate = "" // YYYY-MM-DD, set by build.sh
)

func main() {
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
	user := fs.String("user", "admin", "reset-password: user name (created as administrator if missing)")
	target := fs.String("target", "/usr/local/bin/coremon-master", "apply-update: binary to replace")
	fs.Parse(args)
	log.SetFlags(0)

	switch cmd {
	case "version":
		fmt.Println(version)
	case "reset-password":
		pw, err := master.ResetPassword(*data, *user)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("New password for %s: %s\n(restart the master service: systemctl restart coremon-master)\n", *user, pw)
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
		fmt.Fprintf(os.Stderr, "usage: coremon-master [serve|apply-update|reset-password|version] [flags]\n")
		os.Exit(2)
	}
}
