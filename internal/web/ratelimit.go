package web

import (
	"sync"
	"time"
)

type rateBucket struct {
	tokens  float64
	updated time.Time
}

type rateLimiter struct {
	mu          sync.Mutex
	rate        float64
	burst       float64
	buckets     map[string]*rateBucket
	lastCleanup time.Time
	now         func() time.Time
}

func newRateLimiter(perMinute, burst int) *rateLimiter {
	return &rateLimiter{
		rate:        float64(perMinute) / 60.0,
		burst:       float64(burst),
		buckets:     make(map[string]*rateBucket),
		lastCleanup: time.Now(),
		now:         time.Now,
	}
}

func (l *rateLimiter) allow(key string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.lastCleanup) >= 10*time.Minute {
		l.cleanupLocked(now)
		l.lastCleanup = now
	}

	bucket, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &rateBucket{tokens: l.burst - 1, updated: now}
		return true
	}
	elapsed := now.Sub(bucket.updated).Seconds()
	bucket.tokens += elapsed * l.rate
	if bucket.tokens > l.burst {
		bucket.tokens = l.burst
	}
	bucket.updated = now
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

func (l *rateLimiter) cleanup() {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupLocked(now)
	l.lastCleanup = now
}

func (l *rateLimiter) cleanupLocked(now time.Time) {
	cutoff := now.Add(-15 * time.Minute)
	for key, bucket := range l.buckets {
		if bucket.updated.Before(cutoff) {
			delete(l.buckets, key)
		}
	}
}
