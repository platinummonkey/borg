package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/platinummonkey/agent-chat/internal/config"
	"github.com/platinummonkey/agent-chat/internal/logging"
	agentOtel "github.com/platinummonkey/agent-chat/internal/otel"
	"github.com/platinummonkey/agent-chat/pkg/ircclient"
	"github.com/platinummonkey/agent-chat/pkg/protocol"
)

// Agent orchestrates the IRC client, event handlers, and lifecycle management.
type Agent struct {
	cfg       *config.AppConfig
	client    ircclient.Client
	lc        *Lifecycle
	state     *StateStore
	context   *ContextStore
	protocol  *ProtocolDispatcher
	notifier  *Notifier
	health    *HealthMonitor
	metrics   *MetricsCollector
	inspector *DebugInspector
	dashboard *Dashboard
	persist      *StatePersistence
	acl          *ACLEngine
	discovery    *DiscoveryStore
	capabilities []string
	discoverStop chan struct{}
	federation    *FederationManager
	otelProvider *agentOtel.Provider

	// Coordination subsystems (Phases 12–16).
	taskBoard      *TaskBoard
	handoff        *HandoffStore
	review         *ReviewStore
	consensus      *ConsensusStore
	roleEngine     *RoleEngine
	workflowEngine *WorkflowEngine
}

// New creates an Agent from an AppConfig.
func New(cfg *config.AppConfig) (*Agent, error) {
	logging.Setup(cfg.LogLevel, cfg.LogFmt, nil)

	client, err := ircclient.NewClient(cfg.IRC)
	if err != nil {
		return nil, fmt.Errorf("create IRC client: %w", err)
	}

	a := &Agent{
		cfg:    cfg,
		client: client,
	}

	// OTel setup (no-op if endpoint is empty).
	if cfg.OTel.Endpoint != "" {
		p, err := agentOtel.Setup(context.Background(), cfg.OTel)
		if err != nil {
			return nil, fmt.Errorf("setup otel: %w", err)
		}
		a.otelProvider = p
	}

	a.initProtocol()
	a.registerHandlers()
	a.lc = NewLifecycle(a)

	return a, nil
}

// NewWithClient creates an Agent using a pre-built Client (useful for testing).
func NewWithClient(cfg *config.AppConfig, client ircclient.Client) *Agent {
	a := &Agent{
		cfg:    cfg,
		client: client,
	}
	a.initProtocol()
	a.registerHandlers()
	a.lc = NewLifecycle(a)
	return a
}

