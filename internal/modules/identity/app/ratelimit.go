package app

import (
	"sync"
	"time"
)

// loginLimiter is a simple in-process, per-key (email) exponential
// backoff limiter (brief §27 "login throttling", §63 rate limiting on
// login). In-process because Stage 2 targets a single-replica deployment
// (docs/architecture.md §16); once apps/server runs more than one
// replica, this must move to a shared store (Redis, or a Postgres table)
// or an attacker can simply round-robin across replicas to bypass it —
// that migration is flagged here rather than silently left as a latent
// gap.
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]*attemptState
}

type attemptState struct {
	failures    int
	lockedUntil time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: make(map[string]*attemptState)}
}

// Allow reports whether key (typically a lowercased email) may attempt a
// login right now.
func (l *loginLimiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	st, ok := l.attempts[key]
	if !ok {
		return true
	}
	return now.After(st.lockedUntil)
}

// RecordFailure increments the failure count and, once past a threshold,
// locks the key out for an exponentially increasing duration (capped).
func (l *loginLimiter) RecordFailure(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	st, ok := l.attempts[key]
	if !ok {
		st = &attemptState{}
		l.attempts[key] = st
	}
	st.failures++
	const freeAttempts = 5
	if st.failures > freeAttempts {
		backoff := time.Duration(1<<uint(st.failures-freeAttempts-1)) * time.Second
		const maxBackoff = 15 * time.Minute
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
		st.lockedUntil = now.Add(backoff)
	}
}

// RecordSuccess clears a key's failure history.
func (l *loginLimiter) RecordSuccess(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}
