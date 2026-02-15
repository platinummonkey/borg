package agent

import (
	"fmt"
	"sync"

	"github.com/platinummonkey/borg/pkg/ircclient"
	"github.com/platinummonkey/borg/pkg/protocol"
)

// NotificationEvent identifies the type of event that triggers a notification.
type NotificationEvent string

const (
	NotifyTaskCompleted    NotificationEvent = "task_completed"
	NotifyTaskBlocked      NotificationEvent = "task_blocked"
	NotifyTaskStarted      NotificationEvent = "task_started"
	NotifyHelpNeeded       NotificationEvent = "help_needed"
	NotifyContextUpdate    NotificationEvent = "context_update"
	NotifyUnblocked        NotificationEvent = "unblocked"
	NotifyTaskOffered      NotificationEvent = "task_offered"
	NotifyTaskClaimed      NotificationEvent = "task_claimed"
	NotifyTaskAssigned     NotificationEvent = "task_assigned"
	NotifyTaskYielded      NotificationEvent = "task_yielded"
	NotifyCheckpoint       NotificationEvent = "checkpoint"
	NotifyHandoff          NotificationEvent = "handoff"
	NotifyReviewRequested  NotificationEvent = "review_requested"
	NotifyReviewCompleted  NotificationEvent = "review_completed"
	NotifyGatePassed       NotificationEvent = "gate_passed"
	NotifyGateFailed       NotificationEvent = "gate_failed"
	NotifyVote             NotificationEvent = "vote"
	NotifyEscalation       NotificationEvent = "escalation"
)

// NotificationFormatter formats a notification event and protocol message into a string.
type NotificationFormatter func(event NotificationEvent, msg *protocol.Message) string

// NotificationRule defines when and where to send a notification.
type NotificationRule struct {
	Event     NotificationEvent
	Channel   string
	Formatter NotificationFormatter // nil uses default
}

// Notifier routes protocol messages to notification channels based on configured rules.
type Notifier struct {
	client ircclient.Client
	mu     sync.RWMutex
	rules  []NotificationRule
}

// NewNotifier creates a Notifier that sends notifications via the given client.
func NewNotifier(client ircclient.Client) *Notifier {
	return &Notifier{client: client}
}

// AddRule registers a notification rule.
func (n *Notifier) AddRule(rule NotificationRule) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.rules = append(n.rules, rule)
}

// RemoveRulesForEvent removes all rules matching the given event type.
func (n *Notifier) RemoveRulesForEvent(event NotificationEvent) {
	n.mu.Lock()
	defer n.mu.Unlock()
	filtered := n.rules[:0]
	for _, r := range n.rules {
		if r.Event != event {
			filtered = append(filtered, r)
		}
	}
	n.rules = filtered
}

// RemoveRulesForChannel removes all rules targeting the given channel.
func (n *Notifier) RemoveRulesForChannel(channel string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	filtered := n.rules[:0]
	for _, r := range n.rules {
		if r.Channel != channel {
			filtered = append(filtered, r)
		}
	}
	n.rules = filtered
}

// Rules returns a copy of all configured rules.
func (n *Notifier) Rules() []NotificationRule {
	n.mu.RLock()
	defer n.mu.RUnlock()
	result := make([]NotificationRule, len(n.rules))
	copy(result, n.rules)
	return result
}

// HandleMessage maps an incoming protocol message to notification events and sends
// formatted messages to configured channels. This method can be registered as a
// ProtocolHandler on the dispatcher.
func (n *Notifier) HandleMessage(msg *protocol.Message) {
	event := actionToEvent(msg.Action)
	if event == "" {
		return
	}

	// Check for auto-unblock acknowledgement.
	if msg.Action == protocol.ActionAcknowledged && msg.HasTag("auto-unblock") {
		event = NotifyUnblocked
	}
	// Check for gate fail.
	if msg.Action == protocol.ActionGateCheck && msg.Get("status") == "failed" {
		event = NotifyGateFailed
	}

	n.mu.RLock()
	var matching []NotificationRule
	for _, r := range n.rules {
		if r.Event == event {
			matching = append(matching, r)
		}
	}
	n.mu.RUnlock()

	for _, rule := range matching {
		text := DefaultNotificationFormatter(event, msg)
		if rule.Formatter != nil {
			text = rule.Formatter(event, msg)
		}
		if text != "" {
			n.client.SendMessage(rule.Channel, text)
		}
	}
}

