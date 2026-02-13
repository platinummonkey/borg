package protocol

import (
	"testing"
)

func TestMessageString(t *testing.T) {
	tests := []struct {
		name string
		msg  *Message
		want string
	}{
		{
			name: "simple started",
			msg: &Message{
				Action: ActionStarted,
				Fields: map[string]string{"task": "login", "priority": "high"},
				Tags:   []string{"urgent"},
			},
			want: `STARTED priority=high task=login #urgent`,
		},
		{
			name: "completed with tag",
			msg: &Message{
				Action: ActionCompleted,
				Fields: map[string]string{"task": "db-migration"},
				Tags:   []string{"unblocks-others"},
			},
			want: `COMPLETED task=db-migration #unblocks-others`,
		},
		{
			name: "quoted value with spaces",
			msg: &Message{
				Action: ActionStarted,
				Fields: map[string]string{"task": "login", "description": "implement login flow"},
			},
			want: `STARTED description="implement login flow" task=login`,
		},
		{
			name: "sharing context with payload",
			msg: &Message{
				Action:  ActionSharingContext,
				Fields:  map[string]string{},
				Payload: "aHR0cDovL2V4YW1wbGUuY29t",
			},
			want: `SHARING-CONTEXT aHR0cDovL2V4YW1wbGUuY29t`,
		},
		{
			name: "sharing context empty payload",
			msg: &Message{
				Action: ActionSharingContext,
			},
			want: `SHARING-CONTEXT`,
		},
		{
			name: "action only",
			msg: &Message{
				Action: ActionCompleted,
				Fields: map[string]string{},
			},
			want: `COMPLETED`,
		},
		{
			name: "multiple tags",
			msg: &Message{
				Action: ActionCompleted,
				Fields: map[string]string{"task": "auth"},
				Tags:   []string{"ready", "urgent"},
			},
			want: `COMPLETED task=auth #ready #urgent`,
		},
		{
			name: "sorted keys deterministic",
			msg: &Message{
				Action: ActionContext,
				Fields: map[string]string{"z": "1", "a": "2", "m": "3"},
			},
			want: `CONTEXT a=2 m=3 z=1`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.msg.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"started simple", "STARTED priority=high task=login #urgent"},
		{"completed", "COMPLETED task=db-migration #unblocks-others"},
		{"blocked", "BLOCKED task=payment waiting-for=api-keys"},
		{"quoted value", `STARTED description="implement login flow" task=login`},
		{"context", "CONTEXT component=auth project=webapp status=updated"},
		{"request context", "REQUEST-CONTEXT component=auth"},
		{"sharing context", "SHARING-CONTEXT aHR0cDovL2V4YW1wbGUuY29t"},
		{"action only", "COMPLETED"},
		{"multiple tags", "COMPLETED task=auth #ready #urgent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := Parse(tt.raw)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.raw, err)
			}

			serialized := msg.String()
			msg2, err := Parse(serialized)
			if err != nil {
				t.Fatalf("Parse(serialized) error: %v\n  serialized: %q", err, serialized)
			}

			if msg.Action != msg2.Action {
				t.Errorf("Round-trip Action: %q != %q", msg.Action, msg2.Action)
			}
			if len(msg.Fields) != len(msg2.Fields) {
				t.Errorf("Round-trip Fields count: %d != %d", len(msg.Fields), len(msg2.Fields))
			}
			for k, v := range msg.Fields {
				if msg2.Fields[k] != v {
					t.Errorf("Round-trip Fields[%q]: %q != %q", k, v, msg2.Fields[k])
				}
			}
			if len(msg.Tags) != len(msg2.Tags) {
				t.Errorf("Round-trip Tags: %v != %v", msg.Tags, msg2.Tags)
			}
			if msg.Payload != msg2.Payload {
				t.Errorf("Round-trip Payload: %q != %q", msg.Payload, msg2.Payload)
			}
		})
	}
}
