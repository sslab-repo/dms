package auth

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// loginRateLimiter enforces per-IP attempt limits on the login endpoint.
// It uses a token-bucket approach: each IP gets 5 attempts per 15 minutes.
type loginRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens    int
	lastRefil time.Time
}

const (
	maxLoginAttempts  = 5
	loginWindowSecs   = 15 * 60 // 15 minutes
)

var loginLimiter = &loginRateLimiter{
	buckets: make(map[string]*bucket),
}

func (l *loginRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[ip]
	if !ok {
		l.buckets[ip] = &bucket{tokens: maxLoginAttempts - 1, lastRefil: now}
		return true
	}

	// Refill if window has passed.
	if now.Sub(b.lastRefil) >= time.Duration(loginWindowSecs)*time.Second {
		b.tokens = maxLoginAttempts - 1
		b.lastRefil = now
		return true
	}

	if b.tokens <= 0 {
		return false
	}
	b.tokens--
	return true
}

// RateLimitLogin wraps a handler and blocks IPs that exceed the login attempt limit.
func RateLimitLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if !loginLimiter.allow(ip) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{
				"error": "too many login attempts, try again later",
			})
			return
		}
		next(w, r)
	}
}
