package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"emailauto-server/internal/model"
	"emailauto-server/internal/pipeline"
	"emailauto-server/internal/worker"
)

// OrchestratorState represents the execution status of the orchestrator engine.
type OrchestratorState string

const (
	StateIdle    OrchestratorState = "IDLE"
	StateRunning OrchestratorState = "RUNNING"
	StatePaused  OrchestratorState = "PAUSED"
)

const StateSettingsKey = "orchestrator_state"

// ScheduleState encapsulates the persistent schedule and arming state in the database.
type ScheduleState struct {
	AutoEnabled     bool              `json:"auto_enabled"`
	State           OrchestratorState `json:"state"`
	ArmedDate       string            `json:"armed_date"` // YYYY-MM-DD of manual run
	LastManualRunAt *time.Time        `json:"last_manual_run_at,omitempty"`
	LastRunAt       *time.Time        `json:"last_run_at,omitempty"`
	NextRunAt       *time.Time        `json:"next_run_at,omitempty"`
}

// OrchestratorStatus represents runtime metrics exposed to the API and UI dashboard.
type OrchestratorStatus struct {
	State            OrchestratorState `json:"state"`
	AutoEnabled      bool              `json:"auto_enabled"`
	Armed            bool              `json:"armed"`
	LastManualRunAt  *time.Time        `json:"last_manual_run_at,omitempty"`
	LastRunAt        *time.Time        `json:"last_run_at,omitempty"`
	NextRunAt        *time.Time        `json:"next_run_at,omitempty"`
	NextRunInSeconds int               `json:"next_run_in_seconds"`
	DailySent        int               `json:"daily_sent"`
	DailyCap         int               `json:"daily_cap"`
}

// SettingsStateStore manages persistence of the orchestrator state in settings.
type SettingsStateStore interface {
	GetSettingRaw(ctx context.Context, key string) ([]byte, error)
	SaveSettingRaw(ctx context.Context, key string, value []byte) error
	Get(ctx context.Context) (*model.Settings, error)
}

// PostStore defines database operations for posts, runs, and deduplication.
type PostStore interface {
	HasPostWithURL(ctx context.Context, postURL string) (bool, error)
	CreateRun(ctx context.Context, run *model.Run) (*model.Run, error)
	UpdateRun(ctx context.Context, runID, status string, finishedAt time.Time, stats map[string]any) error
}

// WorkerScraper defines interface for scraping posts via Playwright worker.
type WorkerScraper interface {
	Scrape(ctx context.Context, req worker.ScrapeRequest) (*worker.ScrapeResponse, error)
}

// PipelineService defines post extraction, verification, and draft generation.
type PipelineService interface {
	ProcessPost(ctx context.Context, post *model.Post, settings model.Settings) (*pipeline.ProcessPostResult, error)
}

// BatchPipelineService is an optional interface for batched post processing and 10-post LLM drafting.
type BatchPipelineService interface {
	ProcessAndDraftBatch(ctx context.Context, posts []model.Post, settings model.Settings) ([]pipeline.ProcessPostResult, error)
}

// OutreachCounter defines access to sent counts.
type OutreachCounter interface {
	CountSentToday(ctx context.Context) (int, error)
}

// Orchestrator coordinates the 2-hour autonomous loop, arming checks, and state transitions.
type Orchestrator struct {
	mu              sync.RWMutex
	state           OrchestratorState
	autoEnabled     bool
	lastManualRunAt *time.Time
	lastRunAt       *time.Time
	nextRunAt       *time.Time
	armedDate       string

	interval      time.Duration
	stopCh        chan struct{}
	runningLoop   bool
	settingsStore SettingsStateStore
	postStore     PostStore
	workerClient  WorkerScraper
	pipeline      PipelineService
	counter       OutreachCounter
	logger        *slog.Logger

	// Testable clock hook
	nowFunc func() time.Time
}

// NewOrchestrator creates a new Orchestrator instance.
func NewOrchestrator(
	settingsStore SettingsStateStore,
	postStore PostStore,
	workerClient WorkerScraper,
	pipeline PipelineService,
	counter OutreachCounter,
	logger *slog.Logger,
) *Orchestrator {
	if logger == nil {
		logger = slog.Default()
	}

	orc := &Orchestrator{
		state:         StateIdle,
		autoEnabled:   false,
		interval:      2 * time.Hour,
		stopCh:        make(chan struct{}),
		settingsStore: settingsStore,
		postStore:     postStore,
		workerClient:  workerClient,
		pipeline:      pipeline,
		counter:       counter,
		logger:        logger,
		nowFunc:       time.Now,
	}

	return orc
}

