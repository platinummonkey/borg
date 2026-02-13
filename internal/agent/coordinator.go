package agent

import (
	"fmt"
	"sync"

	"github.com/platinummonkey/agent-chat/pkg/ircclient"
	"github.com/platinummonkey/agent-chat/pkg/protocol"
)

// UnblockNotifier watches for COMPLETED messages and automatically sends
// ACKNOWLEDGED messages for tasks that become unblocked as a result.
type UnblockNotifier struct {
	state    *StateStore
	client   ircclient.Client
	selfNick func() string

	mu       sync.Mutex
	notified map[string]struct{}
}

// NewUnblockNotifier creates an UnblockNotifier wired to the given state store and client.
func NewUnblockNotifier(state *StateStore, client ircclient.Client, selfNick func() string) *UnblockNotifier {
	return &UnblockNotifier{
		state:    state,
		client:   client,
		selfNick: selfNick,
		notified: make(map[string]struct{}),
	}
}

// OnTaskCompleted is called by the dispatcher after UpdateTask for COMPLETED messages.
// It checks for newly unblocked tasks and sends auto-acknowledgement messages.
func (n *UnblockNotifier) OnTaskCompleted(msg *protocol.Message) {
	if msg.Action != protocol.ActionCompleted {
		return
	}

	completedTask := msg.Get("task")
	if completedTask == "" {
		return
	}

	unblocked := n.state.UnblockedTasks()
	if len(unblocked) == 0 {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	for _, task := range unblocked {
		if _, already := n.notified[task.Name]; already {
			continue
		}
		n.notified[task.Name] = struct{}{}

		ackMsg := &protocol.Message{
			Action: protocol.ActionAcknowledged,
			Fields: map[string]string{
				"task":         task.Name,
				"unblocked-by": completedTask,
			},
			Tags: []string{"auto-unblock"},
		}

		target := msg.Channel
		if target == "" {
			target = msg.Nick
		}
		n.client.SendMessage(target, ackMsg.String())
	}
}

// Reset clears the notified set. Useful for testing.
func (n *UnblockNotifier) Reset() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.notified = make(map[string]struct{})
}

// NotifiedTasks returns the set of task names that have been auto-acknowledged.
func (n *UnblockNotifier) NotifiedTasks() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	result := make([]string, 0, len(n.notified))
	for name := range n.notified {
		result = append(result, name)
	}
	return result
}

// FormatUnblockMessage returns the default unblock notification string.
func FormatUnblockMessage(unblockedTask, completedTask string) string {
	return fmt.Sprintf("ACKNOWLEDGED task=%s unblocked-by=%s #auto-unblock", unblockedTask, completedTask)
}
