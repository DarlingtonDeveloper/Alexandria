package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MikeSquared-Agency/Alexandria/internal/store"
)

// SemanticHandler provides semantic search and clustering endpoints.
type SemanticHandler struct {
	db *store.DB
}

// NewSemanticHandler creates a new SemanticHandler.
func NewSemanticHandler(db *store.DB) *SemanticHandler {
	return &SemanticHandler{db: db}
}

// Status handles GET /api/v1/semantic/status.
func (h *SemanticHandler) Status(w http.ResponseWriter, r *http.Request) {
	db := h.db.DBTX()

	var entitiesTotal, entitiesEmbedded, clustersActive, proposalsPending int

	h.db.Pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM vault_entities WHERE deleted_at IS NULL`).Scan(&entitiesTotal)
	h.db.Pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM vault_entity_embeddings`).Scan(&entitiesEmbedded)

	clusters, _ := store.ListActiveClusters(r.Context(), db)
	clustersActive = len(clusters)

	proposals, _ := store.PendingMergeProposals(r.Context(), db)
	proposalsPending = len(proposals)

	writeSuccess(w, http.StatusOK, map[string]any{
		"entities_total":    entitiesTotal,
		"entities_embedded": entitiesEmbedded,
		"clusters_active":   clustersActive,
		"proposals_pending": proposalsPending,
		"embedding_gap":     entitiesTotal - entitiesEmbedded,
	})
}

// SimilarEntities handles GET /api/v1/semantic/similar/{id}.
func (h *SemanticHandler) SimilarEntities(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid entity ID")
		return
	}

	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	minSimilarity := 0.7
	if v := r.URL.Query().Get("min_similarity"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			minSimilarity = f
		}
	}

	similar, err := store.FindSimilarToEntity(r.Context(), h.db.DBTX(), id, limit, minSimilarity)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to find similar entities")
		return
	}
	if similar == nil {
		similar = []store.SimilarEntity{}
	}

	writeSuccess(w, http.StatusOK, similar)
}

// ListClusters handles GET /api/v1/semantic/clusters.
func (h *SemanticHandler) ListClusters(w http.ResponseWriter, r *http.Request) {
	clusters, err := store.ListActiveClusters(r.Context(), h.db.DBTX())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list clusters")
		return
	}
	if clusters == nil {
		clusters = []store.SemanticCluster{}
	}

	writeSuccess(w, http.StatusOK, clusters)
}

// ClusterMembers handles GET /api/v1/semantic/clusters/{id}/members.
func (h *SemanticHandler) ClusterMembers(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid cluster ID")
		return
	}

	members, err := store.ClusterMembers(r.Context(), h.db.DBTX(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list cluster members")
		return
	}
	if members == nil {
		members = []store.ClusterMembership{}
	}

	writeSuccess(w, http.StatusOK, members)
}

// EntityClusters handles GET /api/v1/semantic/entities/{id}/clusters.
func (h *SemanticHandler) EntityClusters(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid entity ID")
		return
	}

	clusters, err := store.EntityClusters(r.Context(), h.db.DBTX(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list entity clusters")
		return
	}
	if clusters == nil {
		clusters = []store.SemanticCluster{}
	}

	writeSuccess(w, http.StatusOK, clusters)
}

// Proposals handles GET /api/v1/semantic/proposals.
func (h *SemanticHandler) Proposals(w http.ResponseWriter, r *http.Request) {
	proposals, err := store.PendingMergeProposals(r.Context(), h.db.DBTX())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list proposals")
		return
	}
	if proposals == nil {
		proposals = []store.MergeProposal{}
	}

	writeSuccess(w, http.StatusOK, proposals)
}

// ReviewProposal handles POST /api/v1/semantic/proposals/{id}/review.
func (h *SemanticHandler) ReviewProposal(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid proposal ID")
		return
	}

	var req struct {
		Status     string `json:"status"`
		ReviewedBy string `json:"reviewed_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	if req.Status != "approved" && req.Status != "rejected" {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Status must be 'approved' or 'rejected'")
		return
	}

	if err := store.ResolveMergeProposal(r.Context(), h.db.DBTX(), id, req.Status, req.ReviewedBy); err != nil {
		writeError(w, http.StatusInternalServerError, "REVIEW_ERROR", err.Error())
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{"reviewed": true, "status": req.Status})
}
