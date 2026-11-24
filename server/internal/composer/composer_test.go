package composer

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"emailauto-server/internal/llm"
	"emailauto-server/internal/model"
)

func TestComposer_ComposeBasic(t *testing.T) {
	ctx := context.Background()

	llmResp := ComposedEmail{
		Subject: "Go Backend Developer - Pratyush",
		Body:    "Hi Sarah,\n\nI noticed your opening for a Go Backend Developer at TechCorp. Having built high-throughput microservices using Go and PostgreSQL, my background aligns closely with your roadmap.\n\nI am fully available to work remotely. Attached is my resume for your review. Would you be open to a brief chat this week?\n\nBest regards,\nPratyush",
	}
	rawResp, _ := json.Marshal(llmResp)
	fakeLLM := llm.NewFakeClient(string(rawResp))

	comp := NewComposer(fakeLLM, nil)

	profile := model.Profile{
		Summary:    "Backend engineer with Go and Postgres experience",
		Skills:     "Go, Postgres, Docker",
		ResumeText: "Pratyush - 3 years building backend services in Go",
	}

	post := model.Post{
		Company:  "TechCorp",
		Role:     "Go Backend Developer",
		WorkMode: "remote",
		Location: "Remote",
		Text:     "Looking for a Go developer to join our backend team.",
	}

	res, err := comp.Compose(ctx, profile, post, "TechCorp is a cloud tooling startup.", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Subject != "Go Backend Developer - Pratyush" {
		t.Errorf("unexpected subject: %s", res.Subject)
	}
	if !strings.Contains(res.Body, "resume") {
		t.Errorf("expected resume mention in body: %s", res.Body)
	}
}

func TestComposer_SimilarityRetry(t *testing.T) {
	ctx := context.Background()

	existingToday := []string{
		"Hi Team, I am applying for the React role at Acme. With 3 years in React, I would love to contribute.",
	}

	// 1st response: near identical to existingToday
	resp1 := ComposedEmail{
		Subject: "React Application",
		Body:    "Hi Team, I am applying for the React role at Acme. With 3 years in React, I would love to contribute.",
	}
	// 2nd response: varied and distinct
	resp2 := ComposedEmail{
		Subject: "Application: React Engineer - Acme",
		Body:    "Hello Hiring Manager,\n\nAcme's modern frontend challenges align directly with my React expertise. My resume is attached for review.\n\nBest,\nPratyush",
	}

	raw1, _ := json.Marshal(resp1)
	raw2, _ := json.Marshal(resp2)
	fakeLLM := llm.NewFakeClient(string(raw1), string(raw2))

	comp := NewComposer(fakeLLM, nil)

	profile := model.Profile{
		Summary: "Frontend Developer in React",
	}
	post := model.Post{
		Company:  "Acme",
		Role:     "React Developer",
		WorkMode: "onsite",
	}

	res, err := comp.Compose(ctx, profile, post, "Acme provides enterprise tools.", existingToday)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have used the second response after retrying
	if res.Subject != "Application: React Engineer - Acme" {
		t.Errorf("expected second response after similarity retry, got: %s", res.Subject)
	}
	if fakeLLM.CallCount() != 2 {
		t.Errorf("expected 2 LLM calls due to similarity retry, got %d", fakeLLM.CallCount())
	}
}
