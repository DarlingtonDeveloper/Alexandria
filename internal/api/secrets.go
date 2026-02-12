package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/warrentherabbit/alexandria/internal/encryption"
	"github.com/warrentherabbit/alexandria/internal/hermes"
	"github.com/warrentherabbit/alexandria/internal/middleware"
	"github.com/warrentherabbit/alexandria/internal/store"
)

// SecretHandler provides secret management endpoints.
type SecretHandler struct {
	secrets   *store.SecretStore
	audit     *store.AuditStore
	encryptor *encryption.Encryptor
	publisher *hermes.Publisher
}

// NewSecretHandler creates a new SecretHandler.
func NewSecretHandler(secrets *store.SecretStore, audit *store.AuditStore, encryptor *encryption.Encryptor, publisher *hermes.Publisher) *SecretHandler {
	return &SecretHandler{
		secrets:   secrets,
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
	Scope                []string `json:"scope,omitempty"`
	RotationIntervalDays *int     `json:"rotation_interval_days,omitempty"`
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

	secret, err := h.secrets.Create(r.Context(), store.SecretCreateInput{
		Name:                 req.Name,
		EncryptedValue:       encrypted,
		Description:          req.Description,
		Scope:                req.Scope,
		RotationIntervalDays: req.RotationIntervalDays,
		CreatedBy:            agentID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create secret")
		return
	}

	h.audit.Log(r.Context(), store.ActionSecretWrite, agentID, &secret.Name, nil, true, nil)
	writeSuccess(w, http.StatusCreated, secret)
}

// List handles GET /secrets — returns names only, not values.
func (h *SecretHandler) List(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())

	secrets, err := h.secrets.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list secrets")
		return
	}

	// Filter to secrets the agent can access
	var visible []store.Secret
	for _, s := range secrets {
		if h.secrets.CanAccess(&s, agentID) {
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
		h.audit.Log(r.Context(), store.ActionSecretRead, agentID, &name, nil, false, nil)
		writeError(w, http.StatusNotFound, "SECRET_NOT_FOUND", "No secret with name '"+name+"'")
		return
	}

	if !h.secrets.CanAccess(secret, agentID) {
		h.audit.Log(r.Context(), store.ActionSecretRead, agentID, &name, nil, false, nil)
		if h.publisher != nil {
			_ = h.publisher.SecretAccessed(r.Context(), agentID, name, false)
		}
		writeError(w, http.StatusForbidden, "ACCESS_DENIED", "Agent not authorized to access this secret")
		return
	}

	// Decrypt
	value, err := h.encryptor.Decrypt(secret.EncryptedValue)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ENCRYPTION_FAILED", "Failed to decrypt secret")
		return
	}

	h.audit.Log(r.Context(), store.ActionSecretRead, agentID, &name, nil, true, nil)
	if h.publisher != nil {
		_ = h.publisher.SecretAccessed(r.Context(), agentID, name, true)
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

	h.audit.Log(r.Context(), store.ActionSecretWrite, agentID, &name, nil, true, nil)
	writeSuccess(w, http.StatusOK, map[string]string{"updated": name})
}

// Delete handles DELETE /secrets/{name}.
func (h *SecretHandler) Delete(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())
	name := chi.URLParam(r, "name")

	if err := h.secrets.Delete(r.Context(), name); err != nil {
		if err.Error() == "not found" {
			writeError(w, http.StatusNotFound, "SECRET_NOT_FOUND", "No secret with name '"+name+"'")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete secret")
		return
	}

	h.audit.Log(r.Context(), store.ActionSecretDelete, agentID, &name, nil, true, nil)
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

	h.audit.Log(r.Context(), store.ActionSecretRotate, agentID, &name, nil, true, nil)
	if h.publisher != nil {
		_ = h.publisher.SecretRotated(r.Context(), name, agentID)
	}

	writeSuccess(w, http.StatusOK, map[string]string{"rotated": name})
}
