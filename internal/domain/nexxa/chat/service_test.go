package chat_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/chat"
	chatmocks "github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/chat/mocks"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService_Chat(t *testing.T) {
	t.Run("success forwards message and session", func(t *testing.T) {
		client := mocks.NewAIClient(t)
		client.On("Complete", mock.Anything, mock.MatchedBy(func(msgs []nexxa.Message) bool {
			if len(msgs) != 2 {
				return false
			}
			return msgs[0].Role == "system" && msgs[1].Role == "user" && msgs[1].Content == "halo"
		}), false).Return("hai", nil)

		svc := chat.NewService(client)
		got, err := svc.Chat(context.Background(), "halo", "session-1")
		require.NoError(t, err)
		require.Equal(t, "hai", got.Output)
	})

	t.Run("keeps history within a session", func(t *testing.T) {
		client := mocks.NewAIClient(t)
		svc := chat.NewService(client)

		client.On("Complete", mock.Anything, mock.MatchedBy(func(msgs []nexxa.Message) bool {
			return len(msgs) == 2
		}), false).Return("balasan", nil).Once()
		client.On("Complete", mock.Anything, mock.MatchedBy(func(msgs []nexxa.Message) bool {
			return len(msgs) == 4
		}), false).Return("balasan", nil).Once()

		first, err := svc.Chat(context.Background(), "halo", "session-1")
		require.NoError(t, err)
		require.Equal(t, "balasan", first.Output)

		_, err = svc.Chat(context.Background(), "lagi", "session-1")
		require.NoError(t, err)
	})

	t.Run("empty message rejected before upstream call", func(t *testing.T) {
		client := mocks.NewAIClient(t)
		svc := chat.NewService(client)
		_, err := svc.Chat(context.Background(), "   ", "session-1")
		require.ErrorIs(t, err, chat.ErrChatMessageRequired)
	})

	t.Run("overlong message rejected", func(t *testing.T) {
		client := mocks.NewAIClient(t)
		svc := chat.NewService(client)
		_, err := svc.Chat(context.Background(), strings.Repeat("a", chat.ChatMessageMaxLen+1), "session-1")
		require.ErrorIs(t, err, chat.ErrChatMessageTooLong)
	})

	t.Run("exactly max length accepted", func(t *testing.T) {
		client := mocks.NewAIClient(t)
		client.On("Complete", mock.Anything, mock.Anything, false).Return("hai", nil)
		svc := chat.NewService(client)
		_, err := svc.Chat(context.Background(), strings.Repeat("a", chat.ChatMessageMaxLen), "session-1")
		require.NoError(t, err)
	})

	t.Run("propagates upstream error", func(t *testing.T) {
		client := mocks.NewAIClient(t)
		client.On("Complete", mock.Anything, mock.Anything, false).Return("", nexxa.ErrUpstreamUnavailable)

		svc := chat.NewService(client)
		_, err := svc.Chat(context.Background(), "halo", "session-1")
		require.ErrorIs(t, err, nexxa.ErrUpstreamUnavailable)
	})
}

func TestService_ChatWithRetrieval(t *testing.T) {
	t.Run("grounds answer with retrieved chunks", func(t *testing.T) {
		client := mocks.NewAIClient(t)
		kb := chatmocks.NewKnowledgeRepository(t)
		embedder := chatmocks.NewEmbedder(t)

		var gotSystem string
		client.On("Complete", mock.Anything, mock.MatchedBy(func(msgs []nexxa.Message) bool {
			gotSystem = msgs[0].Content
			return true
		}), false).Return("informasi PPDB", nil)
		embedder.On("Embed", mock.Anything, []string{"informasi ppdb"}).Return([][]float32{{0.1, 0.2, 0.3}}, nil)
		kb.On("Search", mock.Anything, []float32{0.1, 0.2, 0.3}, 5).Return([]chat.Chunk{
			{DocumentTitle: "PPDB", Content: "PPDB dibuka Januari."},
		}, nil)

		svc := chat.NewService(client).WithRetrieval(kb, embedder, 5)
		got, err := svc.Chat(context.Background(), "informasi ppdb", "session-1")
		require.NoError(t, err)
		require.Equal(t, "informasi PPDB", got.Output)
		require.Contains(t, gotSystem, "PPDB dibuka Januari.")
	})

	t.Run("embedding failure degrades to no context", func(t *testing.T) {
		client := mocks.NewAIClient(t)
		kb := chatmocks.NewKnowledgeRepository(t)
		embedder := chatmocks.NewEmbedder(t)

		var gotSystem string
		client.On("Complete", mock.Anything, mock.MatchedBy(func(msgs []nexxa.Message) bool {
			gotSystem = msgs[0].Content
			return true
		}), false).Return("hai", nil)
		embedder.On("Embed", mock.Anything, []string{"halo"}).Return(nil, nexxa.ErrUpstreamUnavailable)

		svc := chat.NewService(client).WithRetrieval(kb, embedder, 5)
		_, err := svc.Chat(context.Background(), "halo", "session-1")
		require.NoError(t, err)
		require.NotContains(t, gotSystem, "Knowledge context:")
	})
}