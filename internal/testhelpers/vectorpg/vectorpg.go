// Package vectorpg boots a throwaway pgvector-enabled Postgres container for
// integration and end-to-end tests.
package vectorpg

import (
	"context"

	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func Run(ctx context.Context) (*postgres.PostgresContainer, error) {
	return postgres.Run(ctx, "pgvector/pgvector:pg16",
		postgres.WithDatabase("jhic"),
		postgres.WithUsername("jhic"),
		postgres.WithPassword("jhic"),
		postgres.BasicWaitStrategies(),
	)
}