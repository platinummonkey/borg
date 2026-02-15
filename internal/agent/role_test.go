package agent

import (
	"testing"

	"github.com/platinummonkey/borg/pkg/protocol"
)

func TestRoleEngine_Registration(t *testing.T) {
	re := NewRoleEngine([]string{RoleArchitectureReviewer, RoleSecurityReviewer})

	if !re.HasRole(RoleArchitectureReviewer) {
		t.Error("should have architecture-reviewer role")
	}
	if re.HasRole(RoleMergeCoordinator) {
		t.Error("should not have merge-coordinator role")
	}
}

func TestRoleEngine_ExpertiseTags(t *testing.T) {
	re := NewRoleEngine([]string{RoleArchitectureReviewer})
	tags := re.ExpertiseTags()
	if len(tags) != 1 {
		t.Fatalf("ExpertiseTags = %d, want 1", len(tags))
	}
	if tags[0] != "role:architecture-reviewer" {
		t.Errorf("tag = %q, want 'role:architecture-reviewer'", tags[0])
	}
}

func TestRoleEngine_TriggerMatching(t *testing.T) {
	re := NewRoleEngine([]string{RoleArchitectureReviewer})
	b := BuiltinBehavior(RoleArchitectureReviewer)
	if b == nil {
		t.Fatal("BuiltinBehavior returned nil for architecture-reviewer")
	}
	re.RegisterBehavior(b)

	// Should trigger on REVIEW-REQUEST with review-type=architecture.
	msg := &protocol.Message{
		Action:  protocol.ActionReviewRequest,
		Fields:  map[string]string{"task": "auth", "pr": "PR-1", "review-type": "architecture"},
		Nick:    "alice",
		Channel: "#project",
	}
	responses := re.HandleMessage(msg)
	if len(responses) != 1 {
		t.Fatalf("HandleMessage = %d responses, want 1", len(responses))
	}
	if responses[0].Action != protocol.ActionReviewComplete {
		t.Errorf("response action = %s, want REVIEW-COMPLETE", responses[0].Action)
	}
	if responses[0].Get("verdict") != string(ReviewApproved) {
		t.Errorf("verdict = %q, want approved", responses[0].Get("verdict"))
	}
}

func TestRoleEngine_NoTriggerForWrongType(t *testing.T) {
	re := NewRoleEngine([]string{RoleArchitectureReviewer})
	b := BuiltinBehavior(RoleArchitectureReviewer)
	re.RegisterBehavior(b)

	// Should NOT trigger for security review type.
	msg := &protocol.Message{
		Action:  protocol.ActionReviewRequest,
		Fields:  map[string]string{"task": "auth", "review-type": "security"},
		Nick:    "alice",
		Channel: "#project",
	}
	responses := re.HandleMessage(msg)
	if len(responses) != 0 {
		t.Errorf("HandleMessage = %d responses, want 0 for wrong review-type", len(responses))
	}
}

func TestRoleEngine_MonitoringGuardian(t *testing.T) {
	re := NewRoleEngine([]string{RoleMonitoringGuardian})
	b := BuiltinBehavior(RoleMonitoringGuardian)
	re.RegisterBehavior(b)

	msg := &protocol.Message{
		Action:  protocol.ActionReviewRequest,
		Fields:  map[string]string{"task": "auth", "review-type": "monitoring"},
		Nick:    "alice",
		Channel: "#project",
	}
	responses := re.HandleMessage(msg)
	if len(responses) != 1 {
		t.Fatalf("HandleMessage = %d responses, want 1", len(responses))
	}
	if responses[0].Action != protocol.ActionGateCheck {
		t.Errorf("response action = %s, want GATE-CHECK", responses[0].Action)
	}
}

func TestRoleEngine_MergeCoordinator(t *testing.T) {
	re := NewRoleEngine([]string{RoleMergeCoordinator})
	b := BuiltinBehavior(RoleMergeCoordinator)
	re.RegisterBehavior(b)

	msg := &protocol.Message{
		Action:  protocol.ActionReviewComplete,
		Fields:  map[string]string{"task": "auth", "verdict": string(ReviewApproved)},
		Nick:    "reviewer",
		Channel: "#project",
	}
	responses := re.HandleMessage(msg)
	if len(responses) != 1 {
		t.Fatalf("HandleMessage = %d responses, want 1", len(responses))
	}
	if responses[0].Get("task") != "auth-merge" {
		t.Errorf("task = %q, want auth-merge", responses[0].Get("task"))
	}
}

func TestRoleEngine_IncidentResponder(t *testing.T) {
	re := NewRoleEngine([]string{RoleIncidentResponder})
	b := BuiltinBehavior(RoleIncidentResponder)
	re.RegisterBehavior(b)

	// Low severity — should not trigger.
	msg := &protocol.Message{
		Action:  protocol.ActionEscalate,
		Fields:  map[string]string{"task": "auth", "severity": "low"},
		Nick:    "bot",
		Channel: "#project",
	}
	responses := re.HandleMessage(msg)
	if len(responses) != 0 {
		t.Errorf("low severity should not trigger incident responder")
	}

	// High severity — should trigger.
	msg.Fields["severity"] = "high"
	responses = re.HandleMessage(msg)
	if len(responses) != 1 {
		t.Fatalf("high severity should trigger incident responder, got %d responses", len(responses))
	}
	if responses[0].Get("task") != "auth-incident" {
		t.Errorf("task = %q, want auth-incident", responses[0].Get("task"))
	}
}

func TestBuiltinBehavior_Unknown(t *testing.T) {
	if b := BuiltinBehavior("nonexistent-role"); b != nil {
		t.Error("unknown role should return nil behavior")
	}
}
