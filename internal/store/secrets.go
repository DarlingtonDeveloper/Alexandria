package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Secret represents a stored encrypted secret.
type Secret struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	EncryptedValue       string     `json:"-"`
	Description          *string    `json:"description,omitempty"`
	Scope                []string   `json:"scope"`
	RotationIntervalDays *int       `json:"rotation_interval_days,omitempty"`
	LastRotatedAt        *time.Time `json:"last_rotated_at,omitempty"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	CreatedBy            string     `json:"created_by"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// SecretCreateInput is the input for creating a secret.
type SecretCreateInput struct {
	Name                 string   `json:"name"`
	EncryptedValue       string   `json:"-"`
	Description          *string  `json:"description,omitempty"`
	Scope                []string `json:"scope"`
	RotationIntervalDays *int     `json:"rotation_interval_days,omitempty"`
	CreatedBy            string   `json:"created_by"`
}

// SecretStore provides secret CRUD operations.
type SecretStore struct {
	db *DB
}

// NewSecretStore creates a new SecretStore.
func NewSecretStore(db *DB) *SecretStore {
	return &SecretStore{db: db}
}

// Create inserts a new secret.
func (s *SecretStore) Create(ctx context.Context, input SecretCreateInput) (*Secret, error) {
	query := `
		INSERT INTO vault_secrets (name, encrypted_value, description, scope, rotation_interval_days, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, name, description, scope, rotation_interval_days, last_rotated_at, expires_at, created_by, created_at, updated_at`

	secret := &Secret{EncryptedValue: input.EncryptedValue}
	err := s.db.Pool.QueryRow(ctx, query,
		input.Name, input.EncryptedValue, input.Description, input.Scope,
		input.RotationIntervalDays, input.CreatedBy,
	).Scan(
		&secret.ID, &secret.Name, &secret.Description, &secret.Scope,
		&secret.RotationIntervalDays, &secret.LastRotatedAt, &secret.ExpiresAt,
		&secret.CreatedBy, &secret.CreatedAt, &secret.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating secret: %w", err)
	}
	return secret, nil
}

// GetByName retrieves a secret by name. Returns encrypted value.
func (s *SecretStore) GetByName(ctx context.Context, name string) (*Secret, error) {
	query := `
		SELECT id, name, encrypted_value, description, scope, rotation_interval_days,
		       last_rotated_at, expires_at, created_by, created_at, updated_at
		FROM vault_secrets WHERE name = $1`

	secret := &Secret{}
	err := s.db.Pool.QueryRow(ctx, query, name).Scan(
		&secret.ID, &secret.Name, &secret.EncryptedValue, &secret.Description,
		&secret.Scope, &secret.RotationIntervalDays, &secret.LastRotatedAt,
		&secret.ExpiresAt, &secret.CreatedBy, &secret.CreatedAt, &secret.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting secret: %w", err)
	}
	return secret, nil
}

// List returns all secrets (without values).
func (s *SecretStore) List(ctx context.Context) ([]Secret, error) {
	query := `
		SELECT id, name, description, scope, rotation_interval_days,
		       last_rotated_at, expires_at, created_by, created_at, updated_at
		FROM vault_secrets ORDER BY name`

	rows, err := s.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing secrets: %w", err)
	}
	defer rows.Close()

	var secrets []Secret
	for rows.Next() {
		var s Secret
		if err := rows.Scan(
			&s.ID, &s.Name, &s.Description, &s.Scope, &s.RotationIntervalDays,
			&s.LastRotatedAt, &s.ExpiresAt, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning secret: %w", err)
		}
		secrets = append(secrets, s)
	}
	return secrets, rows.Err()
}

// Update updates a secret's encrypted value.
func (s *SecretStore) Update(ctx context.Context, name, encryptedValue string) error {
	_, err := s.db.Pool.Exec(ctx,
		"UPDATE vault_secrets SET encrypted_value = $1 WHERE name = $2",
		encryptedValue, name)
	return err
}

// Delete removes a secret.
func (s *SecretStore) Delete(ctx context.Context, name string) error {
	ct, err := s.db.Pool.Exec(ctx, "DELETE FROM vault_secrets WHERE name = $1", name)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("not found")
	}
	return nil
}

// Rotate stores the old value in history and updates with new value.
func (s *SecretStore) Rotate(ctx context.Context, name, newEncryptedValue, rotatedBy string) error {
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get current secret
	var secretID, oldValue string
	err = tx.QueryRow(ctx,
		"SELECT id, encrypted_value FROM vault_secrets WHERE name = $1", name,
	).Scan(&secretID, &oldValue)
	if err != nil {
		return fmt.Errorf("getting current secret: %w", err)
	}

	// Save old value to history
	_, err = tx.Exec(ctx,
		"INSERT INTO vault_secret_history (secret_id, encrypted_value, rotated_by) VALUES ($1, $2, $3)",
		secretID, oldValue, rotatedBy)
	if err != nil {
		return fmt.Errorf("saving history: %w", err)
	}

	// Update with new value
	_, err = tx.Exec(ctx,
		"UPDATE vault_secrets SET encrypted_value = $1, last_rotated_at = NOW() WHERE name = $2",
		newEncryptedValue, name)
	if err != nil {
		return fmt.Errorf("updating secret: %w", err)
	}

	return tx.Commit(ctx)
}

// CanAccess checks if an agent has access to a secret.
func (s *SecretStore) CanAccess(secret *Secret, agentID string) bool {
	if agentID == "warren" {
		return true
	}
	if len(secret.Scope) == 0 {
		return false // admin-only
	}
	for _, a := range secret.Scope {
		if a == agentID || a == "*" {
			return true
		}
	}
	return false
}

// Count returns the total number of secrets.
func (s *SecretStore) Count(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM vault_secrets").Scan(&count)
	return count, err
}
