package pipeline

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"emailauto-server/internal/llm"
)

type mockDNSChecker struct {
	hasMX  bool
	hasDNS bool
	err    error
}

func (m *mockDNSChecker) CheckMX(ctx context.Context, domain string) (bool, bool, error) {
	return m.hasMX, m.hasDNS, m.err
}

type mockWebFetcher struct {
	content string
	err     error
}

func (m *mockWebFetcher) Fetch(ctx context.Context, urlStr string) (string, error) {
	return m.content, m.err
}

func TestIsPersonalDomain(t *testing.T) {
	personal := []string{"gmail.com", "yahoo.com", "HOTMAIL.COM", "outlook.com", "proton.me"}
	for _, d := range personal {
		if !IsPersonalDomain(d) {
			t.Errorf("expected %s to be personal domain", d)
		}
	}

	corporate := []string{"google.com", "stripe.com", "uber.com", "microsoft.com"}
	for _, d := range corporate {
		if IsPersonalDomain(d) {
			t.Errorf("expected %s to NOT be personal domain", d)
		}
	}
}

func TestCalculateTrustScore(t *testing.T) {
	v := &CompanyVerifier{}

	// Case 1: Ideal corporate company with MX, website, valid product
	score, reasons := v.calculateTrustScore(true, true, false, true, "valid", nil)
	if score < 90 {
		t.Errorf("expected high score for legitimate corporate company, got %d (reasons: %v)", score, reasons)
	}

	// Case 2: Personal domain with MX (e.g. gmail)
	scorePersonal, _ := v.calculateTrustScore(true, true, true, false, "valid", nil)
	if scorePersonal >= score {
		t.Errorf("personal email should score lower than corporate domain, got %d vs %d", scorePersonal, score)
	}

	// Case 3: Red flags detected
	scoreRedFlags, reasonsRed := v.calculateTrustScore(true, true, false, true, "suspicious", []string{"pay-to-apply fee"})
	if scoreRedFlags >= 50 {
		t.Errorf("red flags should drop score significantly, got %d", scoreRedFlags)
	}
	foundRedFlagReason := false
	for _, r := range reasonsRed {
		if strings.Contains(r, "pay-to-apply fee") {
			foundRedFlagReason = true
			break
		}
	}
	if !foundRedFlagReason {
		t.Errorf("expected red flag in reasons, got %v", reasonsRed)
	}

	// Case 4: No DNS and no MX
	scoreNoDNS, _ := v.calculateTrustScore(false, false, false, false, "unclear", nil)
	if scoreNoDNS > 20 {
		t.Errorf("expected very low score for non-existent domain, got %d", scoreNoDNS)
	}
}

func TestVerifyCompanyWithCache(t *testing.T) {
	ctx := context.Background()
	cache := NewDomainCache(nil)

	// Pre-populate cache with a 10-day old verification
	cache.Put(ctx, DomainCacheEntry{
		Domain:         "acme.com",
		TrustScore:     85,
		TrustReasons:   []string{"Corporate company domain verified"},
		CompanySummary: "Acme builds anvil technologies.",
		HasMX:          true,
		IsPersonal:     false,
		VerifiedAt:     time.Now().UTC().Add(-10 * 24 * time.Hour),
	})

	dnsMock := &mockDNSChecker{hasMX: true, hasDNS: true}
	webMock := &mockWebFetcher{content: "About Acme Corp"}
	verifier := NewCompanyVerifier(nil, cache, dnsMock, webMock, nil)

	res, err := verifier.Verify(ctx, "Acme", "acme.com", "We are hiring!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.TrustScore != 85 {
		t.Errorf("expected cached trust score 85, got %d", res.TrustScore)
	}
	if !strings.Contains(res.TrustReasons[0], "cached") {
		t.Errorf("expected cached indicator in reasons, got %v", res.TrustReasons)
	}
}

func TestVerifyCompanyWithFakeLLM(t *testing.T) {
	ctx := context.Background()

	llmResponse := CompanyVerificationResult{
		Summary:        "TechCorp builds cloud infrastructure tooling.",
		HasRedFlags:    false,
		RedFlags:       []string{},
		ProductVerdict: "valid",
	}
	rawResp, _ := json.Marshal(llmResponse)

	fakeLLM := llm.NewFakeClient(string(rawResp))

	dnsMock := &mockDNSChecker{hasMX: true, hasDNS: true}
	webMock := &mockWebFetcher{content: "TechCorp cloud platform"}
	cache := NewDomainCache(nil)

	verifier := NewCompanyVerifier(fakeLLM, cache, dnsMock, webMock, nil)

	res, err := verifier.Verify(ctx, "TechCorp", "techcorp.io", "We are looking for a Senior Go Engineer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.TrustScore < 70 {
		t.Errorf("expected trust score >= 70, got %d", res.TrustScore)
	}
	if res.CompanySummary != "TechCorp builds cloud infrastructure tooling." {
		t.Errorf("unexpected summary: %s", res.CompanySummary)
	}

	// Verify it was cached
	cachedEntry, ok := cache.Get(ctx, "techcorp.io")
	if !ok {
		t.Fatalf("expected techcorp.io to be cached")
	}
	if cachedEntry.TrustScore != res.TrustScore {
		t.Errorf("cached score %d != res score %d", cachedEntry.TrustScore, res.TrustScore)
	}
}
