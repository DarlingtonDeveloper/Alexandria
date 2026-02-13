package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/MikeSquared-Agency/Alexandria/internal/bootctx"
	"github.com/MikeSquared-Agency/Alexandria/internal/hermes"
	"github.com/MikeSquared-Agency/Alexandria/internal/middleware"
	"github.com/MikeSquared-Agency/Alexandria/internal/store"
)

// ContextHandler provides the boot-context endpoint.
type ContextHandler struct {
	assembler *bootctx.Assembler
	audit     *store.AuditStore
	publisher *hermes.Publisher
	logger    *slog.Logger
}

// NewContextHandler creates a new ContextHandler.
func NewContextHandler(assembler *bootctx.Assembler, audit *store.AuditStore, publisher *hermes.Publisher, logger *slog.Logger) *ContextHandler {
	return &ContextHandler{
		assembler: assembler,
		audit:     audit,
		publisher: publisher,
		logger:    logger,
	}
}

// Generate handles GET /context/{agent_id}.
func (h *ContextHandler) Generate(w http.ResponseWriter, r *http.Request) {
	requestingAgent := middleware.AgentIDFromContext(r.Context())
	targetAgent := chi.URLParam(r, "agent_id")

	md, err := h.assembler.Generate(r.Context(), targetAgent)
	if err != nil {
		h.logger.Error("boot context generation failed", "agent_id", targetAgent, "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to generate boot context")
		return
	}

	_ = h.audit.Log(r.Context(), store.ActionContextGen, requestingAgent, &targetAgent, nil, true, nil)

	if h.publisher != nil {
		_ = h.publisher.ContextGenerated(r.Context(), targetAgent)
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(md))
}
