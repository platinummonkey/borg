//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/platinummonkey/agent-chat/internal/agent"
	"github.com/platinummonkey/agent-chat/internal/config"
	"github.com/platinummonkey/agent-chat/pkg/ircclient"
	"github.com/platinummonkey/agent-chat/pkg/protocol"
	"github.com/platinummonkey/agent-chat/test/mock"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// createTestAgent creates an agent connected to a mock IRC server using only
// exported API. Channels are auto-joined via the IRC config.
func createTestAgent(
	t *testing.T,
	srv *mock.IRCServer,
	nick, username, password string,
	channels []string,
	opts ...func(*config.AppConfig),
) *agent.Agent {
	t.Helper()

	cfg := &config.AppConfig{
		IRC: ircclient.Config{
			Server:                srv.Addr(),
			Nick:                  nick,
			Username:              username,
			Password:              password,
			RealName:              "Test Agent",
			TLS:                   true,
			TLSInsecureSkipVerify: true,
			SASL:                  true,
			Channels:              channels,
		},
		LogLevel: "error",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	client, err := ircclient.NewClient(cfg.IRC)
	if err != nil {
		t.Fatalf("NewClient for %s failed: %v", nick, err)
	}

	return agent.NewWithClient(cfg, client)
}

// getFreePort returns a free TCP port on localhost.
func getFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("get free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// ---------- Phase 7: Persistence ----------

// TestE2E_Persistence verifies that task/dependency state is saved to a JSON file
// on agent shutdown and correctly restored when a new agent starts with the same file.
func TestE2E_Persistence(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	srv.Accounts["alice"] = "pass1"

	stateFile := filepath.Join(t.TempDir(), "agent-state.json")

	ag := createTestAgent(t, srv, "alice-1", "alice", "pass1", []string{"#project"},
		func(cfg *config.AppConfig) {
			cfg.StateFile = stateFile
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := ag.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	time.Sleep(300 * time.Millisecond) // wait for channel join

	// Send state-changing messages (agent's own sends update local state).
	if err := ag.AnnounceStarted("#project", "db-migration", "high"); err != nil {
		t.Fatalf("AnnounceStarted: %v", err)
	}
	if err := ag.AnnounceBlocked("#project", "api-service", "db-migration", "blocked-by-db-migration"); err != nil {
		t.Fatalf("AnnounceBlocked: %v", err)
	}
	if err := ag.AnnounceCompleted("#project", "db-migration"); err != nil {
		t.Fatalf("AnnounceCompleted: %v", err)
	}
	time.Sleep(200 * time.Millisecond) // let persistence debounce

	// Shutdown flushes persistence.
	ag.Shutdown()

	// Verify state file exists and contains tasks.
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}

	var ps agent.PersistedState
	if err := json.Unmarshal(data, &ps); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	if len(ps.Tasks) < 2 {
		t.Errorf("expected >= 2 tasks in persisted state, got %d: %+v", len(ps.Tasks), ps.Tasks)
	}
	if len(ps.Dependencies) < 1 {
		t.Errorf("expected >= 1 dependency in persisted state, got %d", len(ps.Dependencies))
	}

	// Restore: create a new agent with the same state file.
	ag2 := createTestAgent(t, srv, "alice-2", "alice", "pass1", []string{"#project"},
		func(cfg *config.AppConfig) {
			cfg.StateFile = stateFile
		},
	)

	ctx2, cancel2 := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel2()

	if err := ag2.Start(ctx2); err != nil {
		t.Fatalf("ag2 Start failed: %v", err)
	}
	defer ag2.Shutdown()

	// Verify restored state.
	tasks := ag2.State().ListTasks()
	if len(tasks) < 2 {
		t.Errorf("restored agent has %d tasks, want >= 2", len(tasks))
	}

	task := ag2.State().GetTask("db-migration")
	if task == nil {
		t.Fatal("restored agent missing db-migration task")
	}
	if task.Status != agent.TaskStatusCompleted {
		t.Errorf("db-migration status = %q, want completed", task.Status)
	}

	deps := ag2.State().AllDependencies()
	if len(deps) < 1 {
		t.Errorf("restored agent has %d dependencies, want >= 1", len(deps))
	}

	// Check that the dependency api-service → db-migration was resolved.
	foundResolved := false
	for _, d := range deps {
		if d.Blocked == "api-service" && d.BlockedBy == "db-migration" && d.Resolved {
			foundResolved = true
			break
		}
	}
	if !foundResolved {
		t.Errorf("dependency api-service→db-migration not found or not resolved in %+v", deps)
	}
}

// ---------- Phase 8: ACLs ----------

// TestE2E_ACL_InboundDeny verifies that inbound messages from a denied nick
// are dropped by the ACL engine and do not update the agent's state.
func TestE2E_ACL_InboundDeny(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	srv.Accounts["alice"] = "pass1"
	srv.Accounts["bob"] = "pass2"

	// Alice denies all messages from bob-* on #secure.
	alice := createTestAgent(t, srv, "alice-1", "alice", "pass1", []string{"#secure"},
		func(cfg *config.AppConfig) {
			cfg.ACLRules = []config.ACLRule{
				{
					Channel:     "#secure",
					NickPattern: "bob-*",
					Effect:      "deny",
				},
			}
		},
	)
	bob := createTestAgent(t, srv, "bob-2", "bob", "pass2", []string{"#secure"})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := alice.Start(ctx); err != nil {
		t.Fatalf("alice Start: %v", err)
	}
	defer alice.Shutdown()

	if err := bob.Start(ctx); err != nil {
		t.Fatalf("bob Start: %v", err)
	}
	defer bob.Shutdown()

	time.Sleep(300 * time.Millisecond) // channels joined

	// Track what alice receives via protocol handler.
	var aliceReceived []*protocol.Message
	var mu sync.Mutex
	alice.OnProtocolMessage(func(msg *protocol.Message) {
		mu.Lock()
		aliceReceived = append(aliceReceived, msg)
		mu.Unlock()
	})

	// Bob sends STARTED to #secure — should be blocked by alice's ACL.
	if err := bob.AnnounceStarted("#secure", "bob-task", "low"); err != nil {
		t.Fatalf("bob AnnounceStarted: %v", err)
	}
	time.Sleep(400 * time.Millisecond)

	// Alice should NOT have the task in her state.
	if task := alice.State().GetTask("bob-task"); task != nil {
		t.Errorf("alice state has bob-task (ACL should have blocked it): %+v", task)
	}

	// Alice's protocol handler should NOT have received bob's message.
	mu.Lock()
	for _, msg := range aliceReceived {
		if msg.Get("task") == "bob-task" {
			t.Errorf("alice protocol handler received bob-task (ACL should block inbound)")
		}
	}
	mu.Unlock()

	// Alice's own messages should still work.
	if err := alice.AnnounceStarted("#secure", "alice-task", "high"); err != nil {
		t.Fatalf("alice AnnounceStarted: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	if task := alice.State().GetTask("alice-task"); task == nil {
		t.Error("alice state missing alice-task (own messages should still work)")
	}
}

// TestE2E_ACL_OutboundDeny verifies that the ACL engine blocks outbound messages
// and returns an error to the caller.
func TestE2E_ACL_OutboundDeny(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	srv.Accounts["alice"] = "pass1"

	// Alice denies all outbound to #readonly.
	alice := createTestAgent(t, srv, "alice-1", "alice", "pass1", []string{"#readonly"},
		func(cfg *config.AppConfig) {
			cfg.ACLRules = []config.ACLRule{
				{
					Channel: "#readonly",
					Effect:  "deny",
				},
			}
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := alice.Start(ctx); err != nil {
		t.Fatalf("alice Start: %v", err)
	}
	defer alice.Shutdown()

	time.Sleep(300 * time.Millisecond)

	// Sending to #readonly should be denied.
	err = alice.AnnounceStarted("#readonly", "denied-task", "high")
	if err == nil {
		t.Fatal("expected ACL denied error, got nil")
	}
	if !strings.Contains(err.Error(), "ACL denied") {
		t.Errorf("expected error containing 'ACL denied', got: %v", err)
	}
}

// ---------- Phase 9: Discovery ----------

// TestE2E_Discovery_Roundtrip verifies the full DISCOVER → CAPABILITIES exchange
// between agents and validates the discovery store contents.
func TestE2E_Discovery_Roundtrip(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	srv.Accounts["alice"] = "pass1"
	srv.Accounts["bob"] = "pass2"
	srv.Accounts["carol"] = "pass3"

	// Use a short TTL so the heartbeat fires frequently (every ttl/2 = 2s).
	// The initial heartbeat fires before channels are joined; the ticker retry
	// ensures CAPABILITIES are sent once channels are ready.
	discoveryTTL := 4 * time.Second

	alice := createTestAgent(t, srv, "alice-1", "alice", "pass1", []string{"#project"},
		func(cfg *config.AppConfig) {
			cfg.Capabilities = []string{"database", "testing"}
			cfg.DiscoveryTTL = discoveryTTL
		},
	)
	bob := createTestAgent(t, srv, "bob-2", "bob", "pass2", []string{"#project"},
		func(cfg *config.AppConfig) {
			cfg.Capabilities = []string{"frontend"}
			cfg.DiscoveryTTL = discoveryTTL
		},
	)
	carol := createTestAgent(t, srv, "carol-3", "carol", "pass3", []string{"#project"},
		func(cfg *config.AppConfig) {
			cfg.Capabilities = []string{"database"}
			cfg.DiscoveryTTL = discoveryTTL
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := alice.Start(ctx); err != nil {
		t.Fatalf("alice Start: %v", err)
	}
	defer alice.Shutdown()

	if err := bob.Start(ctx); err != nil {
		t.Fatalf("bob Start: %v", err)
	}
	defer bob.Shutdown()

	if err := carol.Start(ctx); err != nil {
		t.Fatalf("carol Start: %v", err)
	}
	defer carol.Shutdown()

	// Wait for channels to be joined and at least one CAPABILITIES heartbeat cycle.
	// With TTL=4s, heartbeat interval=2s. Wait long enough for the retry.
	time.Sleep(3 * time.Second)

	// Each agent should have received the other agents' CAPABILITIES via heartbeat.
	// Alice should know about bob and carol (dispatcher skips self-echo, so not alice herself).
	known := alice.KnownAgents()
	knownNicks := make(map[string]bool)
	for _, a := range known {
		knownNicks[a.Nick] = true
	}
	if !knownNicks["bob-2"] {
		t.Errorf("alice does not know bob-2, known: %v", knownNicks)
	}
	if !knownNicks["carol-3"] {
		t.Errorf("alice does not know carol-3, known: %v", knownNicks)
	}

	// Alice sends DISCOVER expertise=database on #project.
	if err := alice.Discover("#project", "database"); err != nil {
		t.Fatalf("alice Discover: %v", err)
	}
	time.Sleep(600 * time.Millisecond)

	// After DISCOVER, carol (who has database) should have responded with CAPABILITIES.
	// Alice's discovery store should already have carol from heartbeat, but the DISCOVER
	// response refreshes it. Verify alice knows about agents with database expertise.
	known = alice.KnownAgents()
	var dbAgents []string
	for _, a := range known {
		for _, e := range a.Expertise {
			if strings.EqualFold(e, "database") {
				dbAgents = append(dbAgents, a.Nick)
				break
			}
		}
	}
	if len(dbAgents) == 0 {
		t.Error("alice found no agents with database expertise")
	}
	foundCarol := false
	for _, nick := range dbAgents {
		if nick == "carol-3" {
			foundCarol = true
		}
	}
	if !foundCarol {
		t.Errorf("carol-3 not found in database experts, found: %v", dbAgents)
	}
}

// TestE2E_Discovery_Dashboard verifies that the /discovery dashboard endpoint
// returns the agent's own capabilities and other known agents.
func TestE2E_Discovery_Dashboard(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	srv.Accounts["alice"] = "pass1"
	srv.Accounts["bob"] = "pass2"

	dashPort := getFreePort(t)
	dashAddr := fmt.Sprintf("127.0.0.1:%d", dashPort)

	alice := createTestAgent(t, srv, "alice-1", "alice", "pass1", []string{"#project"},
		func(cfg *config.AppConfig) {
			cfg.Capabilities = []string{"testing", "ci"}
			cfg.DiscoveryTTL = 4 * time.Second
			cfg.DashboardAddr = dashAddr
		},
	)
	bob := createTestAgent(t, srv, "bob-2", "bob", "pass2", []string{"#project"},
		func(cfg *config.AppConfig) {
			cfg.Capabilities = []string{"frontend"}
			cfg.DiscoveryTTL = 4 * time.Second
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := alice.Start(ctx); err != nil {
		t.Fatalf("alice Start: %v", err)
	}
	defer alice.Shutdown()

	if err := bob.Start(ctx); err != nil {
		t.Fatalf("bob Start: %v", err)
	}
	defer bob.Shutdown()

	// Wait for channels to join and heartbeat cycle (TTL=4s, interval=2s).
	time.Sleep(3 * time.Second)

	// GET /discovery
	resp, err := http.Get(fmt.Sprintf("http://%s/discovery", dashAddr))
	if err != nil {
		t.Fatalf("GET /discovery: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /discovery status %d, body: %s", resp.StatusCode, body)
	}

	var agents []*agent.AgentCapability
	if err := json.Unmarshal(body, &agents); err != nil {
		t.Fatalf("unmarshal /discovery: %v", err)
	}

	// Should contain bob's capabilities (alice's own heartbeat is skipped by dispatcher).
	foundBob := false
	for _, a := range agents {
		if a.Nick == "bob-2" {
			foundBob = true
			if len(a.Expertise) == 0 || a.Expertise[0] != "frontend" {
				t.Errorf("bob expertise = %v, want [frontend]", a.Expertise)
			}
		}
	}
	if !foundBob {
		t.Errorf("bob-2 not found in /discovery response: %s", body)
	}
}

// ---------- Phase 11: OTel ----------

// TestE2E_OTel_NoopMode verifies that the agent operates normally when
// OTel is not configured (no panics, no errors).
func TestE2E_OTel_NoopMode(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	srv.Accounts["alice"] = "pass1"
	srv.Accounts["bob"] = "pass2"

	alice := createTestAgent(t, srv, "alice-1", "alice", "pass1", []string{"#project"})
	bob := createTestAgent(t, srv, "bob-2", "bob", "pass2", []string{"#project"})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := alice.Start(ctx); err != nil {
		t.Fatalf("alice Start: %v", err)
	}
	defer alice.Shutdown()

	if err := bob.Start(ctx); err != nil {
		t.Fatalf("bob Start: %v", err)
	}
	defer bob.Shutdown()

	time.Sleep(300 * time.Millisecond)

	// All operations should succeed without panics.
	if err := alice.AnnounceStarted("#project", "noop-task", "low"); err != nil {
		t.Fatalf("AnnounceStarted: %v", err)
	}
	if err := alice.AnnounceCompleted("#project", "noop-task"); err != nil {
		t.Fatalf("AnnounceCompleted: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Verify metrics still work.
	snap := alice.Metrics().Snapshot()
	if snap.MessagesSent < 2 {
		t.Errorf("MessagesSent = %d, want >= 2", snap.MessagesSent)
	}

	// Bob should have received the messages.
	task := bob.State().GetTask("noop-task")
	if task == nil {
		t.Fatal("bob missing noop-task")
	}
	if task.Status != agent.TaskStatusCompleted {
		t.Errorf("noop-task status = %q, want completed", task.Status)
	}
}

// TestE2E_OTel_MetricsBridge verifies that OTel metric counters mirror
// the atomic counters when a MeterProvider is registered.
func TestE2E_OTel_MetricsBridge(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	srv.Accounts["alice"] = "pass1"
	srv.Accounts["bob"] = "pass2"

	alice := createTestAgent(t, srv, "alice-1", "alice", "pass1", []string{"#project"})
	bob := createTestAgent(t, srv, "bob-2", "bob", "pass2", []string{"#project"})

	// Register OTel metrics with a manual reader so we can collect them.
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer mp.Shutdown(context.Background())

	bob.Metrics().RegisterOTelMetrics(mp.Meter("test-agent"))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := alice.Start(ctx); err != nil {
		t.Fatalf("alice Start: %v", err)
	}
	defer alice.Shutdown()

	if err := bob.Start(ctx); err != nil {
		t.Fatalf("bob Start: %v", err)
	}
	defer bob.Shutdown()

	time.Sleep(300 * time.Millisecond)

	// Alice sends messages that bob will receive and count.
	if err := alice.AnnounceStarted("#project", "otel-task", "high"); err != nil {
		t.Fatalf("AnnounceStarted: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	if err := alice.AnnounceCompleted("#project", "otel-task"); err != nil {
		t.Fatalf("AnnounceCompleted: %v", err)
	}
	time.Sleep(400 * time.Millisecond)

	// Verify atomic counters on bob.
	snap := bob.Metrics().Snapshot()
	if snap.ProtocolMessagesIn < 2 {
		t.Errorf("bob ProtocolMessagesIn = %d, want >= 2", snap.ProtocolMessagesIn)
	}
	if snap.TasksStarted < 1 {
		t.Errorf("bob TasksStarted = %d, want >= 1", snap.TasksStarted)
	}
	if snap.TasksCompleted < 1 {
		t.Errorf("bob TasksCompleted = %d, want >= 1", snap.TasksCompleted)
	}

	// Verify OTel counters via manual reader.
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect OTel metrics: %v", err)
	}

	otelCounters := make(map[string]int64)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
				for _, dp := range sum.DataPoints {
					otelCounters[m.Name] += dp.Value
				}
			}
		}
	}

	if v := otelCounters["agent.messages.in"]; v < 2 {
		t.Errorf("OTel agent.messages.in = %d, want >= 2", v)
	}
	if v := otelCounters["agent.tasks.started"]; v < 1 {
		t.Errorf("OTel agent.tasks.started = %d, want >= 1", v)
	}
	if v := otelCounters["agent.tasks.completed"]; v < 1 {
		t.Errorf("OTel agent.tasks.completed = %d, want >= 1", v)
	}
}
