//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/platinummonkey/borg/pkg/ircclient"
)

// TestIRCIntegration_FullLifecycle tests against a real Ergo IRC server running in Docker.
// Run with: go test -tags=integration ./test/integration/...
//
// Prerequisites:
//   cd deploy/irc-server && docker-compose up -d
//   Register a test account on the Ergo server.
func TestIRCIntegration_FullLifecycle(t *testing.T) {
	server := os.Getenv("IRC_TEST_SERVER")
	if server == "" {
		server = "localhost:6697"
	}
	username := os.Getenv("IRC_TEST_USERNAME")
	if username == "" {
		t.Skip("IRC_TEST_USERNAME not set; skipping integration test")
	}
	password := os.Getenv("IRC_TEST_PASSWORD")
	if password == "" {
		t.Skip("IRC_TEST_PASSWORD not set; skipping integration test")
	}

	cfg := ircclient.Config{
		Server:                server,
		Nick:                  "integration-test",
		Username:              username,
		Password:              password,
		TLS:                   true,
		TLSInsecureSkipVerify: true,
		SASL:                  true,
		SASLMech:              "PLAIN",
		Reconnect:             false,
		PingFrequency:         2 * time.Minute,
		Timeout:               30 * time.Second,
		QuitMessage:           "integration test done",
	}

	client, err := ircclient.NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Connect.
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Disconnect()

	t.Log("connected to", server)

	// Join a channel.
	client.Join("#integration-test")
	time.Sleep(1 * time.Second)

	channels := client.JoinedChannels()
	found := false
	for _, ch := range channels {
		if ch == "#integration-test" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected to join #integration-test, channels: %v", channels)
	}

	// Send a message.
	client.SendMessage("#integration-test", "HELLO from integration test")
	time.Sleep(500 * time.Millisecond)

	// Part and disconnect.
	client.Part("#integration-test")
	time.Sleep(500 * time.Millisecond)

	client.Disconnect()
	t.Log("integration test complete")
}
