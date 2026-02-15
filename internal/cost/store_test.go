package cost

import (
	"sync"
	"testing"
	"time"

	"github.com/platinummonkey/agent-chat/pkg/protocol"
)

func TestRecordCost(t *testing.T) {
	cs := NewCostStore()

	msg := &protocol.Message{
		Action: protocol.ActionCostReport,
		Fields: map[string]string{
			"task":          "auth",
			"input-tokens":  "1500",
			"output-tokens": "500",
			"total-tokens":  "2000",
			"cost-usd":      "0.0125",
			"model":         "claude-sonnet-4-5-20250929",
			"provider":      "anthropic",
		},
		Nick:      "agent-1",
		Channel:   "#project",
		Timestamp: time.Now(),
	}

	cs.RecordCost(msg)

	entries := cs.Entries(0)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Agent != "agent-1" {
		t.Errorf("agent = %q, want %q", e.Agent, "agent-1")
	}
	if e.Task != "auth" {
		t.Errorf("task = %q, want %q", e.Task, "auth")
	}
	if e.InputTokens != 1500 {
		t.Errorf("input_tokens = %d, want %d", e.InputTokens, 1500)
	}
	if e.OutputTokens != 500 {
		t.Errorf("output_tokens = %d, want %d", e.OutputTokens, 500)
	}
	if e.TotalTokens != 2000 {
		t.Errorf("total_tokens = %d, want %d", e.TotalTokens, 2000)
	}
	if e.CostUSD != 0.0125 {
		t.Errorf("cost_usd = %f, want %f", e.CostUSD, 0.0125)
	}
	if e.Model != "claude-sonnet-4-5-20250929" {
		t.Errorf("model = %q, want %q", e.Model, "claude-sonnet-4-5-20250929")
	}
	if e.Provider != "anthropic" {
		t.Errorf("provider = %q, want %q", e.Provider, "anthropic")
	}
}

func TestRecordCost_SkipsNonCostAction(t *testing.T) {
	cs := NewCostStore()
	msg := &protocol.Message{
		Action: protocol.ActionStarted,
		Fields: map[string]string{"task": "auth"},
	}
	cs.RecordCost(msg)
	if len(cs.Entries(0)) != 0 {
		t.Fatal("expected no entries for non-COST-REPORT action")
	}
}

func TestTotalSummary(t *testing.T) {
	cs := NewCostStore()
	cs.Record(CostEntry{Agent: "a1", Task: "t1", InputTokens: 100, OutputTokens: 50, TotalTokens: 150, CostUSD: 0.01})
	cs.Record(CostEntry{Agent: "a2", Task: "t2", InputTokens: 200, OutputTokens: 100, TotalTokens: 300, CostUSD: 0.02})

	s := cs.TotalSummary()
	if s.EntryCount != 2 {
		t.Errorf("entry_count = %d, want 2", s.EntryCount)
	}
	if s.TotalCostUSD != 0.03 {
		t.Errorf("total_cost = %f, want 0.03", s.TotalCostUSD)
	}
	if s.TotalTokens != 450 {
		t.Errorf("total_tokens = %d, want 450", s.TotalTokens)
	}
	if s.InputTokens != 300 {
		t.Errorf("input_tokens = %d, want 300", s.InputTokens)
	}
	if s.OutputTokens != 150 {
		t.Errorf("output_tokens = %d, want 150", s.OutputTokens)
	}
}

func TestByAgent(t *testing.T) {
	cs := NewCostStore()
	cs.Record(CostEntry{Agent: "a1", CostUSD: 0.01, TotalTokens: 100})
	cs.Record(CostEntry{Agent: "a1", CostUSD: 0.02, TotalTokens: 200})
	cs.Record(CostEntry{Agent: "a2", CostUSD: 0.05, TotalTokens: 500})

	byAgent := cs.ByAgent()
	if len(byAgent) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(byAgent))
	}
	if byAgent["a1"].EntryCount != 2 {
		t.Errorf("a1 entry count = %d, want 2", byAgent["a1"].EntryCount)
	}
	if byAgent["a1"].TotalCostUSD != 0.03 {
		t.Errorf("a1 cost = %f, want 0.03", byAgent["a1"].TotalCostUSD)
	}
	if byAgent["a2"].EntryCount != 1 {
		t.Errorf("a2 entry count = %d, want 1", byAgent["a2"].EntryCount)
	}
}

func TestByTask(t *testing.T) {
	cs := NewCostStore()
	cs.Record(CostEntry{Task: "auth", CostUSD: 0.01})
	cs.Record(CostEntry{Task: "auth", CostUSD: 0.02})
	cs.Record(CostEntry{Task: "api", CostUSD: 0.05})

	byTask := cs.ByTask()
	if byTask["auth"].EntryCount != 2 {
		t.Errorf("auth entry count = %d, want 2", byTask["auth"].EntryCount)
	}
	if byTask["api"].EntryCount != 1 {
		t.Errorf("api entry count = %d, want 1", byTask["api"].EntryCount)
	}
}

func TestByModel(t *testing.T) {
	cs := NewCostStore()
	cs.Record(CostEntry{Model: "gpt-4", CostUSD: 0.10})
	cs.Record(CostEntry{Model: "claude", CostUSD: 0.05})
	cs.Record(CostEntry{Model: "claude", CostUSD: 0.03})

	byModel := cs.ByModel()
	if byModel["gpt-4"].EntryCount != 1 {
		t.Errorf("gpt-4 entry count = %d, want 1", byModel["gpt-4"].EntryCount)
	}
	if byModel["claude"].TotalCostUSD != 0.08 {
		t.Errorf("claude cost = %f, want 0.08", byModel["claude"].TotalCostUSD)
	}
}

func TestEntries_Limit(t *testing.T) {
	cs := NewCostStore()
	for i := range 10 {
		cs.Record(CostEntry{Agent: "a", CostUSD: float64(i)})
	}

	entries := cs.Entries(3)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	// Should be the last 3.
	if entries[0].CostUSD != 7 {
		t.Errorf("first entry cost = %f, want 7", entries[0].CostUSD)
	}
}

func TestEntriesForAgent(t *testing.T) {
	cs := NewCostStore()
	cs.Record(CostEntry{Agent: "a1", CostUSD: 0.01})
	cs.Record(CostEntry{Agent: "a2", CostUSD: 0.02})
	cs.Record(CostEntry{Agent: "a1", CostUSD: 0.03})

	entries := cs.EntriesForAgent("a1", 0)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries for a1, got %d", len(entries))
	}

	entries = cs.EntriesForAgent("a1", 1)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for a1 with limit, got %d", len(entries))
	}
	if entries[0].CostUSD != 0.03 {
		t.Errorf("cost = %f, want 0.03", entries[0].CostUSD)
	}
}

func TestConcurrentAccess(t *testing.T) {
	cs := NewCostStore()
	var wg sync.WaitGroup

	for i := range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cs.Record(CostEntry{Agent: "a", CostUSD: float64(i) * 0.001})
		}()
	}

	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cs.TotalSummary()
			_ = cs.ByAgent()
			_ = cs.Entries(10)
		}()
	}

	wg.Wait()

	s := cs.TotalSummary()
	if s.EntryCount != 100 {
		t.Errorf("entry_count = %d, want 100", s.EntryCount)
	}
}