// initProtocol sets up the state store, context store, protocol dispatcher,
// notifier, health monitor, metrics collector, and debug inspector.
func (a *Agent) initProtocol() {
	a.state = NewStateStore()

	// Persistence: load saved state before wiring dispatchers.
	if a.cfg.StateFile != "" {
		a.persist = NewStatePersistence(a.cfg.StateFile, a.state, 5*time.Second)
		if err := a.persist.Load(); err != nil {
			slog.Error("failed to load persisted state", "error", err)
		}
	}

	a.context = NewContextStore()
	a.protocol = NewProtocolDispatcher(a.client, a.state, a.context)

	// ACL engine.
	if len(a.cfg.ACLRules) > 0 {
		a.acl = NewACLEngine(a.cfg.ACLRules)
		a.protocol.acl = a.acl
	}

	// Discovery store.
	a.discovery = NewDiscoveryStore(a.cfg.DiscoveryTTL)
	a.protocol.discovery = a.discovery
	a.capabilities = a.cfg.Capabilities
	a.protocol.selfCaps = a.capabilities

	a.protocol.Register()
	a.notifier = NewNotifier(a.client)
	a.protocol.OnProtocolMessage(a.notifier.HandleMessage)

	a.health = NewHealthMonitor(a.client, a.state)
	a.metrics = NewMetricsCollector()
	if a.otelProvider != nil && a.otelProvider.MeterProvider() != nil {
		a.metrics.RegisterOTelMetrics(a.otelProvider.MeterProvider().Meter("agent-chat"))
	}
	a.inspector = NewDebugInspector(a.state, a.context, 1000)

	// Register metrics and inspector as protocol handlers.
	a.protocol.OnProtocolMessage(a.metrics.HandleProtocolMessage)
	a.protocol.OnProtocolMessage(func(msg *protocol.Message) {
		a.inspector.RecordMessage(MessageLogEntry{
			Timestamp: msg.Timestamp,
			Direction: "in",
			Channel:   msg.Channel,
			Nick:      msg.Nick,
			Action:    string(msg.Action),
			Raw:       msg.String(),
		})
	})

	// Mark state dirty on state-changing protocol messages.
	if a.persist != nil {
		a.protocol.OnProtocolMessage(func(msg *protocol.Message) {
			switch msg.Action {
			case protocol.ActionStarted, protocol.ActionCompleted,
				protocol.ActionBlocked, protocol.ActionAcknowledged,
				protocol.ActionOffer, protocol.ActionClaim, protocol.ActionAssign,
				protocol.ActionAccept, protocol.ActionDecline, protocol.ActionYield,
				protocol.ActionCheckpoint, protocol.ActionHandoff:
				a.persist.MarkDirty()
			}
		})
	}

	// Coordination subsystems (Phases 12–16).
	a.taskBoard = NewTaskBoard(a.state, a.cfg.ClaimJitter)
	a.protocol.taskBoard = a.taskBoard

	a.handoff = NewHandoffStore()
	a.protocol.handoff = a.handoff

	a.review = NewReviewStore(a.cfg.MaxReviewIterations)
	a.protocol.review = a.review

	a.consensus = NewConsensusStore()
	a.protocol.consensus = a.consensus

	// Role engine (Phase 16).
	if len(a.cfg.Roles) > 0 {
		a.roleEngine = NewRoleEngine(a.cfg.Roles)
		for _, role := range a.cfg.Roles {
			if behavior := BuiltinBehavior(role); behavior != nil {
				a.roleEngine.RegisterBehavior(behavior)
			}
		}
		a.capabilities = append(a.capabilities, a.roleEngine.ExpertiseTags()...)
		a.protocol.selfCaps = a.capabilities
		a.protocol.OnProtocolMessage(func(msg *protocol.Message) {
			for _, resp := range a.roleEngine.HandleMessage(msg) {
				_ = a.SendProtocolMessage(msg.Channel, resp)
			}
		})
	}

	// Workflow engine (Phase 16).
	a.workflowEngine = NewWorkflowEngine()
}

// Start connects the agent to the IRC server and optionally starts the dashboard.
func (a *Agent) Start(ctx context.Context) error {
	if a.cfg.DashboardAddr != "" {
		a.dashboard = NewDashboard(a.cfg.DashboardAddr, a.health, a.metrics, a.inspector, a.state, a.context, a.discovery, a.taskBoard, a.handoff, a.review, a.consensus)
		if err := a.dashboard.Start(); err != nil {
			return fmt.Errorf("start dashboard: %w", err)
		}
	}
	if err := a.client.Connect(ctx); err != nil {
		return err
	}

	// Start federation if configured.
	if len(a.cfg.FederationServers) > 0 {
		if err := a.startFederation(ctx); err != nil {
			return fmt.Errorf("start federation: %w", err)
		}
	}

	// Start discovery heartbeat if capabilities are configured.
	if len(a.capabilities) > 0 {
		a.startDiscoveryHeartbeat()
	}
	return nil
}

// Run blocks until the agent is shut down (via signal or context cancellation).
func (a *Agent) Run(ctx context.Context) error {
	if err := a.Start(ctx); err != nil {
		return err
	}
	return a.lc.Wait(ctx)
}

// startFederation creates and starts the federation manager from config.
func (a *Agent) startFederation(ctx context.Context) error {
	a.federation = NewFederationManager(a.client, a.client.Nick)

	for _, srv := range a.cfg.FederationServers {
		client, err := ircclient.NewClient(srv.IRC)
		if err != nil {
			return fmt.Errorf("create federation client %q: %w", srv.Name, err)
		}
		a.federation.AddLink(srv.Name, client, srv.IRC.Channels)
	}

	for _, m := range a.cfg.FederationMappings {
		a.federation.AddMapping(m)
	}

	return a.federation.Start(ctx)
}

