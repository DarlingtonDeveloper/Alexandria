package embeddings

import (
	"context"
	"hash/fnv"
	"math"
	"strings"

	pgvector "github.com/pgvector/pgvector-go"
)

// SimpleProvider generates embeddings using a simple keyword hashing approach.
// This is a Phase 1 implementation — not semantically meaningful, but deterministic
// and sufficient for basic similarity matching on shared keywords.
type SimpleProvider struct{}

// NewSimpleProvider creates a new SimpleProvider.
func NewSimpleProvider() *SimpleProvider {
	return &SimpleProvider{}
}

// Name returns the provider name.
func (p *SimpleProvider) Name() string {
	return "simple"
}

// Embed generates a pseudo-embedding by hashing words into vector dimensions.
// Words are lowercased, split on whitespace/punctuation, then each word is hashed
// to a dimension index and its contribution is added. The vector is then normalized.
func (p *SimpleProvider) Embed(_ context.Context, text string) (pgvector.Vector, error) {
	vec := make([]float32, Dimensions)

	// Tokenize: lowercase, split on non-alphanumeric
	words := tokenize(text)

	// Hash each word to a dimension and accumulate
	for _, word := range words {
		h := fnv.New64a()
		h.Write([]byte(word))
		idx := h.Sum64() % uint64(Dimensions)
		vec[idx] += 1.0

		// Also add bigram hashes for slightly better similarity
		// by capturing word ordering
	}

	// Add bigrams
	for i := 0; i < len(words)-1; i++ {
		bigram := words[i] + " " + words[i+1]
		h := fnv.New64a()
		h.Write([]byte(bigram))
		idx := h.Sum64() % uint64(Dimensions)
		vec[idx] += 0.5
	}

	// L2 normalize
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	if norm > 0 {
		norm = math.Sqrt(norm)
		for i := range vec {
			vec[i] = float32(float64(vec[i]) / norm)
		}
	}

	return pgvector.NewVector(vec), nil
}

// tokenize splits text into lowercase word tokens.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	// Replace common punctuation with spaces
	for _, c := range ".,;:!?()[]{}\"'`~@#$%^&*+=|\\/<>" {
		text = strings.ReplaceAll(text, string(c), " ")
	}
	fields := strings.Fields(text)
	// Filter very short tokens
	var result []string
	for _, f := range fields {
		if len(f) >= 2 {
			result = append(result, f)
		}
	}
	return result
}
