package spawner

import (
	"context"
	"testing"
)

func TestDockerSpawner_PreSpawn_Validation(t *testing.T) {
	s := NewDockerSpawner()

	tests := []struct {
		name    string
		cfg     SpawnConfig
		wantErr bool
	}{
		{
			name: "valid",
			cfg: SpawnConfig{
				Nick: "a", Server: "s", Username: "u", Password: "p",
				DockerImage: "borg:latest",
			},
		},
		{
			name: "missing docker_image",
			cfg: SpawnConfig{
				Nick: "a", Server: "s", Username: "u", Password: "p",
			},
			wantErr: true,
		},
		{
			name: "missing nick",
			cfg: SpawnConfig{
				Server: "s", Username: "u", Password: "p",
				DockerImage: "borg:latest",
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

func TestBuildDockerRunCommand(t *testing.T) {
	cfg := SpawnConfig{
		Nick:          "agent-1",
		Server:        "irc:6697",
		Username:      "user",
		Password:      "pass",
		Channels:      []string{"#test"},
		DockerImage:   "borg:latest",
		DockerNetwork: "agent-net",
	}

	cmd := BuildDockerRunCommand(cfg)

	if cmd[0] != "docker" {
		t.Errorf("cmd[0] = %q, want docker", cmd[0])
	}
	if cmd[1] != "run" {
		t.Errorf("cmd[1] = %q, want run", cmd[1])
	}
	if cmd[2] != "-d" {
		t.Errorf("cmd[2] = %q, want -d", cmd[2])
	}

	// Check --name.
	foundName := false
	foundNetwork := false
	foundImage := false
	for i, arg := range cmd {
		if arg == "--name" && i+1 < len(cmd) && cmd[i+1] == "agent-1" {
			foundName = true
		}
		if arg == "--network" && i+1 < len(cmd) && cmd[i+1] == "agent-net" {
			foundNetwork = true
		}
		if arg == "borg:latest" {
			foundImage = true
		}
	}
	if !foundName {
		t.Error("expected --name agent-1")
	}
	if !foundNetwork {
		t.Error("expected --network agent-net")
	}
	if !foundImage {
		t.Error("expected image borg:latest")
	}
}

func TestBuildDockerRunCommand_NoNetwork(t *testing.T) {
	cfg := SpawnConfig{
		Nick: "a", Server: "s", Username: "u", Password: "p",
		DockerImage: "img:v1",
	}

	cmd := BuildDockerRunCommand(cfg)
	for _, arg := range cmd {
		if arg == "--network" {
			t.Error("unexpected --network flag when DockerNetwork is empty")
		}
	}
}

func TestDockerSpawner_Type(t *testing.T) {
	s := NewDockerSpawner()
	if s.Type() != "docker" {
		t.Errorf("type = %q, want %q", s.Type(), "docker")
	}
}
