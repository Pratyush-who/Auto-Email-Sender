package composer

import "testing"

func TestSimilarity_IdenticalText(t *testing.T) {
	text := "Hi Alex, I am writing to express my interest in the Full Stack Go developer role."
	sim := CalculateSimilarity(text, text)
	if sim < 0.99 {
		t.Errorf("expected identical texts to have similarity ~1.0, got %f", sim)
	}
}

func TestSimilarity_DifferentText(t *testing.T) {
	textA := "Hi Alex, I am writing to express my interest in the Full Stack Go developer role at SupraBase."
	textB := "Dear recruiter, please find attached my resume for the Machine Learning position in Tokyo."

	sim := CalculateSimilarity(textA, textB)
	if sim > 0.20 {
		t.Errorf("expected different texts to have low similarity, got %f", sim)
	}
}

func TestSimilarity_NearIdenticalDetection(t *testing.T) {
	existing := []string{
		"Hi Alex,\n\nI noticed your opening for a Go Developer at TechCorp. I have extensive experience building scalable microservices in Go and Postgres.\n\nAttached is my resume. Let me know if you are open to chat.\n\nBest,\nPratyush",
	}

	// Minor change: swapped word "scalable" -> "high-throughput"
	candidateNear := "Hi Alex,\n\nI noticed your opening for a Go Developer at TechCorp. I have extensive experience building high-throughput microservices in Go and Postgres.\n\nAttached is my resume. Let me know if you are open to chat.\n\nBest,\nPratyush"

	near, score := IsNearIdentical(candidateNear, existing, 0.70)
	if !near {
		t.Errorf("expected candidate to be flagged as near identical, got false with score %f", score)
	}

	// Completely distinct candidate
	candidateDistinct := "Hello Alex,\n\nTechCorp's cloud observability platform caught my attention. Having built automated monitoring pipelines in React and Go, I would love to contribute to your engineering goals.\n\nMy resume is attached for details. Would next Tuesday work for a quick intro call?\n\nWarm regards,\nPratyush"

	nearDistinct, scoreDistinct := IsNearIdentical(candidateDistinct, existing, 0.70)
	if nearDistinct {
		t.Errorf("expected distinct email to not be flagged, got true with score %f", scoreDistinct)
	}
}
