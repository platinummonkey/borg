package cost

import (
	"strconv"
	"sync"
	"time"

	"github.com/platinummonkey/borg/pkg/protocol"
)

// CostEntry represents a single cost report from an agent.
type CostEntry struct {
	Agent        string    `json:"agent"`
	Task         string    `json:"task"`
	Model        string    `json:"model"`
	Provider     string    `json:"provider"`
	Channel      string    `json:"channel"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	TotalTokens  int64     `json:"total_tokens"`
	CostUSD      float64   `json:"cost_usd"`
	Timestamp    time.Time `json:"timestamp"`
}

// CostSummary provides an aggregate view of cost data.
type CostSummary struct {
	TotalCostUSD float64 `json:"total_cost_usd"`
	TotalTokens  int64   `json:"total_tokens"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	EntryCount   int     `json:"entry_count"`
}

// CostStore records cost entries and provides aggregation queries.
// It is thread-safe.
type CostStore struct {
	mu      sync.RWMutex
	entries []CostEntry
}

// NewCostStore creates an empty CostStore.
func NewCostStore() *CostStore {
	return &CostStore{}
}

// RecordCost parses a COST-REPORT protocol message and stores the entry.
func (cs *CostStore) RecordCost(msg *protocol.Message) {
	if msg.Action != protocol.ActionCostReport {
		return
	}

	entry := CostEntry{
		Agent:    msg.Nick,
		Task:     msg.Get("task"),
		Model:    msg.Get("model"),
		Provider: msg.Get("provider"),
		Channel:  msg.Channel,
	}
	if msg.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	} else {
		entry.Timestamp = msg.Timestamp
	}

	if v := msg.Get("input-tokens"); v != "" {
		entry.InputTokens, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := msg.Get("output-tokens"); v != "" {
		entry.OutputTokens, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := msg.Get("total-tokens"); v != "" {
		entry.TotalTokens, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := msg.Get("cost-usd"); v != "" {
		entry.CostUSD, _ = strconv.ParseFloat(v, 64)
	}

	cs.mu.Lock()
	cs.entries = append(cs.entries, entry)
	cs.mu.Unlock()
}

// Record adds a pre-built CostEntry directly.
func (cs *CostStore) Record(entry CostEntry) {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	cs.mu.Lock()
	cs.entries = append(cs.entries, entry)
	cs.mu.Unlock()
}

// TotalSummary returns an aggregate summary across all entries.
func (cs *CostStore) TotalSummary() CostSummary {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return summarize(cs.entries)
}

// ByAgent returns cost summaries grouped by agent nick.
func (cs *CostStore) ByAgent() map[string]CostSummary {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return groupBy(cs.entries, func(e CostEntry) string { return e.Agent })
}

// ByTask returns cost summaries grouped by task name.
func (cs *CostStore) ByTask() map[string]CostSummary {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return groupBy(cs.entries, func(e CostEntry) string { return e.Task })
}

// ByModel returns cost summaries grouped by model name.
func (cs *CostStore) ByModel() map[string]CostSummary {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return groupBy(cs.entries, func(e CostEntry) string { return e.Model })
}

// Entries returns the most recent n entries (or all if n <= 0).
func (cs *CostStore) Entries(n int) []CostEntry {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if n <= 0 || n > len(cs.entries) {
		n = len(cs.entries)
	}
	start := len(cs.entries) - n
	result := make([]CostEntry, n)
	copy(result, cs.entries[start:])
	return result
}

// EntriesForAgent returns the most recent n entries for a specific agent.
func (cs *CostStore) EntriesForAgent(nick string, n int) []CostEntry {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	var filtered []CostEntry
	for _, e := range cs.entries {
		if e.Agent == nick {
			filtered = append(filtered, e)
		}
	}
	if n <= 0 || n > len(filtered) {
		n = len(filtered)
	}
	if n == 0 {
		return nil
	}
	start := len(filtered) - n
	return filtered[start:]
}

func summarize(entries []CostEntry) CostSummary {
	var s CostSummary
	s.EntryCount = len(entries)
	for _, e := range entries {
		s.TotalCostUSD += e.CostUSD
		s.TotalTokens += e.TotalTokens
		s.InputTokens += e.InputTokens
		s.OutputTokens += e.OutputTokens
	}
	return s
}

func groupBy(entries []CostEntry, keyFn func(CostEntry) string) map[string]CostSummary {
	groups := make(map[string][]CostEntry)
	for _, e := range entries {
		key := keyFn(e)
		groups[key] = append(groups[key], e)
	}
	result := make(map[string]CostSummary, len(groups))
	for key, group := range groups {
		result[key] = summarize(group)
	}
	return result
}
