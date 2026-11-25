package orchestrator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"emailauto-server/internal/model"
	"emailauto-server/internal/pipeline"
	"emailauto-server/internal/worker"
)

type mockSettingsStoreForOrc struct {
	data     map[string][]byte
	settings *model.Settings
}

func newMockSettingsStoreForOrc() *mockSettingsStoreForOrc {
	return &mockSettingsStoreForOrc{
		data:     make(map[string][]byte),
		settings: &model.Settings{DailyCap: 30},
	}
}

func (m *mockSettingsStoreForOrc) GetSettingRaw(ctx context.Context, key string) ([]byte, error) {
	return m.data[key], nil
}

func (m *mockSettingsStoreForOrc) SaveSettingRaw(ctx context.Context, key string, value []byte) error {
	m.data[key] = value
	return nil
}

func (m *mockSettingsStoreForOrc) Get(ctx context.Context) (*model.Settings, error) {
	return m.settings, nil
}

type mockPostStoreForOrc struct {
	seenURLs map[string]bool
	runs     []model.Run
}

func (m *mockPostStoreForOrc) HasPostWithURL(ctx context.Context, postURL string) (bool, error) {
	return m.seenURLs[postURL], nil
}

func (m *mockPostStoreForOrc) CreateRun(ctx context.Context, run *model.Run) (*model.Run, error) {
	run.ID = "run_test_id"
	m.runs = append(m.runs, *run)
	return run, nil
}

func (m *mockPostStoreForOrc) UpdateRun(ctx context.Context, runID, status string, finishedAt time.Time, stats map[string]any) error {
	for i := range m.runs {
		if m.runs[i].ID == runID {
			m.runs[i].Status = status
			m.runs[i].FinishedAt = &finishedAt
			m.runs[i].Stats = stats
			break
		}
	}
	return nil
}

type mockWorkerClientForOrc struct {
	posts []worker.ScrapedPost
}

func (m *mockWorkerClientForOrc) Scrape(ctx context.Context, req worker.ScrapeRequest) (*worker.ScrapeResponse, error) {
	return &worker.ScrapeResponse{
		Success:        true,
		TotalExtracted: len(m.posts),
		Posts:          m.posts,
	}, nil
}

type mockPipelineForOrc struct {
	processedCount int
}

func (m *mockPipelineForOrc) ProcessPost(ctx context.Context, post *model.Post, settings model.Settings) (*pipeline.ProcessPostResult, error) {
	m.processedCount++
	return &pipeline.ProcessPostResult{
		Post:   post,
		Passed: true,
		Outreach: &model.Outreach{
			ID: "draft_1",
		},
	}, nil
}

func TestOrchestrator_ArmedLogic(t *testing.T) {
	ctx := context.Background()
	settingsStore := newMockSettingsStoreForOrc()
	orc := NewOrchestrator(settingsStore, nil, nil, nil, nil, nil)

	fixedNow := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	orc.nowFunc = func() time.Time { return fixedNow }

	// 1. Initially not armed
	if orc.IsArmed(fixedNow) {
		t.Errorf("expected engine to be un-armed initially")
	}

	// 2. Manual run arms the engine for today
	orc.ArmManualRun(ctx, fixedNow)
	if !orc.IsArmed(fixedNow) {
		t.Errorf("expected engine to be armed after manual run today")
	}

	// 3. Tomorrow, engine is automatically disarmed without clicking Run now
	tomorrow := fixedNow.Add(24 * time.Hour)
	if orc.IsArmed(tomorrow) {
		t.Errorf("expected engine to automatically disarm on next calendar day")
	}
}

func TestOrchestrator_NoCatchUpOnSleepOrRestart(t *testing.T) {
	ctx := context.Background()
	settingsStore := newMockSettingsStoreForOrc()

	// Persist schedule where next_run_at was 4 hours ago (laptop was asleep)
	now := time.Date(2026, 11, 24, 16, 0, 0, 0, time.UTC)
	fourHoursAgo := now.Add(-4 * time.Hour)

	state := ScheduleState{
		AutoEnabled: true,
		State:       StateIdle,
		NextRunAt:   &fourHoursAgo,
	}
	raw, _ := json.Marshal(state)
	_ = settingsStore.SaveSettingRaw(ctx, StateSettingsKey, raw)

	orc := NewOrchestrator(settingsStore, nil, nil, nil, nil, nil)
	orc.nowFunc = func() time.Time { return now }

	err := orc.InitFromDB(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// NO-CATCH-UP: NextRunAt must be pushed to future (now + 2h = 18:00), NOT in the past
	if orc.nextRunAt == nil || !orc.nextRunAt.After(now) {
		t.Errorf("expected nextRunAt to be in the future, got: %v (now was %v)", orc.nextRunAt, now)
	}
	expectedFuture := now.Add(2 * time.Hour)
	if !orc.nextRunAt.Equal(expectedFuture) {
		t.Errorf("expected nextRunAt to be %v, got %v", expectedFuture, orc.nextRunAt)
	}
}

func TestOrchestrator_Deduplication(t *testing.T) {
	ctx := context.Background()
	settingsStore := newMockSettingsStoreForOrc()
	postStore := &mockPostStoreForOrc{
		seenURLs: map[string]bool{
			"https://linkedin.com/feed/update/already_seen": true,
		},
	}
	workerClient := &mockWorkerClientForOrc{
		posts: []worker.ScrapedPost{
			{URL: "https://linkedin.com/feed/update/already_seen", Text: "Old post"},
			{URL: "https://linkedin.com/feed/update/brand_new", Text: "Brand new post"},
		},
	}
	pipeMock := &mockPipelineForOrc{}

	orc := NewOrchestrator(settingsStore, postStore, workerClient, pipeMock, nil, nil)
	run, err := orc.RunPipeline(ctx, "manual", "golang developer", "24h", "latest", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if run.Stats["found"] != 2 {
		t.Errorf("expected 2 found, got %v", run.Stats["found"])
	}
	// Only brand_new should have been processed by the pipeline; already_seen skipped
	if pipeMock.processedCount != 1 {
		t.Errorf("expected exactly 1 post processed by pipeline (1 skipped as dupe), got %d", pipeMock.processedCount)
	}
}

func TestOrchestrator_KillSwitch(t *testing.T) {
	ctx := context.Background()
	settingsStore := newMockSettingsStoreForOrc()
	orc := NewOrchestrator(settingsStore, nil, nil, nil, nil, nil)

	now := time.Now()
	orc.ToggleAuto(ctx, true)
	orc.ArmManualRun(ctx, now)

	// Activate Kill Switch
	orc.KillSwitch(ctx)

	status, _ := orc.GetStatus(ctx)
	if status.State != StatePaused {
		t.Errorf("expected state PAUSED, got %s", status.State)
	}
	if status.AutoEnabled {
		t.Errorf("expected auto_enabled to be false")
	}
	if status.Armed {
		t.Errorf("expected engine to be disarmed")
	}
}
