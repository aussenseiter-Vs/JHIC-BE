package chat

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa"
)

type Service struct {
	client nexxa.N8NClient
}

func NewService(client nexxa.N8NClient) *Service {
	return &Service{client: client}
}

func (s *Service) Chat(ctx context.Context, chatInput, sessionID string) (*nexxa.ChatResponse, error) {
	chatInput = strings.TrimSpace(chatInput)
	if chatInput == "" {
		return nil, ErrChatMessageRequired
	}
	if len(chatInput) > ChatMessageMaxLen {
		return nil, ErrChatMessageTooLong
	}
	if sessionID == "" {
		sessionID = newSessionID()
	}
	return s.client.Chat(ctx, chatInput, sessionID)
}

func newSessionID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}