package handler

import (
	"encoding/json"
	"net/http"

	"emailauto-server/internal/model"
)

// SettingsHandler handles /api/settings endpoints.
type SettingsHandler struct {
	store SettingsStore
}

// NewSettingsHandler creates a new SettingsHandler.
func NewSettingsHandler(store SettingsStore) *SettingsHandler {
	return &SettingsHandler{store: store}
}

// GetSettings handles GET /api/settings.
func (h *SettingsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		defaults := model.DefaultSettings()
		writeJSON(w, http.StatusOK, defaults)
		return
	}

	settings, err := h.store.Get(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, settings)
}

// UpdateSettings handles PUT /api/settings.
func (h *SettingsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "settings store is not available"})
		return
	}

	var input model.Settings
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json payload: " + err.Error()})
		return
	}

	// Validate settings
	if errs := input.Validate(); len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":  "validation failed",
			"fields": errs,
		})
		return
	}

	saved, err := h.store.Save(r.Context(), &input)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, saved)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
