package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimit returns a middleware that allows at most rps requests per second
// from a single host (identified by the remote IP). Excess requests receive
// HTTP 429 Too Many Requests.
func RateLimit(rps float64) func(http.Handler) http.Handler {
	rl := &rateLimiter{
		rps:     rps,
		buckets: make(map[string]*bucket),
	}
	go rl.cleanup()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := remoteIP(r)
			if !rl.allow(host) {
				http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ── token-bucket implementation ──────────────────────────────────────────────

type bucket struct {
	tokens   float64
	lastSeen time.Time
}

type rateLimiter struct {
	mu      sync.Mutex
	rps     float64
	buckets map[string]*bucket
}

func (rl *rateLimiter) allow(host string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[host]
	if !ok {
		b = &bucket{tokens: rl.rps, lastSeen: now}
		rl.buckets[host] = b
	}

	// Refill tokens proportional to elapsed time.
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.tokens += elapsed * rl.rps
	if b.tokens > rl.rps {
		b.tokens = rl.rps
	}
	b.lastSeen = now

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// cleanup removes stale buckets every minute to prevent unbounded memory growth.
func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		rl.mu.Lock()
		for host, b := range rl.buckets {
			if time.Since(b.lastSeen) > 5*time.Minute {
				delete(rl.buckets, host)
			}
		}
		rl.mu.Unlock()
	}
}

func remoteIP(r *http.Request) string {
	// Prefer X-Forwarded-For when running behind a proxy.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
