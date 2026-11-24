package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"emailauto-server/internal/model"
	"emailauto-server/internal/orchestrator"
	"emailauto-server/internal/poller"
)

// OrchestratorController defines actions for orchestrating runs and status.
type OrchestratorController interface {
	GetStatus(ctx context.Context) (*orchestrator.OrchestratorStatus, error)
	ToggleAuto(ctx context.Context, enabled bool) bool
	KillSwitch(ctx context.Context)
	RunPipeline(ctx context.Context, trigger, timeFilter, sortBy string, maxPosts int) (*model.Run, error)
	ArmManualRun(ctx context.Context, now time.Time)
}

// ReplyStore defines retrieval of recorded email replies.
type ReplyStore interface {
	ListReplies(ctx context.Context, limit, offset int) ([]model.Reply, error)
}

// PollerController defines on-demand execution of thread checking and follow-ups.
type PollerController interface {
	PollOnce(ctx context.Context) (*poller.PollReport, error)
}

// OrchestratorHandler handles API requests for auto scheduling, kill switch, run logs, and replies.
type OrchestratorHandler struct {
	orc        OrchestratorController
	replyStore ReplyStore
	poller     PollerController
}

// NewOrchestratorHandler creates an OrchestratorHandler.
func NewOrchestratorHandler(
	orc OrchestratorController,
	replyStore ReplyStore,
	poller PollerController,
) *OrchestratorHandler {
	return &OrchestratorHandler{
		orc:        orc,
		replyStore: replyStore,
		poller:     poller,
	}
}

// GetStatus handles GET /api/orchestrator/status.
func (h *OrchestratorHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if h.orc == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"state":               "IDLE",
			"auto_enabled":        false,
			"armed":               false,
			"next_run_in_seconds": 0,
		})
		return
	}

	status, err := h.orc.GetStatus(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, status)
}

// ToggleAuto handles POST /api/orchestrator/toggle-auto.
func (h *OrchestratorHandler) ToggleAuto(w http.ResponseWriter, r *http.Request) {
	if h.orc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "orchestrator not configured"})
		return
	}

	var payload struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}

	enabled := h.orc.ToggleAuto(r.Context(), payload.Enabled)
	status, _ := h.orc.GetStatus(r.Context())

	writeJSON(w, http.StatusOK, map[string]any{
		"auto_enabled": enabled,
		"status":       status,
	})
}

// KillSwitch handles POST /api/orchestrator/kill-switch.
func (h *OrchestratorHandler) KillSwitch(w http.ResponseWriter, r *http.Request) {
	if h.orc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "orchestrator not configured"})
		return
	}

	h.orc.KillSwitch(r.Context())
	status, _ := h.orc.GetStatus(r.Context())

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "KILL SWITCH ACTIVATED: auto stopped and engine disarmed",
		"status":  status,
	})
}

// RunNow handles POST /api/orchestrator/run-now (manual trigger that also arms the engine for today).
func (h *OrchestratorHandler) RunNow(w http.ResponseWriter, r *http.Request) {
	if h.orc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "orchestrator not configured"})
		return
	}

	var payload struct {
		TimeFilter string `json:"time_filter"`
		SortBy     string `json:"sort_by"`
		MaxPosts   int    `json:"max_posts"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)

	run, err := h.orc.RunPipeline(r.Context(), "manual", payload.TimeFilter, payload.SortBy, payload.MaxPosts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	status, _ := h.orc.GetStatus(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "pipeline run initiated and engine armed for today",
		"run":     run,
		"status":  status,
	})
}

// ListReplies handles GET /api/replies.
func (h *OrchestratorHandler) ListReplies(w http.ResponseWriter, r *http.Request) {
	if h.replyStore == nil {
		writeJSON(w, http.StatusOK, []model.Reply{})
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	replies, err := h.replyStore.ListReplies(r.Context(), limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if replies == nil {
		replies = []model.Reply{}
	}

	writeJSON(w, http.StatusOK, replies)
}

// PollNow handles POST /api/poller/poll (on-demand thread check & follow-up draft generation).
func (h *OrchestratorHandler) PollNow(w http.ResponseWriter, r *http.Request) {
	if h.poller == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "poller not configured"})
		return
	}

	report, err := h.poller.PollOnce(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, report)
}
