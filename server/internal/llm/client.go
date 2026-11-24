package llm

import "context"

// Client is the interface for LLM JSON generation.
type Client interface {
	// GenerateJSON sends a prompt and schema to the model and unmarshals the JSON response into out.
	GenerateJSON(ctx context.Context, prompt string, schema any, out any) error
}
