package ircclient

import (
	"context"
	"sync"
	"time"
)

// RateLimiter implements a token bucket algorithm for rate limiting outgoing messages.
type RateLimiter struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// NewRateLimiter creates a new token bucket rate limiter.
// rate is tokens per second; burst is the maximum bucket size.
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	return &RateLimiter{
		tokens:     float64(burst),
		maxTokens:  float64(burst),
		refillRate: rate,
		lastRefill: time.Now(),
	}
}

// refill adds tokens based on elapsed time. Must be called with mu held.
func (r *RateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(r.lastRefill).Seconds()
	r.tokens += elapsed * r.refillRate
	if r.tokens > r.maxTokens {
		r.tokens = r.maxTokens
	}
	r.lastRefill = now
}

// Allow attempts to consume one token without blocking.
// Returns true if a token was consumed.
func (r *RateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.refill()
	if r.tokens >= 1 {
		r.tokens--
		return true
	}
	return false
}

// Wait blocks until a token is available or ctx is cancelled.
func (r *RateLimiter) Wait(ctx context.Context) error {
	for {
		r.mu.Lock()
		r.refill()
		if r.tokens >= 1 {
			r.tokens--
			r.mu.Unlock()
			return nil
		}
		// Calculate how long until the next token arrives.
		deficit := 1 - r.tokens
		waitDur := time.Duration(deficit / r.refillRate * float64(time.Second))
		r.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDur):
			// Loop back to try consuming a token.
		}
	}
}
