package agent

import (
	"fmt"
	"time"

	"github.com/platinummonkey/agent-chat/pkg/ircclient"
)

// TaskStatsInfo holds aggregate task statistics.
type TaskStatsInfo struct {
	Total     int `json:"total"`
	Started   int `json:"started"`
	Completed int `json:"completed"`
	Blocked   int `json:"blocked"`
}

// HealthStatus represents the current health state of an agent.
type HealthStatus struct {
	Connected       bool                `json:"connected"`
	Healthy         bool                `json:"healthy"`
	Nick            string              `json:"nick"`
	Channels        []string            `json:"channels"`
	Uptime          time.Duration       `json:"uptime_ns"`
	UptimeHuman     string              `json:"uptime"`
	StartedAt       time.Time           `json:"started_at"`
	TaskStats       TaskStatsInfo       `json:"task_stats"`
	DependencyStats DependencyStatsInfo `json:"dependency_stats"`
}

// HealthMonitor provides pull-based health checks for an agent.
// No background goroutine — Check() computes status on demand.
type HealthMonitor struct {
	client    ircclient.Client
	state     *StateStore
	startedAt time.Time
}

// NewHealthMonitor creates a HealthMonitor.
func NewHealthMonitor(client ircclient.Client, state *StateStore) *HealthMonitor {
	return &HealthMonitor{
		client:    client,
		state:     state,
		startedAt: time.Now(),
	}
}

// Check returns the current health status of the agent.
func (h *HealthMonitor) Check() HealthStatus {
	uptime := time.Since(h.startedAt)
	tasks := h.state.ListTasks()

	var stats TaskStatsInfo
	stats.Total = len(tasks)
	for _, t := range tasks {
		switch t.Status {
		case TaskStatusStarted:
			stats.Started++
		case TaskStatusCompleted:
			stats.Completed++
		case TaskStatusBlocked:
			stats.Blocked++
		}
	}

	channels := h.client.JoinedChannels()
	if channels == nil {
		channels = []string{}
	}

	return HealthStatus{
		Connected:       h.client.Connected(),
		Healthy:         h.client.Healthy(),
		Nick:            h.client.Nick(),
		Channels:        channels,
		Uptime:          uptime,
		UptimeHuman:     formatDuration(uptime),
		StartedAt:       h.startedAt,
		TaskStats:       stats,
		DependencyStats: h.state.DependencyStats(),
	}
}

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dh%dm%ds", h, m, s)
}
