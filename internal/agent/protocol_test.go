package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/platinummonkey/agent-chat/pkg/ircclient"
	"github.com/platinummonkey/agent-chat/pkg/protocol"
)

// mockIRCClient implements ircclient.Client for protocol dispatcher testing.
type mockIRCClient struct {
	nick     string
	messages []sentMessage

	mu       sync.Mutex
	handlers map[ircclient.HandlerID]ircclient.MessageHandler
	nextID   ircclient.HandlerID
}

type sentMessage struct {
	target  string
	message string
}

func newMockIRCClient(nick string) *mockIRCClient {
	return &mockIRCClient{
		nick:     nick,
		handlers: make(map[ircclient.HandlerID]ircclient.MessageHandler),
	}
}

func (m *mockIRCClient) Nick() string                    { return m.nick }
func (m *mockIRCClient) SetNick(nick string)             { m.nick = nick }
func (m *mockIRCClient) Connect(_ context.Context) error { return nil }
func (m *mockIRCClient) Disconnect()                     {}
func (m *mockIRCClient) Connected() bool                 { return true }
func (m *mockIRCClient) Healthy() bool                   { return true }
func (m *mockIRCClient) Join(channel string)             {}
func (m *mockIRCClient) Part(channel string)             {}
func (m *mockIRCClient) JoinedChannels() []string        { return nil }
func (m *mockIRCClient) SendRaw(message string)          {}
func (m *mockIRCClient) Wait() error                     { return nil }

func (m *mockIRCClient) SendMessage(target, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, sentMessage{target, message})
}

func (m *mockIRCClient) SendNotice(target, message string) {}

func (m *mockIRCClient) OnMessage(handler ircclient.MessageHandler) ircclient.HandlerID {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextID
	m.nextID++
	m.handlers[id] = handler
	return id
}

func (m *mockIRCClient) OnJoin(handler ircclient.JoinHandler) ircclient.HandlerID       { return 0 }
func (m *mockIRCClient) OnPart(handler ircclient.PartHandler) ircclient.HandlerID       { return 0 }
func (m *mockIRCClient) OnKick(handler ircclient.KickHandler) ircclient.HandlerID       { return 0 }
func (m *mockIRCClient) OnError(handler ircclient.ErrorHandler) ircclient.HandlerID     { return 0 }
func (m *mockIRCClient) OnConnect(handler ircclient.ConnectHandler) ircclient.HandlerID { return 0 }
func (m *mockIRCClient) OnDisconnect(handler ircclient.DisconnectHandler) ircclient.HandlerID {
	return 0
}
func (m *mockIRCClient) RemoveHandler(id ircclient.HandlerID) {}

// simulateMessage simulates a received IRC message, dispatching to all handlers.
func (m *mockIRCClient) simulateMessage(ev ircclient.MessageEvent) {
	m.mu.Lock()
	handlers := make([]ircclient.MessageHandler, 0, len(m.handlers))
	for _, h := range m.handlers {
		handlers = append(handlers, h)
	}
	m.mu.Unlock()

	for _, h := range handlers {
		h(ev)
	}
}

func (m *mockIRCClient) sentMessages() []sentMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]sentMessage, len(m.messages))
	copy(result, m.messages)
	return result
}

func TestProtocolDispatcher_ParsesAndDispatch(t *testing.T) {
	client := newMockIRCClient("self-agent")
	state := NewStateStore()
	ctx := NewContextStore()

	pd := NewProtocolDispatcher(client, state, ctx)
	pd.Register()

	var received []*protocol.Message
	var mu sync.Mutex
	pd.OnProtocolMessage(func(msg *protocol.Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	})

	// Simulate a STARTED message from another agent.
	client.simulateMessage(ircclient.MessageEvent{
		Channel:   "#project",
		Nick:      "agent-2",
		Message:   "STARTED task=implement-login priority=high #feature",
		Timestamp: time.Now(),
	})

	// Give handler a moment to process.
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("received %d messages, want 1", len(received))
	}

	msg := received[0]
	if msg.Action != protocol.ActionStarted {
		t.Errorf("Action = %q, want %q", msg.Action, protocol.ActionStarted)
	}
	if msg.Get("task") != "implement-login" {
		t.Errorf("task = %q, want %q", msg.Get("task"), "implement-login")
	}
	if msg.Channel != "#project" {
		t.Errorf("Channel = %q, want %q", msg.Channel, "#project")
	}
	if msg.Nick != "agent-2" {
		t.Errorf("Nick = %q, want %q", msg.Nick, "agent-2")
	}

	// State should be updated.
	info := state.GetTask("implement-login")
	if info == nil {
		t.Fatal("StateStore.GetTask returned nil")
	}
	if info.Status != TaskStatusStarted {
		t.Errorf("State status = %q, want %q", info.Status, TaskStatusStarted)
	}
}

func TestProtocolDispatcher_SkipsSelf(t *testing.T) {
	client := newMockIRCClient("self-agent")
	state := NewStateStore()
	ctx := NewContextStore()

	pd := NewProtocolDispatcher(client, state, ctx)
	pd.Register()

	var received []*protocol.Message
	pd.OnProtocolMessage(func(msg *protocol.Message) {
		received = append(received, msg)
	})

	// Message from self should be ignored.
	client.simulateMessage(ircclient.MessageEvent{
		Channel:   "#project",
		Nick:      "self-agent",
		Message:   "STARTED task=my-task",
		Timestamp: time.Now(),
	})

	time.Sleep(10 * time.Millisecond)

	if len(received) != 0 {
		t.Errorf("received %d messages from self, want 0", len(received))
	}
}

