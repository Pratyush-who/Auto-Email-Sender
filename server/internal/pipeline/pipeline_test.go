package pipeline

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"emailauto-server/internal/llm"
	"emailauto-server/internal/model"
)

type memoryPostStore struct {
	posts map[string]*model.Post
}

func (m *memoryPostStore) SavePost(ctx context.Context, p *model.Post) (*model.Post, error) {
	if m.posts == nil {
		m.posts = make(map[string]*model.Post)
	}
	if p.ID == "" {
		p.ID = "test-post-uuid"
	}
	cp := *p
	m.posts[p.ID] = &cp
	return &cp, nil
}

type memoryContactStore struct {
	contacts map[string]*model.Contact
}

func (m *memoryContactStore) SaveContact(ctx context.Context, c *model.Contact) (*model.Contact, error) {
	if m.contacts == nil {
		m.contacts = make(map[string]*model.Contact)
	}
	if c.ID == "" {
		c.ID = "test-contact-uuid"
	}
	cp := *c
	m.contacts[c.Email] = &cp
	return &cp, nil
}

func TestPipeline_PostPassed(t *testing.T) {
	ctx := context.Background()

	// 1. LLM client responds to extraction and verification
	fakeLLM := &multiResponseFakeClient{
		responses: map[string]string{
			"hiring": `{
				"is_hiring_post": true,
				"emails": ["jobs@techcorp.io"],
				"company": "TechCorp",
				"role": "Go Backend Developer",
				"work_mode": "remote",
				"location": "Remote",
				"paid_status": "paid",
				"apply_via_email": true
			}`,
			"due-diligence": `{
				"summary": "TechCorp is an enterprise cloud software company.\nThey build developer observability platforms.",
				"has_red_flags": false,
				"red_flags": [],
				"product_verdict": "valid"
			}`,
		},
	}

	extractor := NewPostExtractor(fakeLLM)
	verifier := NewCompanyVerifier(
		fakeLLM,
		NewDomainCache(nil),
		&mockDNSChecker{hasMX: true, hasDNS: true},
		&mockWebFetcher{content: "TechCorp developer platform"},
		nil,
	)

	postStore := &memoryPostStore{}
	contactStore := &memoryContactStore{}

	p := NewPipeline(extractor, verifier, postStore, contactStore, nil)

	post := &model.Post{
		URL:  "https://linkedin.com/feed/update/urn:li:activity:12345",
		Text: "We are hiring a Go Backend Developer at TechCorp! Send resume to jobs@techcorp.io",
	}

	settings := model.DefaultSettings()
	res, err := p.ProcessPost(ctx, post, settings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.Passed {
		t.Fatalf("expected post to pass, got skipped with reason: %s", res.Reason)
	}

	if res.Post.Status != "passed" {
		t.Errorf("expected status 'passed', got '%s'", res.Post.Status)
	}
	if res.Post.Company != "TechCorp" {
		t.Errorf("expected company TechCorp, got '%s'", res.Post.Company)
	}
	if res.Contact == nil {
		t.Fatalf("expected contact to be created")
	}
	if res.Contact.Email != "jobs@techcorp.io" {
		t.Errorf("expected email jobs@techcorp.io, got '%s'", res.Contact.Email)
	}
	if res.Contact.TrustScore < 70 {
		t.Errorf("expected trust score >= 70, got %.0f", res.Contact.TrustScore)
	}
	if res.Post.Extracted.CompanySummary == "" {
		t.Errorf("expected company summary to be saved in post extracted")
	}
}

func TestPipeline_PostSkipped_NotHiring(t *testing.T) {
	ctx := context.Background()

	fakeLLM := llm.NewFakeClient(`{
			"is_hiring_post": false,
			"emails": [],
			"company": "TechCorp",
			"role": "",
			"work_mode": "unknown",
			"paid_status": "unknown",
			"apply_via_email": false
		}`)

	extractor := NewPostExtractor(fakeLLM)
	verifier := NewCompanyVerifier(fakeLLM, NewDomainCache(nil), &mockDNSChecker{}, &mockWebFetcher{}, nil)
	p := NewPipeline(extractor, verifier, &memoryPostStore{}, &memoryContactStore{}, nil)

	post := &model.Post{
		URL:  "https://linkedin.com/feed/update/urn:li:activity:67890",
		Text: "Proud to announce our series B funding today!",
	}

	res, err := p.ProcessPost(ctx, post, model.DefaultSettings())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Passed {
		t.Fatalf("expected post to be skipped")
	}
	if !strings.Contains(res.Reason, "not a hiring") {
		t.Errorf("unexpected reason: %s", res.Reason)
	}
	if res.Post.Status != "skipped" {
		t.Errorf("expected status skipped, got %s", res.Post.Status)
	}
}

func TestPipeline_PostSkipped_LowTrustScore(t *testing.T) {
	ctx := context.Background()

	fakeLLM := &multiResponseFakeClient{
		responses: map[string]string{
			"hiring": `{
				"is_hiring_post": true,
				"emails": ["recruiter@shadybiz.xyz"],
				"company": "ShadyBiz",
				"role": "Data Entry Specialist",
				"work_mode": "remote",
				"paid_status": "paid",
				"apply_via_email": true
			}`,
			"due-diligence": `{
				"summary": "ShadyBiz offers work from home typing tasks.",
				"has_red_flags": true,
				"red_flags": ["pay-to-apply fee for registration", "vague business model"],
				"product_verdict": "suspicious"
			}`,
		},
	}

	extractor := NewPostExtractor(fakeLLM)
	// Shady domain with no MX
	verifier := NewCompanyVerifier(
		fakeLLM,
		NewDomainCache(nil),
		&mockDNSChecker{hasMX: false, hasDNS: true},
		&mockWebFetcher{content: "Register here and pay $50 processing fee"},
		nil,
	)

	postStore := &memoryPostStore{}
	contactStore := &memoryContactStore{}
	p := NewPipeline(extractor, verifier, postStore, contactStore, nil)

	post := &model.Post{
		URL:  "https://linkedin.com/feed/update/urn:li:activity:99999",
		Text: "Earn $500/day from home! Email recruiter@shadybiz.xyz to apply now.",
	}

	settings := model.DefaultSettings() // trust threshold = 40
	res, err := p.ProcessPost(ctx, post, settings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Passed {
		t.Fatalf("expected shady post to be skipped due to low trust score or spam flag")
	}
	if !strings.Contains(res.Reason, "trust score") && !strings.Contains(res.Reason, "SPAM") && !strings.Contains(res.Reason, "SCAM") {
		t.Errorf("expected trust score or spam flag skip reason, got: %s", res.Reason)
	}
}

// multiResponseFakeClient dispatches responses based on prompt keywords
type multiResponseFakeClient struct {
	responses map[string]string
}

func (m *multiResponseFakeClient) GenerateJSON(ctx context.Context, prompt string, schema any, out any) error {
	lowerPrompt := strings.ToLower(prompt)
	if strings.Contains(lowerPrompt, "due-diligence") {
		if resp, ok := m.responses["due-diligence"]; ok {
			return json.Unmarshal([]byte(resp), out)
		}
	}
	if strings.Contains(lowerPrompt, "recruiter and job scraper") || strings.Contains(lowerPrompt, "extract structured data") {
		if resp, ok := m.responses["hiring"]; ok {
			return json.Unmarshal([]byte(resp), out)
		}
	}
	for _, resp := range m.responses {
		return json.Unmarshal([]byte(resp), out)
	}
	return nil
}
