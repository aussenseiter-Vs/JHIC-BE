//go:build integration

package pg_test

import (
	"context"
	"testing"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/chat/pg"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/pkg/pgvector"
	"github.com/stretchr/testify/require"
)

func vec(dim1, dim2 float32) []float32 {
	v := make([]float32, 384)
	v[0], v[1] = dim1, dim2
	return v
}

func seedChunk(t *testing.T, slug, title, content string, embedding []float32) {
	t.Helper()
	ctx := context.Background()

	var docID int64
	err := testPool.QueryRow(ctx,
		`INSERT INTO kb_documents (slug, title, source_path) VALUES ($1, $2, $3) RETURNING id`,
		slug, title, "kb/"+slug+".md",
	).Scan(&docID)
	require.NoError(t, err)

	_, err = testPool.Exec(ctx,
		`INSERT INTO kb_chunks (document_id, content, embedding) VALUES ($1, $2, $3::vector)`,
		docID, content, pgvector.Literal(embedding),
	)
	require.NoError(t, err)
}

func TestRepository_Search(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := pg.NewRepository(pool)

	seedChunk(t, "pplg", "Jurusan PPLG", "pengembangan perangkat lunak", vec(1, 0))
	seedChunk(t, "perhotelan", "Jurusan Perhotelan", "layanan kamar hotel", vec(0.7071, 0.7071))
	seedChunk(t, "akuntansi", "Jurusan Akuntansi", "pembukuan dan laporan keuangan", vec(0, 1))

	chunks, err := repo.Search(ctx, vec(1, 0), 2)
	require.NoError(t, err)
	require.Len(t, chunks, 2)
	require.Equal(t, "Jurusan PPLG", chunks[0].DocumentTitle)
	require.Equal(t, "pengembangan perangkat lunak", chunks[0].Content)
	require.Equal(t, "Jurusan Perhotelan", chunks[1].DocumentTitle)
}

func TestRepository_Search_EmptyKB(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := pg.NewRepository(pool)

	chunks, err := repo.Search(ctx, vec(1, 0), 5)
	require.NoError(t, err)
	require.Empty(t, chunks)
}

func TestRepository_Search_NoConnectionLeak(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := pg.NewRepository(pool)

	seedChunk(t, "pplg", "Jurusan PPLG", "konten", vec(1, 0))

	before := pool.Stat().AcquiredConns()
	for i := 0; i < 5; i++ {
		_, err := repo.Search(ctx, vec(1, 0), 5)
		require.NoError(t, err)
	}
	require.Equal(t, before, pool.Stat().AcquiredConns())
}