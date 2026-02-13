package agent

import (
	"testing"
	"time"

	"github.com/platinummonkey/agent-chat/pkg/protocol"
)

func TestHealthMonitor_Check_Initial(t *testing.T) {
	client := &stubClient{nick: "test-agent"}
	state := NewStateStore()
	hm := NewHealthMonitor(client, state)

	status := hm.Check()

	if !status.Connected {
		t.Error("Connected should be true for stubClient")
	}
	if !status.Healthy {
		t.Error("Healthy should be true for stubClient")
	}
	if status.Nick != "test-agent" {
		t.Errorf("Nick = %q, want test-agent", status.Nick)
	}
	if status.TaskStats.Total != 0 {
		t.Errorf("TaskStats.Total = %d, want 0", status.TaskStats.Total)
	}
	if status.StartedAt.IsZero() {
		t.Error("StartedAt should not be zero")
	}
	if status.Uptime <= 0 {
		t.Error("Uptime should be positive")
	}
	if status.UptimeHuman == "" {
		t.Error("UptimeHuman should not be empty")
	}
}

func TestHealthMonitor_Check_WithTasks(t *testing.T) {
	client := &stubClient{nick: "test-agent"}
	state := NewStateStore()
	hm := NewHealthMonitor(client, state)

	state.UpdateTask(&protocol.Message{
		Action: protocol.ActionStarted, Fields: map[string]string{"task": "a"}, Timestamp: time.Now(),
	})
	state.UpdateTask(&protocol.Message{
		Action: protocol.ActionCompleted, Fields: map[string]string{"task": "b"}, Timestamp: time.Now(),
	})
	state.UpdateTask(&protocol.Message{
		Action: protocol.ActionBlocked, Fields: map[string]string{"task": "c", "waiting-for": "d"}, Timestamp: time.Now(),
	})

	status := hm.Check()
	if status.TaskStats.Total != 3 {
		t.Errorf("TaskStats.Total = %d, want 3", status.TaskStats.Total)
	}
	if status.TaskStats.Started != 1 {
		t.Errorf("TaskStats.Started = %d, want 1", status.TaskStats.Started)
	}
	if status.TaskStats.Completed != 1 {
		t.Errorf("TaskStats.Completed = %d, want 1", status.TaskStats.Completed)
	}
	if status.TaskStats.Blocked != 1 {
		t.Errorf("TaskStats.Blocked = %d, want 1", status.TaskStats.Blocked)
	}
}

func TestHealthMonitor_Check_DependencyStats(t *testing.T) {
	client := &stubClient{nick: "test-agent"}
	state := NewStateStore()
	hm := NewHealthMonitor(client, state)

	state.UpdateTask(&protocol.Message{
		Action: protocol.ActionBlocked, Fields: map[string]string{"task": "b", "waiting-for": "a"}, Timestamp: time.Now(),
	})

	status := hm.Check()
	if status.DependencyStats.TotalEdges != 1 {
		t.Errorf("DependencyStats.TotalEdges = %d, want 1", status.DependencyStats.TotalEdges)
	}
	if status.DependencyStats.UnresolvedEdges != 1 {
		t.Errorf("DependencyStats.UnresolvedEdges = %d, want 1", status.DependencyStats.UnresolvedEdges)
	}
}

func TestHealthMonitor_Check_ChannelsEmpty(t *testing.T) {
	client := &stubClient{nick: "test-agent"}
	state := NewStateStore()
	hm := NewHealthMonitor(client, state)

	status := hm.Check()
	if status.Channels == nil {
		t.Error("Channels should not be nil (should be empty slice)")
	}
	if len(status.Channels) != 0 {
		t.Errorf("Channels = %v, want empty", status.Channels)
	}
}
