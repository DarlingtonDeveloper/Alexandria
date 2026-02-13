package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MikeSquared-Agency/Alexandria/internal/identity"
	"github.com/MikeSquared-Agency/Alexandria/internal/middleware"
	"github.com/MikeSquared-Agency/Alexandria/internal/store"
)

// IdentityHandler provides identity resolution endpoints.
type IdentityHandler struct {
	resolver *identity.Resolver
	db       *store.DB
	audit    *store.AuditStore
}

// NewIdentityHandler creates a new IdentityHandler.
func NewIdentityHandler(resolver *identity.Resolver, db *store.DB, audit *store.AuditStore) *IdentityHandler {
	return &IdentityHandler{resolver: resolver, db: db, audit: audit}
}

// Resolve handles POST /api/v1/identity/resolve.
func (h *IdentityHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())

	var req identity.ResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	result, err := h.resolver.Resolve(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "RESOLVE_ERROR", err.Error())
		return
	}

	h.audit.Log(r.Context(), store.ActionIdentityResolve, agentID, nil, nil, true, map[string]any{
		"alias_type":  req.AliasType,
		"alias_value": req.AliasValue,
		"outcome":     result.Outcome,
	})

	writeSuccess(w, http.StatusOK, result)
}

// Merge handles POST /api/v1/identity/merge.
func (h *IdentityHandler) Merge(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())

	var req struct {
		SurvivorID uuid.UUID `json:"survivor_id"`
		MergedID   uuid.UUID `json:"merged_id"`
		ApprovedBy string    `json:"approved_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	if req.ApprovedBy == "" {
		req.ApprovedBy = agentID
	}

	result, err := h.resolver.Merge(r.Context(), req.SurvivorID, req.MergedID, req.ApprovedBy)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "MERGE_ERROR", err.Error())
		return
	}

	h.audit.Log(r.Context(), store.ActionIdentityMerge, agentID, nil, nil, true, map[string]any{
		"survivor_id": req.SurvivorID,
		"merged_id":   req.MergedID,
	})

	writeSuccess(w, http.StatusOK, result)
}

// Pending handles GET /api/v1/identity/pending.
func (h *IdentityHandler) Pending(w http.ResponseWriter, r *http.Request) {
	aliases, err := store.PendingReviews(r.Context(), h.db.DBTX())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list pending reviews")
		return
	}
	if aliases == nil {
		aliases = []store.Alias{}
	}
	writeSuccess(w, http.StatusOK, aliases)
}

// ReviewAlias handles POST /api/v1/identity/aliases/{id}/review.
func (h *IdentityHandler) ReviewAlias(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid alias ID")
		return
	}

	var req struct {
		Approved bool `json:"approved"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	if err := store.MarkReviewed(r.Context(), h.db.DBTX(), id, req.Approved); err != nil {
		writeError(w, http.StatusInternalServerError, "REVIEW_ERROR", err.Error())
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{"reviewed": true, "approved": req.Approved})
}

// EntityLookup handles GET /api/v1/identity/entities/{id}.
func (h *IdentityHandler) EntityLookup(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid entity ID")
		return
	}

	entity, err := store.GetEntityTx(r.Context(), h.db.DBTX(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Entity not found")
		return
	}

	aliases, err := store.ListAliasesByCanonical(r.Context(), h.db.DBTX(), id)
	if err != nil {
		aliases = []store.Alias{}
	}

	edgesFrom, err := store.EdgesFrom(r.Context(), h.db.DBTX(), id)
	if err != nil {
		edgesFrom = []store.CGEdge{}
	}

	edgesTo, err := store.EdgesTo(r.Context(), h.db.DBTX(), id)
	if err != nil {
		edgesTo = []store.CGEdge{}
	}

	writeSuccess(w, http.StatusOK, map[string]any{
		"entity":     entity,
		"aliases":    aliases,
		"edges_from": edgesFrom,
		"edges_to":   edgesTo,
	})
}
