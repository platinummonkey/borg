package agent

import (
	"sync"
	"time"
)

// ReviewVerdict is the outcome of a code review.
type ReviewVerdict string

const (
	ReviewApproved         ReviewVerdict = "approved"
	ReviewChangesRequested ReviewVerdict = "changes-requested"
	ReviewRejected         ReviewVerdict = "rejected"
)

// GateStatus is the result of a gate check.
type GateStatus string

const (
	GatePassed  GateStatus = "passed"
	GateFailed  GateStatus = "failed"
	GatePending GateStatus = "pending"
)

// ReviewRecord tracks a single review request/response.
type ReviewRecord struct {
	Task        string
	PR          string
	ReviewType  string
	RequestedBy string
	Reviewer    string
	Verdict     ReviewVerdict
	Details     string
	Channel     string
	RequestedAt time.Time
	CompletedAt time.Time
	Iteration   int
}

// GateRecord tracks a single gate check result.
type GateRecord struct {
	Task      string
	Gate      string
	Status    GateStatus
	Details   string
	CheckedBy string
	Channel   string
	CheckedAt time.Time
}

// ReviewSummary provides an aggregate view of reviews for a task.
type ReviewSummary struct {
	Task            string
	TotalReviews    int
	PendingReviews  int
	ApprovedReviews int
	AllGatesPassed  bool
	IterationCount  int
	NeedsEscalation bool
}

// ReviewStore tracks reviews and gates by task.
type ReviewStore struct {
	mu            sync.RWMutex
	reviews       map[string][]*ReviewRecord // task → review records
	gates         map[string][]*GateRecord   // task → gate records
	iterations    map[string]int             // task → iteration count
	maxIterations int
}

// NewReviewStore creates a ReviewStore. maxIterations of 0 means unlimited.
func NewReviewStore(maxIterations int) *ReviewStore {
	return &ReviewStore{
		reviews:       make(map[string][]*ReviewRecord),
		gates:         make(map[string][]*GateRecord),
		iterations:    make(map[string]int),
		maxIterations: maxIterations,
	}
}

// RecordRequest records a new review request.
func (rs *ReviewStore) RecordRequest(task, pr, reviewType, requestedBy, channel string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.iterations[task]++
	rs.reviews[task] = append(rs.reviews[task], &ReviewRecord{
		Task:        task,
		PR:          pr,
		ReviewType:  reviewType,
		RequestedBy: requestedBy,
		Channel:     channel,
		RequestedAt: time.Now(),
		Iteration:   rs.iterations[task],
	})
}

// RecordComplete records a review completion with a verdict.
func (rs *ReviewStore) RecordComplete(task, pr, reviewer string, verdict ReviewVerdict, details, channel string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	// Find the most recent pending review for this task+pr.
	for i := len(rs.reviews[task]) - 1; i >= 0; i-- {
		r := rs.reviews[task][i]
		if r.Task == task && (pr == "" || r.PR == pr) && r.Verdict == "" {
			r.Reviewer = reviewer
			r.Verdict = verdict
			r.Details = details
			r.CompletedAt = time.Now()
			break
		}
	}
}

// RecordGate records a gate check result.
func (rs *ReviewStore) RecordGate(task, gate string, status GateStatus, details, checkedBy, channel string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.gates[task] = append(rs.gates[task], &GateRecord{
		Task:      task,
		Gate:      gate,
		Status:    status,
		Details:   details,
		CheckedBy: checkedBy,
		Channel:   channel,
		CheckedAt: time.Now(),
	})
}

// Summary returns an aggregate review summary for a task.
func (rs *ReviewStore) Summary(task string) ReviewSummary {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	s := ReviewSummary{Task: task}
	for _, r := range rs.reviews[task] {
		s.TotalReviews++
		if r.Verdict == "" {
			s.PendingReviews++
		} else if r.Verdict == ReviewApproved {
			s.ApprovedReviews++
		}
	}
	s.IterationCount = rs.iterations[task]
	s.AllGatesPassed = rs.allGatesPassedLocked(task)
	if rs.maxIterations > 0 && s.IterationCount > rs.maxIterations {
		s.NeedsEscalation = true
	}
	return s
}

// AllGatesPassed returns true if all gates for a task have passed.
func (rs *ReviewStore) AllGatesPassed(task string) bool {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.allGatesPassedLocked(task)
}

func (rs *ReviewStore) allGatesPassedLocked(task string) bool {
	gates := rs.gates[task]
	if len(gates) == 0 {
		return false
	}
	// Build latest status per gate name.
	latest := make(map[string]GateStatus)
	for _, g := range gates {
		latest[g.Gate] = g.Status
	}
	for _, s := range latest {
		if s != GatePassed {
			return false
		}
	}
	return true
}

// Reviews returns all review records for a task.
func (rs *ReviewStore) Reviews(task string) []*ReviewRecord {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	src := rs.reviews[task]
	result := make([]*ReviewRecord, len(src))
	for i, r := range src {
		cp := *r
		result[i] = &cp
	}
	return result
}

// Gates returns all gate records for a task.
func (rs *ReviewStore) Gates(task string) []*GateRecord {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	src := rs.gates[task]
	result := make([]*GateRecord, len(src))
	for i, g := range src {
		cp := *g
		result[i] = &cp
	}
	return result
}

// ListReviews returns all tracked review records across all tasks.
func (rs *ReviewStore) ListReviews() []*ReviewRecord {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	var result []*ReviewRecord
	for _, reviews := range rs.reviews {
		for _, r := range reviews {
			cp := *r
			result = append(result, &cp)
		}
	}
	return result
}
