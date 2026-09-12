//go:build integration

package pg_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/database"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/pkg/id"
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

	if err := database.RunMigrations(ctx, testPool, "../../../../cmd/server/migrations"); err != nil {
		fmt.Printf("migrations: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	testPool.Close()
	_ = container.Terminate(ctx)
	os.Exit(code)
}

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `TRUNCATE sessions, berita, users CASCADE`)
	require.NoError(t, err)
	return testPool
}

func seedUser(t *testing.T, id id.ID, email, name, role string) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, name, role, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW(), NOW())`,
		int64(id), email, "hash", name, role,
	)
	require.NoError(t, err)
}

func seedGuru(t *testing.T, id id.ID, email, position, class, jurusan string) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, name, role, class, jurusan, position, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())`,
		int64(id), email, "hash", "Guru", "guru", class, jurusan, position,
	)
	require.NoError(t, err)
}
