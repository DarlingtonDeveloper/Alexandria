package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Alias maps an external identifier to a canonical entity.
type Alias struct {
	ID          uuid.UUID `json:"id"`
	AliasType   string    `json:"alias_type"`
	AliasValue  string    `json:"alias_value"`
	CanonicalID uuid.UUID `json:"canonical_id"`
	Confidence  float64   `json:"confidence"`
	Source      string    `json:"source"`
	Reviewed    bool      `json:"reviewed"`
	CreatedAt   time.Time `json:"created_at"`
}

// LookupAlias finds an alias by type+value.
func LookupAlias(ctx context.Context, db DBTX, aliasType, aliasValue string) (*Alias, error) {
	a := &Alias{}
	err := db.QueryRow(ctx, `
		SELECT id, alias_type, alias_value, canonical_id, confidence, source, reviewed, created_at
		FROM vault_aliases WHERE alias_type = $1 AND alias_value = $2
	`, aliasType, aliasValue).Scan(&a.ID, &a.AliasType, &a.AliasValue, &a.CanonicalID,
		&a.Confidence, &a.Source, &a.Reviewed, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// CreateAlias inserts a new alias record.
func CreateAlias(ctx context.Context, db DBTX, a *Alias) error {
	return db.QueryRow(ctx, `
		INSERT INTO vault_aliases (alias_type, alias_value, canonical_id, confidence, source)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, reviewed, created_at
	`, a.AliasType, a.AliasValue, a.CanonicalID, a.Confidence, a.Source).
		Scan(&a.ID, &a.Reviewed, &a.CreatedAt)
}

// ListAliasesByCanonical returns all aliases pointing to a canonical entity.
func ListAliasesByCanonical(ctx context.Context, db DBTX, canonicalID uuid.UUID) ([]Alias, error) {
	rows, err := db.Query(ctx, `
		SELECT id, alias_type, alias_value, canonical_id, confidence, source, reviewed, created_at
		FROM vault_aliases WHERE canonical_id = $1 ORDER BY created_at
	`, canonicalID)
	if err != nil {
		return nil, fmt.Errorf("list aliases: %w", err)
	}
	defer rows.Close()

	var result []Alias
	for rows.Next() {
		var a Alias
		if err := rows.Scan(&a.ID, &a.AliasType, &a.AliasValue, &a.CanonicalID,
			&a.Confidence, &a.Source, &a.Reviewed, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan alias: %w", err)
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// PendingReviews returns unreviewed low-confidence aliases.
func PendingReviews(ctx context.Context, db DBTX) ([]Alias, error) {
	rows, err := db.Query(ctx, `
		SELECT id, alias_type, alias_value, canonical_id, confidence, source, reviewed, created_at
		FROM vault_aliases WHERE reviewed = FALSE AND confidence < 0.9
		ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("pending reviews: %w", err)
	}
	defer rows.Close()

	var result []Alias
	for rows.Next() {
		var a Alias
		if err := rows.Scan(&a.ID, &a.AliasType, &a.AliasValue, &a.CanonicalID,
			&a.Confidence, &a.Source, &a.Reviewed, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan alias: %w", err)
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// MarkReviewed approves or rejects an alias. If rejected, deletes it.
func MarkReviewed(ctx context.Context, db DBTX, id uuid.UUID, approved bool) error {
	var query string
	if approved {
		query = `UPDATE vault_aliases SET reviewed = TRUE, confidence = 1.0 WHERE id = $1`
	} else {
		query = `DELETE FROM vault_aliases WHERE id = $1`
	}
	tag, err := db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("mark reviewed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("alias %s not found", id)
	}
	return nil
}

// RePointAliases moves all aliases from one entity to another.
func RePointAliases(ctx context.Context, db DBTX, fromID, toID uuid.UUID) error {
	_, err := db.Exec(ctx, `UPDATE vault_aliases SET canonical_id = $1 WHERE canonical_id = $2`, toID, fromID)
	if err != nil {
		return fmt.Errorf("repoint aliases: %w", err)
	}
	return nil
}

// GetAlias retrieves an alias by ID.
func GetAlias(ctx context.Context, db DBTX, id uuid.UUID) (*Alias, error) {
	a := &Alias{}
	err := db.QueryRow(ctx, `
		SELECT id, alias_type, alias_value, canonical_id, confidence, source, reviewed, created_at
		FROM vault_aliases WHERE id = $1
	`, id).Scan(&a.ID, &a.AliasType, &a.AliasValue, &a.CanonicalID,
		&a.Confidence, &a.Source, &a.Reviewed, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get alias %s: %w", id, err)
	}
	return a, nil
}
