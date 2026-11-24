package pipeline

import (
	"context"
	"testing"

	"emailauto-server/internal/llm"
)

func TestExtractEmailsWithRegex(t *testing.T) {
	text := `We are hiring! Reach out at jobs@startup.io or info@startup.io. You can also cc founder@startup.io.`
	emails := ExtractEmailsWithRegex(text)

	if len(emails) != 2 {
		t.Fatalf("expected 2 emails (info filtered out), got %d: %+v", len(emails), emails)
	}

	for _, e := range emails {
		if e == "info@startup.io" {
			t.Errorf("info@ address should have been filtered out since better addresses exist")
		}
	}
}

func TestFilterAndDeduplicateEmails_KeepOnlyOne(t *testing.T) {
	// If info@ is the only address, it should be kept
	single := []string{"info@company.com"}
	res := FilterAndDeduplicateEmails(single)
	if len(res) != 1 || res[0] != "info@company.com" {
		t.Errorf("expected info@ to be kept when it is the only address, got %+v", res)
	}

	// Multiple with noreply and valid address
	multi := []string{"noreply@company.com", "careers@company.com", "CAREERS@company.com"}
	res2 := FilterAndDeduplicateEmails(multi)
	if len(res2) != 1 || res2[0] != "careers@company.com" {
		t.Errorf("expected noreply filtered and duplicates collapsed, got %+v", res2)
	}
}

func TestPostExtractor_ExtractPost(t *testing.T) {
	fakeLLM := llm.NewFakeClient(`{
		"is_hiring_post": true,
		"emails": ["recruiting@supralabs.io"],
		"company": "SupraLabs",
		"role": "Go Systems Architect",
		"work_mode": "remote",
		"location": "Remote",
		"experience_required": "3+ years",
		"experience_years": 3.0,
		"tech_stack": ["golang", "docker", "k8s"],
		"stipend_text": "₹45,000/month",
		"stipend_amount": 45000,
		"paid_status": "paid",
		"apply_instructions": "Send resume with subject 'Application: Go Systems'",
		"required_subject_line": "Application: Go Systems",
		"apply_via_email": true
	}`)

	extractor := NewPostExtractor(fakeLLM)
	postText := `Hiring Go Systems Architect at SupraLabs. Send resume to recruiting@supralabs.io with subject 'Application: Go Systems'.`

	res, err := extractor.ExtractPost(context.Background(), postText)
	if err != nil {
		t.Fatalf("ExtractPost failed: %v", err)
	}

	if !res.IsHiringPost {
		t.Errorf("expected is_hiring_post to be true")
	}
	if res.Company != "SupraLabs" {
		t.Errorf("unexpected company: %s", res.Company)
	}
	if res.RequiredSubjectLine != "Application: Go Systems" {
		t.Errorf("unexpected required subject line: %s", res.RequiredSubjectLine)
	}
	if res.PaidStatus != "paid" {
		t.Errorf("expected paid_status 'paid', got %s", res.PaidStatus)
	}
}

func TestPostExtractor_ForceUnpaidOnExposure(t *testing.T) {
	fakeLLM := llm.NewFakeClient(`{
		"is_hiring_post": true,
		"emails": ["interns@unpaid.org"],
		"company": "Startup",
		"role": "Intern",
		"work_mode": "remote",
		"paid_status": "paid",
		"apply_via_email": true
	}`)

	extractor := NewPostExtractor(fakeLLM)
	postText := `Looking for energetic interns for exposure and valuable experience. Contact interns@unpaid.org.`

	res, err := extractor.ExtractPost(context.Background(), postText)
	if err != nil {
		t.Fatalf("ExtractPost failed: %v", err)
	}

	if res.PaidStatus != "unpaid" {
		t.Errorf("expected post mentioning 'exposure' to be forced to 'unpaid', got %s", res.PaidStatus)
	}
}
