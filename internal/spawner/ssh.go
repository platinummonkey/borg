package spawner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// SSHSpawner launches agent processes on remote hosts via SSH.
type SSHSpawner struct{}

// NewSSHSpawner creates an SSHSpawner.
func NewSSHSpawner() *SSHSpawner { return &SSHSpawner{} }

func (s *SSHSpawner) Type() string { return "ssh" }

func (s *SSHSpawner) PreSpawn(_ context.Context, cfg SpawnConfig) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("ssh: %w", err)
	}
	if cfg.SSHHost == "" {
		return fmt.Errorf("ssh: ssh_host is required")
	}
	if cfg.SSHUser == "" {
		return fmt.Errorf("ssh: ssh_user is required")
	}
	if cfg.BinaryPath == "" {
		return fmt.Errorf("ssh: binary_path is required (remote path)")
	}
	return nil
}

func (s *SSHSpawner) Spawn(ctx context.Context, cfg SpawnConfig) (*AgentInstance, error) {
	agentArgs := BuildCLIArgs(cfg)
	remoteCmd := cfg.BinaryPath + " " + strings.Join(agentArgs, " ")
	// Run in background with nohup.
	remoteCmd = fmt.Sprintf("nohup %s > /dev/null 2>&1 & echo $!", remoteCmd)

	sshArgs := buildSSHArgs(cfg, remoteCmd)
	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ssh spawn: %w: %s", err, out.String())
	}

	pidStr := strings.TrimSpace(out.String())
	pid, _ := strconv.Atoi(pidStr)

	inst := &AgentInstance{
		ID:          fmt.Sprintf("ssh-%s-%s-%d", cfg.SSHHost, cfg.Nick, pid),
		Nick:        cfg.Nick,
		SpawnerType: "ssh",
		Host:        cfg.SSHHost,
		Config:      cfg,
		Status:      StatusStarting,
		PID:         pid,
		StartedAt:   time.Now(),
	}
	if cfg.DashboardAddr != "" {
		inst.DashboardURL = fmt.Sprintf("http://%s%s", cfg.SSHHost, cfg.DashboardAddr)
	}

	return inst, nil
}

func (s *SSHSpawner) PostSpawn(_ context.Context, inst *AgentInstance) error {
	inst.Status = StatusRunning
	return nil
}

func (s *SSHSpawner) PreStop(ctx context.Context, inst *AgentInstance) error {
	if inst.PID <= 0 {
		return nil
	}
	remoteCmd := fmt.Sprintf("kill -TERM %d", inst.PID)
	sshArgs := buildSSHArgs(inst.Config, remoteCmd)
	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	_ = cmd.Run()
	inst.Status = StatusStopping
	return nil
}

func (s *SSHSpawner) Stop(ctx context.Context, inst *AgentInstance) error {
	if inst.PID <= 0 {
		inst.Status = StatusStopped
		inst.StoppedAt = time.Now()
		return nil
	}

	// Check if still alive after SIGTERM.
	status, _ := s.Status(ctx, inst)
	if status == StatusRunning {
		remoteCmd := fmt.Sprintf("kill -KILL %d", inst.PID)
		sshArgs := buildSSHArgs(inst.Config, remoteCmd)
		cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
		_ = cmd.Run()
	}

	inst.Status = StatusStopped
	inst.StoppedAt = time.Now()
	return nil
}

func (s *SSHSpawner) PostStop(_ context.Context, inst *AgentInstance) error {
	return nil
}

func (s *SSHSpawner) Status(ctx context.Context, inst *AgentInstance) (InstanceStatus, error) {
	if inst.PID <= 0 {
		return StatusStopped, nil
	}
	remoteCmd := fmt.Sprintf("kill -0 %d 2>/dev/null && echo alive || echo dead", inst.PID)
	sshArgs := buildSSHArgs(inst.Config, remoteCmd)
	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	var out bytes.Buffer
	cmd.Stdout = &out
	_ = cmd.Run()

	if strings.TrimSpace(out.String()) == "alive" {
		return StatusRunning, nil
	}
	return StatusStopped, nil
}

func buildSSHArgs(cfg SpawnConfig, remoteCmd string) []string {
	var args []string
	args = append(args, "-o", "StrictHostKeyChecking=no", "-o", "BatchMode=yes")
	if cfg.SSHKeyPath != "" {
		args = append(args, "-i", cfg.SSHKeyPath)
	}
	target := cfg.SSHHost
	if cfg.SSHUser != "" {
		target = cfg.SSHUser + "@" + cfg.SSHHost
	}
	args = append(args, target, remoteCmd)
	return args
}

// BuildSSHCommand constructs the full SSH command for testing/inspection.
func BuildSSHCommand(cfg SpawnConfig, remoteCmd string) []string {
	return append([]string{"ssh"}, buildSSHArgs(cfg, remoteCmd)...)
}
