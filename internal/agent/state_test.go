package agent

import (
	"testing"
	"time"

	"github.com/platinummonkey/agent-chat/pkg/protocol"
)

func TestStateStore_UpdateTask_Started(t *testing.T) {
	s := NewStateStore()

	msg := &protocol.Message{
		Action:    protocol.ActionStarted,
		Fields:    map[string]string{"task": "implement-login", "priority": "high"},
		Tags:      []string{"feature"},
		Nick:      "agent-1",
		Timestamp: time.Now(),
	}
	s.UpdateTask(msg)

	info := s.GetTask("implement-login")
	if info == nil {
		t.Fatal("GetTask returned nil")
	}
	if info.Status != TaskStatusStarted {
		t.Errorf("Status = %q, want %q", info.Status, TaskStatusStarted)
	}
	if info.Priority != "high" {
		t.Errorf("Priority = %q, want %q", info.Priority, "high")
	}
	if info.LastAgent != "agent-1" {
		t.Errorf("LastAgent = %q, want %q", info.LastAgent, "agent-1")
	}
}

func TestStateStore_UpdateTask_Completed(t *testing.T) {
	s := NewStateStore()

	// Start then complete.
	s.UpdateTask(&protocol.Message{
		Action:    protocol.ActionStarted,
		Fields:    map[string]string{"task": "db-migration"},
		Nick:      "agent-1",
		Timestamp: time.Now(),
	})
	s.UpdateTask(&protocol.Message{
		Action:    protocol.ActionCompleted,
		Fields:    map[string]string{"task": "db-migration"},
		Tags:      []string{"unblocks-others"},
		Nick:      "agent-1",
		Timestamp: time.Now(),
	})

	info := s.GetTask("db-migration")
	if info == nil {
		t.Fatal("GetTask returned nil")
	}
	if info.Status != TaskStatusCompleted {
		t.Errorf("Status = %q, want %q", info.Status, TaskStatusCompleted)
	}
}

func TestStateStore_UpdateTask_Blocked(t *testing.T) {
	s := NewStateStore()

	msg := &protocol.Message{
		Action:    protocol.ActionBlocked,
		Fields:    map[string]string{"task": "payment", "waiting-for": "api-keys"},
		Tags:      []string{"blocked-by-auth"},
		Nick:      "agent-2",
		Timestamp: time.Now(),
	}
	s.UpdateTask(msg)

	info := s.GetTask("payment")
	if info == nil {
		t.Fatal("GetTask returned nil")
	}
	if info.Status != TaskStatusBlocked {
		t.Errorf("Status = %q, want %q", info.Status, TaskStatusBlocked)
	}
	if info.WaitingFor != "api-keys" {
		t.Errorf("WaitingFor = %q, want %q", info.WaitingFor, "api-keys")
	}
}

func TestStateStore_EmptyTask(t *testing.T) {
	s := NewStateStore()

	// Message with no task field should be ignored.
	s.UpdateTask(&protocol.Message{
		Action:    protocol.ActionStarted,
		Fields:    map[string]string{"priority": "high"},
		Timestamp: time.Now(),
	})

	tasks := s.ListTasks()
	if len(tasks) != 0 {
		t.Errorf("ListTasks = %d tasks, want 0", len(tasks))
	}
}

func TestStateStore_GetTask_Unknown(t *testing.T) {
	s := NewStateStore()
	if info := s.GetTask("nonexistent"); info != nil {
		t.Errorf("GetTask(nonexistent) = %v, want nil", info)
	}
}

func TestStateStore_ListTasks(t *testing.T) {
	s := NewStateStore()

	for _, name := range []string{"task-a", "task-b", "task-c"} {
		s.UpdateTask(&protocol.Message{
			Action:    protocol.ActionStarted,
			Fields:    map[string]string{"task": name},
			Timestamp: time.Now(),
		})
	}

	tasks := s.ListTasks()
	if len(tasks) != 3 {
		t.Errorf("ListTasks = %d tasks, want 3", len(tasks))
	}
}

