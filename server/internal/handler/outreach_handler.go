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
	ComposeBatch(ctx context.Context, profile model.Profile, items []composer.BatchComposeItem, existingBodiesToday []string) (map[string]composer.ComposedEmail, error)
}

// OutreachHandler handles API endpoints for viewing and editing outreach emails.
type OutreachHandler struct {
	outreachStore OutreachStore
	profileStore  ProfileStore
	postStore     PostStore
	contactStore  ContactStore
	composer      ComposerService
}

// NewOutreachHandler creates an OutreachHandler instance.
func NewOutreachHandler(
	outreachStore OutreachStore,
	profileStore ProfileStore,
	postStore PostStore,
	contactStore ContactStore,
	composer ComposerService,
) *OutreachHandler {
	return &OutreachHandler{
		outreachStore: outreachStore,
		profileStore:  profileStore,
		postStore:     postStore,
		contactStore:  contactStore,
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
		Force     bool   `json:"force"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}

	// Idempotency: If an outreach draft already exists for this PostID or ContactID (same email), return it directly unless force=true (resend)
	if !payload.Force && h.outreachStore != nil && (payload.PostID != "" || payload.ContactID != "") {
		allOutreach, err := h.outreachStore.ListOutreach(r.Context(), model.OutreachFilter{Limit: 500})
		if err == nil {
			for _, o := range allOutreach {
				if (payload.PostID != "" && o.PostID == payload.PostID) ||
					(payload.ContactID != "" && o.ContactID == payload.ContactID && o.Kind == model.OutreachKindInitial) {
					writeJSON(w, http.StatusOK, o)
					return
				}
			}
		}
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

	// If ContactID is missing, resolve from existing contact or create from post extracted emails
	if payload.ContactID == "" && h.contactStore != nil {
		if post.ID != "" {
			existingContacts, err := h.contactStore.GetContacts(r.Context(), 100)
			if err == nil {
				for _, c := range existingContacts {
					if c.PostID == post.ID || (len(post.Extracted.Emails) > 0 && c.Email == post.Extracted.Emails[0]) {
						payload.ContactID = c.ID
						break
					}
				}
			}
		}

		if payload.ContactID == "" && len(post.Extracted.Emails) > 0 {
			primaryEmail := post.Extracted.Emails[0]
			company := post.Company
			if company == "" {
				company = post.Role
			}
			domain := ""
			if parts := strings.Split(primaryEmail, "@"); len(parts) == 2 {
				domain = parts[1]
			}
			newContact, err := h.contactStore.SaveContact(r.Context(), &model.Contact{
				Email:        primaryEmail,
				Company:      company,
				Domain:       domain,
				TrustScore:   100,
				TrustReasons: []string{"manually initiated outreach"},
				PostID:       post.ID,
			})
			if err == nil && newContact != nil {
				payload.ContactID = newContact.ID
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

// QueueClearer is an optional store interface for clearing all queued/approved outreach items.
type QueueClearer interface {
	ClearQueue(ctx context.Context) (int64, error)
}

// ClearQueue handles DELETE /api/outreach/queue to cancel and empty the entire outreach queue.
func (h *OutreachHandler) ClearQueue(w http.ResponseWriter, r *http.Request) {
	if h.outreachStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"cleared": 0})
		return
	}

	if clearer, ok := h.outreachStore.(QueueClearer); ok {
		count, err := clearer.ClearQueue(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"cleared": count, "message": "queue emptied successfully"})
		return
	}

	// Fallback if store only implements base OutreachStore
	items, err := h.outreachStore.ListOutreach(r.Context(), model.OutreachFilter{Limit: 500})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var cleared int64
	for _, it := range items {
		if it.Status == model.OutreachStatusQueued || it.Status == model.OutreachStatusApproved {
			if err := h.outreachStore.UpdateStatus(r.Context(), it.ID, model.OutreachStatusSkipped, "", "", "cancelled from queue", nil); err == nil {
				cleared++
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": cleared, "message": "queue emptied successfully"})
}

// GenerateOutreachBatch handles POST /api/composer/generate-batch to draft multiple selected posts in batched Gemini calls.
func (h *OutreachHandler) GenerateOutreachBatch(w http.ResponseWriter, r *http.Request) {
	if h.composer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "composer is not configured"})
		return
	}

	var payload struct {
		PostIDs []string `json:"post_ids"`
		Force   bool     `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}
	if len(payload.PostIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "post_ids cannot be empty"})
		return
	}

	profile, _ := h.profileStore.Get(r.Context())
	if profile == nil {
		profile = &model.Profile{}
	}

	posts, err := h.postStore.GetPosts(r.Context(), 500, 0)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	postByID := make(map[string]model.Post, len(posts))
	for _, p := range posts {
		postByID[p.ID] = p
	}

	existingOutreach, _ := h.outreachStore.ListOutreach(r.Context(), model.OutreachFilter{Limit: 500})
	existingByPost := make(map[string]bool)
	existingByContact := make(map[string]bool)
	for _, o := range existingOutreach {
		if o.PostID != "" {
			existingByPost[o.PostID] = true
		}
		if o.ContactID != "" && o.Kind == model.OutreachKindInitial {
			existingByContact[o.ContactID] = true
		}
	}

	existingContacts, _ := h.contactStore.GetContacts(r.Context(), 500)
	contactByEmail := make(map[string]model.Contact)
	for _, c := range existingContacts {
		contactByEmail[strings.ToLower(strings.TrimSpace(c.Email))] = c
	}

	type draftTarget struct {
		item      composer.BatchComposeItem
		contactID string
	}
	var targets []draftTarget
	seenEmails := make(map[string]bool)
	skippedDuplicates := 0

	for _, pid := range payload.PostIDs {
		p, ok := postByID[pid]
		if !ok || len(p.Extracted.Emails) == 0 {
			continue
		}
		if !payload.Force && existingByPost[p.ID] {
			skippedDuplicates++
			continue
		}

		primaryEmail := strings.ToLower(strings.TrimSpace(p.Extracted.Emails[0]))
		if primaryEmail == "" || seenEmails[primaryEmail] {
			skippedDuplicates++
			continue
		}

		var contactID string
		if c, found := contactByEmail[primaryEmail]; found {
			contactID = c.ID
		} else if h.contactStore != nil {
			company := p.Company
			if company == "" {
				company = p.Role
			}
			domain := ""
			if parts := strings.Split(primaryEmail, "@"); len(parts) == 2 {
				domain = parts[1]
			}
			if newC, err := h.contactStore.SaveContact(r.Context(), &model.Contact{
				Email:        primaryEmail,
				Company:      company,
				Domain:       domain,
				TrustScore:   100,
				TrustReasons: []string{"batch manually drafted"},
				PostID:       p.ID,
			}); err == nil && newC != nil {
				contactID = newC.ID
				contactByEmail[primaryEmail] = *newC
			}
		}

		if !payload.Force && contactID != "" && existingByContact[contactID] {
			skippedDuplicates++
			continue
		}

		seenEmails[primaryEmail] = true
		targets = append(targets, draftTarget{
			item: composer.BatchComposeItem{
				PostID:         p.ID,
				Post:           p,
				CompanySummary: p.Extracted.CompanySummary,
			},
			contactID: contactID,
		})
	}

	var createdDrafts []model.Outreach
	bodiesToday, _ := h.outreachStore.GetSentBodiesToday(r.Context())
	const batchSize = 15

	for start := 0; start < len(targets); start += batchSize {
		end := start + batchSize
		if end > len(targets) {
			end = len(targets)
		}
		chunk := targets[start:end]
		batchItems := make([]composer.BatchComposeItem, len(chunk))
		for i, t := range chunk {
			batchItems[i] = t.item
		}

		composedMap, compErr := h.composer.ComposeBatch(r.Context(), *profile, batchItems, bodiesToday)
		if compErr != nil {
			continue
		}

		for _, t := range chunk {
			res, ok := composedMap[t.item.PostID]
			if !ok || strings.TrimSpace(res.Body) == "" || t.contactID == "" {
				continue
			}
			created, err := h.outreachStore.CreateOutreach(r.Context(), &model.Outreach{
				ContactID: t.contactID,
				PostID:    t.item.PostID,
				Kind:      model.OutreachKindInitial,
				Subject:   res.Subject,
				Body:      res.Body,
				Status:    model.OutreachStatusDraft,
			})
			if err == nil && created != nil {
				createdDrafts = append(createdDrafts, *created)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"drafted":            len(createdDrafts),
		"skipped_duplicates": skippedDuplicates,
		"drafts":             createdDrafts,
	})
}
