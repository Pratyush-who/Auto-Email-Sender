package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type testResponse struct {
	Greeting string `json:"greeting"`
	Target   string `json:"target"`
}

func mockGeminiServerResponse(text string, promptTokens, candidateTokens int) string {
	resp := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiCandidateContent{
					Parts: []geminiPart{
						{Text: text},
					},
					Role: "model",
				},
				FinishReason: "STOP",
			},
		},
		UsageMetadata: geminiUsageMetadata{
			PromptTokenCount:     promptTokens,
			CandidatesTokenCount: candidateTokens,
			TotalTokenCount:      promptTokens + candidateTokens,
		},
	}
	bytes, _ := json.Marshal(resp)
	return string(bytes)
}

func TestGeminiClient_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockGeminiServerResponse(`{"greeting": "hello", "target": "world"}`, 15, 20)))
	}))
	defer server.Close()

	client := NewGeminiClient(GeminiConfig{
		APIKey:      "fake-key",
		BaseURL:     server.URL,
		RPM:         600,
		BaseBackoff: 10 * time.Millisecond,
	})

	var out testResponse
	err := client.GenerateJSON(context.Background(), "say hello", map[string]any{"type": "object"}, &out)
	if err != nil {
		t.Fatalf("GenerateJSON failed: %v", err)
	}

	if out.Greeting != "hello" || out.Target != "world" {
		t.Errorf("unexpected result: %+v", out)
	}
}

func TestGeminiClient_MarkdownFenceCleaning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Model wrapped json in ```json ... ```
		fenced := "```json\n{\n  \"greeting\": \"hey\",\n  \"target\": \"team\"\n}\n```"
		_, _ = w.Write([]byte(mockGeminiServerResponse(fenced, 10, 10)))
	}))
	defer server.Close()

	client := NewGeminiClient(GeminiConfig{
		APIKey:      "fake-key",
		BaseURL:     server.URL,
		RPM:         600,
		BaseBackoff: 10 * time.Millisecond,
	})

	var out testResponse
	err := client.GenerateJSON(context.Background(), "test", nil, &out)
	if err != nil {
		t.Fatalf("GenerateJSON failed: %v", err)
	}

	if out.Greeting != "hey" || out.Target != "team" {
		t.Errorf("unexpected result from fenced output: %+v", out)
	}
}

func TestGeminiClient_RetryOn429(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&attempts, 1)
		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error": {"code": 429, "message": "rate limit exceeded"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockGeminiServerResponse(`{"greeting": "hi", "target": "after 429"}`, 10, 10)))
	}))
	defer server.Close()

	client := NewGeminiClient(GeminiConfig{
		APIKey:      "fake-key",
		BaseURL:     server.URL,
		RPM:         600,
		MaxRetries:  2,
		BaseBackoff: 10 * time.Millisecond,
	})

	var out testResponse
	err := client.GenerateJSON(context.Background(), "retry test", nil, &out)
	if err != nil {
		t.Fatalf("expected retry to succeed, got error: %v", err)
	}

	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
	if out.Target != "after 429" {
		t.Errorf("unexpected result: %+v", out)
	}
}

func TestGeminiClient_RetryOn503(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&attempts, 1)
		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error": {"code": 503, "message": "service unavailable"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockGeminiServerResponse(`{"greeting": "hi", "target": "after 503"}`, 10, 10)))
	}))
	defer server.Close()

	client := NewGeminiClient(GeminiConfig{
		APIKey:      "fake-key",
		BaseURL:     server.URL,
		RPM:         600,
		MaxRetries:  2,
		BaseBackoff: 10 * time.Millisecond,
	})

	var out testResponse
	err := client.GenerateJSON(context.Background(), "retry 503 test", nil, &out)
	if err != nil {
		t.Fatalf("expected retry to succeed, got error: %v", err)
	}

	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
	if out.Target != "after 503" {
		t.Errorf("unexpected result: %+v", out)
	}
}

func TestGeminiClient_NonRetryableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": {"code": 400, "message": "invalid argument"}}`))
	}))
	defer server.Close()

	client := NewGeminiClient(GeminiConfig{
		APIKey:      "fake-key",
		BaseURL:     server.URL,
		RPM:         600,
		MaxRetries:  2,
		BaseBackoff: 10 * time.Millisecond,
	})

	var out testResponse
	err := client.GenerateJSON(context.Background(), "bad request test", nil, &out)
	if err == nil {
		t.Fatalf("expected error on 400 Bad Request, got nil")
	}
}

func TestGeminiClient_AutoRepairSuccess(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if call == 1 {
			// Malformed JSON on first response
			_, _ = w.Write([]byte(mockGeminiServerResponse(`{greeting: invalid-json}`, 10, 10)))
			return
		}
		// Valid repaired JSON on auto-repair attempt
		_, _ = w.Write([]byte(mockGeminiServerResponse(`{"greeting": "repaired", "target": "successfully"}`, 12, 14)))
	}))
	defer server.Close()

	client := NewGeminiClient(GeminiConfig{
		APIKey:      "fake-key",
		BaseURL:     server.URL,
		RPM:         600,
		BaseBackoff: 10 * time.Millisecond,
	})

	var out testResponse
	err := client.GenerateJSON(context.Background(), "produce invalid first", nil, &out)
	if err != nil {
		t.Fatalf("expected auto-repair to succeed, got error: %v", err)
	}

	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("expected 2 calls (original + 1 auto-repair), got %d", callCount)
	}
	if out.Greeting != "repaired" || out.Target != "successfully" {
		t.Errorf("unexpected auto-repaired result: %+v", out)
	}
}

func TestGeminiClient_AutoRepairFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Model always returns broken json
		_, _ = w.Write([]byte(mockGeminiServerResponse(`not valid json at all`, 10, 10)))
	}))
	defer server.Close()

	client := NewGeminiClient(GeminiConfig{
		APIKey:      "fake-key",
		BaseURL:     server.URL,
		RPM:         600,
		BaseBackoff: 10 * time.Millisecond,
	})

	var out testResponse
	err := client.GenerateJSON(context.Background(), "always invalid", nil, &out)
	if err == nil {
		t.Fatalf("expected error when auto-repair also returns invalid json")
	}
}

func TestGeminiClient_MissingAPIKey(t *testing.T) {
	client := NewGeminiClient(GeminiConfig{
		APIKey: "",
	})

	var out testResponse
	err := client.GenerateJSON(context.Background(), "prompt", nil, &out)
	if err == nil {
		t.Fatalf("expected error when API key is missing")
	}
}

func TestRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(600) // 10 tokens per second

	ctx := context.Background()
	start := time.Now()
	for i := 0; i < 5; i++ {
		if err := limiter.Wait(ctx); err != nil {
			t.Fatalf("limiter wait failed: %v", err)
		}
	}
	elapsed := time.Since(start)

	if elapsed > 1*time.Second {
		t.Errorf("burst tokens should execute immediately, took %v", elapsed)
	}

	// Test context cancellation
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	err := limiter.Wait(canceledCtx)
	if err == nil {
		t.Errorf("expected error for canceled context")
	}
}
