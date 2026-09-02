// Rate limiting for the credential-guessing surfaces: login, recovery, and
// registration finishes. In-memory sliding window per client IP — a single
// instance protecting a single user needs nothing fancier, it just must
// exist (recovery codes especially: 50-bit codes are strong against an
// online attacker only if the attacker can't try fast).
//
// "Per client IP" is doing real work here, so who the client is has to be
// right: behind a proxy every request shares one address, and one bucket
// for everybody means a stranger hammering /login locks the owner out. See
// clientip.go for how the forwarded address is established, and only
// trusted when it can be.
package auth

import (
	"net/http"
	"sync"
	"time"
)

const (
	rateWindow   = 5 * time.Minute
	rateAttempts = 10
)

// maxTrackedClients bounds the map. A public endpoint keyed by client
// address is otherwise an unbounded allocation: an attacker rotating source
// addresses grows it forever. At the cap the oldest-seen entries are dropped
// — the cost of that is one attacker occasionally getting a fresh budget,
// which is strictly better than running out of memory.
const maxTrackedClients = 4096

type rateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{attempts: map[string][]time.Time{}}
}

// allow records an attempt for client and reports whether it is within
// budget. Old entries age out as they fall past the window.
func (rl *rateLimiter) allow(client string) bool {
	host := client
	now := time.Now()
	cutoff := now.Add(-rateWindow)

	rl.mu.Lock()
	defer rl.mu.Unlock()
	if len(rl.attempts) >= maxTrackedClients {
		rl.evictStale(cutoff)
	}
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

// evictStale drops clients with no attempt inside the window, and if that
// frees nothing (every tracked client is currently active) clears the map
// outright rather than growing without bound. Callers hold the lock.
func (rl *rateLimiter) evictStale(cutoff time.Time) {
	for client, times := range rl.attempts {
		if len(times) == 0 || times[len(times)-1].Before(cutoff) {
			delete(rl.attempts, client)
		}
	}
	if len(rl.attempts) >= maxTrackedClients {
		clear(rl.attempts)
	}
}

// limited wraps an auth handler with the rate check, answering 429 with
// Retry-After per the house baseline.
func (h *HTTPConfig) limited(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.limiter.allow(ClientIP(r, h.TrustedProxies)) {
			w.Header().Set("Retry-After", "300")
			authError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many attempts — try again in a few minutes")
			return
		}
		next(w, r)
	}
}
