//go:build podman

package integration

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/platinummonkey/agent-chat/internal/agent"
	"github.com/platinummonkey/agent-chat/internal/config"
	"github.com/platinummonkey/agent-chat/pkg/ircclient"
)

const (
	podmanContainerName = "agent-chat-test-irc"
	ergoImage           = "ghcr.io/ergochat/ergo:stable"
)

// podmanEnv holds the shared test infrastructure for Podman E2E tests.
type podmanEnv struct {
	t       *testing.T
	port    int
	tmpDir  string
	cleanup func()
}

// setupPodman starts an Ergo IRC server in a Podman container.
// It generates self-signed TLS certs and copies the test config.
func setupPodman(t *testing.T) *podmanEnv {
	t.Helper()

	// Check podman availability.
	if _, err := exec.LookPath("podman"); err != nil {
		t.Skip("podman not found, skipping podman E2E test")
	}

	tmpDir := t.TempDir()

	// Generate self-signed TLS certs.
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")
	generateCerts(t, certPath, keyPath)

	// Copy Ergo config to tmpDir.
	srcConfig := filepath.Join("testdata", "ircd_test.yaml")
	configData, err := os.ReadFile(srcConfig)
	if err != nil {
		t.Fatalf("read test config: %v", err)
	}
	dstConfig := filepath.Join(tmpDir, "ircd.yaml")
	if err := os.WriteFile(dstConfig, configData, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Stop any leftover container from a previous failed run.
	exec.Command("podman", "rm", "-f", podmanContainerName).Run()

	// Start Ergo in Podman.
	cmd := exec.Command("podman", "run", "-d",
		"--name", podmanContainerName,
		"-p", "0:6697",
		"-v", certPath+":/tls/cert.pem:ro",
		"-v", keyPath+":/tls/key.pem:ro",
		"-v", dstConfig+":/ircd.yaml:ro",
		ergoImage,
		"run", "--conf", "/ircd.yaml",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("podman run failed: %v\noutput: %s", err, out)
	}

	// Get the mapped port.
	portCmd := exec.Command("podman", "port", podmanContainerName, "6697")
	portOut, err := portCmd.Output()
	if err != nil {
		teardownPodman(t)
		t.Fatalf("podman port failed: %v", err)
	}

	// Parse "0.0.0.0:XXXXX" or ":::XXXXX"
	portStr := strings.TrimSpace(string(portOut))
	parts := strings.Split(portStr, "\n")
	hostPort := ""
	for _, line := range parts {
		line = strings.TrimSpace(line)
		if idx := strings.LastIndex(line, ":"); idx >= 0 {
			hostPort = line[idx+1:]
			break
		}
	}
	if hostPort == "" {
		teardownPodman(t)
		t.Fatalf("could not parse port from: %q", portStr)
	}

	port := 0
	fmt.Sscanf(hostPort, "%d", &port)
	if port == 0 {
		teardownPodman(t)
		t.Fatalf("invalid port: %q", hostPort)
	}

	// Wait for Ergo to be ready (poll TLS connect).
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := tls.DialWithDialer(
			&net.Dialer{Timeout: 1 * time.Second},
			"tcp",
			addr,
			&tls.Config{InsecureSkipVerify: true},
		)
		if err == nil {
			conn.Close()
			goto ready
		}
		time.Sleep(500 * time.Millisecond)
	}
	teardownPodman(t)
	t.Fatalf("Ergo did not become ready at %s within 15s", addr)

ready:
	t.Logf("Ergo ready at %s", addr)

	// Register test accounts using a raw IRC client.
	registerAccount(t, addr, "alice", "alicepass")
	registerAccount(t, addr, "bob", "bobpass")

	return &podmanEnv{
		t:      t,
		port:   port,
		tmpDir: tmpDir,
		cleanup: func() {
			teardownPodman(t)
		},
	}
}

func teardownPodman(t *testing.T) {
	t.Helper()
	exec.Command("podman", "stop", podmanContainerName).Run()
	exec.Command("podman", "rm", "-f", podmanContainerName).Run()
}

