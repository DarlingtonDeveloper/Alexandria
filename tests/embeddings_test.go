package tests

import (
	"context"
	"testing"

	"github.com/warrentherabbit/alexandria/internal/embeddings"
)

func TestSimpleProvider_Embed(t *testing.T) {
	p := embeddings.NewSimpleProvider()

	if p.Name() != "simple" {
		t.Errorf("expected name 'simple', got '%s'", p.Name())
	}

	vec, err := p.Embed(context.Background(), "hello world test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(vec.Slice()) != embeddings.Dimensions {
		t.Errorf("expected %d dimensions, got %d", embeddings.Dimensions, len(vec.Slice()))
	}

	// Check normalization: L2 norm should be ~1.0
	var norm float64
	for _, v := range vec.Slice() {
		norm += float64(v) * float64(v)
	}
	if norm < 0.99 || norm > 1.01 {
		t.Errorf("expected L2 norm ~1.0, got %f", norm)
	}
}

func TestSimpleProvider_SimilarTexts(t *testing.T) {
	p := embeddings.NewSimpleProvider()
	ctx := context.Background()

	v1, _ := p.Embed(ctx, "the cat sat on the mat")
	v2, _ := p.Embed(ctx, "the cat sat on the mat")  // identical
	v3, _ := p.Embed(ctx, "quantum physics equations") // very different

	sim12 := cosineSimilarity(v1.Slice(), v2.Slice())
	sim13 := cosineSimilarity(v1.Slice(), v3.Slice())

	if sim12 < 0.99 {
		t.Errorf("identical texts should have similarity ~1.0, got %f", sim12)
	}
	if sim13 >= sim12 {
		t.Errorf("different texts should have lower similarity: same=%f different=%f", sim12, sim13)
	}
}

func TestSimpleProvider_EmptyText(t *testing.T) {
	p := embeddings.NewSimpleProvider()
	vec, err := p.Embed(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty text should produce zero vector
	allZero := true
	for _, v := range vec.Slice() {
		if v != 0 {
			allZero = false
			break
		}
	}
	if !allZero {
		t.Error("empty text should produce zero vector")
	}
}

func cosineSimilarity(a, b []float32) float64 {
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (sqrt(normA) * sqrt(normB))
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 100; i++ {
		z = (z + x/z) / 2
	}
	return z
}
