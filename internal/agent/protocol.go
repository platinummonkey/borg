package agent

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/platinummonkey/borg/pkg/ircclient"
	"github.com/platinummonkey/borg/pkg/protocol"
)

// ProtocolHandler is a callback for incoming protocol messages.
type ProtocolHandler func(*protocol.Message)

// ProtocolDispatcher wires the protocol parser into the IRC client's OnMessage
// handler. It dispatches parsed protocol messages to registered handlers and
// updates the state and context stores.
type ProtocolDispatcher struct {
	client          ircclient.Client
	state           *StateStore
	context         *ContextStore
	selfNick        func() string
	unblockNotifier *UnblockNotifier
	acl             *ACLEngine
	discovery       *DiscoveryStore
	selfCaps        []string // this agent's expertise tags
	taskBoard       *TaskBoard
	handoff         *HandoffStore
	review          *ReviewStore
	consensus       *ConsensusStore

	mu       sync.RWMutex
	handlers map[int]ProtocolHandler
	nextID   int
}

// NewProtocolDispatcher creates a dispatcher wired to the given stores and client.
func NewProtocolDispatcher(client ircclient.Client, state *StateStore, context *ContextStore) *ProtocolDispatcher {
	pd := &ProtocolDispatcher{
		client:   client,
		state:    state,
		context:  context,
		selfNick: client.Nick,
		handlers: make(map[int]ProtocolHandler),
	}
	pd.unblockNotifier = NewUnblockNotifier(state, client, client.Nick)
	return pd
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

// updateLocalState updates the stores for a message the agent itself is sending.
// This ensures the agent's own state reflects its own actions, since the
// dispatcher skips self-echo from IRC.
func (pd *ProtocolDispatcher) updateLocalState(msg *protocol.Message) {
	switch msg.Action {
	case protocol.ActionStarted, protocol.ActionCompleted, protocol.ActionBlocked, protocol.ActionAcknowledged,
		protocol.ActionOffer, protocol.ActionClaim, protocol.ActionAssign,
		protocol.ActionAccept, protocol.ActionDecline, protocol.ActionYield,
		protocol.ActionCheckpoint, protocol.ActionHandoff:
		pd.state.UpdateTask(msg)
		if taskName := msg.Get("task"); taskName != "" {
			pd.state.UpdateAgentStatus(msg.Nick, msg.Channel, taskName)
		}
	}

	switch msg.Action {
	case protocol.ActionContext:
		pd.context.Store(msg)
	case protocol.ActionSharingContext:
		pd.context.StorePayload(msg)
	}

	pd.updateCoordinationStores(msg)
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

	// Tracing span for protocol dispatch.
	_, span := startSpan(context.Background(), "protocol.dispatch",
		protocolAttrs(string(msg.Action), msg.Channel, msg.Nick)...)
	defer span.End()

	// ACL check.
	if pd.acl != nil && !pd.acl.Check(msg.Nick, msg.Channel, msg.Action) {
		slog.Warn("ACL denied inbound", "nick", msg.Nick, "channel", msg.Channel, "action", msg.Action)
		return
	}

	// Update state store.
	switch msg.Action {
	case protocol.ActionStarted, protocol.ActionCompleted, protocol.ActionBlocked, protocol.ActionAcknowledged,
		protocol.ActionOffer, protocol.ActionClaim, protocol.ActionAssign,
		protocol.ActionAccept, protocol.ActionDecline, protocol.ActionYield,
		protocol.ActionCheckpoint, protocol.ActionHandoff:
		pd.state.UpdateTask(msg)
		if taskName := msg.Get("task"); taskName != "" {
			pd.state.UpdateAgentStatus(msg.Nick, msg.Channel, taskName)
		}
		if msg.Action == protocol.ActionCompleted {
			pd.unblockNotifier.OnTaskCompleted(msg)
		}
	}

	// Update coordination stores (taskboard, handoff, review, consensus).
	pd.updateCoordinationStores(msg)

	// Update context store.
	switch msg.Action {
	case protocol.ActionContext:
		pd.context.Store(msg)
	case protocol.ActionSharingContext:
		pd.context.StorePayload(msg)
	case protocol.ActionRequestContext:
		pd.context.TrackRequest(msg)
		pd.context.HandleContextRequest(msg, pd.client)
	}

	// Discovery protocol.
	if pd.discovery != nil {
		switch msg.Action {
		case protocol.ActionCapabilities:
			pd.handleCapabilities(msg)
		case protocol.ActionDiscover:
			pd.handleDiscover(msg)
		}
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

// handleCapabilities processes a CAPABILITIES message and updates the discovery store.
func (pd *ProtocolDispatcher) handleCapabilities(msg *protocol.Message) {
	expertise := msg.Get("expertise")
	channels := msg.Get("channels")
	currentTask := msg.Get("current-task")

	var expertiseTags []string
	if expertise != "" {
		for _, tag := range strings.Split(expertise, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				expertiseTags = append(expertiseTags, tag)
			}
		}
	}

	var channelList []string
	if channels != "" {
		for _, ch := range strings.Split(channels, ",") {
			ch = strings.TrimSpace(ch)
			if ch != "" {
				channelList = append(channelList, ch)
			}
		}
	}

	cap := &AgentCapability{
		Nick:        msg.Nick,
		Expertise:   expertiseTags,
		Channels:    channelList,
		CurrentTask: currentTask,
		UpdatedAt:   msg.Timestamp,
	}
	if role := msg.Get("role"); role != "" {
		cap.Role = role
	}
	if l := msg.Get("load"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			cap.Load = v
		}
	}
	if ml := msg.Get("max-load"); ml != "" {
		if v, err := strconv.Atoi(ml); err == nil {
			cap.MaxLoad = v
		}
	}
	if at := msg.Get("active-tasks"); at != "" {
		for _, t := range strings.Split(at, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				cap.ActiveTasks = append(cap.ActiveTasks, t)
			}
		}
	}
	pd.discovery.Update(cap)
}

