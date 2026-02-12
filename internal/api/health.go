// Package api provides HTTP handlers for the Alexandria REST API.
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/warrentherabbit/alexandria/internal/hermes"
	"github.com/warrentherabbit/alexandria/internal/store"
)

// HealthHandler provides health and stats endpoints.
type HealthHandler struct {
	db        *store.DB
	knowledge *store.KnowledgeStore
	secrets   *store.SecretStore
	hermes    *hermes.Client
	startTime time.Time
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(db *store.DB, knowledge *store.KnowledgeStore, secrets *store.SecretStore, hermesClient *hermes.Client) *HealthHandler {
	return &HealthHandler{
		db:        db,
		knowledge: knowledge,
		secrets:   secrets,
		hermes:    hermesClient,
		startTime: time.Now(),
	}
}

// Health returns the service health status.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	supabaseStatus := "connected"
	if err := h.db.HealthCheck(ctx); err != nil {
		supabaseStatus = "disconnected"
	}

	hermesStatus := "disconnected"
	if h.hermes != nil && h.hermes.IsConnected() {
		hermesStatus = "connected"
	}

	knowledgeCount, _ := h.knowledge.Count(ctx)

	resp := map[string]any{
		"status":          "healthy",
		"supabase":        supabaseStatus,
		"hermes":          hermesStatus,
		"knowledge_count": knowledgeCount,
		"uptime_seconds":  int(time.Since(h.startTime).Seconds()),
	}

	if supabaseStatus == "disconnected" {
		resp["status"] = "degraded"
	}

	writeJSON(w, http.StatusOK, resp)
}

// Stats returns detailed service statistics.
func (h *HealthHandler) Stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	knowledgeCount, _ := h.knowledge.Count(ctx)
	secretCount, _ := h.secrets.Count(ctx)

	writeJSON(w, http.StatusOK, map[string]any{
		"knowledge_count": knowledgeCount,
		"secret_count":    secretCount,
		"uptime_seconds":  int(time.Since(h.startTime).Seconds()),
	})
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
		"meta": map[string]any{
			"timestamp": time.Now().Format(time.RFC3339),
		},
	})
}

// writeSuccess writes a standard success response.
func writeSuccess(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, map[string]any{
		"data": data,
		"meta": map[string]any{
			"timestamp": time.Now().Format(time.RFC3339),
		},
	})
}
