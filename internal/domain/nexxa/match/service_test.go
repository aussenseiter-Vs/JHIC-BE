package match_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/match"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/match/content"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const matchJSON = `{"nama_jurusan":"PPLG","alasan":"cocok","persentase_pplg":60,"persentase_akuntansi":30,"persentase_hotel":10}`

func TestService_NexxaMatch(t *testing.T) {
	valid := []string{"a", "b", "c", "d", "e", "f", "g", "h"}

	t.Run("success sends normalized answers in messages", func(t *testing.T) {
		client := mocks.NewAIClient(t)
		client.On("Complete", mock.Anything, mock.MatchedBy(func(msgs []nexxa.Message) bool {
			if len(msgs) != 2 || msgs[0].Role != "system" || msgs[1].Role != "user" {
				return false
			}
			for i, v := range valid {
				if !strings.Contains(msgs[1].Content, string(rune('a'+i))) {
					return false
				}
				if strings.TrimSpace(v) != v {
					return false
				}
			}
			return true
		}), true).Return(matchJSON, nil)

		svc := match.NewService(client)
		got, err := svc.NexxaMatch(context.Background(), valid)
		require.NoError(t, err)
		require.Equal(t, "PPLG", got.NamaJurusan)
		require.Equal(t, "cocok", got.Alasan)
		require.Equal(t, 60, got.PersentasePPLG)
		require.Equal(t, 30, got.PersentaseAkuntansi)
		require.Equal(t, 10, got.PersentaseHotel)
	})

	t.Run("invalid model output rejected", func(t *testing.T) {
		client := mocks.NewAIClient(t)
		client.On("Complete", mock.Anything, mock.Anything, true).Return("not json at all", nil)

		svc := match.NewService(client)
		_, err := svc.NexxaMatch(context.Background(), valid)
		require.ErrorIs(t, err, match.ErrNexxaOutputInvalid)
	})

	t.Run("wrong answer count rejected before upstream call", func(t *testing.T) {
		for _, answers := range [][]string{
			{"a", "b", "c", "d", "e", "f", "g"},
			append(valid, "i"),
		} {
			client := mocks.NewAIClient(t)
			svc := match.NewService(client)
			_, err := svc.NexxaMatch(context.Background(), answers)
			require.ErrorIs(t, err, match.ErrAnswersRequired)
		}
	})

	t.Run("empty answer rejected", func(t *testing.T) {
		client := mocks.NewAIClient(t)
		svc := match.NewService(client)
		_, err := svc.NexxaMatch(context.Background(), []string{"a", "b", "  ", "d", "e", "f", "g", "h"})
		require.ErrorIs(t, err, match.ErrAnswersRequired)
	})

	t.Run("overlong answer rejected", func(t *testing.T) {
		client := mocks.NewAIClient(t)
		svc := match.NewService(client)
		answers := append([]string(nil), valid...)
		answers[0] = strings.Repeat("a", content.NexxaAnswerMaxLen+1)
		_, err := svc.NexxaMatch(context.Background(), answers)
		require.ErrorIs(t, err, match.ErrAnswerTooLong)
	})
}