// handleDiscover processes a DISCOVER message. If the requested expertise matches
// this agent's capabilities, it responds with a CAPABILITIES message.
func (pd *ProtocolDispatcher) handleDiscover(msg *protocol.Message) {
	requested := strings.ToLower(msg.Get("expertise"))
	if requested == "" || len(pd.selfCaps) == 0 {
		return
	}

	for _, cap := range pd.selfCaps {
		if strings.ToLower(cap) == requested {
			// Respond with our capabilities.
			reply := &protocol.Message{
				Action: protocol.ActionCapabilities,
				Fields: map[string]string{
					"expertise": strings.Join(pd.selfCaps, ","),
				},
			}
			target := msg.Channel
			if target == "" {
				target = msg.Nick
			}
			pd.client.SendMessage(target, reply.String())
			return
		}
	}
}

// updateCoordinationStores dispatches protocol messages to the coordination
// subsystem stores: TaskBoard, HandoffStore, ReviewStore, ConsensusStore.
func (pd *ProtocolDispatcher) updateCoordinationStores(msg *protocol.Message) {
	task := msg.Get("task")

	// TaskBoard actions.
	if pd.taskBoard != nil {
		switch msg.Action {
		case protocol.ActionOffer:
			pd.taskBoard.RecordOffer(task, msg.Channel, msg.Nick, msg.Get("priority"), msg.Get("scope"))
		case protocol.ActionClaim:
			load := 0
			if l := msg.Get("load"); l != "" {
				if v, err := strconv.Atoi(l); err == nil {
					load = v
				}
			}
			pd.taskBoard.RecordClaim(task, msg.Nick, load)
		case protocol.ActionAssign:
			pd.taskBoard.RecordAssign(task, msg.Get("to"), msg.Nick, msg.Channel)
		case protocol.ActionDecline:
			pd.taskBoard.RecordDecline(task)
		case protocol.ActionYield:
			pd.taskBoard.RecordYield(task)
		}
	}

	// HandoffStore actions.
	if pd.handoff != nil {
		switch msg.Action {
		case protocol.ActionCheckpoint:
			progress := 0
			if p := msg.Get("progress"); p != "" {
				if v, err := strconv.Atoi(p); err == nil {
					progress = v
				}
			}
			pd.handoff.RecordCheckpoint(task, msg.Nick, progress, msg.Get("summary"), msg.Channel)
		case protocol.ActionHandoff:
			pd.handoff.RecordHandoff(task, msg.Nick, msg.Get("to"), msg.Get("context-id"), msg.Channel)
		case protocol.ActionAccept:
			pd.handoff.AcceptHandoff(task, msg.Nick)
		}
	}

	// ReviewStore actions.
	if pd.review != nil {
		switch msg.Action {
		case protocol.ActionReviewRequest:
			pd.review.RecordRequest(task, msg.Get("pr"), msg.Get("review-type"), msg.Nick, msg.Channel)
		case protocol.ActionReviewComplete:
			pd.review.RecordComplete(task, msg.Get("pr"), msg.Nick, ReviewVerdict(msg.Get("verdict")), msg.Get("details"), msg.Channel)
		case protocol.ActionGateCheck:
			pd.review.RecordGate(task, msg.Get("gate"), GateStatus(msg.Get("status")), msg.Get("details"), msg.Nick, msg.Channel)
		}
	}

	// ConsensusStore actions.
	if pd.consensus != nil {
		switch msg.Action {
		case protocol.ActionVote:
			pd.consensus.RecordVote(msg.Get("topic"), msg.Nick, msg.Get("choice"), msg.Channel)
		case protocol.ActionEscalate:
			pd.consensus.RecordEscalation(task, msg.Get("to"), msg.Get("reason"), msg.Get("severity"), msg.Nick, msg.Channel)
		}
	}
}
