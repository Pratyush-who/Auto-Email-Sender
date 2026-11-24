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

// ExtractPost analyzes post text using the LLM client with JSON schema and validates email addresses.
func (e *PostExtractor) ExtractPost(ctx context.Context, postText string) (*PostExtractionResult, error) {
	if e.llmClient == nil {
		return nil, fmt.Errorf("llm client is not configured")
	}

	prompt := fmt.Sprintf(`You are an expert recruiter and job scraper parser.
Analyze the following LinkedIn post and extract structured data strictly conforming to the JSON schema.

RULES:
1. is_hiring_post: set true if the post is advertising an open job, internship, contract, or hiring need. Set false if it is personal news, general advice, celebratory, or promotional.
2. emails: extract all contact email addresses mentioned for applying.
3. company: name of the hiring company or startup (infer from context or poster description if not explicit).
4. role: specific job title or role mentioned (e.g. "Go Backend Engineer", "React Developer").
5. work_mode: choose from "remote", "onsite", "hybrid", "unknown".
6. location: city, region, or "Remote".
7. experience_required: description of experience level. If numeric years can be deduced, provide experience_years as a float.
8. tech_stack: array of programming languages, frameworks, and technologies mentioned.
9. paid_status: must be "paid", "unpaid", or "unknown".
   CRITICAL: If the post mentions "unpaid", "no stipend", "stipend: none", "unpaid internship", "for exposure", "exposure", "for experience", or "volunteer", you MUST set paid_status to "unpaid".
10. stipend_text: exact stipend or salary text if mentioned, and stipend_amount as a numeric monthly figure if parseable.
11. apply_instructions: summary of how to apply.
12. required_subject_line: if the poster specifies a required email subject line (e.g. "Subject: Application for Go Dev - [Name]"), extract the exact pattern.
13. apply_via_email: true if an email address is provided or requested for applications.
14. is_suspicious: set true if post asks for payment/training fees/security deposits, promotes MLM/pyramid schemes, uses crypto phishing, or appears to be a fraudulent fake job scam.
15. suspicion_reason: explanation if the post is deemed suspicious or fake.

Post Text:
"""
%s
"""`, postText)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"is_hiring_post":        map[string]any{"type": "boolean"},
			"emails":                map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"company":               map[string]any{"type": "string"},
			"role":                  map[string]any{"type": "string"},
			"work_mode":             map[string]any{"type": "string", "enum": []string{"remote", "onsite", "hybrid", "unknown"}},
			"location":              map[string]any{"type": "string"},
			"experience_required":  map[string]any{"type": "string"},
			"experience_years":     map[string]any{"type": "number"},
			"tech_stack":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"stipend_text":         map[string]any{"type": "string"},
			"stipend_amount":       map[string]any{"type": "number"},
			"paid_status":          map[string]any{"type": "string", "enum": []string{"paid", "unpaid", "unknown"}},
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
		return nil, fmt.Errorf("llm post extraction failed: %w", err)
	}

	// Cross-check emails with regex against the raw post text
	regexEmails := ExtractEmailsWithRegex(postText)
	combinedEmails := append(result.Emails, regexEmails...)
	result.Emails = FilterAndDeduplicateEmails(combinedEmails)

	if len(result.Emails) > 0 {
		result.ApplyViaEmail = true
	}

	// Extra safety check for unpaid keywords in post text
	lowerText := strings.ToLower(postText)
	unpaidIndicators := []string{"unpaid", "no stipend", "for exposure", "for experience", "volunteer"}
	for _, ind := range unpaidIndicators {
		if strings.Contains(lowerText, ind) {
			result.PaidStatus = "unpaid"
			break
		}
	}

	// Heuristic scam & fake job indicators
	scamIndicators := []string{
		"registration fee", "pay application fee", "application fee",
		"security deposit", "deposit for laptop", "wire money",
		"pay to apply", "training fee",
	}
	for _, scam := range scamIndicators {
		if strings.Contains(lowerText, scam) {
			result.IsSuspicious = true
			if result.SuspicionReason == "" {
				result.SuspicionReason = fmt.Sprintf("contains fraudulent scam indicator: '%s'", scam)
			}
			break
		}
	}

	return &result, nil
}
