package cost

import (
	"fmt"

	"github.com/platinummonkey/agent-chat/pkg/ircclient"
	"github.com/platinummonkey/agent-chat/pkg/protocol"
)

// UsageReport contains LLM usage data for a single request or session.
type UsageReport struct {
	Task         string
	Model        string
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	CostUSD      float64
}

// LLMAdapter abstracts LLM provider-specific cost reporting.
type LLMAdapter interface {
	// Provider returns the provider name (e.g. "anthropic", "openai").
	Provider() string

	// ReportUsage sends a COST-REPORT protocol message for the given usage.
	ReportUsage(client ircclient.Client, channel string, usage UsageReport) error
}

// GenericAdapter is a provider-agnostic LLM adapter that sends cost reports
// using pre-calculated cost values.
type GenericAdapter struct {
	provider string
}

// NewGenericAdapter creates an adapter with the given provider name.
func NewGenericAdapter(provider string) *GenericAdapter {
	return &GenericAdapter{provider: provider}
}

func (a *GenericAdapter) Provider() string { return a.provider }

func (a *GenericAdapter) ReportUsage(client ircclient.Client, channel string, usage UsageReport) error {
	msg := buildCostMessage(a.provider, usage)
	if err := protocol.Sanitize(msg); err != nil {
		return err
	}
	client.SendMessage(channel, msg.String())
	return nil
}

// AnthropicAdapter calculates cost from Anthropic pricing and sends cost reports.
type AnthropicAdapter struct{}

func NewAnthropicAdapter() *AnthropicAdapter { return &AnthropicAdapter{} }

func (a *AnthropicAdapter) Provider() string { return "anthropic" }

func (a *AnthropicAdapter) ReportUsage(client ircclient.Client, channel string, usage UsageReport) error {
	if usage.CostUSD == 0 {
		usage.CostUSD = anthropicCost(usage.Model, usage.InputTokens, usage.OutputTokens)
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	msg := buildCostMessage("anthropic", usage)
	if err := protocol.Sanitize(msg); err != nil {
		return err
	}
	client.SendMessage(channel, msg.String())
	return nil
}

// OpenAIAdapter calculates cost from OpenAI pricing and sends cost reports.
type OpenAIAdapter struct{}

func NewOpenAIAdapter() *OpenAIAdapter { return &OpenAIAdapter{} }

func (a *OpenAIAdapter) Provider() string { return "openai" }

func (a *OpenAIAdapter) ReportUsage(client ircclient.Client, channel string, usage UsageReport) error {
	if usage.CostUSD == 0 {
		usage.CostUSD = openaiCost(usage.Model, usage.InputTokens, usage.OutputTokens)
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	msg := buildCostMessage("openai", usage)
	if err := protocol.Sanitize(msg); err != nil {
		return err
	}
	client.SendMessage(channel, msg.String())
	return nil
}

func buildCostMessage(provider string, usage UsageReport) *protocol.Message {
	return &protocol.Message{
		Action: protocol.ActionCostReport,
		Fields: map[string]string{
			"task":          usage.Task,
			"model":         usage.Model,
			"provider":      provider,
			"input-tokens":  fmt.Sprintf("%d", usage.InputTokens),
			"output-tokens": fmt.Sprintf("%d", usage.OutputTokens),
			"total-tokens":  fmt.Sprintf("%d", usage.TotalTokens),
			"cost-usd":      fmt.Sprintf("%.6f", usage.CostUSD),
		},
	}
}

// anthropicCost estimates USD cost based on known Anthropic pricing per 1M tokens.
func anthropicCost(model string, inputTokens, outputTokens int64) float64 {
	var inputPer1M, outputPer1M float64
	switch model {
	case "claude-opus-4-6", "claude-opus-4-0-20250514":
		inputPer1M, outputPer1M = 15.0, 75.0
	case "claude-sonnet-4-5-20250929", "claude-sonnet-4-0-20250514":
		inputPer1M, outputPer1M = 3.0, 15.0
	case "claude-haiku-4-5-20251001":
		inputPer1M, outputPer1M = 0.80, 4.0
	default:
		inputPer1M, outputPer1M = 3.0, 15.0
	}
	return (float64(inputTokens)/1_000_000)*inputPer1M + (float64(outputTokens)/1_000_000)*outputPer1M
}

// openaiCost estimates USD cost based on known OpenAI pricing per 1M tokens.
func openaiCost(model string, inputTokens, outputTokens int64) float64 {
	var inputPer1M, outputPer1M float64
	switch model {
	case "gpt-4o":
		inputPer1M, outputPer1M = 2.50, 10.0
	case "gpt-4o-mini":
		inputPer1M, outputPer1M = 0.15, 0.60
	case "o1":
		inputPer1M, outputPer1M = 15.0, 60.0
	default:
		inputPer1M, outputPer1M = 2.50, 10.0
	}
	return (float64(inputTokens)/1_000_000)*inputPer1M + (float64(outputTokens)/1_000_000)*outputPer1M
}
