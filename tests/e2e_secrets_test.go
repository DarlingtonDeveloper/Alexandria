//go:build integration

// E2E integration tests for the Alexandria secrets API.
// Run with: go test ./tests/ -tags=integration -run TestE2E -v
// Requires a running Alexandria instance (default: http://127.0.0.1:8500).
// Override with ALEXANDRIA_URL env var.
//
// Each test uses a unique X-Agent-ID to avoid the per-agent rate limit (10 req/min).
// Secrets are scoped to ["*"] (wildcard) so any agent can access them.

package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func alexandriaURL() string {
	if url := os.Getenv("ALEXANDRIA_URL"); url != "" {
		return url
	}
	return "http://127.0.0.1:8500"
}

// e2eRequest is a helper that builds and executes an HTTP request with agent header.
func e2eRequest(t *testing.T, method, url, agentID, body string) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("X-Agent-ID", agentID)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, url, err)
	}
	return resp
}

func TestE2E_SecretLifecycle(t *testing.T) {
	baseURL := alexandriaURL()
	agent := "e2e-lifecycle"
	secretName := "e2e-test-lifecycle"
	secretValue := "e2e-test-value-12345"

	// 1. Create (scoped to wildcard so our agent can access)
	body := fmt.Sprintf(`{"name":%q,"value":%q,"description":"E2E lifecycle test","scope":["*"]}`, secretName, secretValue)
	resp := e2eRequest(t, "POST", baseURL+"/api/v1/secrets", agent, body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}

	defer func() {
		resp := e2eRequest(t, "DELETE", baseURL+"/api/v1/secrets/"+secretName, agent, "")
		resp.Body.Close()
	}()

	// 2. Read — verify decrypted value matches
	resp = e2eRequest(t, "GET", baseURL+"/api/v1/secrets/"+secretName, agent, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("read: expected 200, got %d: %s", resp.StatusCode, b)
	}

	var readResult struct {
		Data struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&readResult)
	if readResult.Data.Value != secretValue {
		t.Errorf("read: expected value %q, got %q", secretValue, readResult.Data.Value)
	}
	if readResult.Data.Name != secretName {
		t.Errorf("read: expected name %q, got %q", secretName, readResult.Data.Name)
	}

	// 3. Update
	resp = e2eRequest(t, "PUT", baseURL+"/api/v1/secrets/"+secretName, agent, `{"value":"updated-e2e-value"}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: expected 200, got %d", resp.StatusCode)
	}

	// 4. Read updated value
	resp = e2eRequest(t, "GET", baseURL+"/api/v1/secrets/"+secretName, agent, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("read-after-update: expected 200, got %d: %s", resp.StatusCode, b)
	}

	_ = json.NewDecoder(resp.Body).Decode(&readResult)
	if readResult.Data.Value != "updated-e2e-value" {
		t.Errorf("read-after-update: expected 'updated-e2e-value', got %q", readResult.Data.Value)
	}

	// 5. Delete
	resp = e2eRequest(t, "DELETE", baseURL+"/api/v1/secrets/"+secretName, agent, "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d", resp.StatusCode)
	}

	// 6. Verify deleted — should 404
	resp = e2eRequest(t, "GET", baseURL+"/api/v1/secrets/"+secretName, agent, "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("read-after-delete: expected 404, got %d", resp.StatusCode)
	}
}

func TestE2E_SecretAccessDenied(t *testing.T) {
	baseURL := alexandriaURL()
	setupAgent := "e2e-access-setup"
	secretName := "e2e-restricted-secret"

	// Create with empty scope (admin-only via CanAccess)
	body := fmt.Sprintf(`{"name":%q,"value":"restricted-value","scope":[]}`, secretName)
	resp := e2eRequest(t, "POST", baseURL+"/api/v1/secrets", setupAgent, body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}

	defer func() {
		resp := e2eRequest(t, "DELETE", baseURL+"/api/v1/secrets/"+secretName, "warren", "")
		resp.Body.Close()
	}()

	// Non-admin agent without scope access should get 403
	resp = e2eRequest(t, "GET", baseURL+"/api/v1/secrets/"+secretName, "e2e-unauthorized", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for unauthorized agent, got %d", resp.StatusCode)
	}

	// Admin (warren) should still have access
	resp = e2eRequest(t, "GET", baseURL+"/api/v1/secrets/"+secretName, "warren", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for admin, got %d", resp.StatusCode)
	}
}

func TestE2E_SecretRotation(t *testing.T) {
	baseURL := alexandriaURL()
	agent := "e2e-rotation"
	secretName := "e2e-rotate-secret"

	// Create with wildcard scope
	body := fmt.Sprintf(`{"name":%q,"value":"original-value","scope":["*"]}`, secretName)
	resp := e2eRequest(t, "POST", baseURL+"/api/v1/secrets", agent, body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}

	defer func() {
		resp := e2eRequest(t, "DELETE", baseURL+"/api/v1/secrets/"+secretName, agent, "")
		resp.Body.Close()
	}()

	// Rotate
	resp = e2eRequest(t, "POST", baseURL+"/api/v1/secrets/"+secretName+"/rotate", agent, `{"new_value":"rotated-value"}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rotate: expected 200, got %d", resp.StatusCode)
	}

	// Read — should return rotated value
	resp = e2eRequest(t, "GET", baseURL+"/api/v1/secrets/"+secretName, agent, "")
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Value string `json:"value"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if result.Data.Value != "rotated-value" {
		t.Errorf("expected 'rotated-value', got %q", result.Data.Value)
	}
}

func TestE2E_HealthEndpoint(t *testing.T) {
	baseURL := alexandriaURL()

	resp, err := http.Get(baseURL + "/api/v1/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health: expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got %v", result["status"])
	}
}
