// Rate limiting for the credential-guessing surfaces: login, recovery, and
// registration finishes. In-memory sliding window per client IP — a single
// instance protecting a single user needs nothing fancier, it just must
// exist (recovery codes especially: 50-bit codes are strong against an
// online attacker only if the attacker can't try fast).
package auth

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	rateWindow   = 5 * time.Minute
	rateAttempts = 10
)

type rateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{attempts: map[string][]time.Time{}}
}

// allow records an attempt for the client and reports whether it is within
// budget. Old entries age out as they fall past the window.
func (rl *rateLimiter) allow(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	now := time.Now()
	cutoff := now.Add(-rateWindow)

	rl.mu.Lock()
	defer rl.mu.Unlock()
	recent := rl.attempts[host][:0]
	for _, t := range rl.attempts[host] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= rateAttempts {
		rl.attempts[host] = recent
		return false
	}
	rl.attempts[host] = append(recent, now)
	return true
}

// limited wraps an auth handler with the rate check, answering 429 with
// Retry-After per the house baseline.
func (h *HTTPConfig) limited(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.limiter.allow(r.RemoteAddr) {
			w.Header().Set("Retry-After", "300")
			authError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many attempts — try again in a few minutes")
			return
		}
		next(w, r)
	}
}
