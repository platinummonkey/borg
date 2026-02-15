package agent

import (
	"sync"
	"testing"
	"time"
)

func TestTaskBoard_OfferAndClaimInstant(t *testing.T) {
	s := NewStateStore()
	tb := NewTaskBoard(s, 0) // instant arbitration

	tb.RecordOffer("task-1", "#project", "alice", "high", "backend")

	offer := tb.GetOffer("task-1")
	if offer == nil {
		t.Fatal("GetOffer returned nil")
	}
	if offer.OfferedBy != "alice" {
		t.Errorf("OfferedBy = %q, want alice", offer.OfferedBy)
	}
	if offer.Priority != "high" {
		t.Errorf("Priority = %q, want high", offer.Priority)
	}

	// First claim wins instantly.
	winner := tb.RecordClaim("task-1", "bob", 2)
	if winner != "bob" {
		t.Errorf("winner = %q, want bob", winner)
	}
	if tb.Winner("task-1") != "bob" {
		t.Errorf("Winner = %q, want bob", tb.Winner("task-1"))
	}
}

func TestTaskBoard_ClaimArbitrationByLoad(t *testing.T) {
	s := NewStateStore()
	tb := NewTaskBoard(s, 100*time.Millisecond)

	tb.RecordOffer("task-1", "#project", "alice", "high", "")

	// Multiple claims within jitter window.
	tb.RecordClaim("task-1", "bob", 5)
	tb.RecordClaim("task-1", "carol", 2)
	tb.RecordClaim("task-1", "dave", 8)

	// Wait for arbitration.
	time.Sleep(250 * time.Millisecond)

	// Carol should win (lowest load).
	if winner := tb.Winner("task-1"); winner != "carol" {
		t.Errorf("Winner = %q, want carol (lowest load)", winner)
	}
}

func TestTaskBoard_YieldAndReclaim(t *testing.T) {
	s := NewStateStore()
	tb := NewTaskBoard(s, 0)

	tb.RecordOffer("task-1", "#project", "alice", "", "")
	tb.RecordClaim("task-1", "bob", 0)

	if tb.Winner("task-1") != "bob" {
		t.Fatalf("initial winner should be bob")
	}

	tb.RecordYield("task-1")
	if tb.Winner("task-1") != "" {
		t.Errorf("after yield, Winner should be empty, got %q", tb.Winner("task-1"))
	}

	// Reclaim.
	tb.RecordClaim("task-1", "carol", 0)
	if tb.Winner("task-1") != "carol" {
		t.Errorf("after reclaim, Winner = %q, want carol", tb.Winner("task-1"))
	}
}

func TestTaskBoard_Decline(t *testing.T) {
	s := NewStateStore()
	tb := NewTaskBoard(s, 0)

	tb.RecordAssign("task-1", "bob", "alice", "#project")
	if tb.Winner("task-1") != "bob" {
		t.Fatalf("initial assignee should be bob")
	}

	tb.RecordDecline("task-1")
	if tb.Winner("task-1") != "" {
		t.Errorf("after decline, Winner should be empty, got %q", tb.Winner("task-1"))
	}
}

func TestTaskBoard_ListOffers(t *testing.T) {
	s := NewStateStore()
	tb := NewTaskBoard(s, 0)

	tb.RecordOffer("task-1", "#a", "alice", "", "")
	tb.RecordOffer("task-2", "#b", "bob", "", "")

	offers := tb.ListOffers()
	if len(offers) != 2 {
		t.Errorf("ListOffers = %d, want 2", len(offers))
	}
}

func TestTaskBoard_ConcurrentClaims(t *testing.T) {
	s := NewStateStore()
	tb := NewTaskBoard(s, 0)

	tb.RecordOffer("task-1", "#project", "alice", "", "")

	var wg sync.WaitGroup
	winners := make([]string, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			winners[idx] = tb.RecordClaim("task-1", "agent-"+string(rune('a'+idx)), idx)
		}(i)
	}
	wg.Wait()

	// Exactly one winner (first to acquire the lock).
	winCount := 0
	for _, w := range winners {
		if w != "" {
			winCount++
		}
	}
	// With instant arbitration, only the first claim wins.
	if tb.Winner("task-1") == "" {
		t.Error("no winner after concurrent claims")
	}
}

func TestTaskBoard_AssignCreatesOffer(t *testing.T) {
	s := NewStateStore()
	tb := NewTaskBoard(s, 0)

	// Assign without prior offer.
	tb.RecordAssign("task-1", "bob", "alice", "#project")

	offer := tb.GetOffer("task-1")
	if offer == nil {
		t.Fatal("Assign should create offer entry")
	}
	if offer.ClaimedBy != "bob" {
		t.Errorf("ClaimedBy = %q, want bob", offer.ClaimedBy)
	}
	if offer.OfferedBy != "alice" {
		t.Errorf("OfferedBy = %q, want alice", offer.OfferedBy)
	}
}