// InitFromDB loads persisted schedule state from DB with strict no-catch-up recovery after sleep/restart.
func (o *Orchestrator) InitFromDB(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	now := o.nowFunc()

	if o.settingsStore != nil {
		raw, err := o.settingsStore.GetSettingRaw(ctx, StateSettingsKey)
		if err == nil && len(raw) > 0 {
			var persisted ScheduleState
			if err := json.Unmarshal(raw, &persisted); err == nil {
				o.autoEnabled = persisted.AutoEnabled
				o.armedDate = persisted.ArmedDate
				o.lastManualRunAt = persisted.LastManualRunAt
				o.lastRunAt = persisted.LastRunAt

				if persisted.State == StatePaused {
					o.state = StatePaused
				} else {
					o.state = StateIdle
				}

				// NO-CATCH-UP LOGIC:
				// If persisted next_run_at is in the past (e.g. laptop slept or server restarted),
				// do NOT catch up missed ticks! Just wait for the next tick in the future.
				if persisted.NextRunAt != nil && persisted.NextRunAt.Before(now) {
					futureNext := now.Add(o.interval)
					o.nextRunAt = &futureNext
					o.logger.Info("laptop sleep or restart detected: no-catch-up enforced, scheduled next tick",
						"missed_tick", persisted.NextRunAt,
						"new_next_tick", o.nextRunAt,
					)
				} else {
					o.nextRunAt = persisted.NextRunAt
				}

				return nil
			}
		}
	}

	// Default initialization
	next := now.Add(o.interval)
	o.nextRunAt = &next
	return nil
}

// Start launches the background 2-hour scheduler loop.
func (o *Orchestrator) Start() {
	o.mu.Lock()
	if o.runningLoop {
		o.mu.Unlock()
		return
	}
	o.runningLoop = true
	o.stopCh = make(chan struct{})
	o.mu.Unlock()

	o.logger.Info("starting orchestrator autonomous scheduler (2-hour interval)")

	go func() {
		ticker := time.NewTicker(1 * time.Minute) // checks schedule every minute
		defer ticker.Stop()

		for {
			select {
			case <-o.stopCh:
				o.logger.Info("orchestrator scheduler stopped")
				return
			case <-ticker.C:
				ctx := context.Background()
				if o.ShouldAutoRun(ctx) {
					o.logger.Info("triggering scheduled 2-hour autonomous pipeline run")
					_, err := o.RunPipeline(ctx, "auto", "", "24h", "latest", 60)
					if err != nil {
						o.logger.Error("error during autonomous pipeline run", "error", err)
					}
				}
			}
		}
	}()
}

// Stop shuts down the scheduler loop.
func (o *Orchestrator) Stop() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.runningLoop {
		return
	}
	close(o.stopCh)
	o.runningLoop = false
}

// ArmManualRun records a manual "Run now" execution for today, arming the auto pipeline.
func (o *Orchestrator) ArmManualRun(ctx context.Context, now time.Time) {
	o.mu.Lock()
	defer o.mu.Unlock()

	dateStr := now.Format("2006-01-02")
	o.armedDate = dateStr
	o.lastManualRunAt = &now
	o.logger.Info("engine armed via manual run", "date", dateStr)

	o.persistState(ctx)
}

// IsArmed checks if there was a manual "Run now" execution today.
func (o *Orchestrator) IsArmed(now time.Time) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if o.lastManualRunAt == nil || o.armedDate == "" {
		return false
	}

	todayStr := now.Format("2006-01-02")
	return o.armedDate == todayStr
}

// ShouldAutoRun evaluates all conditions required to trigger the 2-hour auto pipeline.
func (o *Orchestrator) ShouldAutoRun(ctx context.Context) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()

	now := o.nowFunc()

	// 1. auto_enabled must be true
	if !o.autoEnabled {
		return false
	}

	// 2. state must be IDLE (not RUNNING and not PAUSED)
	if o.state != StateIdle {
		return false
	}

	// 3. Armed condition: manual "Run now" must have occurred today
	todayStr := now.Format("2006-01-02")
	if o.armedDate != todayStr || o.lastManualRunAt == nil {
		return false
	}

	// 4. Timer check: must be at or past nextRunAt
	if o.nextRunAt != nil && now.Before(*o.nextRunAt) {
		return false
	}

	return true
}

