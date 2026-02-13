//go:build integration

// E2E integration tests for Alexandria semantic layer APIs.
// Run with: go test ./tests/ -tags=integration -run TestE2E_Semantic -v
// Requires a running Alexandria instance (default: http://127.0.0.1:8500).
// Override with ALEXANDRIA_URL env var.
//
// The 003_context_graph_merge migration must be applied before running these tests.

package tests

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// TestE2E_SemanticStatus tests the semantic status endpoint.
func TestE2E_SemanticStatus(t *testing.T) {
	baseURL := alexandriaURL()
	agent := "e2e-semantic-status"

	resp := e2eRequest(t, "GET", baseURL+"/api/v1/semantic/status", agent, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: expected 200, got %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Data struct {
			EntitiesTotal    int `json:"entities_total"`
			EntitiesEmbedded int `json:"entities_embedded"`
			ClustersActive   int `json:"clusters_active"`
			ProposalsPending int `json:"proposals_pending"`
			EmbeddingGap     int `json:"embedding_gap"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)

	// Counts should be non-negative
	if result.Data.EntitiesTotal < 0 {
		t.Errorf("entities_total should be >= 0, got %d", result.Data.EntitiesTotal)
	}
	if result.Data.EntitiesEmbedded < 0 {
		t.Errorf("entities_embedded should be >= 0, got %d", result.Data.EntitiesEmbedded)
	}
	if result.Data.ClustersActive < 0 {
		t.Errorf("clusters_active should be >= 0, got %d", result.Data.ClustersActive)
	}

	// EmbeddingGap should be total - embedded
	expectedGap := result.Data.EntitiesTotal - result.Data.EntitiesEmbedded
	if result.Data.EmbeddingGap != expectedGap {
		t.Errorf("embedding_gap should be %d, got %d", expectedGap, result.Data.EmbeddingGap)
	}
}

// TestE2E_SemanticClusters tests the clusters list endpoint.
func TestE2E_SemanticClusters(t *testing.T) {
	baseURL := alexandriaURL()
	agent := "e2e-semantic-clusters"

	resp := e2eRequest(t, "GET", baseURL+"/api/v1/semantic/clusters", agent, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("clusters: expected 200, got %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Data []any `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)

	// Should return array (possibly empty)
	if result.Data == nil {
		t.Error("expected data to be an array, got nil")
	}
}

// TestE2E_SemanticProposals tests the merge proposals list endpoint.
func TestE2E_SemanticProposals(t *testing.T) {
	baseURL := alexandriaURL()
	agent := "e2e-semantic-proposals"

	resp := e2eRequest(t, "GET", baseURL+"/api/v1/semantic/proposals", agent, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("proposals: expected 200, got %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Data []any `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)

	if result.Data == nil {
		t.Error("expected data to be an array, got nil")
	}
}

// TestE2E_SemanticSimilar_InvalidID tests the similar endpoint with an invalid ID.
func TestE2E_SemanticSimilar_InvalidID(t *testing.T) {
	baseURL := alexandriaURL()
	agent := "e2e-semantic-similar-invalid"

	resp := e2eRequest(t, "GET", baseURL+"/api/v1/semantic/similar/not-a-uuid", agent, "")
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d", resp.StatusCode)
	}
}

// TestE2E_SemanticClusterMembers_InvalidID tests the cluster members endpoint with invalid ID.
func TestE2E_SemanticClusterMembers_InvalidID(t *testing.T) {
	baseURL := alexandriaURL()
	agent := "e2e-semantic-members-invalid"

	resp := e2eRequest(t, "GET", baseURL+"/api/v1/semantic/clusters/not-a-uuid/members", agent, "")
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d", resp.StatusCode)
	}
}

// TestE2E_GraphEntities tests that the graph entities endpoint still works
// after the schema upgrade (backward compatibility).
func TestE2E_GraphEntities(t *testing.T) {
	baseURL := alexandriaURL()
	agent := "e2e-graph-compat"

	resp := e2eRequest(t, "GET", baseURL+"/api/v1/graph/entities", agent, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("graph entities: expected 200, got %d: %s", resp.StatusCode, b)
	}
}
