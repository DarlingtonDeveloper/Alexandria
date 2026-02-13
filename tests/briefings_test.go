package tests

import (
	"testing"

	"github.com/MikeSquared-Agency/Alexandria/internal/briefings"
)

func TestBriefingSectionStructure(t *testing.T) {
	// Test that briefing types are well-formed
	b := briefings.Briefing{
		AgentID: "kai",
		Content: briefings.BriefingContent{
			Summary: "Test briefing",
			Sections: []briefings.BriefingSection{
				{
					Title: "Swarm Events",
					Items: []briefings.BriefingItem{
						{Content: "Something happened", Source: "hermes:test", Relevance: 0.9},
					},
				},
			},
			SecretsAvailable: []string{"api_key"},
			PendingTasks:     []any{},
		},
	}

	if b.AgentID != "kai" {
		t.Errorf("expected agent_id 'kai', got '%s'", b.AgentID)
	}
	if len(b.Content.Sections) != 1 {
		t.Errorf("expected 1 section, got %d", len(b.Content.Sections))
	}
	if b.Content.Sections[0].Title != "Swarm Events" {
		t.Errorf("unexpected section title: %s", b.Content.Sections[0].Title)
	}
	if len(b.Content.SecretsAvailable) != 1 {
		t.Errorf("expected 1 secret, got %d", len(b.Content.SecretsAvailable))
	}
}