// ToggleAuto enables or disables auto mode.
func (o *Orchestrator) ToggleAuto(ctx context.Context, enabled bool) bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.autoEnabled = enabled
	if enabled && o.state == StatePaused {
		o.state = StateIdle
	}

	now := o.nowFunc()
	if enabled && (o.nextRunAt == nil || o.nextRunAt.Before(now)) {
		next := now.Add(o.interval)
		o.nextRunAt = &next
	}

	o.logger.Info("orchestrator auto mode toggled", "enabled", enabled, "state", o.state)
	o.persistState(ctx)
	return o.autoEnabled
}

// KillSwitch immediately pauses the engine, disables auto mode, and disarms the engine.
func (o *Orchestrator) KillSwitch(ctx context.Context) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.state = StatePaused
	o.autoEnabled = false
	o.armedDate = ""
	o.lastManualRunAt = nil

	o.logger.Warn("KILL SWITCH ACTIVATED: auto disabled, engine disarmed, state paused")
	o.persistState(ctx)
}

// GetStatus returns the current orchestrator status report for UI dashboard.
func (o *Orchestrator) GetStatus(ctx context.Context) (*OrchestratorStatus, error) {
	o.mu.RLock()
	state := o.state
	autoEnabled := o.autoEnabled
	lastManual := o.lastManualRunAt
	lastRun := o.lastRunAt
	nextRun := o.nextRunAt
	armedDate := o.armedDate
	o.mu.RUnlock()

	now := o.nowFunc()
	todayStr := now.Format("2006-01-02")
	armed := (lastManual != nil && armedDate == todayStr)

	nextRunInSec := 0
	if nextRun != nil && nextRun.After(now) {
		nextRunInSec = int(nextRun.Sub(now).Seconds())
	}

	dailySent := 0
	dailyCap := 30

	if o.counter != nil {
		if count, err := o.counter.CountSentToday(ctx); err == nil {
			dailySent = count
		}
	}

	if o.settingsStore != nil {
		if st, err := o.settingsStore.Get(ctx); err == nil && st != nil {
			dailyCap = st.DailyCap
		}
	}

	return &OrchestratorStatus{
		State:            state,
		AutoEnabled:      autoEnabled,
		Armed:            armed,
		LastManualRunAt:  lastManual,
		LastRunAt:        lastRun,
		NextRunAt:        nextRun,
		NextRunInSeconds: nextRunInSec,
		DailySent:        dailySent,
		DailyCap:         dailyCap,
	}, nil
}

