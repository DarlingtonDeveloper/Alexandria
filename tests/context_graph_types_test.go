package tests

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/warrentherabbit/alexandria/internal/store"
)

func TestCGEntityStructure(t *testing.T) {
	e := store.CGEntity{
		ID:          uuid.New(),
		Type:        "person",
		Key:         "email:test@example.com",
		DisplayName: "Test Person",
		Summary:     "A test entity",
		Metadata:    json.RawMessage(`{"role":"admin"}`),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if e.Type != "person" {
		t.Errorf("expected type 'person', got '%s'", e.Type)
	}
	if e.Key != "email:test@example.com" {
		t.Errorf("expected key 'email:test@example.com', got '%s'", e.Key)
	}
	if e.DeletedAt != nil {
		t.Error("expected DeletedAt to be nil for active entity")
	}
}

func TestCGEntityJSON(t *testing.T) {
	e := store.CGEntity{
		ID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Type:        "agent",
		Key:         "name:kai",
		DisplayName: "Kai",
		Summary:     "Orchestrator agent",
		Metadata:    json.RawMessage(`{}`),
		CreatedAt:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded store.CGEntity
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ID != e.ID {
		t.Errorf("ID mismatch: %s vs %s", decoded.ID, e.ID)
	}
	if decoded.Type != e.Type {
		t.Errorf("Type mismatch: %s vs %s", decoded.Type, e.Type)
	}
	if decoded.DisplayName != e.DisplayName {
		t.Errorf("DisplayName mismatch: %s vs %s", decoded.DisplayName, e.DisplayName)
	}
}

func TestCGEdgeStructure(t *testing.T) {
	e := store.CGEdge{
		ID:         uuid.New(),
		FromID:     uuid.New(),
		ToID:       uuid.New(),
		Type:       "works_on",
		Confidence: 0.95,
		Source:     "manual",
		ValidFrom:  time.Now(),
		Metadata:   json.RawMessage(`{}`),
		CreatedAt:  time.Now(),
	}

	if e.Type != "works_on" {
		t.Errorf("expected type 'works_on', got '%s'", e.Type)
	}
	if e.Confidence != 0.95 {
		t.Errorf("expected confidence 0.95, got %f", e.Confidence)
	}
	if e.ValidTo != nil {
		t.Error("expected ValidTo to be nil for active edge")
	}
}

func TestAliasStructure(t *testing.T) {
	a := store.Alias{
		ID:          uuid.New(),
		AliasType:   "email",
		AliasValue:  "test@example.com",
		CanonicalID: uuid.New(),
		Confidence:  1.0,
		Source:      "manual",
		Reviewed:    false,
		CreatedAt:   time.Now(),
	}

	if a.AliasType != "email" {
		t.Errorf("expected alias_type 'email', got '%s'", a.AliasType)
	}
	if a.Confidence != 1.0 {
		t.Errorf("expected confidence 1.0, got %f", a.Confidence)
	}
	if a.Reviewed {
		t.Error("expected Reviewed to be false")
	}
}

func TestProvenanceStructure(t *testing.T) {
	p := store.Provenance{
		ID:           uuid.New(),
		TargetID:     uuid.New(),
		TargetType:   "entity",
		SourceSystem: "identity-resolver",
		SourceRef:    "merge:a→b",
		Snippet:      "Merged by admin",
		CreatedAt:    time.Now(),
	}

	if p.TargetType != "entity" {
		t.Errorf("expected target_type 'entity', got '%s'", p.TargetType)
	}
	if p.SourceSystem != "identity-resolver" {
		t.Errorf("expected source_system 'identity-resolver', got '%s'", p.SourceSystem)
	}
	if p.SourceIdempotencyKey != nil {
		t.Error("expected SourceIdempotencyKey to be nil")
	}
}

func TestSemanticClusterStructure(t *testing.T) {
	c := store.SemanticCluster{
		ID:        uuid.New(),
		Label:     "AI Agents",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if c.Label != "AI Agents" {
		t.Errorf("expected label 'AI Agents', got '%s'", c.Label)
	}
	if c.DissolvedAt != nil {
		t.Error("expected DissolvedAt to be nil for active cluster")
	}
}

func TestMergeProposalStructure(t *testing.T) {
	p := store.MergeProposal{
		ID:           uuid.New(),
		EntityAID:    uuid.New(),
		EntityBID:    uuid.New(),
		Similarity:   0.92,
		ProposalType: "cluster",
		Status:       "pending",
		CreatedAt:    time.Now(),
	}

	if p.Status != "pending" {
		t.Errorf("expected status 'pending', got '%s'", p.Status)
	}
	if p.Similarity != 0.92 {
		t.Errorf("expected similarity 0.92, got %f", p.Similarity)
	}
	if p.ReviewedBy != nil {
		t.Error("expected ReviewedBy to be nil")
	}
}

func TestEntityExtendedFields(t *testing.T) {
	// Test the extended Entity struct (from graph.go)
	e := store.Entity{
		ID:          "test-id",
		Name:        "Test Entity",
		EntityType:  "person",
		Key:         "person:test",
		DisplayName: "Test Entity",
		Summary:     "A test entity with extended fields",
	}

	if e.Key != "person:test" {
		t.Errorf("expected Key 'person:test', got '%s'", e.Key)
	}
	if e.DisplayName != "Test Entity" {
		t.Errorf("expected DisplayName 'Test Entity', got '%s'", e.DisplayName)
	}
	if e.Summary != "A test entity with extended fields" {
		t.Errorf("expected Summary set, got '%s'", e.Summary)
	}
	if e.DeletedAt != nil {
		t.Error("expected DeletedAt to be nil")
	}
}

func TestRelationshipExtendedFields(t *testing.T) {
	// Test the extended Relationship struct (from graph.go)
	r := store.Relationship{
		SourceEntityID:   "src",
		TargetEntityID:   "tgt",
		RelationshipType: "manages",
		Strength:         0.8,
		Confidence:       0.9,
		Source:           "agent-kai",
	}

	if r.Confidence != 0.9 {
		t.Errorf("expected Confidence 0.9, got %f", r.Confidence)
	}
	if r.Source != "agent-kai" {
		t.Errorf("expected Source 'agent-kai', got '%s'", r.Source)
	}
	if r.ValidTo != nil {
		t.Error("expected ValidTo to be nil")
	}
}

func TestAuditActions(t *testing.T) {
	actions := []store.AccessAction{
		store.ActionIdentityResolve,
		store.ActionIdentityMerge,
		store.ActionSemanticRead,
	}

	expected := map[store.AccessAction]string{
		store.ActionIdentityResolve: "identity.resolve",
		store.ActionIdentityMerge:   "identity.merge",
		store.ActionSemanticRead:    "semantic.read",
	}

	for _, a := range actions {
		if string(a) != expected[a] {
			t.Errorf("expected %q, got %q", expected[a], a)
		}
	}
}
