package nexxa

import "context"

// Message is a single chat-completions message sent to the LLM client.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AIClient performs LLM chat completions. The concrete implementation lives in
// internal/infrastructure/llm/ and translates upstream failures into the
// domain's sentinel errors (ErrUpstreamUnavailable / ErrUpstreamTimeout).
type AIClient interface {
	// Complete runs a chat completion and returns the assistant's raw output.
	// When jsonMode is true the client requests structured JSON output.
	Complete(ctx context.Context, messages []Message, jsonMode bool) (string, error)
}