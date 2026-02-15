package agent

import (
	"context"
	"testing"
	"time"

	"github.com/platinummonkey/borg/internal/config"
	"github.com/platinummonkey/borg/pkg/ircclient"
	"github.com/platinummonkey/borg/test/mock"
)

func testAppConfig(serverAddr string) *config.AppConfig {
	return &config.AppConfig{
		IRC: ircclient.Config{
			Server:                serverAddr,
			Nick:                  "testagent",
			Username:              "testuser",
			Password:              "testpass",
			TLS:                   true,
			TLSInsecureSkipVerify: true,
			SASL:                  true,
			SASLMech:              "PLAIN",
			Reconnect:             false,
			PingFrequency:         5 * time.Minute,
			Timeout:               10 * time.Second,
			QuitMessage:           "test shutdown",
		},
		LogLevel: "info",
		LogFmt:   "text",
	}
}

func TestNew(t *testing.T) {
	cfg := testAppConfig("localhost:6697")
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if a == nil {
		t.Fatal("expected non-nil agent")
	}
}

func TestAgent_StartAndShutdown(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	cfg := testAppConfig(srv.Addr())
	cfg.IRC.Channels = []string{"#test"}

	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give time for joins.
	time.Sleep(500 * time.Millisecond)

	a.Shutdown()
}
