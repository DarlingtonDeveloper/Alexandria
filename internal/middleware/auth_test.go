package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestAPIKeyAuth_DisabledWhenEmpty(t *testing.T) {
	h := APIKeyAuth("")(okHandler())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAPIKeyAuth_GETPassesThrough(t *testing.T) {
	h := APIKeyAuth("secret")(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAPIKeyAuth_HealthPassesThrough(t *testing.T) {
	h := APIKeyAuth("secret")(okHandler())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAPIKeyAuth_RejectsMissingKey(t *testing.T) {
	h := APIKeyAuth("secret")(okHandler())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAPIKeyAuth_RejectsWrongKey(t *testing.T) {
	h := APIKeyAuth("secret")(okHandler())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/", nil)
	req.Header.Set("X-API-Key", "wrong")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAPIKeyAuth_AcceptsValidKey(t *testing.T) {
	h := APIKeyAuth("secret")(okHandler())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/", nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAPIKeyAuth_PUTRequiresKey(t *testing.T) {
	h := APIKeyAuth("secret")(okHandler())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/knowledge/123", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAPIKeyAuth_DELETERequiresKey(t *testing.T) {
	h := APIKeyAuth("secret")(okHandler())
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/knowledge/123", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
