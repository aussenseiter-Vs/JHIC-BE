package pg

import (
	"context"
	"fmt"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/chat"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/pkg/pgvector"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Search(ctx context.Context, query []float32, limit int) ([]chat.Chunk, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT d.title, c.content
		 FROM kb_chunks c
		 JOIN kb_documents d ON d.id = c.document_id
		 ORDER BY c.embedding <=> $1::vector
		 LIMIT $2`,
		pgvector.Literal(query), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search kb: %w", err)
	}
	defer rows.Close()

	var chunks []chat.Chunk
	for rows.Next() {
		var c chat.Chunk
		if err := rows.Scan(&c.DocumentTitle, &c.Content); err != nil {
			return nil, fmt.Errorf("scan kb chunk: %w", err)
		}
		chunks = append(chunks, c)
	}
	return chunks, rows.Err()
}