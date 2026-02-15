package otel

import (
	"context"
	"testing"
)

func TestSetup_EmptyEndpoint(t *testing.T) {
	p, err := Setup(context.Background(), Config{})
	if err != nil {
		t.Fatalf("Setup with empty endpoint: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if p.TracerProvider() != nil {
		t.Error("expected nil TracerProvider for no-op")
	}
	if p.MeterProvider() != nil {
		t.Error("expected nil MeterProvider for no-op")
	}
	// Shutdown should be safe.
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestSetup_ValidConfig(t *testing.T) {
	// Use a non-routable endpoint — we just test that providers are created.
	// The exporters won't actually connect in test, but the providers initialize.
	cfg := Config{
		Endpoint:    "localhost:14318",
		ServiceName: "test-agent",
		SampleRate:  0.5,
	}
	p, err := Setup(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if p.TracerProvider() == nil {
		t.Error("expected non-nil TracerProvider")
	}
	if p.MeterProvider() == nil {
		t.Error("expected non-nil MeterProvider")
	}
	// Shutdown may return connection errors since no real endpoint exists.
	// We only verify that it doesn't panic.
	_ = p.Shutdown(context.Background())
}

func TestShutdown_Idempotent(t *testing.T) {
	p, _ := Setup(context.Background(), Config{})
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("first Shutdown: %v", err)
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
}

func TestShutdown_NilProvider(t *testing.T) {
	var p *Provider
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("nil Shutdown: %v", err)
	}
}
