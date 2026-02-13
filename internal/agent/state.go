package agent

import (
	"strings"
	"sync"
	"time"

	"github.com/platinummonkey/agent-chat/pkg/protocol"
)

// TaskStatus represents the current status of a tracked task.
type TaskStatus string

const (
	TaskStatusStarted   TaskStatus = "started"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusBlocked   TaskStatus = "blocked"
)

// TaskInfo holds the tracked state of a task.
type TaskInfo struct {
	Name       string
	Status     TaskStatus
	Priority   string
	WaitingFor string
	Tags       []string
	LastAgent  string
	UpdatedAt  time.Time
}

// DependencyEdge represents a dependency between two tasks.
type DependencyEdge struct {
	Blocked    string // task name that is blocked
	BlockedBy  string // task name that is blocking
	Resolved   bool
	ResolvedAt time.Time
}

// AgentStatus tracks what an agent is currently doing.
type AgentStatus struct {
	Nick      string
	Channel   string
	TaskName  string
	UpdatedAt time.Time
}

// StateStore tracks task status, dependencies, and agent activity.
// It is thread-safe and keyed by task name.
type StateStore struct {
	mu           sync.RWMutex
	tasks        map[string]*TaskInfo
	dependencies []DependencyEdge
	agents       map[string]*AgentStatus
}

// NewStateStore creates an empty StateStore.
func NewStateStore() *StateStore {
	return &StateStore{
		tasks:  make(map[string]*TaskInfo),
		agents: make(map[string]*AgentStatus),
	}
}

// UpdateTask updates the state store based on a protocol message.
func (s *StateStore) UpdateTask(msg *protocol.Message) {
	taskName := msg.Get("task")
	if taskName == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	info, ok := s.tasks[taskName]
	if !ok {
		info = &TaskInfo{Name: taskName}
		s.tasks[taskName] = info
	}

	info.Tags = msg.Tags
	info.LastAgent = msg.Nick
	info.UpdatedAt = msg.Timestamp

	switch msg.Action {
	case protocol.ActionStarted:
		info.Status = TaskStatusStarted
		info.Priority = msg.Get("priority")
	case protocol.ActionCompleted:
		info.Status = TaskStatusCompleted
		s.resolveDependenciesLocked(taskName)
	case protocol.ActionBlocked:
		info.Status = TaskStatusBlocked
		info.WaitingFor = msg.Get("waiting-for")
		s.extractDependenciesLocked(msg, taskName)
	}
}

// extractDependenciesLocked parses dependency info from tags and fields.
// Must be called with s.mu held.
func (s *StateStore) extractDependenciesLocked(msg *protocol.Message, taskName string) {
	// From waiting-for field.
	if wf := msg.Get("waiting-for"); wf != "" {
		s.addDependencyLocked(DependencyEdge{Blocked: taskName, BlockedBy: wf})
	}

	// From #blocked-by-X tags.
	for _, tag := range msg.Tags {
		if strings.HasPrefix(tag, "blocked-by-") {
			blockedBy := strings.TrimPrefix(tag, "blocked-by-")
			if blockedBy != "" {
				s.addDependencyLocked(DependencyEdge{Blocked: taskName, BlockedBy: blockedBy})
			}
		}
	}
}

// resolveDependenciesLocked marks all dependencies on taskName as resolved.
// Must be called with s.mu held.
func (s *StateStore) resolveDependenciesLocked(taskName string) {
	now := time.Now()
	for i := range s.dependencies {
		if s.dependencies[i].BlockedBy == taskName && !s.dependencies[i].Resolved {
			s.dependencies[i].Resolved = true
			s.dependencies[i].ResolvedAt = now
		}
	}
}

// addDependencyLocked adds a dependency edge if it doesn't already exist.
// Must be called with s.mu held.
func (s *StateStore) addDependencyLocked(edge DependencyEdge) {
	for _, e := range s.dependencies {
		if e.Blocked == edge.Blocked && e.BlockedBy == edge.BlockedBy {
			return
		}
	}
	s.dependencies = append(s.dependencies, edge)
}

// GetTask returns the tracked info for a task, or nil if unknown.
func (s *StateStore) GetTask(name string) *TaskInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	info := s.tasks[name]
	if info == nil {
		return nil
	}
	// Return a copy.
	cp := *info
	cp.Tags = append([]string(nil), info.Tags...)
	return &cp
}

// ListTasks returns all tracked tasks.
func (s *StateStore) ListTasks() []*TaskInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*TaskInfo, 0, len(s.tasks))
	for _, info := range s.tasks {
		cp := *info
		cp.Tags = append([]string(nil), info.Tags...)
		result = append(result, &cp)
	}
	return result
}

// TasksByStatus returns tasks matching the given status.
func (s *StateStore) TasksByStatus(status TaskStatus) []*TaskInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*TaskInfo
	for _, info := range s.tasks {
		if info.Status == status {
			cp := *info
			cp.Tags = append([]string(nil), info.Tags...)
			result = append(result, &cp)
		}
	}
	return result
}

// AddDependency adds a dependency edge explicitly.
func (s *StateStore) AddDependency(edge DependencyEdge) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addDependencyLocked(edge)
}

// ResolvedDependencies returns all resolved dependency edges.
func (s *StateStore) ResolvedDependencies() []DependencyEdge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []DependencyEdge
	for _, e := range s.dependencies {
		if e.Resolved {
			result = append(result, e)
		}
	}
	return result
}

// UnblockedTasks returns tasks that were blocked but now have all dependencies resolved.
func (s *StateStore) UnblockedTasks() []*TaskInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*TaskInfo
	for _, info := range s.tasks {
		if info.Status != TaskStatusBlocked {
			continue
		}
		// Check if all dependency edges for this task are resolved.
		allResolved := true
		hasDeps := false
		for _, e := range s.dependencies {
			if e.Blocked == info.Name {
				hasDeps = true
				if !e.Resolved {
					allResolved = false
					break
				}
			}
		}
		if hasDeps && allResolved {
			cp := *info
			cp.Tags = append([]string(nil), info.Tags...)
			result = append(result, &cp)
		}
	}
	return result
}

// UpdateAgentStatus records what an agent is currently working on.
func (s *StateStore) UpdateAgentStatus(nick, channel, taskName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents[nick] = &AgentStatus{
		Nick:      nick,
		Channel:   channel,
		TaskName:  taskName,
		UpdatedAt: time.Now(),
	}
}

// GetAgentStatus returns the current status of an agent, or nil if unknown.
func (s *StateStore) GetAgentStatus(nick string) *AgentStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	status := s.agents[nick]
	if status == nil {
		return nil
	}
	cp := *status
	return &cp
}
