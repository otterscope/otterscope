package ingest

import (
	"sync"
	"time"
)

// rateLimiter is a per-key token-bucket limiter for ingest. A nil limiter or
// a non-positive rate means unlimited. Keys are ingest projects, so one
// noisy source can't starve others.
type rateLimiter struct {
	mu      sync.Mutex
	perSec  float64
	burst   float64
	buckets map[string]*tokenBucket
	now     func() time.Time // swappable in tests
}

type tokenBucket struct {
	tokens float64
	last   time.Time
}

// newRateLimiter builds a limiter. burst <= 0 defaults to 2x the rate (min 1).
func newRateLimiter(perSec, burst float64) *rateLimiter {
	if perSec <= 0 {
		return nil // unlimited
	}
	if burst <= 0 {
		burst = perSec * 2
	}
	if burst < 1 {
		burst = 1
	}
	return &rateLimiter{perSec: perSec, burst: burst, buckets: map[string]*tokenBucket{}, now: time.Now}
}

// allow consumes a token for key, reporting whether the request may proceed.
func (l *rateLimiter) allow(key string) bool {
	if l == nil || l.perSec <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b := l.buckets[key]
	if b == nil {
		b = &tokenBucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens += now.Sub(b.last).Seconds() * l.perSec
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}
