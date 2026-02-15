// Package main demonstrates multi-agent coordination through an in-process mock IRC server.
// It spawns 4 agents that communicate task dependencies, share context, and resolve
// a dependency chain — all observable via notification channels.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/platinummonkey/borg/internal/agent"
	"github.com/platinummonkey/borg/internal/config"
	"github.com/platinummonkey/borg/pkg/ircclient"
	"github.com/platinummonkey/borg/test/mock"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	srv, err := mock.NewIRCServer()
	if err != nil {
		return fmt.Errorf("start mock IRC server: %w", err)
	}
	defer srv.Close()

	// Register accounts.
	srv.Accounts["alice"] = "alice-pass"
	srv.Accounts["bob"] = "bob-pass"
	srv.Accounts["carol"] = "carol-pass"
	srv.Accounts["dave"] = "dave-pass"

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	printPhase("Multi-Agent Coordination Demo")

	channels := []string{"#project-webapp", "#ops-alerts", "#help-desk"}
	alice := createDemoAgent(srv, "alice-1", "alice", "alice-pass", channels)
	bob := createDemoAgent(srv, "bob-2", "bob", "bob-pass", channels)
	carol := createDemoAgent(srv, "carol-3", "carol", "carol-pass", channels)
	dave := createDemoAgent(srv, "dave-4", "dave", "dave-pass", channels)

	agents := []*agent.Agent{alice, bob, carol, dave}
	names := []string{"alice-1", "bob-2", "carol-3", "dave-4"}
	for i, a := range agents {
		if err := a.Start(ctx); err != nil {
			return fmt.Errorf("start %s: %w", names[i], err)
		}
		defer a.Shutdown()
	}
	time.Sleep(300 * time.Millisecond)

	// Configure dave as the observer with notification rules.
	dave.NotifyCompletionsTo("#ops-alerts")
	dave.NotifyBlockedTo("#ops-alerts")
	dave.NotifyHelpTo("#help-desk")

	// === Phase 1: Task Announcements ===
	printPhase("Phase 1: Task Announcements")
	if err := alice.AnnounceStarted("#project-webapp", "db-migration", "high", "infrastructure"); err != nil {
		return err
	}
	printAction("alice-1", "STARTED db-migration (priority: high)")

	if err := bob.AnnounceStarted("#project-webapp", "api-service", "high", "backend"); err != nil {
		return err
	}
	printAction("bob-2", "STARTED api-service (priority: high)")

	if err := carol.AnnounceStarted("#project-webapp", "frontend", "medium", "ui"); err != nil {
		return err
	}
	printAction("carol-3", "STARTED frontend (priority: medium)")
	time.Sleep(200 * time.Millisecond)

	// === Phase 2: Dependencies ===
	printPhase("Phase 2: Dependency Declarations")
	if err := bob.AnnounceBlocked("#project-webapp", "api-service", "db-migration", "blocked-by-db-migration"); err != nil {
		return err
	}
	printAction("bob-2", "BLOCKED api-service (waiting for: db-migration)")

	if err := carol.AnnounceBlocked("#project-webapp", "frontend", "api-service", "blocked-by-api-service"); err != nil {
		return err
	}
	printAction("carol-3", "BLOCKED frontend (waiting for: api-service)")
	time.Sleep(200 * time.Millisecond)

	// Show transitive dependencies.
	deps := alice.State().TransitiveDependencies("frontend")
	printAction("state", fmt.Sprintf("frontend transitive deps: %v", deps))

	// === Phase 3: Context Sharing ===
	printPhase("Phase 3: Context Sharing")
	if err := alice.ShareContext("#project-webapp", "database", "webapp", "migrated-to-v3"); err != nil {
		return err
	}
	printAction("alice-1", "CONTEXT database project=webapp status=migrated-to-v3")
	time.Sleep(200 * time.Millisecond)

	if err := bob.RequestContext("#project-webapp", "database"); err != nil {
		return err
	}
	printAction("bob-2", "REQUEST-CONTEXT database")
	time.Sleep(300 * time.Millisecond)

	entry := bob.ContextEntries().Get("database")
	if entry != nil {
		printAction("bob-2", fmt.Sprintf("Received context: component=%s status=%s", entry.Component, entry.Status))
	}

	// === Phase 4: Help Request ===
	printPhase("Phase 4: Help Request")
	if err := carol.RequestHelp("#project-webapp", "css-layout", "css", "need-review"); err != nil {
		return err
	}
	printAction("carol-3", "HELP-NEEDED css-layout (expertise: css)")
	time.Sleep(200 * time.Millisecond)

	// === Phase 5: Dependency Chain Resolution ===
	printPhase("Phase 5: Dependency Chain Resolution")
	if err := alice.AnnounceCompleted("#project-webapp", "db-migration", "schema-ready"); err != nil {
		return err
	}
	printAction("alice-1", "COMPLETED db-migration")
	time.Sleep(500 * time.Millisecond)

	unblocked := alice.State().UnblockedTasks()
	for _, t := range unblocked {
		printAction(">>", fmt.Sprintf("%s UNBLOCKED (was waiting for db-migration)", t.Name))
	}

	if err := bob.AnnounceCompleted("#project-webapp", "api-service", "endpoints-ready"); err != nil {
		return err
	}
	printAction("bob-2", "COMPLETED api-service")
	time.Sleep(500 * time.Millisecond)

	unblocked = bob.State().UnblockedTasks()
	for _, t := range unblocked {
		printAction(">>", fmt.Sprintf("%s UNBLOCKED (was waiting for api-service)", t.Name))
	}

	if err := carol.AnnounceCompleted("#project-webapp", "frontend", "ui-complete"); err != nil {
		return err
	}
	printAction("carol-3", "COMPLETED frontend")
	time.Sleep(300 * time.Millisecond)

	// === Phase 6: Summary ===
	printPhase("Summary")
	printSummary(agents, names)

	return nil
}

