package model

import (
	"strings"
	"time"
)

// Settings represents all configurable search, filter, and scheduling options.
// All filters are optional (nil = disabled).
type Settings struct {
	MinStipend         *float64  `json:"min_stipend"`          // null = disabled
	AllowedLocations   []string  `json:"allowed_locations"`    // null/empty = disabled
	WorkModes          []string  `json:"work_modes"`           // null/empty = disabled
	MinExperienceYears *float64  `json:"min_experience_years"` // null = disabled
	MaxExperienceYears *float64  `json:"max_experience_years"` // null = disabled
	IncludeKeywords    []string  `json:"include_keywords"`     // null/empty = disabled
	ExcludeKeywords    []string  `json:"exclude_keywords"`     // null/empty = disabled
	SkipUnpaid         bool      `json:"skip_unpaid"`          // default true
	SkipUnknownPaid    bool      `json:"skip_unknown_paid"`     // default false
	TrustThreshold     int       `json:"trust_threshold"`      // default 40
	DailyCap           int       `json:"daily_cap"`            // default 30
	SendWindow         string    `json:"send_window"`          // default "09:00-19:00 IST"
	GapMinMinutes      int       `json:"gap_min_minutes"`      // default 2
	GapMaxMinutes      int       `json:"gap_max_minutes"`      // default 6
	ApprovalMode       bool      `json:"approval_mode"`        // default true
	FollowupAfterHours int       `json:"followup_after_hours"` // default 24
	MaxFollowups       int       `json:"max_followups"`        // default 2
	UpdatedAt          time.Time `json:"updated_at,omitempty"`
}

// DefaultSettings returns a Settings instance populated with all standard defaults.
func DefaultSettings() Settings {
	return Settings{
		MinStipend:         nil,
		AllowedLocations:   nil,
		WorkModes:          nil,
		MinExperienceYears: nil,
		MaxExperienceYears: nil,
		IncludeKeywords:    nil,
		ExcludeKeywords:    nil,
		SkipUnpaid:         true,
		SkipUnknownPaid:    false,
		TrustThreshold:     40,
		DailyCap:           30,
		SendWindow:         "09:00-19:00 IST",
		GapMinMinutes:      2,
		GapMaxMinutes:      6,
		ApprovalMode:       true,
		FollowupAfterHours: 24,
		MaxFollowups:       2,
		UpdatedAt:          time.Now().UTC(),
	}
}

// Validate checks settings values and returns a map of field -> error message.
func (s *Settings) Validate() map[string]string {
	errs := make(map[string]string)

	if s.MinStipend != nil && *s.MinStipend < 0 {
		errs["min_stipend"] = "min_stipend cannot be negative"
	}
	if s.MinExperienceYears != nil && *s.MinExperienceYears < 0 {
		errs["min_experience_years"] = "min_experience_years cannot be negative"
	}
	if s.MaxExperienceYears != nil && *s.MaxExperienceYears < 0 {
		errs["max_experience_years"] = "max_experience_years cannot be negative"
	}
	if s.MinExperienceYears != nil && s.MaxExperienceYears != nil && *s.MinExperienceYears > *s.MaxExperienceYears {
		errs["experience_years"] = "min_experience_years cannot be greater than max_experience_years"
	}
	if s.GapMinMinutes <= 0 {
		errs["gap_min_minutes"] = "gap_min_minutes must be greater than 0"
	}
	if s.GapMaxMinutes < s.GapMinMinutes {
		errs["gap_max_minutes"] = "gap_max_minutes must be greater than or equal to gap_min_minutes"
	}
	if s.TrustThreshold < 0 || s.TrustThreshold > 100 {
		errs["trust_threshold"] = "trust_threshold must be between 0 and 100"
	}
	if s.DailyCap <= 0 {
		errs["daily_cap"] = "daily_cap must be greater than 0"
	}
	if strings.TrimSpace(s.SendWindow) == "" {
		errs["send_window"] = "send_window cannot be empty"
	}
	if s.FollowupAfterHours <= 0 {
		errs["followup_after_hours"] = "followup_after_hours must be greater than 0"
	}
	if s.MaxFollowups < 0 {
		errs["max_followups"] = "max_followups cannot be negative"
	}

	return errs
}
