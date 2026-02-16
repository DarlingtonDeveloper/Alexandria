package tests

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/MikeSquared-Agency/Alexandria/internal/hermes"
)

func TestCorrectionSignalParsing(t *testing.T) {
	raw := `{
		"session_ref": "sess-001",
		"decision_id": "dec-abc",
		"agent_id": "developer",
		"model_id": "claude-sonnet-4-5-20250929",
		"model_tier": "standard",
		"correction_type": "rejected",
		"category": "architecture",
		"severity": "significant"
	}`

	var signal hermes.CorrectionSignal
	err := json.Unmarshal([]byte(raw), &signal)
	if err != nil {
		t.Fatalf("failed to parse CorrectionSignal: %v", err)
	}

	if signal.SessionRef != "sess-001" {
		t.Errorf("expected session_ref 'sess-001', got '%s'", signal.SessionRef)
	}
	if signal.AgentID != "developer" {
		t.Errorf("expected agent_id 'developer', got '%s'", signal.AgentID)
	}
	if signal.CorrectionType != "rejected" {
		t.Errorf("expected correction_type 'rejected', got '%s'", signal.CorrectionType)
	}
	if signal.Category != "architecture" {
		t.Errorf("expected category 'architecture', got '%s'", signal.Category)
	}
	if signal.ModelTier != "standard" {
		t.Errorf("expected model_tier 'standard', got '%s'", signal.ModelTier)
	}
}

func TestCorrectionEnvelopeParsing(t *testing.T) {
	raw := `{
		"id": "evt-123",
		"type": "dredd.correction",
		"source": "dredd",
		"timestamp": "2026-02-15T12:00:00Z",
		"data": {
			"session_ref": "sess-002",
			"decision_id": "dec-xyz",
			"agent_id": "reviewer",
			"model_id": "claude-opus-4-6",
			"model_tier": "premium",
			"correction_type": "confirmed",
			"category": "security",
			"severity": "critical"
		}
	}`

	var envelope hermes.CorrectionEnvelope
	err := json.Unmarshal([]byte(raw), &envelope)
	if err != nil {
		t.Fatalf("failed to parse CorrectionEnvelope: %v", err)
	}

	if envelope.ID != "evt-123" {
		t.Errorf("expected id 'evt-123', got '%s'", envelope.ID)
	}
	if envelope.Type != "dredd.correction" {
		t.Errorf("expected type 'dredd.correction', got '%s'", envelope.Type)
	}
	if envelope.Source != "dredd" {
		t.Errorf("expected source 'dredd', got '%s'", envelope.Source)
	}
	expected := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	if !envelope.Timestamp.Equal(expected) {
		t.Errorf("expected timestamp %v, got %v", expected, envelope.Timestamp)
	}

	// Parse inner data
	var signal hermes.CorrectionSignal
	err = json.Unmarshal(envelope.Data, &signal)
	if err != nil {
		t.Fatalf("failed to parse inner CorrectionSignal: %v", err)
	}
	if signal.AgentID != "reviewer" {
		t.Errorf("expected agent_id 'reviewer', got '%s'", signal.AgentID)
	}
	if signal.CorrectionType != "confirmed" {
		t.Errorf("expected correction_type 'confirmed', got '%s'", signal.CorrectionType)
	}
	if signal.Severity != "critical" {
		t.Errorf("expected severity 'critical', got '%s'", signal.Severity)
	}
}

func TestCorrectionSignalRoundTrip(t *testing.T) {
	signal := hermes.CorrectionSignal{
		SessionRef:     "sess-rt",
		DecisionID:     "dec-rt",
		AgentID:        "architect",
		ModelID:        "claude-haiku-4-5-20251001",
		ModelTier:      "economy",
		CorrectionType: "rejected",
		Category:       "naming",
		Severity:       "routine",
	}

	data, err := json.Marshal(signal)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed hermes.CorrectionSignal
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if parsed != signal {
		t.Errorf("round-trip mismatch: got %+v, want %+v", parsed, signal)
	}
}
