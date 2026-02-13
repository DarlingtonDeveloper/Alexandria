package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CGEdge is the context-graph edge model used by DBTX-based functions.
type CGEdge struct {
	ID         uuid.UUID       `json:"id"`
	FromID     uuid.UUID       `json:"from_id"`
	ToID       uuid.UUID       `json:"to_id"`
	Type       string          `json:"type"`
	Confidence float64         `json:"confidence"`
	Source     string          `json:"source"`
	ValidFrom  time.Time       `json:"valid_from"`
	ValidTo    *time.Time      `json:"valid_to,omitempty"`
	Metadata   json.RawMessage `json:"metadata"`
	CreatedAt  time.Time       `json:"created_at"`
}

// CreateEdgeTx inserts a new edge using a DBTX.
func CreateEdgeTx(ctx context.Context, db DBTX, e *CGEdge) error {
	if len(e.Metadata) == 0 {
		e.Metadata = json.RawMessage(`{}`)
	}
	return db.QueryRow(ctx, `
		INSERT INTO vault_relationships (source_entity_id, target_entity_id, relationship_type, confidence, source, metadata, strength)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, valid_from, created_at
	`, e.FromID, e.ToID, e.Type, e.Confidence, e.Source, e.Metadata, e.Confidence).
		Scan(&e.ID, &e.ValidFrom, &e.CreatedAt)
}

// EdgesFrom returns active edges originating from an entity.
func EdgesFrom(ctx context.Context, db DBTX, entityID uuid.UUID) ([]CGEdge, error) {
	return queryEdges(ctx, db, `
		SELECT id, source_entity_id, target_entity_id, relationship_type, confidence, source,
		       valid_from, valid_to, metadata, created_at
		FROM vault_relationships WHERE source_entity_id = $1 AND valid_to IS NULL ORDER BY created_at
	`, entityID)
}

// EdgesTo returns active edges pointing to an entity.
func EdgesTo(ctx context.Context, db DBTX, entityID uuid.UUID) ([]CGEdge, error) {
	return queryEdges(ctx, db, `
		SELECT id, source_entity_id, target_entity_id, relationship_type, confidence, source,
		       valid_from, valid_to, metadata, created_at
		FROM vault_relationships WHERE target_entity_id = $1 AND valid_to IS NULL ORDER BY created_at
	`, entityID)
}

// RePointEdges moves all edge references from one entity to another.
func RePointEdges(ctx context.Context, db DBTX, fromID, toID uuid.UUID) error {
	if _, err := db.Exec(ctx, `UPDATE vault_relationships SET source_entity_id = $1 WHERE source_entity_id = $2`, toID, fromID); err != nil {
		return fmt.Errorf("repoint edges source: %w", err)
	}
	if _, err := db.Exec(ctx, `UPDATE vault_relationships SET target_entity_id = $1 WHERE target_entity_id = $2`, toID, fromID); err != nil {
		return fmt.Errorf("repoint edges target: %w", err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM vault_relationships WHERE source_entity_id = target_entity_id`); err != nil {
		return fmt.Errorf("delete self-edges: %w", err)
	}
	return nil
}

// UpsertSemanticEdge inserts a semantic_similarity edge or updates its confidence.
func UpsertSemanticEdge(ctx context.Context, db DBTX, e *CGEdge) error {
	if len(e.Metadata) == 0 {
		e.Metadata = json.RawMessage(`{}`)
	}
	return db.QueryRow(ctx, `
		INSERT INTO vault_relationships (source_entity_id, target_entity_id, relationship_type, confidence, source, metadata, strength)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (source_entity_id, target_entity_id, relationship_type)
			WHERE relationship_type = 'semantic_similarity' AND valid_to IS NULL
		DO UPDATE SET confidence = EXCLUDED.confidence, source = EXCLUDED.source
		RETURNING id, valid_from, created_at
	`, e.FromID, e.ToID, e.Type, e.Confidence, e.Source, e.Metadata, e.Confidence).
		Scan(&e.ID, &e.ValidFrom, &e.CreatedAt)
}

func queryEdges(ctx context.Context, db DBTX, query string, args ...any) ([]CGEdge, error) {
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query edges: %w", err)
	}
	defer rows.Close()

	var result []CGEdge
	for rows.Next() {
		var e CGEdge
		if err := rows.Scan(&e.ID, &e.FromID, &e.ToID, &e.Type, &e.Confidence, &e.Source,
			&e.ValidFrom, &e.ValidTo, &e.Metadata, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan edge: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