func TestStateStore_TasksByStatus(t *testing.T) {
	s := NewStateStore()

	s.UpdateTask(&protocol.Message{
		Action: protocol.ActionStarted, Fields: map[string]string{"task": "a"}, Timestamp: time.Now(),
	})
	s.UpdateTask(&protocol.Message{
		Action: protocol.ActionCompleted, Fields: map[string]string{"task": "b"}, Timestamp: time.Now(),
	})
	s.UpdateTask(&protocol.Message{
		Action: protocol.ActionStarted, Fields: map[string]string{"task": "c"}, Timestamp: time.Now(),
	})

	started := s.TasksByStatus(TaskStatusStarted)
	if len(started) != 2 {
		t.Errorf("TasksByStatus(started) = %d, want 2", len(started))
	}

	completed := s.TasksByStatus(TaskStatusCompleted)
	if len(completed) != 1 {
		t.Errorf("TasksByStatus(completed) = %d, want 1", len(completed))
	}
}

func TestStateStore_Dependencies(t *testing.T) {
	s := NewStateStore()

	// Task B is blocked by task A.
	s.UpdateTask(&protocol.Message{
		Action:    protocol.ActionBlocked,
		Fields:    map[string]string{"task": "task-b", "waiting-for": "task-a"},
		Tags:      []string{"blocked-by-task-a"},
		Timestamp: time.Now(),
	})

	// task-b should not yet be unblocked.
	unblocked := s.UnblockedTasks()
	if len(unblocked) != 0 {
		t.Errorf("UnblockedTasks = %d, want 0", len(unblocked))
	}

	// Complete task A.
	s.UpdateTask(&protocol.Message{
		Action:    protocol.ActionCompleted,
		Fields:    map[string]string{"task": "task-a"},
		Timestamp: time.Now(),
	})

	// Now task-b should be unblocked.
	unblocked = s.UnblockedTasks()
	if len(unblocked) != 1 {
		t.Fatalf("UnblockedTasks = %d, want 1", len(unblocked))
	}
	if unblocked[0].Name != "task-b" {
		t.Errorf("Unblocked task = %q, want %q", unblocked[0].Name, "task-b")
	}

	// Resolved deps should be reported.
	resolved := s.ResolvedDependencies()
	if len(resolved) == 0 {
		t.Fatal("ResolvedDependencies = 0, want > 0")
	}
}

func TestStateStore_AddDependency(t *testing.T) {
	s := NewStateStore()

	edge := DependencyEdge{Blocked: "b", BlockedBy: "a"}
	s.AddDependency(edge)
	// Adding duplicate should not create a second entry.
	s.AddDependency(edge)

	// Create the blocked task.
	s.UpdateTask(&protocol.Message{
		Action: protocol.ActionBlocked, Fields: map[string]string{"task": "b"}, Timestamp: time.Now(),
	})

	unblocked := s.UnblockedTasks()
	if len(unblocked) != 0 {
		t.Errorf("UnblockedTasks before resolve = %d, want 0", len(unblocked))
	}
}

func TestStateStore_AgentStatus(t *testing.T) {
	s := NewStateStore()

	s.UpdateAgentStatus("agent-1", "#project", "login-task")

	status := s.GetAgentStatus("agent-1")
	if status == nil {
		t.Fatal("GetAgentStatus returned nil")
	}
	if status.Nick != "agent-1" {
		t.Errorf("Nick = %q, want %q", status.Nick, "agent-1")
	}
	if status.Channel != "#project" {
		t.Errorf("Channel = %q, want %q", status.Channel, "#project")
	}
	if status.TaskName != "login-task" {
		t.Errorf("TaskName = %q, want %q", status.TaskName, "login-task")
	}

	// Unknown agent.
	if s.GetAgentStatus("unknown") != nil {
		t.Error("GetAgentStatus(unknown) != nil")
	}
}

