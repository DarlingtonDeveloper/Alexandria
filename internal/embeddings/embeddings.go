// Package embeddings provides a swappable interface for text embedding generation.
package embeddings

import (
	"context"

	pgvector "github.com/pgvector/pgvector-go"
)

// Dimensions is the embedding vector size (384 = all-MiniLM-L6-v2).
// OpenAI text-embedding-3-small also supports 384 via the dimensions parameter.
const Dimensions = 384

// Provider generates text embeddings.
type Provider interface {
	// Embed generates an embedding vector for the given text.
	Embed(ctx context.Context, text string) (pgvector.Vector, error)

	// Name returns the provider name for logging.
	Name() string
}
