package spawner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSpawnConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     SpawnConfig
		wantErr bool
	}{
		{
			name: "valid",
			cfg: SpawnConfig{
				Nick: "agent-1", Server: "irc:6697",
				Username: "user", Password: "pass",
			},
		},
		{name: "missing nick", cfg: SpawnConfig{Server: "irc:6697", Username: "u", Password: "p"}, wantErr: true},
		{name: "missing server", cfg: SpawnConfig{Nick: "a", Username: "u", Password: "p"}, wantErr: true},
		{name: "missing username", cfg: SpawnConfig{Nick: "a", Server: "s", Password: "p"}, wantErr: true},
		{name: "missing password", cfg: SpawnConfig{Nick: "a", Server: "s", Username: "u"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuildCLIArgs(t *testing.T) {
	cfg := SpawnConfig{
		Nick:          "agent-1",
		Server:        "irc:6697",
		Username:      "user",
		Password:      "pass",
		Channels:      []string{"#a", "#b"},
		Capabilities:  []string{"go", "python"},
		DashboardAddr: ":8080",
	}

	args := BuildCLIArgs(cfg)

	expected := map[string]string{
		"--server":         "irc:6697",
		"--nick":           "agent-1",
		"--username":       "user",
		"--password":       "pass",
		"--channels":       "#a,#b",
		"--capabilities":   "go,python",
		"--dashboard-addr": ":8080",
	}

	argMap := make(map[string]string)
	for i := 0; i < len(args)-1; i += 2 {
		argMap[args[i]] = args[i+1]
	}

	for k, v := range expected {
		if argMap[k] != v {
			t.Errorf("arg %s = %q, want %q", k, argMap[k], v)
		}
	}
}

func TestLocalSpawner_PreSpawn_NoBinary(t *testing.T) {
	s := NewLocalSpawner()
	cfg := SpawnConfig{
		Nick: "a", Server: "s", Username: "u", Password: "p",
		BinaryPath: "/nonexistent/binary",
	}
	err := s.PreSpawn(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}
}

func TestLocalSpawner_PreSpawn_MissingBinaryPath(t *testing.T) {
	s := NewLocalSpawner()
	cfg := SpawnConfig{
		Nick: "a", Server: "s", Username: "u", Password: "p",
	}
	err := s.PreSpawn(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for missing binary path")
	}
}

func TestLocalSpawner_PreSpawn_ValidBinary(t *testing.T) {
	// Use a real binary that exists.
	bin, err := os.Executable()
	if err != nil {
		t.Skip("cannot determine executable")
	}
	s := NewLocalSpawner()
	cfg := SpawnConfig{
		Nick: "a", Server: "s", Username: "u", Password: "p",
		BinaryPath: bin,
	}
	if err := s.PreSpawn(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLocalSpawner_SpawnAndStop(t *testing.T) {
	// Spawn "sleep" as a test process.
	sleepBin := "/bin/sleep"
	if _, err := os.Stat(sleepBin); err != nil {
		t.Skip("sleep binary not found")
	}

	s := NewLocalSpawner()
	cfg := SpawnConfig{
		Nick: "test-agent", Server: "irc:6697",
		Username: "u", Password: "p",
		BinaryPath: sleepBin,
		// sleep doesn't understand our flags, but it still starts.
		// We override ExtraFlags to pass the duration.
	}

	// Trick: spawn "sleep 60" by building a temp script.
	dir := t.TempDir()
	script := filepath.Join(dir, "agent.sh")
	os.WriteFile(script, []byte("#!/bin/sh\nsleep 60\n"), 0755)
	cfg.BinaryPath = script

	ctx := context.Background()
	inst, err := s.Spawn(ctx, cfg)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if inst.PID <= 0 {
		t.Fatal("expected positive PID")
	}
	if inst.Status != StatusStarting {
		t.Errorf("status = %q, want %q", inst.Status, StatusStarting)
	}

	// Check status.
	status, err := s.Status(ctx, inst)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status != StatusRunning {
		t.Errorf("status = %q, want %q", status, StatusRunning)
	}

	// Stop.
	_ = s.PreStop(ctx, inst)
	if err := s.Stop(ctx, inst); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if inst.Status != StatusStopped {
		t.Errorf("status after stop = %q, want %q", inst.Status, StatusStopped)
	}
}

func TestLocalSpawner_Type(t *testing.T) {
	s := NewLocalSpawner()
	if s.Type() != "local" {
		t.Errorf("type = %q, want %q", s.Type(), "local")
	}
}
