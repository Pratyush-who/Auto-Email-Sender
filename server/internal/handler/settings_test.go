package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"emailauto-server/internal/model"
)

type mockSettingsStore struct {
	settings *model.Settings
	saveErr  error
	getErr   error
}

func (m *mockSettingsStore) Get(ctx context.Context) (*model.Settings, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.settings == nil {
		defaults := model.DefaultSettings()
		return &defaults, nil
	}
	return m.settings, nil
}

func (m *mockSettingsStore) Save(ctx context.Context, s *model.Settings) (*model.Settings, error) {
	if m.saveErr != nil {
		return nil, m.saveErr
	}
	m.settings = s
	return s, nil
}

func TestGetSettings(t *testing.T) {
	mockStore := &mockSettingsStore{}
	handler := NewSettingsHandler(mockStore)

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rec := httptest.NewRecorder()

	handler.GetSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var res model.Settings
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse response json: %v", err)
	}

	if !res.SkipUnpaid {
		t.Errorf("expected SkipUnpaid to default to true")
	}
	if res.TrustThreshold != 40 {
		t.Errorf("expected TrustThreshold to default to 40, got %d", res.TrustThreshold)
	}
}

func TestUpdateSettings_Valid(t *testing.T) {
	mockStore := &mockSettingsStore{}
	handler := NewSettingsHandler(mockStore)

	updated := model.DefaultSettings()
	stipend := 25000.0
	updated.MinStipend = &stipend
	updated.AllowedLocations = []string{"Bangalore", "Remote"}
	updated.DailyCap = 50

	body, _ := json.Marshal(updated)
	req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.UpdateSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var res model.Settings
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if res.MinStipend == nil || *res.MinStipend != 25000.0 {
		t.Errorf("expected MinStipend 25000, got %v", res.MinStipend)
	}
	if res.DailyCap != 50 {
		t.Errorf("expected DailyCap 50, got %d", res.DailyCap)
	}
}

func TestUpdateSettings_ValidationFailure(t *testing.T) {
	mockStore := &mockSettingsStore{}
	handler := NewSettingsHandler(mockStore)

	invalid := model.DefaultSettings()
	negStipend := -500.0
	invalid.MinStipend = &negStipend
	invalid.DailyCap = -10
	invalid.GapMinMinutes = 10
	invalid.GapMaxMinutes = 2 // max < min

	body, _ := json.Marshal(invalid)
	req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.UpdateSettings(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status 422 Unprocessable Entity, got %d", rec.Code)
	}

	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode error body: %v", err)
	}

	fields, ok := res["fields"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'fields' in validation response: %+v", res)
	}

	if _, ok := fields["min_stipend"]; !ok {
		t.Errorf("expected error for min_stipend")
	}
	if _, ok := fields["daily_cap"]; !ok {
		t.Errorf("expected error for daily_cap")
	}
	if _, ok := fields["gap_max_minutes"]; !ok {
		t.Errorf("expected error for gap_max_minutes")
	}
}
