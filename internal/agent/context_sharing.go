package agent

import (
	"log/slog"
	"sync"
	"time"

	"github.com/platinummonkey/agent-chat/pkg/ircclient"
	"github.com/platinummonkey/agent-chat/pkg/protocol"
)

// ContextEntry holds shared context for a component.
type ContextEntry struct {
	Component string
	Project   string
	Status    string
	Payload   string
	SharedBy  string
	UpdatedAt time.Time
}

// ContextStore tracks context announcements and payloads.
// It is thread-safe and keyed by component name.
type ContextStore struct {
	mu      sync.RWMutex
	entries map[string]*ContextEntry
}

// NewContextStore creates an empty ContextStore.
func NewContextStore() *ContextStore {
	return &ContextStore{
		entries: make(map[string]*ContextEntry),
	}
}

// Store records a CONTEXT announcement message.
func (cs *ContextStore) Store(msg *protocol.Message) {
	component := msg.Get("component")
	if component == "" {
		return
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.entries[component] = &ContextEntry{
		Component: component,
		Project:   msg.Get("project"),
		Status:    msg.Get("status"),
		SharedBy:  msg.Nick,
		UpdatedAt: msg.Timestamp,
	}
}

// StorePayload records a SHARING-CONTEXT payload, associating it with the
// most recently requested component from the sender.
func (cs *ContextStore) StorePayload(msg *protocol.Message) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Try to find the component this payload is for by looking at existing
	// entries from this sender. If none found, store under a synthetic key.
	var target *ContextEntry
	for _, entry := range cs.entries {
		if entry.SharedBy == msg.Nick {
			target = entry
			break
		}
	}

	if target != nil {
		target.Payload = msg.Payload
		target.UpdatedAt = msg.Timestamp
	} else {
		// Store under sender nick as a fallback key.
		cs.entries["_payload_"+msg.Nick] = &ContextEntry{
			Component: "_payload_" + msg.Nick,
			Payload:   msg.Payload,
			SharedBy:  msg.Nick,
			UpdatedAt: msg.Timestamp,
		}
	}
}

// Get returns the context entry for a component, or nil if not found.
func (cs *ContextStore) Get(component string) *ContextEntry {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	entry := cs.entries[component]
	if entry == nil {
		return nil
	}
	cp := *entry
	return &cp
}

// HandleContextRequest responds to a REQUEST-CONTEXT message by sending
// the stored context for the requested component.
func (cs *ContextStore) HandleContextRequest(msg *protocol.Message, client ircclient.Client) {
	component := msg.Get("component")
	if component == "" {
		return
	}

	entry := cs.Get(component)
	if entry == nil {
		slog.Debug("context request for unknown component", "component", component, "from", msg.Nick)
		return
	}

	// Send the context announcement.
	reply := &protocol.Message{
		Action: protocol.ActionContext,
		Fields: map[string]string{
			"component": entry.Component,
		},
	}
	if entry.Project != "" {
		reply.Fields["project"] = entry.Project
	}
	if entry.Status != "" {
		reply.Fields["status"] = entry.Status
	}

	target := msg.Channel
	if target == "" {
		target = msg.Nick
	}
	client.SendMessage(target, reply.String())

	// If we have a payload, send it too.
	if entry.Payload != "" {
		payloadMsg := &protocol.Message{
			Action:  protocol.ActionSharingContext,
			Payload: entry.Payload,
		}
		client.SendMessage(target, payloadMsg.String())
	}
}
