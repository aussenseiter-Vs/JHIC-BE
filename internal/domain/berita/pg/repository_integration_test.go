//go:build integration

package pg_test

import (
	"context"
	"testing"
	"time"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/berita"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/berita/pg"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/pkg/id"
	"github.com/stretchr/testify/require"
)

func TestRepository_CreateAndByID(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := pg.NewRepository(pool)

	seedUser(t, id.ID(1), "a@example.com")

	now := time.Now().UTC()
	b := &berita.Berita{
		ID:            id.ID(100),
		AuthorID:      id.ID(1),
		Title:         "Title",
		Content:       "Content",
		ImageURL:      "",
		IsAchievement: true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	require.NoError(t, repo.Create(ctx, b))

	got, err := repo.ByID(ctx, b.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, b.Title, got.Title)
	require.Equal(t, b.Content, got.Content)
	require.Equal(t, id.ID(1), got.AuthorID)
	require.Empty(t, got.ImageURL)
	require.True(t, got.IsAchievement)
	require.False(t, got.CreatedAt.IsZero())
}

func TestRepository_ByID_NotFound(t *testing.T) {
	pool := startPostgres(t)
	repo := pg.NewRepository(pool)

	got, err := repo.ByID(context.Background(), id.ID(999))
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestRepository_List(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := pg.NewRepository(pool)

	seedUser(t, id.ID(1), "a@example.com")
	seedUser(t, id.ID(2), "b@example.com")

	now := time.Now().UTC()
	for _, b := range []*berita.Berita{
		{ID: id.ID(100), AuthorID: id.ID(1), Title: "First", Content: "One", CreatedAt: now, UpdatedAt: now},
		{ID: id.ID(101), AuthorID: id.ID(2), Title: "Second", Content: "Two", CreatedAt: now.Add(-time.Minute), UpdatedAt: now},
	} {
		require.NoError(t, repo.Create(ctx, b))
	}

	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.Equal(t, id.ID(100), list[0].ID)
	require.Equal(t, id.ID(101), list[1].ID)
}

func TestRepository_Update(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := pg.NewRepository(pool)

	seedUser(t, id.ID(1), "a@example.com")
	now := time.Now().UTC()
	require.NoError(t, repo.Create(ctx, &berita.Berita{
		ID: id.ID(100), AuthorID: id.ID(1), Title: "Old", Content: "Old body", CreatedAt: now, UpdatedAt: now,
	}))

	got, err := repo.ByID(ctx, id.ID(100))
	require.NoError(t, err)
	got.Title = "New"
	got.Content = "New body"
	got.ImageURL = "berita/100/photo.png"
	got.IsAchievement = true
	got.UpdatedAt = time.Now().UTC()
	require.NoError(t, repo.Update(ctx, got))

	check, err := repo.ByID(ctx, id.ID(100))
	require.NoError(t, err)
	require.Equal(t, "New", check.Title)
	require.Equal(t, "New body", check.Content)
	require.Equal(t, "berita/100/photo.png", check.ImageURL)
	require.True(t, check.IsAchievement)
	require.False(t, check.UpdatedAt.IsZero())
}

func TestRepository_Delete(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := pg.NewRepository(pool)

	seedUser(t, id.ID(1), "a@example.com")
	now := time.Now().UTC()
	require.NoError(t, repo.Create(ctx, &berita.Berita{
		ID: id.ID(100), AuthorID: id.ID(1), Title: "Title", Content: "Body", CreatedAt: now, UpdatedAt: now,
	}))

	require.NoError(t, repo.Delete(ctx, id.ID(100)))

	got, err := repo.ByID(ctx, id.ID(100))
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestRepository_NoConnectionLeak(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	before := pool.Stat().AcquiredConns()

	repo := pg.NewRepository(pool)
	for i := 0; i < 10; i++ {
		_, err := repo.List(ctx)
		require.NoError(t, err)
		_, err = repo.ByID(ctx, id.ID(999))
		require.NoError(t, err)
	}

	require.Equal(t, before, pool.Stat().AcquiredConns())
}
