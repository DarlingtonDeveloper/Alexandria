package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/warrentherabbit/alexandria/internal/briefings"
	"github.com/warrentherabbit/alexandria/internal/hermes"
	"github.com/warrentherabbit/alexandria/internal/middleware"
	"github.com/warrentherabbit/alexandria/internal/store"
)

// BriefingHandler provides context rehydration endpoints.
type BriefingHandler struct {
	assembler *briefings.Assembler
	audit     *store.AuditStore
	publisher *hermes.Publisher
}

// NewBriefingHandler creates a new BriefingHandler.
func NewBriefingHandler(assembler *briefings.Assembler, audit *store.AuditStore, publisher *hermes.Publisher) *BriefingHandler {
	return &BriefingHandler{
		assembler: assembler,
		audit:     audit,
		publisher: publisher,
	}
}

// Generate handles GET /briefings/{agent_id}.
func (h *BriefingHandler) Generate(w http.ResponseWriter, r *http.Request) {
	requestingAgent := middleware.AgentIDFromContext(r.Context())
	targetAgent := chi.URLParam(r, "agent_id")

	// Parse query params
	since := time.Now().Add(-24 * time.Hour) // default: last 24 hours
	if v := r.URL.Query().Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			since = t
		}
	}

	maxItems := 50
	if v := r.URL.Query().Get("max_items"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			maxItems = n
		}
	}

	briefing, err := h.assembler.Generate(r.Context(), targetAgent, since, maxItems)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to generate briefing")
		return
	}

	h.audit.Log(r.Context(), store.ActionBriefingGen, requestingAgent, &targetAgent, nil, true, nil)

	if h.publisher != nil {
		itemCount := 0
		for _, s := range briefing.Content.Sections {
			itemCount += len(s.Items)
		}
		_ = h.publisher.BriefingGenerated(r.Context(), targetAgent, itemCount)
	}

	writeSuccess(w, http.StatusOK, briefing)
}
