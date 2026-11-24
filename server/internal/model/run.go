package model

import "time"

// Run represents a scrape or search execution recorded in the runs table.
type Run struct {
	ID         string         `json:"id"`
	Query      string         `json:"query"`
	TimeFilter string         `json:"time_filter"`
	SortBy     string         `json:"sort_by"`
	Trigger    string         `json:"trigger"` // "manual" | "auto"
	Status     string         `json:"status"`  // "pending" | "running" | "completed" | "failed"
	StartedAt  *time.Time     `json:"started_at,omitempty"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
	Stats      map[string]any `json:"stats"`
	CreatedAt  time.Time      `json:"created_at"`
}
