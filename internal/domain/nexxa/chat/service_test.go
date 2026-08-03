package chat_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/chat"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService_Chat(t *testing.T) {
	t.Run("success forwards message and session", func(t *testing.T) {
		client := mocks.NewN8NClient(t)
		client.On("Chat", mock.Anything, "halo", "session-1").Return(&nexxa.ChatResponse{Output: "hai"}, nil)

		svc := chat.NewService(client)
		got, err := svc.Chat(context.Background(), "halo", "session-1")
		require.NoError(t, err)
		require.Equal(t, "hai", got.Output)
	})

	t.Run("trims input and forwards empty session", func(t *testing.T) {
		client := mocks.NewN8NClient(t)
		client.On("Chat", mock.Anything, "halo", mock.MatchedBy(func(sid string) bool {
			return len(sid) == 32
		})).Return(&nexxa.ChatResponse{Output: "hai"}, nil)

		svc := chat.NewService(client)
		got, err := svc.Chat(context.Background(), "  halo  ", "")
		require.NoError(t, err)
		require.Equal(t, "hai", got.Output)
	})

	t.Run("empty message rejected", func(t *testing.T) {
		client := mocks.NewN8NClient(t)
		svc := chat.NewService(client)
		_, err := svc.Chat(context.Background(), "   ", "session-1")
		require.ErrorIs(t, err, chat.ErrChatMessageRequired)
	})

	t.Run("overlong message rejected", func(t *testing.T) {
		client := mocks.NewN8NClient(t)
		svc := chat.NewService(client)
		_, err := svc.Chat(context.Background(), strings.Repeat("a", chat.ChatMessageMaxLen+1), "session-1")
		require.ErrorIs(t, err, chat.ErrChatMessageTooLong)
	})

	t.Run("exactly max length accepted", func(t *testing.T) {
		client := mocks.NewN8NClient(t)
		client.On("Chat", mock.Anything, mock.MatchedBy(func(s string) bool {
			return len(s) == chat.ChatMessageMaxLen
		}), "session-1").Return(&nexxa.ChatResponse{Output: "hai"}, nil)

		svc := chat.NewService(client)
		_, err := svc.Chat(context.Background(), strings.Repeat("a", chat.ChatMessageMaxLen), "session-1")
		require.NoError(t, err)
	})

	t.Run("propagates upstream error", func(t *testing.T) {
		client := mocks.NewN8NClient(t)
		client.On("Chat", mock.Anything, "halo", "session-1").Return(nil, nexxa.ErrN8NUnavailable)

		svc := chat.NewService(client)
		_, err := svc.Chat(context.Background(), "halo", "session-1")
		require.ErrorIs(t, err, nexxa.ErrN8NUnavailable)
	})
}