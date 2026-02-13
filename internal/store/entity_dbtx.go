package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CGEntity is the context-graph entity model used by DBTX-based functions.
// It uses uuid.UUID for IDs and json.RawMessage for metadata, matching
// the identity resolver's expectations.
type CGEntity struct {
	ID          uuid.UUID       `json:"id"`
	Type        string          `json:"type"`
	Key         string          `json:"key"`
	DisplayName string          `json:"display_name"`
	Summary     string          `json:"summary"`
	Metadata    json.RawMessage `json:"metadata"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	DeletedAt   *time.Time      `json:"deleted_at,omitempty"`
}

// CreateEntityTx inserts a new entity using a DBTX (transaction-safe).
func CreateEntityTx(ctx context.Context, db DBTX, e *CGEntity) error {
	if len(e.Metadata) == 0 {
		e.Metadata = json.RawMessage(`{}`)
	}
	return db.QueryRow(ctx, `
		INSERT INTO vault_entities (name, entity_type, key, display_name, summary, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`, e.DisplayName, e.Type, e.Key, e.DisplayName, e.Summary, e.Metadata).
		Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt)
}

// GetEntityTx retrieves an entity by ID using a DBTX.
func GetEntityTx(ctx context.Context, db DBTX, id uuid.UUID) (*CGEntity, error) {
	e := &CGEntity{}
	err := db.QueryRow(ctx, `
		SELECT id, entity_type, key, display_name, summary, metadata, created_at, updated_at, deleted_at
		FROM vault_entities WHERE id = $1
	`, id).Scan(&e.ID, &e.Type, &e.Key, &e.DisplayName, &e.Summary, &e.Metadata,
		&e.CreatedAt, &e.UpdatedAt, &e.DeletedAt)
	if err != nil {
		return nil, fmt.Errorf("get entity %s: %w", id, err)
	}
	return e, nil
}

// GetEntityByKey retrieves an entity by its unique key using a DBTX.
func GetEntityByKey(ctx context.Context, db DBTX, key string) (*CGEntity, error) {
	e := &CGEntity{}
	err := db.QueryRow(ctx, `
		SELECT id, entity_type, key, display_name, summary, metadata, created_at, updated_at, deleted_at
		FROM vault_entities WHERE key = $1
	`, key).Scan(&e.ID, &e.Type, &e.Key, &e.DisplayName, &e.Summary, &e.Metadata,
		&e.CreatedAt, &e.UpdatedAt, &e.DeletedAt)
	if err != nil {
		return nil, fmt.Errorf("get entity by key %s: %w", key, err)
	}
	return e, nil
}

// SoftDeleteEntity marks an entity as deleted without removing it.
func SoftDeleteEntity(ctx context.Context, db DBTX, id uuid.UUID) error {
	tag, err := db.Exec(ctx, `UPDATE vault_entities SET deleted_at = now(), updated_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("soft delete entity: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("entity %s not found or already deleted", id)
	}
	return nil
}

// ListEntitiesTx lists non-deleted entities with optional type filter using a DBTX.
func ListEntitiesTx(ctx context.Context, db DBTX, entityType string) ([]CGEntity, error) {
	query := `SELECT id, entity_type, key, display_name, summary, metadata, created_at, updated_at, deleted_at
		FROM vault_entities WHERE deleted_at IS NULL`
	args := []any{}

	if entityType != "" {
		query += ` AND entity_type = $1`
		args = append(args, entityType)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list entities: %w", err)
	}
	defer rows.Close()

	var result []CGEntity
	for rows.Next() {
		var e CGEntity
		if err := rows.Scan(&e.ID, &e.Type, &e.Key, &e.DisplayName, &e.Summary, &e.Metadata,
			&e.CreatedAt, &e.UpdatedAt, &e.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan entity: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
