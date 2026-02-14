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

func TestE2E_ContextEndpoint_LilyGraphOwner(t *testing.T) {
	baseURL := alexandriaURL()

	resp := e2eRequest(t, "GET", baseURL+"/api/v1/context/lily", "lily", "")
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	body, _ := io.ReadAll(resp.Body)
	md := string(body)

	if !strings.Contains(md, "# Boot Context") {
		t.Error("response missing boot context heading")
	}
	if !strings.Contains(md, "Agent: **lily**") {
		t.Error("response missing lily agent identifier")
	}

	// Lily's owner is derived from graph "owns" relationship (Mike Ajijola → owns → Lily).
	if !strings.Contains(md, "## Your Owner") {
		t.Error("lily should have an owner section derived from graph")
	}
	if !strings.Contains(md, "Mike Ajijola") {
		t.Error("lily's owner should be Mike Ajijola")
	}
}

func TestE2E_ContextEndpoint_LilyScopedPeople(t *testing.T) {
	baseURL := alexandriaURL()

	resp := e2eRequest(t, "GET", baseURL+"/api/v1/context/lily", "lily", "")
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	md := string(body)

	// Lily is scoped — should only see her owner in people section
	if !strings.Contains(md, "## People") {
		t.Fatal("lily should have a people section")
	}
	if !strings.Contains(md, "Mike Ajijola") {
		t.Error("lily should see her owner Mike Ajijola in people")
	}
	// Mike Darlington should NOT be visible to lily (scoped access)
	if strings.Contains(md, "Mike Darlington") {
		t.Error("lily should NOT see Mike Darlington (scoped access)")
	}
}

func TestE2E_ContextEndpoint_KaiFullAccess(t *testing.T) {
	baseURL := alexandriaURL()

	resp := e2eRequest(t, "GET", baseURL+"/api/v1/context/kai", "kai", "")
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	md := string(body)

	// Kai has full access — should see all people
	if !strings.Contains(md, "## People") {
		t.Fatal("kai should have a people section")
	}
	if !strings.Contains(md, "Mike Darlington") {
		t.Error("kai should see Mike Darlington (full access)")
	}
	if !strings.Contains(md, "Mike Ajijola") {
		t.Error("kai should see Mike Ajijola (full access)")
	}

	// Kai should have an owner section (derived from graph)
	if !strings.Contains(md, "## Your Owner") {
		t.Error("kai should have an owner section")
	}
}

func TestE2E_ContextEndpoint_ScoutScopedToMikeD(t *testing.T) {
	baseURL := alexandriaURL()

	resp := e2eRequest(t, "GET", baseURL+"/api/v1/context/scout", "scout", "")
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	md := string(body)

	// Scout's owner should be Mike Darlington (from graph)
	if !strings.Contains(md, "## Your Owner") {
		t.Fatal("scout should have an owner section")
	}
	if !strings.Contains(md, "Mike Darlington") {
		t.Error("scout's owner should be Mike Darlington")
	}

	// Scout is scoped — should only see Mike Darlington in people
	if strings.Contains(md, "Mike Ajijola") {
		t.Error("scout should NOT see Mike Ajijola (scoped to Mike D)")
	}
}

func TestE2E_ContextEndpoint_AgentSummaries(t *testing.T) {
	baseURL := alexandriaURL()

	resp := e2eRequest(t, "GET", baseURL+"/api/v1/context/kai", "kai", "")
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	md := string(body)

	if !strings.Contains(md, "## Agents") {
		t.Fatal("should have agents section")
	}

	// Extract just the Agents section
	agentsIdx := strings.Index(md, "## Agents")
	if agentsIdx == -1 {
		t.Fatal("agents section not found")
	}
	agentsEnd := strings.Index(md[agentsIdx+1:], "\n## ")
	var agentsSection string
	if agentsEnd == -1 {
		agentsSection = md[agentsIdx:]
	} else {
		agentsSection = md[agentsIdx : agentsIdx+1+agentsEnd]
	}

	// All agents should be present with non-empty summaries
	agents := []string{"Kai", "Lily", "Scout", "DutyBound", "Celebrimbor"}
	for _, agent := range agents {
		if !strings.Contains(agentsSection, "| "+agent+" |") {
			t.Errorf("agents table should contain %s", agent)
		}
	}

	// Verify no agent has a dash-only summary within the agents section
	if strings.Contains(agentsSection, "| — |") {
		t.Error("no agent should have an empty summary (dash fallback) in the agents section")
	}
}

func TestE2E_ContextEndpoint_RulesSection(t *testing.T) {
	baseURL := alexandriaURL()

	resp := e2eRequest(t, "GET", baseURL+"/api/v1/context/kai", "kai", "")
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	md := string(body)

	if !strings.Contains(md, "## Rules") {
		t.Fatal("should have rules section")
	}

	// Rules come from category=decision, tag=rules
	// Verify at least one rule is present
	rulesIdx := strings.Index(md, "## Rules")
	rulesSection := md[rulesIdx:]
	if !strings.Contains(rulesSection, "- ") {
		t.Error("rules section should contain at least one bullet point")
	}
}
