package ircclient

import (
	"math"
	"math/rand/v2"
	"time"
)

// Backoff implements exponential backoff with full jitter.
type Backoff struct {
	Base    time.Duration
	Max     time.Duration
	attempt int
}

// NewBackoff creates a new Backoff with the given base and max durations.
func NewBackoff(base, max time.Duration) *Backoff {
	return &Backoff{
		Base: base,
		Max:  max,
	}
}

// Next returns the next backoff duration using full jitter:
// rand(0, min(max, base * 2^attempt)), then increments the attempt counter.
func (b *Backoff) Next() time.Duration {
	exp := math.Pow(2, float64(b.attempt))
	ceiling := time.Duration(float64(b.Base) * exp)
	if ceiling > b.Max || ceiling <= 0 { // overflow protection
		ceiling = b.Max
	}
	d := time.Duration(rand.Int64N(int64(ceiling) + 1))
	b.attempt++
	return d
}

// Reset resets the attempt counter back to zero.
func (b *Backoff) Reset() {
	b.attempt = 0
}

// Attempt returns the current attempt number.
func (b *Backoff) Attempt() int {
	return b.attempt
}
