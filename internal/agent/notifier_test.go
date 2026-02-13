package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/platinummonkey/agent-chat/pkg/protocol"
)

func TestNotifier_HandleMessage_Completed(t *testing.T) {
	client := &stubClient{nick: "self"}
	n := NewNotifier(client)
	n.AddRule(NotificationRule{
		Event:   NotifyTaskCompleted,
		Channel: "#ops",
	})

	n.HandleMessage(&protocol.Message{
		Action:    protocol.ActionCompleted,
		Fields:    map[string]string{"task": "auth-refactor"},
		Nick:      "agent-alice-1",
		Channel:   "#project",
		Timestamp: time.Now(),
	})

	msgs := client.sentMessages()
	if len(msgs) != 1 {
		t.Fatalf("sent %d messages, want 1", len(msgs))
	}
	if msgs[0].target != "#ops" {
		t.Errorf("target = %q, want %q", msgs[0].target, "#ops")
	}
	if !strings.Contains(msgs[0].message, "[COMPLETED]") {
		t.Errorf("message missing [COMPLETED]: %q", msgs[0].message)
	}
	if !strings.Contains(msgs[0].message, "auth-refactor") {
		t.Errorf("message missing task name: %q", msgs[0].message)
	}
}

func TestNotifier_HandleMessage_Blocked(t *testing.T) {
	client := &stubClient{nick: "self"}
	n := NewNotifier(client)
	n.AddRule(NotificationRule{
		Event:   NotifyTaskBlocked,
		Channel: "#ops",
	})

	n.HandleMessage(&protocol.Message{
		Action:    protocol.ActionBlocked,
		Fields:    map[string]string{"task": "payment", "waiting-for": "api-keys"},
		Nick:      "agent-bob-2",
		Channel:   "#project",
		Timestamp: time.Now(),
	})

	msgs := client.sentMessages()
	if len(msgs) != 1 {
		t.Fatalf("sent %d messages, want 1", len(msgs))
	}
	if !strings.Contains(msgs[0].message, "[BLOCKED]") {
		t.Errorf("message missing [BLOCKED]: %q", msgs[0].message)
	}
	if !strings.Contains(msgs[0].message, "api-keys") {
		t.Errorf("message missing waiting-for: %q", msgs[0].message)
	}
}

func TestNotifier_HandleMessage_HelpNeeded(t *testing.T) {
	client := &stubClient{nick: "self"}
	n := NewNotifier(client)
	n.AddRule(NotificationRule{
		Event:   NotifyHelpNeeded,
		Channel: "#help",
	})

	n.HandleMessage(&protocol.Message{
		Action:    protocol.ActionHelpNeeded,
		Fields:    map[string]string{"task": "perf-tuning", "expertise": "database"},
		Nick:      "agent-bob-2",
		Channel:   "#project",
		Timestamp: time.Now(),
	})

	msgs := client.sentMessages()
	if len(msgs) != 1 {
		t.Fatalf("sent %d messages, want 1", len(msgs))
	}
	if !strings.Contains(msgs[0].message, "[HELP-NEEDED]") {
		t.Errorf("message missing [HELP-NEEDED]: %q", msgs[0].message)
	}
	if !strings.Contains(msgs[0].message, "database") {
		t.Errorf("message missing expertise: %q", msgs[0].message)
	}
}

func TestNotifier_HandleMessage_Unblocked(t *testing.T) {
	client := &stubClient{nick: "self"}
	n := NewNotifier(client)
	n.AddRule(NotificationRule{
		Event:   NotifyUnblocked,
		Channel: "#ops",
	})

	n.HandleMessage(&protocol.Message{
		Action: protocol.ActionAcknowledged,
		Fields: map[string]string{"task": "integration-tests", "unblocked-by": "auth-refactor"},
		Tags:   []string{"auto-unblock"},
		Nick:   "agent-alice-1",
	})

	msgs := client.sentMessages()
	if len(msgs) != 1 {
		t.Fatalf("sent %d messages, want 1", len(msgs))
	}
	if !strings.Contains(msgs[0].message, "[UNBLOCKED]") {
		t.Errorf("message missing [UNBLOCKED]: %q", msgs[0].message)
	}
	if !strings.Contains(msgs[0].message, "auth-refactor") {
		t.Errorf("message missing unblocked-by: %q", msgs[0].message)
	}
}

func TestNotifier_HandleMessage_NoMatchingRules(t *testing.T) {
	client := &stubClient{nick: "self"}
	n := NewNotifier(client)
	// Only rule for COMPLETED — a STARTED message should not trigger.
	n.AddRule(NotificationRule{
		Event:   NotifyTaskCompleted,
		Channel: "#ops",
	})

	n.HandleMessage(&protocol.Message{
		Action:    protocol.ActionStarted,
		Fields:    map[string]string{"task": "x"},
		Nick:      "agent-1",
		Timestamp: time.Now(),
	})

	msgs := client.sentMessages()
	if len(msgs) != 0 {
		t.Errorf("sent %d messages, want 0 (no matching rule)", len(msgs))
	}
}

