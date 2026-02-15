package spawner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DockerSpawner launches agent processes in Docker containers.
type DockerSpawner struct{}

// NewDockerSpawner creates a DockerSpawner.
func NewDockerSpawner() *DockerSpawner { return &DockerSpawner{} }

func (s *DockerSpawner) Type() string { return "docker" }

func (s *DockerSpawner) PreSpawn(_ context.Context, cfg SpawnConfig) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("docker: %w", err)
	}
	if cfg.DockerImage == "" {
		return fmt.Errorf("docker: docker_image is required")
	}
	return nil
}

func (s *DockerSpawner) Spawn(ctx context.Context, cfg SpawnConfig) (*AgentInstance, error) {
	dockerArgs := buildDockerRunArgs(cfg)
	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker spawn: %w: %s", err, errOut.String())
	}

	containerID := strings.TrimSpace(out.String())
	if len(containerID) > 12 {
		containerID = containerID[:12]
	}

	inst := &AgentInstance{
		ID:          fmt.Sprintf("docker-%s-%s", cfg.Nick, containerID),
		Nick:        cfg.Nick,
		SpawnerType: "docker",
		ContainerID: containerID,
		Config:      cfg,
		Status:      StatusStarting,
		StartedAt:   time.Now(),
	}
	if cfg.DashboardAddr != "" {
		inst.DashboardURL = fmt.Sprintf("http://%s%s", containerID, cfg.DashboardAddr)
	}

	return inst, nil
}

func (s *DockerSpawner) PostSpawn(_ context.Context, inst *AgentInstance) error {
	inst.Status = StatusRunning
	return nil
}

func (s *DockerSpawner) PreStop(ctx context.Context, inst *AgentInstance) error {
	inst.Status = StatusStopping
	return nil
}

func (s *DockerSpawner) Stop(ctx context.Context, inst *AgentInstance) error {
	if inst.ContainerID == "" {
		inst.Status = StatusStopped
		inst.StoppedAt = time.Now()
		return nil
	}

	cmd := exec.CommandContext(ctx, "docker", "stop", inst.ContainerID)
	_ = cmd.Run()

	cmd = exec.CommandContext(ctx, "docker", "rm", inst.ContainerID)
	_ = cmd.Run()

	inst.Status = StatusStopped
	inst.StoppedAt = time.Now()
	return nil
}

func (s *DockerSpawner) PostStop(_ context.Context, inst *AgentInstance) error {
	return nil
}

func (s *DockerSpawner) Status(ctx context.Context, inst *AgentInstance) (InstanceStatus, error) {
	if inst.ContainerID == "" {
		return StatusStopped, nil
	}

	cmd := exec.CommandContext(ctx, "docker", "inspect",
		"--format={{.State.Running}}", inst.ContainerID)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return StatusStopped, nil
	}

	if strings.TrimSpace(out.String()) == "true" {
		return StatusRunning, nil
	}
	return StatusStopped, nil
}

func buildDockerRunArgs(cfg SpawnConfig) []string {
	args := []string{"run", "-d", "--name", cfg.Nick}

	if cfg.DockerNetwork != "" {
		args = append(args, "--network", cfg.DockerNetwork)
	}

	args = append(args, cfg.DockerImage)
	args = append(args, BuildCLIArgs(cfg)...)
	return args
}

// BuildDockerRunCommand constructs the full docker run command for testing/inspection.
func BuildDockerRunCommand(cfg SpawnConfig) []string {
	return append([]string{"docker"}, buildDockerRunArgs(cfg)...)
}
