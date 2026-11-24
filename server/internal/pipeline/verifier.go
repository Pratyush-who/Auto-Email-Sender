package pipeline

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"emailauto-server/internal/llm"
)

// Known personal/freemail domains
var PersonalEmailDomains = map[string]bool{
	"gmail.com":      true,
	"googlemail.com": true,
	"yahoo.com":      true,
	"ymail.com":      true,
	"hotmail.com":    true,
	"outlook.com":    true,
	"live.com":       true,
	"msn.com":        true,
	"icloud.com":     true,
	"me.com":         true,
	"mac.com":        true,
	"proton.me":      true,
	"protonmail.com": true,
	"zoho.com":       true,
	"aol.com":        true,
	"mail.com":       true,
	"gmx.com":        true,
	"fastmail.com":   true,
	"yandex.com":     true,
	"rediffmail.com": true,
}

// DNSChecker defines DNS & MX inspection capabilities.
type DNSChecker interface {
	CheckMX(ctx context.Context, domain string) (hasMX bool, hasDNS bool, err error)
}

// WebFetcher defines web page retrieval capabilities.
type WebFetcher interface {
	Fetch(ctx context.Context, urlStr string) (string, error)
}

// DefaultDNSChecker performs live DNS and MX lookups using the Go net package.
type DefaultDNSChecker struct{}

func (d *DefaultDNSChecker) CheckMX(ctx context.Context, domain string) (bool, bool, error) {
	resolver := net.DefaultResolver

	// Check MX records first
	mxRecords, err := resolver.LookupMX(ctx, domain)
	if err == nil && len(mxRecords) > 0 {
		return true, true, nil
	}

	// Fallback to checking if the host resolves to any IP (A/AAAA)
	ips, ipErr := resolver.LookupIP(ctx, "ip", domain)
	if ipErr == nil && len(ips) > 0 {
		return false, true, nil
	}

	return false, false, err
}

// DefaultWebFetcher fetches web pages with small byte limit, robots-friendly User-Agent, and short timeouts.
type DefaultWebFetcher struct {
	client *http.Client
}

func NewDefaultWebFetcher(timeout time.Duration) *DefaultWebFetcher {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &DefaultWebFetcher{
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("stopped after 5 redirects")
				}
				return nil
			},
		},
	}
}

var (
	scriptTagRegex = regexp.MustCompile(`(?is)<script.*?</script>`)
	styleTagRegex  = regexp.MustCompile(`(?is)<style.*?</style>`)
	htmlTagRegex   = regexp.MustCompile(`(?is)<.*?>`)
	spacesRegex    = regexp.MustCompile(`\s+`)
)

func (w *DefaultWebFetcher) Fetch(ctx context.Context, urlStr string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; JobBot/1.0; +https://github.com/emailauto)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9")

	resp, err := w.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("http status %d", resp.StatusCode)
	}

	// Limit response reading to 100KB to prevent memory exhaustion
	limitReader := io.LimitReader(resp.Body, 100*1024)
	bodyBytes, err := io.ReadAll(limitReader)
	if err != nil {
		return "", err
	}

	return cleanHTMLToText(string(bodyBytes)), nil
}

func cleanHTMLToText(html string) string {
	text := scriptTagRegex.ReplaceAllString(html, " ")
	text = styleTagRegex.ReplaceAllString(text, " ")
	text = htmlTagRegex.ReplaceAllString(text, " ")
	text = spacesRegex.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)
	if len(text) > 2500 {
		text = text[:2500]
	}
	return text
}

// CompanyVerifier handles verification of company and email domains.
type CompanyVerifier struct {
	llmClient  llm.Client
	cache      *DomainCache
	dnsChecker DNSChecker
	webFetcher WebFetcher
	logger     *slog.Logger
}

