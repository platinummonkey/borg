package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/platinummonkey/borg/pkg/protocol"
)

func TestUnblockNotifier_OnTaskCompleted(t *testing.T) {
	state := NewStateStore()
	client := &stubClient{nick: "self"}
	notifier := NewUnblockNotifier(state, client, client.Nick)

	// Set up: task-b blocked by task-a.
	state.UpdateTask(&protocol.Message{
		Action:    protocol.ActionBlocked,
		Fields:    map[string]string{"task": "task-b", "waiting-for": "task-a"},
		Nick:      "agent-2",
		Timestamp: time.Now(),
	})

	// Complete task-a.
	completedMsg := &protocol.Message{
		Action:    protocol.ActionCompleted,
		Fields:    map[string]string{"task": "task-a"},
		Nick:      "agent-1",
		Channel:   "#project",
		Timestamp: time.Now(),
	}
	state.UpdateTask(completedMsg)
	notifier.OnTaskCompleted(completedMsg)

	msgs := client.sentMessages()
	if len(msgs) != 1 {
		t.Fatalf("sent %d messages, want 1", len(msgs))
	}
	if msgs[0].target != "#project" {
		t.Errorf("target = %q, want %q", msgs[0].target, "#project")
	}
	if !strings.Contains(msgs[0].message, "task=task-b") {
		t.Errorf("message missing task=task-b: %q", msgs[0].message)
	}
	if !strings.Contains(msgs[0].message, "unblocked-by=task-a") {
		t.Errorf("message missing unblocked-by=task-a: %q", msgs[0].message)
	}
	if !strings.Contains(msgs[0].message, "#auto-unblock") {
		t.Errorf("message missing #auto-unblock tag: %q", msgs[0].message)
	}
}

func TestUnblockNotifier_NoDuplicate(t *testing.T) {
	state := NewStateStore()
	client := &stubClient{nick: "self"}
	notifier := NewUnblockNotifier(state, client, client.Nick)

	// task-b blocked by task-a.
	state.UpdateTask(&protocol.Message{
		Action:    protocol.ActionBlocked,
		Fields:    map[string]string{"task": "task-b", "waiting-for": "task-a"},
		Nick:      "agent-2",
		Timestamp: time.Now(),
	})

	completedMsg := &protocol.Message{
		Action:    protocol.ActionCompleted,
		Fields:    map[string]string{"task": "task-a"},
		Nick:      "agent-1",
		Channel:   "#project",
		Timestamp: time.Now(),
	}
	state.UpdateTask(completedMsg)

	// Call twice.
	notifier.OnTaskCompleted(completedMsg)
	notifier.OnTaskCompleted(completedMsg)

	msgs := client.sentMessages()
	if len(msgs) != 1 {
		t.Errorf("sent %d messages, want 1 (no duplicates)", len(msgs))
	}
}

func TestUnblockNotifier_NoUnblockedTasks(t *testing.T) {
	state := NewStateStore()
	client := &stubClient{nick: "self"}
	notifier := NewUnblockNotifier(state, client, client.Nick)

	// Complete a task with no dependents.
	completedMsg := &protocol.Message{
		Action:    protocol.ActionCompleted,
		Fields:    map[string]string{"task": "standalone"},
		Nick:      "agent-1",
		Channel:   "#project",
		Timestamp: time.Now(),
	}
	state.UpdateTask(completedMsg)
	notifier.OnTaskCompleted(completedMsg)

	msgs := client.sentMessages()
	if len(msgs) != 0 {
		t.Errorf("sent %d messages, want 0", len(msgs))
	}
}

func TestUnblockNotifier_NonCompletedIgnored(t *testing.T) {
	state := NewStateStore()
	client := &stubClient{nick: "self"}
	notifier := NewUnblockNotifier(state, client, client.Nick)

	notifier.OnTaskCompleted(&protocol.Message{
		Action:    protocol.ActionStarted,
		Fields:    map[string]string{"task": "x"},
		Nick:      "agent-1",
		Timestamp: time.Now(),
	})

	msgs := client.sentMessages()
	if len(msgs) != 0 {
		t.Errorf("sent %d messages, want 0 (non-COMPLETED ignored)", len(msgs))
	}
}

func TestUnblockNotifier_Reset(t *testing.T) {
	state := NewStateStore()
	client := &stubClient{nick: "self"}
	notifier := NewUnblockNotifier(state, client, client.Nick)

	// Set up and trigger.
	state.UpdateTask(&protocol.Message{
		Action:    protocol.ActionBlocked,
		Fields:    map[string]string{"task": "task-b", "waiting-for": "task-a"},
		Nick:      "agent-2",
		Timestamp: time.Now(),
	})
	completedMsg := &protocol.Message{
		Action:    protocol.ActionCompleted,
		Fields:    map[string]string{"task": "task-a"},
		Nick:      "agent-1",
		Channel:   "#project",
		Timestamp: time.Now(),
	}
	state.UpdateTask(completedMsg)
	notifier.OnTaskCompleted(completedMsg)

	notified := notifier.NotifiedTasks()
	if len(notified) != 1 {
		t.Fatalf("NotifiedTasks = %d, want 1", len(notified))
	}

	notifier.Reset()
	notified = notifier.NotifiedTasks()
	if len(notified) != 0 {
		t.Errorf("NotifiedTasks after Reset = %d, want 0", len(notified))
	}
}

func TestUnblockNotifier_MultipleUnblocked(t *testing.T) {
	state := NewStateStore()
	client := &stubClient{nick: "self"}
	notifier := NewUnblockNotifier(state, client, client.Nick)

	// Both task-b and task-c blocked by task-a.
	state.UpdateTask(&protocol.Message{
		Action:    protocol.ActionBlocked,
		Fields:    map[string]string{"task": "task-b", "waiting-for": "task-a"},
		Nick:      "agent-2",
		Timestamp: time.Now(),
	})
	state.UpdateTask(&protocol.Message{
		Action:    protocol.ActionBlocked,
		Fields:    map[string]string{"task": "task-c", "waiting-for": "task-a"},
		Nick:      "agent-3",
		Timestamp: time.Now(),
	})

	completedMsg := &protocol.Message{
		Action:    protocol.ActionCompleted,
		Fields:    map[string]string{"task": "task-a"},
		Nick:      "agent-1",
		Channel:   "#project",
		Timestamp: time.Now(),
	}
	state.UpdateTask(completedMsg)
	notifier.OnTaskCompleted(completedMsg)

	msgs := client.sentMessages()
	if len(msgs) != 2 {
		t.Fatalf("sent %d messages, want 2", len(msgs))
	}
}
