package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"emailauto-server/internal/filter"
	"emailauto-server/internal/model"
	"emailauto-server/internal/pipeline"
	"emailauto-server/internal/worker"
)

// PostStore defines repository operations required for posts and runs.
type PostStore interface {
	SavePost(ctx context.Context, p *model.Post) (*model.Post, error)
	GetPosts(ctx context.Context, limit, offset int) ([]model.Post, error)
	CreateRun(ctx context.Context, run *model.Run) (*model.Run, error)
	UpdateRun(ctx context.Context, runID, status string, finishedAt time.Time, stats map[string]any) error
	GetRuns(ctx context.Context, limit int) ([]model.Run, error)
}

// ContactStore defines contact listing operations for the handler.
type ContactStore interface {
	GetContacts(ctx context.Context, limit int) ([]model.Contact, error)
}

// PostPipeline defines extraction, filtering, and verification execution.
type PostPipeline interface {
	ProcessPost(ctx context.Context, post *model.Post, settings model.Settings) (*pipeline.ProcessPostResult, error)
	ProcessBatch(ctx context.Context, posts []model.Post, settings model.Settings) ([]pipeline.ProcessPostResult, error)
}

// WorkerScraper defines interface for interacting with the Playwright worker.
type WorkerScraper interface {
	Scrape(ctx context.Context, req worker.ScrapeRequest) (*worker.ScrapeResponse, error)
}

// PostHandler handles /api/posts and /api/runs endpoints.
type PostHandler struct {
	postStore     PostStore
	contactStore  ContactStore
	settingsStore SettingsStore
	workerClient  WorkerScraper
	pipeline      PostPipeline
}

// NewPostHandler creates a PostHandler.
func NewPostHandler(
	postStore PostStore,
	contactStore ContactStore,
	settingsStore SettingsStore,
	workerClient WorkerScraper,
	pipeline PostPipeline,
) *PostHandler {
	return &PostHandler{
		postStore:     postStore,
		contactStore:  contactStore,
		settingsStore: settingsStore,
		workerClient:  workerClient,
		pipeline:      pipeline,
	}
}

