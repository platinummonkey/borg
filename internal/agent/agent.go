package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/platinummonkey/agent-chat/internal/config"
	"github.com/platinummonkey/agent-chat/internal/logging"
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
	a.context = NewContextStore()
	a.protocol = NewProtocolDispatcher(a.client, a.state, a.context)
	a.protocol.Register()
	a.notifier = NewNotifier(a.client)
	a.protocol.OnProtocolMessage(a.notifier.HandleMessage)

	a.health = NewHealthMonitor(a.client, a.state)
	a.metrics = NewMetricsCollector()
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
}

// Start connects the agent to the IRC server and optionally starts the dashboard.
func (a *Agent) Start(ctx context.Context) error {
	if a.cfg.DashboardAddr != "" {
		a.dashboard = NewDashboard(a.cfg.DashboardAddr, a.health, a.metrics, a.inspector, a.state, a.context)
		if err := a.dashboard.Start(); err != nil {
			return fmt.Errorf("start dashboard: %w", err)
		}
	}
	return a.client.Connect(ctx)
}

// Run blocks until the agent is shut down (via signal or context cancellation).
func (a *Agent) Run(ctx context.Context) error {
	if err := a.Start(ctx); err != nil {
		return err
	}
	return a.lc.Wait(ctx)
}

// Shutdown performs a graceful shutdown: stops dashboard, parts channels, disconnects.
func (a *Agent) Shutdown() {
	slog.Info("agent shutting down")
	if a.dashboard != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.dashboard.Shutdown(ctx); err != nil {
			slog.Error("dashboard shutdown error", "error", err)
		}
	}
	for _, ch := range a.client.JoinedChannels() {
		a.client.Part(ch)
	}
	a.client.Disconnect()
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
	if err := protocol.Sanitize(msg); err != nil {
		return err
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
