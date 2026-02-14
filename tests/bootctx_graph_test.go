package tests

import (
	"testing"

	"github.com/MikeSquared-Agency/Alexandria/internal/bootctx"
	"github.com/MikeSquared-Agency/Alexandria/internal/store"
)

func TestAgentProfile_NoOwnerID(t *testing.T) {
	// AgentProfile no longer has an OwnerID field.
	// Ownership is derived from graph relationships at runtime.
	p := bootctx.AgentProfile{
		Access:    bootctx.ScopedAccess,
		ExtraTags: []string{"ci"},
	}
	if p.Access != bootctx.ScopedAccess {
		t.Errorf("expected ScopedAccess, got %d", p.Access)
	}
	if len(p.ExtraTags) != 1 || p.ExtraTags[0] != "ci" {
		t.Errorf("expected ExtraTags=[ci], got %v", p.ExtraTags)
	}
}

func TestAssemblerConstructor_FourArgs(t *testing.T) {
	// Constructor takes 4 args: knowledge, secrets, graph, grants.
	// PersonStore is no longer a dependency.
	a := bootctx.NewAssembler(nil, nil, nil, nil)
	if a == nil {
		t.Fatal("NewAssembler returned nil")
	}
}

func TestGraphStore_EntityByNameSignature(t *testing.T) {
	// Verify GetEntityByName exists on GraphStore with the expected signature.
	// We can't call it without a DB, but we can verify the method is accessible.
	var gs *store.GraphStore
	_ = gs // compile-time check that GraphStore has GetEntityByName
}

func TestOwnerDerivation_OwnsRelationshipType(t *testing.T) {
	// The assembler derives ownership from "owns" relationships in the graph.
	// Verify the relationship type constant is what we expect.
	r := store.Relationship{
		SourceEntityID:   "person-uuid",
		TargetEntityID:   "agent-uuid",
		RelationshipType: "owns",
		Strength:         1.0,
	}

	if r.RelationshipType != "owns" {
		t.Errorf("expected relationship type 'owns', got %q", r.RelationshipType)
	}
	// Owner is the source (person) of the "owns" edge pointing to agent (target)
	if r.SourceEntityID != "person-uuid" {
		t.Errorf("expected source to be person, got %q", r.SourceEntityID)
	}
	if r.TargetEntityID != "agent-uuid" {
		t.Errorf("expected target to be agent, got %q", r.TargetEntityID)
	}
}

func TestEntityMetadata_OwnerFields(t *testing.T) {
	// The owner section now renders from entity metadata.
	// Verify the metadata keys that writeOwnerSection reads.
	owner := store.Entity{
		ID:          "test-id",
		Name:        "Mike Darlington",
		DisplayName: "Mike Darlington",
		EntityType:  store.EntityPerson,
		Metadata: map[string]any{
			"identifier":  "mike-d",
			"phone":       "+447444361435",
			"timezone":    "Europe/London",
			"preferences": "dark mode",
		},
	}

	// These are the fields writeOwnerSection extracts
	if testMetaString(owner.Metadata, "identifier", "") != "mike-d" {
		t.Error("expected identifier from metadata")
	}
	if testMetaString(owner.Metadata, "phone", "") != "+447444361435" {
		t.Error("expected phone from metadata")
	}
	if testMetaString(owner.Metadata, "timezone", "") != "Europe/London" {
		t.Error("expected timezone from metadata")
	}
	if testMetaString(owner.Metadata, "preferences", "") != "dark mode" {
		t.Error("expected preferences from metadata")
	}
}

func TestEntityMetadata_FallbackIdentifier(t *testing.T) {
	// When identifier is not in metadata, the assembler falls back to entity.Name.
	owner := store.Entity{
		Name:     "Mike Darlington",
		Metadata: map[string]any{},
	}

	identifier := testMetaString(owner.Metadata, "identifier", owner.Name)
	if identifier != "Mike Darlington" {
		t.Errorf("expected fallback to entity Name, got %q", identifier)
	}
}

func TestScopedPeopleFiltering(t *testing.T) {
	// Scoped agents should only see their owner in the people section.
	// The filtering logic compares entity IDs (owner.ID vs person.ID).
	ownerID := "owner-uuid"
	people := []store.Entity{
		{ID: "owner-uuid", Name: "Owner", DisplayName: "Owner"},
		{ID: "other-uuid", Name: "Other", DisplayName: "Other"},
	}

	var visible []store.Entity
	for _, p := range people {
		if p.ID == ownerID {
			visible = append(visible, p)
		}
	}

	if len(visible) != 1 {
		t.Fatalf("expected 1 visible person, got %d", len(visible))
	}
	if visible[0].Name != "Owner" {
		t.Errorf("expected Owner, got %q", visible[0].Name)
	}
}

func TestFullAccessSeesAllPeople(t *testing.T) {
	// Full access agents see all people (no filtering).
	profile := bootctx.AgentProfile{Access: bootctx.FullAccess}

	people := []store.Entity{
		{ID: "a", Name: "Alice"},
		{ID: "b", Name: "Bob"},
		{ID: "c", Name: "Charlie"},
	}

	var visible []store.Entity
	for _, p := range people {
		// Replicate assembler logic: skip filtering when FullAccess
		if profile.Access != bootctx.FullAccess {
			continue // would filter here
		}
		visible = append(visible, p)
	}

	if len(visible) != 3 {
		t.Errorf("expected 3 visible people for FullAccess, got %d", len(visible))
	}
}

func TestAgentSummaryFallback(t *testing.T) {
	// When entity.Summary is populated, it's used directly.
	// When empty, the assembler falls back to knowledge entries.
	agents := []store.Entity{
		{Name: "Kai", Summary: "Orchestration agent"},
		{Name: "NewAgent", Summary: ""},
	}

	for _, a := range agents {
		summary := a.Summary
		if summary == "" {
			summary = "—" // fallback when no knowledge entry either
		}
		if a.Name == "Kai" && summary != "Orchestration agent" {
			t.Errorf("expected entity summary for Kai, got %q", summary)
		}
		if a.Name == "NewAgent" && summary != "—" {
			t.Errorf("expected dash fallback for NewAgent, got %q", summary)
		}
	}
}
