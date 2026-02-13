//go:build integration

// E2E integration tests for Alexandria identity resolution and semantic APIs.
// Run with: go test ./tests/ -tags=integration -run TestE2E_Identity -v
// Requires a running Alexandria instance (default: http://127.0.0.1:8500).
// Override with ALEXANDRIA_URL env var.
//
// The 003_context_graph_merge migration must be applied before running these tests.

package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

// TestE2E_IdentityResolveCreate tests creating a new entity via identity resolution.
func TestE2E_IdentityResolveCreate(t *testing.T) {
	baseURL := alexandriaURL()
	agent := "e2e-identity-resolve"

	// Resolve a new alias — should create entity + alias
	body := `{"alias_type":"email","alias_value":"e2e-resolve@example.com","entity_type":"person","display_name":"E2E Resolve","source":"e2e-test"}`
	resp := e2eRequest(t, "POST", baseURL+"/api/v1/identity/resolve", agent, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("resolve: expected 200, got %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Data struct {
			EntityID string `json:"entity_id"`
			AliasID  string `json:"alias_id"`
			Outcome  string `json:"outcome"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if result.Data.Outcome != "created" {
		t.Errorf("expected outcome 'created', got '%s'", result.Data.Outcome)
	}
	if result.Data.EntityID == "" {
		t.Error("expected non-empty entity_id")
	}
	if result.Data.AliasID == "" {
		t.Error("expected non-empty alias_id")
	}

	// Cleanup: we can't easily delete identity entities, but they won't interfere
	// with other tests due to unique alias values
}

// TestE2E_IdentityResolveMatch tests matching an existing alias.
func TestE2E_IdentityResolveMatch(t *testing.T) {
	baseURL := alexandriaURL()
	agent := "e2e-identity-match"

	// First resolve — creates entity
	body := `{"alias_type":"email","alias_value":"e2e-match@example.com","entity_type":"person","display_name":"E2E Match","source":"e2e-test"}`
	resp := e2eRequest(t, "POST", baseURL+"/api/v1/identity/resolve", agent, body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first resolve: expected 200, got %d", resp.StatusCode)
	}

	// Second resolve — should match existing
	resp = e2eRequest(t, "POST", baseURL+"/api/v1/identity/resolve", agent, body)
	defer resp.Body.Close()

	var result struct {
		Data struct {
			EntityID string `json:"entity_id"`
			Outcome  string `json:"outcome"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if result.Data.Outcome != "matched" {
		t.Errorf("expected outcome 'matched', got '%s'", result.Data.Outcome)
	}
}

// TestE2E_IdentityResolveValidation tests validation errors.
func TestE2E_IdentityResolveValidation(t *testing.T) {
	baseURL := alexandriaURL()
	agent := "e2e-identity-validation"

	// Missing required fields
	body := `{"alias_type":"email"}`
	resp := e2eRequest(t, "POST", baseURL+"/api/v1/identity/resolve", agent, body)
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Error("expected error for incomplete resolve request")
	}
}

// TestE2E_IdentityEntityLookup tests the entity detail endpoint.
func TestE2E_IdentityEntityLookup(t *testing.T) {
	baseURL := alexandriaURL()
	agent := "e2e-identity-lookup"

	// Create an entity via resolve
	body := `{"alias_type":"email","alias_value":"e2e-lookup@example.com","entity_type":"person","display_name":"E2E Lookup","source":"e2e-test"}`
	resp := e2eRequest(t, "POST", baseURL+"/api/v1/identity/resolve", agent, body)

	var resolveResult struct {
		Data struct {
			EntityID string `json:"entity_id"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&resolveResult)
	resp.Body.Close()

	if resolveResult.Data.EntityID == "" {
		t.Fatal("no entity_id from resolve")
	}

	// Lookup the entity
	resp = e2eRequest(t, "GET", baseURL+"/api/v1/identity/entities/"+resolveResult.Data.EntityID, agent, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("lookup: expected 200, got %d: %s", resp.StatusCode, b)
	}

	var lookupResult struct {
		Data struct {
			Entity struct {
				ID          string `json:"id"`
				Type        string `json:"type"`
				DisplayName string `json:"display_name"`
			} `json:"entity"`
			Aliases  []any `json:"aliases"`
			EdgesFrom []any `json:"edges_from"`
			EdgesTo   []any `json:"edges_to"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&lookupResult)

	if lookupResult.Data.Entity.ID != resolveResult.Data.EntityID {
		t.Errorf("entity ID mismatch: %s vs %s", lookupResult.Data.Entity.ID, resolveResult.Data.EntityID)
	}
	if lookupResult.Data.Entity.Type != "person" {
		t.Errorf("expected type 'person', got '%s'", lookupResult.Data.Entity.Type)
	}
	if lookupResult.Data.Entity.DisplayName != "E2E Lookup" {
		t.Errorf("expected display_name 'E2E Lookup', got '%s'", lookupResult.Data.Entity.DisplayName)
	}
	if len(lookupResult.Data.Aliases) == 0 {
		t.Error("expected at least one alias")
	}
}

// TestE2E_IdentityPending tests the pending reviews endpoint.
func TestE2E_IdentityPending(t *testing.T) {
	baseURL := alexandriaURL()
	agent := "e2e-identity-pending"

	resp := e2eRequest(t, "GET", baseURL+"/api/v1/identity/pending", agent, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("pending: expected 200, got %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Data []any `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	// Should return array (possibly empty)
	if result.Data == nil {
		t.Error("expected data to be an array, got nil")
	}
}

// TestE2E_IdentityMerge tests merging two entities.
func TestE2E_IdentityMerge(t *testing.T) {
	baseURL := alexandriaURL()
	agent := "e2e-identity-merge"

	// Create two entities
	body1 := `{"alias_type":"email","alias_value":"e2e-merge-a@example.com","entity_type":"person","display_name":"Merge A","source":"e2e-test"}`
	resp := e2eRequest(t, "POST", baseURL+"/api/v1/identity/resolve", agent, body1)
	var r1 struct {
		Data struct{ EntityID string `json:"entity_id"` } `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&r1)
	resp.Body.Close()

	body2 := `{"alias_type":"email","alias_value":"e2e-merge-b@example.com","entity_type":"person","display_name":"Merge B","source":"e2e-test"}`
	resp = e2eRequest(t, "POST", baseURL+"/api/v1/identity/resolve", agent, body2)
	var r2 struct {
		Data struct{ EntityID string `json:"entity_id"` } `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&r2)
	resp.Body.Close()

	if r1.Data.EntityID == "" || r2.Data.EntityID == "" {
		t.Fatal("failed to create entities for merge test")
	}

	// Merge
	mergeBody := fmt.Sprintf(`{"survivor_id":"%s","merged_id":"%s","approved_by":"e2e-test"}`, r1.Data.EntityID, r2.Data.EntityID)
	resp = e2eRequest(t, "POST", baseURL+"/api/v1/identity/merge", agent, mergeBody)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("merge: expected 200, got %d: %s", resp.StatusCode, b)
	}

	var mergeResult struct {
		Data struct {
			SurvivorID string `json:"survivor_id"`
			MergedID   string `json:"merged_id"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&mergeResult)

	if mergeResult.Data.SurvivorID != r1.Data.EntityID {
		t.Errorf("survivor ID mismatch: %s vs %s", mergeResult.Data.SurvivorID, r1.Data.EntityID)
	}
	if mergeResult.Data.MergedID != r2.Data.EntityID {
		t.Errorf("merged ID mismatch: %s vs %s", mergeResult.Data.MergedID, r2.Data.EntityID)
	}

	// Verify merged entity's alias now points to survivor
	resp = e2eRequest(t, "GET", baseURL+"/api/v1/identity/entities/"+r1.Data.EntityID, agent, "")
	var lookup struct {
		Data struct {
			Aliases []struct {
				AliasValue string `json:"alias_value"`
			} `json:"aliases"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&lookup)
	resp.Body.Close()

	aliasValues := make(map[string]bool)
	for _, a := range lookup.Data.Aliases {
		aliasValues[a.AliasValue] = true
	}
	if !aliasValues["e2e-merge-a@example.com"] {
		t.Error("survivor should have original alias")
	}
	if !aliasValues["e2e-merge-b@example.com"] {
		t.Error("survivor should have merged entity's alias")
	}
}
