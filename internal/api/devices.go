package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/MikeSquared-Agency/Alexandria/internal/middleware"
	"github.com/MikeSquared-Agency/Alexandria/internal/store"
)

// DevicesHandler provides devices management endpoints.
type DevicesHandler struct {
	devices *store.DeviceStore
	audit   *store.AuditStore
}

// NewDevicesHandler creates a new DevicesHandler.
func NewDevicesHandler(devices *store.DeviceStore, audit *store.AuditStore) *DevicesHandler {
	return &DevicesHandler{
		devices: devices,
		audit:   audit,
	}
}

// Create handles POST /devices.
func (h *DevicesHandler) Create(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())

	var req store.DeviceCreateInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	if req.Name == "" || req.DeviceType == "" || req.Identifier == "" {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Name, device_type, and identifier are required")
		return
	}

	device, err := h.devices.Create(r.Context(), req)
	if err != nil {
		if err.Error() == "duplicate key value violates unique constraint" || 
		   err.Error() == "UNIQUE constraint failed" {
			writeError(w, http.StatusConflict, "IDENTIFIER_ALREADY_EXISTS", "Device with this identifier already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create device")
		return
	}

	// Log the action
	h.audit.Log(r.Context(), "device.create", agentID, &device.ID, nil, true, nil)

	writeSuccess(w, http.StatusCreated, device)
}

// List handles GET /devices.
func (h *DevicesHandler) List(w http.ResponseWriter, r *http.Request) {
	ownerID := r.URL.Query().Get("owner_id")
	
	var devices []store.Device
	var err error

	if ownerID != "" {
		devices, err = h.devices.ListByOwner(r.Context(), ownerID)
	} else {
		devices, err = h.devices.List(r.Context())
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list devices")
		return
	}

	writeSuccess(w, http.StatusOK, devices)
}

// Get handles GET /devices/{id}.
func (h *DevicesHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	device, err := h.devices.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get device")
		return
	}
	if device == nil {
		writeError(w, http.StatusNotFound, "DEVICE_NOT_FOUND", "No device with ID '"+id+"'")
		return
	}

	writeSuccess(w, http.StatusOK, device)
}

// Update handles PUT /devices/{id}.
func (h *DevicesHandler) Update(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())
	id := chi.URLParam(r, "id")

	var req store.DeviceUpdateInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	device, err := h.devices.Update(r.Context(), id, req)
	if err != nil {
		if err.Error() == "device not found" {
			writeError(w, http.StatusNotFound, "DEVICE_NOT_FOUND", "No device with ID '"+id+"'")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update device")
		return
	}

	// Log the action
	h.audit.Log(r.Context(), "device.update", agentID, &id, nil, true, nil)

	writeSuccess(w, http.StatusOK, device)
}

// Delete handles DELETE /devices/{id}.
func (h *DevicesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	agentID := middleware.AgentIDFromContext(r.Context())
	id := chi.URLParam(r, "id")

	err := h.devices.Delete(r.Context(), id)
	if err != nil {
		if err.Error() == "device not found" {
			writeError(w, http.StatusNotFound, "DEVICE_NOT_FOUND", "No device with ID '"+id+"'")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete device")
		return
	}

	// Log the action
	h.audit.Log(r.Context(), "device.delete", agentID, &id, nil, true, nil)

	writeSuccess(w, http.StatusOK, map[string]string{"deleted": id})
}
