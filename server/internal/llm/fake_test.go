package llm

import (
	"context"
	"testing"
)

type sampleOutput struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestFakeClient_Responses(t *testing.T) {
	fake := NewFakeClient(
		`{"name": "Alice", "age": 30}`,
		`{"name": "Bob", "age": 25}`,
	)

	var first sampleOutput
	err := fake.GenerateJSON(context.Background(), "prompt 1", nil, &first)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if first.Name != "Alice" || first.Age != 30 {
		t.Errorf("unexpected first result: %+v", first)
	}

	var second sampleOutput
	err = fake.GenerateJSON(context.Background(), "prompt 2", nil, &second)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if second.Name != "Bob" || second.Age != 25 {
		t.Errorf("unexpected second result: %+v", second)
	}

	if fake.CallCount() != 2 {
		t.Errorf("expected 2 calls, got %d", fake.CallCount())
	}

	// Empty queue error
	var third sampleOutput
	err = fake.GenerateJSON(context.Background(), "prompt 3", nil, &third)
	if err == nil {
		t.Errorf("expected error when queue is empty")
	}
}

func TestFakeClient_Handler(t *testing.T) {
	fake := &FakeClient{
		Handler: func(ctx context.Context, prompt string, schema any) (string, error) {
			return `{"name": "Dynamo", "age": 99}`, nil
		},
	}

	var out sampleOutput
	err := fake.GenerateJSON(context.Background(), "custom", nil, &out)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if out.Name != "Dynamo" || out.Age != 99 {
		t.Errorf("unexpected result: %+v", out)
	}
}
