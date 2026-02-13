package semantic

import (
	"context"

	"github.com/warrentherabbit/alexandria/internal/embeddings"
)

// EmbeddingProvider computes vector embeddings from text in batches.
type EmbeddingProvider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Model() string
	Dimensions() int
}

// ProviderAdapter bridges Alexandria's single-text embeddings.Provider to the
// batch EmbeddingProvider interface used by the semantic worker.
type ProviderAdapter struct {
	inner embeddings.Provider
}

// NewProviderAdapter wraps an Alexandria embedding provider.
func NewProviderAdapter(inner embeddings.Provider) *ProviderAdapter {
	return &ProviderAdapter{inner: inner}
}

// Embed generates embeddings for each text sequentially using the inner provider.
func (a *ProviderAdapter) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i, text := range texts {
		vec, err := a.inner.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		result[i] = vec.Slice()
	}
	return result, nil
}

// Model returns the model name.
func (a *ProviderAdapter) Model() string { return a.inner.Name() }

// Dimensions returns the embedding dimensions.
func (a *ProviderAdapter) Dimensions() int { return embeddings.Dimensions }
