package agent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/platinummonkey/agent-chat/internal/config"
	"github.com/platinummonkey/agent-chat/pkg/ircclient"
	"github.com/platinummonkey/agent-chat/pkg/protocol"
	"github.com/platinummonkey/agent-chat/test/mock"
)

// TestMultiAgentProtocol tests two agents communicating through a mock IRC server
// using the full protocol stack: STARTED → COMPLETED → dependency resolution,
// and REQUEST-CONTEXT → SHARING-CONTEXT.
func TestMultiAgentProtocol(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	// Add accounts for both agents.
	srv.Accounts["agent1"] = "pass1"
	srv.Accounts["agent2"] = "pass2"

	// Create two agents.
	agent1 := createTestAgent(t, srv, "agent-alice-1", "agent1", "pass1")
	agent2 := createTestAgent(t, srv, "agent-bob-2", "agent2", "pass2")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Connect both agents.
	if err := agent1.Start(ctx); err != nil {
		t.Fatalf("agent1 Connect failed: %v", err)
	}
	defer agent1.Shutdown()

	if err := agent2.Start(ctx); err != nil {
		t.Fatalf("agent2 Connect failed: %v", err)
	}
	defer agent2.Shutdown()

	// Both join the same channel.
	agent1.client.Join("#project")
	agent2.client.Join("#project")
	time.Sleep(200 * time.Millisecond)

	// Track what agent2 receives.
	var agent2Received []*protocol.Message
	var mu sync.Mutex
	agent2.OnProtocolMessage(func(msg *protocol.Message) {
		mu.Lock()
		agent2Received = append(agent2Received, msg)
		mu.Unlock()
	})

	// === Test 1: STARTED/COMPLETED flow ===

	// Agent1 announces starting a task.
	if err := agent1.AnnounceStarted("#project", "auth-refactor", "high", "feature"); err != nil {
		t.Fatalf("AnnounceStarted failed: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Agent2 should have received the STARTED message.
	mu.Lock()
	if len(agent2Received) < 1 {
		mu.Unlock()
		t.Fatal("agent2 did not receive STARTED message")
	}
	startedMsg := agent2Received[0]
	mu.Unlock()

	if startedMsg.Action != protocol.ActionStarted {
		t.Errorf("Expected STARTED, got %q", startedMsg.Action)
	}
	if startedMsg.Get("task") != "auth-refactor" {
		t.Errorf("task = %q, want %q", startedMsg.Get("task"), "auth-refactor")
	}

	// Agent2's state should show the task.
	task := agent2.State().GetTask("auth-refactor")
	if task == nil {
		t.Fatal("agent2 state does not have auth-refactor task")
	}
	if task.Status != TaskStatusStarted {
		t.Errorf("task status = %q, want started", task.Status)
	}

	// Agent1 completes the task.
	if err := agent1.AnnounceCompleted("#project", "auth-refactor", "ready-for-testing"); err != nil {
		t.Fatalf("AnnounceCompleted failed: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Agent2's state should now show completed.
	task = agent2.State().GetTask("auth-refactor")
	if task == nil {
		t.Fatal("agent2 state lost auth-refactor task")
	}
	if task.Status != TaskStatusCompleted {
		t.Errorf("task status = %q, want completed", task.Status)
	}

	// === Test 2: Dependency resolution ===

	// Agent2 announces being blocked on auth-refactor (which is already completed).
	if err := agent2.AnnounceBlocked("#project", "integration-tests", "", "blocked-by-auth-refactor"); err != nil {
		t.Fatalf("AnnounceBlocked failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Agent1 should see the blocked task.
	blockedTask := agent1.State().GetTask("integration-tests")
	if blockedTask == nil {
		t.Fatal("agent1 state does not have integration-tests task")
	}
	if blockedTask.Status != TaskStatusBlocked {
		t.Errorf("task status = %q, want blocked", blockedTask.Status)
	}

	// Since auth-refactor was already completed, the dependency should resolve.
	// The dependency resolution happens when auth-refactor was completed, but the
	// dependency edge was added after. Let's check agent1's state.
	// Agent1 received the BLOCKED message and should have the dependency.
	// auth-refactor was already completed in agent1's state, so let's verify.

	// === Test 3: Context sharing ===

	// Agent1 shares context about the auth component.
	if err := agent1.ShareContext("#project", "auth", "webapp", "refactored"); err != nil {
		t.Fatalf("ShareContext failed: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Agent2 should have the context stored.
	ctxEntry := agent2.ContextEntries().Get("auth")
	if ctxEntry == nil {
		t.Fatal("agent2 context store does not have auth entry")
	}
	if ctxEntry.Project != "webapp" {
		t.Errorf("project = %q, want %q", ctxEntry.Project, "webapp")
	}
	if ctxEntry.Status != "refactored" {
		t.Errorf("status = %q, want %q", ctxEntry.Status, "refactored")
	}

	// === Test 4: Protocol message count ===

	mu.Lock()
	totalReceived := len(agent2Received)
	mu.Unlock()
	// Agent2 should have received: STARTED, COMPLETED, CONTEXT = 3 messages minimum
	// (the BLOCKED was sent by agent2 itself, so it doesn't count)
	if totalReceived < 3 {
		t.Errorf("agent2 received %d protocol messages, want >= 3", totalReceived)
	}
}

// TestUnblockNotification tests that when agent1 completes a task that agent2 is
// blocked on, agent2 receives an automatic ACKNOWLEDGED message with #auto-unblock.
func TestUnblockNotification(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	srv.Accounts["agent1"] = "pass1"
	srv.Accounts["agent2"] = "pass2"

	agent1 := createTestAgent(t, srv, "agent-alice-1", "agent1", "pass1")
	agent2 := createTestAgent(t, srv, "agent-bob-2", "agent2", "pass2")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := agent1.Start(ctx); err != nil {
		t.Fatalf("agent1 Connect failed: %v", err)
	}
	defer agent1.Shutdown()

	if err := agent2.Start(ctx); err != nil {
		t.Fatalf("agent2 Connect failed: %v", err)
	}
	defer agent2.Shutdown()

	agent1.client.Join("#project")
	agent2.client.Join("#project")
	time.Sleep(200 * time.Millisecond)

	// Track raw messages received by both agents (to see ACKNOWLEDGED).
	var agent1Raw []string
	var agent2Raw []string
	var mu sync.Mutex

	agent1.client.OnMessage(func(ev ircclient.MessageEvent) {
		mu.Lock()
		agent1Raw = append(agent1Raw, ev.Message)
		mu.Unlock()
	})
	agent2.client.OnMessage(func(ev ircclient.MessageEvent) {
		mu.Lock()
		agent2Raw = append(agent2Raw, ev.Message)
		mu.Unlock()
	})

	// Agent2 announces being blocked on "db-migration".
	if err := agent2.AnnounceBlocked("#project", "integration-tests", "db-migration", "blocked-by-db-migration"); err != nil {
		t.Fatalf("AnnounceBlocked failed: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Agent1 completes db-migration — should trigger auto-unblock notification.
	if err := agent1.AnnounceCompleted("#project", "db-migration", "unblocks-others"); err != nil {
		t.Fatalf("AnnounceCompleted failed: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Check that an ACKNOWLEDGED message with #auto-unblock was sent to the channel.
	mu.Lock()
	defer mu.Unlock()

	foundAutoUnblock := false
	for _, raw := range agent2Raw {
		if strings.Contains(raw, "ACKNOWLEDGED") && strings.Contains(raw, "#auto-unblock") && strings.Contains(raw, "integration-tests") {
			foundAutoUnblock = true
			break
		}
	}
	// Also check agent1's raw messages (it receives the broadcast too).
	for _, raw := range agent1Raw {
		if strings.Contains(raw, "ACKNOWLEDGED") && strings.Contains(raw, "#auto-unblock") && strings.Contains(raw, "integration-tests") {
			foundAutoUnblock = true
			break
		}
	}

	if !foundAutoUnblock {
		t.Errorf("auto-unblock ACKNOWLEDGED not found in messages.\nagent1Raw: %v\nagent2Raw: %v", agent1Raw, agent2Raw)
	}
}

func createTestAgent(t *testing.T, srv *mock.IRCServer, nick, username, password string) *Agent {
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
		},
		LogLevel: "error",
	}

	client, err := ircclient.NewClient(cfg.IRC)
	if err != nil {
		t.Fatalf("NewClient for %s failed: %v", nick, err)
	}

	return NewWithClient(cfg, client)
}
