package protocol

import (
	"errors"
	"testing"
)

func TestSanitize(t *testing.T) {
	tests := []struct {
		name    string
		msg     *Message
		wantErr bool
	}{
		{
			name: "clean message",
			msg: &Message{
				Action: ActionStarted,
				Fields: map[string]string{"task": "login", "priority": "high"},
			},
			wantErr: false,
		},
		{
			name: "sensitive key password",
			msg: &Message{
				Action: ActionContext,
				Fields: map[string]string{"password": "secret123"},
			},
			wantErr: true,
		},
		{
			name: "sensitive key token",
			msg: &Message{
				Action: ActionContext,
				Fields: map[string]string{"token": "abc123"},
			},
			wantErr: true,
		},
		{
			name: "sensitive key api_key",
			msg: &Message{
				Action: ActionContext,
				Fields: map[string]string{"api_key": "abc123"},
			},
			wantErr: true,
		},
		{
			name: "sensitive key secret",
			msg: &Message{
				Action: ActionContext,
				Fields: map[string]string{"secret": "value"},
			},
			wantErr: true,
		},
		{
			name: "sensitive key case insensitive",
			msg: &Message{
				Action: ActionContext,
				Fields: map[string]string{"PASSWORD": "secret123"},
			},
			wantErr: true,
		},
		{
			name: "aws key in value",
			msg: &Message{
				Action: ActionContext,
				Fields: map[string]string{"data": "key=AKIAIOSFODNN7EXAMPLE"},
			},
			wantErr: true,
		},
		{
			name: "bearer token in value",
			msg: &Message{
				Action: ActionContext,
				Fields: map[string]string{"auth-header": "Bearer eyJhbGciOiJIUzI1NiJ9"},
			},
			wantErr: true,
		},
		{
			name: "github token in value",
			msg: &Message{
				Action: ActionContext,
				Fields: map[string]string{"data": "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij"},
			},
			wantErr: true,
		},
		{
			name: "slack token in value",
			msg: &Message{
				Action: ActionContext,
				Fields: map[string]string{"data": "xoxb-123456-abcdef"},
			},
			wantErr: true,
		},
		{
			name: "long hex string in value",
			msg: &Message{
				Action: ActionContext,
				Fields: map[string]string{"data": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"},
			},
			wantErr: true,
		},
		{
			name: "credential in payload",
			msg: &Message{
				Action:  ActionSharingContext,
				Fields:  map[string]string{},
				Payload: "config: password=AKIAIOSFODNN7EXAMPLE",
			},
			wantErr: true,
		},
		{
			name: "clean payload",
			msg: &Message{
				Action:  ActionSharingContext,
				Fields:  map[string]string{},
				Payload: "https://example.com/context/123",
			},
			wantErr: false,
		},
		{
			name: "short hex string ok",
			msg: &Message{
				Action: ActionContext,
				Fields: map[string]string{"commit": "abc123def"},
			},
			wantErr: false,
		},
		{
			name: "sensitive key api-key",
			msg: &Message{
				Action: ActionContext,
				Fields: map[string]string{"api-key": "something"},
			},
			wantErr: true,
		},
		{
			name: "sensitive key credential",
			msg: &Message{
				Action: ActionContext,
				Fields: map[string]string{"credential": "something"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Sanitize(tt.msg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Sanitize() = nil, want error")
				}
				if !errors.Is(err, ErrCredentialDetected) {
					t.Errorf("Sanitize() error = %v, want ErrCredentialDetected", err)
				}
			} else {
				if err != nil {
					t.Errorf("Sanitize() unexpected error: %v", err)
				}
			}
		})
	}
}
