package knowledge

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/pkg/pgvector"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	DefaultBatchSize = 32
	DefaultMaxChunk  = 800
	DefaultOverlap   = 100
)

// Embedder turns text into embeddings compatible with the KB vectors. It is
// satisfied by the LLM infrastructure client.
type Embedder interface {
	Embed(ctx context.Context, inputs []string) ([][]float32, error)
}

type Config struct {
	Dir       string
	Embedder  Embedder
	BatchSize int
	MaxChunk  int
	Overlap   int
}

func withDefaults(cfg Config) Config {
	if cfg.Dir == "" {
		cfg.Dir = "kb"
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultBatchSize
	}
	if cfg.MaxChunk <= 0 {
		cfg.MaxChunk = DefaultMaxChunk
	}
	if cfg.Overlap < 0 {
		cfg.Overlap = DefaultOverlap
	}
	return cfg
}

// Count returns the number of stored KB chunks.
func Count(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	var n int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM kb_chunks`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count kb chunks: %w", err)
	}
	return n, nil
}

// Seed indexes every markdown file under cfg.Dir, replacing each document by
// slug. Documents are chunked, embedded in batches, and written with their
// vectors.
func Seed(ctx context.Context, pool *pgxpool.Pool, cfg Config) error {
	cfg = withDefaults(cfg)
	paths, err := markdownFiles(cfg.Dir)
	if err != nil {
		return fmt.Errorf("find kb files: %w", err)
	}
	for _, p := range paths {
		if err := seedFile(ctx, pool, cfg, p); err != nil {
			return err
		}
	}
	return nil
}

func seedFile(ctx context.Context, pool *pgxpool.Pool, cfg Config, path string) error {
	rel, err := filepath.Rel(cfg.Dir, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	slug := slugify(strings.TrimSuffix(rel, filepath.Ext(rel)))

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read kb file %s: %w", path, err)
	}
	text := strings.TrimSpace(string(data))
	title := documentTitle(text, slug)
	chunks := chunkText(text, cfg.MaxChunk, cfg.Overlap)

	if _, err := pool.Exec(ctx, `DELETE FROM kb_documents WHERE slug = $1`, slug); err != nil {
		return fmt.Errorf("clear kb doc %s: %w", slug, err)
	}
	if len(chunks) == 0 {
		return nil
	}

	var docID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO kb_documents (slug, title, source_path) VALUES ($1, $2, $3) RETURNING id`,
		slug, title, rel,
	).Scan(&docID); err != nil {
		return fmt.Errorf("insert kb doc %s: %w", slug, err)
	}

	for i := 0; i < len(chunks); i += cfg.BatchSize {
		end := i + cfg.BatchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[i:end]
		vecs, err := cfg.Embedder.Embed(ctx, batch)
		if err != nil {
			return fmt.Errorf("embed kb chunks %s: %w", slug, err)
		}
		if len(vecs) != len(batch) {
			return fmt.Errorf("embed kb chunks %s: expected %d vectors, got %d", slug, len(batch), len(vecs))
		}
		for j, vec := range vecs {
			if _, err := pool.Exec(ctx,
				`INSERT INTO kb_chunks (document_id, content, embedding) VALUES ($1, $2, $3::vector)`,
				docID, batch[j], pgvector.Literal(vec),
			); err != nil {
				return fmt.Errorf("insert kb chunk %s: %w", slug, err)
			}
		}
	}
	return nil
}

func markdownFiles(dir string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(p), ".md") {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func documentTitle(text string, slug string) string {
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "#") {
			return strings.TrimSpace(strings.Trim(t, "#"))
		}
	}
	return slug
}

func slugify(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r >= '\u0100':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// chunkText splits markdown text into overlapping chunks. New chunks begin at
// headings whenever the running chunk exceeds maxChars.
func chunkText(text string, maxChars, overlap int) []string {
	if maxChars <= 0 {
		maxChars = DefaultMaxChunk
	}
	if overlap >= maxChars {
		overlap = maxChars / 2
	}

	var chunks []string
	var cur []string
	curLen := 0

	flush := func() {
		s := strings.TrimSpace(strings.Join(cur, "\n"))
		if s != "" {
			chunks = append(chunks, s)
		}
		cur = nil
		curLen = 0
	}

	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" && len(cur) == 0 {
			continue
		}
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "#") && len(cur) > 0 {
			flush()
		}
		for len(line) > maxChars {
			if len(cur) > 0 {
				flush()
			}
			cut := maxChars
			chunk := line[:cut]
			chunks = append(chunks, chunk)
			keep := chunk
			if len(keep) > overlap {
				keep = keep[len(keep)-overlap:]
			}
			line = keep + line[cut:]
		}
		if curLen+len(line)+1 > maxChars && len(cur) > 0 {
			tail := strings.Join(cur, "\n")
			keep := ""
			if len(tail) > overlap {
				keep = tail[len(tail)-overlap:]
			}
			flush()
			if keep != "" {
				cur = append(cur, keep)
				curLen = len(keep)
			}
		}
		cur = append(cur, line)
		curLen += len(line) + 1
	}
	flush()
	return chunks
}