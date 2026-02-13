package agent

import (
	"sync"
	"testing"
	"time"

	"github.com/platinummonkey/agent-chat/pkg/protocol"
)

func TestContextStore_Store(t *testing.T) {
	cs := NewContextStore()

	msg := &protocol.Message{
		Action: protocol.ActionContext,
		Fields: map[string]string{
			"component": "auth",
			"project":   "webapp",
			"status":    "updated",
		},
		Nick:      "agent-1",
		Timestamp: time.Now(),
	}
	cs.Store(msg)

	entry := cs.Get("auth")
	if entry == nil {
		t.Fatal("Get(auth) returned nil")
	}
	if entry.Component != "auth" {
		t.Errorf("Component = %q, want %q", entry.Component, "auth")
	}
	if entry.Project != "webapp" {
		t.Errorf("Project = %q, want %q", entry.Project, "webapp")
	}
	if entry.Status != "updated" {
		t.Errorf("Status = %q, want %q", entry.Status, "updated")
	}
	if entry.SharedBy != "agent-1" {
		t.Errorf("SharedBy = %q, want %q", entry.SharedBy, "agent-1")
	}
}

func TestContextStore_Store_EmptyComponent(t *testing.T) {
	cs := NewContextStore()

	msg := &protocol.Message{
		Action: protocol.ActionContext,
		Fields: map[string]string{"project": "webapp"},
	}
	cs.Store(msg)

	// Should not store anything without a component.
	if entry := cs.Get(""); entry != nil {
		t.Error("Get('') should return nil for empty component")
	}
}

func TestContextStore_Get_Unknown(t *testing.T) {
	cs := NewContextStore()
	if entry := cs.Get("nonexistent"); entry != nil {
		t.Errorf("Get(nonexistent) = %v, want nil", entry)
	}
}

func TestContextStore_Get_ReturnsCopy(t *testing.T) {
	cs := NewContextStore()
	cs.Store(&protocol.Message{
		Action:    protocol.ActionContext,
		Fields:    map[string]string{"component": "auth", "status": "ok"},
		Nick:      "agent-1",
		Timestamp: time.Now(),
	})

	entry := cs.Get("auth")
	entry.Status = "modified"

	entry2 := cs.Get("auth")
	if entry2.Status != "ok" {
		t.Errorf("Get returned mutable reference: status = %q", entry2.Status)
	}
}

func TestContextStore_StorePayload(t *testing.T) {
	cs := NewContextStore()

	// First store a context announcement.
	cs.Store(&protocol.Message{
		Action:    protocol.ActionContext,
		Fields:    map[string]string{"component": "auth", "project": "webapp"},
		Nick:      "agent-1",
		Timestamp: time.Now(),
	})

	// Then store a payload from the same agent.
	cs.StorePayload(&protocol.Message{
		Action:    protocol.ActionSharingContext,
		Payload:   "aHR0cDovL2V4YW1wbGU=",
		Nick:      "agent-1",
		Timestamp: time.Now(),
	})

	entry := cs.Get("auth")
	if entry == nil {
		t.Fatal("Get(auth) returned nil after StorePayload")
	}
	if entry.Payload != "aHR0cDovL2V4YW1wbGU=" {
		t.Errorf("Payload = %q, want %q", entry.Payload, "aHR0cDovL2V4YW1wbGU=")
	}
}

func TestContextStore_StorePayload_NoExistingEntry(t *testing.T) {
	cs := NewContextStore()

	// Store payload without a prior context announcement.
	cs.StorePayload(&protocol.Message{
		Action:    protocol.ActionSharingContext,
		Payload:   "some-data",
		Nick:      "agent-2",
		Timestamp: time.Now(),
	})

	// Should be stored under a fallback key.
	entry := cs.Get("_payload_agent-2")
	if entry == nil {
		t.Fatal("Get(_payload_agent-2) returned nil")
	}
	if entry.Payload != "some-data" {
		t.Errorf("Payload = %q, want %q", entry.Payload, "some-data")
	}
}

func TestContextStore_Update(t *testing.T) {
	cs := NewContextStore()

	cs.Store(&protocol.Message{
		Action:    protocol.ActionContext,
		Fields:    map[string]string{"component": "auth", "status": "v1"},
		Nick:      "agent-1",
		Timestamp: time.Now(),
	})

	cs.Store(&protocol.Message{
		Action:    protocol.ActionContext,
		Fields:    map[string]string{"component": "auth", "status": "v2"},
		Nick:      "agent-1",
		Timestamp: time.Now(),
	})

	entry := cs.Get("auth")
	if entry.Status != "v2" {
		t.Errorf("Status = %q, want %q (should be updated)", entry.Status, "v2")
	}
}

func TestContextStore_TrackRequest(t *testing.T) {
	cs := NewContextStore()
	cs.TrackRequest(&protocol.Message{
		Action:    protocol.ActionRequestContext,
		Fields:    map[string]string{"component": "auth"},
		Nick:      "agent-1",
		Channel:   "#project",
		Timestamp: time.Now(),
	})

	pending := cs.PendingRequests()
	if len(pending) != 1 {
		t.Fatalf("PendingRequests = %d, want 1", len(pending))
	}
	if pending[0].Component != "auth" {
		t.Errorf("Component = %q, want %q", pending[0].Component, "auth")
	}
	if pending[0].RequestedBy != "agent-1" {
		t.Errorf("RequestedBy = %q, want %q", pending[0].RequestedBy, "agent-1")
	}
}

