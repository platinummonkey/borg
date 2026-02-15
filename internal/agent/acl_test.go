package agent

import (
	"testing"

	"github.com/platinummonkey/borg/internal/config"
	"github.com/platinummonkey/borg/pkg/protocol"
)

func TestACL_DefaultAllow(t *testing.T) {
	e := NewACLEngine(nil)
	if !e.Check("agent-1", "#project", protocol.ActionStarted) {
		t.Error("no rules should default allow")
	}
}

func TestACL_ExplicitDeny(t *testing.T) {
	e := NewACLEngine([]config.ACLRule{
		{Channel: "#secure", NickPattern: "*", Effect: "deny"},
	})
	if e.Check("agent-1", "#secure", protocol.ActionStarted) {
		t.Error("should be denied")
	}
}

func TestACL_ExplicitAllow(t *testing.T) {
	e := NewACLEngine([]config.ACLRule{
		{Channel: "#secure", NickPattern: "agent-alice-*", Effect: "allow"},
		{Channel: "#secure", NickPattern: "*", Effect: "deny"},
	})
	if !e.Check("agent-alice-1", "#secure", protocol.ActionStarted) {
		t.Error("agent-alice-1 should be allowed")
	}
}

func TestACL_FirstMatchWins(t *testing.T) {
	e := NewACLEngine([]config.ACLRule{
		{Channel: "#secure", NickPattern: "agent-alice-*", Effect: "deny"},
		{Channel: "#secure", NickPattern: "agent-alice-*", Effect: "allow"},
	})
	if e.Check("agent-alice-1", "#secure", protocol.ActionStarted) {
		t.Error("first match should win (deny)")
	}
}

func TestACL_WildcardPatterns(t *testing.T) {
	e := NewACLEngine([]config.ACLRule{
		{Channel: "*", NickPattern: "*", Effect: "deny"},
	})
	if e.Check("anyone", "#anything", protocol.ActionCompleted) {
		t.Error("wildcard deny should block everything")
	}
}

func TestACL_EmptyActionsMatchAll(t *testing.T) {
	e := NewACLEngine([]config.ACLRule{
		{Channel: "#ops", NickPattern: "*", Actions: nil, Effect: "deny"},
	})
	if e.Check("agent-1", "#ops", protocol.ActionStarted) {
		t.Error("empty actions should match all actions")
	}
	if e.Check("agent-1", "#ops", protocol.ActionCompleted) {
		t.Error("empty actions should match all actions")
	}
}

func TestACL_SpecificActions(t *testing.T) {
	e := NewACLEngine([]config.ACLRule{
		{Channel: "#ops", NickPattern: "*", Actions: []protocol.Action{protocol.ActionCompleted}, Effect: "deny"},
	})
	if e.Check("agent-1", "#ops", protocol.ActionCompleted) {
		t.Error("COMPLETED should be denied")
	}
	if !e.Check("agent-1", "#ops", protocol.ActionStarted) {
		t.Error("STARTED should be allowed (action not in rule list, falls through to default)")
	}
}

func TestACL_SetRules(t *testing.T) {
	e := NewACLEngine(nil)
	if !e.Check("agent-1", "#secure", protocol.ActionStarted) {
		t.Error("initially should allow")
	}

	e.SetRules([]config.ACLRule{
		{Channel: "#secure", NickPattern: "*", Effect: "deny"},
	})
	if e.Check("agent-1", "#secure", protocol.ActionStarted) {
		t.Error("after SetRules should deny")
	}
}
