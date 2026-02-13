package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/platinummonkey/agent-chat/internal/config"
	"github.com/platinummonkey/agent-chat/internal/logging"
	"github.com/platinummonkey/agent-chat/pkg/ircclient"
	"github.com/platinummonkey/agent-chat/pkg/protocol"
)

// Agent orchestrates the IRC client, event handlers, and lifecycle management.
type Agent struct {
	cfg      *config.AppConfig
	client   ircclient.Client
	lc       *Lifecycle
	state    *StateStore
	context  *ContextStore
	protocol *ProtocolDispatcher
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

// initProtocol sets up the state store, context store, and protocol dispatcher.
func (a *Agent) initProtocol() {
	a.state = NewStateStore()
	a.context = NewContextStore()
	a.protocol = NewProtocolDispatcher(a.client, a.state, a.context)
	a.protocol.Register()
}

// Start connects the agent to the IRC server.
func (a *Agent) Start(ctx context.Context) error {
	return a.client.Connect(ctx)
}

// Run blocks until the agent is shut down (via signal or context cancellation).
func (a *Agent) Run(ctx context.Context) error {
	if err := a.Start(ctx); err != nil {
		return err
	}
	return a.lc.Wait(ctx)
}

// Shutdown performs a graceful shutdown: parts channels, disconnects.
func (a *Agent) Shutdown() {
	slog.Info("agent shutting down")
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
// The message is sanitized before sending.
func (a *Agent) SendProtocolMessage(target string, msg *protocol.Message) error {
	if err := protocol.Sanitize(msg); err != nil {
		return err
	}
	a.client.SendMessage(target, msg.String())
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

// registerHandlers sets up default event handlers for logging.
func (a *Agent) registerHandlers() {
	a.client.OnMessage(func(ev ircclient.MessageEvent) {
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