func TestContextStore_TrackRequest_EmptyComponent(t *testing.T) {
	cs := NewContextStore()
	cs.TrackRequest(&protocol.Message{
		Action:    protocol.ActionRequestContext,
		Fields:    map[string]string{},
		Nick:      "agent-1",
		Timestamp: time.Now(),
	})

	pending := cs.PendingRequests()
	if len(pending) != 0 {
		t.Errorf("PendingRequests = %d, want 0 for empty component", len(pending))
	}
}

func TestContextStore_FulfillRequest(t *testing.T) {
	cs := NewContextStore()
	cs.TrackRequest(&protocol.Message{
		Action:    protocol.ActionRequestContext,
		Fields:    map[string]string{"component": "auth"},
		Nick:      "agent-1",
		Timestamp: time.Now(),
	})

	cs.FulfillRequest("auth")

	pending := cs.PendingRequests()
	if len(pending) != 0 {
		t.Errorf("PendingRequests after fulfill = %d, want 0", len(pending))
	}
}

func TestContextStore_StoreFulfillsRequests(t *testing.T) {
	cs := NewContextStore()
	cs.TrackRequest(&protocol.Message{
		Action:    protocol.ActionRequestContext,
		Fields:    map[string]string{"component": "auth"},
		Nick:      "agent-1",
		Timestamp: time.Now(),
	})

	// Store a context for "auth" — should auto-fulfill the request.
	cs.Store(&protocol.Message{
		Action:    protocol.ActionContext,
		Fields:    map[string]string{"component": "auth", "status": "ready"},
		Nick:      "agent-2",
		Timestamp: time.Now(),
	})

	pending := cs.PendingRequests()
	if len(pending) != 0 {
		t.Errorf("PendingRequests after Store = %d, want 0", len(pending))
	}
}

func TestContextStore_TimedOutRequests(t *testing.T) {
	cs := NewContextStore()
	cs.TrackRequest(&protocol.Message{
		Action:    protocol.ActionRequestContext,
		Fields:    map[string]string{"component": "old-req"},
		Nick:      "agent-1",
		Timestamp: time.Now().Add(-10 * time.Second),
	})
	cs.TrackRequest(&protocol.Message{
		Action:    protocol.ActionRequestContext,
		Fields:    map[string]string{"component": "new-req"},
		Nick:      "agent-2",
		Timestamp: time.Now(),
	})

	timedOut := cs.TimedOutRequests(5 * time.Second)
	if len(timedOut) != 1 {
		t.Fatalf("TimedOutRequests = %d, want 1", len(timedOut))
	}
	if timedOut[0].Component != "old-req" {
		t.Errorf("Component = %q, want %q", timedOut[0].Component, "old-req")
	}
}

func TestContextStore_Subscribe(t *testing.T) {
	cs := NewContextStore()
	var received *ContextEntry
	var mu sync.Mutex

	cs.Subscribe("auth", func(entry *ContextEntry) {
		mu.Lock()
		received = entry
		mu.Unlock()
	})

	cs.Store(&protocol.Message{
		Action:    protocol.ActionContext,
		Fields:    map[string]string{"component": "auth", "status": "ready"},
		Nick:      "agent-1",
		Timestamp: time.Now(),
	})

	mu.Lock()
	defer mu.Unlock()
	if received == nil {
		t.Fatal("subscription handler not called")
	}
	if received.Component != "auth" {
		t.Errorf("received.Component = %q, want %q", received.Component, "auth")
	}
}

func TestContextStore_Subscribe_DifferentComponent(t *testing.T) {
	cs := NewContextStore()
	called := false
	cs.Subscribe("database", func(entry *ContextEntry) {
		called = true
	})

	// Store for a different component.
	cs.Store(&protocol.Message{
		Action:    protocol.ActionContext,
		Fields:    map[string]string{"component": "auth", "status": "ok"},
		Nick:      "agent-1",
		Timestamp: time.Now(),
	})

	if called {
		t.Error("subscription for 'database' should not fire for 'auth' store")
	}
}

func TestContextStore_Unsubscribe(t *testing.T) {
	cs := NewContextStore()
	callCount := 0
	id := cs.Subscribe("auth", func(entry *ContextEntry) {
		callCount++
	})

	cs.Store(&protocol.Message{
		Action:    protocol.ActionContext,
		Fields:    map[string]string{"component": "auth"},
		Nick:      "agent-1",
		Timestamp: time.Now(),
	})
	if callCount != 1 {
		t.Fatalf("callCount = %d, want 1", callCount)
	}

	cs.Unsubscribe(id)
	cs.Store(&protocol.Message{
		Action:    protocol.ActionContext,
		Fields:    map[string]string{"component": "auth"},
		Nick:      "agent-1",
		Timestamp: time.Now(),
	})
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1 (should not fire after unsubscribe)", callCount)
	}
}

func TestContextStore_PendingRequests_ReturnsCopy(t *testing.T) {
	cs := NewContextStore()
	cs.TrackRequest(&protocol.Message{
		Action:    protocol.ActionRequestContext,
		Fields:    map[string]string{"component": "auth"},
		Nick:      "agent-1",
		Timestamp: time.Now(),
	})

	pending := cs.PendingRequests()
	pending[0].Fulfilled = true

	// Original should be unchanged.
	pending2 := cs.PendingRequests()
	if len(pending2) != 1 {
		t.Errorf("PendingRequests after mutation = %d, want 1 (should return copies)", len(pending2))
	}
}
