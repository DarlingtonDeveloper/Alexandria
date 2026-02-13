// Package identity provides identity resolution and entity merging.
package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/warrentherabbit/alexandria/internal/store"
)

// ResolveRequest is the input to identity resolution.
type ResolveRequest struct {
	AliasType   string `json:"alias_type"`
	AliasValue  string `json:"alias_value"`
	Source      string `json:"source"`
	EntityType  string `json:"entity_type"`
	DisplayName string `json:"display_name"`
}

// ResolveResult is the output of identity resolution.
type ResolveResult struct {
	EntityID uuid.UUID `json:"entity_id"`
	AliasID  uuid.UUID `json:"alias_id"`
	Outcome  string    `json:"outcome"` // "matched", "pending_review", "created"
}

// MergeResult is the output of a merge operation.
type MergeResult struct {
	SurvivorID uuid.UUID `json:"survivor_id"`
	MergedID   uuid.UUID `json:"merged_id"`
}

// Store is the interface consumed by Resolver.
// Consumer-defined interface — Alexandria's *store.DB satisfies this.
type Store interface {
	WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error
	DBTX() store.DBTX
}

// Resolver performs identity resolution and entity merging.
type Resolver struct {
	store Store
}

// NewResolver creates a Resolver.
func NewResolver(s Store) *Resolver {
	return &Resolver{store: s}
}

// Resolve looks up an alias and either matches to an existing entity,
// flags for review, or creates a new entity.
func (r *Resolver) Resolve(ctx context.Context, req ResolveRequest) (*ResolveResult, error) {
	if req.AliasType == "" || req.AliasValue == "" {
		return nil, fmt.Errorf("alias_type and alias_value are required")
	}
	if req.EntityType == "" {
		return nil, fmt.Errorf("entity_type is required")
	}

	var result ResolveResult

	err := r.store.WithTx(ctx, func(tx pgx.Tx) error {
		alias, err := store.LookupAlias(ctx, tx, req.AliasType, req.AliasValue)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("lookup alias: %w", err)
		}

		if alias != nil {
			result.EntityID = alias.CanonicalID
			result.AliasID = alias.ID
			if alias.Confidence >= 0.9 {
				result.Outcome = "matched"
			} else {
				result.Outcome = "pending_review"
			}
			return nil
		}

		// Not found — create entity + alias
		entity := &store.CGEntity{
			Type:        req.EntityType,
			Key:         req.AliasType + ":" + req.AliasValue,
			DisplayName: req.DisplayName,
		}
		if err := store.CreateEntityTx(ctx, tx, entity); err != nil {
			return fmt.Errorf("create entity: %w", err)
		}

		alias = &store.Alias{
			AliasType:   req.AliasType,
			AliasValue:  req.AliasValue,
			CanonicalID: entity.ID,
			Confidence:  1.0,
			Source:      req.Source,
		}
		if err := store.CreateAlias(ctx, tx, alias); err != nil {
			// UNIQUE constraint race — another tx created it first. Re-lookup.
			retryAlias, retryErr := store.LookupAlias(ctx, tx, req.AliasType, req.AliasValue)
			if retryErr != nil {
				return fmt.Errorf("retry lookup: %w (original: %w)", retryErr, err)
			}
			result.EntityID = retryAlias.CanonicalID
			result.AliasID = retryAlias.ID
			if retryAlias.Confidence >= 0.9 {
				result.Outcome = "matched"
			} else {
				result.Outcome = "pending_review"
			}
			return nil
		}

		result.EntityID = entity.ID
		result.AliasID = alias.ID
		result.Outcome = "created"
		return nil
	})

	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Merge combines two entities, re-pointing all references to the survivor.
func (r *Resolver) Merge(ctx context.Context, survivorID, mergedID uuid.UUID, approvedBy string) (*MergeResult, error) {
	if survivorID == mergedID {
		return nil, fmt.Errorf("cannot merge entity with itself")
	}

	err := r.store.WithTx(ctx, func(tx pgx.Tx) error {
		// Verify both entities exist
		if _, err := store.GetEntityTx(ctx, tx, survivorID); err != nil {
			return fmt.Errorf("survivor: %w", err)
		}
		if _, err := store.GetEntityTx(ctx, tx, mergedID); err != nil {
			return fmt.Errorf("merged: %w", err)
		}

		// Re-point aliases
		if err := store.RePointAliases(ctx, tx, mergedID, survivorID); err != nil {
			return fmt.Errorf("repoint aliases: %w", err)
		}

		// Re-point edges
		if err := store.RePointEdges(ctx, tx, mergedID, survivorID); err != nil {
			return fmt.Errorf("repoint edges: %w", err)
		}

		// Soft-delete merged entity
		if err := store.SoftDeleteEntity(ctx, tx, mergedID); err != nil {
			return fmt.Errorf("soft delete: %w", err)
		}

		// Touch survivor so semantic worker re-embeds it
		if _, err := tx.Exec(ctx, `UPDATE vault_entities SET updated_at = now() WHERE id = $1`, survivorID); err != nil {
			return fmt.Errorf("touch survivor: %w", err)
		}

		// Record provenance
		p := &store.Provenance{
			TargetID:     survivorID,
			TargetType:   "entity",
			SourceSystem: "identity-resolver",
			SourceRef:    fmt.Sprintf("merge:%s→%s", mergedID, survivorID),
			Snippet:      fmt.Sprintf("Merged by %s", approvedBy),
		}
		if err := store.CreateProvenance(ctx, tx, p); err != nil {
			return fmt.Errorf("provenance: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return &MergeResult{SurvivorID: survivorID, MergedID: mergedID}, nil
}
