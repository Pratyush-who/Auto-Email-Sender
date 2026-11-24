package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"emailauto-server/internal/composer"
	"emailauto-server/internal/model"

	"github.com/go-chi/chi/v5"
)

// OutreachStore defines database operations for outreach items.
type OutreachStore interface {
	CreateOutreach(ctx context.Context, o *model.Outreach) (*model.Outreach, error)
	GetOutreachByID(ctx context.Context, id string) (*model.Outreach, error)
	UpdateOutreach(ctx context.Context, o *model.Outreach) error
	ListOutreach(ctx context.Context, filter model.OutreachFilter) ([]model.Outreach, error)
	GetSentBodiesToday(ctx context.Context) ([]string, error)
	UpdateStatus(ctx context.Context, id string, status model.OutreachStatus, gmailMsgID, gmailThreadID, errText string, sentAt *time.Time) error
}

// ComposerService defines email drafting capabilities.
type ComposerService interface {
	Compose(ctx context.Context, profile model.Profile, post model.Post, companySummary string, existingBodiesToday []string) (*composer.ComposedEmail, error)
}

// OutreachHandler handles API endpoints for viewing and editing outreach emails.
type OutreachHandler struct {
	outreachStore OutreachStore
	profileStore  ProfileStore
	postStore     PostStore
	composer      ComposerService
}

// NewOutreachHandler creates an OutreachHandler instance.
func NewOutreachHandler(
	outreachStore OutreachStore,
	profileStore ProfileStore,
	postStore PostStore,
	composer ComposerService,
) *OutreachHandler {
	return &OutreachHandler{
		outreachStore: outreachStore,
		profileStore:  profileStore,
		postStore:     postStore,
		composer:      composer,
	}
}

// ListOutreach handles GET /api/outreach with optional ?status=draft&kind=initial.
func (h *OutreachHandler) ListOutreach(w http.ResponseWriter, r *http.Request) {
	if h.outreachStore == nil {
		writeJSON(w, http.StatusOK, []model.Outreach{})
		return
	}

	filter := model.OutreachFilter{
		Status: model.OutreachStatus(r.URL.Query().Get("status")),
		Kind:   model.OutreachKind(r.URL.Query().Get("kind")),
		Limit:  50,
		Offset: 0,
	}

	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			filter.Limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			filter.Offset = parsed
		}
	}

	items, err := h.outreachStore.ListOutreach(r.Context(), filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if items == nil {
		items = []model.Outreach{}
	}

	writeJSON(w, http.StatusOK, items)
}

// GetOutreach handles GET /api/outreach/{id}.
func (h *OutreachHandler) GetOutreach(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id parameter is required"})
		return
	}

	item, err := h.outreachStore.GetOutreachByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if item == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "outreach item not found"})
		return
	}

	writeJSON(w, http.StatusOK, item)
}

// UpdateOutreach handles PUT /api/outreach/{id} (edits subject, body, or status in UI).
func (h *OutreachHandler) UpdateOutreach(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id parameter is required"})
		return
	}

	var payload struct {
		Subject string               `json:"subject"`
		Body    string               `json:"body"`
		Status  model.OutreachStatus `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}

	existing, err := h.outreachStore.GetOutreachByID(r.Context(), id)
	if err != nil || existing == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "outreach item not found"})
		return
	}

	if strings.TrimSpace(payload.Subject) != "" {
		existing.Subject = strings.TrimSpace(payload.Subject)
	}
	if strings.TrimSpace(payload.Body) != "" {
		existing.Body = strings.TrimSpace(payload.Body)
	}
	if payload.Status != "" {
		existing.Status = payload.Status
	}

	if err := h.outreachStore.UpdateOutreach(r.Context(), existing); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, existing)
}

// ApproveOutreach handles POST /api/outreach/{id}/approve (moves draft to queued/approved).
func (h *OutreachHandler) ApproveOutreach(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id parameter is required"})
		return
	}

	if err := h.outreachStore.UpdateStatus(r.Context(), id, model.OutreachStatusQueued, "", "", "", nil); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true, "status": "queued"})
}

// SkipOutreach handles POST /api/outreach/{id}/skip.
func (h *OutreachHandler) SkipOutreach(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id parameter is required"})
		return
	}

	if err := h.outreachStore.UpdateStatus(r.Context(), id, model.OutreachStatusSkipped, "", "", "skipped by user", nil); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true, "status": "skipped"})
}

// GenerateOutreach handles POST /api/composer/generate (on-demand drafting for a post).
func (h *OutreachHandler) GenerateOutreach(w http.ResponseWriter, r *http.Request) {
	if h.composer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "composer is not configured"})
		return
	}

	var payload struct {
		PostID    string `json:"post_id"`
		ContactID string `json:"contact_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}

	profile, _ := h.profileStore.Get(r.Context())
	if profile == nil {
		profile = &model.Profile{}
	}

	var post model.Post
	if h.postStore != nil && payload.PostID != "" {
		posts, err := h.postStore.GetPosts(r.Context(), 100, 0)
		if err == nil {
			for _, p := range posts {
				if p.ID == payload.PostID {
					post = p
					break
				}
			}
		}
	}

	bodiesToday, _ := h.outreachStore.GetSentBodiesToday(r.Context())
	res, err := h.composer.Compose(r.Context(), *profile, post, post.Extracted.CompanySummary, bodiesToday)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	outreach := model.Outreach{
		ContactID: payload.ContactID,
		PostID:    payload.PostID,
		Kind:      model.OutreachKindInitial,
		Subject:   res.Subject,
		Body:      res.Body,
		Status:    model.OutreachStatusDraft,
	}

	if payload.ContactID != "" && h.outreachStore != nil {
		created, err := h.outreachStore.CreateOutreach(r.Context(), &outreach)
		if err == nil && created != nil {
			outreach = *created
		}
	}

	writeJSON(w, http.StatusOK, outreach)
}
