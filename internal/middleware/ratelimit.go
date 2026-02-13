package middleware

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// RateLimiter provides per-agent rate limiting.
type RateLimiter struct {
	mu       sync.Mutex
	limits   map[string][]time.Time
	maxReqs  int
	window   time.Duration
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(maxReqs int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		limits:  make(map[string][]time.Time),
		maxReqs: maxReqs,
		window:  window,
	}
}

// Middleware returns an HTTP middleware that enforces rate limits per agent.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agentID := AgentIDFromContext(r.Context())
		if agentID == "" {
			agentID = r.RemoteAddr
		}

		if !rl.allow(agentID) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    "RATE_LIMITED",
					"message": "Too many requests. Try again later.",
				},
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Clean old entries
	times := rl.limits[key]
	var valid []time.Time
	for _, t := range times {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.maxReqs {
		rl.limits[key] = valid
		return false
	}

	rl.limits[key] = append(valid, now)
	return true
}
