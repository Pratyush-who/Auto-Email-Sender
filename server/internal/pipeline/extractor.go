package pipeline

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"emailauto-server/internal/llm"
)

var emailRegex = regexp.MustCompile(`(?i)[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)

// PostExtractor handles LLM-based post extraction with JSON schema and regex cross-checking.
type PostExtractor struct {
	llmClient llm.Client
}

// NewPostExtractor creates a new PostExtractor.
func NewPostExtractor(llmClient llm.Client) *PostExtractor {
	return &PostExtractor{llmClient: llmClient}
}

// ExtractEmailsWithRegex scans text and extracts all valid email addresses.
func ExtractEmailsWithRegex(text string) []string {
	matches := emailRegex.FindAllString(text, -1)
	return FilterAndDeduplicateEmails(matches)
}

// FilterAndDeduplicateEmails normalizes emails, removes duplicates, and filters out noreply/info unless only one.
func FilterAndDeduplicateEmails(emails []string) []string {
	seen := make(map[string]bool)
	var cleaned []string

	for _, e := range emails {
		normalized := strings.ToLower(strings.TrimSpace(e))
		normalized = strings.Trim(normalized, "<>(),;:.[]\"'")
		if normalized != "" && !seen[normalized] && emailRegex.MatchString(normalized) {
			seen[normalized] = true
			cleaned = append(cleaned, normalized)
		}
	}

	if len(cleaned) <= 1 {
		return cleaned
	}

	// Ignore noreply / info-style addresses unless they are the only one available
	genericPrefixes := []string{
		"noreply@", "no-reply@", "donotreply@", "do-not-reply@", "bounce@",
		"info@", "notification@", "notifications@",
	}

	var nonGeneric []string
	for _, e := range cleaned {
		isGeneric := false
		for _, prefix := range genericPrefixes {
			if strings.HasPrefix(e, prefix) {
				isGeneric = true
				break
			}
		}
		if !isGeneric {
			nonGeneric = append(nonGeneric, e)
		}
	}

	// Fallback to generic only if no other email exists
	if len(nonGeneric) == 0 {
		return cleaned
	}

	return nonGeneric
}

// ExtractPhonesWithRegex scans text and extracts all phone numbers and WhatsApp links.
func ExtractPhonesWithRegex(text string) []string {
	var results []string
	seen := make(map[string]bool)

	// Check for wa.me links
	waRegex := regexp.MustCompile(`(?i)wa\.me/(\+?[0-9]{10,15})`)
	for _, m := range waRegex.FindAllStringSubmatch(text, -1) {
		if len(m) > 1 && !seen[m[1]] {
			seen[m[1]] = true
			results = append(results, m[1])
		}
	}

	// Indian mobile numbers (10 digits starting with 6,7,8,9, optionally prefixed with +91 or 0)
	inPhoneRegex := regexp.MustCompile(`(?:(?:\+|00)91[\s.-]?|[0])?([6-9]\d{4}[\s.-]?\d{5})`)
	for _, m := range inPhoneRegex.FindAllStringSubmatch(text, -1) {
		if len(m) > 1 {
			cleaned := strings.Map(func(r rune) rune {
				if r >= '0' && r <= '9' {
					return r
				}
				return -1
			}, m[0])
			if len(cleaned) >= 10 && len(cleaned) <= 13 && !seen[cleaned] {
				seen[cleaned] = true
				results = append(results, cleaned)
			}
		}
	}

	return results
}

// ExtractPostWithRegex provides fast deterministic regex and heuristic parsing with zero LLM API calls.
func ExtractPostWithRegex(postText string) *PostExtractionResult {
	emails := ExtractEmailsWithRegex(postText)
	phones := ExtractPhonesWithRegex(postText)
	lowerText := strings.ToLower(postText)

	isHiring := false
	hiringTerms := []string{
		"hiring", "looking for", "job", "opening", "we are seeking",
		"apply", "reach out", "send your resume", "internship", "developer", "engineer",
	}
	for _, term := range hiringTerms {
		if strings.Contains(lowerText, term) {
			isHiring = true
			break
		}
	}

	// Normalize curly apostrophes so phrases like "I’m" match "I'm"
	normalizedLower := strings.ReplaceAll(lowerText, "’", "'")

	// Reject job-seeker posts (#OpenToWork) that appear in hiring searches
	jobSeekerTerms := []string{
		"open to work", "#opentowork", "exploring new career opportunities",
		"looking for opportunities", "seeking new opportunities", "immediate joiner",
		"available for immediate", "refer me", "looking for my next", "i'm currently exploring",
		"i am currently exploring", "my project experience", "i'm currently open to",
		"i am currently open to", "happy to share my cv", "if your company is looking",
		"looking for software engineer", "open to full-time", "open to remote",
	}
	explicitCompanyHiring := strings.Contains(normalizedLower, "we are hiring") ||
		strings.Contains(normalizedLower, "we're hiring") ||
		strings.Contains(normalizedLower, "we are #hiring") ||
		strings.Contains(normalizedLower, "join our team") ||
		strings.Contains(normalizedLower, "send your cv") ||
		strings.Contains(normalizedLower, "send your resume")
	if !explicitCompanyHiring {
		for _, jsTerm := range jobSeekerTerms {
			if strings.Contains(normalizedLower, jsTerm) {
				isHiring = false
				break
			}
		}
	}

	workMode := "unknown"
	if strings.Contains(lowerText, "remote") {
		workMode = "remote"
	} else if strings.Contains(lowerText, "hybrid") {
		workMode = "hybrid"
	} else if strings.Contains(lowerText, "onsite") || strings.Contains(lowerText, "on-site") {
		workMode = "onsite"
	}

	// Assume paid unless explicitly marked unpaid (even if stipend is not mentioned)
	paidStatus := "paid"
	unpaidIndicators := []string{"unpaid", "no stipend", "without pay", "zero stipend", "for exposure", "volunteer"}
	for _, ind := range unpaidIndicators {
		if strings.Contains(lowerText, ind) {
			paidStatus = "unpaid"
			break
		}
	}

	// Try extracting role from known titles
	role := "Software Developer"
	knownRoles := []string{
		"Flutter Developer", "Flutter App Developer", "Go Developer", "Golang Developer",
		"Backend Developer", "Frontend Developer", "Full Stack Developer", "Android Developer",
		"iOS Developer", "Software Engineer",
	}
	for _, kr := range knownRoles {
		if strings.Contains(lowerText, strings.ToLower(kr)) {
			role = kr
			break
		}
	}

	company := ""
	if len(emails) > 0 {
		parts := strings.Split(emails[0], "@")
		if len(parts) == 2 {
			domainParts := strings.Split(parts[1], ".")
			if len(domainParts) >= 2 && domainParts[0] != "gmail" && domainParts[0] != "yahoo" && domainParts[0] != "outlook" && domainParts[0] != "hotmail" {
				company = strings.ToUpper(string(domainParts[0][0])) + domainParts[0][1:]
			}
		}
	}

	// Detect DM / direct message instructions
	applyViaDM := false
	dmPhrases := []string{
		"dm me", "dm us", "send a dm", "drop a dm", "message me", "message us",
		"inbox me", "ping me", "reach out via dm", "connect and message", "direct message",
	}
	for _, phrase := range dmPhrases {
		if strings.Contains(lowerText, phrase) {
			applyViaDM = true
			break
		}
	}

	// Extract required subject line if specified (e.g. "Subject: Flutter Developer - Karachi")
	requiredSubject := ""
	subjRegex := regexp.MustCompile(`(?im)^(?:\s*(?:email\s+)?subject(?:\s+line)?\s*[:\-]\s*)(.+)$`)
	if m := subjRegex.FindStringSubmatch(postText); len(m) > 1 {
		requiredSubject = strings.TrimSpace(m[1])
		if len(requiredSubject) > 120 {
			requiredSubject = requiredSubject[:120]
		}
	}

	applyInstructions := "Apply via email or contact details in post"
	if len(emails) == 0 && applyViaDM {
		applyInstructions = "Message / DM directly on LinkedIn"
	} else if len(emails) == 0 && len(phones) > 0 {
		applyInstructions = "Contact via Phone / WhatsApp"
	}

	return &PostExtractionResult{
		IsHiringPost:        isHiring,
		Emails:              emails,
		Phones:              phones,
		Company:             company,
		Role:                role,
		WorkMode:            workMode,
		PaidStatus:          paidStatus,
		ApplyViaEmail:       len(emails) > 0,
		ApplyViaDM:          applyViaDM,
		RequiredSubjectLine: requiredSubject,
		ApplyInstructions:   applyInstructions,
	}
}

// ExtractPost analyzes post text using fast regex pre-filtering and optional LLM enrichment with fallback.
func (e *PostExtractor) ExtractPost(ctx context.Context, postText string) (*PostExtractionResult, error) {
	regexEmails := ExtractEmailsWithRegex(postText)
	regexPhones := ExtractPhonesWithRegex(postText)

	if len(regexEmails) == 0 && len(regexPhones) == 0 {
		return ExtractPostWithRegex(postText), nil
	}

	if e.llmClient == nil {
		return ExtractPostWithRegex(postText), nil
	}

	prompt := fmt.Sprintf(`Extract structured data strictly conforming to JSON schema from: %s`, postText)
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"is_hiring_post":        map[string]any{"type": "boolean"},
			"emails":                map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"company":               map[string]any{"type": "string"},
			"role":                  map[string]any{"type": "string"},
			"work_mode":             map[string]any{"type": "string"},
			"location":              map[string]any{"type": "string"},
			"experience_required":  map[string]any{"type": "string"},
			"experience_years":     map[string]any{"type": "number"},
			"tech_stack":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"stipend_text":         map[string]any{"type": "string"},
			"stipend_amount":       map[string]any{"type": "number"},
			"paid_status":          map[string]any{"type": "string"},
			"apply_instructions":   map[string]any{"type": "string"},
			"required_subject_line": map[string]any{"type": "string"},
			"apply_via_email":       map[string]any{"type": "boolean"},
			"is_suspicious":         map[string]any{"type": "boolean"},
			"suspicion_reason":      map[string]any{"type": "string"},
		},
		"required": []string{"is_hiring_post", "emails", "company", "role", "work_mode", "paid_status", "apply_via_email"},
	}

	var result PostExtractionResult
	if err := e.llmClient.GenerateJSON(ctx, prompt, schema, &result); err != nil {
		return ExtractPostWithRegex(postText), nil
	}

	result.Phones = regexPhones
	result.Emails = FilterAndDeduplicateEmails(append(result.Emails, regexEmails...))
	if len(result.Emails) > 0 {
		result.ApplyViaEmail = true
	}

	lowerText := strings.ToLower(postText)
	unpaidIndicators := []string{"unpaid", "no stipend", "for exposure", "exposure", "for experience", "volunteer"}
	for _, ind := range unpaidIndicators {
		if strings.Contains(lowerText, ind) {
			result.PaidStatus = "unpaid"
			break
		}
	}

	return &result, nil
}
