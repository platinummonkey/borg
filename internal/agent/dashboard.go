package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Dashboard provides an HTTP server with JSON APIs and an HTML dashboard
// for monitoring agent health, metrics, tasks, and messages.
type Dashboard struct {
	health     *HealthMonitor
	metrics    *MetricsCollector
	inspector  *DebugInspector
	state      *StateStore
	context    *ContextStore
	discovery  *DiscoveryStore
	taskBoard  *TaskBoard
	handoff    *HandoffStore
	review     *ReviewStore
	consensus  *ConsensusStore
	server     *http.Server
	addr       string
	listenAddr string
}

// NewDashboard creates a Dashboard wired to the given components.
func NewDashboard(
	addr string,
	health *HealthMonitor,
	metrics *MetricsCollector,
	inspector *DebugInspector,
	state *StateStore,
	context *ContextStore,
	discovery *DiscoveryStore,
	taskBoard *TaskBoard,
	handoff *HandoffStore,
	review *ReviewStore,
	consensus *ConsensusStore,
) *Dashboard {
	return &Dashboard{
		addr:      addr,
		health:    health,
		metrics:   metrics,
		inspector: inspector,
		state:     state,
		context:   context,
		discovery: discovery,
		taskBoard: taskBoard,
		handoff:   handoff,
		review:    review,
		consensus: consensus,
	}
}

// Start begins serving the HTTP dashboard in a background goroutine.
func (d *Dashboard) Start() error {
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

	d.server = &http.Server{
		Addr:         d.addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	ln, err := net.Listen("tcp", d.addr)
	if err != nil {
		return err
	}
	d.listenAddr = ln.Addr().String()

	go func() {
		slog.Info("dashboard started", "addr", ln.Addr().String())
		if err := d.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("dashboard server error", "error", err)
		}
	}()
	return nil
}

// ListenAddr returns the address the dashboard is actually listening on.
func (d *Dashboard) ListenAddr() string { return d.listenAddr }

// Shutdown gracefully shuts down the dashboard HTTP server.
func (d *Dashboard) Shutdown(ctx context.Context) error {
	if d.server == nil {
		return nil
	}
	return d.server.Shutdown(ctx)
}

func (d *Dashboard) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, d.health.Check())
}

func (d *Dashboard) handleMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, d.metrics.Snapshot())
}

func (d *Dashboard) handleTasks(w http.ResponseWriter, r *http.Request) {
	tasks := d.state.ListTasks()
	if tasks == nil {
		tasks = []*TaskInfo{}
	}
	writeJSON(w, tasks)
}

func (d *Dashboard) handleDependencies(w http.ResponseWriter, r *http.Request) {
	graph := d.inspector.TaskGraph()
	if graph == nil {
		graph = []TaskGraphNode{}
	}
	writeJSON(w, graph)
}

func (d *Dashboard) handleAgents(w http.ResponseWriter, r *http.Request) {
	activity := d.inspector.AgentActivity()
	if activity == nil {
		activity = []AgentActivitySummary{}
	}
	writeJSON(w, activity)
}

func (d *Dashboard) handleContext(w http.ResponseWriter, r *http.Request) {
	entries := d.context.ListEntries()
	if entries == nil {
		entries = []*ContextEntry{}
	}
	writeJSON(w, entries)
}

func (d *Dashboard) handleMessages(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, d.inspector.RecentMessages(100))
}

func (d *Dashboard) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	if d.discovery == nil {
		writeJSON(w, []*AgentCapability{})
		return
	}
	agents := d.discovery.ListActive()
	if agents == nil {
		agents = []*AgentCapability{}
	}
	writeJSON(w, agents)
}

func (d *Dashboard) handleTaskBoard(w http.ResponseWriter, r *http.Request) {
	if d.taskBoard == nil {
		writeJSON(w, []*OfferInfo{})
		return
	}
	offers := d.taskBoard.ListOffers()
	if offers == nil {
		offers = []*OfferInfo{}
	}
	writeJSON(w, offers)
}

func (d *Dashboard) handleHandoffs(w http.ResponseWriter, r *http.Request) {
	if d.handoff == nil {
		writeJSON(w, []*HandoffRecord{})
		return
	}
	handoffs := d.handoff.ListHandoffs()
	if handoffs == nil {
		handoffs = []*HandoffRecord{}
	}
	writeJSON(w, handoffs)
}