// NewCompanyVerifier creates a new CompanyVerifier.
func NewCompanyVerifier(
	llmClient llm.Client,
	cache *DomainCache,
	dnsChecker DNSChecker,
	webFetcher WebFetcher,
	logger *slog.Logger,
) *CompanyVerifier {
	if dnsChecker == nil {
		dnsChecker = &DefaultDNSChecker{}
	}
	if webFetcher == nil {
		webFetcher = NewDefaultWebFetcher(5 * time.Second)
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &CompanyVerifier{
		llmClient:  llmClient,
		cache:      cache,
		dnsChecker: dnsChecker,
		webFetcher: webFetcher,
		logger:     logger,
	}
}

// IsPersonalDomain checks if the domain belongs to a free/consumer email provider.
func IsPersonalDomain(domain string) bool {
	cleaned := strings.ToLower(strings.TrimSpace(domain))
	return PersonalEmailDomains[cleaned]
}

// Verify evaluates the company and email domain, producing trust score, reasons, and 2-line summary.
func (v *CompanyVerifier) Verify(ctx context.Context, company, domain, postText string) (*VerificationResult, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = strings.TrimPrefix(domain, "@")
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimRight(domain, "/")

	if domain == "" {
		return &VerificationResult{
			Domain:         "",
			TrustScore:     0,
			TrustReasons:   []string{"Missing or invalid email domain"},
			CompanySummary: "Unknown company",
			HasMX:          false,
			IsPersonal:     false,
		}, nil
	}

	// 1. Check 30-day domain cache
	if v.cache != nil {
		if cached, ok := v.cache.Get(ctx, domain); ok {
			v.logger.Info("using cached domain verification", "domain", domain, "score", cached.TrustScore)
			return &VerificationResult{
				Domain:         cached.Domain,
				TrustScore:     cached.TrustScore,
				TrustReasons:   append([]string{"(30-day cached result)"}, cached.TrustReasons...),
				CompanySummary: cached.CompanySummary,
				RedFlags:       cached.RedFlags,
				HasMX:          cached.HasMX,
				IsPersonal:     cached.IsPersonal,
			}, nil
		}
	}

	isPersonal := IsPersonalDomain(domain)

	// 2. DNS & MX Check
	hasMX, hasDNS, _ := v.dnsChecker.CheckMX(ctx, domain)

	// 3. Website fetching (skipped for consumer/personal emails like gmail/yahoo)
	var siteContent string
	var websiteAccessible bool
	if !isPersonal {
		homeURL := "https://" + domain
		content, err := v.webFetcher.Fetch(ctx, homeURL)
		if err == nil && len(content) > 50 {
			siteContent = content
			websiteAccessible = true
		} else {
			// Fallback to http or /about
			aboutURL := "https://" + domain + "/about"
			aboutContent, err := v.webFetcher.Fetch(ctx, aboutURL)
			if err == nil && len(aboutContent) > 50 {
				siteContent = aboutContent
				websiteAccessible = true
			} else {
				httpURL := "http://" + domain
				httpContent, err := v.webFetcher.Fetch(ctx, httpURL)
				if err == nil && len(httpContent) > 50 {
					siteContent = httpContent
					websiteAccessible = true
				}
			}
		}
	}

	// 4. Gemini analysis: 2-line company summary and red-flag detection
	summary, redFlags, productVerdict, err := v.analyzeCompanyWithLLM(ctx, company, domain, postText, siteContent, isPersonal)
	if err != nil {
		v.logger.Warn("llm company verification fallback due to error", "error", err)
		summary = fmt.Sprintf("%s is a company operating in its respective industry.", company)
		productVerdict = "unclear"
	}

	// 5. Compute trust score (0-100) and compile trust reasons
	score, reasons := v.calculateTrustScore(hasMX, hasDNS, isPersonal, websiteAccessible, productVerdict, redFlags)

	result := &VerificationResult{
		Domain:         domain,
		TrustScore:     score,
		TrustReasons:   reasons,
		CompanySummary: summary,
		RedFlags:       redFlags,
		HasMX:          hasMX,
		IsPersonal:     isPersonal,
	}

	// 6. Cache result for 30 days
	if v.cache != nil {
		v.cache.Put(ctx, DomainCacheEntry{
			Domain:         domain,
			TrustScore:     score,
			TrustReasons:   reasons,
			CompanySummary: summary,
			RedFlags:       redFlags,
			HasMX:          hasMX,
			IsPersonal:     isPersonal,
			VerifiedAt:     time.Now().UTC(),
		})
	}

	return result, nil
}

func (v *CompanyVerifier) analyzeCompanyWithLLM(
	ctx context.Context,
	company string,
	domain string,
	postText string,
	siteContent string,
	isPersonal bool,
) (string, []string, string, error) {
	if v.llmClient == nil {
		return fmt.Sprintf("%s operates at %s.", company, domain), nil, "unclear", nil
	}

	prompt := fmt.Sprintf(`You are a due-diligence expert evaluating a company recruiting online.
Evaluate the company, summarize its core business in exactly 2 lines, and detect any recruitment or business red flags.

Company Name: %s
Domain: %s (Personal free email: %v)

Post context:
"""
%s
"""

Website / About content:
"""
%s
"""

RED FLAGS TO DETECT:
- "pay-to-apply" or fees for training, application, equipment, or assessment
- "vague" or "no real product" (cannot determine what product/service the company actually offers)
- MLM / network marketing / pyramid schemes
- Phishing / suspicious recruitment scam / unverified identity
- Unpaid labor disguised as an internship or trial task

Return JSON conforming strictly to the schema.`, company, domain, isPersonal, postText, siteContent)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary": map[string]any{
				"type":        "string",
				"description": "Exactly 2 lines summarizing what the company does and its primary products or services",
			},
			"has_red_flags": map[string]any{"type": "boolean"},
			"red_flags": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"product_verdict": map[string]any{
				"type": "string",
				"enum": []string{"valid", "suspicious", "unclear"},
			},
		},
		"required": []string{"summary", "has_red_flags", "red_flags", "product_verdict"},
	}

	var res CompanyVerificationResult
	if err := v.llmClient.GenerateJSON(ctx, prompt, schema, &res); err != nil {
		return "", nil, "", err
	}

	return res.Summary, res.RedFlags, res.ProductVerdict, nil
}