func TestNotifier_HandleMessage_CustomFormatter(t *testing.T) {
	client := &stubClient{nick: "self"}
	n := NewNotifier(client)
	n.AddRule(NotificationRule{
		Event:   NotifyTaskCompleted,
		Channel: "#ops",
		Formatter: func(event NotificationEvent, msg *protocol.Message) string {
			return "custom: " + msg.Get("task")
		},
	})

	n.HandleMessage(&protocol.Message{
		Action: protocol.ActionCompleted,
		Fields: map[string]string{"task": "my-task"},
		Nick:   "agent-1",
	})

	msgs := client.sentMessages()
	if len(msgs) != 1 {
		t.Fatalf("sent %d messages, want 1", len(msgs))
	}
	if msgs[0].message != "custom: my-task" {
		t.Errorf("message = %q, want %q", msgs[0].message, "custom: my-task")
	}
}

func TestNotifier_RemoveRulesForEvent(t *testing.T) {
	client := &stubClient{nick: "self"}
	n := NewNotifier(client)
	n.AddRule(NotificationRule{Event: NotifyTaskCompleted, Channel: "#ops"})
	n.AddRule(NotificationRule{Event: NotifyTaskBlocked, Channel: "#ops"})

	n.RemoveRulesForEvent(NotifyTaskCompleted)

	rules := n.Rules()
	if len(rules) != 1 {
		t.Fatalf("Rules = %d, want 1", len(rules))
	}
	if rules[0].Event != NotifyTaskBlocked {
		t.Errorf("remaining rule event = %q, want %q", rules[0].Event, NotifyTaskBlocked)
	}
}

func TestNotifier_RemoveRulesForChannel(t *testing.T) {
	client := &stubClient{nick: "self"}
	n := NewNotifier(client)
	n.AddRule(NotificationRule{Event: NotifyTaskCompleted, Channel: "#ops"})
	n.AddRule(NotificationRule{Event: NotifyTaskBlocked, Channel: "#alerts"})

	n.RemoveRulesForChannel("#ops")

	rules := n.Rules()
	if len(rules) != 1 {
		t.Fatalf("Rules = %d, want 1", len(rules))
	}
	if rules[0].Channel != "#alerts" {
		t.Errorf("remaining rule channel = %q, want %q", rules[0].Channel, "#alerts")
	}
}

func TestNotifier_MultipleRulesSameEvent(t *testing.T) {
	client := &stubClient{nick: "self"}
	n := NewNotifier(client)
	n.AddRule(NotificationRule{Event: NotifyTaskCompleted, Channel: "#ops"})
	n.AddRule(NotificationRule{Event: NotifyTaskCompleted, Channel: "#alerts"})

	n.HandleMessage(&protocol.Message{
		Action: protocol.ActionCompleted,
		Fields: map[string]string{"task": "x"},
		Nick:   "agent-1",
	})

	msgs := client.sentMessages()
	if len(msgs) != 2 {
		t.Fatalf("sent %d messages, want 2", len(msgs))
	}
	targets := map[string]bool{msgs[0].target: true, msgs[1].target: true}
	if !targets["#ops"] || !targets["#alerts"] {
		t.Errorf("targets = %v, want #ops and #alerts", targets)
	}
}

func TestNotifier_ContextUpdateEvent(t *testing.T) {
	client := &stubClient{nick: "self"}
	n := NewNotifier(client)
	n.AddRule(NotificationRule{Event: NotifyContextUpdate, Channel: "#ops"})

	n.HandleMessage(&protocol.Message{
		Action: protocol.ActionContext,
		Fields: map[string]string{"component": "auth"},
		Nick:   "agent-1",
	})

	msgs := client.sentMessages()
	if len(msgs) != 1 {
		t.Fatalf("sent %d messages, want 1", len(msgs))
	}
	if !strings.Contains(msgs[0].message, "[CONTEXT]") {
		t.Errorf("message missing [CONTEXT]: %q", msgs[0].message)
	}
}

func TestNotifier_UnknownAction(t *testing.T) {
	client := &stubClient{nick: "self"}
	n := NewNotifier(client)
	n.AddRule(NotificationRule{Event: NotifyTaskCompleted, Channel: "#ops"})

	// REQUEST-CONTEXT does not map to a notification event.
	n.HandleMessage(&protocol.Message{
		Action: protocol.ActionRequestContext,
		Fields: map[string]string{"component": "auth"},
		Nick:   "agent-1",
	})

	msgs := client.sentMessages()
	if len(msgs) != 0 {
		t.Errorf("sent %d messages, want 0 (unknown action)", len(msgs))
	}
}
