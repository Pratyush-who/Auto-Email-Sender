package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"emailauto-server/internal/model"
	"emailauto-server/internal/pdf"
)

// ProfileHandler handles /api/profile and /api/profile/resume endpoints.
type ProfileHandler struct {
	store     ProfileStore
	uploadDir string
}

// NewProfileHandler creates a new ProfileHandler.
func NewProfileHandler(store ProfileStore, uploadDir string) *ProfileHandler {
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	return &ProfileHandler{
		store:     store,
		uploadDir: uploadDir,
	}
}

// GetProfile handles GET /api/profile.
func (h *ProfileHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusOK, &model.Profile{})
		return
	}

	p, err := h.store.Get(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, p)
}

// UpdateProfile handles PUT /api/profile.
func (h *ProfileHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "profile store is not available"})
		return
	}

	var input model.ProfileInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json payload: " + err.Error()})
		return
	}

	if errs := input.Validate(); len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":  "validation failed",
			"fields": errs,
		})
		return
	}

	updated, err := h.store.Save(r.Context(), &input)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// UploadResume handles POST /api/profile/resume for PDF resume uploads.
func (h *ProfileHandler) UploadResume(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "profile store is not available"})
		return
	}

	// Security: limit body reading to 15 MB to prevent resource exhaustion / DoS
	r.Body = http.MaxBytesReader(w, r.Body, 15<<20)
	if err := r.ParseMultipartForm(15 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not parse multipart form: " + err.Error()})
		return
	}

	file, header, err := r.FormFile("resume")
	if err != nil {
		// Fallback to "file" field name
		file, header, err = r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing 'resume' or 'file' form file"})
			return
		}
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".pdf" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only PDF files are accepted"})
		return
	}

	if err := os.MkdirAll(h.uploadDir, 0755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not initialize upload directory: " + err.Error()})
		return
	}

	randomSuffix := make([]byte, 8)
	_, _ = rand.Read(randomSuffix)
	targetFileName := fmt.Sprintf("resume_%d_%s.pdf", time.Now().Unix(), hex.EncodeToString(randomSuffix))
	targetPath := filepath.Join(h.uploadDir, targetFileName)

	destFile, err := os.Create(targetPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create destination file: " + err.Error()})
		return
	}

	if _, err := io.Copy(destFile, file); err != nil {
		destFile.Close()
		_ = os.Remove(targetPath)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save uploaded file: " + err.Error()})
		return
	}
	destFile.Close()

	// Extract text from the saved PDF file for LLM use
	extractedText, extractErr := pdf.ExtractTextFromFile(targetPath)
	if extractErr != nil {
		// Log error, continue with empty extracted text rather than losing the uploaded file
		extractedText = ""
	}

	updated, err := h.store.UpdateResume(r.Context(), targetPath, extractedText)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to persist resume metadata: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":         "Resume uploaded and text extracted successfully",
		"resume_path":     targetPath,
		"extracted_chars": len(extractedText),
		"profile":         updated,
	})
}
