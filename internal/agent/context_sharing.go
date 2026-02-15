package agent

import (
	"log/slog"
	"sync"
	"time"

	"github.com/platinummonkey/borg/pkg/ircclient"
	"github.com/platinummonkey/borg/pkg/protocol"
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

// ContextRequest tracks an incoming REQUEST-CONTEXT message.
type ContextRequest struct {
	Component   string
	RequestedBy string
	Channel     string
	RequestedAt time.Time
	Fulfilled   bool
	FulfilledAt time.Time
}

// contextSubscription holds a subscription callback for context updates.
type contextSubscription struct {
	component string
	handler   func(*ContextEntry)
}

// ContextStore tracks context announcements and payloads.
// It is thread-safe and keyed by component name.
type ContextStore struct {
	mu      sync.RWMutex
	entries map[string]*ContextEntry

	reqMu    sync.Mutex
	requests []*ContextRequest

	subMu         sync.RWMutex
	subscriptions map[int]*contextSubscription
	nextSubID     int
}

// NewContextStore creates an empty ContextStore.
func NewContextStore() *ContextStore {
	return &ContextStore{
		entries:       make(map[string]*ContextEntry),
		subscriptions: make(map[int]*contextSubscription),
	}
}

// Store records a CONTEXT announcement message.
// It also fulfills pending requests for the component and fires subscription callbacks.
func (cs *ContextStore) Store(msg *protocol.Message) {
	component := msg.Get("component")
	if component == "" {
		return
	}

	entry := &ContextEntry{
		Component: component,
		Project:   msg.Get("project"),
		Status:    msg.Get("status"),
		SharedBy:  msg.Nick,
		UpdatedAt: msg.Timestamp,
	}

	cs.mu.Lock()
	cs.entries[component] = entry
	cs.mu.Unlock()

	cs.FulfillRequest(component)
	cs.fireSubscriptions(component, entry)
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

// ListEntries returns all stored context entries.
func (cs *ContextStore) ListEntries() []*ContextEntry {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	result := make([]*ContextEntry, 0, len(cs.entries))
	for _, entry := range cs.entries {
		cp := *entry
		result = append(result, &cp)
	}
	return result
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

// TrackRequest records an incoming REQUEST-CONTEXT message.
func (cs *ContextStore) TrackRequest(msg *protocol.Message) {
	component := msg.Get("component")
	if component == "" {
		return
	}
	cs.reqMu.Lock()
	defer cs.reqMu.Unlock()
	cs.requests = append(cs.requests, &ContextRequest{
		Component:   component,
		RequestedBy: msg.Nick,
		Channel:     msg.Channel,
		RequestedAt: msg.Timestamp,
	})
}

// FulfillRequest marks all pending requests for the given component as fulfilled.
func (cs *ContextStore) FulfillRequest(component string) {
	now := time.Now()
	cs.reqMu.Lock()
	defer cs.reqMu.Unlock()
	for _, r := range cs.requests {
		if r.Component == component && !r.Fulfilled {
			r.Fulfilled = true
			r.FulfilledAt = now
		}
	}
}

// PendingRequests returns all unfulfilled context requests.
func (cs *ContextStore) PendingRequests() []*ContextRequest {
	cs.reqMu.Lock()
	defer cs.reqMu.Unlock()
	var result []*ContextRequest
	for _, r := range cs.requests {
		if !r.Fulfilled {
			cp := *r
			result = append(result, &cp)
		}
	}
	return result
}

// TimedOutRequests returns unfulfilled requests older than the given timeout.
func (cs *ContextStore) TimedOutRequests(timeout time.Duration) []*ContextRequest {
	cutoff := time.Now().Add(-timeout)
	cs.reqMu.Lock()
	defer cs.reqMu.Unlock()
	var result []*ContextRequest
	for _, r := range cs.requests {
		if !r.Fulfilled && r.RequestedAt.Before(cutoff) {
			cp := *r
			result = append(result, &cp)
		}
	}
	return result
}

// Subscribe registers a callback that fires when context for the given component is stored.
// Returns a subscription ID for later removal.
func (cs *ContextStore) Subscribe(component string, handler func(*ContextEntry)) int {
	cs.subMu.Lock()
	defer cs.subMu.Unlock()
	id := cs.nextSubID
	cs.nextSubID++
	cs.subscriptions[id] = &contextSubscription{
		component: component,
		handler:   handler,
	}
	return id
}

// Unsubscribe removes a previously registered subscription.
func (cs *ContextStore) Unsubscribe(id int) {
	cs.subMu.Lock()
	defer cs.subMu.Unlock()
	delete(cs.subscriptions, id)
}

// fireSubscriptions calls all subscription handlers matching the component.
func (cs *ContextStore) fireSubscriptions(component string, entry *ContextEntry) {
	cs.subMu.RLock()
	var handlers []func(*ContextEntry)
	for _, sub := range cs.subscriptions {
		if sub.component == component {
			handlers = append(handlers, sub.handler)
		}
	}
	cs.subMu.RUnlock()

	cp := *entry
	for _, h := range handlers {
		h(&cp)
	}
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
