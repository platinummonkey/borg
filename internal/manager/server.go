package manager

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/platinummonkey/borg/internal/agent"
	"github.com/platinummonkey/borg/internal/spawner"
)

// Server is the HTTP + WebSocket server for the manager web UI.
type Server struct {
	addr       string
	listenAddr string
	manager    *Manager
	httpServer *http.Server
}

// NewServer creates a Server bound to the given manager.
func NewServer(addr string, mgr *Manager) *Server {
	return &Server{
		addr:    addr,
		manager: mgr,
	}
}

// Start begins serving the web UI.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API routes.
	mux.HandleFunc("GET /api/agents", s.handleAPIAgents)
	mux.HandleFunc("GET /api/agents/{nick}", s.handleAPIAgentDetail)
	mux.HandleFunc("POST /api/agents/spawn", s.handleAPISpawn)
	mux.HandleFunc("POST /api/agents/{nick}/stop", s.handleAPIStop)
	mux.HandleFunc("GET /api/tasks", s.handleAPITasks)
	mux.HandleFunc("GET /api/costs", s.handleAPICosts)
	mux.HandleFunc("GET /api/costs/by-agent", s.handleAPICostsByAgent)
	mux.HandleFunc("GET /api/costs/by-task", s.handleAPICostsByTask)
	mux.HandleFunc("GET /api/costs/by-model", s.handleAPICostsByModel)
	mux.HandleFunc("GET /api/messages", s.handleAPIMessages)
	mux.HandleFunc("GET /api/discovery", s.handleAPIDiscovery)

	// WebSocket.
	mux.HandleFunc("GET /ws", s.handleWS)

	// Pages (serve templates - Phase 6 will add full templates).
	mux.HandleFunc("GET /agents/{nick}", s.handlePageAgentDetail)
	mux.HandleFunc("GET /spawn", s.handlePageSpawn)
	mux.HandleFunc("GET /costs", s.handlePageCosts)
	mux.HandleFunc("GET /taskboard", s.handlePageTaskboard)
	mux.HandleFunc("GET /", s.handlePageDashboard)

	s.httpServer = &http.Server{
		Addr:         s.addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.listenAddr = ln.Addr().String()

	go func() {
		slog.Info("web server started", "addr", s.listenAddr)
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("web server error", "error", err)
		}
	}()

	return nil
}

// ListenAddr returns the actual listen address.
func (s *Server) ListenAddr() string { return s.listenAddr }

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

// --- API Handlers ---

func (s *Server) handleAPIAgents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager.Registry().List())
}

func (s *Server) handleAPIAgentDetail(w http.ResponseWriter, r *http.Request) {
	nick := r.PathValue("nick")
	rec := s.manager.Registry().Get(nick)
	if rec == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	writeJSON(w, rec)
}

func (s *Server) handleAPISpawn(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SpawnerType string             `json:"spawner_type"`
		Config      spawnConfigRequest `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cfg := req.Config.toSpawnConfig()
	rec, err := s.manager.SpawnAgent(r.Context(), req.SpawnerType, cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, rec)
}

func (s *Server) handleAPIStop(w http.ResponseWriter, r *http.Request) {
	nick := r.PathValue("nick")
	if err := s.manager.StopAgent(r.Context(), nick); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "stopped", "nick": nick})
}

func (s *Server) handleAPITasks(w http.ResponseWriter, r *http.Request) {
	tasks := s.manager.State().ListTasks()
	if tasks == nil {
		tasks = []*agent.TaskInfo{}
	}
	writeJSON(w, tasks)
}

func (s *Server) handleAPICosts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager.CostStore().TotalSummary())
}

func (s *Server) handleAPICostsByAgent(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager.CostStore().ByAgent())
}

func (s *Server) handleAPICostsByTask(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager.CostStore().ByTask())
}

func (s *Server) handleAPICostsByModel(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager.CostStore().ByModel())
}

func (s *Server) handleAPIMessages(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager.Inspector().RecentMessages(100))
}

func (s *Server) handleAPIDiscovery(w http.ResponseWriter, r *http.Request) {
	agents := s.manager.Discovery().ListActive()
	if agents == nil {
		agents = []*agent.AgentCapability{}
	}
	writeJSON(w, agents)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	serveWS(s.manager.Hub(), w, r)
}

// Page handlers (templates will be added in Phase 6).
func (s *Server) handlePageDashboard(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "dashboard.html", s.manager)
}

func (s *Server) handlePageAgentDetail(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "agent_detail.html", map[string]string{"nick": r.PathValue("nick")})
}

func (s *Server) handlePageSpawn(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "spawn.html", nil)
}

func (s *Server) handlePageCosts(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "costs.html", nil)
}

func (s *Server) handlePageTaskboard(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "taskboard.html", nil)
}

// spawnConfigRequest is the JSON request body for spawning agents.
type spawnConfigRequest struct {
	Nick          string            `json:"nick"`
	Server        string            `json:"server"`
	Username      string            `json:"username"`
	Password      string            `json:"password"`
	Channels      []string          `json:"channels"`
	Capabilities  []string          `json:"capabilities"`
	Roles         []string          `json:"roles"`
	DashboardAddr string            `json:"dashboard_addr"`
	ExtraFlags    map[string]string `json:"extra_flags"`
	BinaryPath    string            `json:"binary_path"`
	SSHHost       string            `json:"ssh_host"`
	SSHUser       string            `json:"ssh_user"`
	SSHKeyPath    string            `json:"ssh_key_path"`
	DockerImage   string            `json:"docker_image"`
	DockerNetwork string            `json:"docker_network"`
}

func (r *spawnConfigRequest) toSpawnConfig() spawner.SpawnConfig {
	return spawner.SpawnConfig{
		Nick:          r.Nick,
		Server:        r.Server,
		Username:      r.Username,
		Password:      r.Password,
		Channels:      r.Channels,
		Capabilities:  r.Capabilities,
		Roles:         r.Roles,
		DashboardAddr: r.DashboardAddr,
		ExtraFlags:    r.ExtraFlags,
		BinaryPath:    r.BinaryPath,
		SSHHost:       r.SSHHost,
		SSHUser:       r.SSHUser,
		SSHKeyPath:    r.SSHKeyPath,
		DockerImage:   r.DockerImage,
		DockerNetwork: r.DockerNetwork,
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		slog.Error("JSON encode error", "error", err)
	}
}

// serveWS upgrades an HTTP connection to WebSocket (implemented in Phase 5).
func serveWS(hub *Hub, w http.ResponseWriter, r *http.Request) {
	// Upgrade using standard library (no external dependency needed).
	// We'll use a simple Server-Sent Events fallback for now, and upgrade
	// to full WebSocket in Phase 5 with nhooyr.io/websocket.
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	conn := &wsConn{
		send: make(chan []byte, 256),
		done: make(chan struct{}),
	}
	hub.Register(conn)
	defer hub.Unregister(conn)

	ctx := r.Context()
	for {
		select {
		case msg, ok := <-conn.send:
			if !ok {
				return
			}
			w.Write([]byte("data: "))
			w.Write(msg)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}

// renderTemplate renders an HTML template (implemented in Phase 6).
func renderTemplate(w http.ResponseWriter, name string, data any) {
	if err := executeTemplate(w, name, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
