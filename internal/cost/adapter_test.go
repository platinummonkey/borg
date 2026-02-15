package cost

import (
	"testing"

	"github.com/platinummonkey/agent-chat/pkg/protocol"
)

func TestBuildCostMessage(t *testing.T) {
	msg := buildCostMessage("anthropic", UsageReport{
		Task:         "auth",
		Model:        "claude-sonnet-4-5-20250929",
		InputTokens:  1500,
		OutputTokens: 500,
		TotalTokens:  2000,
		CostUSD:      0.0125,
	})

	if msg.Action != protocol.ActionCostReport {
		t.Errorf("action = %q, want %q", msg.Action, protocol.ActionCostReport)
	}
	if msg.Get("task") != "auth" {
		t.Errorf("task = %q, want %q", msg.Get("task"), "auth")
	}
	if msg.Get("provider") != "anthropic" {
		t.Errorf("provider = %q, want %q", msg.Get("provider"), "anthropic")
	}
	if msg.Get("input-tokens") != "1500" {
		t.Errorf("input-tokens = %q, want %q", msg.Get("input-tokens"), "1500")
	}

	// Round-trip: serialize and re-parse.
	raw := msg.String()
	parsed, err := protocol.Parse(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if parsed.Action != protocol.ActionCostReport {
		t.Errorf("parsed action = %q, want %q", parsed.Action, protocol.ActionCostReport)
	}
	if parsed.Get("model") != "claude-sonnet-4-5-20250929" {
		t.Errorf("parsed model = %q, want %q", parsed.Get("model"), "claude-sonnet-4-5-20250929")
	}
}

func TestAnthropicCost(t *testing.T) {
	cost := anthropicCost("claude-sonnet-4-5-20250929", 1_000_000, 1_000_000)
	expected := 3.0 + 15.0
	if cost != expected {
		t.Errorf("cost = %f, want %f", cost, expected)
	}
}

func TestAnthropicCost_Opus(t *testing.T) {
	cost := anthropicCost("claude-opus-4-6", 1_000_000, 1_000_000)
	expected := 15.0 + 75.0
	if cost != expected {
		t.Errorf("cost = %f, want %f", cost, expected)
	}
}

func TestOpenAICost(t *testing.T) {
	cost := openaiCost("gpt-4o", 1_000_000, 1_000_000)
	expected := 2.50 + 10.0
	if cost != expected {
		t.Errorf("cost = %f, want %f", cost, expected)
	}
}

func TestGenericAdapter_Provider(t *testing.T) {
	a := NewGenericAdapter("custom")
	if a.Provider() != "custom" {
		t.Errorf("provider = %q, want %q", a.Provider(), "custom")
	}
}

func TestAnthropicAdapter_Provider(t *testing.T) {
	a := NewAnthropicAdapter()
	if a.Provider() != "anthropic" {
		t.Errorf("provider = %q, want %q", a.Provider(), "anthropic")
	}
}

func TestOpenAIAdapter_Provider(t *testing.T) {
	a := NewOpenAIAdapter()
	if a.Provider() != "openai" {
		t.Errorf("provider = %q, want %q", a.Provider(), "openai")
	}
}
