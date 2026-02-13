package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/MikeSquared-Agency/Alexandria/internal/embeddings"
	"github.com/MikeSquared-Agency/Alexandria/internal/hermes"
	"github.com/MikeSquared-Agency/Alexandria/internal/middleware"
	"github.com/MikeSquared-Agency/Alexandria/internal/store"
)

// KnowledgeHandler provides knowledge CRUD and search endpoints.
type KnowledgeHandler struct {
	knowledge *store.KnowledgeStore
	audit     *store.AuditStore
	embedder  embeddings.Provider
	publisher *hermes.Publisher
}

// NewKnowledgeHandler creates a new KnowledgeHandler.
func NewKnowledgeHandler(knowledge *store.KnowledgeStore, audit *store.AuditStore, embedder embeddings.Provider, publisher *hermes.Publisher) *KnowledgeHandler {
	return &KnowledgeHandler{
		knowledge: knowledge,
		audit:     audit,
		embedder:  embedder,
		publisher: publisher,
	}
}

// CreateRequest is the request body for creating knowledge.
type CreateRequest struct {
	Content        string                  `json:"content"`
	Summary        *string                 `json:"summary,omitempty"`
	Category       store.KnowledgeCategory `json:"category"`
	Scope          store.KnowledgeScope    `json:"scope"`
	SharedWith     []string                `json:"shared_with,omitempty"`
	Tags           []string                `json:"tags,omitempty"`
	Metadata       map[string]any          `json:"metadata,omitempty"`
	Confidence     *float64                `json:"confidence,omitempty"`
	RelevanceDecay *store.RelevanceDecay   `json:"relevance_decay,omitempty"`
}

// Create handles POST /knowledge.
func (h *KnowledgeHandler) Create(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())

	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	if req.Content == "" {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Content is required")
		return
	}
	if len(req.Content) > 102400 {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Content exceeds 100KB limit")
		return
	}

	confidence := 0.8
	if req.Confidence != nil {
		confidence = *req.Confidence
	}
	decay := store.DecaySlow
	if req.RelevanceDecay != nil {
		decay = *req.RelevanceDecay
	}
	if req.Scope == "" {
		req.Scope = store.ScopePublic
	}
	if req.Category == "" {
		req.Category = store.CategoryDiscovery
	}

	// Generate embedding
	embedding, err := h.embedder.Embed(r.Context(), req.Content)
	if err != nil {
		// Log but don't fail — store without embedding
		_ = err
	}

	input := store.KnowledgeCreateInput{
		Content:        req.Content,
		Summary:        req.Summary,
		SourceAgent:    agentID,
		Category:       req.Category,
		Scope:          req.Scope,
		SharedWith:     req.SharedWith,
		Tags:           req.Tags,
		Embedding:      embedding,
		Metadata:       req.Metadata,
		Confidence:     confidence,
		RelevanceDecay: decay,
	}

	entry, err := h.knowledge.Create(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create knowledge entry")
		return
	}

	// Audit
	_ = h.audit.Log(r.Context(), store.ActionKnowledgeWrite, agentID, &entry.ID, nil, true, nil)

	// Publish event
	if h.publisher != nil {
		_ = h.publisher.KnowledgeCreated(r.Context(), entry)
	}

	writeSuccess(w, http.StatusCreated, entry)
}

// List handles GET /knowledge.
func (h *KnowledgeHandler) List(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())
	q := r.URL.Query()

	filter := store.KnowledgeFilter{
		AgentID: agentID,
		Limit:   50,
	}

	if v := q.Get("category"); v != "" {
		cat := store.KnowledgeCategory(v)
		filter.Category = &cat
	}
	if v := q.Get("scope"); v != "" {
		scope := store.KnowledgeScope(v)
		filter.Scope = &scope
	}
	if v := q.Get("source_agent"); v != "" {
		filter.SourceAgent = &v
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Offset = n
		}
	}

	entries, err := h.knowledge.List(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list knowledge")
		return
	}

	_ = h.audit.Log(r.Context(), store.ActionKnowledgeRead, agentID, nil, nil, true, nil)
	writeSuccess(w, http.StatusOK, entries)
}

// Get handles GET /knowledge/{id}.
func (h *KnowledgeHandler) Get(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())
	id := chi.URLParam(r, "id")

	entry, err := h.knowledge.GetByID(r.Context(), id, agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get knowledge entry")
		return
	}
	if entry == nil {
		writeError(w, http.StatusNotFound, "KNOWLEDGE_NOT_FOUND", "No knowledge entry with ID '"+id+"'")
		return
	}

	_ = h.audit.Log(r.Context(), store.ActionKnowledgeRead, agentID, &id, nil, true, nil)
	writeSuccess(w, http.StatusOK, entry)
}

