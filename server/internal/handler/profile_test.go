package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"emailauto-server/internal/model"
)

type mockProfileStore struct {
	profile *model.Profile
	getErr  error
	saveErr error
}

func (m *mockProfileStore) Get(ctx context.Context) (*model.Profile, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.profile == nil {
		return &model.Profile{
			Summary: "",
			Skills:  "",
		}, nil
	}
	return m.profile, nil
}

func (m *mockProfileStore) Save(ctx context.Context, input *model.ProfileInput) (*model.Profile, error) {
	if m.saveErr != nil {
		return nil, m.saveErr
	}
	if m.profile == nil {
		m.profile = &model.Profile{}
	}
	m.profile.Summary = input.Summary
	m.profile.Skills = input.Skills
	m.profile.ExampleText = input.ExampleText
	return m.profile, nil
}

func (m *mockProfileStore) UpdateResume(ctx context.Context, resumePath, resumeText string) (*model.Profile, error) {
	if m.profile == nil {
		m.profile = &model.Profile{}
	}
	m.profile.ResumePath = resumePath
	m.profile.ResumeText = resumeText
	return m.profile, nil
}

func TestGetProfile(t *testing.T) {
	store := &mockProfileStore{
		profile: &model.Profile{
			Summary: "Senior Go Developer",
			Skills:  "Go, Kubernetes, Postgres",
		},
	}
	handler := NewProfileHandler(store, "./test_uploads")

	req := httptest.NewRequest(http.MethodGet, "/api/profile", nil)
	rec := httptest.NewRecorder()

	handler.GetProfile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var res model.Profile
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.Summary != "Senior Go Developer" {
		t.Errorf("unexpected summary: %s", res.Summary)
	}
}

func TestUpdateProfile(t *testing.T) {
	store := &mockProfileStore{}
	handler := NewProfileHandler(store, "./test_uploads")

	input := model.ProfileInput{
		Summary:     "Backend Engineer",
		Skills:      "Go, Docker",
		ExampleText: "Hello {name}, I'm interested in your opening...",
	}

	body, _ := json.Marshal(input)
	req := httptest.NewRequest(http.MethodPut, "/api/profile", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.UpdateProfile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var res model.Profile
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse profile response: %v", err)
	}

	if res.Summary != "Backend Engineer" || res.Skills != "Go, Docker" {
		t.Errorf("profile not updated as expected: %+v", res)
	}
}

func TestUploadResume_InvalidFileType(t *testing.T) {
	store := &mockProfileStore{}
	handler := NewProfileHandler(store, "./test_uploads")

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("resume", "resume.txt")
	_, _ = io.WriteString(part, "plain text resume")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/profile/resume", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	handler.UploadResume(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for non-pdf file, got %d", rec.Code)
	}
}

func TestUploadResume_ValidFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "resume_upload_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := &mockProfileStore{}
	handler := NewProfileHandler(store, tempDir)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("resume", "sample_resume.pdf")
	// Minimal PDF header bytes
	_, _ = io.WriteString(part, "%PDF-1.4\n%%EOF")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/profile/resume", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	handler.UploadResume(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for valid upload, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	savedPath, ok := res["resume_path"].(string)
	if !ok || savedPath == "" {
		t.Fatalf("expected resume_path in response: %+v", res)
	}

	if _, err := os.Stat(savedPath); os.IsNotExist(err) {
		t.Errorf("saved file does not exist on disk at %s", savedPath)
	}
	_ = filepath.Base(savedPath)
}
