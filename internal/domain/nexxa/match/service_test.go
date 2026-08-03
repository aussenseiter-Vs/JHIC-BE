package match_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/match"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/match/content"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService_NexxaMatch(t *testing.T) {
	valid := []string{"a", "b", "c", "d", "e", "f", "g", "h"}

	t.Run("success forwards normalized answers", func(t *testing.T) {
		client := mocks.NewN8NClient(t)
		client.On("NexxaMatch", mock.Anything, mock.MatchedBy(func(a []string) bool {
			if len(a) != content.NexxaAnswerCount {
				return false
			}
			for i, v := range a {
				if v != valid[i] || strings.TrimSpace(v) != v {
					return false
				}
			}
			return true
		})).Return(`{"nama_jurusan":"PPLG","alasan":"cocok","persentase_pplg":60,"persentase_akuntansi":30,"persentase_hotel":10}`, nil)

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
		client := mocks.NewN8NClient(t)
		client.On("NexxaMatch", mock.Anything, mock.Anything).Return("not json at all", nil)

		svc := match.NewService(client)
		_, err := svc.NexxaMatch(context.Background(), valid)
		require.ErrorIs(t, err, match.ErrNexxaOutputInvalid)
	})

	t.Run("wrong answer count rejected", func(t *testing.T) {
		for _, answers := range [][]string{
			{"a", "b", "c", "d", "e", "f", "g"},
			append(valid, "i"),
		} {
			client := mocks.NewN8NClient(t)
			svc := match.NewService(client)
			_, err := svc.NexxaMatch(context.Background(), answers)
			require.ErrorIs(t, err, match.ErrAnswersRequired)
		}
	})

	t.Run("empty answer rejected", func(t *testing.T) {
		client := mocks.NewN8NClient(t)
		svc := match.NewService(client)
		_, err := svc.NexxaMatch(context.Background(), []string{"a", "b", "  ", "d", "e", "f", "g", "h"})
		require.ErrorIs(t, err, match.ErrAnswersRequired)
	})

	t.Run("overlong answer rejected", func(t *testing.T) {
		client := mocks.NewN8NClient(t)
		svc := match.NewService(client)
		answers := append([]string(nil), valid...)
		answers[0] = strings.Repeat("a", content.NexxaAnswerMaxLen+1)
		_, err := svc.NexxaMatch(context.Background(), answers)
		require.ErrorIs(t, err, match.ErrAnswerTooLong)
	})
}