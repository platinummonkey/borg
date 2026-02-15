package agent

import (
	"testing"
)

func TestHandoffStore_CheckpointRecording(t *testing.T) {
	hs := NewHandoffStore()

	hs.RecordCheckpoint("auth", "alice", 30, "API endpoints done", "#project")
	hs.RecordCheckpoint("auth", "alice", 60, "Middleware done", "#project")

	cps := hs.Checkpoints("auth")
	if len(cps) != 2 {
		t.Fatalf("Checkpoints = %d, want 2", len(cps))
	}
	if cps[0].Progress != 30 {
		t.Errorf("cp[0].Progress = %d, want 30", cps[0].Progress)
	}
	if cps[1].Summary != "Middleware done" {
		t.Errorf("cp[1].Summary = %q, want 'Middleware done'", cps[1].Summary)
	}
}

func TestHandoffStore_CheckpointHistory(t *testing.T) {
	hs := NewHandoffStore()

	// No checkpoints initially.
	cps := hs.Checkpoints("nonexistent")
	if len(cps) != 0 {
		t.Errorf("Checkpoints for nonexistent = %d, want 0", len(cps))
	}
}

func TestHandoffStore_HandoffLifecycle(t *testing.T) {
	hs := NewHandoffStore()

	// Checkpoint first.
	hs.RecordCheckpoint("auth", "alice", 50, "Half done", "#project")

	// Handoff.
	hs.RecordHandoff("auth", "alice", "bob", "ctx-001", "#project")

	h := hs.GetHandoff("auth")
	if h == nil {
		t.Fatal("GetHandoff returned nil")
	}
	if h.From != "alice" {
		t.Errorf("From = %q, want alice", h.From)
	}
	if h.To != "bob" {
		t.Errorf("To = %q, want bob", h.To)
	}
	if h.ContextID != "ctx-001" {
		t.Errorf("ContextID = %q, want ctx-001", h.ContextID)
	}
	if h.Progress != 50 {
		t.Errorf("Progress = %d, want 50", h.Progress)
	}
	if h.Accepted {
		t.Error("Accepted should be false before AcceptHandoff")
	}

	// Accept.
	hs.AcceptHandoff("auth", "bob")

	h = hs.GetHandoff("auth")
	if !h.Accepted {
		t.Error("Accepted should be true after AcceptHandoff")
	}
}

func TestHandoffStore_AcceptWrongAgent(t *testing.T) {
	hs := NewHandoffStore()
	hs.RecordHandoff("auth", "alice", "bob", "", "#project")

	// Carol tries to accept — should not work.
	hs.AcceptHandoff("auth", "carol")

	h := hs.GetHandoff("auth")
	if h.Accepted {
		t.Error("handoff should not be accepted by wrong agent")
	}
}

func TestHandoffStore_ListHandoffs(t *testing.T) {
	hs := NewHandoffStore()
	hs.RecordHandoff("task-1", "alice", "bob", "", "#a")
	hs.RecordHandoff("task-2", "carol", "dave", "", "#b")

	all := hs.ListHandoffs()
	if len(all) != 2 {
		t.Errorf("ListHandoffs = %d, want 2", len(all))
	}
}
