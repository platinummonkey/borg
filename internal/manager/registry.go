package manager

import (
	"sync"
	"time"

	"github.com/platinummonkey/agent-chat/internal/agent"
	"github.com/platinummonkey/agent-chat/internal/cost"
	"github.com/platinummonkey/agent-chat/internal/spawner"
)

// AgentRecord tracks all known information about an agent.
type AgentRecord struct {
	Nick         string                 `json:"nick"`
	Host         string                 `json:"host,omitempty"`
	Status       string                 `json:"status"`
	SpawnerType  string                 `json:"spawner_type,omitempty"`
	DashboardURL string                 `json:"dashboard_url,omitempty"`
	CurrentTask  string                 `json:"current_task,omitempty"`
	Source       string                 `json:"source"` // "spawned", "discovered", "manual"
	Instance     *spawner.AgentInstance `json:"instance,omitempty"`
	Channels     []string               `json:"channels,omitempty"`
	Capabilities []string               `json:"capabilities,omitempty"`
	Metrics      *agent.MetricsSnapshot `json:"metrics,omitempty"`
	Health       *agent.HealthStatus    `json:"health,omitempty"`
	CostSummary  *cost.CostSummary      `json:"cost_summary,omitempty"`
	LastSeen     time.Time              `json:"last_seen"`
	RegisteredAt time.Time              `json:"registered_at"`
}

// AgentRegistry tracks all known agents (spawned + discovered).
type AgentRegistry struct {
	mu     sync.RWMutex
	agents map[string]*AgentRecord
}

// NewAgentRegistry creates an empty registry.
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		agents: make(map[string]*AgentRecord),
	}
}

// Register adds or replaces an agent record.
func (r *AgentRegistry) Register(rec *AgentRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rec.RegisteredAt.IsZero() {
		rec.RegisteredAt = time.Now()
	}
	if rec.LastSeen.IsZero() {
		rec.LastSeen = time.Now()
	}
	r.agents[rec.Nick] = rec
}

// Get returns a copy of the record for the given nick, or nil.
func (r *AgentRegistry) Get(nick string) *AgentRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec := r.agents[nick]
	if rec == nil {
		return nil
	}
	cp := *rec
	return &cp
}

// List returns all agent records.
func (r *AgentRegistry) List() []*AgentRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*AgentRecord, 0, len(r.agents))
	for _, rec := range r.agents {
		cp := *rec
		result = append(result, &cp)
	}
	return result
}

// UpdateFromDiscovery updates or creates a record from a CAPABILITIES-derived discovery entry.
func (r *AgentRegistry) UpdateFromDiscovery(cap *agent.AgentCapability) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.agents[cap.Nick]
	if !ok {
		rec = &AgentRecord{
			Nick:         cap.Nick,
			Source:       "discovered",
			Status:       "online",
			RegisteredAt: time.Now(),
		}
		r.agents[cap.Nick] = rec
	}

	rec.Capabilities = cap.Expertise
	rec.Channels = cap.Channels
	rec.CurrentTask = cap.CurrentTask
	rec.LastSeen = cap.UpdatedAt
	if rec.Status != "stopping" {
		rec.Status = "online"
	}
}

// UpdateHealth updates the health status for an agent.
func (r *AgentRegistry) UpdateHealth(nick string, health *agent.HealthStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.agents[nick]
	if !ok {
		return
	}
	rec.Health = health
	rec.LastSeen = time.Now()
	if health.Healthy {
		rec.Status = "online"
	}
}

// UpdateMetrics updates the metrics snapshot for an agent.
func (r *AgentRegistry) UpdateMetrics(nick string, metrics *agent.MetricsSnapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.agents[nick]
	if !ok {
		return
	}
	rec.Metrics = metrics
}

// UpdateCostSummary updates the cost summary for an agent.
func (r *AgentRegistry) UpdateCostSummary(nick string, summary *cost.CostSummary) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.agents[nick]
	if !ok {
		return
	}
	rec.CostSummary = summary
}

// UpdateStatus sets the status of an agent.
func (r *AgentRegistry) UpdateStatus(nick, status string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rec, ok := r.agents[nick]; ok {
		rec.Status = status
	}
}

// Remove deletes an agent from the registry.
func (r *AgentRegistry) Remove(nick string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.agents, nick)
}

// Count returns the total number of registered agents.
func (r *AgentRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.agents)
}