func TestStateStore_BlockersOf(t *testing.T) {
	s := NewStateStore()
	s.AddDependency(DependencyEdge{Blocked: "task-c", BlockedBy: "task-a"})
	s.AddDependency(DependencyEdge{Blocked: "task-c", BlockedBy: "task-b"})

	blockers := s.BlockersOf("task-c", false)
	if len(blockers) != 2 {
		t.Fatalf("BlockersOf(task-c) = %d, want 2", len(blockers))
	}

	// Resolve one.
	s.UpdateTask(&protocol.Message{
		Action: protocol.ActionCompleted, Fields: map[string]string{"task": "task-a"}, Timestamp: time.Now(),
	})
	unresolved := s.BlockersOf("task-c", false)
	if len(unresolved) != 1 {
		t.Errorf("BlockersOf(task-c, unresolved) = %d, want 1", len(unresolved))
	}
	all := s.BlockersOf("task-c", true)
	if len(all) != 2 {
		t.Errorf("BlockersOf(task-c, all) = %d, want 2", len(all))
	}
}

func TestStateStore_BlockedBy(t *testing.T) {
	s := NewStateStore()
	s.AddDependency(DependencyEdge{Blocked: "task-b", BlockedBy: "task-a"})
	s.AddDependency(DependencyEdge{Blocked: "task-c", BlockedBy: "task-a"})

	blocking := s.BlockedBy("task-a", false)
	if len(blocking) != 2 {
		t.Fatalf("BlockedBy(task-a) = %d, want 2", len(blocking))
	}
}

func TestStateStore_HasCycle_Direct(t *testing.T) {
	s := NewStateStore()
	// A→B already exists, B→A would create a cycle.
	s.AddDependency(DependencyEdge{Blocked: "a", BlockedBy: "b"})

	if !s.HasCycle("b", "a") {
		t.Error("HasCycle(b, a) = false, want true")
	}
	// Non-cyclic should pass.
	if s.HasCycle("c", "a") {
		t.Error("HasCycle(c, a) = true, want false")
	}
}

func TestStateStore_HasCycle_Transitive(t *testing.T) {
	s := NewStateStore()
	// Chain: a→b→c. Adding c→a would cycle.
	s.AddDependency(DependencyEdge{Blocked: "a", BlockedBy: "b"})
	s.AddDependency(DependencyEdge{Blocked: "b", BlockedBy: "c"})

	if !s.HasCycle("c", "a") {
		t.Error("HasCycle(c, a) = false, want true (transitive)")
	}
}

func TestStateStore_HasCycle_SelfLoop(t *testing.T) {
	s := NewStateStore()
	if !s.HasCycle("x", "x") {
		t.Error("HasCycle(x, x) = false, want true (self-loop)")
	}
}

func TestStateStore_AddDependency_RejectsCycle(t *testing.T) {
	s := NewStateStore()
	s.AddDependency(DependencyEdge{Blocked: "a", BlockedBy: "b"})
	// Attempt to add b→a (cycle).
	s.AddDependency(DependencyEdge{Blocked: "b", BlockedBy: "a"})

	all := s.AllDependencies()
	if len(all) != 1 {
		t.Errorf("AllDependencies = %d, want 1 (cycle should be rejected)", len(all))
	}
}

func TestStateStore_AllDependencies(t *testing.T) {
	s := NewStateStore()
	s.AddDependency(DependencyEdge{Blocked: "x", BlockedBy: "y"})
	s.AddDependency(DependencyEdge{Blocked: "y", BlockedBy: "z"})

	all := s.AllDependencies()
	if len(all) != 2 {
		t.Errorf("AllDependencies = %d, want 2", len(all))
	}
}

