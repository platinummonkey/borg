package protocol

import (
	"slices"
	"strings"
	"time"
)

// Action represents a protocol action type.
type Action string

const (
	ActionStarted        Action = "STARTED"
	ActionCompleted      Action = "COMPLETED"
	ActionBlocked        Action = "BLOCKED"
	ActionAcknowledged   Action = "ACKNOWLEDGED"
	ActionHelpNeeded     Action = "HELP-NEEDED"
	ActionContext        Action = "CONTEXT"
	ActionRequestContext Action = "REQUEST-CONTEXT"
	ActionSharingContext Action = "SHARING-CONTEXT"
	ActionDiscover       Action = "DISCOVER"
	ActionCapabilities   Action = "CAPABILITIES"

	// Phase 12: Task Board
	ActionOffer   Action = "OFFER"
	ActionClaim   Action = "CLAIM"
	ActionAssign  Action = "ASSIGN"
	ActionAccept  Action = "ACCEPT"
	ActionDecline Action = "DECLINE"
	ActionYield   Action = "YIELD"

	// Phase 13: Checkpoints & Handoffs
	ActionCheckpoint Action = "CHECKPOINT"
	ActionHandoff    Action = "HANDOFF"

	// Phase 14: Review & Gate System
	ActionReviewRequest  Action = "REVIEW-REQUEST"
	ActionReviewComplete Action = "REVIEW-COMPLETE"
	ActionGateCheck      Action = "GATE-CHECK"

	// Phase 15: Consensus & Escalation
	ActionVote     Action = "VOTE"
	ActionEscalate Action = "ESCALATE"

	// Phase 17: Agent Management
	ActionCostReport Action = "COST-REPORT"
)

// validActions is the set of recognized protocol actions.
var validActions = map[Action]struct{}{
	ActionStarted:        {},
	ActionCompleted:      {},
	ActionBlocked:        {},
	ActionAcknowledged:   {},
	ActionHelpNeeded:     {},
	ActionContext:        {},
	ActionRequestContext: {},
	ActionSharingContext: {},
	ActionDiscover:       {},
	ActionCapabilities:   {},
	ActionOffer:          {},
	ActionClaim:          {},
	ActionAssign:         {},
	ActionAccept:         {},
	ActionDecline:        {},
	ActionYield:          {},
	ActionCheckpoint:     {},
	ActionHandoff:        {},
	ActionReviewRequest:  {},
	ActionReviewComplete: {},
	ActionGateCheck:      {},
	ActionVote:           {},
	ActionEscalate:       {},
	ActionCostReport:     {},
}

// Valid returns true if the action is a recognized protocol action.
func (a Action) Valid() bool {
	_, ok := validActions[a]
	return ok
}

// Message represents a parsed protocol message.
type Message struct {
	Action    Action
	Fields    map[string]string
	Tags      []string
	Payload   string    // raw content for SHARING-CONTEXT
	Channel   string    // set by dispatcher
	Nick      string    // set by dispatcher
	Timestamp time.Time // set by dispatcher
}

// Get returns the value for a field key, or empty string if not present.
func (m *Message) Get(key string) string {
	if m.Fields == nil {
		return ""
	}
	return m.Fields[key]
}

// HasTag returns true if the message contains the given tag (without #).
func (m *Message) HasTag(tag string) bool {
	return slices.Contains(m.Tags, tag)
}

// actionPrefixes contains all valid action strings for the fast-path check.
var actionPrefixes []string

func init() {
	for a := range validActions {
		actionPrefixes = append(actionPrefixes, string(a)+" ", string(a)+"\t")
	}
}

// IsProtocolMessage performs a cheap prefix check to determine if a raw string
// might be a protocol message. This avoids full parsing for regular chat.
func IsProtocolMessage(raw string) bool {
	raw = strings.TrimSpace(raw)
	for _, prefix := range actionPrefixes {
		if strings.HasPrefix(raw, prefix) {
			return true
		}
	}
	// Also match action-only messages (e.g. just "COMPLETED" with no args — unlikely but valid for parsing)
	_, ok := validActions[Action(raw)]
	return ok
}
