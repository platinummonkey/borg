package agent

import (
	"sync"
	"time"
)

// TaskGraphNode represents a task and its dependency edges in the task graph.
type TaskGraphNode struct {
	Task      *TaskInfo        `json:"task"`
	BlockedBy []DependencyEdge `json:"blocked_by"`
	Blocking  []DependencyEdge `json:"blocking"`
}

// BlockedChainEntry represents a single entry in a blocked dependency chain.
type BlockedChainEntry struct {
	TaskName string `json:"task"`
	Status   string `json:"status"`
	Resolved bool   `json:"resolved"`
}

// AgentActivitySummary represents a summary of an agent's current activity.
type AgentActivitySummary struct {
	Nick      string    `json:"nick"`
	Channel   string    `json:"channel"`
	TaskName  string    `json:"task"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MessageLogEntry represents a single logged protocol message.
type MessageLogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Direction string    `json:"direction"` // "in" or "out"
	Channel   string    `json:"channel"`
	Nick      string    `json:"nick"`
	Action    string    `json:"action"`
	Raw       string    `json:"raw"`
}

// DebugInspector provides debugging and inspection capabilities for the agent.
// It maintains a ring buffer of recent messages and provides methods to query
// the task graph and dependency chains.
type DebugInspector struct {
	state   *StateStore
	context *ContextStore
	mu      sync.Mutex
	msgLog  []MessageLogEntry
	maxLog  int
	logPos  int
	logFull bool
}

// NewDebugInspector creates a DebugInspector with the given ring buffer capacity.
func NewDebugInspector(state *StateStore, context *ContextStore, maxLogEntries int) *DebugInspector {
	if maxLogEntries <= 0 {
		maxLogEntries = 1000
	}
	return &DebugInspector{
		state:   state,
		context: context,
		msgLog:  make([]MessageLogEntry, maxLogEntries),
		maxLog:  maxLogEntries,
	}
}

// TaskGraph returns all tasks with their dependency edges.
func (d *DebugInspector) TaskGraph() []TaskGraphNode {
	tasks := d.state.ListTasks()
	nodes := make([]TaskGraphNode, 0, len(tasks))
	for _, task := range tasks {
		blockedBy := d.state.BlockersOf(task.Name, true)
		if blockedBy == nil {
			blockedBy = []DependencyEdge{}
		}
		blocking := d.state.BlockedBy(task.Name, true)
		if blocking == nil {
			blocking = []DependencyEdge{}
		}
		nodes = append(nodes, TaskGraphNode{
			Task:      task,
			BlockedBy: blockedBy,
			Blocking:  blocking,
		})
	}
	return nodes
}

// BlockedChain returns the transitive dependency chain for a task.
// Each entry includes the task name, its status, and whether the dependency is resolved.
func (d *DebugInspector) BlockedChain(taskName string) []BlockedChainEntry {
	deps := d.state.TransitiveDependencies(taskName)
	chain := make([]BlockedChainEntry, 0, len(deps))
	for _, depName := range deps {
		status := ""
		info := d.state.GetTask(depName)
		if info != nil {
			status = string(info.Status)
		}

		// Check if the edge from taskName to this dep is resolved.
		resolved := false
		edges := d.state.BlockersOf(taskName, true)
		for _, e := range edges {
			if e.BlockedBy == depName {
				resolved = e.Resolved
				break
			}
		}

		chain = append(chain, BlockedChainEntry{
			TaskName: depName,
			Status:   status,
			Resolved: resolved,
		})
	}
	return chain
}

// AgentActivity returns a summary of all known agent activity.
func (d *DebugInspector) AgentActivity() []AgentActivitySummary {
	agents := d.state.ListAgents()
	summaries := make([]AgentActivitySummary, 0, len(agents))
	for _, a := range agents {
		summaries = append(summaries, AgentActivitySummary{
			Nick:      a.Nick,
			Channel:   a.Channel,
			TaskName:  a.TaskName,
			UpdatedAt: a.UpdatedAt,
		})
	}
	return summaries
}

// RecordMessage adds a message to the ring buffer.
func (d *DebugInspector) RecordMessage(entry MessageLogEntry) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.msgLog[d.logPos] = entry
	d.logPos++
	if d.logPos >= d.maxLog {
		d.logPos = 0
		d.logFull = true
	}
}

// RecentMessages returns the most recent n messages from the ring buffer,
// ordered from oldest to newest.
func (d *DebugInspector) RecentMessages(n int) []MessageLogEntry {
	d.mu.Lock()
	defer d.mu.Unlock()

	total := d.logPos
	if d.logFull {
		total = d.maxLog
	}
	if n <= 0 || n > total {
		n = total
	}
	if n == 0 {
		return []MessageLogEntry{}
	}

	result := make([]MessageLogEntry, 0, n)

	if d.logFull {
		// Ring buffer has wrapped. Read from (logPos - n + maxLog) % maxLog.
		start := (d.logPos - n + d.maxLog) % d.maxLog
		for i := 0; i < n; i++ {
			idx := (start + i) % d.maxLog
			result = append(result, d.msgLog[idx])
		}
	} else {
		// Ring buffer hasn't wrapped yet. Read from logPos - n.
		start := d.logPos - n
		for i := 0; i < n; i++ {
			result = append(result, d.msgLog[start+i])
		}
	}

	return result
}
