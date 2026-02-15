package manager

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/platinummonkey/borg/internal/agent"
	"github.com/platinummonkey/borg/pkg/ircclient"
	"github.com/platinummonkey/borg/pkg/protocol"
)

// mockClient implements ircclient.Client for testing.
type mockClient struct {
	nick     string
	messages []string
	handlers struct {
		message    []ircclient.MessageHandler
		join       []ircclient.JoinHandler
		part       []ircclient.PartHandler
		kick       []ircclient.KickHandler
		err        []ircclient.ErrorHandler
		connect    []ircclient.ConnectHandler
		disconnect []ircclient.DisconnectHandler
	}
	connected bool
	channels  []string
	nextID    int
}

func newMockClient(nick string) *mockClient {
	return &mockClient{nick: nick, connected: true}
}

func (m *mockClient) Connect(_ context.Context) error {
	m.connected = true
	return nil
}
func (m *mockClient) Disconnect()        { m.connected = false }
func (m *mockClient) Connected() bool     { return m.connected }
func (m *mockClient) Healthy() bool       { return m.connected }
func (m *mockClient) Join(ch string)      { m.channels = append(m.channels, ch) }
func (m *mockClient) Part(string)         {}
func (m *mockClient) JoinedChannels() []string { return m.channels }
func (m *mockClient) SendMessage(target, msg string) {
	m.messages = append(m.messages, target+":"+msg)
}
func (m *mockClient) SendNotice(_, _ string) {}
func (m *mockClient) SendRaw(_ string)       {}
func (m *mockClient) Nick() string           { return m.nick }
func (m *mockClient) SetNick(nick string)    { m.nick = nick }
func (m *mockClient) Wait() error            { return nil }
func (m *mockClient) RemoveHandler(_ ircclient.HandlerID) {}

func (m *mockClient) OnMessage(h ircclient.MessageHandler) ircclient.HandlerID {
	m.handlers.message = append(m.handlers.message, h)
	m.nextID++
	return ircclient.HandlerID(m.nextID)
}
func (m *mockClient) OnJoin(h ircclient.JoinHandler) ircclient.HandlerID {
	m.handlers.join = append(m.handlers.join, h)
	m.nextID++
	return ircclient.HandlerID(m.nextID)
}
func (m *mockClient) OnPart(h ircclient.PartHandler) ircclient.HandlerID {
	m.handlers.part = append(m.handlers.part, h)
	m.nextID++
	return ircclient.HandlerID(m.nextID)
}
func (m *mockClient) OnKick(h ircclient.KickHandler) ircclient.HandlerID {
	m.handlers.kick = append(m.handlers.kick, h)
	m.nextID++
	return ircclient.HandlerID(m.nextID)
}
func (m *mockClient) OnError(h ircclient.ErrorHandler) ircclient.HandlerID {
	m.handlers.err = append(m.handlers.err, h)
	m.nextID++
	return ircclient.HandlerID(m.nextID)
}
func (m *mockClient) OnConnect(h ircclient.ConnectHandler) ircclient.HandlerID {
	m.handlers.connect = append(m.handlers.connect, h)
	m.nextID++
	return ircclient.HandlerID(m.nextID)
}
func (m *mockClient) OnDisconnect(h ircclient.DisconnectHandler) ircclient.HandlerID {
	m.handlers.disconnect = append(m.handlers.disconnect, h)
	m.nextID++
	return ircclient.HandlerID(m.nextID)
}

func (m *mockClient) simulateMessage(channel, nick, message string) {
	ev := ircclient.MessageEvent{
		Channel:   channel,
		Nick:      nick,
		Message:   message,
		Timestamp: time.Now(),
	}
	for _, h := range m.handlers.message {
		h(ev)
	}
}

func TestManager_HandleCostReport(t *testing.T) {
	client := newMockClient("manager-bot")
	cfg := &ManagerConfig{
		ListenAddr:   ":0",
		PollInterval: time.Minute,
	}
	mgr := NewWithClient(cfg, client)

	// Register message handler.
	client.OnMessage(func(ev ircclient.MessageEvent) {
		mgr.handleIRCMessage(ev)
	})

	// Simulate a COST-REPORT.
	msg := &protocol.Message{
		Action: protocol.ActionCostReport,
		Fields: map[string]string{
			"task": "auth", "input-tokens": "1500", "output-tokens": "500",
			"total-tokens": "2000", "cost-usd": "0.0125",
			"model": "claude-sonnet-4-5-20250929", "provider": "anthropic",
		},
	}
	client.simulateMessage("#dev", "agent-1", msg.String())

	summary := mgr.CostStore().TotalSummary()
	if summary.EntryCount != 1 {
		t.Errorf("cost entries = %d, want 1", summary.EntryCount)
	}
	if summary.TotalCostUSD != 0.0125 {
		t.Errorf("total cost = %f, want 0.0125", summary.TotalCostUSD)
	}
}

