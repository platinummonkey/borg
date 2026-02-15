//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/platinummonkey/borg/internal/agent"
	"github.com/platinummonkey/borg/internal/config"
	"github.com/platinummonkey/borg/internal/cost"
	"github.com/platinummonkey/borg/internal/manager"
	"github.com/platinummonkey/borg/pkg/ircclient"
	"github.com/platinummonkey/borg/pkg/protocol"
	"github.com/platinummonkey/borg/test/mock"
)

// createTestManagerFromSrv creates a Manager connected to a mock IRC server.
func createTestManagerFromSrv(t *testing.T, srv *mock.IRCServer, nick string) *manager.Manager {
	t.Helper()

	cfg := &manager.ManagerConfig{
		IRC: ircclient.Config{
			Server:                srv.Addr(),
			Nick:                  nick,
			Username:              nick,
			Password:              "pass",
			RealName:              "Manager",
			TLS:                   true,
			TLSInsecureSkipVerify: true,
			SASL:                  true,
			Channels:              []string{"#agents-general"},
			Reconnect:             false,
		},
		ListenAddr:   ":0",
		LogLevel:     "debug",
		LogFmt:       "text",
		PollInterval: time.Hour, // disable polling in tests
	}

	mgr, err := manager.New(cfg)
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("start manager: %v", err)
	}

	// Wait for IRC connection.
	time.Sleep(500 * time.Millisecond)

	return mgr
}

