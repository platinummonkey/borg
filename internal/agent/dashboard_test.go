package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/platinummonkey/borg/pkg/protocol"
)

func newTestDashboard() *Dashboard {
	client := &stubClient{nick: "test-agent"}
	state := NewStateStore()
	ctx := NewContextStore()
	health := NewHealthMonitor(client, state)
	metrics := NewMetricsCollector()
	inspector := NewDebugInspector(state, ctx, 100)
	return NewDashboard(":0", health, metrics, inspector, state, ctx, nil, nil, nil, nil, nil)
}

func newTestDashboardWithData() *Dashboard {
	client := &stubClient{nick: "test-agent"}
	state := NewStateStore()
	ctx := NewContextStore()

	state.UpdateTask(&protocol.Message{
		Action: protocol.ActionStarted, Fields: map[string]string{"task": "task-a", "priority": "high"}, Nick: "agent-1", Timestamp: time.Now(),
	})
	state.UpdateTask(&protocol.Message{
		Action: protocol.ActionCompleted, Fields: map[string]string{"task": "task-b"}, Nick: "agent-2", Timestamp: time.Now(),
	})
	state.UpdateAgentStatus("agent-1", "#project", "task-a")

	ctx.Store(&protocol.Message{
		Action: protocol.ActionContext, Fields: map[string]string{"component": "auth", "project": "webapp", "status": "ok"}, Nick: "agent-1", Timestamp: time.Now(),
	})

	health := NewHealthMonitor(client, state)
	metrics := NewMetricsCollector()
	metrics.RecordRawMessageReceived()
	metrics.RecordMessageSent()

	inspector := NewDebugInspector(state, ctx, 100)
	inspector.RecordMessage(MessageLogEntry{
		Timestamp: time.Now(), Direction: "in", Channel: "#test", Nick: "agent-1", Action: "STARTED", Raw: "STARTED task=task-a",
	})

	return NewDashboard(":0", health, metrics, inspector, state, ctx, nil, nil, nil, nil, nil)
}

func dashboardHandler(d *Dashboard) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", d.handleHealth)
	mux.HandleFunc("GET /metrics", d.handleMetrics)
	mux.HandleFunc("GET /tasks", d.handleTasks)
	mux.HandleFunc("GET /dependencies", d.handleDependencies)
	mux.HandleFunc("GET /agents", d.handleAgents)
	mux.HandleFunc("GET /context", d.handleContext)
	mux.HandleFunc("GET /messages", d.handleMessages)
	mux.HandleFunc("GET /discovery", d.handleDiscovery)
	mux.HandleFunc("GET /taskboard", d.handleTaskBoard)
	mux.HandleFunc("GET /handoffs", d.handleHandoffs)
	mux.HandleFunc("GET /reviews", d.handleReviews)
	mux.HandleFunc("GET /consensus", d.handleConsensus)
	mux.HandleFunc("GET /", d.handleIndex)
	return mux
}

func TestDashboard_Health(t *testing.T) {
	d := newTestDashboardWithData()
	srv := httptest.NewServer(dashboardHandler(d))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var status HealthStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.Nick != "test-agent" {
		t.Errorf("Nick = %q, want test-agent", status.Nick)
	}
	if status.TaskStats.Total != 2 {
		t.Errorf("TaskStats.Total = %d, want 2", status.TaskStats.Total)
	}
}

func TestDashboard_Metrics(t *testing.T) {
	d := newTestDashboardWithData()
	srv := httptest.NewServer(dashboardHandler(d))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var snap MetricsSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatal(err)
	}
	if snap.MessagesReceived != 1 {
		t.Errorf("MessagesReceived = %d, want 1", snap.MessagesReceived)
	}
	if snap.MessagesSent != 1 {
		t.Errorf("MessagesSent = %d, want 1", snap.MessagesSent)
	}
}

func TestDashboard_Tasks(t *testing.T) {
	d := newTestDashboardWithData()
	srv := httptest.NewServer(dashboardHandler(d))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/tasks")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var tasks []TaskInfo
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Errorf("tasks = %d, want 2", len(tasks))
	}
}

func TestDashboard_Tasks_Empty(t *testing.T) {
	d := newTestDashboard()
	srv := httptest.NewServer(dashboardHandler(d))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/tasks")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var tasks []TaskInfo
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Errorf("tasks = %d, want 0", len(tasks))
	}
}

func TestDashboard_Dependencies(t *testing.T) {
	d := newTestDashboardWithData()
	srv := httptest.NewServer(dashboardHandler(d))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/dependencies")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var graph []TaskGraphNode
	if err := json.NewDecoder(resp.Body).Decode(&graph); err != nil {
		t.Fatal(err)
	}
	if len(graph) != 2 {
		t.Errorf("graph = %d nodes, want 2", len(graph))
	}
}

func TestDashboard_Agents(t *testing.T) {
	d := newTestDashboardWithData()
	srv := httptest.NewServer(dashboardHandler(d))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/agents")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var agents []AgentActivitySummary
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 {
		t.Errorf("agents = %d, want 1", len(agents))
	}
}

func TestDashboard_Context(t *testing.T) {
	d := newTestDashboardWithData()
	srv := httptest.NewServer(dashboardHandler(d))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/context")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var entries []ContextEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("context entries = %d, want 1", len(entries))
	}
}

func TestDashboard_Messages(t *testing.T) {
	d := newTestDashboardWithData()
	srv := httptest.NewServer(dashboardHandler(d))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/messages")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var messages []MessageLogEntry
	if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Errorf("messages = %d, want 1", len(messages))
	}
}

func TestDashboard_Index(t *testing.T) {
	d := newTestDashboard()
	srv := httptest.NewServer(dashboardHandler(d))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestDashboard_StartShutdown(t *testing.T) {
	d := newTestDashboard()
	d.addr = "127.0.0.1:0"
	if err := d.Start(); err != nil {
		t.Fatal(err)
	}
	if err := d.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}