// Shutdown performs a graceful shutdown: stops dashboard, parts channels, disconnects.
func (a *Agent) Shutdown() {
	slog.Info("agent shutting down")
	if a.discoverStop != nil {
		close(a.discoverStop)
	}
	if a.federation != nil {
		a.federation.Shutdown()
	}
	if a.persist != nil {
		a.persist.Close()
	}
	if a.dashboard != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.dashboard.Shutdown(ctx); err != nil {
			slog.Error("dashboard shutdown error", "error", err)
		}
	}
	if a.otelProvider != nil {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.otelProvider.Shutdown(shutCtx); err != nil {
			slog.Error("otel shutdown error", "error", err)
		}
	}
	for _, ch := range a.client.JoinedChannels() {
		a.client.Part(ch)
	}
	a.client.Disconnect()
}

// startDiscoveryHeartbeat sends periodic CAPABILITIES messages and prunes expired entries.
func (a *Agent) startDiscoveryHeartbeat() {
	ttl := a.cfg.DiscoveryTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	interval := ttl / 2
	if interval < time.Second {
		interval = time.Second
	}

	a.discoverStop = make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Send initial announcement.
		a.announceCapabilities()

		for {
			select {
			case <-ticker.C:
				a.announceCapabilities()
				a.discovery.Prune()
			case <-a.discoverStop:
				return
			}
		}
	}()
}

func (a *Agent) announceCapabilities() {
	if len(a.capabilities) == 0 {
		return
	}
	msg := &protocol.Message{
		Action: protocol.ActionCapabilities,
		Fields: map[string]string{
			"expertise": strings.Join(a.capabilities, ","),
		},
	}
	for _, ch := range a.client.JoinedChannels() {
		if err := a.SendProtocolMessage(ch, msg); err != nil {
			slog.Debug("failed to announce capabilities", "channel", ch, "error", err)
		}
	}
}

// Discover sends a DISCOVER request to a channel for agents with specific expertise.
func (a *Agent) Discover(channel, expertise string) error {
	msg := &protocol.Message{
		Action: protocol.ActionDiscover,
		Fields: map[string]string{"expertise": expertise},
	}
	return a.SendProtocolMessage(channel, msg)
}

// AnnounceCapabilities sends this agent's CAPABILITIES to a specific channel.
func (a *Agent) AnnounceCapabilities(channel string) error {
	if len(a.capabilities) == 0 {
		return nil
	}
	msg := &protocol.Message{
		Action: protocol.ActionCapabilities,
		Fields: map[string]string{
			"expertise": strings.Join(a.capabilities, ","),
		},
	}
	return a.SendProtocolMessage(channel, msg)
}

// KnownAgents returns all non-expired agent capabilities from the discovery store.
func (a *Agent) KnownAgents() []*AgentCapability {
	return a.discovery.ListActive()
}

// State returns the agent's task state store for read access.
func (a *Agent) State() *StateStore {
	return a.state
}

// ContextEntries returns the agent's context store for read access.
func (a *Agent) ContextEntries() *ContextStore {
	return a.context
}

// OnProtocolMessage registers a handler for incoming protocol messages.
func (a *Agent) OnProtocolMessage(handler ProtocolHandler) int {
	return a.protocol.OnProtocolMessage(handler)
}

// SendProtocolMessage sends a protocol message to a target channel or user.
// The message is sanitized before sending. It also updates the local state
// store so the agent tracks its own actions (the dispatcher skips self-echo).
func (a *Agent) SendProtocolMessage(target string, msg *protocol.Message) error {
	_, span := startSpan(context.Background(), "agent.send",
		protocolAttrs(string(msg.Action), target, a.client.Nick())...)
	defer span.End()

	if err := protocol.Sanitize(msg); err != nil {
		return err
	}
	if a.acl != nil && !a.acl.Check(a.client.Nick(), target, msg.Action) {
		return fmt.Errorf("ACL denied: %s on %s", msg.Action, target)
	}
	// Update local state before sending — the dispatcher will skip the echo.
	localMsg := *msg
	localMsg.Channel = target
	localMsg.Nick = a.client.Nick()
	if localMsg.Timestamp.IsZero() {
		localMsg.Timestamp = time.Now()
	}
	a.protocol.updateLocalState(&localMsg)

	wireMsg := msg.String()
	a.client.SendMessage(target, wireMsg)
	a.metrics.RecordMessageSent()
	if a.persist != nil {
		switch msg.Action {
		case protocol.ActionStarted, protocol.ActionCompleted,
			protocol.ActionBlocked, protocol.ActionAcknowledged:
			a.persist.MarkDirty()
		}
	}
	a.inspector.RecordMessage(MessageLogEntry{
		Timestamp: localMsg.Timestamp,
		Direction: "out",
		Channel:   target,
		Nick:      localMsg.Nick,
		Action:    string(msg.Action),
		Raw:       wireMsg,
	})
	return nil
}

