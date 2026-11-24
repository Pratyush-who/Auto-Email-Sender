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
	"emailauto-server/internal/sender"

	"github.com/go-chi/chi/v5"
)

type mockOutreachStoreForHandler struct {
	items []model.Outreach
}

func (m *mockOutreachStoreForHandler) CreateOutreach(ctx context.Context, o *model.Outreach) (*model.Outreach, error) {
	o.ID = "outreach_1"
	m.items = append(m.items, *o)
	return o, nil
}

func (m *mockOutreachStoreForHandler) GetOutreachByID(ctx context.Context, id string) (*model.Outreach, error) {
	for _, it := range m.items {
		if it.ID == id {
			return &it, nil
		}
	}
	return nil, nil
}

func (m *mockOutreachStoreForHandler) UpdateOutreach(ctx context.Context, o *model.Outreach) error {
	for i := range m.items {
		if m.items[i].ID == o.ID {
			m.items[i] = *o
			return nil
		}
	}
	return nil
}

func (m *mockOutreachStoreForHandler) ListOutreach(ctx context.Context, filter model.OutreachFilter) ([]model.Outreach, error) {
	return m.items, nil
}

func (m *mockOutreachStoreForHandler) GetSentBodiesToday(ctx context.Context) ([]string, error) {
	return []string{}, nil
}

func (m *mockOutreachStoreForHandler) UpdateStatus(ctx context.Context, id string, status model.OutreachStatus, gmailMsgID, gmailThreadID, errText string, sentAt *time.Time) error {
	for i := range m.items {
		if m.items[i].ID == id {
			m.items[i].Status = status
			return nil
		}
	}
	return nil
}

func (m *mockOutreachStoreForHandler) CountSentToday(ctx context.Context) (int, error) {
	return 0, nil
}

func (m *mockOutreachStoreForHandler) HasSentToAddressOrCompanyWithinDays(ctx context.Context, email, company string, days int) (bool, error) {
	return false, nil
}

func (m *mockOutreachStoreForHandler) HasExistingInitialOutreach(ctx context.Context, contactID string) (bool, error) {
	return false, nil
}

func (m *mockOutreachStoreForHandler) GetPendingOutreachToSend(ctx context.Context, limit int) ([]model.Outreach, error) {
	return m.items, nil
}

func (m *mockOutreachStoreForHandler) MarkContactInvalid(ctx context.Context, contactID string, reason string) error {
	return nil
}

func TestOutreachHandler_ListAndApprove(t *testing.T) {
	store := &mockOutreachStoreForHandler{
		items: []model.Outreach{
			{
				ID:      "o_test_1",
				Subject: "Initial draft",
				Body:    "Hello recruiter",
				Status:  model.OutreachStatusDraft,
			},
		},
	}

	h := NewOutreachHandler(store, nil, nil, nil)

	// List
	req := httptest.NewRequest(http.MethodGet, "/api/outreach", nil)
	rec := httptest.NewRecorder()
	h.ListOutreach(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var items []model.Outreach
	_ = json.Unmarshal(rec.Body.Bytes(), &items)
	if len(items) != 1 || items[0].ID != "o_test_1" {
		t.Fatalf("unexpected items: %+v", items)
	}

	// Approve
	r := chi.NewRouter()
	r.Post("/api/outreach/{id}/approve", h.ApproveOutreach)

	reqApprove := httptest.NewRequest(http.MethodPost, "/api/outreach/o_test_1/approve", nil)
	recApprove := httptest.NewRecorder()
	r.ServeHTTP(recApprove, reqApprove)

	if recApprove.Code != http.StatusOK {
		t.Fatalf("expected 200 on approve, got %d", recApprove.Code)
	}
	if store.items[0].Status != model.OutreachStatusQueued {
		t.Errorf("expected status to be queued, got %s", store.items[0].Status)
	}
}

func TestOutreachHandler_UpdateDraft(t *testing.T) {
	store := &mockOutreachStoreForHandler{
		items: []model.Outreach{
			{
				ID:      "o_edit",
				Subject: "Original Subject",
				Body:    "Original Body",
				Status:  model.OutreachStatusDraft,
			},
		},
	}

	h := NewOutreachHandler(store, nil, nil, nil)
	r := chi.NewRouter()
	r.Put("/api/outreach/{id}", h.UpdateOutreach)

	updatePayload := map[string]any{
		"subject": "Edited Subject Line",
		"body":    "Edited Body Content",
	}
	raw, _ := json.Marshal(updatePayload)
	req := httptest.NewRequest(http.MethodPut, "/api/outreach/o_edit", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.items[0].Subject != "Edited Subject Line" {
		t.Errorf("expected edited subject, got %s", store.items[0].Subject)
	}
}

func TestSenderHandler_PauseResumeStop(t *testing.T) {
	store := &mockOutreachStoreForHandler{}
	s := sender.NewSender(store, nil, nil, nil, nil)
	h := NewSenderHandler(s)

	// Pause
	reqPause := httptest.NewRequest(http.MethodPost, "/api/sender/pause", nil)
	recPause := httptest.NewRecorder()
	h.Pause(recPause, reqPause)
	if recPause.Code != http.StatusOK {
		t.Fatalf("expected 200 on pause, got %d", recPause.Code)
	}

	// Status
	reqStatus := httptest.NewRequest(http.MethodGet, "/api/sender/status", nil)
	recStatus := httptest.NewRecorder()
	h.GetStatus(recStatus, reqStatus)
	var report sender.StatusReport
	_ = json.Unmarshal(recStatus.Body.Bytes(), &report)
	if report.State != sender.SenderStatusPaused {
		t.Errorf("expected state paused, got %s", report.State)
	}

	// Resume
	reqResume := httptest.NewRequest(http.MethodPost, "/api/sender/resume", nil)
	recResume := httptest.NewRecorder()
	h.Resume(recResume, reqResume)
	if recResume.Code != http.StatusOK {
		t.Fatalf("expected 200 on resume, got %d", recResume.Code)
	}

	// Stop-All
	reqStop := httptest.NewRequest(http.MethodPost, "/api/sender/stop-all", nil)
	recStop := httptest.NewRecorder()
	h.StopAll(recStop, reqStop)
	if recStop.Code != http.StatusOK {
		t.Fatalf("expected 200 on stop-all, got %d", recStop.Code)
	}
}
