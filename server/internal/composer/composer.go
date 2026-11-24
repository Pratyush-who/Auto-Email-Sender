package composer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"emailauto-server/internal/llm"
	"emailauto-server/internal/model"
)

// ComposedEmail represents the structured output of the Composer.
type ComposedEmail struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// Composer handles crafting customized cold outreach emails via LLM with strict guardrails.
type Composer struct {
	llmClient llm.Client
	logger    *slog.Logger
}

// NewComposer creates a new Composer instance.
func NewComposer(llmClient llm.Client, logger *slog.Logger) *Composer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Composer{
		llmClient: llmClient,
		logger:    logger,
	}
}

// Compose creates an initial outreach draft adhering strictly to prompt instructions and similarity constraints.
func (c *Composer) Compose(
	ctx context.Context,
	profile model.Profile,
	post model.Post,
	companySummary string,
	existingBodiesToday []string,
) (*ComposedEmail, error) {
	if c.llmClient == nil {
		return nil, fmt.Errorf("llm client is not configured in composer")
	}

	role := strings.TrimSpace(post.Role)
	if role == "" {
		role = "Software Engineer"
	}
	company := strings.TrimSpace(post.Company)
	if company == "" {
		company = "your company"
	}

	// Location rule: Never mention location, relocation, remote work, or work arrangements.
	// As instructed: for remote/hybrid/unmentioned it is obvious everyone is up for it; for onsite do not mention anything.
	locationRule := "Do NOT mention location, relocation, remote work, or work arrangements anywhere in the email. It is obvious and self-evident. Focus strictly on matching technical skills, relevant background, and expressing availability to discuss the role."

	requiredSubjectNotice := ""
	if strings.TrimSpace(post.Extracted.RequiredSubjectLine) != "" {
		requiredSubjectNotice = fmt.Sprintf(
			"MANDATORY SUBJECT FORMAT: The job post specifies a required subject line: '%s'. You MUST follow this exact subject format.",
			post.Extracted.RequiredSubjectLine,
		)
	}

	exampleToneNotice := ""
	if strings.TrimSpace(profile.ExampleText) != "" {
		exampleToneNotice = fmt.Sprintf(
			"TONE & STYLE REFERENCE (mimic this tone, brevity, and voice):\n\"\"\"\n%s\n\"\"\"",
			profile.ExampleText,
		)
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"subject": map[string]any{
				"type":        "string",
				"description": "Email subject line (clean, professional, or strictly conforming to required format)",
			},
			"body": map[string]any{
				"type":        "string",
				"description": "Plain text email body under ~150 words with no markdown code blocks and no emojis",
			},
		},
		"required": []string{"subject", "body"},
	}

	// Attempt generation (with 1 retry if body is near-identical to an existing email sent today)
	var lastEmail *ComposedEmail
	for attempt := 0; attempt < 2; attempt++ {
		similarityWarning := ""
		if attempt > 0 {
			similarityWarning = "CRITICAL: Your previous draft was too similar to an email already drafted today. Use a completely distinct opening sentence, alternative phrasing, and varied structure."
		}

		prompt := fmt.Sprintf(`You are an expert cold email writer crafting a direct, high-conversion application email to a recruiter or hiring manager.

INPUT DATA:
- Role: %s
- Company: %s
- Work Mode: %s (%s)
- Company Summary: %s
- Job Post Context:
"""
%s
"""

MY CANDIDATE PROFILE (NEVER invent experience, skills, degrees, or certifications outside of this):
- Summary: %s
- Skills: %s
- Resume Background:
"""
%s
"""
%s

RULES:
1. Target length: Under ~150 words total. Plain text only. NO markdown code blocks, NO HTML.
2. Specifically reference the role ("%s") and company ("%s").
3. Highlight 1-2 matching skills or projects ONLY from MY profile (never hallucinate experience).
4. Location rule: %s
5. Filler & Tone: NO generic filler (e.g. "I hope this email finds you well", "I am writing to eagerly apply"). Start strong and directly.
6. NO emojis anywhere in the subject or body.
7. Natural, professional sign-off with candidate name.
8. Explicitly mention that my resume is attached.
9. %s
%s
%s

Return JSON with "subject" and "body".`,
			role, company, post.WorkMode, post.Location, companySummary,
			post.Text, profile.Summary, profile.Skills, profile.ResumeText,
			exampleToneNotice, role, company, locationRule,
			requiredSubjectNotice, similarityWarning,
			"Make the email sound like a genuine, sharp developer reaching out directly.",
		)

		var res ComposedEmail
		if err := c.llmClient.GenerateJSON(ctx, prompt, schema, &res); err != nil {
			return nil, fmt.Errorf("llm email generation failed: %w", err)
		}

		res.Subject = strings.TrimSpace(res.Subject)
		res.Body = strings.TrimSpace(res.Body)

		// Sanitize any stray backticks or emojis
		res.Body = strings.ReplaceAll(res.Body, "```", "")

		lastEmail = &res

		// Similarity check against existing bodies created today
		if len(existingBodiesToday) > 0 {
			isNear, simScore := IsNearIdentical(res.Body, existingBodiesToday, 0.70)
			if isNear {
				c.logger.Warn("composed body is near-identical to existing email today, retrying with variation",
					"similarity", simScore,
					"attempt", attempt,
				)
				continue
			}
		}

		// Passed similarity check
		break
	}

	return lastEmail, nil
}