// AnnounceStarted sends a STARTED message for a task.
func (a *Agent) AnnounceStarted(channel, task, priority string, tags ...string) error {
	msg := &protocol.Message{
		Action: protocol.ActionStarted,
		Fields: map[string]string{"task": task},
		Tags:   tags,
	}
	if priority != "" {
		msg.Fields["priority"] = priority
	}
	return a.SendProtocolMessage(channel, msg)
}

// AnnounceCompleted sends a COMPLETED message for a task.
func (a *Agent) AnnounceCompleted(channel, task string, tags ...string) error {
	msg := &protocol.Message{
		Action: protocol.ActionCompleted,
		Fields: map[string]string{"task": task},
		Tags:   tags,
	}
	return a.SendProtocolMessage(channel, msg)
}

// AnnounceBlocked sends a BLOCKED message for a task.
func (a *Agent) AnnounceBlocked(channel, task, waitingFor string, tags ...string) error {
	msg := &protocol.Message{
		Action: protocol.ActionBlocked,
		Fields: map[string]string{"task": task},
		Tags:   tags,
	}
	if waitingFor != "" {
		msg.Fields["waiting-for"] = waitingFor
	}
	return a.SendProtocolMessage(channel, msg)
}

// RequestHelp sends a HELP-NEEDED message for a task.
func (a *Agent) RequestHelp(channel, task, expertise string, tags ...string) error {
	msg := &protocol.Message{
		Action: protocol.ActionHelpNeeded,
		Fields: map[string]string{"task": task},
		Tags:   tags,
	}
	if expertise != "" {
		msg.Fields["expertise"] = expertise
	}
	return a.SendProtocolMessage(channel, msg)
}

// ShareContext sends a CONTEXT announcement message.
func (a *Agent) ShareContext(channel, component, project, status string) error {
	msg := &protocol.Message{
		Action: protocol.ActionContext,
		Fields: map[string]string{"component": component},
	}
	if project != "" {
		msg.Fields["project"] = project
	}
	if status != "" {
		msg.Fields["status"] = status
	}
	return a.SendProtocolMessage(channel, msg)
}

// RequestContext sends a REQUEST-CONTEXT message for a component.
func (a *Agent) RequestContext(channel, component string) error {
	msg := &protocol.Message{
		Action: protocol.ActionRequestContext,
		Fields: map[string]string{"component": component},
	}
	return a.SendProtocolMessage(channel, msg)
}

// SubscribeContext registers a callback for context updates on a component.
func (a *Agent) SubscribeContext(component string, handler func(*ContextEntry)) int {
	return a.context.Subscribe(component, handler)
}

// UnsubscribeContext removes a context subscription.
func (a *Agent) UnsubscribeContext(id int) {
	a.context.Unsubscribe(id)
}

// PendingContextRequests returns all unfulfilled context requests.
func (a *Agent) PendingContextRequests() []*ContextRequest {
	return a.context.PendingRequests()
}

// AddNotificationRule adds a rule for routing protocol events to notification channels.
func (a *Agent) AddNotificationRule(rule NotificationRule) {
	a.notifier.AddRule(rule)
}

// NotifyCompletionsTo adds a rule to send task completion notifications to a channel.
func (a *Agent) NotifyCompletionsTo(channel string) {
	a.notifier.AddRule(NotificationRule{Event: NotifyTaskCompleted, Channel: channel})
}

// NotifyBlockedTo adds a rule to send task blocked notifications to a channel.
func (a *Agent) NotifyBlockedTo(channel string) {
	a.notifier.AddRule(NotificationRule{Event: NotifyTaskBlocked, Channel: channel})
}

