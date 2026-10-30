package accounts

import (
	"strings"
	"sync"
	"time"
)

const (
	loginWindow          = 15 * time.Minute
	loginFailuresPerUser = 8
	loginFailuresPerIP   = 30
)

// loginLimiter counts recent failures per username+IP and per IP. It is
// in-memory: a restart resets it, which is acceptable for a small team.
type loginLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{failures: make(map[string][]time.Time)}
}

func limiterKeys(username, ip string) (string, string) {
	return "u:" + strings.ToLower(strings.TrimSpace(username)) + "|" + ip, "ip:" + ip
}

func (l *loginLimiter) recent(key string, now time.Time) []time.Time {
	kept := l.failures[key][:0]
	for _, at := range l.failures[key] {
		if now.Sub(at) < loginWindow {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(l.failures, key)
		return nil
	}
	l.failures[key] = kept
	return kept
}

func (l *loginLimiter) allow(username, ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	userKey, ipKey := limiterKeys(username, ip)
	return len(l.recent(userKey, now)) < loginFailuresPerUser && len(l.recent(ipKey, now)) < loginFailuresPerIP
}

func (l *loginLimiter) fail(username, ip string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	userKey, ipKey := limiterKeys(username, ip)
	l.failures[userKey] = append(l.recent(userKey, now), now)
	l.failures[ipKey] = append(l.recent(ipKey, now), now)
}

func (l *loginLimiter) succeed(username, ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	userKey, _ := limiterKeys(username, ip)
	delete(l.failures, userKey)
}
