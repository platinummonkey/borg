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
