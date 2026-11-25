package handler

import (
	"context"
	"net/http"

	"emailauto-server/internal/sender"
)

// SenderController defines operational methods on the Sender.
type SenderController interface {
	Pause(reason string, until *sender.StatusReport)
	Resume()
	StopAll(ctx context.Context) error
	GetStatus(ctx context.Context) (*sender.StatusReport, error)
	SendNext(ctx context.Context, force bool) (*sender.SendResult, error)
}

// SenderHandler manages HTTP requests for pause, resume, stop-all, and sender monitoring.
type SenderHandler struct {
	sender *sender.Sender
}

// NewSenderHandler creates a SenderHandler instance.
func NewSenderHandler(s *sender.Sender) *SenderHandler {
	return &SenderHandler{sender: s}
}

// GetStatus handles GET /api/sender/status.
func (h *SenderHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if h.sender == nil {
		writeJSON(w, http.StatusOK, map[string]any{"state": "idle", "daily_sent": 0, "daily_cap": 30})
		return
	}

	report, err := h.sender.GetStatus(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, report)
}

// Pause handles POST /api/sender/pause.
func (h *SenderHandler) Pause(w http.ResponseWriter, r *http.Request) {
	if h.sender == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sender is not configured"})
		return
	}

	h.sender.Pause("paused by user", nil)
	report, _ := h.sender.GetStatus(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "sender paused",
		"status":  report,
	})
}

// Resume handles POST /api/sender/resume.
func (h *SenderHandler) Resume(w http.ResponseWriter, r *http.Request) {
	if h.sender == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sender is not configured"})
		return
	}

	h.sender.Resume()
	report, _ := h.sender.GetStatus(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "sender resumed",
		"status":  report,
	})
}

// StopAll handles POST /api/sender/stop-all.
func (h *SenderHandler) StopAll(w http.ResponseWriter, r *http.Request) {
	if h.sender == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sender is not configured"})
		return
	}

	if err := h.sender.StopAll(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	report, _ := h.sender.GetStatus(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "sender stopped",
		"status":  report,
	})
}

// SendNext handles POST /api/sender/send-next (dispatches next eligible queued email).
func (h *SenderHandler) SendNext(w http.ResponseWriter, r *http.Request) {
	if h.sender == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sender is not configured"})
		return
	}

	force := r.URL.Query().Get("force") == "true"
	result, err := h.sender.SendNext(r.Context(), force)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}
