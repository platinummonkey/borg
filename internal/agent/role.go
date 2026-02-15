package agent

import (
	"strings"
	"sync"

	"github.com/platinummonkey/agent-chat/pkg/protocol"
)

// Role constants for built-in agent roles.
const (
	RoleArchitectureReviewer = "architecture-reviewer"
	RoleSecurityReviewer     = "security-reviewer"
	RoleMonitoringGuardian   = "monitoring-guardian"
	RoleTestCoverageEnforcer = "test-coverage-enforcer"
	RoleMergeCoordinator     = "merge-coordinator"
	RoleReleaseCoordinator   = "release-coordinator"
	RoleCleanupAgent         = "cleanup-agent"
	RoleIncidentResponder    = "incident-responder"
	RoleTechDebtTracker      = "tech-debt-tracker"
)

// RoleBehavior defines an auto-response behavior triggered by specific actions.
type RoleBehavior struct {
	Role           string
	TriggerActions []protocol.Action
	Handler        func(msg *protocol.Message) []*protocol.Message
}

// Matches returns true if this behavior should trigger for the given message.
func (rb *RoleBehavior) Matches(msg *protocol.Message) bool {
	for _, a := range rb.TriggerActions {
		if msg.Action == a {
			return true
		}
	}
	return false
}

// RoleEngine manages agent roles and their auto-response behaviors.
type RoleEngine struct {
	mu        sync.RWMutex
	roles     []string
	behaviors []*RoleBehavior
}

// NewRoleEngine creates a RoleEngine with the given roles.
func NewRoleEngine(roles []string) *RoleEngine {
	return &RoleEngine{
		roles: roles,
	}
}

// RegisterBehavior adds a behavior to the engine.
func (re *RoleEngine) RegisterBehavior(behavior *RoleBehavior) {
	re.mu.Lock()
	defer re.mu.Unlock()
	re.behaviors = append(re.behaviors, behavior)
}

// HandleMessage checks all registered behaviors against the message
// and returns any auto-response messages.
func (re *RoleEngine) HandleMessage(msg *protocol.Message) []*protocol.Message {
	re.mu.RLock()
	defer re.mu.RUnlock()

	var responses []*protocol.Message
	for _, b := range re.behaviors {
		if b.Matches(msg) {
			if resps := b.Handler(msg); len(resps) > 0 {
				responses = append(responses, resps...)
			}
		}
	}
	return responses
}

// Roles returns the configured roles.
func (re *RoleEngine) Roles() []string {
	re.mu.RLock()
	defer re.mu.RUnlock()
	result := make([]string, len(re.roles))
	copy(result, re.roles)
	return result
}

// HasRole returns true if the engine has the given role.
func (re *RoleEngine) HasRole(role string) bool {
	re.mu.RLock()
	defer re.mu.RUnlock()
	for _, r := range re.roles {
		if strings.EqualFold(r, role) {
			return true
		}
	}
	return false
}

// ExpertiseTags returns expertise tags derived from roles for discovery.
func (re *RoleEngine) ExpertiseTags() []string {
	re.mu.RLock()
	defer re.mu.RUnlock()
	tags := make([]string, 0, len(re.roles))
	for _, r := range re.roles {
		tags = append(tags, "role:"+r)
	}
	return tags
}

// BuiltinBehavior returns a built-in RoleBehavior for the given role name,
// or nil if no built-in behavior exists.
func BuiltinBehavior(role string) *RoleBehavior {
	switch role {
	case RoleArchitectureReviewer:
		return &RoleBehavior{
			Role:           role,
			TriggerActions: []protocol.Action{protocol.ActionReviewRequest},
			Handler: func(msg *protocol.Message) []*protocol.Message {
				if msg.Get("review-type") != "architecture" {
					return nil
				}
				return []*protocol.Message{{
					Action: protocol.ActionReviewComplete,
					Fields: map[string]string{
						"task":    msg.Get("task"),
						"pr":     msg.Get("pr"),
						"verdict": string(ReviewApproved),
						"details": "Architecture review auto-approved",
					},
				}}
			},
		}
	case RoleSecurityReviewer:
		return &RoleBehavior{
			Role:           role,
			TriggerActions: []protocol.Action{protocol.ActionReviewRequest},
			Handler: func(msg *protocol.Message) []*protocol.Message {
				if msg.Get("review-type") != "security" {
					return nil
				}
				return []*protocol.Message{{
					Action: protocol.ActionReviewComplete,
					Fields: map[string]string{
						"task":    msg.Get("task"),
						"pr":     msg.Get("pr"),
						"verdict": string(ReviewApproved),
						"details": "Security review auto-approved",
					},
				}}
			},
		}
	case RoleMonitoringGuardian:
		return &RoleBehavior{
			Role:           role,
			TriggerActions: []protocol.Action{protocol.ActionReviewRequest},
			Handler: func(msg *protocol.Message) []*protocol.Message {
				if msg.Get("review-type") != "monitoring" {
					return nil
				}
				return []*protocol.Message{{
					Action: protocol.ActionGateCheck,
					Fields: map[string]string{
						"task":   msg.Get("task"),
						"gate":   "monitoring",
						"status": string(GatePassed),
						"details": "Monitoring gate auto-passed",
					},
				}}
			},
		}
	case RoleMergeCoordinator:
		return &RoleBehavior{
			Role:           role,
			TriggerActions: []protocol.Action{protocol.ActionReviewComplete},
			Handler: func(msg *protocol.Message) []*protocol.Message {
				if msg.Get("verdict") != string(ReviewApproved) {
					return nil
				}
				return []*protocol.Message{{
					Action: protocol.ActionStarted,
					Fields: map[string]string{
						"task":     msg.Get("task") + "-merge",
						"priority": "high",
					},
				}}
			},
		}
	case RoleReleaseCoordinator:
		return &RoleBehavior{
			Role:           role,
			TriggerActions: []protocol.Action{protocol.ActionCompleted},
			Handler: func(msg *protocol.Message) []*protocol.Message {
				if !strings.HasSuffix(msg.Get("task"), "-merge") {
					return nil
				}
				return []*protocol.Message{{
					Action: protocol.ActionCheckpoint,
					Fields: map[string]string{
						"task":     strings.TrimSuffix(msg.Get("task"), "-merge") + "-release",
						"progress": "0",
						"summary":  "Release initiated",
					},
				}}
			},
		}
	case RoleCleanupAgent:
		return &RoleBehavior{
			Role:           role,
			TriggerActions: []protocol.Action{protocol.ActionCompleted},
			Handler: func(msg *protocol.Message) []*protocol.Message {
				if !strings.HasSuffix(msg.Get("task"), "-release") {
					return nil
				}
				return []*protocol.Message{{
					Action: protocol.ActionOffer,
					Fields: map[string]string{
						"task":  strings.TrimSuffix(msg.Get("task"), "-release") + "-cleanup",
						"scope": "cleanup",
					},
				}}
			},
		}
	case RoleIncidentResponder:
		return &RoleBehavior{
			Role:           role,
			TriggerActions: []protocol.Action{protocol.ActionEscalate},
			Handler: func(msg *protocol.Message) []*protocol.Message {
				sev := msg.Get("severity")
				if sev != "high" && sev != "critical" {
					return nil
				}
				return []*protocol.Message{{
					Action: protocol.ActionStarted,
					Fields: map[string]string{
						"task":     msg.Get("task") + "-incident",
						"priority": "critical",
					},
				}}
			},
		}
	default:
		return nil
	}
}