// RunPipeline executes the full job search, extraction, verification, and drafting workflow.
func (o *Orchestrator) RunPipeline(
	ctx context.Context,
	trigger string,
	query string,
	timeFilter string,
	sortBy string,
	maxPosts int,
) (*model.Run, error) {
	o.mu.Lock()
	if o.state == StateRunning {
		o.mu.Unlock()
		return nil, fmt.Errorf("a pipeline run is already in progress")
	}
	if o.state == StatePaused && trigger == "auto" {
		o.mu.Unlock()
		return nil, fmt.Errorf("cannot run autonomous pipeline while paused")
	}

	o.state = StateRunning
	o.mu.Unlock()

	defer func() {
		o.mu.Lock()
		o.state = StateIdle
		now := o.nowFunc()
		o.lastRunAt = &now
		next := now.Add(o.interval)
		o.nextRunAt = &next
		o.persistState(ctx)
		o.mu.Unlock()
	}()

	now := o.nowFunc()
	if trigger == "manual" {
		o.ArmManualRun(ctx, now)
	}

	if timeFilter == "" {
		timeFilter = "24h"
	}
	if sortBy == "" {
		sortBy = "latest"
	}
	if maxPosts <= 0 {
		maxPosts = 150
	}

	// 1. Load active settings
	var settings model.Settings
	if o.settingsStore != nil {
		if s, err := o.settingsStore.Get(ctx); err == nil && s != nil {
			settings = *s
		} else {
			settings = model.DefaultSettings()
		}
	} else {
		settings = model.DefaultSettings()
	}

	searchQuery := strings.TrimSpace(query)
	if searchQuery == "" {
		if len(settings.IncludeKeywords) > 0 {
			searchQuery = strings.Join(settings.IncludeKeywords, " ")
		} else {
			searchQuery = "golang or react engineer"
		}
	}

	runRecord := &model.Run{
		Query:      searchQuery,
		TimeFilter: timeFilter,
		SortBy:     sortBy,
		Trigger:    trigger,
		Status:     "running",
		StartedAt:  &now,
		Stats:      map[string]any{},
	}

	if o.postStore != nil {
		created, err := o.postStore.CreateRun(ctx, runRecord)
		if err == nil && created != nil {
			runRecord = created
		}
	}

	// 2. Scrape posts from worker
	if o.workerClient == nil {
		o.finishRun(ctx, runRecord, "failed", map[string]any{"error": "worker client not configured", "issue": "worker client not configured"})
		return runRecord, fmt.Errorf("worker client not configured")
	}

	scrapeRes, err := o.workerClient.Scrape(ctx, worker.ScrapeRequest{
		Query:      searchQuery,
		TimeFilter: timeFilter,
		SortBy:     sortBy,
		MaxPosts:   maxPosts,
	})

	if err != nil {
		errMsg := err.Error()
		issue := errMsg
		lowerErr := strings.ToLower(errMsg)
		if strings.Contains(lowerErr, "login wall") || strings.Contains(lowerErr, "captcha") || strings.Contains(lowerErr, "verification url") || strings.Contains(lowerErr, "uas/login") {
			issue = "LinkedIn Login Required: The scraper was redirected to the LinkedIn login/verification screen. Please open the Chrome browser window, log into your LinkedIn account, and re-run."
		}
		o.finishRun(ctx, runRecord, "failed", map[string]any{
			"error": errMsg,
			"issue": issue,
		})
		return runRecord, fmt.Errorf("%s", issue)
	}

	// 3. Process & dedupe posts (with batched 10-post Gemini drafting when available)
	foundCount := len(scrapeRes.Posts)
	filteredCount := 0
	verifiedCount := 0
	skippedCount := 0
	draftedCount := 0

	var newPosts []model.Post
	for _, sp := range scrapeRes.Posts {
		// DEDUPE: skip already seen posts by URL
		if o.postStore != nil {
			seen, _ := o.postStore.HasPostWithURL(ctx, sp.URL)
			if seen {
				skippedCount++
				continue
			}
		}

		p := model.Post{
			URL:     sp.URL,
			Author:  sp.Author,
			Text:    sp.Text,
			Status:  "new",
			FoundAt: time.Now().UTC(),
		}
		if sp.PostedAt != "" {
			p.Extracted.PostedAt = sp.PostedAt
		}
		newPosts = append(newPosts, p)
	}

	if o.pipeline != nil && len(newPosts) > 0 {
		filteredCount = len(newPosts)
		if batchPipe, ok := o.pipeline.(BatchPipelineService); ok {
			results, _ := batchPipe.ProcessAndDraftBatch(ctx, newPosts, settings)
			for _, res := range results {
				if res.Passed {
					verifiedCount++
					if res.Outreach != nil {
						draftedCount++
					}
				} else {
					skippedCount++
				}
			}
		} else {
			for i := range newPosts {
				res, err := o.pipeline.ProcessPost(ctx, &newPosts[i], settings)
				if err == nil && res != nil {
					if res.Passed {
						verifiedCount++
						if res.Outreach != nil {
							draftedCount++
						}
					} else {
						skippedCount++
					}
				} else {
					skippedCount++
				}
			}
		}
	}

	stats := map[string]any{
		"found":    foundCount,
		"filtered": filteredCount,
		"verified": verifiedCount,
		"skipped":  skippedCount,
		"drafted":  draftedCount,
	}

	o.finishRun(ctx, runRecord, "completed", stats)
	return runRecord, nil
}

func (o *Orchestrator) finishRun(ctx context.Context, run *model.Run, status string, stats map[string]any) {
	now := o.nowFunc().UTC()
	run.Status = status
	run.FinishedAt = &now
	run.Stats = stats

	if o.postStore != nil && run.ID != "" {
		_ = o.postStore.UpdateRun(ctx, run.ID, status, now, stats)
	}
}

func (o *Orchestrator) persistState(ctx context.Context) {
	if o.settingsStore == nil {
		return
	}

	state := ScheduleState{
		AutoEnabled:     o.autoEnabled,
		State:           o.state,
		ArmedDate:       o.armedDate,
		LastManualRunAt: o.lastManualRunAt,
		LastRunAt:       o.lastRunAt,
		NextRunAt:       o.nextRunAt,
	}

	raw, err := json.Marshal(state)
	if err == nil {
		_ = o.settingsStore.SaveSettingRaw(ctx, StateSettingsKey, raw)
	}
}
