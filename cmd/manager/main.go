package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/platinummonkey/agent-chat/internal/manager"
)

// Set via ldflags at build time.
var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("agent-manager %s (commit: %s, built: %s)\n", version, commit, buildTime)
		os.Exit(0)
	}

	cfg, err := manager.LoadConfig(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	if err := cfg.IRC.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "config validation error: %v\n", err)
		os.Exit(1)
	}

	mgr, err := manager.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating manager: %v\n", err)
		os.Exit(1)
	}

	printBanner(cfg)

	ctx := context.Background()
	if err := mgr.Run(ctx); err != nil {
		slog.Error("manager exited with error", "error", err)
		os.Exit(1)
	}
}

func printBanner(cfg *manager.ManagerConfig) {
	security := "TLS"
	if cfg.IRC.SASL {
		security += ", SASL " + cfg.IRC.SASLMech
	}

	channels := strings.Join(cfg.IRC.Channels, ", ")
	if channels == "" {
		channels = "(none)"
	}

	fmt.Printf("agent-manager %s (commit: %s)\n", version, commit)
	fmt.Printf("  IRC Server: %s (%s)\n", cfg.IRC.Server, security)
	fmt.Printf("  Nick:       %s\n", cfg.IRC.Nick)
	fmt.Printf("  Channels:   %s\n", channels)
	fmt.Printf("  Web UI:     http://localhost%s\n", cfg.ListenAddr)
	if cfg.AgentBinary != "" {
		fmt.Printf("  Agent bin:  %s\n", cfg.AgentBinary)
	}
	fmt.Println()
}
