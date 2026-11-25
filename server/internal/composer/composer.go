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

		prompt := fmt.Sprintf(`You are writing a concise, authentic developer application email on behalf of Pratyush Mehra to a recruiter or hiring manager.
You MUST personalize this email specifically for %s and the role of %s. Weave in authentic details from the job post (such as their specific product domain, app features, state management e.g. Bloc/Provider, API integration, or system requirements).
Do NOT write generic robotic corporate fluff. Keep Pratyush's direct developer tone, bullet-point highlights, and exact signature.

INPUT JOB DATA:
- Target Role: %s
- Target Company: %s
- Work Mode: %s (%s)
- Job Post Context:
"""
%s
"""
- Company Summary & Due Diligence:
"""
%s
"""

CANDIDATE INFORMATION:
- Name: Pratyush Mehra
- Experience: 2+ years building and deploying responsive production apps with REST APIs, clean code, and proper state management.
- Stacks: Flutter/Dart, GoLang, Spring Boot, System Design, REST APIs, PostgreSQL.
- Apps: Deployed production apps from scratch with 100k+ downloads.

REQUIRED EMAIL STRUCTURE:
1. Greeting: "Hello," or "Hello <FirstName>," (if recruiter name is known from post).
2. Direct intro sentence tailored to the role ("%s"):
   - If Flutter / Mobile: "I'm Pratyush Mehra, a Flutter app dev with 2+ years of experience in building and deploying responsive apps using REST APIs, clean code, and proper state management."
   - If Go / Golang / Backend: "I'm Pratyush Mehra, a Go / Backend developer with 2+ years of experience in building and deploying high-throughput services using REST APIs, clean code, and proper system architecture."
   - If React / Frontend / Full Stack: "I'm Pratyush Mehra, a Full Stack / Frontend developer with 2+ years of experience in building and deploying responsive applications using REST APIs, clean code, and proper state management."
3. 3-4 authentic bullet points tailored to %s and the job requirements:
   • Focus on clean UI, scalable architecture, and writing production ready code (tailor slightly to match their stack or architecture)
   • Worked on large userbase apps with over 100k+ downloads and deployed apps from scratch (mention relevance to their product/domain if applicable)
   • Hands-on knowledge across different tech stacks including Flutter, GoLang, Spring Boot, and system design concepts
   • A customized closing bullet expressing genuine interest in what %s is building and why he is the practical, reliable engineer they need
4. Closing & Exact Signature:
Thanks for your time.
– Pratyush Mehra
8005331766
pratyushwho.xyz
GitHub: https://github.com/Pratyush-who
LinkedIn: https://www.linkedin.com/in/pratyushmehra22/
Resume-- dub.sh/pratyush

RULES:
- %s
- %s
- %s
- NO markdown code fences (no `+"`"+` blocks), plain text only.
- NO corporate filler like "I hope this email finds you well" or "Attached please find my resume".
- Make the email feel genuinely written for %s, not a copy-pasted generic template.

Return JSON with "subject" and "body".`,
			company, role, role, company, post.WorkMode, post.Location,
			post.Text, companySummary, role, company, company,
			locationRule, requiredSubjectNotice, similarityWarning, company,
		)

		var res ComposedEmail
		if err := c.llmClient.GenerateJSON(ctx, prompt, schema, &res); err != nil {
			c.logger.Warn("llm generation failed, falling back to clean template draft", "error", err)
			return c.fallbackDraft(profile, post), nil
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

func (c *Composer) fallbackDraft(profile model.Profile, post model.Post) *ComposedEmail {
	role := strings.TrimSpace(post.Role)
	if role == "" {
		role = "Flutter Developer"
	}
	company := strings.TrimSpace(post.Company)
	if company == "" {
		company = "your team"
	}

	subject := fmt.Sprintf("%s Application - Pratyush Mehra", role)
	if company != "" && company != "your team" {
		subject = fmt.Sprintf("%s Application - %s - Pratyush Mehra", role, company)
	}
	if strings.TrimSpace(post.Extracted.RequiredSubjectLine) != "" {
		subject = strings.TrimSpace(post.Extracted.RequiredSubjectLine)
	}

	author := strings.TrimSpace(post.Author)
	greeting := "Hello"
	if author != "" && !strings.Contains(author, "@") {
		firstName := strings.Fields(author)[0]
		greeting = fmt.Sprintf("Hello %s", firstName)
	}

	lowerRole := strings.ToLower(role)
	lowerPost := strings.ToLower(post.Text)

	introLine := "I'm Pratyush Mehra, a Flutter app dev with 2+ years of experience in building and deploying responsive apps using REST APIs, clean code, and proper state management."
	stackBullet := "• With my experience and hands on knowledge different tech stack, like Spring boot and GoLang along with knowledge of system design concepts."

	if strings.Contains(lowerRole, "golang") || strings.Contains(lowerRole, "go ") || strings.Contains(lowerRole, "backend") || strings.Contains(lowerPost, "golang") {
		introLine = "I'm Pratyush Mehra, a Go / Backend developer with 2+ years of experience in building and deploying high-performance services using REST APIs, clean code, and scalable architecture."
		stackBullet = "• With my experience and hands on knowledge different tech stack, like GoLang, Spring boot and Flutter along with knowledge of system design concepts."
	} else if strings.Contains(lowerRole, "react") || strings.Contains(lowerRole, "frontend") || strings.Contains(lowerRole, "web") {
		introLine = "I'm Pratyush Mehra, a Full Stack / Frontend developer with 2+ years of experience in building and deploying responsive web & mobile apps using REST APIs, clean code, and modern state management."
		stackBullet = "• With my experience and hands on knowledge different tech stack, like React, GoLang and Flutter along with knowledge of system design concepts."
	}

	companyTarget := company
	if companyTarget == "your team" || companyTarget == "" {
		companyTarget = "your team"
	}

	body := fmt.Sprintf(
		`%s,
%s

• Focus on clean UI, scalable architecture, and writing production ready code
• Worked on large userbase apps with over 100k+ downloads and deployed apps from scratch
%s
• I keep exploring stacks and technologies, and I'm really excited about what %s is building. Basically the versatile tech guy you need to move fast and ship reliable features.

Thanks for your time.
– Pratyush Mehra
8005331766
pratyushwho.xyz
GitHub: https://github.com/Pratyush-who
LinkedIn: https://www.linkedin.com/in/pratyushmehra22/
Resume-- dub.sh/pratyush`,
		greeting, introLine, stackBullet, companyTarget,
	)

	return &ComposedEmail{
		Subject: subject,
		Body:    body,
	}
}
