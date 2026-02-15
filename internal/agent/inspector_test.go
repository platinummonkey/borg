package agent

import (
	"testing"
	"time"

	"github.com/platinummonkey/borg/pkg/protocol"
)

func TestDebugInspector_TaskGraph(t *testing.T) {
	state := NewStateStore()
	ctx := NewContextStore()
	inspector := NewDebugInspector(state, ctx, 100)

	state.UpdateTask(&protocol.Message{
		Action: protocol.ActionStarted, Fields: map[string]string{"task": "a"}, Timestamp: time.Now(),
	})
	state.UpdateTask(&protocol.Message{
		Action: protocol.ActionBlocked, Fields: map[string]string{"task": "b", "waiting-for": "a"}, Timestamp: time.Now(),
	})

	graph := inspector.TaskGraph()
	if len(graph) != 2 {
		t.Fatalf("TaskGraph = %d nodes, want 2", len(graph))
	}

	// Find task "a" - it should have a "blocking" edge.
	for _, node := range graph {
		if node.Task.Name == "a" {
			if len(node.Blocking) != 1 {
				t.Errorf("task a Blocking = %d, want 1", len(node.Blocking))
			}
		}
		if node.Task.Name == "b" {
			if len(node.BlockedBy) != 1 {
				t.Errorf("task b BlockedBy = %d, want 1", len(node.BlockedBy))
			}
		}
	}
}

func TestDebugInspector_TaskGraph_Empty(t *testing.T) {
	state := NewStateStore()
	ctx := NewContextStore()
	inspector := NewDebugInspector(state, ctx, 100)

	graph := inspector.TaskGraph()
	if len(graph) != 0 {
		t.Errorf("TaskGraph = %d, want 0 for empty state", len(graph))
	}
}

func TestDebugInspector_BlockedChain(t *testing.T) {
	state := NewStateStore()
	ctx := NewContextStore()
	inspector := NewDebugInspector(state, ctx, 100)

	// Chain: c depends on b, b depends on a.
	state.UpdateTask(&protocol.Message{
		Action: protocol.ActionStarted, Fields: map[string]string{"task": "a"}, Timestamp: time.Now(),
	})
	state.AddDependency(DependencyEdge{Blocked: "b", BlockedBy: "a"})
	state.AddDependency(DependencyEdge{Blocked: "c", BlockedBy: "b"})

	chain := inspector.BlockedChain("c")
	if len(chain) != 2 {
		t.Fatalf("BlockedChain(c) = %d entries, want 2", len(chain))
	}

	found := make(map[string]bool)
	for _, entry := range chain {
		found[entry.TaskName] = true
	}
	if !found["b"] {
		t.Error("BlockedChain missing 'b'")
	}
	if !found["a"] {
		t.Error("BlockedChain missing 'a'")
	}
}

func TestDebugInspector_AgentActivity(t *testing.T) {
	state := NewStateStore()
	ctx := NewContextStore()
	inspector := NewDebugInspector(state, ctx, 100)

	state.UpdateAgentStatus("agent-1", "#project", "task-a")
	state.UpdateAgentStatus("agent-2", "#dev", "task-b")

	activity := inspector.AgentActivity()
	if len(activity) != 2 {
		t.Fatalf("AgentActivity = %d, want 2", len(activity))
	}
}

func TestDebugInspector_RecordMessage_And_RecentMessages(t *testing.T) {
	state := NewStateStore()
	ctx := NewContextStore()
	inspector := NewDebugInspector(state, ctx, 5)

	for i := 0; i < 3; i++ {
		inspector.RecordMessage(MessageLogEntry{
			Timestamp: time.Now(),
			Direction: "in",
			Channel:   "#test",
			Nick:      "agent-1",
			Action:    "STARTED",
			Raw:       "STARTED task=test",
		})
	}

	msgs := inspector.RecentMessages(10)
	if len(msgs) != 3 {
		t.Errorf("RecentMessages(10) = %d, want 3", len(msgs))
	}

	msgs = inspector.RecentMessages(2)
	if len(msgs) != 2 {
		t.Errorf("RecentMessages(2) = %d, want 2", len(msgs))
	}
}

func TestDebugInspector_RingBuffer_Wraps(t *testing.T) {
	state := NewStateStore()
	ctx := NewContextStore()
	inspector := NewDebugInspector(state, ctx, 3)

	// Write 5 entries into a buffer of size 3.
	for i := 0; i < 5; i++ {
		inspector.RecordMessage(MessageLogEntry{
			Timestamp: time.Now(),
			Direction: "in",
			Action:    string(rune('A' + i)),
		})
	}

	msgs := inspector.RecentMessages(10)
	if len(msgs) != 3 {
		t.Fatalf("RecentMessages after wrap = %d, want 3", len(msgs))
	}

	// Should contain the last 3 entries: C, D, E (indices 2, 3, 4).
	if msgs[0].Action != "C" {
		t.Errorf("msgs[0].Action = %q, want C", msgs[0].Action)
	}
	if msgs[1].Action != "D" {
		t.Errorf("msgs[1].Action = %q, want D", msgs[1].Action)
	}
	if msgs[2].Action != "E" {
		t.Errorf("msgs[2].Action = %q, want E", msgs[2].Action)
	}
}

func TestDebugInspector_RecentMessages_Empty(t *testing.T) {
	state := NewStateStore()
	ctx := NewContextStore()
	inspector := NewDebugInspector(state, ctx, 100)

	msgs := inspector.RecentMessages(10)
	if msgs == nil {
		t.Error("RecentMessages should return empty slice, not nil")
	}
	if len(msgs) != 0 {
		t.Errorf("RecentMessages = %d, want 0", len(msgs))
	}
}