// NotifyHelpTo adds a rule to send help-needed notifications to a channel.
func (a *Agent) NotifyHelpTo(channel string) {
	a.notifier.AddRule(NotificationRule{Event: NotifyHelpNeeded, Channel: channel})
}

// AgentNotifier returns the agent's notifier for direct configuration.
func (a *Agent) AgentNotifier() *Notifier {
	return a.notifier
}

// Health returns the agent's health monitor.
func (a *Agent) Health() *HealthMonitor {
	return a.health
}

// Metrics returns the agent's metrics collector.
func (a *Agent) Metrics() *MetricsCollector {
	return a.metrics
}

// Inspector returns the agent's debug inspector.
func (a *Agent) Inspector() *DebugInspector {
	return a.inspector
}

// TaskBoard returns the agent's task board store.
func (a *Agent) TaskBoard() *TaskBoard {
	return a.taskBoard
}

// HandoffStore returns the agent's handoff store.
func (a *Agent) HandoffStore() *HandoffStore {
	return a.handoff
}

// ReviewStore returns the agent's review store.
func (a *Agent) ReviewStore() *ReviewStore {
	return a.review
}

// ConsensusStore returns the agent's consensus store.
func (a *Agent) ConsensusStore() *ConsensusStore {
	return a.consensus
}

// WorkflowEngine returns the agent's workflow engine.
func (a *Agent) WorkflowEngine() *WorkflowEngine {
	return a.workflowEngine
}

// OfferTask sends an OFFER message for a task.
func (a *Agent) OfferTask(channel, task, priority, scope string, tags ...string) error {
	msg := &protocol.Message{
		Action: protocol.ActionOffer,
		Fields: map[string]string{"task": task},
		Tags:   tags,
	}
	if priority != "" {
		msg.Fields["priority"] = priority
	}
	if scope != "" {
		msg.Fields["scope"] = scope
	}
	return a.SendProtocolMessage(channel, msg)
}

// ClaimTask sends a CLAIM message for a task.
func (a *Agent) ClaimTask(channel, task string, load int) error {
	msg := &protocol.Message{
		Action: protocol.ActionClaim,
		Fields: map[string]string{
			"task": task,
			"load": fmt.Sprintf("%d", load),
		},
	}
	return a.SendProtocolMessage(channel, msg)
}

// AssignTask sends an ASSIGN message delegating a task to another agent.
func (a *Agent) AssignTask(channel, task, to string) error {
	msg := &protocol.Message{
		Action: protocol.ActionAssign,
		Fields: map[string]string{"task": task, "to": to},
	}
	return a.SendProtocolMessage(channel, msg)
}

// AcceptTask sends an ACCEPT message for an assigned task.
func (a *Agent) AcceptTask(channel, task string) error {
	msg := &protocol.Message{
		Action: protocol.ActionAccept,
		Fields: map[string]string{"task": task},
	}
	return a.SendProtocolMessage(channel, msg)
}

// DeclineTask sends a DECLINE message for an assigned task.
func (a *Agent) DeclineTask(channel, task, reason string) error {
	msg := &protocol.Message{
		Action: protocol.ActionDecline,
		Fields: map[string]string{"task": task},
	}
	if reason != "" {
		msg.Fields["reason"] = reason
	}
	return a.SendProtocolMessage(channel, msg)
}

// YieldTask sends a YIELD message returning a task to the pool.
func (a *Agent) YieldTask(channel, task, reason string) error {
	msg := &protocol.Message{
		Action: protocol.ActionYield,
		Fields: map[string]string{"task": task},
	}
	if reason != "" {
		msg.Fields["reason"] = reason
	}
	return a.SendProtocolMessage(channel, msg)
}

// Checkpoint sends a CHECKPOINT message reporting progress on a task.
func (a *Agent) Checkpoint(channel, task string, progress int, summary string) error {
	msg := &protocol.Message{
		Action: protocol.ActionCheckpoint,
		Fields: map[string]string{
			"task":     task,
			"progress": fmt.Sprintf("%d", progress),
		},
	}
	if summary != "" {
		msg.Fields["summary"] = summary
	}
	return a.SendProtocolMessage(channel, msg)
}

