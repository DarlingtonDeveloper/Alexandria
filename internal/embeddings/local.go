package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	pgvector "github.com/pgvector/pgvector-go"
)

// LocalProvider generates embeddings by calling the local sidecar service.
type LocalProvider struct {
	url    string
	client *http.Client
}

// NewLocalProvider creates a new local embedding provider.
// url should be the base URL of the sidecar, e.g. "http://localhost:8501".
func NewLocalProvider(url string) *LocalProvider {
	return &LocalProvider{
		url:    url,
		client: &http.Client{},
	}
}

// Name returns the provider name.
func (p *LocalProvider) Name() string {
	return "local"
}

type sidecarRequest struct {
	Texts []string `json:"texts"`
}

type sidecarResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed generates an embedding using the local sidecar.
func (p *LocalProvider) Embed(ctx context.Context, text string) (pgvector.Vector, error) {
	body, err := json.Marshal(sidecarRequest{Texts: []string{text}})
	if err != nil {
		return pgvector.Vector{}, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url+"/embed", bytes.NewReader(body))
	if err != nil {
		return pgvector.Vector{}, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return pgvector.Vector{}, fmt.Errorf("calling sidecar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return pgvector.Vector{}, fmt.Errorf("sidecar returned %d: %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return pgvector.Vector{}, fmt.Errorf("reading response: %w", err)
	}

	var result sidecarResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return pgvector.Vector{}, fmt.Errorf("parsing response: %w", err)
	}

	if len(result.Embeddings) == 0 {
		return pgvector.Vector{}, fmt.Errorf("no embeddings returned")
	}

	return pgvector.NewVector(result.Embeddings[0]), nil
}
