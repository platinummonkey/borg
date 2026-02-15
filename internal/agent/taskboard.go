package agent

import (
	"sync"
	"time"
)

// OfferInfo tracks a task that has been offered for claiming.
type OfferInfo struct {
	Task      string
	Channel   string
	OfferedBy string
	Priority  string
	Scope     string
	OfferedAt time.Time
	ClaimedBy string
}

// ClaimEntry records an agent's claim on an offered task.
type ClaimEntry struct {
	Nick      string
	Load      int
	ClaimedAt time.Time
}

// TaskBoard tracks offered tasks, pending claims, and claim arbitration.
type TaskBoard struct {
	mu            sync.RWMutex
	state         *StateStore
	claimJitter   time.Duration
	offers        map[string]*OfferInfo
	pendingClaims map[string][]ClaimEntry
	timers        map[string]*time.Timer
}

// NewTaskBoard creates a TaskBoard with the given claim jitter window.
// If claimJitter is 0, first-claim-wins (instant arbitration).
func NewTaskBoard(state *StateStore, claimJitter time.Duration) *TaskBoard {
	return &TaskBoard{
		state:         state,
		claimJitter:   claimJitter,
		offers:        make(map[string]*OfferInfo),
		pendingClaims: make(map[string][]ClaimEntry),
		timers:        make(map[string]*time.Timer),
	}
}

// RecordOffer records a new task offer.
func (tb *TaskBoard) RecordOffer(task, channel, offeredBy, priority, scope string) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.offers[task] = &OfferInfo{
		Task:      task,
		Channel:   channel,
		OfferedBy: offeredBy,
		Priority:  priority,
		Scope:     scope,
		OfferedAt: time.Now(),
	}
}

// RecordClaim records a claim on an offered task.
// Returns the winning nick if arbitration completes immediately (claimJitter==0),
// or empty string if the claim is queued for deferred arbitration.
func (tb *TaskBoard) RecordClaim(task, nick string, load int) string {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	entry := ClaimEntry{
		Nick:      nick,
		Load:      load,
		ClaimedAt: time.Now(),
	}
	tb.pendingClaims[task] = append(tb.pendingClaims[task], entry)

	if tb.claimJitter == 0 {
		// Instant arbitration: first claim wins.
		winner := tb.pendingClaims[task][0].Nick
		if offer, ok := tb.offers[task]; ok {
			offer.ClaimedBy = winner
		}
		delete(tb.pendingClaims, task)
		return winner
	}

	// Deferred arbitration: start timer on first claim.
	if _, exists := tb.timers[task]; !exists {
		tb.timers[task] = time.AfterFunc(tb.claimJitter, func() {
			tb.resolveArbitration(task)
		})
	}
	return ""
}

// resolveArbitration picks the winner from pending claims by lowest load.
func (tb *TaskBoard) resolveArbitration(task string) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	claims := tb.pendingClaims[task]
	if len(claims) == 0 {
		return
	}

	winner := claims[0]
	for _, c := range claims[1:] {
		if c.Load < winner.Load {
			winner = c
		}
	}

	if offer, ok := tb.offers[task]; ok {
		offer.ClaimedBy = winner.Nick
	}
	delete(tb.pendingClaims, task)
	delete(tb.timers, task)
}

// RecordAssign records a direct task assignment.
func (tb *TaskBoard) RecordAssign(task, to, assignedBy, channel string) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if offer, ok := tb.offers[task]; ok {
		offer.ClaimedBy = to
	} else {
		tb.offers[task] = &OfferInfo{
			Task:      task,
			Channel:   channel,
			OfferedBy: assignedBy,
			OfferedAt: time.Now(),
			ClaimedBy: to,
		}
	}
}

// RecordDecline removes ownership from a task.
func (tb *TaskBoard) RecordDecline(task string) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if offer, ok := tb.offers[task]; ok {
		offer.ClaimedBy = ""
	}
}

// RecordYield puts a task back into offered state.
func (tb *TaskBoard) RecordYield(task string) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if offer, ok := tb.offers[task]; ok {
		offer.ClaimedBy = ""
	}
}

// GetOffer returns a copy of the offer info for a task, or nil.
func (tb *TaskBoard) GetOffer(task string) *OfferInfo {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	offer, ok := tb.offers[task]
	if !ok {
		return nil
	}
	cp := *offer
	return &cp
}

// ListOffers returns all tracked offers.
func (tb *TaskBoard) ListOffers() []*OfferInfo {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	result := make([]*OfferInfo, 0, len(tb.offers))
	for _, offer := range tb.offers {
		cp := *offer
		result = append(result, &cp)
	}
	return result
}

// PendingClaims returns the pending claims for a task.
func (tb *TaskBoard) PendingClaims(task string) []ClaimEntry {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	claims := tb.pendingClaims[task]
	result := make([]ClaimEntry, len(claims))
	copy(result, claims)
	return result
}

// Winner returns the winning claimant for a task, or empty string if unresolved.
func (tb *TaskBoard) Winner(task string) string {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	if offer, ok := tb.offers[task]; ok {
		return offer.ClaimedBy
	}
	return ""
}
