package chat

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa"
)

type Service struct {
	client     nexxa.AIClient
	kb         KnowledgeRepository
	embedder   Embedder
	memory     *Memory
	maxResults int
}

func NewService(client nexxa.AIClient) *Service {
	return &Service{
		client:     client,
		memory:     NewMemory(DefaultHistoryMaxMessages, DefaultHistoryTTL),
		maxResults: 5,
	}
}

// WithRetrieval wires knowledge-base grounding (RAG) into the chat flow.
func (s *Service) WithRetrieval(kb KnowledgeRepository, embedder Embedder, maxResults int) *Service {
	s.kb = kb
	s.embedder = embedder
	if maxResults > 0 {
		s.maxResults = maxResults
	}
	return s
}

// WithMemory overrides the default per-session history limits.
func (s *Service) WithMemory(maxHistory int, ttl time.Duration) *Service {
	s.memory = NewMemory(maxHistory, ttl)
	return s
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

	context := s.retrieveContext(ctx, chatInput)
	messages := BuildMessages(context, s.memory.Messages(sessionID), chatInput)

	raw, err := s.client.Complete(ctx, messages, false)
	if err != nil {
		return nil, err
	}

	s.memory.Append(sessionID, chatInput, raw)
	return &nexxa.ChatResponse{Output: raw}, nil
}

// retrieveContext ground-truths the latest question against the KB. Embedding
// or search failures degrade gracefully to no context.
func (s *Service) retrieveContext(ctx context.Context, chatInput string) []Chunk {
	if s.kb == nil || s.embedder == nil {
		return nil
	}
	vecs, err := s.embedder.Embed(ctx, []string{chatInput})
	if err != nil || len(vecs) == 0 {
		return nil
	}
	chunks, err := s.kb.Search(ctx, vecs[0], s.maxResults)
	if err != nil {
		return nil
	}
	return chunks
}

func newSessionID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}