// registerAccount connects to the Ergo server and registers a NickServ account.
func registerAccount(t *testing.T, addr, username, password string) {
	t.Helper()

	cfg := ircclient.Config{
		Server:                addr,
		Nick:                  username,
		Username:              username,
		Password:              password,
		RealName:              "Registrar",
		TLS:                   true,
		TLSInsecureSkipVerify: true,
		SASL:                  false,
		Reconnect:             false,
	}

	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 5 * time.Second},
		"tcp",
		addr,
		&tls.Config{InsecureSkipVerify: true},
	)
	if err != nil {
		t.Fatalf("dial for registration: %v", err)
	}
	defer conn.Close()

	// Simple raw IRC registration sequence.
	send := func(msg string) {
		conn.Write([]byte(msg + "\r\n"))
	}

	send(fmt.Sprintf("NICK %s", cfg.Nick))
	send(fmt.Sprintf("USER %s 0 * :%s", cfg.Username, cfg.RealName))
	time.Sleep(1 * time.Second) // wait for welcome

	// Register with NickServ.
	send(fmt.Sprintf("PRIVMSG NickServ :REGISTER %s *", password))
	time.Sleep(1 * time.Second)

	send("QUIT :done")
	t.Logf("registered account %s", username)
}

// generateCerts creates self-signed TLS certs for testing.
func generateCerts(t *testing.T, certPath, keyPath string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}

func (pe *podmanEnv) addr() string {
	return fmt.Sprintf("127.0.0.1:%d", pe.port)
}

