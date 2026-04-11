package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/platinummonkey/borg/internal/agent"
	"github.com/platinummonkey/borg/internal/cost"
	"github.com/platinummonkey/borg/internal/logging"
	"github.com/platinummonkey/borg/internal/spawner"
	"github.com/platinummonkey/borg/pkg/ircclient"
	"github.com/platinummonkey/borg/pkg/protocol"
)

// Manager is the central management plane for observing and controlling agents.
type Manager struct {
	cfg       *ManagerConfig
	client    ircclient.Client
	registry  *AgentRegistry
	costStore *cost.CostStore
	spawners  map[string]spawner.Spawner
	hub       *Hub
	server    *Server

	// Stores populated from IRC observation.
	state     *agent.StateStore
	discovery *agent.DiscoveryStore
	ctxStore  *agent.ContextStore
	taskBoard *agent.TaskBoard
	inspector *agent.DebugInspector

	pollStop chan struct{}
}

// New creates a Manager from configuration.
func New(cfg *ManagerConfig) (*Manager, error) {
	logging.Setup(cfg.LogLevel, cfg.LogFmt, nil)

	client, err := ircclient.NewClient(cfg.IRC)
	if err != nil {
		return nil, fmt.Errorf("create IRC client: %w", err)
	}

	state := agent.NewStateStore()
	ctxStore := agent.NewContextStore()
	discovery := agent.NewDiscoveryStore(5 * time.Minute)
	taskBoard := agent.NewTaskBoard(state, 0)
	inspector := agent.NewDebugInspector(state, ctxStore, 5000)

	hub := NewHub()

	m := &Manager{
		cfg:       cfg,
		client:    client,
		registry:  NewAgentRegistry(),
		costStore: cost.NewCostStore(),
		spawners:  make(map[string]spawner.Spawner),
		hub:       hub,
		state:     state,
		discovery: discovery,
		ctxStore:  ctxStore,
		taskBoard: taskBoard,
		inspector: inspector,
	}

	// Register spawners.
	m.spawners["local"] = spawner.NewLocalSpawner()
	m.spawners["ssh"] = spawner.NewSSHSpawner()
	m.spawners["docker"] = spawner.NewDockerSpawner()

	m.server = NewServer(cfg.ListenAddr, m)

	return m, nil
}

// NewWithClient creates a Manager with a pre-built IRC client (for testing).
func NewWithClient(cfg *ManagerConfig, client ircclient.Client) *Manager {
	state := agent.NewStateStore()
	ctxStore := agent.NewContextStore()
	discovery := agent.NewDiscoveryStore(5 * time.Minute)
	taskBoard := agent.NewTaskBoard(state, 0)
	inspector := agent.NewDebugInspector(state, ctxStore, 5000)

	hub := NewHub()

	m := &Manager{
		cfg:       cfg,
		client:    client,
		registry:  NewAgentRegistry(),
		costStore: cost.NewCostStore(),
		spawners:  make(map[string]spawner.Spawner),
		hub:       hub,
		state:     state,
		discovery: discovery,
		ctxStore:  ctxStore,
		taskBoard: taskBoard,
		inspector: inspector,
	}

	m.spawners["local"] = spawner.NewLocalSpawner()
	m.spawners["ssh"] = spawner.NewSSHSpawner()
	m.spawners["docker"] = spawner.NewDockerSpawner()

	m.server = NewServer(cfg.ListenAddr, m)

	return m
}

// Start connects to IRC, starts the web server, and begins the dashboard poller.
func (m *Manager) Start(ctx context.Context) error {
	// Register IRC message handler.
	m.client.OnMessage(func(ev ircclient.MessageEvent) {
		m.handleIRCMessage(ev)
	})

	if err := m.client.Connect(ctx); err != nil {
		return fmt.Errorf("IRC connect: %w", err)
	}

	if err := m.server.Start(); err != nil {
		return fmt.Errorf("web server: %w", err)
	}

	// Start dashboard poller.
	m.pollStop = make(chan struct{})
	go m.pollLoop()

	slog.Info("manager started",
		"listen_addr", m.server.ListenAddr(),
		"irc_server", m.cfg.IRC.Server,
		"nick", m.cfg.IRC.Nick,
	)

	return nil
}

