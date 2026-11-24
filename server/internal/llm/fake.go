package llm

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
)

// CallRecord tracks an invocation of GenerateJSON.
type CallRecord struct {
	Prompt string
	Schema any
}

// FakeClient is a test double implementing Client.
type FakeClient struct {
	mu        sync.Mutex
	Responses []string
	Handler   func(ctx context.Context, prompt string, schema any) (string, error)
	Calls     []CallRecord
}

// NewFakeClient creates a FakeClient initialized with sequential JSON response strings.
func NewFakeClient(responses ...string) *FakeClient {
	return &FakeClient{
		Responses: responses,
	}
}

// GenerateJSON records the call and unmarshals the next response (or handler output) into out.
func (f *FakeClient) GenerateJSON(ctx context.Context, prompt string, schema any, out any) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, CallRecord{
		Prompt: prompt,
		Schema: schema,
	})

	var raw string
	if f.Handler != nil {
		resp, err := f.Handler(ctx, prompt, schema)
		if err != nil {
			return err
		}
		raw = resp
	} else {
		if len(f.Responses) == 0 {
			return errors.New("fake client: no responses available in queue")
		}
		raw = f.Responses[0]
		f.Responses = f.Responses[1:]
	}

	cleaned := CleanJSON(raw)
	if err := json.Unmarshal([]byte(cleaned), out); err != nil {
		return err
	}

	return nil
}

// CallCount returns the number of times GenerateJSON was called.
func (f *FakeClient) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Calls)
}
