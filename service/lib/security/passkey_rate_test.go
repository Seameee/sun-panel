package security

import (
	"strconv"
	"testing"
	"time"
)

func TestPasskeyBeginLimiterSourceWindow(t *testing.T) {
	l := passkeyBeginLimiter{sources: make(map[string]attempt)}
	now := time.Unix(100, 0)
	for range passkeyBeginSourceLimit {
		if !l.Allow("192.0.2.1", now) {
			t.Fatal("source rejected before reaching its allowance")
		}
	}
	if l.Allow("192.0.2.1", now.Add(4*time.Minute)) {
		t.Fatal("source allowed before its window expires")
	}
	if !l.Allow("192.0.2.2", now.Add(4*time.Minute)) || !l.Allow("192.0.2.1", now.Add(5*time.Minute)) {
		t.Fatal("independent source or expired window rejected")
	}
}

func TestPasskeyBeginLimiterBoundedState(t *testing.T) {
	l := passkeyBeginLimiter{sources: make(map[string]attempt)}
	now := time.Unix(100, 0)
	const capacity = 16384 // same as takeAttempt
	for i := range capacity {
		if !l.Allow(strconv.Itoa(i), now) {
			t.Fatal("source table saturated before its capacity")
		}
	}
	if l.Allow("next", now) || len(l.sources) != capacity {
		t.Fatal("saturated table should reject new entries without growing")
	}
	if !l.Allow("0", now) {
		t.Fatal("existing source with remaining quota should still work")
	}
	if !l.Allow("next", now.Add(5*time.Minute)) || len(l.sources) != 1 {
		t.Fatal("expired entries should be reclaimed for a new source")
	}
}
