package nexxa

import "context"

type N8NClient interface {
	Chat(ctx context.Context, chatInput, sessionID string) (*ChatResponse, error)
	NexxaMatch(ctx context.Context, answers []string) (string, error)
}
