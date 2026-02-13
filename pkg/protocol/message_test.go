package protocol

import (
	"testing"
)

func TestActionValid(t *testing.T) {
	tests := []struct {
		action Action
		valid  bool
	}{
		{ActionStarted, true},
		{ActionCompleted, true},
		{ActionBlocked, true},
		{ActionAcknowledged, true},
		{ActionHelpNeeded, true},
		{ActionContext, true},
		{ActionRequestContext, true},
		{ActionSharingContext, true},
		{Action("UNKNOWN"), false},
		{Action("started"), false}, // case-sensitive
		{Action(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			if got := tt.action.Valid(); got != tt.valid {
				t.Errorf("Action(%q).Valid() = %v, want %v", tt.action, got, tt.valid)
			}
		})
	}
}

func TestMessageGet(t *testing.T) {
	msg := &Message{
		Fields: map[string]string{"task": "login", "priority": "high"},
	}

	if got := msg.Get("task"); got != "login" {
		t.Errorf("Get(task) = %q, want %q", got, "login")
	}
	if got := msg.Get("missing"); got != "" {
		t.Errorf("Get(missing) = %q, want empty", got)
	}

	// Nil fields map.
	nilMsg := &Message{}
	if got := nilMsg.Get("anything"); got != "" {
		t.Errorf("Get on nil Fields = %q, want empty", got)
	}
}

func TestMessageHasTag(t *testing.T) {
	msg := &Message{
		Tags: []string{"ready", "urgent"},
	}

	if !msg.HasTag("ready") {
		t.Error("HasTag(ready) = false, want true")
	}
	if !msg.HasTag("urgent") {
		t.Error("HasTag(urgent) = false, want true")
	}
	if msg.HasTag("missing") {
		t.Error("HasTag(missing) = true, want false")
	}

	// Empty tags.
	emptyMsg := &Message{}
	if emptyMsg.HasTag("anything") {
		t.Error("HasTag on empty Tags = true, want false")
	}
}

func TestIsProtocolMessage(t *testing.T) {
	tests := []struct {
		raw  string
		want bool
	}{
		{"STARTED task=login", true},
		{"COMPLETED task=login #done", true},
		{"BLOCKED task=x waiting-for=y", true},
		{"ACKNOWLEDGED task=x", true},
		{"HELP-NEEDED task=x expertise=db", true},
		{"CONTEXT project=webapp", true},
		{"REQUEST-CONTEXT component=auth", true},
		{"SHARING-CONTEXT aHR0cDovL2V4YW1wbGU=", true},
		{"  STARTED task=login  ", true}, // leading/trailing whitespace
		{"COMPLETED", true},              // action only
		{"hello world", false},
		{"", false},
		{"started task=login", false}, // lowercase
		{"NOTANACTION task=x", false},
		{"STARTEDX task=x", false}, // prefix but not action
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := IsProtocolMessage(tt.raw); got != tt.want {
				t.Errorf("IsProtocolMessage(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}