func TestManager_HandleCapabilities(t *testing.T) {
	client := newMockClient("manager-bot")
	cfg := &ManagerConfig{
		ListenAddr:   ":0",
		PollInterval: time.Minute,
	}
	mgr := NewWithClient(cfg, client)

	client.OnMessage(func(ev ircclient.MessageEvent) {
		mgr.handleIRCMessage(ev)
	})

	msg := &protocol.Message{
		Action: protocol.ActionCapabilities,
		Fields: map[string]string{
			"expertise": "go,python",
		},
	}
	client.simulateMessage("#dev", "agent-1", msg.String())

	rec := mgr.Registry().Get("agent-1")
	if rec == nil {
		t.Fatal("expected agent record after CAPABILITIES")
	}
	if rec.Source != "discovered" {
		t.Errorf("source = %q, want %q", rec.Source, "discovered")
	}
	if len(rec.Capabilities) != 2 {
		t.Errorf("capabilities = %d, want 2", len(rec.Capabilities))
	}
}

func TestManager_HandleStartedCompleted(t *testing.T) {
	client := newMockClient("manager-bot")
	cfg := &ManagerConfig{
		ListenAddr:   ":0",
		PollInterval: time.Minute,
	}
	mgr := NewWithClient(cfg, client)

	client.OnMessage(func(ev ircclient.MessageEvent) {
		mgr.handleIRCMessage(ev)
	})

	// STARTED
	started := &protocol.Message{
		Action: protocol.ActionStarted,
		Fields: map[string]string{"task": "auth", "priority": "high"},
	}
	client.simulateMessage("#dev", "agent-1", started.String())

	task := mgr.State().GetTask("auth")
	if task == nil {
		t.Fatal("expected task after STARTED")
	}
	if task.Status != agent.TaskStatusStarted {
		t.Errorf("status = %q, want %q", task.Status, agent.TaskStatusStarted)
	}

	// COMPLETED
	completed := &protocol.Message{
		Action: protocol.ActionCompleted,
		Fields: map[string]string{"task": "auth"},
	}
	client.simulateMessage("#dev", "agent-1", completed.String())

	task = mgr.State().GetTask("auth")
	if task.Status != agent.TaskStatusCompleted {
		t.Errorf("status = %q, want %q", task.Status, agent.TaskStatusCompleted)
	}
}

func TestServer_APIAgents(t *testing.T) {
	client := newMockClient("manager-bot")
	cfg := &ManagerConfig{
		ListenAddr:   ":0",
		PollInterval: time.Minute,
	}
	mgr := NewWithClient(cfg, client)

	mgr.Registry().Register(&AgentRecord{Nick: "agent-1", Status: "online", Source: "spawned"})
	mgr.Registry().Register(&AgentRecord{Nick: "agent-2", Status: "online", Source: "discovered"})

	srv := NewServer(":0", mgr)
	req := httptest.NewRequest("GET", "/api/agents", nil)
	w := httptest.NewRecorder()
	srv.handleAPIAgents(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var agents []AgentRecord
	if err := json.NewDecoder(w.Body).Decode(&agents); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(agents) != 2 {
		t.Errorf("agents = %d, want 2", len(agents))
	}
}

func TestServer_APICosts(t *testing.T) {
	client := newMockClient("manager-bot")
	cfg := &ManagerConfig{
		ListenAddr:   ":0",
		PollInterval: time.Minute,
	}
	mgr := NewWithClient(cfg, client)

	srv := NewServer(":0", mgr)
	req := httptest.NewRequest("GET", "/api/costs", nil)
	w := httptest.NewRecorder()
	srv.handleAPICosts(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestServer_APISpawn_InvalidBody(t *testing.T) {
	client := newMockClient("manager-bot")
	cfg := &ManagerConfig{
		ListenAddr:   ":0",
		PollInterval: time.Minute,
	}
	mgr := NewWithClient(cfg, client)

	srv := NewServer(":0", mgr)
	req := httptest.NewRequest("POST", "/api/agents/spawn", strings.NewReader("invalid"))
	w := httptest.NewRecorder()
	srv.handleAPISpawn(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestServer_APIAgentDetail_NotFound(t *testing.T) {
	client := newMockClient("manager-bot")
	cfg := &ManagerConfig{
		ListenAddr:   ":0",
		PollInterval: time.Minute,
	}
	mgr := NewWithClient(cfg, client)

	srv := NewServer(":0", mgr)
	req := httptest.NewRequest("GET", "/api/agents/nonexistent", nil)
	req.SetPathValue("nick", "nonexistent")
	w := httptest.NewRecorder()
	srv.handleAPIAgentDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}
