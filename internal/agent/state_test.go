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
