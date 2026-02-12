package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/warrentherabbit/alexandria/internal/middleware"
	"github.com/warrentherabbit/alexandria/internal/store"
)

// GrantsHandler provides grants management endpoints.
type GrantsHandler struct {
	grants *store.GrantStore
	audit  *store.AuditStore
}

// NewGrantsHandler creates a new GrantsHandler.
func NewGrantsHandler(grants *store.GrantStore, audit *store.AuditStore) *GrantsHandler {
	return &GrantsHandler{
		grants: grants,
		audit:  audit,
	}
}

// Create handles POST /grants.
func (h *GrantsHandler) Create(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())

	var req store.AccessGrantCreateInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	if req.ResourceType == "" || req.ResourceID == "" || req.SubjectType == "" || req.SubjectID == "" {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "resource_type, resource_id, subject_type, and subject_id are required")
		return
	}

	// Validate resource_type
	if req.ResourceType != "secret" && req.ResourceType != "knowledge" {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "resource_type must be 'secret' or 'knowledge'")
		return
	}

	// Validate subject_type
	if req.SubjectType != "person" && req.SubjectType != "device" && req.SubjectType != "agent" {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "subject_type must be 'person', 'device', or 'agent'")
		return
	}

	// Validate permission
	if req.Permission != "read" && req.Permission != "write" && req.Permission != "admin" {
		req.Permission = "read" // Default
	}

	// Set granted_by to current agent
	req.GrantedBy = &agentID

	grant, err := h.grants.Create(r.Context(), req)
	if err != nil {
		if err.Error() == "duplicate key value violates unique constraint" || 
		   err.Error() == "UNIQUE constraint failed" {
			writeError(w, http.StatusConflict, "GRANT_ALREADY_EXISTS", "Grant already exists for this subject and resource")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create grant")
		return
	}

	// Log the action
	h.audit.Log(r.Context(), "grant.create", agentID, &grant.ID, nil, true, nil)

	writeSuccess(w, http.StatusCreated, grant)
}

// List handles GET /grants.
func (h *GrantsHandler) List(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	
	var resourceType, resourceID, subjectType, subjectID *string
	
	if rt := query.Get("resource_type"); rt != "" {
		resourceType = &rt
	}
	if ri := query.Get("resource_id"); ri != "" {
		resourceID = &ri
	}
	if st := query.Get("subject_type"); st != "" {
		subjectType = &st
	}
	if si := query.Get("subject_id"); si != "" {
		subjectID = &si
	}

	grants, err := h.grants.List(r.Context(), resourceType, resourceID, subjectType, subjectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list grants")
		return
	}

	writeSuccess(w, http.StatusOK, grants)
}

// Get handles GET /grants/{id}.
func (h *GrantsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	grant, err := h.grants.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get grant")
		return
	}
	if grant == nil {
		writeError(w, http.StatusNotFound, "GRANT_NOT_FOUND", "No grant with ID '"+id+"'")
		return
	}

	writeSuccess(w, http.StatusOK, grant)
}

// CheckAccess handles GET /grants/check.
func (h *GrantsHandler) CheckAccess(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	
	subjectType := query.Get("subject_type")
	subjectID := query.Get("subject_id")
	resourceType := query.Get("resource_type")
	resourceID := query.Get("resource_id")
	permission := query.Get("permission")

	if subjectType == "" || subjectID == "" || resourceType == "" || resourceID == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "subject_type, subject_id, resource_type, and resource_id are required")
		return
	}

	var hasAccess bool
	var err error

	if permission != "" {
		hasAccess, err = h.grants.CheckAccessWithPermission(r.Context(), subjectType, subjectID, resourceType, resourceID, permission)
	} else {
		hasAccess, err = h.grants.CheckAccess(r.Context(), subjectType, subjectID, resourceType, resourceID)
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to check access")
		return
	}

	writeSuccess(w, http.StatusOK, map[string]bool{
		"has_access": hasAccess,
	})
}

// Delete handles DELETE /grants/{id}.
func (h *GrantsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())
	id := chi.URLParam(r, "id")

	err := h.grants.Delete(r.Context(), id)
	if err != nil {
		if err.Error() == "access grant not found" {
			writeError(w, http.StatusNotFound, "GRANT_NOT_FOUND", "No grant with ID '"+id+"'")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete grant")
		return
	}

	// Log the action
	h.audit.Log(r.Context(), "grant.delete", agentID, &id, nil, true, nil)

	writeSuccess(w, http.StatusOK, map[string]string{"deleted": id})
}
