package agent

import (
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
