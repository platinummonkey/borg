package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StatePersistence handles saving and loading StateStore to a JSON file.
// It supports debounced writes and atomic file replacement.
type StatePersistence struct {
	path        string
	state       *StateStore
	mu          sync.Mutex
	debounce    *time.Timer
	debounceDur time.Duration
}

// NewStatePersistence creates a StatePersistence that saves state to path.
// An empty path disables persistence entirely.
func NewStatePersistence(path string, state *StateStore, debounce time.Duration) *StatePersistence {
	if debounce <= 0 {
		debounce = 5 * time.Second
	}
	return &StatePersistence{
		path:        path,
		state:       state,
		debounceDur: debounce,
	}
}

// Load reads the state file and restores the state store.
// A nonexistent file is treated as a fresh start (no error).
func (sp *StatePersistence) Load() error {
	if sp.path == "" {
		return nil
	}

	data, err := os.ReadFile(sp.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read state file: %w", err)
	}

	var ps PersistedState
	if err := json.Unmarshal(data, &ps); err != nil {
		return fmt.Errorf("parse state file: %w", err)
	}

	sp.state.Restore(&ps)
	slog.Info("state restored", "path", sp.path, "tasks", len(ps.Tasks), "saved_at", ps.SavedAt)
	return nil
}

// Save writes the current state to disk atomically (write temp, rename).
func (sp *StatePersistence) Save() error {
	if sp.path == "" {
		return nil
	}

	snap := sp.state.Snapshot()
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	dir := filepath.Dir(sp.path)
	tmp, err := os.CreateTemp(dir, ".agent-state-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpName, sp.path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename temp to state file: %w", err)
	}

	slog.Debug("state saved", "path", sp.path)
	return nil
}

// MarkDirty resets the debounce timer; Save is called when the timer fires.
func (sp *StatePersistence) MarkDirty() {
	if sp.path == "" {
		return
	}

	sp.mu.Lock()
	defer sp.mu.Unlock()

	if sp.debounce != nil {
		sp.debounce.Stop()
	}
	sp.debounce = time.AfterFunc(sp.debounceDur, func() {
		if err := sp.Save(); err != nil {
			slog.Error("failed to persist state", "error", err)
		}
	})
}

// Close flushes any pending save and stops the timer.
func (sp *StatePersistence) Close() {
	if sp.path == "" {
		return
	}

	sp.mu.Lock()
	if sp.debounce != nil {
		sp.debounce.Stop()
		sp.debounce = nil
	}
	sp.mu.Unlock()

	if err := sp.Save(); err != nil {
		slog.Error("failed to persist state on close", "error", err)
	}
}