// actionToEvent maps a protocol action to a notification event.
func actionToEvent(action protocol.Action) NotificationEvent {
	switch action {
	case protocol.ActionCompleted:
		return NotifyTaskCompleted
	case protocol.ActionBlocked:
		return NotifyTaskBlocked
	case protocol.ActionStarted:
		return NotifyTaskStarted
	case protocol.ActionHelpNeeded:
		return NotifyHelpNeeded
	case protocol.ActionContext:
		return NotifyContextUpdate
	case protocol.ActionAcknowledged:
		return NotifyUnblocked
	case protocol.ActionOffer:
		return NotifyTaskOffered
	case protocol.ActionClaim:
		return NotifyTaskClaimed
	case protocol.ActionAssign:
		return NotifyTaskAssigned
	case protocol.ActionYield:
		return NotifyTaskYielded
	case protocol.ActionCheckpoint:
		return NotifyCheckpoint
	case protocol.ActionHandoff:
		return NotifyHandoff
	case protocol.ActionReviewRequest:
		return NotifyReviewRequested
	case protocol.ActionReviewComplete:
		return NotifyReviewCompleted
	case protocol.ActionGateCheck:
		return NotifyGatePassed // default; HandleMessage checks status field
	case protocol.ActionVote:
		return NotifyVote
	case protocol.ActionEscalate:
		return NotifyEscalation
	default:
		return ""
	}
}

// DefaultNotificationFormatter formats notifications in a human-readable style.
func DefaultNotificationFormatter(event NotificationEvent, msg *protocol.Message) string {
	task := msg.Get("task")
	switch event {
	case NotifyTaskCompleted:
		return fmt.Sprintf("[COMPLETED] %s completed task %q in %s", msg.Nick, task, msg.Channel)
	case NotifyTaskBlocked:
		waitFor := msg.Get("waiting-for")
		if waitFor != "" {
			return fmt.Sprintf("[BLOCKED] %s blocked on task %q waiting for %s", msg.Nick, task, waitFor)
		}
		return fmt.Sprintf("[BLOCKED] %s blocked on task %q", msg.Nick, task)
	case NotifyTaskStarted:
		return fmt.Sprintf("[STARTED] %s started task %q in %s", msg.Nick, task, msg.Channel)
	case NotifyHelpNeeded:
		expertise := msg.Get("expertise")
		if expertise != "" {
			return fmt.Sprintf("[HELP-NEEDED] %s needs help with %q (expertise: %s)", msg.Nick, task, expertise)
		}
		return fmt.Sprintf("[HELP-NEEDED] %s needs help with %q", msg.Nick, task)
	case NotifyContextUpdate:
		component := msg.Get("component")
		return fmt.Sprintf("[CONTEXT] %s updated context for %q", msg.Nick, component)
	case NotifyUnblocked:
		unblockedBy := msg.Get("unblocked-by")
		if unblockedBy != "" {
			return fmt.Sprintf("[UNBLOCKED] task %q unblocked by completion of %q", task, unblockedBy)
		}
		return fmt.Sprintf("[UNBLOCKED] task %q unblocked", task)
	case NotifyTaskOffered:
		return fmt.Sprintf("[OFFERED] %s offered task %q in %s", msg.Nick, task, msg.Channel)
	case NotifyTaskClaimed:
		return fmt.Sprintf("[CLAIMED] %s claimed task %q", msg.Nick, task)
	case NotifyTaskAssigned:
		return fmt.Sprintf("[ASSIGNED] %s assigned task %q to %s", msg.Nick, task, msg.Get("to"))
	case NotifyTaskYielded:
		return fmt.Sprintf("[YIELDED] %s yielded task %q", msg.Nick, task)
	case NotifyCheckpoint:
		return fmt.Sprintf("[CHECKPOINT] %s checkpoint on %q: %s%% - %s", msg.Nick, task, msg.Get("progress"), msg.Get("summary"))
	case NotifyHandoff:
		return fmt.Sprintf("[HANDOFF] %s handed off %q to %s", msg.Nick, task, msg.Get("to"))
	case NotifyReviewRequested:
		return fmt.Sprintf("[REVIEW-REQUEST] %s requested %s review for %q", msg.Nick, msg.Get("review-type"), task)
	case NotifyReviewCompleted:
		return fmt.Sprintf("[REVIEW-COMPLETE] %s reviewed %q: %s", msg.Nick, task, msg.Get("verdict"))
	case NotifyGatePassed:
		return fmt.Sprintf("[GATE-CHECK] %s gate %q for %q: %s", msg.Nick, msg.Get("gate"), task, msg.Get("status"))
	case NotifyGateFailed:
		return fmt.Sprintf("[GATE-FAILED] %s gate %q for %q failed: %s", msg.Nick, msg.Get("gate"), task, msg.Get("details"))
	case NotifyVote:
		return fmt.Sprintf("[VOTE] %s voted %q on topic %q", msg.Nick, msg.Get("choice"), msg.Get("topic"))
	case NotifyEscalation:
		return fmt.Sprintf("[ESCALATE] %s escalated %q to %s: %s (severity: %s)", msg.Nick, task, msg.Get("to"), msg.Get("reason"), msg.Get("severity"))
	default:
		return ""
	}
}
