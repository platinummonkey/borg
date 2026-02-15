package spawner

import (
	"context"
	"fmt"
	"time"
)

// InstanceStatus represents the lifecycle state of a spawned agent.
type InstanceStatus string

const (
	StatusPending  InstanceStatus = "pending"
	StatusStarting InstanceStatus = "starting"
	StatusRunning  InstanceStatus = "running"
	StatusStopping InstanceStatus = "stopping"
	StatusStopped  InstanceStatus = "stopped"
	StatusFailed   InstanceStatus = "failed"
)

// SpawnConfig holds all parameters needed to spawn an agent.
type SpawnConfig struct {
	// IRC connection.
	Nick     string `json:"nick"`
	Server   string `json:"server"`
	Username string `json:"username"`
	Password string `json:"password"`

	// Agent config.
	Channels     []string `json:"channels"`
	Capabilities []string `json:"capabilities"`
	Roles        []string `json:"roles"`

	// Dashboard.
	DashboardAddr string `json:"dashboard_addr"`

	// Extra CLI flags passed as --key=value.
	ExtraFlags map[string]string `json:"extra_flags,omitempty"`

	// Spawner-specific fields.
	BinaryPath    string `json:"binary_path,omitempty"`    // Local
	SSHHost       string `json:"ssh_host,omitempty"`       // SSH
	SSHUser       string `json:"ssh_user,omitempty"`       // SSH
	SSHKeyPath    string `json:"ssh_key_path,omitempty"`   // SSH
	DockerImage   string `json:"docker_image,omitempty"`   // Docker
	DockerNetwork string `json:"docker_network,omitempty"` // Docker
}

// Validate checks that required IRC fields are set.
func (c *SpawnConfig) Validate() error {
	if c.Nick == "" {
		return fmt.Errorf("nick is required")
	}
	if c.Server == "" {
		return fmt.Errorf("server is required")
	}
	if c.Username == "" {
		return fmt.Errorf("username is required")
	}
	if c.Password == "" {
		return fmt.Errorf("password is required")
	}
	return nil
}

// AgentInstance represents a running or stopped agent process.
type AgentInstance struct {
	ID           string         `json:"id"`
	Nick         string         `json:"nick"`
	SpawnerType  string         `json:"spawner_type"`
	Host         string         `json:"host,omitempty"`
	ContainerID  string         `json:"container_id,omitempty"`
	DashboardURL string         `json:"dashboard_url,omitempty"`
	Error        string         `json:"error,omitempty"`
	Config       SpawnConfig    `json:"config"`
	Status       InstanceStatus `json:"status"`
	PID          int            `json:"pid,omitempty"`
	StartedAt    time.Time      `json:"started_at"`
	StoppedAt    time.Time      `json:"stopped_at,omitempty"`
}

// Spawner defines the lifecycle interface for agent process management.
type Spawner interface {
	// Type returns the spawner type identifier (e.g. "local", "ssh", "docker").
	Type() string

	// PreSpawn validates the config and prepares the environment.
	PreSpawn(ctx context.Context, cfg SpawnConfig) error

	// Spawn launches the agent process and returns an instance.
	Spawn(ctx context.Context, cfg SpawnConfig) (*AgentInstance, error)

	// PostSpawn performs post-launch checks (e.g. health polling).
	PostSpawn(ctx context.Context, inst *AgentInstance) error

	// PreStop prepares for shutdown (e.g. sends SIGTERM).
	PreStop(ctx context.Context, inst *AgentInstance) error

	// Stop terminates the agent process.
	Stop(ctx context.Context, inst *AgentInstance) error

	// PostStop cleans up after the process is stopped.
	PostStop(ctx context.Context, inst *AgentInstance) error

	// Status checks whether the agent process is alive.
	Status(ctx context.Context, inst *AgentInstance) (InstanceStatus, error)
}

// BuildCLIArgs constructs CLI arguments from a SpawnConfig, matching
// the flags defined in internal/config/loader.go.
func BuildCLIArgs(cfg SpawnConfig) []string {
	var args []string

	if cfg.Server != "" {
		args = append(args, "--server", cfg.Server)
	}
	if cfg.Nick != "" {
		args = append(args, "--nick", cfg.Nick)
	}
	if cfg.Username != "" {
		args = append(args, "--username", cfg.Username)
	}
	if cfg.Password != "" {
		args = append(args, "--password", cfg.Password)
	}
	if len(cfg.Channels) > 0 {
		args = append(args, "--channels", joinComma(cfg.Channels))
	}
	if len(cfg.Capabilities) > 0 {
		args = append(args, "--capabilities", joinComma(cfg.Capabilities))
	}
	if len(cfg.Roles) > 0 {
		args = append(args, "--roles", joinComma(cfg.Roles))
	}
	if cfg.DashboardAddr != "" {
		args = append(args, "--dashboard-addr", cfg.DashboardAddr)
	}
	for k, v := range cfg.ExtraFlags {
		args = append(args, "--"+k, v)
	}

	return args
}

func joinComma(ss []string) string {
	var result string
	for i, s := range ss {
		if i > 0 {
			result += ","
		}
		result += s
	}
	return result
}
