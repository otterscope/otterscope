package ingest

import (
	"testing"
	"time"
)

func TestRateLimiterBurstThenDeny(t *testing.T) {
	l := newRateLimiter(10, 3) // 3 burst
	now := time.Unix(0, 0)
	l.now = func() time.Time { return now }

	// Burst of 3 allowed, 4th denied (no time elapsed to refill).
	for i := 0; i < 3; i++ {
		if !l.allow("p") {
			t.Fatalf("burst token %d denied", i)
		}
	}
	if l.allow("p") {
		t.Fatal("4th request should be denied")
	}

	// After 0.1s at 10/s → 1 token refilled → one more allowed.
	now = now.Add(100 * time.Millisecond)
	if !l.allow("p") {
		t.Fatal("refilled token should be allowed")
	}
	if l.allow("p") {
		t.Fatal("only one token should have refilled")
	}
}

func TestRateLimiterPerKeyIsolation(t *testing.T) {
	l := newRateLimiter(1, 1)
	now := time.Unix(0, 0)
	l.now = func() time.Time { return now }

	if !l.allow("a") || !l.allow("b") {
		t.Fatal("each key has its own bucket")
	}
	if l.allow("a") || l.allow("b") {
		t.Fatal("each key independently exhausted")
	}
}

func TestRateLimiterUnlimited(t *testing.T) {
	var nilLimiter *rateLimiter
	if !nilLimiter.allow("p") {
		t.Fatal("nil limiter must allow")
	}
	if newRateLimiter(0, 0) != nil {
		t.Fatal("rate 0 should build a nil (unlimited) limiter")
	}
}