func TestProtocolDispatcher_SkipsNonProtocol(t *testing.T) {
	client := newMockIRCClient("self-agent")
	state := NewStateStore()
	ctx := NewContextStore()

	pd := NewProtocolDispatcher(client, state, ctx)
	pd.Register()

	var received []*protocol.Message
	pd.OnProtocolMessage(func(msg *protocol.Message) {
		received = append(received, msg)
	})

	// Regular chat message should be ignored.
	client.simulateMessage(ircclient.MessageEvent{
		Channel:   "#general",
		Nick:      "human-user",
		Message:   "Hello everyone!",
		Timestamp: time.Now(),
	})

	time.Sleep(10 * time.Millisecond)

	if len(received) != 0 {
		t.Errorf("received %d messages for non-protocol, want 0", len(received))
	}
}

func TestProtocolDispatcher_UpdatesState(t *testing.T) {
	client := newMockIRCClient("self-agent")
	state := NewStateStore()
	ctx := NewContextStore()

	pd := NewProtocolDispatcher(client, state, ctx)
	pd.Register()

	// STARTED
	client.simulateMessage(ircclient.MessageEvent{
		Channel: "#project", Nick: "agent-2",
		Message: "STARTED task=auth priority=high", Timestamp: time.Now(),
	})
	time.Sleep(10 * time.Millisecond)

	info := state.GetTask("auth")
	if info == nil || info.Status != TaskStatusStarted {
		t.Fatalf("Expected task auth started, got %v", info)
	}

	// COMPLETED
	client.simulateMessage(ircclient.MessageEvent{
		Channel: "#project", Nick: "agent-2",
		Message: "COMPLETED task=auth #ready", Timestamp: time.Now(),
	})
	time.Sleep(10 * time.Millisecond)

	info = state.GetTask("auth")
	if info.Status != TaskStatusCompleted {
		t.Errorf("Expected completed, got %q", info.Status)
	}

	// Agent status should be tracked.
	agentStatus := state.GetAgentStatus("agent-2")
	if agentStatus == nil {
		t.Fatal("Agent status not tracked")
	}
	if agentStatus.TaskName != "auth" {
		t.Errorf("Agent task = %q, want %q", agentStatus.TaskName, "auth")
	}
}

func TestProtocolDispatcher_ContextFlow(t *testing.T) {
	client := newMockIRCClient("self-agent")
	state := NewStateStore()
	ctxStore := NewContextStore()

	pd := NewProtocolDispatcher(client, state, ctxStore)
	pd.Register()

	// CONTEXT announcement.
	client.simulateMessage(ircclient.MessageEvent{
		Channel: "#project", Nick: "agent-2",
		Message: "CONTEXT component=auth project=webapp status=updated", Timestamp: time.Now(),
	})
	time.Sleep(10 * time.Millisecond)

	entry := ctxStore.Get("auth")
	if entry == nil {
		t.Fatal("Context not stored")
	}
	if entry.Project != "webapp" {
		t.Errorf("Project = %q, want %q", entry.Project, "webapp")
	}

	// REQUEST-CONTEXT should trigger a reply.
	client.simulateMessage(ircclient.MessageEvent{
		Channel: "#project", Nick: "agent-3",
		Message: "REQUEST-CONTEXT component=auth", Timestamp: time.Now(),
	})
	time.Sleep(10 * time.Millisecond)

	msgs := client.sentMessages()
	if len(msgs) == 0 {
		t.Fatal("No reply sent for REQUEST-CONTEXT")
	}

	// The reply should contain a CONTEXT message.
	found := false
	for _, m := range msgs {
		if m.target == "#project" && protocol.IsProtocolMessage(m.message) {
			parsed, err := protocol.Parse(m.message)
			if err == nil && parsed.Action == protocol.ActionContext && parsed.Get("component") == "auth" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("Expected CONTEXT reply for auth, got messages: %v", msgs)
	}
}

func TestProtocolDispatcher_RemoveHandler(t *testing.T) {
	client := newMockIRCClient("self-agent")
	state := NewStateStore()
	ctx := NewContextStore()

	pd := NewProtocolDispatcher(client, state, ctx)
	pd.Register()

	callCount := 0
	id := pd.OnProtocolMessage(func(msg *protocol.Message) {
		callCount++
	})

	client.simulateMessage(ircclient.MessageEvent{
		Channel: "#project", Nick: "agent-2",
		Message: "STARTED task=x", Timestamp: time.Now(),
	})
	time.Sleep(10 * time.Millisecond)

	if callCount != 1 {
		t.Fatalf("Expected 1 call, got %d", callCount)
	}

	pd.RemoveProtocolHandler(id)

	client.simulateMessage(ircclient.MessageEvent{
		Channel: "#project", Nick: "agent-2",
		Message: "COMPLETED task=x", Timestamp: time.Now(),
	})
	time.Sleep(10 * time.Millisecond)

	if callCount != 1 {
		t.Errorf("Expected 1 call after removal, got %d", callCount)
	}
}
