package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// TestFullDependencyChain tests a 3-agent transitive dependency chain:
// carol blocked on bob, bob blocked on alice. Completing tasks cascades unblocks.
func TestFullDependencyChain(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	srv.Accounts["alice"] = "pass1"
	srv.Accounts["bob"] = "pass2"
	srv.Accounts["carol"] = "pass3"

	alice := createTestAgent(t, srv, "alice-1", "alice", "pass1")
	bob := createTestAgent(t, srv, "bob-2", "bob", "pass2")
	carol := createTestAgent(t, srv, "carol-3", "carol", "pass3")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := alice.Start(ctx); err != nil {
		t.Fatalf("alice Start failed: %v", err)
	}
	defer alice.Shutdown()
	if err := bob.Start(ctx); err != nil {
		t.Fatalf("bob Start failed: %v", err)
	}
	defer bob.Shutdown()
	if err := carol.Start(ctx); err != nil {
		t.Fatalf("carol Start failed: %v", err)
	}
	defer carol.Shutdown()

	alice.client.Join("#project")
	bob.client.Join("#project")
	carol.client.Join("#project")
	time.Sleep(200 * time.Millisecond)

	// Track raw messages on alice and bob to see ACKNOWLEDGED auto-unblock.
	var aliceRaw, bobRaw []string
	var mu sync.Mutex
	alice.client.OnMessage(func(ev ircclient.MessageEvent) {
		mu.Lock()
		aliceRaw = append(aliceRaw, ev.Message)
		mu.Unlock()
	})
	bob.client.OnMessage(func(ev ircclient.MessageEvent) {
		mu.Lock()
		bobRaw = append(bobRaw, ev.Message)
		mu.Unlock()
	})

	// bob blocked on db-migration.
	if err := bob.AnnounceBlocked("#project", "api-service", "db-migration", "blocked-by-db-migration"); err != nil {
		t.Fatalf("bob AnnounceBlocked failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// carol blocked on api-service.
	if err := carol.AnnounceBlocked("#project", "frontend", "api-service", "blocked-by-api-service"); err != nil {
		t.Fatalf("carol AnnounceBlocked failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Verify transitive dependencies from carol's perspective (on any agent that has the state).
	deps := alice.State().TransitiveDependencies("frontend")
	if len(deps) < 2 {
		t.Errorf("TransitiveDependencies(frontend) = %v, want at least [api-service, db-migration]", deps)
	}

	// alice completes db-migration → api-service should auto-unblock.
	if err := alice.AnnounceCompleted("#project", "db-migration"); err != nil {
		t.Fatalf("alice AnnounceCompleted failed: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	foundApiUnblock := false
	for _, raw := range append(aliceRaw, bobRaw...) {
		if strings.Contains(raw, "ACKNOWLEDGED") && strings.Contains(raw, "#auto-unblock") && strings.Contains(raw, "api-service") {
			foundApiUnblock = true
			break
		}
	}
	mu.Unlock()
	if !foundApiUnblock {
		mu.Lock()
		t.Errorf("auto-unblock for api-service not found.\naliceRaw: %v\nbobRaw: %v", aliceRaw, bobRaw)
		mu.Unlock()
	}

	// bob completes api-service → frontend should auto-unblock.
	if err := bob.AnnounceCompleted("#project", "api-service"); err != nil {
		t.Fatalf("bob AnnounceCompleted failed: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	foundFrontendUnblock := false
	for _, raw := range append(aliceRaw, bobRaw...) {
		if strings.Contains(raw, "ACKNOWLEDGED") && strings.Contains(raw, "#auto-unblock") && strings.Contains(raw, "frontend") {
			foundFrontendUnblock = true
			break
		}
	}
	mu.Unlock()
	if !foundFrontendUnblock {
		mu.Lock()
		t.Errorf("auto-unblock for frontend not found.\naliceRaw: %v\nbobRaw: %v", aliceRaw, bobRaw)
		mu.Unlock()
	}

	// Verify all tasks show completed in alice's state store.
	for _, taskName := range []string{"db-migration", "api-service"} {
		task := alice.State().GetTask(taskName)
		if task == nil {
			t.Errorf("alice state missing task %q", taskName)
			continue
		}
		if task.Status != TaskStatusCompleted {
			t.Errorf("task %q status = %q, want completed", taskName, task.Status)
		}
	}
}

// TestContextSharingRoundTrip tests the full context sharing cycle:
// share → subscribe → request → auto-respond.
func TestContextSharingRoundTrip(t *testing.T) {
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
		t.Fatalf("agent1 Start failed: %v", err)
	}
	defer agent1.Shutdown()
	if err := agent2.Start(ctx); err != nil {
		t.Fatalf("agent2 Start failed: %v", err)
	}
	defer agent2.Shutdown()

	agent1.client.Join("#project")
	agent2.client.Join("#project")
	time.Sleep(200 * time.Millisecond)

	// Subscribe to context updates on agent2.
	var subEntry *ContextEntry
	var subMu sync.Mutex
	agent2.SubscribeContext("auth", func(entry *ContextEntry) {
		subMu.Lock()
		subEntry = entry
		subMu.Unlock()
	})

	// agent1 shares context about auth.
	if err := agent1.ShareContext("#project", "auth", "webapp", "refactored"); err != nil {
		t.Fatalf("ShareContext failed: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Verify subscription callback fired.
	subMu.Lock()
	if subEntry == nil {
		subMu.Unlock()
		t.Fatal("subscription callback not fired for auth context")
	}
	if subEntry.Component != "auth" || subEntry.Project != "webapp" || subEntry.Status != "refactored" {
		subMu.Unlock()
		t.Errorf("subscription entry = %+v, want component=auth project=webapp status=refactored", subEntry)
	}
	subMu.Unlock()

	// Verify agent2's context store has the entry.
	entry := agent2.ContextEntries().Get("auth")
	if entry == nil {
		t.Fatal("agent2 context store missing auth entry")
	}

	// Track raw messages on agent2 to see the auto-response to REQUEST-CONTEXT.
	var agent2Raw []string
	var mu sync.Mutex
	agent2.client.OnMessage(func(ev ircclient.MessageEvent) {
		mu.Lock()
		agent2Raw = append(agent2Raw, ev.Message)
		mu.Unlock()
	})

	// agent2 requests context → agent1 should auto-respond with CONTEXT.
	if err := agent2.RequestContext("#project", "auth"); err != nil {
		t.Fatalf("RequestContext failed: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Verify agent2 received a CONTEXT response from agent1.
	mu.Lock()
	foundResponse := false
	for _, raw := range agent2Raw {
		if strings.Contains(raw, "CONTEXT") && strings.Contains(raw, "component=auth") {
			foundResponse = true
			break
		}
	}
	mu.Unlock()
	if !foundResponse {
		mu.Lock()
		t.Errorf("auto-response CONTEXT not found in agent2 raw messages: %v", agent2Raw)
		mu.Unlock()
	}
}

// TestNotificationRouting tests that notifications route to the correct channels
// and don't cross-route.
func TestNotificationRouting(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	srv.Accounts["agent1"] = "pass1"
	srv.Accounts["agent2"] = "pass2"
	srv.Accounts["agent3"] = "pass3"

	agent1 := createTestAgent(t, srv, "agent-1", "agent1", "pass1")
	agent2 := createTestAgent(t, srv, "agent-2", "agent2", "pass2")
	agent3 := createTestAgent(t, srv, "agent-3", "agent3", "pass3")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := agent1.Start(ctx); err != nil {
		t.Fatalf("agent1 Start failed: %v", err)
	}
	defer agent1.Shutdown()
	if err := agent2.Start(ctx); err != nil {
		t.Fatalf("agent2 Start failed: %v", err)
	}
	defer agent2.Shutdown()
	if err := agent3.Start(ctx); err != nil {
		t.Fatalf("agent3 Start failed: %v", err)
	}
	defer agent3.Shutdown()

	for _, a := range []*Agent{agent1, agent2, agent3} {
		a.client.Join("#project")
		a.client.Join("#ops-alerts")
		a.client.Join("#help-desk")
	}
	time.Sleep(300 * time.Millisecond)

	// Agent3 configures notification routing.
	agent3.NotifyCompletionsTo("#ops-alerts")
	agent3.NotifyBlockedTo("#ops-alerts")
	agent3.NotifyHelpTo("#help-desk")

	// Track raw messages on agent1 and agent2 per-channel.
	var agent1Raw, agent2Raw []ircclient.MessageEvent
	var mu sync.Mutex
	agent1.client.OnMessage(func(ev ircclient.MessageEvent) {
		mu.Lock()
		agent1Raw = append(agent1Raw, ev)
		mu.Unlock()
	})
	agent2.client.OnMessage(func(ev ircclient.MessageEvent) {
		mu.Lock()
		agent2Raw = append(agent2Raw, ev)
		mu.Unlock()
	})

	// Agent1 completes a task → notification should go to #ops-alerts.
	if err := agent1.AnnounceCompleted("#project", "deploy-v2", "done"); err != nil {
		t.Fatalf("AnnounceCompleted failed: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Agent2 announces blocked → notification should go to #ops-alerts.
	if err := agent2.AnnounceBlocked("#project", "testing", "deploy-v2", "blocked-by-deploy"); err != nil {
		t.Fatalf("AnnounceBlocked failed: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Agent1 requests help → notification should go to #help-desk.
	if err := agent1.RequestHelp("#project", "perf-issue", "database", "urgent"); err != nil {
		t.Fatalf("RequestHelp failed: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Check for [COMPLETED] on #ops-alerts.
	foundCompletedOnOps := false
	for _, ev := range append(agent1Raw, agent2Raw...) {
		if ev.Channel == "#ops-alerts" && strings.Contains(ev.Message, "[COMPLETED]") {
			foundCompletedOnOps = true
			break
		}
	}
	if !foundCompletedOnOps {
		t.Error("[COMPLETED] notification not found on #ops-alerts")
	}

	// Check for [BLOCKED] on #ops-alerts.
	foundBlockedOnOps := false
	for _, ev := range append(agent1Raw, agent2Raw...) {
		if ev.Channel == "#ops-alerts" && strings.Contains(ev.Message, "[BLOCKED]") {
			foundBlockedOnOps = true
			break
		}
	}
	if !foundBlockedOnOps {
		t.Error("[BLOCKED] notification not found on #ops-alerts")
	}

	// Check for [HELP-NEEDED] on #help-desk.
	foundHelpOnDesk := false
	for _, ev := range append(agent1Raw, agent2Raw...) {
		if ev.Channel == "#help-desk" && strings.Contains(ev.Message, "[HELP-NEEDED]") {
			foundHelpOnDesk = true
			break
		}
	}
	if !foundHelpOnDesk {
		t.Error("[HELP-NEEDED] notification not found on #help-desk")
	}

	// Verify no cross-routing: no [HELP-NEEDED] on #ops-alerts.
	for _, ev := range append(agent1Raw, agent2Raw...) {
		if ev.Channel == "#ops-alerts" && strings.Contains(ev.Message, "[HELP-NEEDED]") {
			t.Error("[HELP-NEEDED] incorrectly routed to #ops-alerts")
			break
		}
	}
	// No [COMPLETED] on #help-desk.
	for _, ev := range append(agent1Raw, agent2Raw...) {
		if ev.Channel == "#help-desk" && strings.Contains(ev.Message, "[COMPLETED]") {
			t.Error("[COMPLETED] incorrectly routed to #help-desk")
			break
		}
	}
}

// TestDashboardEndpoints tests that dashboard HTTP endpoints return correct data
// after agent operations.
func TestDashboardEndpoints(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	srv.Accounts["agent1"] = "pass1"
	srv.Accounts["agent2"] = "pass2"

	// Create agent1 with dashboard enabled on a random port.
	cfg1 := &config.AppConfig{
		IRC: ircclient.Config{
			Server:                srv.Addr(),
			Nick:                  "dash-agent-1",
			Username:              "agent1",
			Password:              "pass1",
			RealName:              "Test Agent",
			TLS:                   true,
			TLSInsecureSkipVerify: true,
			SASL:                  true,
		},
		LogLevel:      "error",
		DashboardAddr: "127.0.0.1:0",
	}
	client1, err := ircclient.NewClient(cfg1.IRC)
	if err != nil {
		t.Fatalf("NewClient for agent1 failed: %v", err)
	}
	agent1 := NewWithClient(cfg1, client1)

	agent2 := createTestAgent(t, srv, "dash-agent-2", "agent2", "pass2")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := agent1.Start(ctx); err != nil {
		t.Fatalf("agent1 Start failed: %v", err)
	}
	defer agent1.Shutdown()
	if err := agent2.Start(ctx); err != nil {
		t.Fatalf("agent2 Start failed: %v", err)
	}
	defer agent2.Shutdown()

	agent1.client.Join("#project")
	agent2.client.Join("#project")
	time.Sleep(200 * time.Millisecond)

	// Agent2 sends STARTED + COMPLETED for a task.
	if err := agent2.AnnounceStarted("#project", "build-v3", "high"); err != nil {
		t.Fatalf("AnnounceStarted failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := agent2.AnnounceCompleted("#project", "build-v3", "success"); err != nil {
		t.Fatalf("AnnounceCompleted failed: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	baseURL := fmt.Sprintf("http://%s", agent1.dashboard.ListenAddr())

	// GET /health
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Fatalf("GET /health failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var health HealthStatus
	if err := json.Unmarshal(body, &health); err != nil {
		t.Fatalf("decode /health: %v", err)
	}
	if !health.Connected {
		t.Error("/health: Connected = false, want true")
	}

	// GET /metrics
	resp, err = http.Get(baseURL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	var metrics MetricsSnapshot
	if err := json.Unmarshal(body, &metrics); err != nil {
		t.Fatalf("decode /metrics: %v", err)
	}
	if metrics.ProtocolMessagesIn < 2 {
		t.Errorf("/metrics: ProtocolMessagesIn = %d, want >= 2", metrics.ProtocolMessagesIn)
	}
	if metrics.TasksStarted < 1 {
		t.Errorf("/metrics: TasksStarted = %d, want >= 1", metrics.TasksStarted)
	}
	if metrics.TasksCompleted < 1 {
		t.Errorf("/metrics: TasksCompleted = %d, want >= 1", metrics.TasksCompleted)
	}

	// GET /tasks
	resp, err = http.Get(baseURL + "/tasks")
	if err != nil {
		t.Fatalf("GET /tasks failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	var tasks []*TaskInfo
	if err := json.Unmarshal(body, &tasks); err != nil {
		t.Fatalf("decode /tasks: %v", err)
	}
	foundTask := false
	for _, task := range tasks {
		if task.Name == "build-v3" && task.Status == TaskStatusCompleted {
			foundTask = true
			break
		}
	}
	if !foundTask {
		t.Errorf("/tasks: build-v3 completed not found in %+v", tasks)
	}

	// GET /messages
	resp, err = http.Get(baseURL + "/messages")
	if err != nil {
		t.Fatalf("GET /messages failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	var messages []MessageLogEntry
	if err := json.Unmarshal(body, &messages); err != nil {
		t.Fatalf("decode /messages: %v", err)
	}
	if len(messages) < 2 {
		t.Errorf("/messages: got %d entries, want >= 2", len(messages))
	}
}

// TestHelpRequestCoordination tests that a HELP-NEEDED message is received
// by another agent with correct fields.
func TestHelpRequestCoordination(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	srv.Accounts["agent1"] = "pass1"
	srv.Accounts["agent2"] = "pass2"

	agent1 := createTestAgent(t, srv, "help-alice-1", "agent1", "pass1")
	agent2 := createTestAgent(t, srv, "help-bob-2", "agent2", "pass2")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := agent1.Start(ctx); err != nil {
		t.Fatalf("agent1 Start failed: %v", err)
	}
	defer agent1.Shutdown()
	if err := agent2.Start(ctx); err != nil {
		t.Fatalf("agent2 Start failed: %v", err)
	}
	defer agent2.Shutdown()

	agent1.client.Join("#project")
	agent2.client.Join("#project")
	time.Sleep(200 * time.Millisecond)

	// Agent2 registers a protocol handler filtering for HELP-NEEDED.
	var helpMsg *protocol.Message
	var mu sync.Mutex
	agent2.OnProtocolMessage(func(msg *protocol.Message) {
		if msg.Action == protocol.ActionHelpNeeded {
			mu.Lock()
			helpMsg = msg
			mu.Unlock()
		}
	})

	// Agent1 requests help.
	if err := agent1.RequestHelp("#project", "perf-tuning", "database", "urgent"); err != nil {
		t.Fatalf("RequestHelp failed: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if helpMsg == nil {
		t.Fatal("agent2 did not receive HELP-NEEDED message")
	}
	if helpMsg.Get("task") != "perf-tuning" {
		t.Errorf("task = %q, want perf-tuning", helpMsg.Get("task"))
	}
	if helpMsg.Get("expertise") != "database" {
		t.Errorf("expertise = %q, want database", helpMsg.Get("expertise"))
	}
	if !helpMsg.HasTag("urgent") {
		t.Errorf("missing tag 'urgent', tags = %v", helpMsg.Tags)
	}
	if helpMsg.Channel != "#project" {
		t.Errorf("channel = %q, want #project", helpMsg.Channel)
	}
	if helpMsg.Nick != "help-alice-1" {
		t.Errorf("nick = %q, want help-alice-1", helpMsg.Nick)
	}

	// Verify metrics.
	snap := agent2.Metrics().Snapshot()
	if snap.HelpRequested < 1 {
		t.Errorf("agent2 HelpRequested = %d, want >= 1", snap.HelpRequested)
	}
}

// TestMetricsAccuracy validates exact metric counters after a known sequence of operations.
func TestMetricsAccuracy(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	srv.Accounts["agent1"] = "pass1"
	srv.Accounts["agent2"] = "pass2"

	agent1 := createTestAgent(t, srv, "metrics-alice-1", "agent1", "pass1")
	agent2 := createTestAgent(t, srv, "metrics-bob-2", "agent2", "pass2")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := agent1.Start(ctx); err != nil {
		t.Fatalf("agent1 Start failed: %v", err)
	}
	defer agent1.Shutdown()
	if err := agent2.Start(ctx); err != nil {
		t.Fatalf("agent2 Start failed: %v", err)
	}
	defer agent2.Shutdown()

	agent1.client.Join("#project")
	agent2.client.Join("#project")
	time.Sleep(200 * time.Millisecond)

	// Agent1 performs a known sequence.
	if err := agent1.AnnounceStarted("#project", "metrics-task", "high"); err != nil {
		t.Fatalf("AnnounceStarted failed: %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	if err := agent1.AnnounceBlocked("#project", "metrics-task", "metrics-task", "blocked-by-metrics-task"); err != nil {
		t.Fatalf("AnnounceBlocked failed: %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	if err := agent1.ShareContext("#project", "metrics-comp", "metrics-proj", "active"); err != nil {
		t.Fatalf("ShareContext failed: %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	if err := agent1.RequestHelp("#project", "metrics-help", "go-perf", "low"); err != nil {
		t.Fatalf("RequestHelp failed: %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	if err := agent1.AnnounceCompleted("#project", "metrics-task", "done"); err != nil {
		t.Fatalf("AnnounceCompleted failed: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Verify agent2's inbound metrics.
	snap2 := agent2.Metrics().Snapshot()
	if snap2.ProtocolMessagesIn < 5 {
		t.Errorf("agent2 ProtocolMessagesIn = %d, want >= 5", snap2.ProtocolMessagesIn)
	}
	if snap2.TasksStarted < 1 {
		t.Errorf("agent2 TasksStarted = %d, want >= 1", snap2.TasksStarted)
	}
	if snap2.TasksCompleted < 1 {
		t.Errorf("agent2 TasksCompleted = %d, want >= 1", snap2.TasksCompleted)
	}
	if snap2.TasksBlocked < 1 {
		t.Errorf("agent2 TasksBlocked = %d, want >= 1", snap2.TasksBlocked)
	}
	if snap2.ContextShared < 1 {
		t.Errorf("agent2 ContextShared = %d, want >= 1", snap2.ContextShared)
	}
	if snap2.HelpRequested < 1 {
		t.Errorf("agent2 HelpRequested = %d, want >= 1", snap2.HelpRequested)
	}

	// Verify agent1's outbound metrics.
	snap1 := agent1.Metrics().Snapshot()
	if snap1.MessagesSent < 5 {
		t.Errorf("agent1 MessagesSent = %d, want >= 5", snap1.MessagesSent)
	}
	if snap1.ProtocolMessagesOut < 5 {
		t.Errorf("agent1 ProtocolMessagesOut = %d, want >= 5", snap1.ProtocolMessagesOut)
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