// Update handles PUT /knowledge/{id}.
func (h *KnowledgeHandler) Update(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())
	id := chi.URLParam(r, "id")

	var input store.KnowledgeUpdateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	// Re-generate embedding if content changed
	if input.Content != nil {
		emb, err := h.embedder.Embed(r.Context(), *input.Content)
		if err == nil {
			input.Embedding = &emb
		}
	}

	entry, err := h.knowledge.Update(r.Context(), id, agentID, input)
	if err != nil {
		if err.Error() == "access denied: only owner or admin can update" {
			writeError(w, http.StatusForbidden, "ACCESS_DENIED", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update knowledge entry")
		return
	}
	if entry == nil {
		writeError(w, http.StatusNotFound, "KNOWLEDGE_NOT_FOUND", "No knowledge entry with ID '"+id+"'")
		return
	}

	_ = h.audit.Log(r.Context(), store.ActionKnowledgeWrite, agentID, &id, nil, true, nil)

	if h.publisher != nil {
		_ = h.publisher.KnowledgeUpdated(r.Context(), entry)
	}

	writeSuccess(w, http.StatusOK, entry)
}

// Delete handles DELETE /knowledge/{id}.
func (h *KnowledgeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())
	id := chi.URLParam(r, "id")

	if err := h.knowledge.Delete(r.Context(), id, agentID); err != nil {
		if err.Error() == "access denied" {
			writeError(w, http.StatusForbidden, "ACCESS_DENIED", "Only the owner or admin can delete this entry")
			return
		}
		if err.Error() == "not found" {
			writeError(w, http.StatusNotFound, "KNOWLEDGE_NOT_FOUND", "No knowledge entry with ID '"+id+"'")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete knowledge entry")
		return
	}

	_ = h.audit.Log(r.Context(), store.ActionKnowledgeDelete, agentID, &id, nil, true, nil)
	writeSuccess(w, http.StatusOK, map[string]string{"deleted": id})
}

// SearchRequest is the request body for semantic search.
type SearchRequest struct {
	Query          string                    `json:"query"`
	Limit          int                       `json:"limit,omitempty"`
	Scope          *store.KnowledgeScope     `json:"scope,omitempty"`
	Categories     []store.KnowledgeCategory `json:"categories,omitempty"`
	MinRelevance   float64                   `json:"min_relevance,omitempty"`
	IncludeExpired bool                      `json:"include_expired,omitempty"`
}

// Search handles POST /knowledge/search.
func (h *KnowledgeHandler) Search(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())

	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	if req.Query == "" {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Query is required")
		return
	}

	// Generate query embedding
	queryEmbedding, err := h.embedder.Embed(r.Context(), req.Query)
	if err != nil {
		writeError(w, http.StatusBadGateway, "EMBEDDING_FAILED", "Failed to generate query embedding")
		return
	}

	results, err := h.knowledge.Search(r.Context(), store.SearchInput{
		QueryEmbedding: queryEmbedding,
		Limit:          req.Limit,
		Scope:          req.Scope,
		Categories:     req.Categories,
		AgentID:        agentID,
		MinRelevance:   req.MinRelevance,
		IncludeExpired: req.IncludeExpired,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Search failed")
		return
	}

	_ = h.audit.Log(r.Context(), store.ActionKnowledgeSearch, agentID, nil, nil, true, map[string]any{
		"query":        req.Query,
		"result_count": len(results),
	})

	if h.publisher != nil {
		_ = h.publisher.KnowledgeSearched(r.Context(), agentID, req.Query, len(results))
	}

	writeSuccess(w, http.StatusOK, map[string]any{
		"results":       results,
		"total_results": len(results),
	})
}

// BatchCreateRequest is the request body for batch knowledge creation.
type BatchCreateRequest struct {
	Entries []CreateRequest `json:"entries"`
}

// BatchCreate handles POST /knowledge/batch.
func (h *KnowledgeHandler) BatchCreate(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())

	var req BatchCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	if len(req.Entries) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "At least one entry is required")
		return
	}
	if len(req.Entries) > 100 {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Maximum 100 entries per batch")
		return
	}

	var created []*store.KnowledgeEntry
	for _, entry := range req.Entries {
		confidence := 0.8
		if entry.Confidence != nil {
			confidence = *entry.Confidence
		}
		decay := store.DecaySlow
		if entry.RelevanceDecay != nil {
			decay = *entry.RelevanceDecay
		}

		embedding, _ := h.embedder.Embed(r.Context(), entry.Content)

		input := store.KnowledgeCreateInput{
			Content:        entry.Content,
			Summary:        entry.Summary,
			SourceAgent:    agentID,
			Category:       entry.Category,
			Scope:          entry.Scope,
			SharedWith:     entry.SharedWith,
			Tags:           entry.Tags,
			Embedding:      embedding,
			Metadata:       entry.Metadata,
			Confidence:     confidence,
			RelevanceDecay: decay,
		}

		result, err := h.knowledge.Create(r.Context(), input)
		if err != nil {
			continue // skip failed entries in batch
		}
		created = append(created, result)
	}

	_ = h.audit.Log(r.Context(), store.ActionKnowledgeWrite, agentID, nil, nil, true, map[string]any{
		"batch_size": len(req.Entries),
		"created":    len(created),
	})

	writeSuccess(w, http.StatusCreated, map[string]any{
		"created": created,
		"count":   len(created),
	})
}
