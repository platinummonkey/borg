package manager

import (
	"sync"
	"testing"
	"time"

	"github.com/platinummonkey/borg/internal/agent"
	"github.com/platinummonkey/borg/internal/cost"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewAgentRegistry()
	r.Register(&AgentRecord{Nick: "agent-1", Status: "online", Source: "spawned"})

	rec := r.Get("agent-1")
	if rec == nil {
		t.Fatal("expected non-nil record")
	}
	if rec.Nick != "agent-1" {
		t.Errorf("nick = %q, want %q", rec.Nick, "agent-1")
	}
	if rec.Source != "spawned" {
		t.Errorf("source = %q, want %q", rec.Source, "spawned")
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewAgentRegistry()
	if r.Get("nonexistent") != nil {
		t.Error("expected nil for nonexistent agent")
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewAgentRegistry()
	r.Register(&AgentRecord{Nick: "a1", Status: "online"})
	r.Register(&AgentRecord{Nick: "a2", Status: "offline"})

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(list))
	}
}

func TestRegistry_UpdateFromDiscovery(t *testing.T) {
	r := NewAgentRegistry()

	cap := &agent.AgentCapability{
		Nick:      "agent-1",
		Expertise: []string{"go", "python"},
		Channels:  []string{"#dev"},
		UpdatedAt: time.Now(),
	}
	r.UpdateFromDiscovery(cap)

	rec := r.Get("agent-1")
	if rec == nil {
		t.Fatal("expected record after discovery")
	}
	if rec.Source != "discovered" {
		t.Errorf("source = %q, want %q", rec.Source, "discovered")
	}
	if len(rec.Capabilities) != 2 {
		t.Errorf("capabilities = %d, want 2", len(rec.Capabilities))
	}
}

func TestRegistry_UpdateFromDiscovery_ExistingAgent(t *testing.T) {
	r := NewAgentRegistry()
	r.Register(&AgentRecord{Nick: "agent-1", Source: "spawned", Status: "online"})

	cap := &agent.AgentCapability{
		Nick:        "agent-1",
		Expertise:   []string{"go"},
		CurrentTask: "auth",
		UpdatedAt:   time.Now(),
	}
	r.UpdateFromDiscovery(cap)

	rec := r.Get("agent-1")
	if rec.Source != "spawned" {
		t.Errorf("source should remain %q, got %q", "spawned", rec.Source)
	}
	if rec.CurrentTask != "auth" {
		t.Errorf("current_task = %q, want %q", rec.CurrentTask, "auth")
	}
}

func TestRegistry_UpdateHealth(t *testing.T) {
	r := NewAgentRegistry()
	r.Register(&AgentRecord{Nick: "a1", Status: "unknown"})

	r.UpdateHealth("a1", &agent.HealthStatus{Healthy: true, Nick: "a1"})

	rec := r.Get("a1")
	if rec.Status != "online" {
		t.Errorf("status = %q, want %q", rec.Status, "online")
	}
	if rec.Health == nil {
		t.Error("expected non-nil health")
	}
}

func TestRegistry_UpdateMetrics(t *testing.T) {
	r := NewAgentRegistry()
	r.Register(&AgentRecord{Nick: "a1"})

	snap := &agent.MetricsSnapshot{MessagesReceived: 42}
	r.UpdateMetrics("a1", snap)

	rec := r.Get("a1")
	if rec.Metrics == nil || rec.Metrics.MessagesReceived != 42 {
		t.Error("expected updated metrics")
	}
}

func TestRegistry_UpdateCostSummary(t *testing.T) {
	r := NewAgentRegistry()
	r.Register(&AgentRecord{Nick: "a1"})

	summary := &cost.CostSummary{TotalCostUSD: 1.50, EntryCount: 3}
	r.UpdateCostSummary("a1", summary)

	rec := r.Get("a1")
	if rec.CostSummary == nil || rec.CostSummary.TotalCostUSD != 1.50 {
		t.Error("expected updated cost summary")
	}
}

func TestRegistry_Remove(t *testing.T) {
	r := NewAgentRegistry()
	r.Register(&AgentRecord{Nick: "a1"})
	r.Remove("a1")
	if r.Get("a1") != nil {
		t.Error("expected nil after remove")
	}
	if r.Count() != 0 {
		t.Errorf("count = %d, want 0", r.Count())
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewAgentRegistry()
	var wg sync.WaitGroup

	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			nick := "agent-" + string(rune('A'+i%26))
			r.Register(&AgentRecord{Nick: nick, Status: "online"})
		}()
	}

	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.List()
			_ = r.Count()
		}()
	}

	wg.Wait()
}
