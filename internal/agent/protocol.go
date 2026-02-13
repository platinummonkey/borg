package agent

import (
	"log/slog"
	"sync"
	"time"

	"github.com/platinummonkey/agent-chat/pkg/ircclient"
	"github.com/platinummonkey/agent-chat/pkg/protocol"
)

// ProtocolHandler is a callback for incoming protocol messages.
type ProtocolHandler func(*protocol.Message)

// ProtocolDispatcher wires the protocol parser into the IRC client's OnMessage
// handler. It dispatches parsed protocol messages to registered handlers and
// updates the state and context stores.
type ProtocolDispatcher struct {
	client   ircclient.Client
	state    *StateStore
	context  *ContextStore
	selfNick func() string

	mu       sync.RWMutex
	handlers map[int]ProtocolHandler
	nextID   int
}

// NewProtocolDispatcher creates a dispatcher wired to the given stores and client.
func NewProtocolDispatcher(client ircclient.Client, state *StateStore, context *ContextStore) *ProtocolDispatcher {
	return &ProtocolDispatcher{
		client:   client,
		state:    state,
		context:  context,
		selfNick: client.Nick,
		handlers: make(map[int]ProtocolHandler),
	}
}

// Register hooks the dispatcher into the client's OnMessage handler.
// Returns the HandlerID for later removal.
func (pd *ProtocolDispatcher) Register() ircclient.HandlerID {
	return pd.client.OnMessage(func(ev ircclient.MessageEvent) {
		pd.handleMessage(ev)
	})
}

// OnProtocolMessage registers a handler for parsed protocol messages.
// Returns an ID for later removal.
func (pd *ProtocolDispatcher) OnProtocolMessage(handler ProtocolHandler) int {
	pd.mu.Lock()
	defer pd.mu.Unlock()
	id := pd.nextID
	pd.nextID++
	pd.handlers[id] = handler
	return id
}

// RemoveProtocolHandler removes a previously registered protocol handler.
func (pd *ProtocolDispatcher) RemoveProtocolHandler(id int) {
	pd.mu.Lock()
	defer pd.mu.Unlock()
	delete(pd.handlers, id)
}

func (pd *ProtocolDispatcher) handleMessage(ev ircclient.MessageEvent) {
	// Fast-path: skip non-protocol messages.
	if !protocol.IsProtocolMessage(ev.Message) {
		return
	}

	// Skip messages from self.
	if ev.Nick == pd.selfNick() {
		return
	}

	msg, err := protocol.Parse(ev.Message)
	if err != nil {
		slog.Debug("failed to parse protocol message", "error", err, "raw", ev.Message)
		return
	}

	// Enrich with dispatcher context.
	msg.Channel = ev.Channel
	msg.Nick = ev.Nick
	msg.Timestamp = ev.Timestamp
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	// Update state store.
	switch msg.Action {
	case protocol.ActionStarted, protocol.ActionCompleted, protocol.ActionBlocked, protocol.ActionAcknowledged:
		pd.state.UpdateTask(msg)
		if taskName := msg.Get("task"); taskName != "" {
			pd.state.UpdateAgentStatus(msg.Nick, msg.Channel, taskName)
		}
	}

	// Update context store.
	switch msg.Action {
	case protocol.ActionContext:
		pd.context.Store(msg)
	case protocol.ActionSharingContext:
		pd.context.StorePayload(msg)
	case protocol.ActionRequestContext:
		pd.context.HandleContextRequest(msg, pd.client)
	}

	// Dispatch to registered handlers.
	pd.mu.RLock()
	handlers := make([]ProtocolHandler, 0, len(pd.handlers))
	for _, h := range pd.handlers {
		handlers = append(handlers, h)
	}
	pd.mu.RUnlock()

	for _, h := range handlers {
		h(msg)
	}
}
