package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Provenance tracks the origin and audit trail of data changes.
type Provenance struct {
	ID                   uuid.UUID  `json:"id"`
	TargetID             uuid.UUID  `json:"target_id"`
	TargetType           string     `json:"target_type"`
	SourceSystem         string     `json:"source_system"`
	SourceRef            string     `json:"source_ref"`
	SourceIdempotencyKey *string    `json:"source_idempotency_key,omitempty"`
	Snippet              string     `json:"snippet"`
	AgentID              *uuid.UUID `json:"agent_id,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
}

// CreateProvenance inserts a provenance record.
func CreateProvenance(ctx context.Context, db DBTX, p *Provenance) error {
	return db.QueryRow(ctx, `
		INSERT INTO vault_provenance (target_id, target_type, source_system, source_ref, source_idempotency_key, snippet, agent_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`, p.TargetID, p.TargetType, p.SourceSystem, p.SourceRef, p.SourceIdempotencyKey, p.Snippet, p.AgentID).
		Scan(&p.ID, &p.CreatedAt)
}

// ListProvenanceByTarget returns provenance records for a given target.
func ListProvenanceByTarget(ctx context.Context, db DBTX, targetID uuid.UUID, targetType string) ([]Provenance, error) {
	rows, err := db.Query(ctx, `
		SELECT id, target_id, target_type, source_system, source_ref, source_idempotency_key, snippet, agent_id, created_at
		FROM vault_provenance WHERE target_id = $1 AND target_type = $2
		ORDER BY created_at
	`, targetID, targetType)
	if err != nil {
		return nil, fmt.Errorf("list provenance: %w", err)
	}
	defer rows.Close()

	var result []Provenance
	for rows.Next() {
		var p Provenance
		if err := rows.Scan(&p.ID, &p.TargetID, &p.TargetType, &p.SourceSystem, &p.SourceRef,
			&p.SourceIdempotencyKey, &p.Snippet, &p.AgentID, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan provenance: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
