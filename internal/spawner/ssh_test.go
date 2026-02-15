package spawner

import (
	"context"
	"testing"
)

func TestSSHSpawner_PreSpawn_Validation(t *testing.T) {
	s := NewSSHSpawner()

	tests := []struct {
		name    string
		cfg     SpawnConfig
		wantErr bool
	}{
		{
			name: "valid",
			cfg: SpawnConfig{
				Nick: "a", Server: "s", Username: "u", Password: "p",
				SSHHost: "host", SSHUser: "user", BinaryPath: "/usr/bin/borg",
			},
		},
		{
			name: "missing ssh_host",
			cfg: SpawnConfig{
				Nick: "a", Server: "s", Username: "u", Password: "p",
				SSHUser: "user", BinaryPath: "/usr/bin/borg",
			},
			wantErr: true,
		},
		{
			name: "missing ssh_user",
			cfg: SpawnConfig{
				Nick: "a", Server: "s", Username: "u", Password: "p",
				SSHHost: "host", BinaryPath: "/usr/bin/borg",
			},
			wantErr: true,
		},
		{
			name: "missing binary_path",
			cfg: SpawnConfig{
				Nick: "a", Server: "s", Username: "u", Password: "p",
				SSHHost: "host", SSHUser: "user",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.PreSpawn(context.Background(), tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("PreSpawn() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuildSSHCommand(t *testing.T) {
	cfg := SpawnConfig{
		SSHHost:    "10.0.0.1",
		SSHUser:    "deploy",
		SSHKeyPath: "/home/deploy/.ssh/id_ed25519",
	}

	cmd := BuildSSHCommand(cfg, "borg --nick test")

	// Verify structure.
	if cmd[0] != "ssh" {
		t.Errorf("cmd[0] = %q, want ssh", cmd[0])
	}

	// Find the key arg.
	foundKey := false
	for i, arg := range cmd {
		if arg == "-i" && i+1 < len(cmd) {
			if cmd[i+1] != "/home/deploy/.ssh/id_ed25519" {
				t.Errorf("key path = %q, want %q", cmd[i+1], "/home/deploy/.ssh/id_ed25519")
			}
			foundKey = true
		}
	}
	if !foundKey {
		t.Error("expected -i flag in SSH command")
	}

	// Check target.
	if cmd[len(cmd)-2] != "deploy@10.0.0.1" {
		t.Errorf("target = %q, want %q", cmd[len(cmd)-2], "deploy@10.0.0.1")
	}
	if cmd[len(cmd)-1] != "borg --nick test" {
		t.Errorf("remote cmd = %q, want %q", cmd[len(cmd)-1], "borg --nick test")
	}
}

func TestSSHSpawner_Type(t *testing.T) {
	s := NewSSHSpawner()
	if s.Type() != "ssh" {
		t.Errorf("type = %q, want %q", s.Type(), "ssh")
	}
}
