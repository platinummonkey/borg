//go:build integration

package integration

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/platinummonkey/borg/internal/config"
	"github.com/platinummonkey/borg/pkg/ircclient"
	"github.com/platinummonkey/borg/test/mock"
)

// TestE2E_Federation_TwoServers verifies that the federation manager correctly
// relays messages between two IRC servers and prevents relay loops.
func TestE2E_Federation_TwoServers(t *testing.T) {
	// Start two independent mock IRC servers.
	srv1, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("server1: %v", err)
	}
	defer srv1.Close()

	srv2, err := mock.NewIRCServer()
	if err != nil {
		t.Fatalf("server2: %v", err)
	}
	defer srv2.Close()

	// Register accounts.
	srv1.Accounts["hub"] = "hubpass"
	srv1.Accounts["alice"] = "pass1"
	srv2.Accounts["hubfed"] = "hubfedpass"
	srv2.Accounts["bob"] = "pass2"

	// Hub agent on server1 with federation link to server2.
	hub := createTestAgent(t, srv1, "hub-1", "hub", "hubpass", []string{"#project"},
		func(cfg *config.AppConfig) {
			cfg.FederationServers = []config.FederationServerConfig{
				{
					Name: "remote",
					IRC: ircclient.Config{
						Server:                srv2.Addr(),
						Nick:                  "hub-fed",
						Username:              "hubfed",
						Password:              "hubfedpass",
						RealName:              "Hub Federation",
						TLS:                   true,
						TLSInsecureSkipVerify: true,
						SASL:                  true,
						Channels:              []string{"#project"},
					},
				},
			}
			cfg.FederationMappings = []config.ChannelMapping{
				{
					LocalChannel:  "#project",
					RemoteChannel: "#project",
					LinkName:      "remote",
				},
			}
		},
	)

	// Alice: regular agent on server1 (observer for relayed messages).
	alice := createTestAgent(t, srv1, "alice-1", "alice", "pass1", []string{"#project"})

	// Bob: regular agent on server2.
	bob := createTestAgent(t, srv2, "bob-2", "bob", "pass2", []string{"#project"})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Start hub first (includes federation setup).
	if err := hub.Start(ctx); err != nil {
		t.Fatalf("hub Start: %v", err)
	}
	defer hub.Shutdown()

	if err := alice.Start(ctx); err != nil {
		t.Fatalf("alice Start: %v", err)
	}
	defer alice.Shutdown()

	if err := bob.Start(ctx); err != nil {
		t.Fatalf("bob Start: %v", err)
	}
	defer bob.Shutdown()

	// Wait for all channels to be joined and federation link to connect.
	time.Sleep(800 * time.Millisecond)

	// Track raw messages on server1 and server2 via observer clients.
	var aliceRaw []string
	var bobRaw []string
	var mu sync.Mutex

	// Create observer clients to capture raw messages on each server.
	obsCfg := ircclient.Config{
		Server:                srv1.Addr(),
		Nick:                  "obs-1",
		Username:              "alice", // reuse alice's credentials
		Password:              "pass1",
		RealName:              "Observer",
		TLS:                   true,
		TLSInsecureSkipVerify: true,
		SASL:                  true,
		Channels:              []string{"#project"},
	}
	// We need a separate account for the observer.
	srv1.Accounts["obs"] = "obspass"
	obsCfg.Username = "obs"
	obsCfg.Password = "obspass"

	obsClient, err := ircclient.NewClient(obsCfg)
	if err != nil {
		t.Fatalf("observer NewClient: %v", err)
	}
	obsClient.OnMessage(func(ev ircclient.MessageEvent) {
		mu.Lock()
		aliceRaw = append(aliceRaw, ev.Message)
		mu.Unlock()
	})
	if err := obsClient.Connect(ctx); err != nil {
		t.Fatalf("observer Connect: %v", err)
	}
	defer obsClient.Disconnect()
	time.Sleep(400 * time.Millisecond)

	// Create observer on server2 to track relayed messages from server1.
	srv2.Accounts["obs2"] = "obs2pass"
	obs2Cfg := ircclient.Config{
		Server:                srv2.Addr(),
		Nick:                  "obs-2",
		Username:              "obs2",
		Password:              "obs2pass",
		RealName:              "Observer2",
		TLS:                   true,
		TLSInsecureSkipVerify: true,
		SASL:                  true,
		Channels:              []string{"#project"},
	}
	obs2Client, err := ircclient.NewClient(obs2Cfg)
	if err != nil {
		t.Fatalf("observer2 NewClient: %v", err)
	}
	obs2Client.OnMessage(func(ev ircclient.MessageEvent) {
		mu.Lock()
		bobRaw = append(bobRaw, ev.Message)
		mu.Unlock()
	})
	if err := obs2Client.Connect(ctx); err != nil {
		t.Fatalf("observer2 Connect: %v", err)
	}
	defer obs2Client.Disconnect()
	time.Sleep(400 * time.Millisecond)

	// Clear captured messages from join noise.
	mu.Lock()
	aliceRaw = nil
	bobRaw = nil
	mu.Unlock()

	// Bob sends another STARTED on server2.
	if err := bob.AnnounceStarted("#project", "fed-task-1", "high"); err != nil {
		t.Fatalf("bob AnnounceStarted: %v", err)
	}
	time.Sleep(600 * time.Millisecond)

	// Observer on server1 should see the federated relay.
	mu.Lock()
	foundRemoteRelay := false
	for _, raw := range aliceRaw {
		if strings.Contains(raw, "[fed:remote]") && strings.Contains(raw, "bob-2") && strings.Contains(raw, "STARTED") {
			foundRemoteRelay = true
			break
		}
	}
	mu.Unlock()

	if !foundRemoteRelay {
		mu.Lock()
		t.Errorf("remote→local relay not found on server1.\naliceRaw: %v", aliceRaw)
		mu.Unlock()
	}

	// --- Test 2: Local → Remote relay ---
	// Alice sends COMPLETED on server1 #project.
	if err := alice.AnnounceCompleted("#project", "local-task-1"); err != nil {
		t.Fatalf("alice AnnounceCompleted: %v", err)
	}
	time.Sleep(600 * time.Millisecond)

	// Observer on server2 should see the federated relay with [fed:local] prefix.
	mu.Lock()
	foundLocalRelay := false
	for _, raw := range bobRaw {
		if strings.Contains(raw, "[fed:local]") && strings.Contains(raw, "alice-1") && strings.Contains(raw, "COMPLETED") {
			foundLocalRelay = true
			break
		}
	}
	mu.Unlock()

	if !foundLocalRelay {
		mu.Lock()
		t.Errorf("local→remote relay not found on server2.\nbobRaw: %v", bobRaw)
		mu.Unlock()
	}

	// --- Test 3: Loop prevention ---
	// Send an already-federated message on server1.
	// It should NOT be re-relayed to server2.
	mu.Lock()
	bobRawBefore := len(bobRaw)
	mu.Unlock()

	// Directly send a federated-looking message via the observer on server1.
	obsClient.SendMessage("#project", "[fed:local] <someone> already-relayed-msg")
	time.Sleep(600 * time.Millisecond)

	mu.Lock()
	newRelayed := false
	for i := bobRawBefore; i < len(bobRaw); i++ {
		if strings.Contains(bobRaw[i], "already-relayed-msg") {
			newRelayed = true
			break
		}
	}
	mu.Unlock()

	if newRelayed {
		t.Error("federated message was re-relayed (loop prevention failed)")
	}
}
