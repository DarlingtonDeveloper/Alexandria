package tests

import (
	"testing"

	"github.com/MikeSquared-Agency/Alexandria/internal/store"
)

func TestEntityTypes(t *testing.T) {
	types := []store.EntityType{
		store.EntityPerson,
		store.EntityAgent,
		store.EntityService,
		store.EntityProject,
		store.EntityConcept,
		store.EntityCredential,
	}

	for _, et := range types {
		if et == "" {
			t.Error("entity type should not be empty")
		}
	}

	if store.EntityPerson != "person" {
		t.Errorf("expected 'person', got '%s'", store.EntityPerson)
	}
}

func TestRelationshipStructure(t *testing.T) {
	r := store.Relationship{
		SourceEntityID:   "src-id",
		TargetEntityID:   "tgt-id",
		RelationshipType: "owns",
		Strength:         0.9,
	}

	if r.RelationshipType != "owns" {
		t.Errorf("expected 'owns', got '%s'", r.RelationshipType)
	}
	if r.Strength != 0.9 {
		t.Errorf("expected 0.9, got %f", r.Strength)
	}
}
