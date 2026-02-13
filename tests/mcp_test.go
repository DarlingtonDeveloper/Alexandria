package tests

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"testing"
)

// TestMCPServer_Initialize verifies the MCP server responds to JSON-RPC initialize.
// Requires the binary to be built first: go build -o bin/alexandria-mcp ./cmd/alexandria-mcp
func TestMCPServer_Initialize(t *testing.T) {
	// Build the binary
	buildCmd := exec.Command("go", "build", "-o", "/tmp/alexandria-mcp-test", "./cmd/alexandria-mcp")
	buildCmd.Dir = ".."
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	// Send initialize request
	req := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"
	cmd := exec.Command("/tmp/alexandria-mcp-test")
	cmd.Stdin = bytes.NewReader([]byte(req))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("mcp server failed: %v", err)
	}

	var resp struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			ProtocolVersion string `json:"protocolVersion"`
			ServerInfo      struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"serverInfo"`
			Capabilities struct {
				Tools *struct{} `json:"tools"`
			} `json:"capabilities"`
		} `json:"result"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nraw: %s", err, out)
	}

	if resp.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc '2.0', got '%s'", resp.JSONRPC)
	}
	if resp.ID != 1 {
		t.Errorf("expected id 1, got %d", resp.ID)
	}
	if resp.Result.ProtocolVersion != "2025-11-25" {
		t.Errorf("expected protocol '2025-11-25', got '%s'", resp.Result.ProtocolVersion)
	}
	if resp.Result.ServerInfo.Name != "alexandria" {
		t.Errorf("expected server name 'alexandria', got '%s'", resp.Result.ServerInfo.Name)
	}
	if resp.Result.ServerInfo.Version != "0.1.0" {
		t.Errorf("expected version '0.1.0', got '%s'", resp.Result.ServerInfo.Version)
	}
	if resp.Result.Capabilities.Tools == nil {
		t.Error("expected tools capability to be present")
	}
}

// TestMCPServer_ToolsList verifies the MCP server returns the expected tool definitions.
func TestMCPServer_ToolsList(t *testing.T) {
	buildCmd := exec.Command("go", "build", "-o", "/tmp/alexandria-mcp-test", "./cmd/alexandria-mcp")
	buildCmd.Dir = ".."
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	// Send initialize + tools/list
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n"

	cmd := exec.Command("/tmp/alexandria-mcp-test")
	cmd.Stdin = bytes.NewReader([]byte(input))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("mcp server failed: %v", err)
	}

	// Parse both lines
	lines := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
	if len(lines) < 2 {
		t.Fatalf("expected 2 response lines, got %d: %s", len(lines), out)
	}

	var resp struct {
		ID     int `json:"id"`
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(lines[1], &resp); err != nil {
		t.Fatalf("unmarshal tools/list: %v\nraw: %s", err, lines[1])
	}

	expectedTools := []string{
		"cg_resolve", "cg_lookup", "cg_search", "cg_record_edge",
		"cg_pending", "cg_similar", "cg_clusters", "cg_semantic_status",
	}

	if len(resp.Result.Tools) != len(expectedTools) {
		t.Fatalf("expected %d tools, got %d", len(expectedTools), len(resp.Result.Tools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range resp.Result.Tools {
		toolNames[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("tool %s has empty description", tool.Name)
		}
	}

	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("missing expected tool: %s", name)
		}
	}
}

// TestMCPServer_UnknownMethod verifies the server returns method-not-found for unknown methods.
func TestMCPServer_UnknownMethod(t *testing.T) {
	buildCmd := exec.Command("go", "build", "-o", "/tmp/alexandria-mcp-test", "./cmd/alexandria-mcp")
	buildCmd.Dir = ".."
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	req := `{"jsonrpc":"2.0","id":1,"method":"nonexistent"}` + "\n"
	cmd := exec.Command("/tmp/alexandria-mcp-test")
	cmd.Stdin = bytes.NewReader([]byte(req))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("mcp server failed: %v", err)
	}

	var resp struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error response")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected error code -32601, got %d", resp.Error.Code)
	}
}