// Run starts the manager and blocks until context cancellation or signal.
func (m *Manager) Run(ctx context.Context) error {
	if err := m.Start(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	m.Shutdown()
	return nil
}

// Shutdown stops all components gracefully.
func (m *Manager) Shutdown() {
	slog.Info("manager shutting down")
	if m.pollStop != nil {
		close(m.pollStop)
	}
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = m.server.Shutdown(shutCtx)
	m.client.Disconnect()
}

// SpawnAgent launches an agent using the specified spawner type.
func (m *Manager) SpawnAgent(ctx context.Context, spawnerType string, cfg spawner.SpawnConfig) (*AgentRecord, error) {
	sp, ok := m.spawners[spawnerType]
	if !ok {
		return nil, fmt.Errorf("unknown spawner type: %s", spawnerType)
	}

	// Fill in default binary path for local spawner.
	if spawnerType == "local" && cfg.BinaryPath == "" {
		cfg.BinaryPath = m.cfg.AgentBinary
	}

	if err := sp.PreSpawn(ctx, cfg); err != nil {
		return nil, fmt.Errorf("pre-spawn: %w", err)
	}

	inst, err := sp.Spawn(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("spawn: %w", err)
	}

	if err := sp.PostSpawn(ctx, inst); err != nil {
		slog.Warn("post-spawn check failed", "nick", cfg.Nick, "error", err)
	}

	rec := &AgentRecord{
		Nick:         cfg.Nick,
		Host:         inst.Host,
		Status:       string(inst.Status),
		SpawnerType:  spawnerType,
		DashboardURL: inst.DashboardURL,
		Source:       "spawned",
		Instance:     inst,
		Channels:     cfg.Channels,
		Capabilities: cfg.Capabilities,
	}
	m.registry.Register(rec)

	m.broadcastEvent("agent_update", rec)

	return rec, nil
}

// StopAgent stops a spawned agent.
func (m *Manager) StopAgent(ctx context.Context, nick string) error {
	rec := m.registry.Get(nick)
	if rec == nil {
		return fmt.Errorf("agent %q not found", nick)
	}
	if rec.Instance == nil {
		return fmt.Errorf("agent %q was not spawned by manager", nick)
	}

	sp, ok := m.spawners[rec.SpawnerType]
	if !ok {
		return fmt.Errorf("unknown spawner type: %s", rec.SpawnerType)
	}

	m.registry.UpdateStatus(nick, "stopping")

	if err := sp.PreStop(ctx, rec.Instance); err != nil {
		slog.Warn("pre-stop failed", "nick", nick, "error", err)
	}
	if err := sp.Stop(ctx, rec.Instance); err != nil {
		return fmt.Errorf("stop: %w", err)
	}
	if err := sp.PostStop(ctx, rec.Instance); err != nil {
		slog.Warn("post-stop failed", "nick", nick, "error", err)
	}

	m.registry.UpdateStatus(nick, "stopped")
	m.broadcastEvent("agent_update", m.registry.Get(nick))

	return nil
}

// Registry returns the agent registry.
func (m *Manager) Registry() *AgentRegistry { return m.registry }

// CostStore returns the cost store.
func (m *Manager) CostStore() *cost.CostStore { return m.costStore }

// State returns the state store.
func (m *Manager) State() *agent.StateStore { return m.state }

// Discovery returns the discovery store.
func (m *Manager) Discovery() *agent.DiscoveryStore { return m.discovery }

// Inspector returns the debug inspector.
func (m *Manager) Inspector() *agent.DebugInspector { return m.inspector }

// TaskBoard returns the task board.
func (m *Manager) TaskBoard() *agent.TaskBoard { return m.taskBoard }

// Hub returns the WebSocket hub.
func (m *Manager) Hub() *Hub { return m.hub }

// handleIRCMessage processes all incoming IRC messages for protocol content.
func (m *Manager) handleIRCMessage(ev ircclient.MessageEvent) {
	if !protocol.IsProtocolMessage(ev.Message) {
		return
	}

	msg, err := protocol.Parse(ev.Message)
	if err != nil {
		slog.Debug("failed to parse protocol message", "error", err, "raw", ev.Message)
		return
	}

	msg.Channel = ev.Channel
	msg.Nick = ev.Nick
	msg.Timestamp = ev.Timestamp
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	// Record message in inspector.
	m.inspector.RecordMessage(agent.MessageLogEntry{
		Timestamp: msg.Timestamp,
		Direction: "in",
		Channel:   msg.Channel,
		Nick:      msg.Nick,
		Action:    string(msg.Action),
		Raw:       msg.String(),
	})

	// Update stores based on action.
	m.dispatchToStores(msg)

	// Broadcast to WebSocket clients.
	m.broadcastEvent("message", map[string]string{
		"nick":    msg.Nick,
		"channel": msg.Channel,
		"action":  string(msg.Action),
		"raw":     msg.String(),
	})
}

// dispatchToStores routes protocol messages to the appropriate stores.
func (m *Manager) dispatchToStores(msg *protocol.Message) {
	switch msg.Action {
	case protocol.ActionStarted, protocol.ActionCompleted, protocol.ActionBlocked,
		protocol.ActionAcknowledged, protocol.ActionOffer, protocol.ActionClaim,
		protocol.ActionAssign, protocol.ActionAccept, protocol.ActionDecline,
		protocol.ActionYield, protocol.ActionCheckpoint, protocol.ActionHandoff:
		m.state.UpdateTask(msg)
		if taskName := msg.Get("task"); taskName != "" {
			m.state.UpdateAgentStatus(msg.Nick, msg.Channel, taskName)
		}

	case protocol.ActionContext:
		m.ctxStore.Store(msg)
	case protocol.ActionSharingContext:
		m.ctxStore.StorePayload(msg)
	case protocol.ActionRequestContext:
		m.ctxStore.TrackRequest(msg)

	case protocol.ActionCapabilities:
		m.handleCapabilities(msg)

	case protocol.ActionCostReport:
		m.costStore.RecordCost(msg)
		m.broadcastEvent("cost", map[string]string{
			"agent": msg.Nick,
			"task":  msg.Get("task"),
			"cost":  msg.Get("cost-usd"),
		})
	}

	// TaskBoard updates.
	task := msg.Get("task")
	switch msg.Action {
	case protocol.ActionOffer:
		m.taskBoard.RecordOffer(task, msg.Channel, msg.Nick, msg.Get("priority"), msg.Get("scope"))
		m.broadcastEvent("task_update", nil)
	case protocol.ActionClaim:
		load := 0
		if l := msg.Get("load"); l != "" {
			_, _ = fmt.Sscanf(l, "%d", &load)
		}
		m.taskBoard.RecordClaim(task, msg.Nick, load)
	case protocol.ActionAssign:
		m.taskBoard.RecordAssign(task, msg.Get("to"), msg.Nick, msg.Channel)
	case protocol.ActionDecline:
		m.taskBoard.RecordDecline(task)
	case protocol.ActionYield:
		m.taskBoard.RecordYield(task)
	}
}

// handleCapabilities processes CAPABILITIES messages for discovery.
func (m *Manager) handleCapabilities(msg *protocol.Message) {
	expertise := msg.Get("expertise")
	var expertiseTags []string
	if expertise != "" {
		for _, tag := range strings.Split(expertise, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				expertiseTags = append(expertiseTags, tag)
			}
		}
	}

	cap := &agent.AgentCapability{
		Nick:        msg.Nick,
		Expertise:   expertiseTags,
		CurrentTask: msg.Get("current-task"),
		UpdatedAt:   msg.Timestamp,
	}
	if channels := msg.Get("channels"); channels != "" {
		for _, ch := range strings.Split(channels, ",") {
			ch = strings.TrimSpace(ch)
			if ch != "" {
				cap.Channels = append(cap.Channels, ch)
			}
		}
	}

	m.discovery.Update(cap)
	m.registry.UpdateFromDiscovery(cap)
	m.broadcastEvent("agent_update", m.registry.Get(msg.Nick))
}

