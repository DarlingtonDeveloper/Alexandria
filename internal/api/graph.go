package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/MikeSquared-Agency/Alexandria/internal/middleware"
	"github.com/MikeSquared-Agency/Alexandria/internal/store"
)

// GraphHandler provides knowledge graph endpoints.
type GraphHandler struct {
	graph *store.GraphStore
	audit *store.AuditStore
}

// NewGraphHandler creates a new GraphHandler.
func NewGraphHandler(graph *store.GraphStore, audit *store.AuditStore) *GraphHandler {
	return &GraphHandler{graph: graph, audit: audit}
}

// ListEntities handles GET /graph/entities.
func (h *GraphHandler) ListEntities(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())
	q := r.URL.Query()

	var entityType *store.EntityType
	if v := q.Get("type"); v != "" {
		et := store.EntityType(v)
		entityType = &et
	}

	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = n
		}
	}

	entities, err := h.graph.ListEntities(r.Context(), entityType, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list entities")
		return
	}

	_ = h.audit.Log(r.Context(), store.ActionGraphRead, agentID, nil, nil, true, nil)
	writeSuccess(w, http.StatusOK, entities)
}

// GetEntity handles GET /graph/entities/{id}.
func (h *GraphHandler) GetEntity(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())
	id := chi.URLParam(r, "id")

	entity, err := h.graph.GetEntity(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get entity")
		return
	}
	if entity == nil {
		writeError(w, http.StatusNotFound, "ENTITY_NOT_FOUND", "No entity with ID '"+id+"'")
		return
	}

	rels, err := h.graph.GetRelationships(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get relationships")
		return
	}

	_ = h.audit.Log(r.Context(), store.ActionGraphRead, agentID, &id, nil, true, nil)
	writeSuccess(w, http.StatusOK, map[string]any{
		"entity":        entity,
		"relationships": rels,
	})
}

// GetRelatedEntities handles GET /graph/entities/{id}/related.
func (h *GraphHandler) GetRelatedEntities(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())
	id := chi.URLParam(r, "id")

	depth := 2
	if v := r.URL.Query().Get("depth"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			depth = n
		}
	}

	entities, rels, err := h.graph.GetRelatedEntities(r.Context(), id, depth)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get related entities")
		return
	}

	_ = h.audit.Log(r.Context(), store.ActionGraphRead, agentID, &id, nil, true, nil)
	writeSuccess(w, http.StatusOK, map[string]any{
		"entities":      entities,
		"relationships": rels,
	})
}

// EntityCreateRequest is the request body for creating an entity.
type EntityCreateRequest struct {
	Name       string         `json:"name"`
	EntityType store.EntityType `json:"entity_type"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// CreateEntity handles POST /graph/entities.
func (h *GraphHandler) CreateEntity(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())

	var req EntityCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Name is required")
		return
	}

	entity, err := h.graph.CreateEntity(r.Context(), req.Name, req.EntityType, req.Metadata)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create entity")
		return
	}

	_ = h.audit.Log(r.Context(), store.ActionGraphWrite, agentID, &entity.ID, nil, true, nil)
	writeSuccess(w, http.StatusCreated, entity)
}

// RelationshipCreateRequest is the request body for creating a relationship.
type RelationshipCreateRequest struct {
	SourceEntityID   string  `json:"source_entity_id"`
	TargetEntityID   string  `json:"target_entity_id"`
	RelationshipType string  `json:"relationship_type"`
	Strength         float64 `json:"strength"`
	KnowledgeID      *string `json:"knowledge_id,omitempty"`
}

// CreateRelationship handles POST /graph/relationships.
func (h *GraphHandler) CreateRelationship(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())

	var req RelationshipCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	if req.SourceEntityID == "" || req.TargetEntityID == "" || req.RelationshipType == "" {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Source, target, and type are required")
		return
	}

	if req.Strength <= 0 {
		req.Strength = 1.0
	}

	rel, err := h.graph.CreateRelationship(r.Context(), req.SourceEntityID, req.TargetEntityID, req.RelationshipType, req.Strength, req.KnowledgeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create relationship")
		return
	}

	_ = h.audit.Log(r.Context(), store.ActionGraphWrite, agentID, &rel.ID, nil, true, nil)
	writeSuccess(w, http.StatusCreated, rel)
}
