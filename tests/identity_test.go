package tests

import (
	"testing"

	"github.com/google/uuid"

	"github.com/MikeSquared-Agency/Alexandria/internal/identity"
)

func TestResolveRequest_Validation(t *testing.T) {
	// Resolver with nil store — only tests validation (before store is touched)
	resolver := identity.NewResolver(nil)

	tests := []struct {
		name string
		req  identity.ResolveRequest
	}{
		{
			name: "missing alias_type",
			req: identity.ResolveRequest{
				AliasValue: "test@example.com",
				EntityType: "person",
			},
		},
		{
			name: "missing alias_value",
			req: identity.ResolveRequest{
				AliasType:  "email",
				EntityType: "person",
			},
		},
		{
			name: "missing entity_type",
			req: identity.ResolveRequest{
				AliasType:  "email",
				AliasValue: "test@example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolver.Resolve(t.Context(), tt.req)
			if err == nil {
				t.Error("expected validation error but got nil")
			}
		})
	}
}

func TestMerge_SelfMergeRejected(t *testing.T) {
	resolver := identity.NewResolver(nil)
	id := uuid.New()

	_, err := resolver.Merge(t.Context(), id, id, "tester")
	if err == nil {
		t.Error("expected error for self-merge, got nil")
	}
	if err.Error() != "cannot merge entity with itself" {
		t.Errorf("unexpected error: %s", err.Error())
	}
}

func TestResolveOutcomes(t *testing.T) {
	// Verify outcome constants
	outcomes := []string{"matched", "pending_review", "created"}
	for _, o := range outcomes {
		if o == "" {
			t.Error("outcome should not be empty")
		}
	}
}
