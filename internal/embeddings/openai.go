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

// OpenAIProvider generates embeddings using OpenAI's API.
type OpenAIProvider struct {
	apiKey string
	model  string
	client *http.Client
}

// NewOpenAIProvider creates a new OpenAI embedding provider.
func NewOpenAIProvider(apiKey, model string) *OpenAIProvider {
	if model == "" {
		model = "text-embedding-3-small"
	}
	return &OpenAIProvider{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{},
	}
}

// Name returns the provider name.
func (p *OpenAIProvider) Name() string {
	return "openai"
}

type openAIRequest struct {
	Input      string `json:"input"`
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions,omitempty"`
}

type openAIResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Embed generates an embedding using the OpenAI API.
func (p *OpenAIProvider) Embed(ctx context.Context, text string) (pgvector.Vector, error) {
	body, err := json.Marshal(openAIRequest{
		Input:      text,
		Model:      p.model,
		Dimensions: Dimensions, // request 384 dims to match local model
	})
	if err != nil {
		return pgvector.Vector{}, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return pgvector.Vector{}, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return pgvector.Vector{}, fmt.Errorf("calling OpenAI: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return pgvector.Vector{}, fmt.Errorf("reading response: %w", err)
	}

	var result openAIResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return pgvector.Vector{}, fmt.Errorf("parsing response: %w", err)
	}

	if result.Error != nil {
		return pgvector.Vector{}, fmt.Errorf("OpenAI error: %s", result.Error.Message)
	}

	if len(result.Data) == 0 {
		return pgvector.Vector{}, fmt.Errorf("no embeddings returned")
	}

	return pgvector.NewVector(result.Data[0].Embedding), nil
}
