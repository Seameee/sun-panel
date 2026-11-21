package security

import (
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type attempt struct {
	count   int
	expires time.Time
}

type attemptLimiter struct {
	mu                sync.Mutex
	accounts, sources map[string]attempt
	sweepAt           time.Time
}

var PasswordAttempts = attemptLimiter{accounts: make(map[string]attempt), sources: make(map[string]attempt)}

// PasswordSource uses the socket peer unless Gin has an explicitly trusted proxy.
func PasswordSource(c *gin.Context) string {
	source := c.ClientIP()
	if ip := net.ParseIP(source); ip != nil {
		if ip.To4() == nil {
			return ip.Mask(net.CIDRMask(64, 128)).String()
		}
		return ip.String()
	}
	return source
}

func (l *attemptLimiter) Allow(account, source string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !now.Before(l.sweepAt) {
		for _, table := range []map[string]attempt{l.accounts, l.sources} {
			for key, entry := range table {
				if !now.Before(entry.expires) {
					delete(table, key)
				}
			}
		}
		l.sweepAt = now.Add(time.Minute)
	}
	account = strings.ToLower(strings.TrimSpace(account))
	if !takeAttempt(l.sources, source, 60, now) {
		return false
	}
	return takeAttempt(l.accounts, account, 12, now)
}

func takeAttempt(table map[string]attempt, key string, limit int, now time.Time) bool {
	entry, exists := table[key]
	if !exists && len(table) >= 16384 {
		return false
	}
	if !now.Before(entry.expires) {
		entry = attempt{expires: now.Add(5 * time.Minute)}
	}
	if entry.count >= limit {
		return false
	}
	entry.count++
	table[key] = entry
	return true
}
