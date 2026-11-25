package pipeline

// PostExtractionResult defines structured metadata extracted from a post by the LLM.
type PostExtractionResult struct {
	IsHiringPost        bool     `json:"is_hiring_post"`
	Emails              []string `json:"emails"`
	Phones              []string `json:"phones,omitempty"`
	Company             string   `json:"company"`
	Role                string   `json:"role"`
	WorkMode            string   `json:"work_mode"` // "remote", "onsite", "hybrid", "unknown"
	Location            string   `json:"location"`
	ExperienceRequired  string   `json:"experience_required"`
	ExperienceYears     *float64 `json:"experience_years,omitempty"`
	TechStack           []string `json:"tech_stack"`
	StipendText         string   `json:"stipend_text"`
	StipendAmount       *float64 `json:"stipend_amount,omitempty"`
	PaidStatus          string   `json:"paid_status"` // "paid", "unpaid", "unknown"
	ApplyInstructions   string   `json:"apply_instructions"`
	RequiredSubjectLine string   `json:"required_subject_line"`
	ApplyViaEmail       bool     `json:"apply_via_email"`
	IsSuspicious        bool     `json:"is_suspicious"`
	SuspicionReason     string   `json:"suspicion_reason"`
}

// CompanyVerificationResult defines company analysis & red flags produced by the LLM.
type CompanyVerificationResult struct {
	Summary        string   `json:"summary"`         // 2-line company summary
	HasRedFlags    bool     `json:"has_red_flags"`
	RedFlags       []string `json:"red_flags"`       // e.g. "pay-to-apply", "vague", "no real product"
	ProductVerdict string   `json:"product_verdict"` // "valid", "suspicious", "unclear"
}

// VerificationResult contains the comprehensive trust evaluation for a domain/company.
type VerificationResult struct {
	Domain         string   `json:"domain"`
	TrustScore     int      `json:"trust_score"`
	TrustReasons   []string `json:"trust_reasons"`
	CompanySummary string   `json:"company_summary"`
	RedFlags       []string `json:"red_flags"`
	HasMX          bool     `json:"has_mx"`
	IsPersonal     bool     `json:"is_personal"`
}
