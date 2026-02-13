//go:build integration

// E2E integration tests for the Alexandria boot context API.
// Run with: go test ./tests/ -tags=integration -run TestE2E_Context -v
// Requires a running Alexandria instance (default: http://127.0.0.1:8500).
// Override with ALEXANDRIA_URL env var.

package tests

import (
	"io"
	"strings"
	"testing"
)

func TestE2E_ContextEndpoint_ReturnsMarkdown(t *testing.T) {
	baseURL := alexandriaURL()

	resp := e2eRequest(t, "GET", baseURL+"/api/v1/context/kai", "kai", "")
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/markdown") {
		t.Errorf("expected Content-Type text/markdown, got %q", ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	md := string(body)

	if !strings.Contains(md, "# Boot Context") {
		t.Error("response missing '# Boot Context' heading")
	}
	if !strings.Contains(md, "Agent: **kai**") {
		t.Error("response missing agent identifier")
	}
}

func TestE2E_ContextEndpoint_DifferentAgents(t *testing.T) {
	baseURL := alexandriaURL()

	agents := []string{"kai", "lily", "scout", "dutybound", "celebrimbor"}
	for _, agent := range agents {
		t.Run(agent, func(t *testing.T) {
			resp := e2eRequest(t, "GET", baseURL+"/api/v1/context/"+agent, agent, "")
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("expected 200 for %s, got %d: %s", agent, resp.StatusCode, body)
			}

			body, _ := io.ReadAll(resp.Body)
			md := string(body)

			if !strings.Contains(md, "Agent: **"+agent+"**") {
				t.Errorf("response for %s missing correct agent identifier", agent)
			}
		})
	}
}

func TestE2E_ContextEndpoint_UnknownAgent(t *testing.T) {
	baseURL := alexandriaURL()

	// Unknown agents should still get a valid response (minimal context)
	resp := e2eRequest(t, "GET", baseURL+"/api/v1/context/unknown-agent", "unknown-agent", "")
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 for unknown agent, got %d: %s", resp.StatusCode, body)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/markdown") {
		t.Errorf("expected Content-Type text/markdown, got %q", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	md := string(body)
	if !strings.Contains(md, "# Boot Context") {
		t.Error("unknown agent should still get boot context heading")
	}
}

func TestE2E_ContextEndpoint_LilyScopedOwner(t *testing.T) {
	baseURL := alexandriaURL()

	resp := e2eRequest(t, "GET", baseURL+"/api/v1/context/lily", "lily", "")
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	body, _ := io.ReadAll(resp.Body)
	md := string(body)

	// Lily's profile has owner "mike-a", so if that person exists in the DB,
	// there should be an owner section. If not, the section is just omitted.
	// Either way the document should be valid markdown starting with the heading.
	if !strings.Contains(md, "# Boot Context") {
		t.Error("response missing boot context heading")
	}
	if !strings.Contains(md, "Agent: **lily**") {
		t.Error("response missing lily agent identifier")
	}
}
