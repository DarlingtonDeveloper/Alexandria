package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/warrentherabbit/alexandria/internal/middleware"
)

func TestAgentAuth_SetsAgentID(t *testing.T) {
	handler := middleware.AgentAuth("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agentID := middleware.AgentIDFromContext(r.Context())
		w.Write([]byte(agentID))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Agent-ID", "kai")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Body.String() != "kai" {
		t.Errorf("expected 'kai', got '%s'", rec.Body.String())
	}
}

func TestAgentAuth_DefaultsToAnonymous(t *testing.T) {
	handler := middleware.AgentAuth("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agentID := middleware.AgentIDFromContext(r.Context())
		w.Write([]byte(agentID))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Body.String() != "anonymous" {
		t.Errorf("expected 'anonymous', got '%s'", rec.Body.String())
	}
}

func TestRateLimiter(t *testing.T) {
	rl := middleware.NewRateLimiter(3, 60_000_000_000) // 3 req/min

	// Set up a handler that uses the rate limiter
	handler := middleware.AgentAuth("")(rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	// First 3 requests should succeed
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Agent-ID", "test-agent")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i, rec.Code)
		}
	}

	// 4th request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Agent-ID", "test-agent")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rec.Code)
	}

	// Different agent should not be rate limited
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Agent-ID", "other-agent")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("different agent should not be rate limited, got %d", rec.Code)
	}
}
