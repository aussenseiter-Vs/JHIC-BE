package berita_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/berita"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/berita/mocks"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/pkg/id"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService_List(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		expected := []berita.Berita{{ID: id.ID(1), Title: "First"}, {ID: id.ID(2), Title: "Second"}}
		repo.On("List", mock.Anything).Return(expected, nil)

		svc := berita.NewService(repo)
		got, err := svc.List(context.Background())
		require.NoError(t, err)
		require.Equal(t, expected, got)
	})

	t.Run("repository error", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		repo.On("List", mock.Anything).Return(nil, errors.New("db down"))

		svc := berita.NewService(repo)
		_, err := svc.List(context.Background())
		require.EqualError(t, err, "db down")
	})
}

func TestService_ByID(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		expected := &berita.Berita{ID: id.ID(1), Title: "First"}
		repo.On("ByID", mock.Anything, id.ID(1)).Return(expected, nil)

		svc := berita.NewService(repo)
		got, err := svc.ByID(context.Background(), id.ID(1))
		require.NoError(t, err)
		require.Equal(t, expected, got)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		repo.On("ByID", mock.Anything, id.ID(99)).Return((*berita.Berita)(nil), nil)

		svc := berita.NewService(repo)
		got, err := svc.ByID(context.Background(), id.ID(99))
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("repository error", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		repo.On("ByID", mock.Anything, id.ID(1)).Return((*berita.Berita)(nil), errors.New("db down"))

		svc := berita.NewService(repo)
		_, err := svc.ByID(context.Background(), id.ID(1))
		require.EqualError(t, err, "db down")
	})
}

func TestService_Create(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		repo.On("Create", mock.Anything, mock.MatchedBy(func(b *berita.Berita) bool {
			return b.ID != 0 && b.AuthorID == id.ID(10) && b.Title == "News" && b.Content == "Body" && b.IsAchievement && !b.CreatedAt.IsZero() && !b.UpdatedAt.IsZero()
		})).Return(nil)

		svc := berita.NewService(repo)
		got, err := svc.Create(context.Background(), id.ID(10), "News", "Body", true)
		require.NoError(t, err)
		require.NotEmpty(t, got.ID)
		require.Equal(t, id.ID(10), got.AuthorID)
		require.Equal(t, "News", got.Title)
		require.True(t, got.IsAchievement)
	})

	t.Run("non achievement", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		repo.On("Create", mock.Anything, mock.MatchedBy(func(b *berita.Berita) bool {
			return !b.IsAchievement
		})).Return(nil)

		svc := berita.NewService(repo)
		got, err := svc.Create(context.Background(), id.ID(10), "News", "Body", false)
		require.NoError(t, err)
		require.False(t, got.IsAchievement)
	})

	t.Run("empty content rejected", func(t *testing.T) {
		repo := mocks.NewRepository(t)
		svc := berita.NewService(repo)
		_, err := svc.Create(context.Background(), id.ID(10), "News", "   ", false)
		require.ErrorIs(t, err, berita.ErrContentRequired)
	})

	t.Run("content too large rejected", func(t *testing.T) {
		repo := mocks.NewRepository(t)
		svc := berita.NewService(repo)
		_, err := svc.Create(context.Background(), id.ID(10), "News", strings.Repeat("x", 100*1024+1), false)
		require.ErrorIs(t, err, berita.ErrContentTooLarge)
	})

	t.Run("repository error", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		repo.On("Create", mock.Anything, mock.Anything).Return(errors.New("insert failed"))

		svc := berita.NewService(repo)
		_, err := svc.Create(context.Background(), id.ID(10), "News", "Body", false)
		require.EqualError(t, err, "insert failed")
	})
}

func TestService_Update(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		existing := &berita.Berita{ID: id.ID(1), AuthorID: id.ID(10), Title: "Old", Content: "Old body"}
		repo.On("ByID", mock.Anything, id.ID(1)).Return(existing, nil)
		repo.On("Update", mock.Anything, mock.MatchedBy(func(b *berita.Berita) bool {
			return b.ID == id.ID(1) && b.Title == "New" && b.Content == "New body" && b.IsAchievement && !b.UpdatedAt.IsZero()
		})).Return(nil)

		svc := berita.NewService(repo)
		got, err := svc.Update(context.Background(), id.ID(1), id.ID(10), "New", "New body", true)
		require.NoError(t, err)
		require.Equal(t, "New", got.Title)
		require.True(t, got.IsAchievement)
	})

	t.Run("berita not found", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		repo.On("ByID", mock.Anything, id.ID(99)).Return((*berita.Berita)(nil), nil)

		svc := berita.NewService(repo)
		_, err := svc.Update(context.Background(), id.ID(99), id.ID(10), "New", "Body", false)
		require.EqualError(t, err, "berita not found")
	})

	t.Run("forbidden not author", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		existing := &berita.Berita{ID: id.ID(1), AuthorID: id.ID(10), Title: "Old"}
		repo.On("ByID", mock.Anything, id.ID(1)).Return(existing, nil)

		svc := berita.NewService(repo)
		_, err := svc.Update(context.Background(), id.ID(1), id.ID(11), "New", "Body", false)
		require.EqualError(t, err, "forbidden: not the author")
	})

	t.Run("by id error", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		repo.On("ByID", mock.Anything, id.ID(1)).Return((*berita.Berita)(nil), errors.New("db down"))

		svc := berita.NewService(repo)
		_, err := svc.Update(context.Background(), id.ID(1), id.ID(10), "New", "Body", false)
		require.EqualError(t, err, "db down")
	})

	t.Run("update error", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		existing := &berita.Berita{ID: id.ID(1), AuthorID: id.ID(10)}
		repo.On("ByID", mock.Anything, id.ID(1)).Return(existing, nil)
		repo.On("Update", mock.Anything, mock.Anything).Return(errors.New("update failed"))

		svc := berita.NewService(repo)
		_, err := svc.Update(context.Background(), id.ID(1), id.ID(10), "New", "Body", false)
		require.EqualError(t, err, "update failed")
	})

	t.Run("empty content rejected", func(t *testing.T) {
		repo := mocks.NewRepository(t)
		svc := berita.NewService(repo)
		_, err := svc.Update(context.Background(), id.ID(1), id.ID(10), "New", "", false)
		require.ErrorIs(t, err, berita.ErrContentRequired)
	})

	t.Run("content too large rejected", func(t *testing.T) {
		repo := mocks.NewRepository(t)
		svc := berita.NewService(repo)
		_, err := svc.Update(context.Background(), id.ID(1), id.ID(10), "New", strings.Repeat("x", 100*1024+1), false)
		require.ErrorIs(t, err, berita.ErrContentTooLarge)
	})
}

