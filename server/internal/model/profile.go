package model

import (
	"strings"
	"time"
)

// Profile represents the candidate's background, skills, resume, and preferences.
type Profile struct {
	ID          string    `json:"id"`
	Summary     string    `json:"summary"`
	Skills      string    `json:"skills"`
	ResumePath  string    `json:"resume_path"`
	ResumeText  string    `json:"resume_text"`
	ExampleText string    `json:"example_text"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ProfileInput represents the editable text fields for a Profile.
type ProfileInput struct {
	Summary     string `json:"summary"`
	Skills      string `json:"skills"`
	ExampleText string `json:"example_text"`
}

// Validate checks profile input fields and returns any validation errors.
func (p *ProfileInput) Validate() map[string]string {
	errs := make(map[string]string)
	if len(p.Summary) > 10000 {
		errs["summary"] = "summary exceeds maximum allowed length of 10000 characters"
	}
	if len(p.Skills) > 5000 {
		errs["skills"] = "skills exceeds maximum allowed length of 5000 characters"
	}
	if len(p.ExampleText) > 10000 {
		errs["example_text"] = "example_text exceeds maximum allowed length of 10000 characters"
	}
	p.Summary = strings.TrimSpace(p.Summary)
	p.Skills = strings.TrimSpace(p.Skills)
	p.ExampleText = strings.TrimSpace(p.ExampleText)
	return errs
}
