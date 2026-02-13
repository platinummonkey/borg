package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/platinummonkey/agent-chat/internal/config"
	"github.com/platinummonkey/agent-chat/internal/logging"
	"github.com/platinummonkey/agent-chat/pkg/ircclient"
)

// Agent orchestrates the IRC client, event handlers, and lifecycle management.
type Agent struct {
	cfg    *config.AppConfig
	client ircclient.Client
	lc     *Lifecycle
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
	a.registerHandlers()
	a.lc = NewLifecycle(a)
	return a
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
