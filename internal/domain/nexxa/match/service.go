package match

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/match/content"
)

type Service struct {
	client nexxa.N8NClient
}

func NewService(client nexxa.N8NClient) *Service {
	return &Service{client: client}
}

func (s *Service) NexxaMatch(ctx context.Context, answers []string) (*NexxaResponse, error) {
	if len(answers) != content.NexxaAnswerCount {
		return nil, ErrAnswersRequired
	}
	normalized := make([]string, content.NexxaAnswerCount)
	for i, a := range answers {
		a = strings.TrimSpace(a)
		if a == "" {
			return nil, ErrAnswersRequired
		}
		if len(a) > content.NexxaAnswerMaxLen {
			return nil, ErrAnswerTooLong
		}
		normalized[i] = a
	}

	raw, err := s.client.NexxaMatch(ctx, normalized)
	if err != nil {
		return nil, err
	}

	data, errs := content.NormalizeNexxaOutput(raw)
	if len(errs) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrNexxaOutputInvalid, errs[0].Message)
	}

	return &NexxaResponse{
		NamaJurusan:         data.NamaJurusan,
		Alasan:              data.Alasan,
		PersentasePPLG:      data.PersentasePPLG,
		PersentaseAkuntansi: data.PersentaseAkuntansi,
		PersentaseHotel:     data.PersentaseHotel,
	}, nil
}

func (s *Service) ValidateNexxaInput(raw map[string]json.RawMessage) (map[string]string, []content.APIError) {
	return content.ValidateNexxaInput(raw)
}

func (s *Service) NormalizeNexxaOutput(raw string) (*content.NormalizeOutputData, []content.APIError) {
	return content.NormalizeNexxaOutput(raw)
}