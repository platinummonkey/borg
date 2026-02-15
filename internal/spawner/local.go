package spawner

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// LocalSpawner launches agent processes on the local machine via os/exec.
type LocalSpawner struct{}

// NewLocalSpawner creates a LocalSpawner.
func NewLocalSpawner() *LocalSpawner { return &LocalSpawner{} }

func (s *LocalSpawner) Type() string { return "local" }

func (s *LocalSpawner) PreSpawn(_ context.Context, cfg SpawnConfig) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("local: %w", err)
	}
	if cfg.BinaryPath == "" {
		return fmt.Errorf("local: binary_path is required")
	}
	if _, err := os.Stat(cfg.BinaryPath); err != nil {
		return fmt.Errorf("local: binary not found: %w", err)
	}
	return nil
}

func (s *LocalSpawner) Spawn(ctx context.Context, cfg SpawnConfig) (*AgentInstance, error) {
	args := BuildCLIArgs(cfg)
	cmd := exec.CommandContext(ctx, cfg.BinaryPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Start in its own process group so we can kill it cleanly.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("local: start: %w", err)
	}

	inst := &AgentInstance{
		ID:          fmt.Sprintf("local-%s-%d", cfg.Nick, cmd.Process.Pid),
		Nick:        cfg.Nick,
		SpawnerType: "local",
		Host:        "localhost",
		Config:      cfg,
		Status:      StatusStarting,
		PID:         cmd.Process.Pid,
		StartedAt:   time.Now(),
	}
	if cfg.DashboardAddr != "" {
		inst.DashboardURL = "http://localhost" + cfg.DashboardAddr
	}

	// Wait in background to avoid zombie processes.
	go func() {
		if err := cmd.Wait(); err != nil {
			slog.Debug("agent process exited", "nick", cfg.Nick, "pid", inst.PID, "error", err)
		}
	}()

	return inst, nil
}

func (s *LocalSpawner) PostSpawn(ctx context.Context, inst *AgentInstance) error {
	if inst.DashboardURL == "" {
		inst.Status = StatusRunning
		return nil
	}

	deadline := time.Now().Add(10 * time.Second)
	url := inst.DashboardURL + "/health"

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				inst.Status = StatusRunning
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	inst.Status = StatusRunning // Assume running even without health check
	slog.Warn("post-spawn health check timed out, assuming running", "nick", inst.Nick)
	return nil
}

func (s *LocalSpawner) PreStop(_ context.Context, inst *AgentInstance) error {
	if inst.PID <= 0 {
		return fmt.Errorf("local: no PID for %s", inst.Nick)
	}
	inst.Status = StatusStopping
	proc, err := os.FindProcess(inst.PID)
	if err != nil {
		return nil // process not found is OK
	}
	return proc.Signal(syscall.SIGTERM)
}

func (s *LocalSpawner) Stop(ctx context.Context, inst *AgentInstance) error {
	if inst.PID <= 0 {
		inst.Status = StatusStopped
		inst.StoppedAt = time.Now()
		return nil
	}

	// Wait up to 5 seconds for graceful shutdown.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(inst.PID) {
			inst.Status = StatusStopped
			inst.StoppedAt = time.Now()
			return nil
		}
		select {
		case <-ctx.Done():
			break
		case <-time.After(250 * time.Millisecond):
		}
	}

	// Force kill.
	proc, err := os.FindProcess(inst.PID)
	if err == nil {
		_ = proc.Signal(syscall.SIGKILL)
	}

	inst.Status = StatusStopped
	inst.StoppedAt = time.Now()
	return nil
}

func (s *LocalSpawner) PostStop(_ context.Context, inst *AgentInstance) error {
	return nil
}

func (s *LocalSpawner) Status(_ context.Context, inst *AgentInstance) (InstanceStatus, error) {
	if inst.PID <= 0 {
		return StatusStopped, nil
	}
	if processAlive(inst.PID) {
		return StatusRunning, nil
	}
	return StatusStopped, nil
}

// processAlive checks if a process is alive by sending signal 0.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
