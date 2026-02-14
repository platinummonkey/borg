package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/platinummonkey/agent-chat/internal/agent"
	"github.com/platinummonkey/agent-chat/internal/config"
)

// Set via ldflags at build time.
var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("agent-chat %s (commit: %s, built: %s)\n", version, commit, buildTime)
		os.Exit(0)
	}

	cfg, err := config.Load(os.Args[1:])
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

	a, err := agent.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating agent: %v\n", err)
		os.Exit(1)
	}

	printBanner(cfg)

	ctx := context.Background()
	if err := a.Run(ctx); err != nil {
		slog.Error("agent exited with error", "error", err)
		os.Exit(1)
	}
}

func printBanner(cfg *config.AppConfig) {
	security := "TLS"
	if cfg.IRC.SASL {
		security += ", SASL " + cfg.IRC.SASLMech
	}

	channels := strings.Join(cfg.IRC.Channels, ", ")
	if channels == "" {
		channels = "(none)"
	}

	fmt.Printf("agent-chat %s (commit: %s)\n", version, commit)
	fmt.Printf("  Server:     %s (%s)\n", cfg.IRC.Server, security)
	fmt.Printf("  Nick:       %s\n", cfg.IRC.Nick)
	fmt.Printf("  Channels:   %s\n", channels)
	if cfg.IRC.RateLimit > 0 {
		fmt.Printf("  Rate limit: %.1f msg/s (burst: %d)\n", cfg.IRC.RateLimit, cfg.IRC.RateLimitBurst)
	}
	if cfg.DashboardAddr != "" {
		fmt.Printf("  Dashboard:  http://localhost%s\n", cfg.DashboardAddr)
	}
	fmt.Println()
}
