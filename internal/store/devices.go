package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Device represents a device in the vault.
type Device struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	DeviceType string                 `json:"device_type"`
	OwnerID    *string                `json:"owner_id,omitempty"`
	Identifier string                 `json:"identifier"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	LastSeen   *time.Time             `json:"last_seen,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

// DeviceCreateInput is the input for creating a device.
type DeviceCreateInput struct {
	Name       string                 `json:"name"`
	DeviceType string                 `json:"device_type"`
	OwnerID    *string                `json:"owner_id,omitempty"`
	Identifier string                 `json:"identifier"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// DeviceUpdateInput is the input for updating a device.
type DeviceUpdateInput struct {
	Name       *string                `json:"name,omitempty"`
	DeviceType *string                `json:"device_type,omitempty"`
	OwnerID    *string                `json:"owner_id,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// DeviceStore provides device CRUD operations.
type DeviceStore struct {
	db *DB
}

// NewDeviceStore creates a new DeviceStore.
func NewDeviceStore(db *DB) *DeviceStore {
	return &DeviceStore{db: db}
}

// Create inserts a new device.
func (s *DeviceStore) Create(ctx context.Context, input DeviceCreateInput) (*Device, error) {
	query := `
		INSERT INTO vault_devices (name, device_type, owner_id, identifier, metadata)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, device_type, owner_id, identifier, metadata, last_seen, created_at, updated_at`

	device := &Device{}
	err := s.db.Pool.QueryRow(ctx, query,
		input.Name, input.DeviceType, input.OwnerID, input.Identifier, input.Metadata,
	).Scan(
		&device.ID, &device.Name, &device.DeviceType, &device.OwnerID,
		&device.Identifier, &device.Metadata, &device.LastSeen,
		&device.CreatedAt, &device.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating device: %w", err)
	}
	return device, nil
}

// GetByID retrieves a device by ID.
func (s *DeviceStore) GetByID(ctx context.Context, id string) (*Device, error) {
	query := `
		SELECT id, name, device_type, owner_id, identifier, metadata, last_seen, created_at, updated_at
		FROM vault_devices WHERE id = $1`

	device := &Device{}
	err := s.db.Pool.QueryRow(ctx, query, id).Scan(
		&device.ID, &device.Name, &device.DeviceType, &device.OwnerID,
		&device.Identifier, &device.Metadata, &device.LastSeen,
		&device.CreatedAt, &device.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting device: %w", err)
	}
	return device, nil
}

// GetByIdentifier retrieves a device by identifier.
func (s *DeviceStore) GetByIdentifier(ctx context.Context, identifier string) (*Device, error) {
	query := `
		SELECT id, name, device_type, owner_id, identifier, metadata, last_seen, created_at, updated_at
		FROM vault_devices WHERE identifier = $1`

	device := &Device{}
	err := s.db.Pool.QueryRow(ctx, query, identifier).Scan(
		&device.ID, &device.Name, &device.DeviceType, &device.OwnerID,
		&device.Identifier, &device.Metadata, &device.LastSeen,
		&device.CreatedAt, &device.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting device by identifier: %w", err)
	}
	return device, nil
}

// List returns all devices.
func (s *DeviceStore) List(ctx context.Context) ([]Device, error) {
	query := `
		SELECT id, name, device_type, owner_id, identifier, metadata, last_seen, created_at, updated_at
		FROM vault_devices ORDER BY name`

	rows, err := s.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing devices: %w", err)
	}
	defer rows.Close()

	var devices []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(
			&d.ID, &d.Name, &d.DeviceType, &d.OwnerID,
			&d.Identifier, &d.Metadata, &d.LastSeen,
			&d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning device: %w", err)
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

// ListByOwner returns devices owned by a specific person.
func (s *DeviceStore) ListByOwner(ctx context.Context, ownerID string) ([]Device, error) {
	query := `
		SELECT id, name, device_type, owner_id, identifier, metadata, last_seen, created_at, updated_at
		FROM vault_devices WHERE owner_id = $1 ORDER BY name`

	rows, err := s.db.Pool.Query(ctx, query, ownerID)
	if err != nil {
		return nil, fmt.Errorf("listing devices by owner: %w", err)
	}
	defer rows.Close()

	var devices []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(
			&d.ID, &d.Name, &d.DeviceType, &d.OwnerID,
			&d.Identifier, &d.Metadata, &d.LastSeen,
			&d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning device: %w", err)
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

// Update updates a device.
func (s *DeviceStore) Update(ctx context.Context, id string, input DeviceUpdateInput) (*Device, error) {
	// Build dynamic query
	setParts := []string{}
	args := []interface{}{}
	argCount := 0

	if input.Name != nil {
		argCount++
		setParts = append(setParts, fmt.Sprintf("name = $%d", argCount))
		args = append(args, *input.Name)
	}

	if input.DeviceType != nil {
		argCount++
		setParts = append(setParts, fmt.Sprintf("device_type = $%d", argCount))
		args = append(args, *input.DeviceType)
	}

	if input.OwnerID != nil {
		argCount++
		setParts = append(setParts, fmt.Sprintf("owner_id = $%d", argCount))
		args = append(args, *input.OwnerID)
	}

	if input.Metadata != nil {
		argCount++
		setParts = append(setParts, fmt.Sprintf("metadata = $%d", argCount))
		args = append(args, input.Metadata)
	}

	if len(setParts) == 0 {
		return s.GetByID(ctx, id)
	}

	argCount++
	args = append(args, id)

	setClause := setParts[0]
	for i := 1; i < len(setParts); i++ {
		setClause += ", " + setParts[i]
	}

	query := fmt.Sprintf(`
		UPDATE vault_devices SET %s
		WHERE id = $%d
		RETURNING id, name, device_type, owner_id, identifier, metadata, last_seen, created_at, updated_at`,
		setClause,
		argCount,
	)

	device := &Device{}
	err := s.db.Pool.QueryRow(ctx, query, args...).Scan(
		&device.ID, &device.Name, &device.DeviceType, &device.OwnerID,
		&device.Identifier, &device.Metadata, &device.LastSeen,
		&device.CreatedAt, &device.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("device not found")
		}
		return nil, fmt.Errorf("updating device: %w", err)
	}
	return device, nil
}

// UpdateLastSeen updates the last_seen timestamp for a device.
func (s *DeviceStore) UpdateLastSeen(ctx context.Context, identifier string) error {
	_, err := s.db.Pool.Exec(ctx,
		"UPDATE vault_devices SET last_seen = NOW() WHERE identifier = $1",
		identifier)
	return err
}

// Delete removes a device.
func (s *DeviceStore) Delete(ctx context.Context, id string) error {
	ct, err := s.db.Pool.Exec(ctx, "DELETE FROM vault_devices WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("deleting device: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("device not found")
	}
	return nil
}

// Count returns the total number of devices.
func (s *DeviceStore) Count(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM vault_devices").Scan(&count)
	return count, err
}
