package ircclient

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestRateLimiter_AllowBurst(t *testing.T) {
	rl := NewRateLimiter(1.0, 3)

	for i := 0; i < 3; i++ {
		if !rl.Allow() {
			t.Errorf("Allow() #%d should return true within burst", i+1)
		}
	}
	if rl.Allow() {
		t.Error("Allow() after burst exhausted should return false")
	}
}

func TestRateLimiter_Refill(t *testing.T) {
	rl := NewRateLimiter(10.0, 1) // 10 tokens/sec, burst 1

	if !rl.Allow() {
		t.Fatal("first Allow() should succeed")
	}
	if rl.Allow() {
		t.Fatal("second Allow() should fail (burst exhausted)")
	}

	// Wait enough time for a refill.
	time.Sleep(150 * time.Millisecond)

	if !rl.Allow() {
		t.Error("Allow() after refill should succeed")
	}
}

func TestRateLimiter_WaitBlocks(t *testing.T) {
	rl := NewRateLimiter(10.0, 1) // 10 tokens/sec

	// Consume the only token.
	if !rl.Allow() {
		t.Fatal("initial Allow() should succeed")
	}

	start := time.Now()
	if err := rl.Wait(context.Background()); err != nil {
		t.Fatalf("Wait() returned error: %v", err)
	}
	elapsed := time.Since(start)

	// Should have waited approximately 100ms (1/10 sec).
	if elapsed < 50*time.Millisecond {
		t.Errorf("Wait() returned too quickly: %v", elapsed)
	}
	if elapsed > 300*time.Millisecond {
		t.Errorf("Wait() took too long: %v", elapsed)
	}
}

func TestRateLimiter_WaitContextCancelled(t *testing.T) {
	rl := NewRateLimiter(0.5, 1) // very slow refill

	// Exhaust the burst.
	rl.Allow()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := rl.Wait(ctx)
	if err == nil {
		t.Fatal("Wait() should return error when context cancelled")
	}
	if err != context.Canceled {
		t.Errorf("Wait() error = %v, want context.Canceled", err)
	}
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := NewRateLimiter(100.0, 10)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rl.Allow()
			_ = rl.Wait(context.Background())
		}()
	}
	wg.Wait()
}
