// Package middleware provides HTTP middleware for Alexandria.
package middleware

import (
	"context"
	"net/http"
)

// contextKey is a private type for context keys.
type contextKey string

const agentIDKey contextKey = "agent_id"

// AgentIDFromContext extracts the agent ID from the request context.
func AgentIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(agentIDKey).(string); ok {
		return v
	}
	return ""
}

// APIKeyAuth requires a valid X-API-Key header on mutating requests (POST/PUT/DELETE).
// GET requests and /api/v1/health are exempt. Disabled when apiKey is empty.
func APIKeyAuth(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if apiKey == "" || r.Method == http.MethodGet || r.URL.Path == "/api/v1/health" {
				next.ServeHTTP(w, r)
				return
			}
			if r.Header.Get("X-API-Key") != apiKey {
				http.Error(w, `{"error":{"code":"unauthorized","message":"invalid or missing API key"}}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AgentAuth extracts the agent ID from X-Agent-ID header and injects it into context.
// Phase 1: trust-based (overlay network only). Phase 2: JWT verification.
func AgentAuth(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			agentID := r.Header.Get("X-Agent-ID")
			if agentID == "" {
				agentID = "anonymous"
			}

			// Phase 2: verify JWT from Authorization header
			// For now, trust X-Agent-ID (internal overlay traffic only)

			ctx := context.WithValue(r.Context(), agentIDKey, agentID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
