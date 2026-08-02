package pg

import (
	"context"
	"fmt"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/berita"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/pkg/id"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) List(ctx context.Context) ([]berita.Berita, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, author_id, title, content, COALESCE(image_url, ''), is_achievement, created_at, updated_at
		 FROM berita ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list berita: %w", err)
	}
	defer rows.Close()

	var list []berita.Berita
	for rows.Next() {
		var b berita.Berita
		if err := rows.Scan(&b.ID, &b.AuthorID, &b.Title, &b.Content, &b.ImageURL, &b.IsAchievement, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan berita: %w", err)
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

func (r *Repository) ByID(ctx context.Context, id id.ID) (*berita.Berita, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, author_id, title, content, COALESCE(image_url, ''), is_achievement, created_at, updated_at
		 FROM berita WHERE id = $1`, id,
	)
	b := &berita.Berita{}
	err := row.Scan(&b.ID, &b.AuthorID, &b.Title, &b.Content, &b.ImageURL, &b.IsAchievement, &b.CreatedAt, &b.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("berita by id: %w", err)
	}
	return b, nil
}

func (r *Repository) Create(ctx context.Context, b *berita.Berita) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO berita (id, author_id, title, content, image_url, is_achievement, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		b.ID, b.AuthorID, b.Title, b.Content, b.ImageURL, b.IsAchievement, b.CreatedAt, b.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create berita: %w", err)
	}
	return nil
}

func (r *Repository) Update(ctx context.Context, b *berita.Berita) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE berita SET title = $2, content = $3, image_url = $4, is_achievement = $5, updated_at = $6
		 WHERE id = $1`,
		b.ID, b.Title, b.Content, b.ImageURL, b.IsAchievement, b.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update berita: %w", err)
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, id id.ID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM berita WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete berita: %w", err)
	}
	return nil
}
