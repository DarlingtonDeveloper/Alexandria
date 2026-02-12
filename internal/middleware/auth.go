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