func (v *CompanyVerifier) calculateTrustScore(
	hasMX bool,
	hasDNS bool,
	isPersonal bool,
	websiteAccessible bool,
	productVerdict string,
	redFlags []string,
) (int, []string) {
	score := 50
	var reasons []string

	// MX & DNS
	if hasMX {
		score += 20
		reasons = append(reasons, "Valid MX mail exchange records active")
	} else if hasDNS {
		score -= 20
		reasons = append(reasons, "Domain exists but has no MX mail records configured")
	} else {
		score -= 40
		reasons = append(reasons, "Domain has no valid DNS or MX records")
	}

	// Email domain ownership
	if isPersonal {
		score -= 25
		reasons = append(reasons, "Recruiter uses personal/consumer email provider")
	} else if hasDNS || hasMX {
		score += 15
		reasons = append(reasons, "Corporate company domain verified")
	} else {
		score -= 10
		reasons = append(reasons, "Domain failed DNS resolution")
	}

	// Website accessibility
	if websiteAccessible {
		score += 15
		reasons = append(reasons, "Company website/about page is active and reachable")
	} else if !isPersonal {
		score -= 10
		reasons = append(reasons, "Company website was unreachable or returned empty")
	}

	// Product verdict
	switch productVerdict {
	case "valid":
		score += 10
		reasons = append(reasons, "Legitimate product and business model verified")
	case "suspicious":
		score -= 30
		reasons = append(reasons, "Suspicious product or business model detected")
	case "unclear":
		// Neutral
	}

	// Red flags
	if len(redFlags) > 0 {
		score -= (30 + len(redFlags)*10)
		for _, rf := range redFlags {
			reasons = append(reasons, fmt.Sprintf("Red flag detected: %s", rf))
		}
	}

	// Clamp between 0 and 100
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}

	return score, reasons
}
