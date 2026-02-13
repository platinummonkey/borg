package ircclient

import (
	"context"
	"testing"
	"time"

	"github.com/platinummonkey/agent-chat/test/mock"
)

func validTestConfig(addr string) Config {
	return Config{
		Server:                addr,
		Nick:                  "testnick",
		Username:              "testuser",
		Password:              "testpass",
		TLS:                   true,
		TLSInsecureSkipVerify: true,
		SASL:                  true,
		SASLMech:              "PLAIN",
		Reconnect:             false,
		PingFrequency:         5 * time.Minute,
		Timeout:               10 * time.Second,
		QuitMessage:           "bye",
	}
}

func TestNewClient_ValidConfig(t *testing.T) {
	cfg := validTestConfig("localhost:6697")
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewClient_InvalidConfig(t *testing.T) {
	cfg := Config{} // missing everything
	_, err := NewClient(cfg)
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
}

func TestConnectAndJoinChannel(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	cfg := validTestConfig(srv.Addr())
	cfg.Channels = []string{"#test"}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Disconnect()

	// Give time for the JOIN callback to fire.
	time.Sleep(500 * time.Millisecond)

	channels := client.JoinedChannels()
	found := false
	for _, ch := range channels {
		if ch == "#test" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to be in #test, joined channels: %v", channels)
	}

	if !client.Connected() {
		t.Error("expected client to be connected")
	}
}

func TestConnectContext_Timeout(t *testing.T) {
	// Connect to an address that won't respond to test context timeout.
	cfg := validTestConfig("192.0.2.1:6697") // RFC 5737 TEST-NET, should timeout
	cfg.Timeout = 1 * time.Second

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = client.Connect(ctx)
	if err == nil {
		t.Fatal("expected error for connection to unreachable address")
		client.Disconnect()
	}
}

func TestOnMessage_Handler(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	cfg := validTestConfig(srv.Addr())

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	received := make(chan MessageEvent, 1)
	client.OnMessage(func(ev MessageEvent) {
		received <- ev
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Disconnect()

	// The mock server echoes PRIVMSG back to us.
	client.SendMessage("#test", "hello world")

	select {
	case ev := <-received:
		if ev.Message != "hello world" {
			t.Errorf("message = %q, want 'hello world'", ev.Message)
		}
	case <-time.After(5 * time.Second):
		t.Error("timed out waiting for message")
	}
}

func TestRemoveHandler(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	cfg := validTestConfig(srv.Addr())

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	called := make(chan struct{}, 1)
	id := client.OnMessage(func(ev MessageEvent) {
		called <- struct{}{}
	})

	// Remove the handler before connecting.
	client.RemoveHandler(id)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Disconnect()

	client.SendMessage("#test", "should not trigger")

	select {
	case <-called:
		t.Error("handler should not have been called after removal")
	case <-time.After(1 * time.Second):
		// Expected — handler was removed.
	}
}

func TestNick(t *testing.T) {
	srv, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	cfg := validTestConfig(srv.Addr())

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Disconnect()

	nick := client.Nick()
	if nick != "testnick" {
		t.Errorf("nick = %q, want testnick", nick)
	}
}
