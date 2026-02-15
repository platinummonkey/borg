package agent

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/platinummonkey/borg/internal/agent"

// tracer returns the package-level OTel tracer.
func tracer() trace.Tracer {
	return otel.Tracer(tracerName)
}

// startSpan creates a new span. Returns the context and span; caller must call span.End().
func startSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return tracer().Start(ctx, name, trace.WithAttributes(attrs...))
}

// protocolAttrs returns common attributes for protocol message spans.
func protocolAttrs(action, channel, nick string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("protocol.action", action),
		attribute.String("protocol.channel", channel),
		attribute.String("protocol.nick", nick),
	}
}
