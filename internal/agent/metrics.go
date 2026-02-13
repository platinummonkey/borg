package agent

import (
	"sync/atomic"
	"time"

	"github.com/platinummonkey/agent-chat/pkg/protocol"
)

// MetricsSnapshot holds a point-in-time snapshot of agent metrics.
type MetricsSnapshot struct {
	MessagesReceived     int64     `json:"messages_received"`
	MessagesSent         int64     `json:"messages_sent"`
	ProtocolMessagesIn   int64     `json:"protocol_messages_in"`
	ProtocolMessagesOut  int64     `json:"protocol_messages_out"`
	TasksStarted         int64     `json:"tasks_started"`
	TasksCompleted       int64     `json:"tasks_completed"`
	TasksBlocked         int64     `json:"tasks_blocked"`
	DependenciesResolved int64     `json:"dependencies_resolved"`
	ContextRequests      int64     `json:"context_requests"`
	ContextShared        int64     `json:"context_shared"`
	NotificationsSent    int64     `json:"notifications_sent"`
	HelpRequested        int64     `json:"help_requested"`
	CollectedAt          time.Time `json:"collected_at"`
}

// MetricsCollector tracks agent activity counters using lock-free atomic operations.
type MetricsCollector struct {
	messagesReceived     atomic.Int64
	messagesSent         atomic.Int64
	protocolMessagesIn   atomic.Int64
	protocolMessagesOut  atomic.Int64
	tasksStarted         atomic.Int64
	tasksCompleted       atomic.Int64
	tasksBlocked         atomic.Int64
	dependenciesResolved atomic.Int64
	contextRequests      atomic.Int64
	contextShared        atomic.Int64
	notificationsSent    atomic.Int64
	helpRequested        atomic.Int64
}

// NewMetricsCollector creates a new MetricsCollector.
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{}
}

// Snapshot returns a point-in-time snapshot of all metrics.
func (mc *MetricsCollector) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		MessagesReceived:     mc.messagesReceived.Load(),
		MessagesSent:         mc.messagesSent.Load(),
		ProtocolMessagesIn:   mc.protocolMessagesIn.Load(),
		ProtocolMessagesOut:  mc.protocolMessagesOut.Load(),
		TasksStarted:         mc.tasksStarted.Load(),
		TasksCompleted:       mc.tasksCompleted.Load(),
		TasksBlocked:         mc.tasksBlocked.Load(),
		DependenciesResolved: mc.dependenciesResolved.Load(),
		ContextRequests:      mc.contextRequests.Load(),
		ContextShared:        mc.contextShared.Load(),
		NotificationsSent:    mc.notificationsSent.Load(),
		HelpRequested:        mc.helpRequested.Load(),
		CollectedAt:          time.Now(),
	}
}

// HandleProtocolMessage inspects an incoming protocol message and increments
// the appropriate counters. Suitable for use as a ProtocolHandler.
func (mc *MetricsCollector) HandleProtocolMessage(msg *protocol.Message) {
	mc.protocolMessagesIn.Add(1)

	switch msg.Action {
	case protocol.ActionStarted:
		mc.tasksStarted.Add(1)
	case protocol.ActionCompleted:
		mc.tasksCompleted.Add(1)
	case protocol.ActionBlocked:
		mc.tasksBlocked.Add(1)
	case protocol.ActionAcknowledged:
		mc.dependenciesResolved.Add(1)
	case protocol.ActionRequestContext:
		mc.contextRequests.Add(1)
	case protocol.ActionContext, protocol.ActionSharingContext:
		mc.contextShared.Add(1)
	case protocol.ActionHelpNeeded:
		mc.helpRequested.Add(1)
	}
}

// RecordMessageSent increments the outgoing message counters.
func (mc *MetricsCollector) RecordMessageSent() {
	mc.messagesSent.Add(1)
	mc.protocolMessagesOut.Add(1)
}

// RecordRawMessageReceived increments the raw messages received counter.
func (mc *MetricsCollector) RecordRawMessageReceived() {
	mc.messagesReceived.Add(1)
}

// RecordNotificationSent increments the notifications sent counter.
func (mc *MetricsCollector) RecordNotificationSent() {
	mc.notificationsSent.Add(1)
}
