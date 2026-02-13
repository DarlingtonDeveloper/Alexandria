package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MikeSquared-Agency/Alexandria/internal/embeddings"
)

func TestLocalProvider_Embed(t *testing.T) {
	// Create a mock sidecar server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embed" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Texts []string `json:"texts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Return fake embeddings with correct dimensions
		resp := struct {
			Embeddings [][]float32 `json:"embeddings"`
		}{
			Embeddings: make([][]float32, len(req.Texts)),
		}
		for i := range req.Texts {
			vec := make([]float32, embeddings.Dimensions)
			for j := range vec {
				vec[j] = float32(j+i) * 0.001
			}
			resp.Embeddings[i] = vec
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := embeddings.NewLocalProvider(server.URL)

	if p.Name() != "local" {
		t.Errorf("expected name 'local', got '%s'", p.Name())
	}

	vec, err := p.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(vec.Slice()) != embeddings.Dimensions {
		t.Errorf("expected %d dimensions, got %d", embeddings.Dimensions, len(vec.Slice()))
	}
}

func TestLocalProvider_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	p := embeddings.NewLocalProvider(server.URL)
	_, err := p.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for server error response")
	}
}

func TestLocalProvider_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Embeddings [][]float32 `json:"embeddings"`
		}{Embeddings: [][]float32{}})
	}))
	defer server.Close()

	p := embeddings.NewLocalProvider(server.URL)
	_, err := p.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for empty embeddings response")
	}
}
