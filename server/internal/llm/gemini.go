package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"
)

// GeminiConfig holds options for creating a GeminiClient.
type GeminiConfig struct {
	APIKey      string
	Model       string        // default: "gemini-2.5-flash"
	BaseURL     string        // default: "https://generativelanguage.googleapis.com"
	RPM         int           // default: 30
	Timeout     time.Duration // default: 30s
	MaxRetries  int           // default: 3
	BaseBackoff time.Duration // default: 500ms
	HTTPClient  *http.Client
	Logger      *slog.Logger
}

// GeminiClient implements Client using the official Gemini REST API.
type GeminiClient struct {
	apiKey      string
	model       string
	baseURL     string
	limiter     *RateLimiter
	timeout     time.Duration
	maxRetries  int
	baseBackoff time.Duration
	httpClient  *http.Client
	logger      *slog.Logger
}

// NewGeminiClient creates a configured GeminiClient.
func NewGeminiClient(cfg GeminiConfig) *GeminiClient {
	if cfg.Model == "" {
		cfg.Model = "gemini-2.5-flash"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://generativelanguage.googleapis.com"
	}
	if cfg.RPM <= 0 {
		cfg.RPM = 30
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = 500 * time.Millisecond
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: cfg.Timeout}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &GeminiClient{
		apiKey:      cfg.APIKey,
		model:       cfg.Model,
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		limiter:     NewRateLimiter(cfg.RPM),
		timeout:     cfg.Timeout,
		maxRetries:  cfg.MaxRetries,
		baseBackoff: cfg.BaseBackoff,
		httpClient:  cfg.HTTPClient,
		logger:      cfg.Logger,
	}
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	ResponseMimeType string `json:"responseMimeType,omitempty"`
	ResponseSchema   any    `json:"responseSchema,omitempty"`
}

type geminiRequest struct {
	Contents         []geminiContent        `json:"contents"`
	GenerationConfig geminiGenerationConfig `json:"generationConfig"`
}

type geminiCandidateContent struct {
	Parts []geminiPart `json:"parts"`
	Role  string       `json:"role,omitempty"`
}

type geminiCandidate struct {
	Content      geminiCandidateContent `json:"content"`
	FinishReason string                 `json:"finishReason,omitempty"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate   `json:"candidates"`
	UsageMetadata geminiUsageMetadata `json:"usageMetadata"`
	Error         *geminiError        `json:"error,omitempty"`
}

type geminiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// GenerateJSON sends a prompt and optional schema to Gemini, enforcing JSON output.
// Handles rate limiting, exponential backoff retries, schema unmarshaling, and one auto-repair retry.
func (c *GeminiClient) GenerateJSON(ctx context.Context, prompt string, schema any, out any) error {
	if c.apiKey == "" {
		return errors.New("gemini api key is not configured")
	}

	rawJSON, err := c.executePromptWithRetry(ctx, prompt, schema)
	if err != nil {
		return err
	}

	cleaned := CleanJSON(rawJSON)
	if err := json.Unmarshal([]byte(cleaned), out); err != nil {
		c.logger.Warn("gemini returned invalid json, triggering auto-repair",
			"error", err,
			"raw", rawJSON,
		)

		// One auto-repair retry on invalid JSON
		repairPrompt := fmt.Sprintf(
			"The following output was expected to be valid JSON conforming to the schema, but failed to parse with error:\n%v\n\nOriginal output:\n%s\n\nRepair the output and return strictly valid JSON conforming to the schema with no markdown formatting or commentary.",
			err, rawJSON,
		)

		repairedRaw, repairErr := c.executePromptWithRetry(ctx, repairPrompt, schema)
		if repairErr != nil {
			return fmt.Errorf("auto-repair request failed: %w (original parse error: %v)", repairErr, err)
		}

		repairedClean := CleanJSON(repairedRaw)
		if unmarshalErr := json.Unmarshal([]byte(repairedClean), out); unmarshalErr != nil {
			return fmt.Errorf("auto-repair failed to produce valid json: %w (raw: %s)", unmarshalErr, repairedRaw)
		}

		c.logger.Info("auto-repair successfully recovered valid json")
	}

	return nil
}

func (c *GeminiClient) executePromptWithRetry(ctx context.Context, prompt string, schema any) (string, error) {
	// Respect global rate limiter
	if err := c.limiter.Wait(ctx); err != nil {
		return "", fmt.Errorf("rate limiter error: %w", err)
	}

	// Apply timeout if context has no deadline
	reqCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Role: "user",
				Parts: []geminiPart{
					{Text: prompt},
				},
			},
		},
		GenerationConfig: geminiGenerationConfig{
			ResponseMimeType: "application/json",
		},
	}

	if schema != nil {
		reqBody.GenerationConfig.ResponseSchema = schema
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal gemini request: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", c.baseURL, c.model, c.apiKey)

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(float64(c.baseBackoff) * math.Pow(2, float64(attempt-1)))
			c.logger.Warn("retrying gemini request",
				"attempt", attempt,
				"max_retries", c.maxRetries,
				"backoff", backoff,
				"error", lastErr,
			)

			select {
			case <-reqCtx.Done():
				return "", reqCtx.Err()
			case <-time.After(backoff):
			}
		}

		start := time.Now()
		httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(reqBytes))
		if err != nil {
			return "", fmt.Errorf("failed to create http request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = err
			if reqCtx.Err() != nil {
				return "", reqCtx.Err()
			}
			continue
		}

		respBytes, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		latency := time.Since(start)

		if err != nil {
			lastErr = fmt.Errorf("failed to read response body: %w", err)
			continue
		}

		// Retry on 429 Too Many Requests or 5xx Server Errors
		if resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode >= 500 && resp.StatusCode <= 599) {
			lastErr = fmt.Errorf("gemini api returned status %d: %s", resp.StatusCode, string(respBytes))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("gemini api error (status %d): %s", resp.StatusCode, string(respBytes))
		}

		var geminiResp geminiResponse
		if err := json.Unmarshal(respBytes, &geminiResp); err != nil {
			return "", fmt.Errorf("failed to decode gemini response json: %w", err)
		}

		if geminiResp.Error != nil {
			return "", fmt.Errorf("gemini api returned error code %d: %s", geminiResp.Error.Code, geminiResp.Error.Message)
		}

		if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
			return "", errors.New("gemini api returned no candidate content")
		}

		c.logger.Info("gemini request completed",
			slog.String("model", c.model),
			slog.Duration("latency", latency),
			slog.Int("prompt_tokens", geminiResp.UsageMetadata.PromptTokenCount),
			slog.Int("candidate_tokens", geminiResp.UsageMetadata.CandidatesTokenCount),
			slog.Int("total_tokens", geminiResp.UsageMetadata.TotalTokenCount),
		)

		return geminiResp.Candidates[0].Content.Parts[0].Text, nil
	}

	return "", fmt.Errorf("gemini request failed after %d retries: %w", c.maxRetries, lastErr)
}

// CleanJSON trims whitespace and removes optional markdown code fences.
func CleanJSON(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "```") {
		// Strip first line (e.g. ```json)
		if idx := strings.Index(trimmed, "\n"); idx != -1 {
			trimmed = trimmed[idx+1:]
		}
		// Strip trailing ```
		if strings.HasSuffix(trimmed, "```") {
			trimmed = strings.TrimSuffix(trimmed, "```")
		}
		trimmed = strings.TrimSpace(trimmed)
	}
	return trimmed
}
