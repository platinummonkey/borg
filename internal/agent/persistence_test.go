package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistence_SaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	state := NewStateStore()
	state.UpdateAgentStatus("agent-1", "#project", "task-a")
	state.AddDependency(DependencyEdge{Blocked: "task-b", BlockedBy: "task-a"})

	// Manually set a task.
	state.mu.Lock()
	state.tasks["task-a"] = &TaskInfo{
		Name:     "task-a",
		Status:   TaskStatusStarted,
		Priority: "high",
		Tags:     []string{"feature"},
	}
	state.mu.Unlock()

	sp := NewStatePersistence(path, state, time.Second)
	if err := sp.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load into a fresh state store.
	state2 := NewStateStore()
	sp2 := NewStatePersistence(path, state2, time.Second)
	if err := sp2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	task := state2.GetTask("task-a")
	if task == nil {
		t.Fatal("task-a not restored")
	}
	if task.Status != TaskStatusStarted {
		t.Errorf("status = %q, want started", task.Status)
	}
	if task.Priority != "high" {
		t.Errorf("priority = %q, want high", task.Priority)
	}

	deps := state2.AllDependencies()
	if len(deps) != 1 {
		t.Fatalf("dependencies = %d, want 1", len(deps))
	}
	if deps[0].Blocked != "task-b" || deps[0].BlockedBy != "task-a" {
		t.Errorf("dependency = %+v", deps[0])
	}

	agent := state2.GetAgentStatus("agent-1")
	if agent == nil {
		t.Fatal("agent-1 not restored")
	}
	if agent.TaskName != "task-a" {
		t.Errorf("agent task = %q, want task-a", agent.TaskName)
	}
}

func TestPersistence_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	state := NewStateStore()
	state.mu.Lock()
	state.tasks["x"] = &TaskInfo{Name: "x", Status: TaskStatusCompleted}
	state.mu.Unlock()

	sp := NewStatePersistence(path, state, time.Second)
	if err := sp.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify no temp files remain.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "state.json" {
			t.Errorf("unexpected file: %s", e.Name())
		}
	}

	// Verify file is valid JSON.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty state file")
	}
}

func TestPersistence_Debounce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	state := NewStateStore()
	sp := NewStatePersistence(path, state, 50*time.Millisecond)

	// Multiple MarkDirty calls should result in one save.
	sp.MarkDirty()
	sp.MarkDirty()
	sp.MarkDirty()

	// Wait for debounce to fire.
	time.Sleep(200 * time.Millisecond)

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state file not created after debounce: %v", err)
	}
}

func TestPersistence_NonexistentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.json")

	state := NewStateStore()
	sp := NewStatePersistence(path, state, time.Second)

	if err := sp.Load(); err != nil {
		t.Fatalf("Load nonexistent file should not error: %v", err)
	}
}

func TestPersistence_EmptyPath(t *testing.T) {
	state := NewStateStore()
	sp := NewStatePersistence("", state, time.Second)

	if err := sp.Load(); err != nil {
		t.Fatalf("Load empty path: %v", err)
	}
	if err := sp.Save(); err != nil {
		t.Fatalf("Save empty path: %v", err)
	}
	sp.MarkDirty()
	sp.Close()
}
