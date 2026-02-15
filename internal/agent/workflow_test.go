package agent

import (
	"testing"
)

func TestWorkflowEngine_BuiltinDefinitions(t *testing.T) {
	we := NewWorkflowEngine()

	gp := we.GetDefinition("gated-pipeline")
	if gp == nil {
		t.Fatal("gated-pipeline definition missing")
	}
	if len(gp.Stages) != 5 {
		t.Errorf("gated-pipeline stages = %d, want 5", len(gp.Stages))
	}

	ch := we.GetDefinition("collaborative-handoff")
	if ch == nil {
		t.Fatal("collaborative-handoff definition missing")
	}
}

func TestWorkflowEngine_StartAndAdvance(t *testing.T) {
	we := NewWorkflowEngine()

	inst := we.StartWorkflow("gated-pipeline", "auth")
	if inst == nil {
		t.Fatal("StartWorkflow returned nil")
	}
	if inst.CurrentStage != "implement" {
		t.Errorf("initial stage = %q, want implement", inst.CurrentStage)
	}

	// Advance through stages.
	next := we.AdvanceStage("auth")
	if next != "review" {
		t.Errorf("after advance = %q, want review", next)
	}

	next = we.AdvanceStage("auth")
	if next != "merge" {
		t.Errorf("after advance = %q, want merge", next)
	}

	next = we.AdvanceStage("auth")
	if next != "release" {
		t.Errorf("after advance = %q, want release", next)
	}

	next = we.AdvanceStage("auth")
	if next != "cleanup" {
		t.Errorf("after advance = %q, want cleanup", next)
	}

	// Final advance — workflow complete.
	next = we.AdvanceStage("auth")
	if next != "" {
		t.Errorf("after final advance = %q, want empty (completed)", next)
	}

	inst = we.GetInstance("auth")
	if !inst.Completed {
		t.Error("workflow should be completed")
	}
}

func TestWorkflowEngine_CurrentStage(t *testing.T) {
	we := NewWorkflowEngine()
	we.StartWorkflow("gated-pipeline", "auth")

	stage := we.CurrentStage("auth")
	if stage == nil {
		t.Fatal("CurrentStage returned nil")
	}
	if stage.Name != "implement" {
		t.Errorf("stage name = %q, want implement", stage.Name)
	}

	we.AdvanceStage("auth")
	stage = we.CurrentStage("auth")
	if stage.Name != "review" {
		t.Errorf("stage name = %q, want review", stage.Name)
	}
	if len(stage.RequiredReviews) != 2 {
		t.Errorf("RequiredReviews = %d, want 2", len(stage.RequiredReviews))
	}
	if len(stage.RequiredGates) != 2 {
		t.Errorf("RequiredGates = %d, want 2", len(stage.RequiredGates))
	}
}

func TestWorkflowEngine_UnknownWorkflow(t *testing.T) {
	we := NewWorkflowEngine()

	inst := we.StartWorkflow("nonexistent", "task")
	if inst != nil {
		t.Error("StartWorkflow should return nil for unknown workflow")
	}
}

func TestWorkflowEngine_GetInstanceNonexistent(t *testing.T) {
	we := NewWorkflowEngine()
	if we.GetInstance("nonexistent") != nil {
		t.Error("GetInstance should return nil for unknown task")
	}
}

func TestWorkflowEngine_ListInstances(t *testing.T) {
	we := NewWorkflowEngine()
	we.StartWorkflow("gated-pipeline", "auth")
	we.StartWorkflow("collaborative-handoff", "api")

	instances := we.ListInstances()
	if len(instances) != 2 {
		t.Errorf("ListInstances = %d, want 2", len(instances))
	}
}

func TestWorkflowEngine_RegisterDefinition(t *testing.T) {
	we := NewWorkflowEngine()

	we.RegisterDefinition(&WorkflowDefinition{
		Name: "custom",
		Stages: []*WorkflowStage{
			{Name: "start", NextStage: "end"},
			{Name: "end"},
		},
	})

	inst := we.StartWorkflow("custom", "task")
	if inst == nil {
		t.Fatal("StartWorkflow returned nil for custom workflow")
	}
	if inst.CurrentStage != "start" {
		t.Errorf("initial stage = %q, want start", inst.CurrentStage)
	}

	next := we.AdvanceStage("task")
	if next != "end" {
		t.Errorf("after advance = %q, want end", next)
	}
}
