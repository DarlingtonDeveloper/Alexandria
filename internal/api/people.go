package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/MikeSquared-Agency/Alexandria/internal/middleware"
	"github.com/MikeSquared-Agency/Alexandria/internal/store"
)

// PeopleHandler provides people management endpoints.
type PeopleHandler struct {
	people *store.PersonStore
	audit  *store.AuditStore
}

// NewPeopleHandler creates a new PeopleHandler.
func NewPeopleHandler(people *store.PersonStore, audit *store.AuditStore) *PeopleHandler {
	return &PeopleHandler{
		people: people,
		audit:  audit,
	}
}

// Create handles POST /people.
func (h *PeopleHandler) Create(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())

	var req store.PersonCreateInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	if req.Name == "" || req.Identifier == "" {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Name and identifier are required")
		return
	}

	person, err := h.people.Create(r.Context(), req)
	if err != nil {
		if err.Error() == "duplicate key value violates unique constraint" || 
		   err.Error() == "UNIQUE constraint failed" {
			writeError(w, http.StatusConflict, "IDENTIFIER_ALREADY_EXISTS", "Person with this identifier already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create person")
		return
	}

	// Log the action
	_ = h.audit.Log(r.Context(), "person.create", agentID, &person.ID, nil, true, nil)

	writeSuccess(w, http.StatusCreated, person)
}

// List handles GET /people.
func (h *PeopleHandler) List(w http.ResponseWriter, r *http.Request) {
	people, err := h.people.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list people")
		return
	}

	writeSuccess(w, http.StatusOK, people)
}

// Get handles GET /people/{id}.
func (h *PeopleHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	person, err := h.people.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get person")
		return
	}
	if person == nil {
		writeError(w, http.StatusNotFound, "PERSON_NOT_FOUND", "No person with ID '"+id+"'")
		return
	}

	writeSuccess(w, http.StatusOK, person)
}

// Update handles PUT /people/{id}.
func (h *PeopleHandler) Update(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())
	id := chi.URLParam(r, "id")

	var req store.PersonUpdateInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	person, err := h.people.Update(r.Context(), id, req)
	if err != nil {
		if err.Error() == "person not found" {
			writeError(w, http.StatusNotFound, "PERSON_NOT_FOUND", "No person with ID '"+id+"'")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update person")
		return
	}

	// Log the action
	_ = h.audit.Log(r.Context(), "person.update", agentID, &id, nil, true, nil)

	writeSuccess(w, http.StatusOK, person)
}

// Delete handles DELETE /people/{id}.
func (h *PeopleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())
	id := chi.URLParam(r, "id")

	err := h.people.Delete(r.Context(), id)
	if err != nil {
		if err.Error() == "person not found" {
			writeError(w, http.StatusNotFound, "PERSON_NOT_FOUND", "No person with ID '"+id+"'")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete person")
		return
	}

	// Log the action
	_ = h.audit.Log(r.Context(), "person.delete", agentID, &id, nil, true, nil)

	writeSuccess(w, http.StatusOK, map[string]string{"deleted": id})
}
