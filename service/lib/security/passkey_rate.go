package security

import (
	"sync"
	"time"
)

const passkeyBeginSourceLimit = 30

// PasskeyBeginAttempts tracks public Passkey initiation independently of
// password attempts, using the same bounded five-minute attempt windows.
var PasskeyBeginAttempts = passkeyBeginLimiter{sources: make(map[string]attempt)}

type passkeyBeginLimiter struct {
	mu      sync.Mutex
	sources map[string]attempt
	sweepAt time.Time
}

func (l *passkeyBeginLimiter) Allow(source string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !now.Before(l.sweepAt) {
		for key, entry := range l.sources {
			if !now.Before(entry.expires) {
				delete(l.sources, key)
			}
		}
		l.sweepAt = now.Add(time.Minute)
	}
	return takeAttempt(l.sources, source, passkeyBeginSourceLimit, now)
}
