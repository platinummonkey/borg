package agent

import (
	"testing"
)

func TestReviewStore_RequestAndComplete(t *testing.T) {
	rs := NewReviewStore(0)

	rs.RecordRequest("auth", "PR-1", "architecture", "alice", "#project")

	reviews := rs.Reviews("auth")
	if len(reviews) != 1 {
		t.Fatalf("Reviews = %d, want 1", len(reviews))
	}
	if reviews[0].Verdict != "" {
		t.Error("review should be pending initially")
	}

	rs.RecordComplete("auth", "PR-1", "bob", ReviewApproved, "LGTM", "#project")

	reviews = rs.Reviews("auth")
	if reviews[0].Verdict != ReviewApproved {
		t.Errorf("Verdict = %q, want approved", reviews[0].Verdict)
	}
	if reviews[0].Reviewer != "bob" {
		t.Errorf("Reviewer = %q, want bob", reviews[0].Reviewer)
	}
}

func TestReviewStore_GatePassFail(t *testing.T) {
	rs := NewReviewStore(0)

	rs.RecordGate("auth", "coverage", GatePassed, "85%", "ci-bot", "#project")
	rs.RecordGate("auth", "security", GateFailed, "vuln found", "sec-bot", "#project")

	if rs.AllGatesPassed("auth") {
		t.Error("AllGatesPassed should be false when security failed")
	}

	// Fix security gate.
	rs.RecordGate("auth", "security", GatePassed, "fixed", "sec-bot", "#project")

	if !rs.AllGatesPassed("auth") {
		t.Error("AllGatesPassed should be true after security fix")
	}
}

func TestReviewStore_IterationCounting(t *testing.T) {
	rs := NewReviewStore(0)

	rs.RecordRequest("auth", "PR-1", "architecture", "alice", "#project")
	rs.RecordRequest("auth", "PR-1", "architecture", "alice", "#project")
	rs.RecordRequest("auth", "PR-1", "architecture", "alice", "#project")

	summary := rs.Summary("auth")
	if summary.IterationCount != 3 {
		t.Errorf("IterationCount = %d, want 3", summary.IterationCount)
	}
}

func TestReviewStore_MaxIterationEscalation(t *testing.T) {
	rs := NewReviewStore(2) // max 2 iterations

	rs.RecordRequest("auth", "PR-1", "architecture", "alice", "#project")
	rs.RecordRequest("auth", "PR-1", "architecture", "alice", "#project")

	s := rs.Summary("auth")
	if s.NeedsEscalation {
		t.Error("should not need escalation at exactly max iterations")
	}

	rs.RecordRequest("auth", "PR-1", "architecture", "alice", "#project")

	s = rs.Summary("auth")
	if !s.NeedsEscalation {
		t.Error("should need escalation after exceeding max iterations")
	}
}

func TestReviewStore_Summary(t *testing.T) {
	rs := NewReviewStore(0)

	rs.RecordRequest("auth", "PR-1", "arch", "alice", "#project")
	rs.RecordRequest("auth", "PR-1", "security", "alice", "#project")
	rs.RecordComplete("auth", "PR-1", "bob", ReviewApproved, "", "#project")

	s := rs.Summary("auth")
	if s.TotalReviews != 2 {
		t.Errorf("TotalReviews = %d, want 2", s.TotalReviews)
	}
	if s.PendingReviews != 1 {
		t.Errorf("PendingReviews = %d, want 1", s.PendingReviews)
	}
	if s.ApprovedReviews != 1 {
		t.Errorf("ApprovedReviews = %d, want 1", s.ApprovedReviews)
	}
}

func TestReviewStore_NoGatesMeansNotPassed(t *testing.T) {
	rs := NewReviewStore(0)

	if rs.AllGatesPassed("nonexistent") {
		t.Error("AllGatesPassed should be false when no gates exist")
	}
}

func TestReviewStore_ListReviews(t *testing.T) {
	rs := NewReviewStore(0)
	rs.RecordRequest("auth", "PR-1", "arch", "alice", "#project")
	rs.RecordRequest("api", "PR-2", "security", "bob", "#project")

	all := rs.ListReviews()
	if len(all) != 2 {
		t.Errorf("ListReviews = %d, want 2", len(all))
	}
}
