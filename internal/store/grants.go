package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// AccessGrant represents an access grant in the vault.
type AccessGrant struct {
	ID           string    `json:"id"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	SubjectType  string    `json:"subject_type"`
	SubjectID    string    `json:"subject_id"`
	Permission   string    `json:"permission"`
	GrantedBy    *string   `json:"granted_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// AccessGrantCreateInput is the input for creating an access grant.
type AccessGrantCreateInput struct {
	ResourceType string  `json:"resource_type"`
	ResourceID   string  `json:"resource_id"`
	SubjectType  string  `json:"subject_type"`
	SubjectID    string  `json:"subject_id"`
	Permission   string  `json:"permission"`
	GrantedBy    *string `json:"granted_by,omitempty"`
}

// GrantStore provides access grant CRUD operations.
type GrantStore struct {
	db *DB
}

// NewGrantStore creates a new GrantStore.
func NewGrantStore(db *DB) *GrantStore {
	return &GrantStore{db: db}
}

// Create inserts a new access grant.
func (s *GrantStore) Create(ctx context.Context, input AccessGrantCreateInput) (*AccessGrant, error) {
	query := `
		INSERT INTO vault_access_grants (resource_type, resource_id, subject_type, subject_id, permission, granted_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, resource_type, resource_id, subject_type, subject_id, permission, granted_by, created_at`

	grant := &AccessGrant{}
	err := s.db.Pool.QueryRow(ctx, query,
		input.ResourceType, input.ResourceID, input.SubjectType,
		input.SubjectID, input.Permission, input.GrantedBy,
	).Scan(
		&grant.ID, &grant.ResourceType, &grant.ResourceID,
		&grant.SubjectType, &grant.SubjectID, &grant.Permission,
		&grant.GrantedBy, &grant.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating access grant: %w", err)
	}
	return grant, nil
}

// GetByID retrieves an access grant by ID.
func (s *GrantStore) GetByID(ctx context.Context, id string) (*AccessGrant, error) {
	query := `
		SELECT id, resource_type, resource_id, subject_type, subject_id, permission, granted_by, created_at
		FROM vault_access_grants WHERE id = $1`

	grant := &AccessGrant{}
	err := s.db.Pool.QueryRow(ctx, query, id).Scan(
		&grant.ID, &grant.ResourceType, &grant.ResourceID,
		&grant.SubjectType, &grant.SubjectID, &grant.Permission,
		&grant.GrantedBy, &grant.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting access grant: %w", err)
	}
	return grant, nil
}

// List returns all access grants with optional filtering.
func (s *GrantStore) List(ctx context.Context, resourceType, resourceID, subjectType, subjectID *string) ([]AccessGrant, error) {
	query := `
		SELECT id, resource_type, resource_id, subject_type, subject_id, permission, granted_by, created_at
		FROM vault_access_grants WHERE 1=1`
	args := []interface{}{}
	argCount := 0

	if resourceType != nil {
		argCount++
		query += fmt.Sprintf(" AND resource_type = $%d", argCount)
		args = append(args, *resourceType)
	}
	if resourceID != nil {
		argCount++
		query += fmt.Sprintf(" AND resource_id = $%d", argCount)
		args = append(args, *resourceID)
	}
	if subjectType != nil {
		argCount++
		query += fmt.Sprintf(" AND subject_type = $%d", argCount)
		args = append(args, *subjectType)
	}
	if subjectID != nil {
		argCount++
		query += fmt.Sprintf(" AND subject_id = $%d", argCount)
		args = append(args, *subjectID)
	}

	query += " ORDER BY created_at DESC"

	rows, err := s.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing access grants: %w", err)
	}
	defer rows.Close()

	var grants []AccessGrant
	for rows.Next() {
		var g AccessGrant
		if err := rows.Scan(
			&g.ID, &g.ResourceType, &g.ResourceID,
			&g.SubjectType, &g.SubjectID, &g.Permission,
			&g.GrantedBy, &g.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning access grant: %w", err)
		}
		grants = append(grants, g)
	}
	return grants, rows.Err()
}

// CheckAccess checks if a subject has access to a resource.
func (s *GrantStore) CheckAccess(ctx context.Context, subjectType, subjectID, resourceType, resourceID string) (bool, error) {
	query := `
		SELECT COUNT(*)
		FROM vault_access_grants
		WHERE subject_type = $1 AND subject_id = $2
		  AND resource_type = $3 AND resource_id = $4`

	var count int
	err := s.db.Pool.QueryRow(ctx, query, subjectType, subjectID, resourceType, resourceID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking access: %w", err)
	}
	return count > 0, nil
}

// CheckAccessWithPermission checks if a subject has specific permission to a resource.
func (s *GrantStore) CheckAccessWithPermission(ctx context.Context, subjectType, subjectID, resourceType, resourceID, permission string) (bool, error) {
	query := `
		SELECT COUNT(*)
		FROM vault_access_grants
		WHERE subject_type = $1 AND subject_id = $2
		  AND resource_type = $3 AND resource_id = $4
		  AND (permission = $5 OR permission = 'admin')`

	var count int
	err := s.db.Pool.QueryRow(ctx, query, subjectType, subjectID, resourceType, resourceID, permission).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking access with permission: %w", err)
	}
	return count > 0, nil
}

// Delete removes an access grant.
func (s *GrantStore) Delete(ctx context.Context, id string) error {
	ct, err := s.db.Pool.Exec(ctx, "DELETE FROM vault_access_grants WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("deleting access grant: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("access grant not found")
	}
	return nil
}

// DeleteByResource removes all access grants for a specific resource.
func (s *GrantStore) DeleteByResource(ctx context.Context, resourceType, resourceID string) error {
	_, err := s.db.Pool.Exec(ctx,
		"DELETE FROM vault_access_grants WHERE resource_type = $1 AND resource_id = $2",
		resourceType, resourceID)
	return err
}

// DeleteBySubject removes all access grants for a specific subject.
func (s *GrantStore) DeleteBySubject(ctx context.Context, subjectType, subjectID string) error {
	_, err := s.db.Pool.Exec(ctx,
		"DELETE FROM vault_access_grants WHERE subject_type = $1 AND subject_id = $2",
		subjectType, subjectID)
	return err
}

// Count returns the total number of access grants.
func (s *GrantStore) Count(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM vault_access_grants").Scan(&count)
	return count, err
}
