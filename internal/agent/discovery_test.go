package agent

import (
	"testing"
	"time"
)

func TestDiscovery_UpdateGet(t *testing.T) {
	ds := NewDiscoveryStore(5 * time.Minute)

	ds.Update(&AgentCapability{
		Nick:        "agent-1",
		Expertise:   []string{"database", "testing"},
		Channels:    []string{"#project"},
		CurrentTask: "task-a",
	})

	cap := ds.Get("agent-1")
	if cap == nil {
		t.Fatal("expected capability for agent-1")
	}
	if cap.Nick != "agent-1" {
		t.Errorf("nick = %q, want agent-1", cap.Nick)
	}
	if len(cap.Expertise) != 2 || cap.Expertise[0] != "database" {
		t.Errorf("expertise = %v, want [database testing]", cap.Expertise)
	}
	if cap.CurrentTask != "task-a" {
		t.Errorf("current_task = %q, want task-a", cap.CurrentTask)
	}
}

func TestDiscovery_FindByExpertise(t *testing.T) {
	ds := NewDiscoveryStore(5 * time.Minute)

	ds.Update(&AgentCapability{Nick: "agent-1", Expertise: []string{"database", "testing"}})
	ds.Update(&AgentCapability{Nick: "agent-2", Expertise: []string{"frontend"}})
	ds.Update(&AgentCapability{Nick: "agent-3", Expertise: []string{"database"}})

	results := ds.FindByExpertise("database")
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	nicks := map[string]bool{}
	for _, r := range results {
		nicks[r.Nick] = true
	}
	if !nicks["agent-1"] || !nicks["agent-3"] {
		t.Errorf("expected agent-1 and agent-3, got %v", nicks)
	}
}

func TestDiscovery_FindByExpertise_CaseInsensitive(t *testing.T) {
	ds := NewDiscoveryStore(5 * time.Minute)
	ds.Update(&AgentCapability{Nick: "agent-1", Expertise: []string{"Database"}})

	results := ds.FindByExpertise("database")
	if len(results) != 1 {
		t.Fatalf("expected 1 result (case-insensitive), got %d", len(results))
	}
}

func TestDiscovery_TTLExpiry(t *testing.T) {
	ds := NewDiscoveryStore(50 * time.Millisecond)

	ds.Update(&AgentCapability{Nick: "agent-1", Expertise: []string{"db"}})

	cap := ds.Get("agent-1")
	if cap == nil {
		t.Fatal("should be present immediately")
	}

	time.Sleep(100 * time.Millisecond)

	cap = ds.Get("agent-1")
	if cap != nil {
		t.Error("should be expired")
	}

	active := ds.ListActive()
	if len(active) != 0 {
		t.Errorf("expected 0 active, got %d", len(active))
	}
}

func TestDiscovery_Prune(t *testing.T) {
	ds := NewDiscoveryStore(50 * time.Millisecond)

	ds.Update(&AgentCapability{Nick: "agent-1", Expertise: []string{"db"}})
	ds.Update(&AgentCapability{Nick: "agent-2", Expertise: []string{"api"}})

	time.Sleep(100 * time.Millisecond)

	ds.Prune()

	ds.mu.RLock()
	count := len(ds.agents)
	ds.mu.RUnlock()

	if count != 0 {
		t.Errorf("expected 0 agents after prune, got %d", count)
	}
}

func TestDiscovery_CopyOnRead(t *testing.T) {
	ds := NewDiscoveryStore(5 * time.Minute)
	ds.Update(&AgentCapability{Nick: "agent-1", Expertise: []string{"db"}})

	cap1 := ds.Get("agent-1")
	cap1.Expertise[0] = "modified"

	cap2 := ds.Get("agent-1")
	if cap2.Expertise[0] != "db" {
		t.Errorf("mutation leaked: expertise = %v", cap2.Expertise)
	}
}
