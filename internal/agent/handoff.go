package agent

import (
	"sync"
	"time"
)

// CheckpointRecord tracks an incremental progress report for a task.
type CheckpointRecord struct {
	Task      string
	Nick      string
	Progress  int
	Summary   string
	Channel   string
	Timestamp time.Time
}

// HandoffRecord tracks a task handoff between agents.
type HandoffRecord struct {
	Task      string
	From      string
	To        string
	ContextID string
	Channel   string
	Progress  int
	Accepted  bool
	CreatedAt time.Time
}

// HandoffStore tracks checkpoints and handoffs.
type HandoffStore struct {
	mu          sync.RWMutex
	checkpoints map[string][]CheckpointRecord // task → checkpoint history
	handoffs    map[string]*HandoffRecord      // task → latest handoff
}

// NewHandoffStore creates a new HandoffStore.
func NewHandoffStore() *HandoffStore {
	return &HandoffStore{
		checkpoints: make(map[string][]CheckpointRecord),
		handoffs:    make(map[string]*HandoffRecord),
	}
}

// RecordCheckpoint records an incremental progress checkpoint.
func (hs *HandoffStore) RecordCheckpoint(task, nick string, progress int, summary, channel string) {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hs.checkpoints[task] = append(hs.checkpoints[task], CheckpointRecord{
		Task:      task,
		Nick:      nick,
		Progress:  progress,
		Summary:   summary,
		Channel:   channel,
		Timestamp: time.Now(),
	})
}

// RecordHandoff records a task handoff.
func (hs *HandoffStore) RecordHandoff(task, from, to, contextID, channel string) {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	// Capture latest progress from checkpoints.
	progress := 0
	if cps := hs.checkpoints[task]; len(cps) > 0 {
		progress = cps[len(cps)-1].Progress
	}

	hs.handoffs[task] = &HandoffRecord{
		Task:      task,
		From:      from,
		To:        to,
		ContextID: contextID,
		Channel:   channel,
		Progress:  progress,
		CreatedAt: time.Now(),
	}
}

// AcceptHandoff marks a handoff as accepted by the target agent.
func (hs *HandoffStore) AcceptHandoff(task, nick string) {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	if h, ok := hs.handoffs[task]; ok && h.To == nick {
		h.Accepted = true
	}
}

// GetHandoff returns a copy of the latest handoff for a task, or nil.
func (hs *HandoffStore) GetHandoff(task string) *HandoffRecord {
	hs.mu.RLock()
	defer hs.mu.RUnlock()
	h, ok := hs.handoffs[task]
	if !ok {
		return nil
	}
	cp := *h
	return &cp
}

// Checkpoints returns the checkpoint history for a task.
func (hs *HandoffStore) Checkpoints(task string) []CheckpointRecord {
	hs.mu.RLock()
	defer hs.mu.RUnlock()
	cps := hs.checkpoints[task]
	result := make([]CheckpointRecord, len(cps))
	copy(result, cps)
	return result
}

// ListHandoffs returns all tracked handoffs.
func (hs *HandoffStore) ListHandoffs() []*HandoffRecord {
	hs.mu.RLock()
	defer hs.mu.RUnlock()
	result := make([]*HandoffRecord, 0, len(hs.handoffs))
	for _, h := range hs.handoffs {
		cp := *h
		result = append(result, &cp)
	}
	return result
}
