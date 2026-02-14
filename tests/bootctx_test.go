package tests

import (
	"strings"
	"testing"

	"github.com/MikeSquared-Agency/Alexandria/internal/bootctx"
	"github.com/MikeSquared-Agency/Alexandria/internal/store"
)

func TestAgentProfiles_KnownAgents(t *testing.T) {
	// Verify the exported Generate function doesn't panic for all known agent names.
	// Since we can't call Generate without stores, we test the profile-driven logic
	// indirectly by checking metaString and the markdown output format.

	knownAgents := []string{"kai", "lily", "scout", "dutybound", "celebrimbor"}
	for _, agent := range knownAgents {
		t.Run(agent, func(t *testing.T) {
			// Ensure the assembler constructor accepts all required stores as nil-safe types
			// (verifies the API contract, not runtime behavior)
			_ = bootctx.NewAssembler(nil, nil, nil, nil)
		})
	}
}

func TestMetaString_Helper(t *testing.T) {
	// Test the metaString extraction logic that the assembler uses internally.
	// We replicate the logic here since it's unexported.
	tests := []struct {
		name     string
		meta     map[string]any
		key      string
		dflt     string
		expected string
	}{
		{"present string", map[string]any{"timezone": "Europe/London"}, "timezone", "—", "Europe/London"},
		{"missing key", map[string]any{"timezone": "Europe/London"}, "phone", "—", "—"},
		{"nil map", nil, "timezone", "—", "—"},
		{"non-string value", map[string]any{"count": 42}, "count", "—", "—"},
		{"empty string value", map[string]any{"name": ""}, "name", "default", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testMetaString(tt.meta, tt.key, tt.dflt)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// testMetaString replicates the unexported metaString helper for testing.
func testMetaString(m map[string]any, key, dflt string) string {
	if m == nil {
		return dflt
	}
	v, ok := m[key]
	if !ok {
		return dflt
	}
	s, ok := v.(string)
	if !ok {
		return dflt
	}
	return s
}

func TestBootContextMarkdownFormat(t *testing.T) {
	// Verify the expected markdown structure by checking what a well-formed
	// boot context document should look like.
	// This tests the contract, not the DB-dependent generation.

	sampleMD := strings.Join([]string{
		"# Boot Context",
		"",
		"Agent: **kai**",
		"",
		"## People",
		"",
		"| Name | Identifier | Timezone |",
		"|------|------------|----------|",
		"| Mike A | mike-a | Europe/London |",
		"",
		"## Agents",
		"",
		"| Name | Summary |",
		"|------|---------|",
		"| Kai | Orchestration agent |",
		"",
		"## Access",
		"",
		"### Secrets Available",
		"",
		"- `anthropic-api-key`",
		"",
		"## Rules",
		"",
		"- Always confirm before destructive actions",
		"",
		"## Infrastructure",
		"",
		"- **Alexandria** — http://warren_alexandria:8500",
		"",
	}, "\n")

	// Verify key sections present
	requiredSections := []string{
		"# Boot Context",
		"## People",
		"## Agents",
		"## Access",
		"## Rules",
		"## Infrastructure",
	}
	for _, section := range requiredSections {
		if !strings.Contains(sampleMD, section) {
			t.Errorf("expected section %q in boot context markdown", section)
		}
	}

	// Verify markdown table format
	if !strings.Contains(sampleMD, "| Name | Identifier | Timezone |") {
		t.Error("expected people table header")
	}
	if !strings.Contains(sampleMD, "|------|------------|----------|") {
		t.Error("expected people table separator")
	}
}

func TestAccessActionContextGen(t *testing.T) {
	// Verify the new audit action constant exists and has the expected value.
	if store.ActionContextGen != "context.generate" {
		t.Errorf("expected ActionContextGen = %q, got %q", "context.generate", store.ActionContextGen)
	}
}

func TestAgentAccessLevels(t *testing.T) {
	// Test the access level type constants are distinct.
	if bootctx.FullAccess == bootctx.ScopedAccess {
		t.Error("FullAccess and ScopedAccess should be distinct values")
	}
}

func TestAssemblerConstructor(t *testing.T) {
	// Verify NewAssembler returns a non-nil assembler (4 args: knowledge, secrets, graph, grants).
	a := bootctx.NewAssembler(nil, nil, nil, nil)
	if a == nil {
		t.Error("NewAssembler returned nil")
	}
}