// pollLoop periodically fetches health and metrics from agent dashboards.
func (m *Manager) pollLoop() {
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.pollAgentDashboards()
		case <-m.pollStop:
			return
		}
	}
}

// pollAgentDashboards fetches /health and /metrics from all agents with dashboard URLs.
func (m *Manager) pollAgentDashboards() {
	agents := m.registry.List()
	for _, rec := range agents {
		if rec.DashboardURL == "" {
			continue
		}
		go m.pollSingleAgent(rec.Nick, rec.DashboardURL)
	}
}

func (m *Manager) pollSingleAgent(nick, dashboardURL string) {
	client := &http.Client{Timeout: 5 * time.Second}

	// Poll health.
	healthURL := dashboardURL + "/health"
	resp, err := client.Get(healthURL)
	if err != nil {
		slog.Debug("poll health failed", "nick", nick, "error", err)
		return
	}
	defer resp.Body.Close()

	var health agent.HealthStatus
	if err := json.NewDecoder(resp.Body).Decode(&health); err == nil {
		m.registry.UpdateHealth(nick, &health)
	}

	// Poll metrics.
	metricsURL := dashboardURL + "/metrics"
	resp2, err := client.Get(metricsURL)
	if err != nil {
		return
	}
	defer func() { _ = resp2.Body.Close() }()

	var metrics agent.MetricsSnapshot
	if err := json.NewDecoder(resp2.Body).Decode(&metrics); err == nil {
		m.registry.UpdateMetrics(nick, &metrics)
	}
}

// broadcastEvent sends a typed event to all WebSocket clients.
func (m *Manager) broadcastEvent(eventType string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	m.hub.Broadcast(WSMessage{
		Type:    eventType,
		Payload: data,
	})
}
