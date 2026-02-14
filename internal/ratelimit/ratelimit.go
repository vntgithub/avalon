package ratelimit

import (
	"sync"
	"time"
)

// Limiter decides if a request from key should be allowed.
// Allow returns (allowed, retryAfterSeconds). When allowed is false, retryAfterSeconds
// may be set for the Retry-After response header (0 = omit).
type Limiter interface {
	Allow(key string) (allowed bool, retryAfterSec int)
}

// Noop allows all requests.
type Noop struct{}

func (Noop) Allow(key string) (bool, int) { return true, 0 }

// InMemory is a sliding-window rate limiter per key (single-instance only).
type InMemory struct {
	mu      sync.Mutex
	entries map[string][]time.Time
	limit   int
	window  time.Duration
	nowFunc func() time.Time
}

// NewInMemory allows up to limit requests per key per window.
func NewInMemory(limit int, window time.Duration) *InMemory {
	return &InMemory{
		entries: make(map[string][]time.Time),
		limit:   limit,
		window:  window,
		nowFunc: time.Now,
	}
}

func (r *InMemory) Allow(key string) (allowed bool, retryAfterSec int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.nowFunc()
	cutoff := now.Add(-r.window)
	times := r.entries[key]
	i := 0
	for _, t := range times {
		if t.After(cutoff) {
			times[i] = t
			i++
		}
	}
	times = times[:i]
	if len(times) >= r.limit {
		oldest := times[0]
		retryAfter := oldest.Add(r.window).Sub(now)
		if retryAfter > 0 {
			retryAfterSec = int(retryAfter.Seconds())
			if retryAfterSec < 1 {
				retryAfterSec = 1
			}
		}
		return false, retryAfterSec
	}
	times = append(times, now)
	r.entries[key] = times
	return true, 0
}
