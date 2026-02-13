package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/MikeSquared-Agency/Alexandria/internal/encryption"
	"github.com/MikeSquared-Agency/Alexandria/internal/hermes"
	"github.com/MikeSquared-Agency/Alexandria/internal/middleware"
	"github.com/MikeSquared-Agency/Alexandria/internal/store"
)

// SecretHandler provides secret management endpoints.
type SecretHandler struct {
	secrets   *store.SecretStore
	grants    *store.GrantStore
	audit     *store.AuditStore
	encryptor *encryption.Encryptor
	publisher *hermes.Publisher
}

// NewSecretHandler creates a new SecretHandler.
func NewSecretHandler(secrets *store.SecretStore, grants *store.GrantStore, audit *store.AuditStore, encryptor *encryption.Encryptor, publisher *hermes.Publisher) *SecretHandler {
	return &SecretHandler{
		secrets:   secrets,
		grants:    grants,
		audit:     audit,
		encryptor: encryptor,
		publisher: publisher,
	}
}

// SecretCreateRequest is the request body for creating a secret.
type SecretCreateRequest struct {
	Name                 string   `json:"name"`
	Value                string   `json:"value"`
	Description          *string  `json:"description,omitempty"`
	Scope                []string `json:"scope,omitempty"` // Kept for backward compatibility
	RotationIntervalDays *int     `json:"rotation_interval_days,omitempty"`
	OwnerType            *string  `json:"owner_type,omitempty"` // New: 'agent', 'person', 'device'
	OwnerID              *string  `json:"owner_id,omitempty"`   // New: owner UUID or agent name
}

// getSubjectFromHeaders extracts subject info from request headers.
func (h *SecretHandler) getSubjectFromHeaders(r *http.Request) (subjectType, subjectID string) {
	// Try X-Agent-ID first (backward compatibility)
	if agentID := r.Header.Get("X-Agent-ID"); agentID != "" {
		return "agent", agentID
	}
	
	// Try X-Device-ID
	if deviceID := r.Header.Get("X-Device-ID"); deviceID != "" {
		return "device", deviceID
	}

	// Fall back to middleware agent ID
	agentID := middleware.AgentIDFromContext(r.Context())
	return "agent", agentID
}

// checkSecretAccess checks if subject can access a secret.
func (h *SecretHandler) checkSecretAccess(r *http.Request, secret *store.Secret, permission string) (bool, string) {
	subjectType, subjectID := h.getSubjectFromHeaders(r)
	
	// Check legacy scope-based access first (backward compatibility)
	if secret != nil && h.secrets.CanAccess(secret, subjectID) {
		return true, subjectID
	}

	// Check new access grant system
	hasAccess, err := h.grants.CheckAccessWithPermission(r.Context(), subjectType, subjectID, "secret", secret.Name, permission)
	if err != nil {
		return false, subjectID
	}
	
	return hasAccess, subjectID
}

// Create handles POST /secrets.
func (h *SecretHandler) Create(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())

	var req SecretCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	if req.Name == "" || req.Value == "" {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Name and value are required")
		return
	}
	if len(req.Value) > 10240 {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Secret value exceeds 10KB limit")
		return
	}

	encrypted, err := h.encryptor.Encrypt(req.Value)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ENCRYPTION_FAILED", "Failed to encrypt secret")
		return
	}

	// Set defaults for ownership
	ownerType := "agent"
	ownerID := agentID
	if req.OwnerType != nil {
		ownerType = *req.OwnerType
	}
	if req.OwnerID != nil {
		ownerID = *req.OwnerID
	}

	secret, err := h.secrets.Create(r.Context(), store.SecretCreateInput{
		Name:                 req.Name,
		EncryptedValue:       encrypted,
		Description:          req.Description,
		Scope:                req.Scope, // Kept for backward compatibility
		RotationIntervalDays: req.RotationIntervalDays,
		CreatedBy:            agentID,
		OwnerType:            &ownerType,
		OwnerID:              &ownerID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create secret")
		return
	}

	_ = h.audit.Log(r.Context(), store.ActionSecretWrite, agentID, &secret.Name, nil, true, nil)
	writeSuccess(w, http.StatusCreated, secret)
}

// List handles GET /secrets — returns names only, not values.
func (h *SecretHandler) List(w http.ResponseWriter, r *http.Request) {
	subjectType, subjectID := h.getSubjectFromHeaders(r)

	secrets, err := h.secrets.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list secrets")
		return
	}

	// Filter to secrets the subject can access
	var visible []store.Secret
	for _, s := range secrets {
		// Check legacy scope-based access first
		if h.secrets.CanAccess(&s, subjectID) {
			visible = append(visible, s)
			continue
		}

		// Check new access grant system
		hasAccess, err := h.grants.CheckAccess(r.Context(), subjectType, subjectID, "secret", s.Name)
		if err == nil && hasAccess {
			visible = append(visible, s)
		}
	}

	writeSuccess(w, http.StatusOK, visible)
}

