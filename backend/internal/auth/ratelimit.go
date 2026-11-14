package auth

import (
	"sync"
	"time"
)

type bucket struct {
	count   int
	resetAt time.Time
}

// RateLimiter enforces fixed-window request limits in process memory.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]bucket
	now     func() time.Time
}

// NewRateLimiter returns an in-memory RateLimiter.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]bucket),
		now:     time.Now,
	}
}

// Allow consumes one token for key if under limit within window.
// It returns true if allowed, and the remaining wait duration if blocked.
func (l *RateLimiter) Allow(key string, limit int, window time.Duration) (bool, time.Duration) {
	if limit <= 0 || window <= 0 {
		return false, window
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b, ok := l.buckets[key]
	if !ok || !now.Before(b.resetAt) {
		l.buckets[key] = bucket{count: 1, resetAt: now.Add(window)}
		return true, 0
	}
	if b.count >= limit {
		return false, b.resetAt.Sub(now)
	}
	b.count++
	l.buckets[key] = b
	return true, 0
}