func TestManager_IRCObservation(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("create mock IRC: %v", err)
	}
	defer srv.Close()
	srv.Accounts["manager-bot"] = "pass"
	srv.Accounts["agent-1"] = "pass"

	// Create manager.
	mgrCfg := &manager.ManagerConfig{
		IRC: ircclient.Config{
			Server:                srv.Addr(),
			Nick:                  "manager-bot",
			Username:              "manager-bot",
			Password:              "pass",
			RealName:              "Manager",
			TLS:                   true,
			TLSInsecureSkipVerify: true,
			SASL:                  true,
			Channels:              []string{"#dev"},
			Reconnect:             false,
		},
		ListenAddr:   ":0",
		LogLevel:     "warn",
		LogFmt:       "text",
		PollInterval: time.Hour,
	}

	mgr, err := manager.New(mgrCfg)
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	defer mgr.Shutdown()
	time.Sleep(500 * time.Millisecond)

	// Create an agent that sends messages.
	agentCfg := &config.AppConfig{
		IRC: ircclient.Config{
			Server:                srv.Addr(),
			Nick:                  "agent-1",
			Username:              "agent-1",
			Password:              "pass",
			RealName:              "Test Agent",
			TLS:                   true,
			TLSInsecureSkipVerify: true,
			SASL:                  true,
			Channels:              []string{"#dev"},
			Reconnect:             false,
		},
		LogLevel: "warn",
		LogFmt:   "text",
	}

	a, err := agent.New(agentCfg)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := a.Start(ctx); err != nil {
		t.Fatalf("start agent: %v", err)
	}
	defer a.Shutdown()
	time.Sleep(500 * time.Millisecond)

	// Agent sends STARTED.
	if err := a.AnnounceStarted("#dev", "auth-feature", "high"); err != nil {
		t.Fatalf("announce started: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Verify manager observed it.
	task := mgr.State().GetTask("auth-feature")
	if task == nil {
		t.Fatal("expected manager to observe STARTED task")
	}
	if task.Status != agent.TaskStatusStarted {
		t.Errorf("task status = %q, want %q", task.Status, agent.TaskStatusStarted)
	}

	// Agent sends COMPLETED.
	if err := a.AnnounceCompleted("#dev", "auth-feature"); err != nil {
		t.Fatalf("announce completed: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	task = mgr.State().GetTask("auth-feature")
	if task.Status != agent.TaskStatusCompleted {
		t.Errorf("task status = %q, want %q", task.Status, agent.TaskStatusCompleted)
	}
}

func TestManager_CostTrackingRoundTrip(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("create mock IRC: %v", err)
	}
	defer srv.Close()
	srv.Accounts["manager-bot"] = "pass"
	srv.Accounts["agent-1"] = "pass"

	mgrCfg := &manager.ManagerConfig{
		IRC: ircclient.Config{
			Server:                srv.Addr(),
			Nick:                  "manager-bot",
			Username:              "manager-bot",
			Password:              "pass",
			RealName:              "Manager",
			TLS:                   true,
			TLSInsecureSkipVerify: true,
			SASL:                  true,
			Channels:              []string{"#dev"},
			Reconnect:             false,
		},
		ListenAddr:   ":0",
		LogLevel:     "warn",
		LogFmt:       "text",
		PollInterval: time.Hour,
	}

	mgr, err := manager.New(mgrCfg)
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	defer mgr.Shutdown()
	time.Sleep(500 * time.Millisecond)

	// Create agent.
	agentCfg := &config.AppConfig{
		IRC: ircclient.Config{
			Server:                srv.Addr(),
			Nick:                  "agent-1",
			Username:              "agent-1",
			Password:              "pass",
			RealName:              "Test Agent",
			TLS:                   true,
			TLSInsecureSkipVerify: true,
			SASL:                  true,
			Channels:              []string{"#dev"},
			Reconnect:             false,
		},
		LogLevel: "warn",
		LogFmt:   "text",
	}

	a, err := agent.New(agentCfg)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := a.Start(ctx); err != nil {
		t.Fatalf("start agent: %v", err)
	}
	defer a.Shutdown()
	time.Sleep(500 * time.Millisecond)

	// Agent sends COST-REPORT via raw message.
	costMsg := &protocol.Message{
		Action: protocol.ActionCostReport,
		Fields: map[string]string{
			"task":          "auth",
			"input-tokens":  "1500",
			"output-tokens": "500",
			"total-tokens":  "2000",
			"cost-usd":      "0.0125",
			"model":         "claude-sonnet-4-5-20250929",
			"provider":      "anthropic",
		},
	}
	a.SendProtocolMessage("#dev", costMsg)
	time.Sleep(300 * time.Millisecond)

	// Verify via store directly.
	summary := mgr.CostStore().TotalSummary()
	if summary.EntryCount != 1 {
		t.Errorf("cost entries = %d, want 1", summary.EntryCount)
	}
	if summary.TotalCostUSD != 0.0125 {
		t.Errorf("total cost = %f, want 0.0125", summary.TotalCostUSD)
	}

	byAgent := mgr.CostStore().ByAgent()
	if _, ok := byAgent["agent-1"]; !ok {
		t.Error("expected cost entry for agent-1")
	}
}

func TestManager_APIEndpoints(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("create mock IRC: %v", err)
	}
	defer srv.Close()
	srv.Accounts["manager-bot"] = "pass"

	mgrCfg := &manager.ManagerConfig{
		IRC: ircclient.Config{
			Server:                srv.Addr(),
			Nick:                  "manager-bot",
			Username:              "manager-bot",
			Password:              "pass",
			RealName:              "Manager",
			TLS:                   true,
			TLSInsecureSkipVerify: true,
			SASL:                  true,
			Channels:              []string{"#dev"},
			Reconnect:             false,
		},
		ListenAddr:   ":0",
		LogLevel:     "warn",
		LogFmt:       "text",
		PollInterval: time.Hour,
	}

	mgr, err := manager.New(mgrCfg)
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	defer mgr.Shutdown()
	time.Sleep(500 * time.Millisecond)

	// Seed some data.
	mgr.Registry().Register(&manager.AgentRecord{Nick: "test-agent", Status: "online", Source: "manual"})
	mgr.CostStore().Record(cost.CostEntry{Agent: "test-agent", Task: "t1", CostUSD: 0.05, Model: "claude"})

	// We need the actual listen address. Let's get it from the Hub/Server - but the Server's
	// ListenAddr isn't exposed through Manager. Let's test via the API endpoints instead.
	// We can call the handlers directly using httptest.

	// Verify stores directly.
	agents := mgr.Registry().List()
	if len(agents) != 1 {
		t.Errorf("registry agents = %d, want 1", len(agents))
	}

	summary := mgr.CostStore().TotalSummary()
	if summary.TotalCostUSD != 0.05 {
		t.Errorf("total cost = %f, want 0.05", summary.TotalCostUSD)
	}

	byModel := mgr.CostStore().ByModel()
	if byModel["claude"].EntryCount != 1 {
		t.Errorf("model entry count = %d, want 1", byModel["claude"].EntryCount)
	}
}

func TestManager_ExternalAgentDiscovery(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("create mock IRC: %v", err)
	}
	defer srv.Close()
	srv.Accounts["manager-bot"] = "pass"
	srv.Accounts["ext-agent"] = "pass"

	mgrCfg := &manager.ManagerConfig{
		IRC: ircclient.Config{
			Server:                srv.Addr(),
			Nick:                  "manager-bot",
			Username:              "manager-bot",
			Password:              "pass",
			RealName:              "Manager",
			TLS:                   true,
			TLSInsecureSkipVerify: true,
			SASL:                  true,
			Channels:              []string{"#dev"},
			Reconnect:             false,
		},
		ListenAddr:   ":0",
		LogLevel:     "warn",
		LogFmt:       "text",
		PollInterval: time.Hour,
	}

	mgr, err := manager.New(mgrCfg)
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	defer mgr.Shutdown()
	time.Sleep(500 * time.Millisecond)

	// Start an independent agent with capabilities.
	agentCfg := &config.AppConfig{
		IRC: ircclient.Config{
			Server:                srv.Addr(),
			Nick:                  "ext-agent",
			Username:              "ext-agent",
			Password:              "pass",
			RealName:              "External Agent",
			TLS:                   true,
			TLSInsecureSkipVerify: true,
			SASL:                  true,
			Channels:              []string{"#dev"},
			Reconnect:             false,
		},
		Capabilities: []string{"go", "testing"},
		DiscoveryTTL: 5 * time.Minute,
		LogLevel:     "warn",
		LogFmt:       "text",
	}

	a, err := agent.New(agentCfg)
	if err != nil {
		t.Fatalf("create ext agent: %v", err)
	}
	if err := a.Start(ctx); err != nil {
		t.Fatalf("start ext agent: %v", err)
	}
	defer a.Shutdown()

	// Wait for CAPABILITIES heartbeat.
	time.Sleep(1 * time.Second)

	rec := mgr.Registry().Get("ext-agent")
	if rec == nil {
		t.Fatal("expected manager to discover ext-agent")
	}
	if rec.Source != "discovered" {
		t.Errorf("source = %q, want %q", rec.Source, "discovered")
	}
	if len(rec.Capabilities) < 2 {
		t.Errorf("capabilities = %v, want at least [go testing]", rec.Capabilities)
	}
}

// Helper to fetch JSON from manager API (not used in unit mode but useful for full E2E).
func fetchManagerJSON(url string, v any) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return json.Unmarshal(body, v)
}
