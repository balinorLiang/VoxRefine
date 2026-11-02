package web

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimiterBurstAndRefill(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter := newRateLimiter(60, 3)
	limiter.now = func() time.Time { return now }
	limiter.lastCleanup = now

	for i := 0; i < 3; i++ {
		if !limiter.allow("client") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if limiter.allow("client") {
		t.Fatal("fourth request should be rate limited")
	}

	now = now.Add(time.Second)
	if !limiter.allow("client") {
		t.Fatal("one token should refill after one second")
	}
	if limiter.allow("client") {
		t.Fatal("refilled token should be consumed")
	}
}

func TestRateLimiterKeysAreIndependent(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter := newRateLimiter(60, 1)
	limiter.now = func() time.Time { return now }
	limiter.lastCleanup = now

	if !limiter.allow("a") || !limiter.allow("b") {
		t.Fatal("different clients should have independent buckets")
	}
	if limiter.allow("a") || limiter.allow("b") {
		t.Fatal("each bucket should be exhausted")
	}
}

func TestRateLimiterCleanupDropsStaleBuckets(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter := newRateLimiter(60, 2)
	limiter.now = func() time.Time { return now }
	limiter.lastCleanup = now
	_ = limiter.allow("stale")

	now = now.Add(16 * time.Minute)
	_ = limiter.allow("fresh")
	if _, ok := limiter.buckets["stale"]; ok {
		t.Fatal("stale bucket should be removed")
	}
	if _, ok := limiter.buckets["fresh"]; !ok {
		t.Fatal("fresh bucket should remain")
	}
}

func TestRateLimiterConcurrentBurstIsBounded(t *testing.T) {
	limiter := newRateLimiter(60, 10)
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if limiter.allow("same-client") {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := allowed.Load(); got != 10 {
		t.Fatalf("allowed = %d, want burst of 10", got)
	}
}
