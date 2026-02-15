package agent

import (
	"testing"

	"github.com/platinummonkey/borg/pkg/protocol"
)

func TestMetricsCollector_Snapshot_Initial(t *testing.T) {
	mc := NewMetricsCollector()
	snap := mc.Snapshot()

	if snap.MessagesReceived != 0 {
		t.Errorf("MessagesReceived = %d, want 0", snap.MessagesReceived)
	}
	if snap.MessagesSent != 0 {
		t.Errorf("MessagesSent = %d, want 0", snap.MessagesSent)
	}
	if snap.CollectedAt.IsZero() {
		t.Error("CollectedAt should not be zero")
	}
}

func TestMetricsCollector_RecordRawMessageReceived(t *testing.T) {
	mc := NewMetricsCollector()
	mc.RecordRawMessageReceived()
	mc.RecordRawMessageReceived()
	mc.RecordRawMessageReceived()

	snap := mc.Snapshot()
	if snap.MessagesReceived != 3 {
		t.Errorf("MessagesReceived = %d, want 3", snap.MessagesReceived)
	}
}

func TestMetricsCollector_RecordMessageSent(t *testing.T) {
	mc := NewMetricsCollector()
	mc.RecordMessageSent()
	mc.RecordMessageSent()

	snap := mc.Snapshot()
	if snap.MessagesSent != 2 {
		t.Errorf("MessagesSent = %d, want 2", snap.MessagesSent)
	}
	if snap.ProtocolMessagesOut != 2 {
		t.Errorf("ProtocolMessagesOut = %d, want 2", snap.ProtocolMessagesOut)
	}
}

func TestMetricsCollector_HandleProtocolMessage(t *testing.T) {
	mc := NewMetricsCollector()

	tests := []struct {
		action protocol.Action
		field  string
	}{
		{protocol.ActionStarted, "TasksStarted"},
		{protocol.ActionCompleted, "TasksCompleted"},
		{protocol.ActionBlocked, "TasksBlocked"},
		{protocol.ActionAcknowledged, "DependenciesResolved"},
		{protocol.ActionRequestContext, "ContextRequests"},
		{protocol.ActionContext, "ContextShared"},
		{protocol.ActionSharingContext, "ContextShared"},
		{protocol.ActionHelpNeeded, "HelpRequested"},
	}

	for _, tt := range tests {
		mc.HandleProtocolMessage(&protocol.Message{Action: tt.action})
	}

	snap := mc.Snapshot()

	// Each action was sent once, except ContextShared which got 2 (Context + SharingContext).
	if snap.ProtocolMessagesIn != int64(len(tests)) {
		t.Errorf("ProtocolMessagesIn = %d, want %d", snap.ProtocolMessagesIn, len(tests))
	}
	if snap.TasksStarted != 1 {
		t.Errorf("TasksStarted = %d, want 1", snap.TasksStarted)
	}
	if snap.TasksCompleted != 1 {
		t.Errorf("TasksCompleted = %d, want 1", snap.TasksCompleted)
	}
	if snap.TasksBlocked != 1 {
		t.Errorf("TasksBlocked = %d, want 1", snap.TasksBlocked)
	}
	if snap.DependenciesResolved != 1 {
		t.Errorf("DependenciesResolved = %d, want 1", snap.DependenciesResolved)
	}
	if snap.ContextRequests != 1 {
		t.Errorf("ContextRequests = %d, want 1", snap.ContextRequests)
	}
	if snap.ContextShared != 2 {
		t.Errorf("ContextShared = %d, want 2", snap.ContextShared)
	}
	if snap.HelpRequested != 1 {
		t.Errorf("HelpRequested = %d, want 1", snap.HelpRequested)
	}
}

func TestMetricsCollector_RecordNotificationSent(t *testing.T) {
	mc := NewMetricsCollector()
	mc.RecordNotificationSent()

	snap := mc.Snapshot()
	if snap.NotificationsSent != 1 {
		t.Errorf("NotificationsSent = %d, want 1", snap.NotificationsSent)
	}
}
