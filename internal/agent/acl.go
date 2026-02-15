package agent

import (
	"path"
	"sync"

	"github.com/platinummonkey/borg/internal/config"
	"github.com/platinummonkey/borg/pkg/protocol"
)

// ACLEngine evaluates authorization rules for protocol messages.
// Rules are evaluated in order; first match wins. No match = allow (backward compatible).
type ACLEngine struct {
	mu    sync.RWMutex
	rules []config.ACLRule
}

// NewACLEngine creates an ACLEngine with the given rules.
func NewACLEngine(rules []config.ACLRule) *ACLEngine {
	return &ACLEngine{rules: rules}
}

// Check returns true if the given nick/channel/action is allowed.
// First matching rule wins; no match = allow.
func (e *ACLEngine) Check(nick, channel string, action protocol.Action) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, rule := range e.rules {
		if !matchGlob(rule.Channel, channel) {
			continue
		}
		if !matchGlob(rule.NickPattern, nick) {
			continue
		}
		if len(rule.Actions) > 0 && !containsAction(rule.Actions, action) {
			continue
		}
		return rule.Effect == "allow"
	}
	return true // default allow
}

// SetRules replaces the rule set (hot-reload).
func (e *ACLEngine) SetRules(rules []config.ACLRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = rules
}

// matchGlob uses path.Match for glob-style matching.
func matchGlob(pattern, value string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	matched, err := path.Match(pattern, value)
	if err != nil {
		return false
	}
	return matched
}

func containsAction(actions []protocol.Action, action protocol.Action) bool {
	for _, a := range actions {
		if a == action {
			return true
		}
	}
	return false
}