func TestStateStore_DependencyStats(t *testing.T) {
	s := NewStateStore()
	// Create: task-b blocked by task-a, task-c blocked by task-a.
	s.UpdateTask(&protocol.Message{
		Action: protocol.ActionBlocked, Fields: map[string]string{"task": "task-b", "waiting-for": "task-a"}, Timestamp: time.Now(),
	})
	s.UpdateTask(&protocol.Message{
		Action: protocol.ActionBlocked, Fields: map[string]string{"task": "task-c", "waiting-for": "task-a"}, Timestamp: time.Now(),
	})

	stats := s.DependencyStats()
	if stats.TotalEdges != 2 {
		t.Errorf("TotalEdges = %d, want 2", stats.TotalEdges)
	}
	if stats.UnresolvedEdges != 2 {
		t.Errorf("UnresolvedEdges = %d, want 2", stats.UnresolvedEdges)
	}
	if stats.BlockedTasks != 2 {
		t.Errorf("BlockedTasks = %d, want 2", stats.BlockedTasks)
	}

	// Complete task-a.
	s.UpdateTask(&protocol.Message{
		Action: protocol.ActionCompleted, Fields: map[string]string{"task": "task-a"}, Timestamp: time.Now(),
	})

	stats = s.DependencyStats()
	if stats.ResolvedEdges != 2 {
		t.Errorf("ResolvedEdges = %d, want 2", stats.ResolvedEdges)
	}
	if stats.UnblockedTasks != 2 {
		t.Errorf("UnblockedTasks = %d, want 2", stats.UnblockedTasks)
	}
}

func TestStateStore_TransitiveDependencies(t *testing.T) {
	s := NewStateStore()
	// Chain: a→b→c→d
	s.AddDependency(DependencyEdge{Blocked: "a", BlockedBy: "b"})
	s.AddDependency(DependencyEdge{Blocked: "b", BlockedBy: "c"})
	s.AddDependency(DependencyEdge{Blocked: "c", BlockedBy: "d"})

	deps := s.TransitiveDependencies("a")
	if len(deps) != 3 {
		t.Fatalf("TransitiveDependencies(a) = %v, want 3 items", deps)
	}

	// Verify all expected deps are present.
	found := make(map[string]bool)
	for _, d := range deps {
		found[d] = true
	}
	for _, want := range []string{"b", "c", "d"} {
		if !found[want] {
			t.Errorf("TransitiveDependencies(a) missing %q", want)
		}
	}
}

func TestStateStore_TransitiveDependencies_SkipsResolved(t *testing.T) {
	s := NewStateStore()
	// a→b→c, but resolve b→c.
	s.AddDependency(DependencyEdge{Blocked: "a", BlockedBy: "b"})
	s.AddDependency(DependencyEdge{Blocked: "b", BlockedBy: "c"})
	s.UpdateTask(&protocol.Message{
		Action: protocol.ActionCompleted, Fields: map[string]string{"task": "c"}, Timestamp: time.Now(),
	})

	deps := s.TransitiveDependencies("a")
	// b is still unresolved blocker of a, but c's edge to b is resolved.
	if len(deps) != 1 {
		t.Fatalf("TransitiveDependencies(a) = %v, want [b]", deps)
	}
	if deps[0] != "b" {
		t.Errorf("TransitiveDependencies(a) = %v, want [b]", deps)
	}
}

func TestStateStore_GetTask_ReturnsCopy(t *testing.T) {
	s := NewStateStore()
	s.UpdateTask(&protocol.Message{
		Action:    protocol.ActionStarted,
		Fields:    map[string]string{"task": "x"},
		Tags:      []string{"original"},
		Timestamp: time.Now(),
	})

	info := s.GetTask("x")
	info.Tags = append(info.Tags, "modified")

	// Original should not be modified.
	info2 := s.GetTask("x")
	if len(info2.Tags) != 1 {
		t.Errorf("GetTask returned mutable reference: tags = %v", info2.Tags)
	}
}
