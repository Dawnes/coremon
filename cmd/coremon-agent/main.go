// Command coremon-agent is agent-core: a small service that connects out
// to the coremon master and runs signed modules on its behalf.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"coremon/internal/agent"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version)
		return
	}
	cfg := flag.String("config", "/etc/coremon/agent.json", "config file")
	state := flag.String("state", "/var/lib/coremon", "state directory")
	flag.Parse()
	log.SetFlags(0) // journald adds timestamps

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	code, err := agent.Run(ctx, agent.Options{ConfigPath: *cfg, StateDir: *state, Version: version})
	if err != nil {
		log.Printf("fatal: %v", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}
