package ircclient

import (
	"testing"
	"time"
)

func TestBackoff_ExponentialGrowth(t *testing.T) {
	b := NewBackoff(100*time.Millisecond, 1*time.Minute)

	// Collect averages at each attempt level.
	const samples = 200
	prevAvg := time.Duration(0)
	for attempt := 0; attempt < 4; attempt++ {
		var total time.Duration
		for i := 0; i < samples; i++ {
			bb := NewBackoff(100*time.Millisecond, 1*time.Minute)
			bb.attempt = attempt
			total += bb.Next()
		}
		avg := total / samples
		if attempt > 0 && avg <= prevAvg {
			t.Errorf("attempt %d average (%v) should be greater than attempt %d average (%v)",
				attempt, avg, attempt-1, prevAvg)
		}
		prevAvg = avg
	}
	_ = b
}

func TestBackoff_MaxCap(t *testing.T) {
	maxDur := 500 * time.Millisecond
	b := NewBackoff(100*time.Millisecond, maxDur)

	for i := 0; i < 100; i++ {
		d := b.Next()
		if d > maxDur {
			t.Errorf("attempt %d: duration %v exceeds max %v", i, d, maxDur)
		}
		if d < 0 {
			t.Errorf("attempt %d: duration %v is negative", i, d)
		}
	}
}

func TestBackoff_Reset(t *testing.T) {
	b := NewBackoff(100*time.Millisecond, 1*time.Minute)

	// Advance several attempts.
	for i := 0; i < 10; i++ {
		b.Next()
	}
	if b.Attempt() != 10 {
		t.Errorf("Attempt() = %d, want 10", b.Attempt())
	}

	b.Reset()
	if b.Attempt() != 0 {
		t.Errorf("Attempt() after Reset() = %d, want 0", b.Attempt())
	}

	// After reset, delays should be small (within base range).
	const samples = 100
	var total time.Duration
	for i := 0; i < samples; i++ {
		bb := NewBackoff(100*time.Millisecond, 1*time.Minute)
		total += bb.Next()
	}
	avg := total / samples
	// Average of uniform [0, 100ms] should be ~50ms.
	if avg > 150*time.Millisecond {
		t.Errorf("average after reset is too large: %v", avg)
	}
}

func TestBackoff_Jitter(t *testing.T) {
	// Multiple calls at the same attempt should produce different values.
	seen := make(map[time.Duration]bool)
	for i := 0; i < 50; i++ {
		b := NewBackoff(1*time.Second, 1*time.Minute)
		b.attempt = 5
		d := b.Next()
		seen[d] = true
	}
	if len(seen) < 2 {
		t.Errorf("expected randomized durations, got %d unique values", len(seen))
	}
}
