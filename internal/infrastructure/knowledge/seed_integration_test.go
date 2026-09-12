//go:build integration

package knowledge_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/database"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/knowledge"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/testhelpers/vectorpg"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := vectorpg.Run(ctx)
	if err != nil {
		fmt.Printf("start postgres container: %v\n", err)
		os.Exit(1)
	}
	url, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Printf("postgres connection string: %v\n", err)
		os.Exit(1)
	}
	testPool, err = database.Connect(ctx, url)
	if err != nil {
		fmt.Printf("connect: %v\n", err)
		os.Exit(1)
	}
	if err := database.RunMigrations(ctx, testPool, "../../../cmd/server/migrations"); err != nil {
		fmt.Printf("migrations: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	testPool.Close()
	_ = container.Terminate(ctx)
	os.Exit(code)
}

// stubEmbedder deterministically maps each input to a fixed 384-dim vector.
type stubEmbedder struct{}

func (stubEmbedder) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	out := make([][]float32, len(inputs))
	rng := rand.New(rand.NewSource(42))
	for i := range inputs {
		vec := make([]float32, 384)
		for j := range vec {
			vec[j] = rng.Float32()
		}
		out[i] = vec
	}
	return out, nil
}

func TestSeed_IndexesAndReseeds(t *testing.T) {
	ctx := context.Background()
	_, err := testPool.Exec(ctx, `TRUNCATE kb_chunks, kb_documents CASCADE`)
	require.NoError(t, err)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ppdb.md"), []byte(
		"# PPDB\n\nPPDB dibuka setiap tahun.\nPersyaratan dan jadwal diumumkan oleh panitia.",
	), 0o600))

	err = knowledge.Seed(ctx, testPool, knowledge.Config{Dir: dir, Embedder: stubEmbedder{}})
	require.NoError(t, err)

	count, err := knowledge.Count(ctx, testPool)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	var title string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT title FROM kb_documents WHERE slug = 'ppdb'`).Scan(&title))
	require.Equal(t, "PPDB", title)

	// Re-seeding the same doc replaces it without duplicating rows.
	require.NoError(t, knowledge.Seed(ctx, testPool, knowledge.Config{Dir: dir, Embedder: stubEmbedder{}}))
	count, err = knowledge.Count(ctx, testPool)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}