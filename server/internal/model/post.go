package model

import "time"

// PostExtracted contains structured metadata extracted from a post.
type PostExtracted struct {
	Stipend             *float64       `json:"stipend,omitempty"`
	ExperienceYears     *float64       `json:"experience_years,omitempty"`
	Keywords            []string       `json:"keywords,omitempty"`
	TechStack           []string       `json:"tech_stack,omitempty"`
	Emails              []string       `json:"emails,omitempty"`
	RequiredSubjectLine string         `json:"required_subject_line,omitempty"`
	ApplyInstructions   string         `json:"apply_instructions,omitempty"`
	CompanySummary      string         `json:"company_summary,omitempty"`
	IsSuspicious        bool           `json:"is_suspicious,omitempty"`
	SuspicionReason     string         `json:"suspicion_reason,omitempty"`
	RedFlags            []string       `json:"red_flags,omitempty"`
	Raw                 map[string]any `json:"raw,omitempty"`
}

// Post represents an opportunity or job post found by scraper/worker.
type Post struct {
	ID         string        `json:"id"`
	URL        string        `json:"url"`
	Author     string        `json:"author"`
	Text       string        `json:"text"`
	Company    string        `json:"company"`
	Role       string        `json:"role"`
	WorkMode   string        `json:"work_mode"`   // e.g. "remote", "hybrid", "onsite"
	PaidStatus string        `json:"paid_status"` // e.g. "paid", "unpaid", "unknown"
	Location   string        `json:"location"`    // e.g. "Bangalore", "San Francisco"
	Extracted  PostExtracted `json:"extracted"`
	Status     string        `json:"status"`
	SkipReason string        `json:"skip_reason,omitempty"`
	FoundAt    time.Time     `json:"found_at"`
	CreatedAt  time.Time     `json:"created_at"`
}