func createDemoAgent(srv *mock.IRCServer, nick, username, password string, channels []string) *agent.Agent {
	cfg := &config.AppConfig{
		IRC: ircclient.Config{
			Server:                srv.Addr(),
			Nick:                  nick,
			Username:              username,
			Password:              password,
			RealName:              "Demo Agent",
			TLS:                   true,
			TLSInsecureSkipVerify: true,
			SASL:                  true,
			Channels:              channels,
		},
		LogLevel: "error",
	}
	client, err := ircclient.NewClient(cfg.IRC)
	if err != nil {
		panic(fmt.Sprintf("NewClient for %s failed: %v", nick, err))
	}
	return agent.NewWithClient(cfg, client)
}

func printPhase(name string) {
	fmt.Printf("\n=== %s ===\n", name)
}

func printAction(nick, msg string) {
	fmt.Printf("  [%s] %s\n", nick, msg)
}

func printSummary(agents []*agent.Agent, names []string) {
	fmt.Println("Tasks:")
	tasks := agents[0].State().ListTasks()
	for _, t := range tasks {
		fmt.Printf("  %-25s %-12s (%s)\n", t.Name, t.Status, t.LastAgent)
	}

	fmt.Println("\nDependencies:")
	allDeps := agents[0].State().AllDependencies()
	for _, d := range allDeps {
		resolved := "unresolved"
		if d.Resolved {
			resolved = "resolved"
		}
		fmt.Printf("  %s -> %s [%s]\n", d.Blocked, d.BlockedBy, resolved)
	}

	fmt.Println("\nMetrics (per agent):")
	for i, a := range agents {
		snap := a.Metrics().Snapshot()
		parts := []string{
			fmt.Sprintf("sent=%d", snap.MessagesSent),
			fmt.Sprintf("recv=%d", snap.ProtocolMessagesIn),
			fmt.Sprintf("started=%d", snap.TasksStarted),
			fmt.Sprintf("completed=%d", snap.TasksCompleted),
			fmt.Sprintf("blocked=%d", snap.TasksBlocked),
		}
		fmt.Printf("  %-10s %s\n", names[i], strings.Join(parts, " "))
	}
}
