package berita

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/pkg/id"
)

const maxContentBytes = 100 * 1024

var (
	ErrContentRequired = errors.New("content is required")
	ErrContentTooLarge = errors.New("content exceeds the 100 KB limit")
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context) ([]Berita, error) {
	return s.repo.List(ctx)
}

func (s *Service) ByID(ctx context.Context, id id.ID) (*Berita, error) {
	return s.repo.ByID(ctx, id)
}

func validateContent(content string) error {
	if strings.TrimSpace(content) == "" {
		return ErrContentRequired
	}
	if len(content) > maxContentBytes {
		return ErrContentTooLarge
	}
	return nil
}

func (s *Service) Create(ctx context.Context, authorID id.ID, title, content string, isAchievement bool) (*Berita, error) {
	if err := validateContent(content); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	b := &Berita{
		ID:            id.New(),
		AuthorID:      authorID,
		Title:         title,
		Content:       content,
		IsAchievement: isAchievement,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.repo.Create(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *Service) Update(ctx context.Context, id, callerID id.ID, title, content string, isAchievement bool) (*Berita, error) {
	if err := validateContent(content); err != nil {
		return nil, err
	}
	b, err := s.repo.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, fmt.Errorf("berita not found")
	}
	if b.AuthorID != callerID {
		return nil, fmt.Errorf("forbidden: not the author")
	}
	b.Title = title
	b.Content = content
	b.IsAchievement = isAchievement
	b.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *Service) Delete(ctx context.Context, id, callerID id.ID) error {
	b, err := s.repo.ByID(ctx, id)
	if err != nil {
		return err
	}
	if b == nil {
		return fmt.Errorf("berita not found")
	}
	if b.AuthorID != callerID {
		return fmt.Errorf("forbidden: not the author")
	}
	return s.repo.Delete(ctx, id)
}

func (s *Service) SetImage(ctx context.Context, id, callerID id.ID, imageURL string) (*Berita, error) {
	b, err := s.repo.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, fmt.Errorf("berita not found")
	}
	if b.AuthorID != callerID {
		return nil, fmt.Errorf("forbidden: not the author")
	}
	b.ImageURL = imageURL
	b.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}
