package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"emailauto-server/internal/model"
	"emailauto-server/internal/worker"
)

type mockPostStore struct {
	posts []model.Post
	runs  []model.Run
}

func (m *mockPostStore) SavePost(ctx context.Context, p *model.Post) (*model.Post, error) {
	if p.ID == "" {
		p.ID = "p_" + p.URL
	}
	m.posts = append(m.posts, *p)
	return p, nil
}

func (m *mockPostStore) GetPosts(ctx context.Context, limit, offset int) ([]model.Post, error) {
	return m.posts, nil
}

func (m *mockPostStore) CreateRun(ctx context.Context, run *model.Run) (*model.Run, error) {
	run.ID = "run_test_123"
	m.runs = append(m.runs, *run)
	return run, nil
}

func (m *mockPostStore) UpdateRun(ctx context.Context, runID, status string, finishedAt time.Time, stats map[string]any) error {
	for i := range m.runs {
		if m.runs[i].ID == runID {
			m.runs[i].Status = status
			m.runs[i].FinishedAt = &finishedAt
			m.runs[i].Stats = stats
			break
		}
	}
	return nil
}

func (m *mockPostStore) GetRuns(ctx context.Context, limit int) ([]model.Run, error) {
	return m.runs, nil
}

type mockWorkerScraper struct {
	response *worker.ScrapeResponse
	err      error
}

func (m *mockWorkerScraper) Scrape(ctx context.Context, req worker.ScrapeRequest) (*worker.ScrapeResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func TestManualImportPost_Success(t *testing.T) {
	store := &mockPostStore{}
	settingsStore := &mockSettingsStore{
		settings: &model.Settings{
			SkipUnpaid: false,
		},
	}
	handler := NewPostHandler(store, nil, settingsStore, nil, nil)

	payload := map[string]any{
		"url":     "https://www.linkedin.com/feed/update/urn:li:activity:12345678/",
		"text":    "Hiring Go and React engineers for high throughput systems.",
		"company": "NextGen Labs",
		"role":    "Full Stack Engineer",
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/posts/manual", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ManualImportPost(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res["message"] != "post imported successfully" {
		t.Errorf("unexpected message: %v", res["message"])
	}

	if len(store.posts) != 1 {
		t.Fatalf("expected 1 post saved, got %d", len(store.posts))
	}
	if store.posts[0].Status != "new" {
		t.Errorf("expected status 'new', got %s", store.posts[0].Status)
	}
}

func TestManualImportPost_FilterSkipped(t *testing.T) {
	store := &mockPostStore{}
	settingsStore := &mockSettingsStore{
		settings: &model.Settings{
			ExcludeKeywords: []string{"crypto", "blockchain"},
		},
	}
	handler := NewPostHandler(store, nil, settingsStore, nil, nil)

	payload := map[string]any{
		"url":  "https://www.linkedin.com/feed/update/urn:li:activity:99999999/",
		"text": "Join our web3 startup as a crypto protocol lead.",
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/posts/manual", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ManualImportPost(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if len(store.posts) != 1 {
		t.Fatalf("expected 1 post saved, got %d", len(store.posts))
	}
	if store.posts[0].Status != "skipped" {
		t.Errorf("expected status 'skipped', got %s", store.posts[0].Status)
	}
	if store.posts[0].SkipReason == "" {
		t.Errorf("expected skip_reason to be populated")
	}
}

func TestManualImportPost_MissingFields(t *testing.T) {
	handler := NewPostHandler(&mockPostStore{}, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/posts/manual", bytes.NewReader([]byte(`{"url": ""}`)))
	rec := httptest.NewRecorder()

	handler.ManualImportPost(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for missing url, got %d", rec.Code)
	}
}

func TestTriggerRun(t *testing.T) {
	store := &mockPostStore{}
	mockWorker := &mockWorkerScraper{
		response: &worker.ScrapeResponse{
			Success:        true,
			TotalExtracted: 2,
			Posts: []worker.ScrapedPost{
				{
					URL:    "https://linkedin.com/feed/update/1",
					Author: "Alice HR",
					Text:   "Hiring Go developers remote",
				},
				{
					URL:    "https://linkedin.com/feed/update/2",
					Author: "Bob Recruiter",
					Text:   "Crypto smart contract position",
				},
			},
		},
	}

	settingsStore := &mockSettingsStore{
		settings: &model.Settings{
			ExcludeKeywords: []string{"crypto"},
		},
	}

	handler := NewPostHandler(store, nil, settingsStore, mockWorker, nil)

	body := []byte(`{"query": "golang remote", "time_filter": "24h", "sort_by": "latest", "max_posts": 10}`)
	req := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.TriggerRun(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res["status"] != "completed" {
		t.Errorf("expected run status 'completed', got %v", res["status"])
	}

	// Two posts processed
	if len(store.posts) != 2 {
		t.Fatalf("expected 2 posts saved, got %d", len(store.posts))
	}
	// Post 1 passed
	if store.posts[0].Status != "new" {
		t.Errorf("expected post 1 to pass with status 'new', got %s", store.posts[0].Status)
	}
	// Post 2 skipped due to crypto keyword
	if store.posts[1].Status != "skipped" {
		t.Errorf("expected post 2 to be skipped, got %s", store.posts[1].Status)
	}
}
