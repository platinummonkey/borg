package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Config holds OpenTelemetry configuration.
type Config struct {
	Endpoint    string  `yaml:"endpoint"`     // OTLP HTTP endpoint (empty = disabled)
	ServiceName string  `yaml:"service_name"` // default "agent-chat"
	SampleRate  float64 `yaml:"sample_rate"`  // 0.0–1.0, default 1.0
}

// Provider holds the OTel trace and metric providers.
type Provider struct {
	tp *sdktrace.TracerProvider
	mp *sdkmetric.MeterProvider
}

// Setup initializes OTel providers. An empty endpoint returns a no-op provider.
func Setup(ctx context.Context, cfg Config) (*Provider, error) {
	if cfg.Endpoint == "" {
		return &Provider{}, nil
	}

	if cfg.ServiceName == "" {
		cfg.ServiceName = "agent-chat"
	}
	if cfg.SampleRate <= 0 || cfg.SampleRate > 1.0 {
		cfg.SampleRate = 1.0
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	// Trace exporter.
	traceExp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(cfg.Endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("otel trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(cfg.SampleRate)),
	)
	otel.SetTracerProvider(tp)

	// Metric exporter.
	metricExp, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpoint(cfg.Endpoint),
		otlpmetrichttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("otel metric exporter: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	return &Provider{tp: tp, mp: mp}, nil
}

// Shutdown flushes and shuts down the providers.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}
	var firstErr error
	if p.tp != nil {
		if err := p.tp.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if p.mp != nil {
		if err := p.mp.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// TracerProvider returns the underlying TracerProvider, or nil if disabled.
func (p *Provider) TracerProvider() *sdktrace.TracerProvider {
	if p == nil {
		return nil
	}
	return p.tp
}

// MeterProvider returns the underlying MeterProvider, or nil if disabled.
func (p *Provider) MeterProvider() *sdkmetric.MeterProvider {
	if p == nil {
		return nil
	}
	return p.mp
}
