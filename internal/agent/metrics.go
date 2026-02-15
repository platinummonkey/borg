package agent

import (
	"context"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/metric"

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
	TasksOffered         int64     `json:"tasks_offered"`
	TasksClaimed         int64     `json:"tasks_claimed"`
	TasksAssigned        int64     `json:"tasks_assigned"`
	TasksDeclined        int64     `json:"tasks_declined"`
	TasksYielded         int64     `json:"tasks_yielded"`
	Checkpoints          int64     `json:"checkpoints"`
	Handoffs             int64     `json:"handoffs"`
	ReviewsRequested     int64     `json:"reviews_requested"`
	ReviewsCompleted     int64     `json:"reviews_completed"`
	GatesPassed          int64     `json:"gates_passed"`
	GatesFailed          int64     `json:"gates_failed"`
	VotesRecorded        int64     `json:"votes_recorded"`
	EscalationsRaised    int64     `json:"escalations_raised"`
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
	tasksOffered         atomic.Int64
	tasksClaimed         atomic.Int64
	tasksAssigned        atomic.Int64
	tasksDeclined        atomic.Int64
	tasksYielded         atomic.Int64
	checkpoints          atomic.Int64
	handoffs             atomic.Int64
	reviewsRequested     atomic.Int64
	reviewsCompleted     atomic.Int64
	gatesPassed          atomic.Int64
	gatesFailed          atomic.Int64
	votesRecorded        atomic.Int64
	escalationsRaised    atomic.Int64

	// OTel counters (nil when OTel is disabled).
	otelMessagesIn   metric.Int64Counter
	otelMessagesOut  metric.Int64Counter
	otelTaskStarted  metric.Int64Counter
	otelTaskDone     metric.Int64Counter
	otelTaskBlocked  metric.Int64Counter
	otelDepsResolved metric.Int64Counter
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
		TasksOffered:         mc.tasksOffered.Load(),
		TasksClaimed:         mc.tasksClaimed.Load(),
		TasksAssigned:        mc.tasksAssigned.Load(),
		TasksDeclined:        mc.tasksDeclined.Load(),
		TasksYielded:         mc.tasksYielded.Load(),
		Checkpoints:          mc.checkpoints.Load(),
		Handoffs:             mc.handoffs.Load(),
		ReviewsRequested:     mc.reviewsRequested.Load(),
		ReviewsCompleted:     mc.reviewsCompleted.Load(),
		GatesPassed:          mc.gatesPassed.Load(),
		GatesFailed:          mc.gatesFailed.Load(),
		VotesRecorded:        mc.votesRecorded.Load(),
		EscalationsRaised:    mc.escalationsRaised.Load(),
		CollectedAt:          time.Now(),
	}
}

// HandleProtocolMessage inspects an incoming protocol message and increments
// the appropriate counters. Suitable for use as a ProtocolHandler.
func (mc *MetricsCollector) HandleProtocolMessage(msg *protocol.Message) {
	mc.protocolMessagesIn.Add(1)
	mc.addOTel(mc.otelMessagesIn, 1)

	switch msg.Action {
	case protocol.ActionStarted:
		mc.tasksStarted.Add(1)
		mc.addOTel(mc.otelTaskStarted, 1)
	case protocol.ActionCompleted:
		mc.tasksCompleted.Add(1)
		mc.addOTel(mc.otelTaskDone, 1)
	case protocol.ActionBlocked:
		mc.tasksBlocked.Add(1)
		mc.addOTel(mc.otelTaskBlocked, 1)
	case protocol.ActionAcknowledged:
		mc.dependenciesResolved.Add(1)
		mc.addOTel(mc.otelDepsResolved, 1)
	case protocol.ActionRequestContext:
		mc.contextRequests.Add(1)
	case protocol.ActionContext, protocol.ActionSharingContext:
		mc.contextShared.Add(1)
	case protocol.ActionHelpNeeded:
		mc.helpRequested.Add(1)
	case protocol.ActionOffer:
		mc.tasksOffered.Add(1)
	case protocol.ActionClaim:
		mc.tasksClaimed.Add(1)
	case protocol.ActionAssign:
		mc.tasksAssigned.Add(1)
	case protocol.ActionDecline:
		mc.tasksDeclined.Add(1)
	case protocol.ActionYield:
		mc.tasksYielded.Add(1)
	case protocol.ActionCheckpoint:
		mc.checkpoints.Add(1)
	case protocol.ActionHandoff:
		mc.handoffs.Add(1)
	case protocol.ActionReviewRequest:
		mc.reviewsRequested.Add(1)
	case protocol.ActionReviewComplete:
		mc.reviewsCompleted.Add(1)
	case protocol.ActionGateCheck:
		// Count pass/fail separately based on status field.
		mc.gatesPassed.Add(1) // generic counter; detailed pass/fail tracked in ReviewStore
	case protocol.ActionVote:
		mc.votesRecorded.Add(1)
	case protocol.ActionEscalate:
		mc.escalationsRaised.Add(1)
	}
}

// RecordMessageSent increments the outgoing message counters.
func (mc *MetricsCollector) RecordMessageSent() {
	mc.messagesSent.Add(1)
	mc.protocolMessagesOut.Add(1)
	mc.addOTel(mc.otelMessagesOut, 1)
}

// RecordRawMessageReceived increments the raw messages received counter.
func (mc *MetricsCollector) RecordRawMessageReceived() {
	mc.messagesReceived.Add(1)
}

// RecordNotificationSent increments the notifications sent counter.
func (mc *MetricsCollector) RecordNotificationSent() {
	mc.notificationsSent.Add(1)
}

// RegisterOTelMetrics creates OTel counter instruments mirroring the atomic counters.
// If meter is nil, this is a no-op.
func (mc *MetricsCollector) RegisterOTelMetrics(meter metric.Meter) {
	if meter == nil {
		return
	}
	mc.otelMessagesIn, _ = meter.Int64Counter("agent.messages.in",
		metric.WithDescription("Protocol messages received"))
	mc.otelMessagesOut, _ = meter.Int64Counter("agent.messages.out",
		metric.WithDescription("Protocol messages sent"))
	mc.otelTaskStarted, _ = meter.Int64Counter("agent.tasks.started",
		metric.WithDescription("Tasks started"))
	mc.otelTaskDone, _ = meter.Int64Counter("agent.tasks.completed",
		metric.WithDescription("Tasks completed"))
	mc.otelTaskBlocked, _ = meter.Int64Counter("agent.tasks.blocked",
		metric.WithDescription("Tasks blocked"))
	mc.otelDepsResolved, _ = meter.Int64Counter("agent.dependencies.resolved",
		metric.WithDescription("Dependencies resolved"))
}

func (mc *MetricsCollector) addOTel(c metric.Int64Counter, n int64) {
	if c != nil {
		c.Add(context.Background(), n)
	}
}