func (pe *podmanEnv) createAgent(nick, username, password string, channels []string, opts ...func(*config.AppConfig)) *agent.Agent {
	pe.t.Helper()

	cfg := &config.AppConfig{
		IRC: ircclient.Config{
			Server:                pe.addr(),
			Nick:                  nick,
			Username:              username,
			Password:              password,
			RealName:              "Test Agent",
			TLS:                   true,
			TLSInsecureSkipVerify: true,
			SASL:                  true,
			SASLMech:              "PLAIN",
			Channels:              channels,
			Reconnect:             false,
		},
		LogLevel: "error",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	a, err := agent.New(cfg)
	if err != nil {
		pe.t.Fatalf("New agent %s: %v", nick, err)
	}
	return a
}

// TestPodman_MultiAgentProtocol runs the basic multi-agent protocol scenario
// against a real Ergo IRC server.
func TestPodman_MultiAgentProtocol(t *testing.T) {
	pe := setupPodman(t)
	defer pe.cleanup()

	alice := pe.createAgent("alice-1", "alice", "alicepass", []string{"#project"})
	bob := pe.createAgent("bob-2", "bob", "bobpass", []string{"#project"})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := alice.Start(ctx); err != nil {
		t.Fatalf("alice Start: %v", err)
	}
	defer alice.Shutdown()

	if err := bob.Start(ctx); err != nil {
		t.Fatalf("bob Start: %v", err)
	}
	defer bob.Shutdown()

	time.Sleep(1 * time.Second) // real server needs more time

	// Alice starts a task.
	if err := alice.AnnounceStarted("#project", "auth-refactor", "high", "feature"); err != nil {
		t.Fatalf("AnnounceStarted: %v", err)
	}
	time.Sleep(1 * time.Second)

	// Bob should see the task.
	task := bob.State().GetTask("auth-refactor")
	if task == nil {
		t.Fatal("bob does not have auth-refactor task")
	}
	if task.Status != agent.TaskStatusStarted {
		t.Errorf("task status = %q, want started", task.Status)
	}

	// Alice completes it.
	if err := alice.AnnounceCompleted("#project", "auth-refactor", "ready-for-testing"); err != nil {
		t.Fatalf("AnnounceCompleted: %v", err)
	}
	time.Sleep(1 * time.Second)

	task = bob.State().GetTask("auth-refactor")
	if task == nil {
		t.Fatal("bob lost auth-refactor task")
	}
	if task.Status != agent.TaskStatusCompleted {
		t.Errorf("task status = %q, want completed", task.Status)
	}

	// Context sharing.
	if err := alice.ShareContext("#project", "auth", "webapp", "refactored"); err != nil {
		t.Fatalf("ShareContext: %v", err)
	}
	time.Sleep(1 * time.Second)

	entry := bob.ContextEntries().Get("auth")
	if entry == nil {
		t.Fatal("bob context missing auth entry")
	}
	if entry.Project != "webapp" {
		t.Errorf("project = %q, want webapp", entry.Project)
	}
}

// TestPodman_Persistence verifies state persistence and recovery against a real server.
func TestPodman_Persistence(t *testing.T) {
	pe := setupPodman(t)
	defer pe.cleanup()

	stateFile := filepath.Join(t.TempDir(), "state.json")

	alice := pe.createAgent("alice-1", "alice", "alicepass", []string{"#project"},
		func(cfg *config.AppConfig) {
			cfg.StateFile = stateFile
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := alice.Start(ctx); err != nil {
		t.Fatalf("alice Start: %v", err)
	}

	time.Sleep(1 * time.Second)

	if err := alice.AnnounceStarted("#project", "persist-task", "high"); err != nil {
		t.Fatalf("AnnounceStarted: %v", err)
	}
	if err := alice.AnnounceCompleted("#project", "persist-task"); err != nil {
		t.Fatalf("AnnounceCompleted: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	alice.Shutdown()

	// Verify state file.
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}

	var ps agent.PersistedState
	if err := json.Unmarshal(data, &ps); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	if _, ok := ps.Tasks["persist-task"]; !ok {
		t.Errorf("persist-task not found in saved state: %+v", ps.Tasks)
	}

	// Restore with new agent.
	alice2 := pe.createAgent("alice-2", "alice", "alicepass", []string{"#project"},
		func(cfg *config.AppConfig) {
			cfg.StateFile = stateFile
		},
	)

	if err := alice2.Start(ctx); err != nil {
		t.Fatalf("alice2 Start: %v", err)
	}
	defer alice2.Shutdown()

	time.Sleep(1 * time.Second)

	task := alice2.State().GetTask("persist-task")
	if task == nil {
		t.Fatal("restored agent missing persist-task")
	}
	if task.Status != agent.TaskStatusCompleted {
		t.Errorf("persist-task status = %q, want completed", task.Status)
	}
}

// TestPodman_Discovery verifies the DISCOVER/CAPABILITIES exchange against a real server.
func TestPodman_Discovery(t *testing.T) {
	pe := setupPodman(t)
	defer pe.cleanup()

	alice := pe.createAgent("alice-1", "alice", "alicepass", []string{"#project"},
		func(cfg *config.AppConfig) {
			cfg.Capabilities = []string{"database", "testing"}
			cfg.DiscoveryTTL = 30 * time.Second
		},
	)
	bob := pe.createAgent("bob-2", "bob", "bobpass", []string{"#project"},
		func(cfg *config.AppConfig) {
			cfg.Capabilities = []string{"frontend", "database"}
			cfg.DiscoveryTTL = 30 * time.Second
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := alice.Start(ctx); err != nil {
		t.Fatalf("alice Start: %v", err)
	}
	defer alice.Shutdown()

	if err := bob.Start(ctx); err != nil {
		t.Fatalf("bob Start: %v", err)
	}
	defer bob.Shutdown()

	// Wait for heartbeats.
	time.Sleep(2 * time.Second)

	// Alice should know about bob via heartbeat.
	known := alice.KnownAgents()
	foundBob := false
	for _, a := range known {
		if a.Nick == "bob-2" {
			foundBob = true
		}
	}
	if !foundBob {
		nicks := make([]string, len(known))
		for i, a := range known {
			nicks[i] = a.Nick
		}
		t.Errorf("alice does not know bob-2, known: %v", nicks)
	}

	// DISCOVER exchange.
	if err := alice.Discover("#project", "database"); err != nil {
		t.Fatalf("alice Discover: %v", err)
	}
	time.Sleep(2 * time.Second)

	// After DISCOVER, alice should still know bob (refreshed).
	known = alice.KnownAgents()
	var dbAgents []string
	for _, a := range known {
		for _, e := range a.Expertise {
			if strings.EqualFold(e, "database") {
				dbAgents = append(dbAgents, a.Nick)
				break
			}
		}
	}
	if len(dbAgents) == 0 {
		t.Error("no agents with database expertise found")
	}
}
