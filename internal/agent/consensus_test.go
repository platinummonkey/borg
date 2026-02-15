package agent

import (
	"testing"
)

func TestConsensusStore_VoteRecording(t *testing.T) {
	cs := NewConsensusStore()

	cs.RecordVote("deploy-strategy", "alice", "blue-green", "#project")
	cs.RecordVote("deploy-strategy", "bob", "canary", "#project")
	cs.RecordVote("deploy-strategy", "carol", "blue-green", "#project")

	s := cs.TopicSummaryFor("deploy-strategy")
	if s.TotalVotes != 3 {
		t.Errorf("TotalVotes = %d, want 3", s.TotalVotes)
	}
	if s.Votes["blue-green"] != 2 {
		t.Errorf("blue-green votes = %d, want 2", s.Votes["blue-green"])
	}
	if s.Votes["canary"] != 1 {
		t.Errorf("canary votes = %d, want 1", s.Votes["canary"])
	}
}

func TestConsensusStore_DuplicateVoteOverrides(t *testing.T) {
	cs := NewConsensusStore()

	cs.RecordVote("topic", "alice", "option-a", "#project")
	cs.RecordVote("topic", "alice", "option-b", "#project")

	s := cs.TopicSummaryFor("topic")
	if s.TotalVotes != 1 {
		t.Errorf("TotalVotes = %d, want 1 (duplicate should override)", s.TotalVotes)
	}
	if s.Votes["option-b"] != 1 {
		t.Errorf("option-b votes = %d, want 1", s.Votes["option-b"])
	}
	if s.Votes["option-a"] != 0 {
		t.Errorf("option-a votes = %d, want 0 (overridden)", s.Votes["option-a"])
	}
}

func TestConsensusStore_EscalationLifecycle(t *testing.T) {
	cs := NewConsensusStore()

	cs.RecordEscalation("auth", "human-1", "max review iterations exceeded", "medium", "ci-bot", "#project")

	escalations := cs.ListEscalations()
	if len(escalations) != 1 {
		t.Fatalf("ListEscalations = %d, want 1", len(escalations))
	}
	if escalations[0].Task != "auth" {
		t.Errorf("Task = %q, want auth", escalations[0].Task)
	}
	if escalations[0].Resolved {
		t.Error("should not be resolved initially")
	}

	cs.ResolveEscalation("auth")

	escalations = cs.ListEscalations()
	if !escalations[0].Resolved {
		t.Error("should be resolved after ResolveEscalation")
	}
}

func TestConsensusStore_TopicSummaryEmpty(t *testing.T) {
	cs := NewConsensusStore()

	s := cs.TopicSummaryFor("nonexistent")
	if s.TotalVotes != 0 {
		t.Errorf("TotalVotes = %d, want 0 for nonexistent topic", s.TotalVotes)
	}
}

func TestConsensusStore_ListTopics(t *testing.T) {
	cs := NewConsensusStore()

	cs.RecordVote("topic-a", "alice", "yes", "#a")
	cs.RecordVote("topic-b", "bob", "no", "#b")

	topics := cs.ListTopics()
	if len(topics) != 2 {
		t.Errorf("ListTopics = %d, want 2", len(topics))
	}
}