// ManualImportPost handles POST /api/posts/manual (paste post text/URL fallback).
func (h *PostHandler) ManualImportPost(w http.ResponseWriter, r *http.Request) {
	if h.postStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "post store not available"})
		return
	}

	var payload struct {
		URL             string   `json:"url"`
		Text            string   `json:"text"`
		Author          string   `json:"author"`
		Company         string   `json:"company"`
		Role            string   `json:"role"`
		WorkMode        string   `json:"work_mode"`
		PaidStatus      string   `json:"paid_status"`
		Location        string   `json:"location"`
		Stipend         *float64 `json:"stipend"`
		ExperienceYears *float64 `json:"experience_years"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json payload: " + err.Error()})
		return
	}

	if strings.TrimSpace(payload.URL) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "field 'url' is required"})
		return
	}
	if strings.TrimSpace(payload.Text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "field 'text' is required"})
		return
	}

	// Create post model
	post := model.Post{
		URL:        strings.TrimSpace(payload.URL),
		Text:       strings.TrimSpace(payload.Text),
		Author:     strings.TrimSpace(payload.Author),
		Company:    strings.TrimSpace(payload.Company),
		Role:       strings.TrimSpace(payload.Role),
		WorkMode:   strings.ToLower(strings.TrimSpace(payload.WorkMode)),
		PaidStatus: strings.ToLower(strings.TrimSpace(payload.PaidStatus)),
		Location:   strings.TrimSpace(payload.Location),
		Extracted: model.PostExtracted{
			Stipend:         payload.Stipend,
			ExperienceYears: payload.ExperienceYears,
		},
		Status:  "new",
		FoundAt: time.Now().UTC(),
	}

	// Apply filter rules against active settings
	var currentSettings model.Settings
	if h.settingsStore != nil {
		s, err := h.settingsStore.Get(r.Context())
		if err == nil && s != nil {
			currentSettings = *s
		} else {
			currentSettings = model.DefaultSettings()
		}
	} else {
		currentSettings = model.DefaultSettings()
	}

	var saved *model.Post
	var contact *model.Contact
	var pass bool
	var reason string

	if h.pipeline != nil {
		res, err := h.pipeline.ProcessPost(r.Context(), &post, currentSettings)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		saved = res.Post
		contact = res.Contact
		pass = res.Passed
		reason = res.Reason
	} else {
		pass, reason = filter.ApplyFilters(post, currentSettings)
		if !pass {
			post.Status = "skipped"
			post.SkipReason = reason
		}

		var err error
		saved, err = h.postStore.SavePost(r.Context(), &post)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	response := map[string]any{
		"message": "post imported successfully",
		"post":    saved,
		"filter": map[string]any{
			"passed": pass,
			"reason": reason,
		},
	}
	if contact != nil {
		response["contact"] = contact
	}
	writeJSON(w, http.StatusOK, response)
}

// ListPosts handles GET /api/posts.
func (h *PostHandler) ListPosts(w http.ResponseWriter, r *http.Request) {
	if h.postStore == nil {
		writeJSON(w, http.StatusOK, []model.Post{})
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

	posts, err := h.postStore.GetPosts(r.Context(), limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if posts == nil {
		posts = []model.Post{}
	}

	writeJSON(w, http.StatusOK, posts)
}

// TriggerRun handles POST /api/runs (dispatches search to worker and saves extracted posts).
func (h *PostHandler) TriggerRun(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Query      string `json:"query"`
		TimeFilter string `json:"time_filter"`
		SortBy     string `json:"sort_by"`
		MaxPosts   int    `json:"max_posts"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}

	if strings.TrimSpace(input.Query) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query cannot be empty"})
		return
	}

	if input.TimeFilter == "" {
		input.TimeFilter = "24h"
	}
	if input.SortBy == "" {
		input.SortBy = "relevance"
	}
	if input.MaxPosts <= 0 {
		input.MaxPosts = 60
	}

	now := time.Now().UTC()
	runRecord := &model.Run{
		Query:      input.Query,
		TimeFilter: input.TimeFilter,
		SortBy:     input.SortBy,
		Trigger:    "manual",
		Status:     "running",
		StartedAt:  &now,
		Stats:      map[string]any{},
	}

	if h.postStore != nil {
		created, err := h.postStore.CreateRun(r.Context(), runRecord)
		if err == nil && created != nil {
			runRecord = created
		}
	}

	if h.workerClient == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "playwright worker client is not configured"})
		return
	}

	// Dispatch to worker
	scrapeRes, err := h.workerClient.Scrape(r.Context(), worker.ScrapeRequest{
		Query:      input.Query,
		TimeFilter: input.TimeFilter,
		SortBy:     input.SortBy,
		MaxPosts:   input.MaxPosts,
	})

	if err != nil {
		finishedAt := time.Now().UTC()
		if h.postStore != nil && runRecord.ID != "" {
			_ = h.postStore.UpdateRun(r.Context(), runRecord.ID, "failed", finishedAt, map[string]any{
				"error": err.Error(),
			})
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "scrape failed: " + err.Error(),
			"run_id":  runRecord.ID,
			"status":  "failed",
		})
		return
	}

	// Load active settings for filtering
	var currentSettings model.Settings
	if h.settingsStore != nil {
		s, _ := h.settingsStore.Get(r.Context())
		if s != nil {
			currentSettings = *s
		} else {
			currentSettings = model.DefaultSettings()
		}
	} else {
		currentSettings = model.DefaultSettings()
	}

	var savedPosts []model.Post
	passedCount := 0
	skippedCount := 0

	var postsToProcess []model.Post
	for _, sp := range scrapeRes.Posts {
		postsToProcess = append(postsToProcess, model.Post{
			URL:     sp.URL,
			Author:  sp.Author,
			Text:    sp.Text,
			Status:  "new",
			FoundAt: time.Now().UTC(),
		})
	}

	if h.pipeline != nil {
		results, _ := h.pipeline.ProcessBatch(r.Context(), postsToProcess, currentSettings)
		for _, res := range results {
			if res.Post != nil {
				savedPosts = append(savedPosts, *res.Post)
				if res.Passed {
					passedCount++
				} else {
					skippedCount++
				}
			}
		}
	} else {
		for _, post := range postsToProcess {
			pass, reason := filter.ApplyFilters(post, currentSettings)
			if !pass {
				post.Status = "skipped"
				post.SkipReason = reason
				skippedCount++
			} else {
				passedCount++
			}

			if h.postStore != nil {
				saved, err := h.postStore.SavePost(r.Context(), &post)
				if err == nil && saved != nil {
					savedPosts = append(savedPosts, *saved)
				}
			} else {
				savedPosts = append(savedPosts, post)
			}
		}
	}

	finishedAt := time.Now().UTC()
	stats := map[string]any{
		"total_found":   len(scrapeRes.Posts),
		"filter_passed": passedCount,
		"filter_skipped": skippedCount,
	}

	if h.postStore != nil && runRecord.ID != "" {
		_ = h.postStore.UpdateRun(r.Context(), runRecord.ID, "completed", finishedAt, stats)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"run_id":      runRecord.ID,
		"status":      "completed",
		"stats":       stats,
		"total_posts": len(savedPosts),
		"posts":       savedPosts,
	})
}

// ListRuns handles GET /api/runs.
func (h *PostHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	if h.postStore == nil {
		writeJSON(w, http.StatusOK, []model.Run{})
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	runs, err := h.postStore.GetRuns(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if runs == nil {
		runs = []model.Run{}
	}

	writeJSON(w, http.StatusOK, runs)
}

// ListContacts handles GET /api/contacts.
func (h *PostHandler) ListContacts(w http.ResponseWriter, r *http.Request) {
	if h.contactStore == nil {
		writeJSON(w, http.StatusOK, []model.Contact{})
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	contacts, err := h.contactStore.GetContacts(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if contacts == nil {
		contacts = []model.Contact{}
	}

	writeJSON(w, http.StatusOK, contacts)
}