// Handoff sends a HANDOFF message transferring a task to another agent.
func (a *Agent) Handoff(channel, task, to, contextID string) error {
	msg := &protocol.Message{
		Action: protocol.ActionHandoff,
		Fields: map[string]string{"task": task, "to": to},
	}
	if contextID != "" {
		msg.Fields["context-id"] = contextID
	}
	return a.SendProtocolMessage(channel, msg)
}

// RequestReview sends a REVIEW-REQUEST message for a task.
func (a *Agent) RequestReview(channel, task, pr, reviewType string) error {
	msg := &protocol.Message{
		Action: protocol.ActionReviewRequest,
		Fields: map[string]string{"task": task},
	}
	if pr != "" {
		msg.Fields["pr"] = pr
	}
	if reviewType != "" {
		msg.Fields["review-type"] = reviewType
	}
	return a.SendProtocolMessage(channel, msg)
}

// CompleteReview sends a REVIEW-COMPLETE message with a verdict.
func (a *Agent) CompleteReview(channel, task, pr string, verdict ReviewVerdict, details string) error {
	msg := &protocol.Message{
		Action: protocol.ActionReviewComplete,
		Fields: map[string]string{
			"task":    task,
			"verdict": string(verdict),
		},
	}
	if pr != "" {
		msg.Fields["pr"] = pr
	}
	if details != "" {
		msg.Fields["details"] = details
	}
	return a.SendProtocolMessage(channel, msg)
}

// GateCheck sends a GATE-CHECK message reporting a gate status.
func (a *Agent) GateCheck(channel, task, gate string, status GateStatus, details string) error {
	msg := &protocol.Message{
		Action: protocol.ActionGateCheck,
		Fields: map[string]string{
			"task":   task,
			"gate":   gate,
			"status": string(status),
		},
	}
	if details != "" {
		msg.Fields["details"] = details
	}
	return a.SendProtocolMessage(channel, msg)
}

// Vote sends a VOTE message on a topic.
func (a *Agent) Vote(channel, topic, choice string) error {
	msg := &protocol.Message{
		Action: protocol.ActionVote,
		Fields: map[string]string{"topic": topic, "choice": choice},
	}
	return a.SendProtocolMessage(channel, msg)
}

// Escalate sends an ESCALATE message to a human operator.
func (a *Agent) Escalate(channel, task, to, reason, severity string) error {
	msg := &protocol.Message{
		Action: protocol.ActionEscalate,
		Fields: map[string]string{"task": task, "to": to, "reason": reason, "severity": severity},
	}
	return a.SendProtocolMessage(channel, msg)
}

// registerHandlers sets up default event handlers for logging and metrics.
func (a *Agent) registerHandlers() {
	a.client.OnMessage(func(ev ircclient.MessageEvent) {
		a.metrics.RecordRawMessageReceived()

		kind := "PRIVMSG"
		if ev.IsNotice {
			kind = "NOTICE"
		}
		slog.Debug("message received",
			"kind", kind,
			"channel", ev.Channel,
			"nick", ev.Nick,
			"message", ev.Message,
		)
	})

	a.client.OnJoin(func(ev ircclient.JoinEvent) {
		if ev.Nick != a.client.Nick() {
			slog.Debug("user joined", "nick", ev.Nick, "channel", ev.Channel)
		}
	})

	a.client.OnPart(func(ev ircclient.PartEvent) {
		slog.Debug("user parted", "nick", ev.Nick, "channel", ev.Channel)
	})

	a.client.OnKick(func(ev ircclient.KickEvent) {
		slog.Warn("user kicked", "nick", ev.Nick, "channel", ev.Channel, "by", ev.KickedBy, "reason", ev.Message)
	})

	a.client.OnError(func(ev ircclient.ErrorEvent) {
		slog.Error("IRC error", "message", ev.Message)
	})

	a.client.OnConnect(func(ev ircclient.ConnectEvent) {
		slog.Info("agent connected", "server", ev.Server, "nick", ev.Nick)
	})

	a.client.OnDisconnect(func(ev ircclient.DisconnectEvent) {
		slog.Warn("agent disconnected", "server", ev.Server)
	})
}
