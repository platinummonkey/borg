package agent

import (
	"sync"
)

// WorkflowStage defines a stage in a workflow with required gates and reviews.
type WorkflowStage struct {
	Name            string   `json:"name"`
	RequiredGates   []string `json:"required_gates,omitempty"`
	RequiredReviews []string `json:"required_reviews,omitempty"`
	NextStage       string   `json:"next_stage,omitempty"`
}

// WorkflowDefinition defines a named sequence of stages.
type WorkflowDefinition struct {
	Name   string           `json:"name"`
	Stages []*WorkflowStage `json:"stages"`
}

// WorkflowInstance tracks the current state of a workflow execution.
type WorkflowInstance struct {
	Name         string `json:"name"`
	Task         string `json:"task"`
	CurrentStage string `json:"current_stage"`
	Completed    bool   `json:"completed"`
}

// WorkflowEngine manages workflow definitions and active instances.
type WorkflowEngine struct {
	mu          sync.RWMutex
	definitions map[string]*WorkflowDefinition
	instances   map[string]*WorkflowInstance // task → instance
}

// NewWorkflowEngine creates a new WorkflowEngine with built-in templates.
func NewWorkflowEngine() *WorkflowEngine {
	we := &WorkflowEngine{
		definitions: make(map[string]*WorkflowDefinition),
		instances:   make(map[string]*WorkflowInstance),
	}
	we.definitions["gated-pipeline"] = GatedPipelineWorkflow()
	we.definitions["collaborative-handoff"] = CollaborativeHandoffWorkflow()
	return we
}

// RegisterDefinition adds or replaces a workflow definition.
func (we *WorkflowEngine) RegisterDefinition(def *WorkflowDefinition) {
	we.mu.Lock()
	defer we.mu.Unlock()
	we.definitions[def.Name] = def
}

// StartWorkflow creates a new workflow instance for a task.
func (we *WorkflowEngine) StartWorkflow(workflowName, task string) *WorkflowInstance {
	we.mu.Lock()
	defer we.mu.Unlock()

	def, ok := we.definitions[workflowName]
	if !ok || len(def.Stages) == 0 {
		return nil
	}

	inst := &WorkflowInstance{
		Name:         workflowName,
		Task:         task,
		CurrentStage: def.Stages[0].Name,
	}
	we.instances[task] = inst
	return inst
}

// AdvanceStage moves a workflow instance to the next stage.
// Returns the new stage name, or empty string if workflow is complete.
func (we *WorkflowEngine) AdvanceStage(task string) string {
	we.mu.Lock()
	defer we.mu.Unlock()

	inst, ok := we.instances[task]
	if !ok || inst.Completed {
		return ""
	}

	def := we.definitions[inst.Name]
	if def == nil {
		return ""
	}

	// Find current stage and advance.
	for i, stage := range def.Stages {
		if stage.Name == inst.CurrentStage {
			if stage.NextStage != "" {
				inst.CurrentStage = stage.NextStage
				return inst.CurrentStage
			}
			if i+1 < len(def.Stages) {
				inst.CurrentStage = def.Stages[i+1].Name
				return inst.CurrentStage
			}
			inst.Completed = true
			return ""
		}
	}
	return ""
}

// GetInstance returns a copy of the workflow instance for a task, or nil.
func (we *WorkflowEngine) GetInstance(task string) *WorkflowInstance {
	we.mu.RLock()
	defer we.mu.RUnlock()
	inst, ok := we.instances[task]
	if !ok {
		return nil
	}
	cp := *inst
	return &cp
}

// GetDefinition returns a workflow definition by name, or nil.
func (we *WorkflowEngine) GetDefinition(name string) *WorkflowDefinition {
	we.mu.RLock()
	defer we.mu.RUnlock()
	def, ok := we.definitions[name]
	if !ok {
		return nil
	}
	return def
}

// CurrentStage returns the current stage definition for a task's workflow, or nil.
func (we *WorkflowEngine) CurrentStage(task string) *WorkflowStage {
	we.mu.RLock()
	defer we.mu.RUnlock()
	inst, ok := we.instances[task]
	if !ok {
		return nil
	}
	def := we.definitions[inst.Name]
	if def == nil {
		return nil
	}
	for _, stage := range def.Stages {
		if stage.Name == inst.CurrentStage {
			cp := *stage
			cp.RequiredGates = append([]string(nil), stage.RequiredGates...)
			cp.RequiredReviews = append([]string(nil), stage.RequiredReviews...)
			return &cp
		}
	}
	return nil
}

// ListDefinitions returns all registered workflow definitions.
func (we *WorkflowEngine) ListDefinitions() []*WorkflowDefinition {
	we.mu.RLock()
	defer we.mu.RUnlock()
	result := make([]*WorkflowDefinition, 0, len(we.definitions))
	for _, def := range we.definitions {
		result = append(result, def)
	}
	return result
}

// ListInstances returns all active workflow instances.
func (we *WorkflowEngine) ListInstances() []*WorkflowInstance {
	we.mu.RLock()
	defer we.mu.RUnlock()
	result := make([]*WorkflowInstance, 0, len(we.instances))
	for _, inst := range we.instances {
		cp := *inst
		result = append(result, &cp)
	}
	return result
}

// GatedPipelineWorkflow returns the built-in gated pipeline workflow definition.
// Stages: implement → review → merge → release → cleanup
func GatedPipelineWorkflow() *WorkflowDefinition {
	return &WorkflowDefinition{
		Name: "gated-pipeline",
		Stages: []*WorkflowStage{
			{
				Name:      "implement",
				NextStage: "review",
			},
			{
				Name:            "review",
				RequiredReviews: []string{"architecture", "security"},
				RequiredGates:   []string{"monitoring", "coverage"},
				NextStage:       "merge",
			},
			{
				Name:      "merge",
				NextStage: "release",
			},
			{
				Name:      "release",
				NextStage: "cleanup",
			},
			{
				Name: "cleanup",
			},
		},
	}
}

// CollaborativeHandoffWorkflow returns the built-in collaborative handoff workflow.
// Stages: implement → checkpoint → handoff → continue → complete
func CollaborativeHandoffWorkflow() *WorkflowDefinition {
	return &WorkflowDefinition{
		Name: "collaborative-handoff",
		Stages: []*WorkflowStage{
			{
				Name:      "implement",
				NextStage: "checkpoint",
			},
			{
				Name:      "checkpoint",
				NextStage: "handoff",
			},
			{
				Name:      "handoff",
				NextStage: "continue",
			},
			{
				Name:      "continue",
				NextStage: "complete",
			},
			{
				Name: "complete",
			},
		},
	}
}
