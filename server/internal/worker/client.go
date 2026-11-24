package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"emailauto-server/internal/model"
)

// ScrapeRequest represents payload sent to the worker /scrape endpoint.
type ScrapeRequest struct {
	Query      string `json:"query"`
	TimeFilter string `json:"timeFilter"`
	SortBy     string `json:"sortBy"`
	MaxPosts   int    `json:"maxPosts"`
}

// ScrapedPost represents individual post extracted by Playwright worker.
type ScrapedPost struct {
	URL    string `json:"url"`
	Author string `json:"author"`
	Text   string `json:"text"`
}

// ScrapeResponse represents response from worker /scrape endpoint.
type ScrapeResponse struct {
	Success        bool          `json:"success"`
	Query          string        `json:"query"`
	TimeFilter     string        `json:"timeFilter"`
	SortBy         string        `json:"sortBy"`
	TotalExtracted int           `json:"totalExtracted"`
	Posts          []ScrapedPost `json:"posts"`
	Alert          string        `json:"alert,omitempty"`
	Message        string        `json:"message,omitempty"`
	Error          string        `json:"error,omitempty"`
}

// WorkerStatus represents status from worker /status endpoint.
type WorkerStatus struct {
	IsBusy         bool   `json:"isBusy"`
	ActiveRunID    string `json:"activeRunId"`
	RunsToday      int    `json:"runsToday"`
	MaxRunsPerDay  int    `json:"maxRunsPerDay"`
	MaxPostsPerRun int    `json:"maxPostsPerRun"`
}

// Client interacts with the Node.js Playwright worker over HTTP.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new worker Client.
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:3001"
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // Scrapes can take up to a few minutes with human delays
		},
	}
}

// Scrape requests the worker to perform LinkedIn content search and post extraction.
func (c *Client) Scrape(ctx context.Context, req ScrapeRequest) (*ScrapeResponse, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal scrape request: %w", err)
	}

	url := fmt.Sprintf("%s/scrape", c.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to playwright worker: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read worker response: %w", err)
	}

	if resp.StatusCode == http.StatusConflict {
		return nil, errors.New("worker is currently busy with another scrape run")
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, errors.New("worker daily limit reached (max 3 runs/day)")
	}

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("worker stopped due to CAPTCHA or login wall: %s", string(bodyBytes))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("worker returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var scrapeResp ScrapeResponse
	if err := json.Unmarshal(bodyBytes, &scrapeResp); err != nil {
		return nil, fmt.Errorf("failed to decode worker response: %w", err)
	}

	return &scrapeResp, nil
}

// ManualImport sends a pasted post to the worker for normalization.
func (c *Client) ManualImport(ctx context.Context, payload map[string]any) (*model.Post, error) {
	reqBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/manual-import", c.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("manual import failed: %s", string(b))
	}

	var res struct {
		Success bool       `json:"success"`
		Post    model.Post `json:"post"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	return &res.Post, nil
}

// GetStatus checks worker status and quota usage.
func (c *Client) GetStatus(ctx context.Context) (*WorkerStatus, error) {
	url := fmt.Sprintf("%s/status", c.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var status WorkerStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}

	return &status, nil
}