// Get handles GET /secrets/{name} — returns decrypted value (scoped, audited).
func (h *SecretHandler) Get(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())
	name := chi.URLParam(r, "name")

	secret, err := h.secrets.GetByName(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get secret")
		return
	}
	if secret == nil {
		_ = h.audit.Log(r.Context(), store.ActionSecretRead, agentID, &name, nil, false, nil)
		writeError(w, http.StatusNotFound, "SECRET_NOT_FOUND", "No secret with name '"+name+"'")
		return
	}

	hasAccess, subjectID := h.checkSecretAccess(r, secret, "read")
	if !hasAccess {
		_ = h.audit.Log(r.Context(), store.ActionSecretRead, agentID, &name, nil, false, nil)
		if h.publisher != nil {
			_ = h.publisher.SecretAccessed(r.Context(), subjectID, name, false)
		}
		writeError(w, http.StatusForbidden, "ACCESS_DENIED", "Subject not authorized to access this secret")
		return
	}

	// Decrypt
	value, err := h.encryptor.Decrypt(secret.EncryptedValue)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ENCRYPTION_FAILED", "Failed to decrypt secret")
		return
	}

	_ = h.audit.Log(r.Context(), store.ActionSecretRead, agentID, &name, nil, true, nil)
	if h.publisher != nil {
		_ = h.publisher.SecretAccessed(r.Context(), subjectID, name, true)
	}

	writeSuccess(w, http.StatusOK, map[string]any{
		"name":  secret.Name,
		"value": value,
	})
}

// SecretUpdateRequest is the request body for updating a secret.
type SecretUpdateRequest struct {
	Value string `json:"value"`
}

// Update handles PUT /secrets/{name}.
func (h *SecretHandler) Update(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())
	name := chi.URLParam(r, "name")

	// Check if secret exists and if we have write access
	secret, err := h.secrets.GetByName(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get secret")
		return
	}
	if secret == nil {
		writeError(w, http.StatusNotFound, "SECRET_NOT_FOUND", "No secret with name '"+name+"'")
		return
	}

	hasAccess, _ := h.checkSecretAccess(r, secret, "write")
	if !hasAccess {
		writeError(w, http.StatusForbidden, "ACCESS_DENIED", "Subject not authorized to modify this secret")
		return
	}

	var req SecretUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	encrypted, err := h.encryptor.Encrypt(req.Value)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ENCRYPTION_FAILED", "Failed to encrypt secret")
		return
	}

	if err := h.secrets.Update(r.Context(), name, encrypted); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update secret")
		return
	}

	_ = h.audit.Log(r.Context(), store.ActionSecretWrite, agentID, &name, nil, true, nil)
	writeSuccess(w, http.StatusOK, map[string]string{"updated": name})
}

// Delete handles DELETE /secrets/{name}.
func (h *SecretHandler) Delete(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())
	name := chi.URLParam(r, "name")

	// Check if secret exists and if we have admin access
	secret, err := h.secrets.GetByName(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get secret")
		return
	}
	if secret == nil {
		writeError(w, http.StatusNotFound, "SECRET_NOT_FOUND", "No secret with name '"+name+"'")
		return
	}

	hasAccess, _ := h.checkSecretAccess(r, secret, "admin")
	if !hasAccess {
		writeError(w, http.StatusForbidden, "ACCESS_DENIED", "Subject not authorized to delete this secret")
		return
	}

	if err := h.secrets.Delete(r.Context(), name); err != nil {
		if err.Error() == "not found" {
			writeError(w, http.StatusNotFound, "SECRET_NOT_FOUND", "No secret with name '"+name+"'")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete secret")
		return
	}

	// Clean up associated access grants
	_ = h.grants.DeleteByResource(r.Context(), "secret", name)

	_ = h.audit.Log(r.Context(), store.ActionSecretDelete, agentID, &name, nil, true, nil)
	writeSuccess(w, http.StatusOK, map[string]string{"deleted": name})
}

// RotateRequest is the request body for rotating a secret.
type RotateRequest struct {
	NewValue string `json:"new_value"`
}

// Rotate handles POST /secrets/{name}/rotate.
func (h *SecretHandler) Rotate(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())
	name := chi.URLParam(r, "name")

	// Check if secret exists and if we have write access
	secret, err := h.secrets.GetByName(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get secret")
		return
	}
	if secret == nil {
		writeError(w, http.StatusNotFound, "SECRET_NOT_FOUND", "No secret with name '"+name+"'")
		return
	}

	hasAccess, _ := h.checkSecretAccess(r, secret, "write")
	if !hasAccess {
		writeError(w, http.StatusForbidden, "ACCESS_DENIED", "Subject not authorized to rotate this secret")
		return
	}

	var req RotateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	encrypted, err := h.encryptor.Encrypt(req.NewValue)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ENCRYPTION_FAILED", "Failed to encrypt new value")
		return
	}

	if err := h.secrets.Rotate(r.Context(), name, encrypted, agentID); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to rotate secret")
		return
	}

	_ = h.audit.Log(r.Context(), store.ActionSecretRotate, agentID, &name, nil, true, nil)
	if h.publisher != nil {
		_ = h.publisher.SecretRotated(r.Context(), name, agentID)
	}

	writeSuccess(w, http.StatusOK, map[string]string{"rotated": name})
}
