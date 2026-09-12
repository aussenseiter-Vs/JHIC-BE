package chat

import "context"

// Chunk is a knowledge-base snippet retrieved for grounding a chat reply.
type Chunk struct {
	DocumentTitle string
	Content       string
}

// KnowledgeRepository retrieves KB chunks most similar to a query embedding.
type KnowledgeRepository interface {
	Search(ctx context.Context, query []float32, limit int) ([]Chunk, error)
}

// Embedder turns text into embeddings compatible with the KB vectors.
type Embedder interface {
	Embed(ctx context.Context, inputs []string) ([][]float32, error)
}