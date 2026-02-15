package agent

import (
	"sync"
	"time"
)

// VoteRecord tracks a single vote on a topic.
type VoteRecord struct {
	Topic   string
	Nick    string
	Choice  string
	Channel string
	VotedAt time.Time
}

// TopicSummary provides an aggregate view of votes on a topic.
type TopicSummary struct {
	Topic      string
	Votes      map[string]int // choice → count
	TotalVotes int
	Resolved   bool
	Resolution string
}

// EscalationRecord tracks an escalation to a human operator.
type EscalationRecord struct {
	Task        string
	ToNick      string
	Reason      string
	Severity    string
	EscalatedBy string
	Channel     string
	EscalatedAt time.Time
	Resolved    bool
}

// ConsensusStore tracks votes and escalations.
type ConsensusStore struct {
	mu          sync.RWMutex
	votes       map[string]map[string]*VoteRecord // topic → nick → latest vote
	escalations []*EscalationRecord
}

// NewConsensusStore creates a new ConsensusStore.
func NewConsensusStore() *ConsensusStore {
	return &ConsensusStore{
		votes: make(map[string]map[string]*VoteRecord),
	}
}

// RecordVote records or updates a vote. Last vote per nick wins.
func (cs *ConsensusStore) RecordVote(topic, nick, choice, channel string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.votes[topic] == nil {
		cs.votes[topic] = make(map[string]*VoteRecord)
	}
	cs.votes[topic][nick] = &VoteRecord{
		Topic:   topic,
		Nick:    nick,
		Choice:  choice,
		Channel: channel,
		VotedAt: time.Now(),
	}
}

// TopicSummaryFor returns a summary of votes for a topic.
func (cs *ConsensusStore) TopicSummaryFor(topic string) TopicSummary {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	s := TopicSummary{
		Topic: topic,
		Votes: make(map[string]int),
	}
	for _, v := range cs.votes[topic] {
		s.Votes[v.Choice]++
		s.TotalVotes++
	}
	return s
}

// RecordEscalation records an escalation event.
func (cs *ConsensusStore) RecordEscalation(task, toNick, reason, severity, escalatedBy, channel string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.escalations = append(cs.escalations, &EscalationRecord{
		Task:        task,
		ToNick:      toNick,
		Reason:      reason,
		Severity:    severity,
		EscalatedBy: escalatedBy,
		Channel:     channel,
		EscalatedAt: time.Now(),
	})
}

// ResolveEscalation marks the most recent escalation for a task as resolved.
func (cs *ConsensusStore) ResolveEscalation(task string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for i := len(cs.escalations) - 1; i >= 0; i-- {
		if cs.escalations[i].Task == task && !cs.escalations[i].Resolved {
			cs.escalations[i].Resolved = true
			break
		}
	}
}

// ListEscalations returns all escalation records.
func (cs *ConsensusStore) ListEscalations() []*EscalationRecord {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	result := make([]*EscalationRecord, len(cs.escalations))
	for i, e := range cs.escalations {
		cp := *e
		result[i] = &cp
	}
	return result
}

// ListTopics returns summaries for all voted-on topics.
func (cs *ConsensusStore) ListTopics() []TopicSummary {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	result := make([]TopicSummary, 0, len(cs.votes))
	for topic, votes := range cs.votes {
		s := TopicSummary{
			Topic: topic,
			Votes: make(map[string]int),
		}
		for _, v := range votes {
			s.Votes[v.Choice]++
			s.TotalVotes++
		}
		result = append(result, s)
	}
	return result
}