func (d *Dashboard) handleReviews(w http.ResponseWriter, r *http.Request) {
	if d.review == nil {
		writeJSON(w, []*ReviewRecord{})
		return
	}
	reviews := d.review.ListReviews()
	if reviews == nil {
		reviews = []*ReviewRecord{}
	}
	writeJSON(w, reviews)
}

func (d *Dashboard) handleConsensus(w http.ResponseWriter, r *http.Request) {
	if d.consensus == nil {
		writeJSON(w, []TopicSummary{})
		return
	}
	topics := d.consensus.ListTopics()
	if topics == nil {
		topics = []TopicSummary{}
	}
	writeJSON(w, topics)
}

func (d *Dashboard) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(dashboardHTML)) //nolint:errcheck
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		slog.Error("dashboard JSON encode error", "error", err)
	}
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Agent Chat Dashboard</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: monospace; background: #1a1a2e; color: #e0e0e0; padding: 16px; }
  h1 { color: #00d4ff; margin-bottom: 16px; }
  h2 { color: #00d4ff; margin: 16px 0 8px; font-size: 14px; text-transform: uppercase; }
  .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
  .card { background: #16213e; border: 1px solid #0f3460; border-radius: 6px; padding: 12px; overflow-x: auto; }
  .card.full { grid-column: 1 / -1; }
  table { width: 100%; border-collapse: collapse; font-size: 12px; }
  th { text-align: left; color: #00d4ff; padding: 4px 8px; border-bottom: 1px solid #0f3460; }
  td { padding: 4px 8px; border-bottom: 1px solid #0f3460; }
  .status-connected { color: #00ff88; }
  .status-disconnected { color: #ff4444; }
  .status-started { color: #ffaa00; }
  .status-completed { color: #00ff88; }
  .status-blocked { color: #ff4444; }
  .metric { display: inline-block; margin: 4px 12px 4px 0; }
  .metric-value { font-size: 20px; font-weight: bold; color: #00d4ff; }
  .metric-label { font-size: 10px; color: #888; }
  .refresh-info { font-size: 10px; color: #555; margin-top: 8px; }
  #error-banner { display: none; background: #ff4444; color: #fff; padding: 8px; margin-bottom: 8px; border-radius: 4px; }
</style>
</head>
<body>
<h1>Agent Chat Dashboard</h1>
<div id="error-banner"></div>
<div class="grid">
  <div class="card">
    <h2>Health</h2>
    <div id="health">Loading...</div>
  </div>
  <div class="card">
    <h2>Metrics</h2>
    <div id="metrics">Loading...</div>
  </div>
  <div class="card full">
    <h2>Tasks</h2>
    <div id="tasks">Loading...</div>
  </div>
  <div class="card full">
    <h2>Agents</h2>
    <div id="agents">Loading...</div>
  </div>
  <div class="card full">
    <h2>Context Entries</h2>
    <div id="context">Loading...</div>
  </div>
  <div class="card full">
    <h2>Recent Messages</h2>
    <div id="messages">Loading...</div>
  </div>
</div>
<div class="refresh-info">Auto-refreshes every 5 seconds</div>
<script>
function esc(s) {
  if (s == null) return '';
  var d = document.createElement('div');
  d.textContent = String(s);
  return d.innerHTML;
}

async function fetchJSON(url) {
  var res = await fetch(url);
  return res.json();
}

function statusClass(s) {
  if (s === true || s === 'completed') return 'status-connected';
  if (s === false || s === 'blocked') return 'status-disconnected';
  if (s === 'started') return 'status-started';
  return '';
}

function renderHealth(data) {
  var el = document.getElementById('health');
  el.textContent = '';
  var items = [
    {val: data.connected ? 'Yes' : 'No', cls: statusClass(data.connected), label: 'Connected'},
    {val: data.healthy ? 'Yes' : 'No', cls: statusClass(data.healthy), label: 'Healthy'},
    {val: data.nick || '-', cls: '', label: 'Nick'},
    {val: data.uptime || '-', cls: '', label: 'Uptime'},
    {val: (data.channels||[]).join(', ') || '-', cls: '', label: 'Channels'}
  ];
  items.forEach(function(item) {
    var m = document.createElement('div');
    m.className = 'metric';
    var v = document.createElement('div');
    v.className = 'metric-value ' + item.cls;
    v.textContent = item.val;
    var l = document.createElement('div');
    l.className = 'metric-label';
    l.textContent = item.label;
    m.appendChild(v);
    m.appendChild(l);
    el.appendChild(m);
  });
}

function renderMetrics(data) {
  var el = document.getElementById('metrics');
  el.textContent = '';
  var keys = ['messages_received','messages_sent','protocol_messages_in','protocol_messages_out','tasks_started','tasks_completed','tasks_blocked','dependencies_resolved','context_requests','context_shared','notifications_sent','help_requested'];
  keys.forEach(function(k) {
    var m = document.createElement('div');
    m.className = 'metric';
    var v = document.createElement('div');
    v.className = 'metric-value';
    v.textContent = data[k] || 0;
    var l = document.createElement('div');
    l.className = 'metric-label';
    l.textContent = k.replace(/_/g, ' ');
    m.appendChild(v);
    m.appendChild(l);
    el.appendChild(m);
  });
}

function createTable(headers, rows) {
  var table = document.createElement('table');
  var thead = document.createElement('tr');
  headers.forEach(function(h) {
    var th = document.createElement('th');
    th.textContent = h;
    thead.appendChild(th);
  });
  table.appendChild(thead);
  rows.forEach(function(row) {
    var tr = document.createElement('tr');
    row.forEach(function(cell) {
      var td = document.createElement('td');
      if (cell && cell.cls) {
        td.className = cell.cls;
        td.textContent = cell.text;
      } else {
        td.textContent = cell;
      }
      tr.appendChild(td);
    });
    table.appendChild(tr);
  });
  return table;
}

function renderTasks(data) {
  var el = document.getElementById('tasks');
  el.textContent = '';
  if (!data.length) { el.textContent = 'No tasks'; return; }
  var rows = data.map(function(t) {
    return [t.Name, {text: t.Status, cls: statusClass(t.Status)}, t.Priority||'-', t.LastAgent||'-', (t.Tags||[]).join(', ')];
  });
  el.appendChild(createTable(['Task','Status','Priority','Agent','Tags'], rows));
}

function renderAgents(data) {
  var el = document.getElementById('agents');
  el.textContent = '';
  if (!data.length) { el.textContent = 'No agents'; return; }
  var rows = data.map(function(a) {
    return [a.nick, a.channel, a.task||'-', new Date(a.updated_at).toLocaleTimeString()];
  });
  el.appendChild(createTable(['Nick','Channel','Task','Updated'], rows));
}

function renderContext(data) {
  var el = document.getElementById('context');
  el.textContent = '';
  if (!data.length) { el.textContent = 'No context entries'; return; }
  var rows = data.map(function(c) {
    return [c.Component, c.Project||'-', c.Status||'-', c.SharedBy||'-'];
  });
  el.appendChild(createTable(['Component','Project','Status','Shared By'], rows));
}

function renderMessages(data) {
  var el = document.getElementById('messages');
  el.textContent = '';
  if (!data.length) { el.textContent = 'No messages'; return; }
  var rows = data.map(function(m) {
    return [new Date(m.timestamp).toLocaleTimeString(), m.direction, m.channel||'-', m.nick||'-', m.action||'-', (m.raw||'').substring(0,80)];
  });
  el.appendChild(createTable(['Time','Dir','Channel','Nick','Action','Raw'], rows));
}

async function refresh() {
  try {
    var results = await Promise.all([
      fetchJSON('/health'), fetchJSON('/metrics'), fetchJSON('/tasks'),
      fetchJSON('/agents'), fetchJSON('/context'), fetchJSON('/messages')
    ]);
    renderHealth(results[0]);
    renderMetrics(results[1]);
    renderTasks(results[2]);
    renderAgents(results[3]);
    renderContext(results[4]);
    renderMessages(results[5]);
    document.getElementById('error-banner').style.display = 'none';
  } catch(e) {
    var banner = document.getElementById('error-banner');
    banner.style.display = 'block';
    banner.textContent = 'Failed to refresh: ' + e.message;
  }
}
refresh();
setInterval(refresh, 5000);
</script>
</body>
</html>`