func TestService_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		existing := &berita.Berita{ID: id.ID(1), AuthorID: id.ID(10)}
		repo.On("ByID", mock.Anything, id.ID(1)).Return(existing, nil)
		repo.On("Delete", mock.Anything, id.ID(1)).Return(nil)

		svc := berita.NewService(repo)
		require.NoError(t, svc.Delete(context.Background(), id.ID(1), id.ID(10)))
	})

	t.Run("berita not found", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		repo.On("ByID", mock.Anything, id.ID(99)).Return((*berita.Berita)(nil), nil)

		svc := berita.NewService(repo)
		err := svc.Delete(context.Background(), id.ID(99), id.ID(10))
		require.EqualError(t, err, "berita not found")
	})

	t.Run("forbidden not author", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		existing := &berita.Berita{ID: id.ID(1), AuthorID: id.ID(10)}
		repo.On("ByID", mock.Anything, id.ID(1)).Return(existing, nil)

		svc := berita.NewService(repo)
		err := svc.Delete(context.Background(), id.ID(1), id.ID(11))
		require.EqualError(t, err, "forbidden: not the author")
	})

	t.Run("by id error", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		repo.On("ByID", mock.Anything, id.ID(1)).Return((*berita.Berita)(nil), errors.New("db down"))

		svc := berita.NewService(repo)
		err := svc.Delete(context.Background(), id.ID(1), id.ID(10))
		require.EqualError(t, err, "db down")
	})

	t.Run("delete error", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		existing := &berita.Berita{ID: id.ID(1), AuthorID: id.ID(10)}
		repo.On("ByID", mock.Anything, id.ID(1)).Return(existing, nil)
		repo.On("Delete", mock.Anything, id.ID(1)).Return(errors.New("delete failed"))

		svc := berita.NewService(repo)
		err := svc.Delete(context.Background(), id.ID(1), id.ID(10))
		require.EqualError(t, err, "delete failed")
	})
}

func TestService_SetImage(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		existing := &berita.Berita{ID: id.ID(1), AuthorID: id.ID(10)}
		repo.On("ByID", mock.Anything, id.ID(1)).Return(existing, nil)
		repo.On("Update", mock.Anything, mock.MatchedBy(func(b *berita.Berita) bool {
			return b.ID == id.ID(1) && b.ImageURL == "berita/1/photo.png" && !b.UpdatedAt.IsZero()
		})).Return(nil)

		svc := berita.NewService(repo)
		got, err := svc.SetImage(context.Background(), id.ID(1), id.ID(10), "berita/1/photo.png")
		require.NoError(t, err)
		require.Equal(t, "berita/1/photo.png", got.ImageURL)
	})

	t.Run("berita not found", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		repo.On("ByID", mock.Anything, id.ID(99)).Return((*berita.Berita)(nil), nil)

		svc := berita.NewService(repo)
		_, err := svc.SetImage(context.Background(), id.ID(99), id.ID(10), "berita/x.png")
		require.EqualError(t, err, "berita not found")
	})

	t.Run("forbidden not author", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		existing := &berita.Berita{ID: id.ID(1), AuthorID: id.ID(10)}
		repo.On("ByID", mock.Anything, id.ID(1)).Return(existing, nil)

		svc := berita.NewService(repo)
		_, err := svc.SetImage(context.Background(), id.ID(1), id.ID(11), "berita/1/x.png")
		require.EqualError(t, err, "forbidden: not the author")
	})

	t.Run("update error", func(t *testing.T) {
		repo := mocks.NewRepository(t)

		existing := &berita.Berita{ID: id.ID(1), AuthorID: id.ID(10)}
		repo.On("ByID", mock.Anything, id.ID(1)).Return(existing, nil)
		repo.On("Update", mock.Anything, mock.Anything).Return(errors.New("update failed"))

		svc := berita.NewService(repo)
		_, err := svc.SetImage(context.Background(), id.ID(1), id.ID(10), "berita/1/x.png")
		require.EqualError(t, err, "update failed")
	})
}
