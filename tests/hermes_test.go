package tests

import (
	"encoding/json"
	"testing"

	"github.com/warrentherabbit/alexandria/internal/hermes"
)

func TestHermesEventParsing(t *testing.T) {
	raw := `{
		"id": "evt-123",
		"type": "discovery.finding",
		"source": "kai",
		"data": {
			"content": "Mike's Supabase project supports pgvector",
			"tags": ["supabase", "pgvector"]
		}
	}`

	var event hermes.HermesEvent
	err := json.Unmarshal([]byte(raw), &event)
	if err != nil {
		t.Fatalf("failed to parse event: %v", err)
	}

	if event.ID != "evt-123" {
		t.Errorf("expected id 'evt-123', got '%s'", event.ID)
	}
	if event.Source != "kai" {
		t.Errorf("expected source 'kai', got '%s'", event.Source)
	}
	if event.Data.Content != "Mike's Supabase project supports pgvector" {
		t.Errorf("unexpected content: %s", event.Data.Content)
	}
	if len(event.Data.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(event.Data.Tags))
	}
}

func TestVaultEventMarshaling(t *testing.T) {
	event := hermes.VaultEvent{
		ID:     "test-id",
		Type:   "vault.knowledge.created",
		Source: "alexandria",
		Data: map[string]any{
			"id":       "knowledge-id",
			"category": "discovery",
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed map[string]any
	json.Unmarshal(data, &parsed)

	if parsed["type"] != "vault.knowledge.created" {
		t.Errorf("unexpected type: %v", parsed["type"])
	}
	if parsed["source"] != "alexandria" {
		t.Errorf("unexpected source: %v", parsed["source"])
	}
}
