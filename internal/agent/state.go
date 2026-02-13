package agent

import (
	"log/slog"
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

// DependencyStatsInfo holds aggregate statistics about the dependency graph.
type DependencyStatsInfo struct {
	TotalEdges      int
	ResolvedEdges   int
	UnresolvedEdges int
	BlockedTasks    int
	UnblockedTasks  int
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
// Returns false if the edge would create a cycle or already exists.
// Must be called with s.mu held.
func (s *StateStore) addDependencyLocked(edge DependencyEdge) bool {
	for _, e := range s.dependencies {
		if e.Blocked == edge.Blocked && e.BlockedBy == edge.BlockedBy {
			return false
		}
	}
	if s.hasCycleLocked(edge.Blocked, edge.BlockedBy) {
		slog.Warn("dependency rejected: would create cycle",
			"blocked", edge.Blocked, "blocked_by", edge.BlockedBy)
		return false
	}
	s.dependencies = append(s.dependencies, edge)
	return true
}

// hasCycleLocked checks whether adding an edge blocked→blockedBy would create
// a cycle. It does a DFS from blockedBy following unresolved edges to see if
// blocked is reachable. Must be called with s.mu held.
func (s *StateStore) hasCycleLocked(blocked, blockedBy string) bool {
	if blocked == blockedBy {
		return true
	}
	visited := make(map[string]bool)
	stack := []string{blockedBy}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visited[node] {
			continue
		}
		visited[node] = true
		for _, e := range s.dependencies {
			if e.Blocked == node && !e.Resolved {
				if e.BlockedBy == blocked {
					return true
				}
				stack = append(stack, e.BlockedBy)
			}
		}
	}
	return false
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

// ListAgents returns all tracked agent statuses.
func (s *StateStore) ListAgents() []*AgentStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*AgentStatus, 0, len(s.agents))
	for _, status := range s.agents {
		cp := *status
		result = append(result, &cp)
	}
	return result
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

// BlockersOf returns the dependency edges where taskName is the blocked task.
// If includeResolved is false, only unresolved edges are returned.
func (s *StateStore) BlockersOf(taskName string, includeResolved bool) []DependencyEdge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []DependencyEdge
	for _, e := range s.dependencies {
		if e.Blocked == taskName && (includeResolved || !e.Resolved) {
			result = append(result, e)
		}
	}
	return result
}

// BlockedBy returns the dependency edges where taskName is the blocker.
// If includeResolved is false, only unresolved edges are returned.
func (s *StateStore) BlockedBy(taskName string, includeResolved bool) []DependencyEdge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []DependencyEdge
	for _, e := range s.dependencies {
		if e.BlockedBy == taskName && (includeResolved || !e.Resolved) {
			result = append(result, e)
		}
	}
	return result
}

// HasCycle returns true if adding an edge blocked→blockedBy would create a cycle.
func (s *StateStore) HasCycle(blocked, blockedBy string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hasCycleLocked(blocked, blockedBy)
}

// AllDependencies returns a copy of all dependency edges.
func (s *StateStore) AllDependencies() []DependencyEdge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]DependencyEdge, len(s.dependencies))
	copy(result, s.dependencies)
	return result
}

// DependencyStats returns aggregate statistics about the dependency graph.
func (s *StateStore) DependencyStats() DependencyStatsInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := DependencyStatsInfo{TotalEdges: len(s.dependencies)}
	for _, e := range s.dependencies {
		if e.Resolved {
			stats.ResolvedEdges++
		} else {
			stats.UnresolvedEdges++
		}
	}

	for _, info := range s.tasks {
		if info.Status != TaskStatusBlocked {
			continue
		}
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
			stats.UnblockedTasks++
		} else if hasDeps {
			stats.BlockedTasks++
		}
	}
	return stats
}

// TransitiveDependencies returns all tasks that taskName transitively depends on
// (i.e., all transitive blockers). Uses BFS over unresolved edges.
func (s *StateStore) TransitiveDependencies(taskName string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	visited := make(map[string]bool)
	queue := []string{taskName}
	var result []string

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		for _, e := range s.dependencies {
			if e.Blocked == node && !e.Resolved && !visited[e.BlockedBy] {
				visited[e.BlockedBy] = true
				result = append(result, e.BlockedBy)
				queue = append(queue, e.BlockedBy)
			}
		}
	}
	return result
}
