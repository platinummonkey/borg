package protocol

import (
	"errors"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    *Message
		wantErr error
	}{
		{
			name: "simple started",
			raw:  "STARTED task=implement-login priority=high #blocked-by-db-migration",
			want: &Message{
				Action: ActionStarted,
				Fields: map[string]string{"task": "implement-login", "priority": "high"},
				Tags:   []string{"blocked-by-db-migration"},
			},
		},
		{
			name: "completed with tag",
			raw:  "COMPLETED task=db-migration #unblocks-others",
			want: &Message{
				Action: ActionCompleted,
				Fields: map[string]string{"task": "db-migration"},
				Tags:   []string{"unblocks-others"},
			},
		},
		{
			name: "blocked with waiting-for",
			raw:  "BLOCKED task=payment-integration waiting-for=api-keys",
			want: &Message{
				Action: ActionBlocked,
				Fields: map[string]string{"task": "payment-integration", "waiting-for": "api-keys"},
			},
		},
		{
			name: "acknowledged",
			raw:  "ACKNOWLEDGED task=auth-refactor #starting-integration-tests",
			want: &Message{
				Action: ActionAcknowledged,
				Fields: map[string]string{"task": "auth-refactor"},
				Tags:   []string{"starting-integration-tests"},
			},
		},
		{
			name: "help needed",
			raw:  "HELP-NEEDED task=performance-tuning expertise=database",
			want: &Message{
				Action: ActionHelpNeeded,
				Fields: map[string]string{"task": "performance-tuning", "expertise": "database"},
			},
		},
		{
			name: "context announce",
			raw:  "CONTEXT project=webapp component=auth status=updated files=3",
			want: &Message{
				Action: ActionContext,
				Fields: map[string]string{"project": "webapp", "component": "auth", "status": "updated", "files": "3"},
			},
		},
		{
			name: "request context",
			raw:  "REQUEST-CONTEXT component=auth",
			want: &Message{
				Action: ActionRequestContext,
				Fields: map[string]string{"component": "auth"},
			},
		},
		{
			name: "sharing context with payload",
			raw:  "SHARING-CONTEXT aHR0cDovL2V4YW1wbGUuY29t",
			want: &Message{
				Action:  ActionSharingContext,
				Fields:  map[string]string{},
				Payload: "aHR0cDovL2V4YW1wbGUuY29t",
			},
		},
		{
			name: "sharing context with url payload",
			raw:  "SHARING-CONTEXT https://example.com/context/123",
			want: &Message{
				Action:  ActionSharingContext,
				Fields:  map[string]string{},
				Payload: "https://example.com/context/123",
			},
		},
		{
			name: "sharing context with payload containing spaces",
			raw:  "SHARING-CONTEXT some long payload with spaces",
			want: &Message{
				Action:  ActionSharingContext,
				Fields:  map[string]string{},
				Payload: "some long payload with spaces",
			},
		},
		{
			name: "quoted value with spaces",
			raw:  `STARTED task=login description="implement login flow" priority=high`,
			want: &Message{
				Action: ActionStarted,
				Fields: map[string]string{"task": "login", "description": "implement login flow", "priority": "high"},
			},
		},
		{
			name: "multiple tags",
			raw:  "COMPLETED task=auth #ready-for-testing #unblocks-others #urgent",
			want: &Message{
				Action: ActionCompleted,
				Fields: map[string]string{"task": "auth"},
				Tags:   []string{"ready-for-testing", "unblocks-others", "urgent"},
			},
		},
		{
			name: "fields and tags mixed order",
			raw:  "STARTED #urgent task=login priority=high #feature",
			want: &Message{
				Action: ActionStarted,
				Fields: map[string]string{"task": "login", "priority": "high"},
				Tags:   []string{"urgent", "feature"},
			},
		},
		{
			name: "case insensitive action",
			raw:  "started task=login",
			want: &Message{
				Action: ActionStarted,
				Fields: map[string]string{"task": "login"},
			},
		},
		{
			name: "action only",
			raw:  "COMPLETED",
			want: &Message{
				Action: ActionCompleted,
				Fields: map[string]string{},
			},
		},
		{
			name: "extra whitespace",
			raw:  "  STARTED   task=login   priority=high   #tag1  ",
			want: &Message{
				Action: ActionStarted,
				Fields: map[string]string{"task": "login", "priority": "high"},
				Tags:   []string{"tag1"},
			},
		},
		{
			name:    "empty string",
			raw:     "",
			wantErr: ErrEmptyMessage,
		},
		{
			name:    "whitespace only",
			raw:     "   ",
			wantErr: ErrEmptyMessage,
		},
		{
			name:    "unknown action",
			raw:     "UNKNOWN task=x",
			wantErr: ErrUnknownAction,
		},
		{
			name:    "unclosed quote",
			raw:     `STARTED task="no closing quote`,
			wantErr: ErrUnclosedQuote,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.raw)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("Parse(%q) = nil error, want %v", tt.raw, tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Parse(%q) error = %v, want %v", tt.raw, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.raw, err)
			}

			if got.Action != tt.want.Action {
				t.Errorf("Action = %q, want %q", got.Action, tt.want.Action)
			}

			if len(got.Fields) != len(tt.want.Fields) {
				t.Errorf("Fields count = %d, want %d\n  got:  %v\n  want: %v", len(got.Fields), len(tt.want.Fields), got.Fields, tt.want.Fields)
			} else {
				for k, wantV := range tt.want.Fields {
					if gotV, ok := got.Fields[k]; !ok || gotV != wantV {
						t.Errorf("Fields[%q] = %q, want %q", k, gotV, wantV)
					}
				}
			}

			if len(got.Tags) != len(tt.want.Tags) {
				t.Errorf("Tags = %v, want %v", got.Tags, tt.want.Tags)
			} else {
				for i, wantTag := range tt.want.Tags {
					if got.Tags[i] != wantTag {
						t.Errorf("Tags[%d] = %q, want %q", i, got.Tags[i], wantTag)
					}
				}
			}

			if got.Payload != tt.want.Payload {
				t.Errorf("Payload = %q, want %q", got.Payload, tt.want.Payload)
			}
		})
	}
}
