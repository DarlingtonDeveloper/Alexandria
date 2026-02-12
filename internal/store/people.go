package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Person represents a person in the vault.
type Person struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Identifier string                 `json:"identifier"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

// PersonCreateInput is the input for creating a person.
type PersonCreateInput struct {
	Name       string                 `json:"name"`
	Identifier string                 `json:"identifier"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// PersonUpdateInput is the input for updating a person.
type PersonUpdateInput struct {
	Name     *string                `json:"name,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// PersonStore provides person CRUD operations.
type PersonStore struct {
	db *DB
}

// NewPersonStore creates a new PersonStore.
func NewPersonStore(db *DB) *PersonStore {
	return &PersonStore{db: db}
}

// Create inserts a new person.
func (s *PersonStore) Create(ctx context.Context, input PersonCreateInput) (*Person, error) {
	query := `
		INSERT INTO vault_people (name, identifier, metadata)
		VALUES ($1, $2, $3)
		RETURNING id, name, identifier, metadata, created_at, updated_at`

	person := &Person{}
	err := s.db.Pool.QueryRow(ctx, query,
		input.Name, input.Identifier, input.Metadata,
	).Scan(
		&person.ID, &person.Name, &person.Identifier,
		&person.Metadata, &person.CreatedAt, &person.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating person: %w", err)
	}
	return person, nil
}

// GetByID retrieves a person by ID.
func (s *PersonStore) GetByID(ctx context.Context, id string) (*Person, error) {
	query := `
		SELECT id, name, identifier, metadata, created_at, updated_at
		FROM vault_people WHERE id = $1`

	person := &Person{}
	err := s.db.Pool.QueryRow(ctx, query, id).Scan(
		&person.ID, &person.Name, &person.Identifier,
		&person.Metadata, &person.CreatedAt, &person.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting person: %w", err)
	}
	return person, nil
}

// GetByIdentifier retrieves a person by identifier.
func (s *PersonStore) GetByIdentifier(ctx context.Context, identifier string) (*Person, error) {
	query := `
		SELECT id, name, identifier, metadata, created_at, updated_at
		FROM vault_people WHERE identifier = $1`

	person := &Person{}
	err := s.db.Pool.QueryRow(ctx, query, identifier).Scan(
		&person.ID, &person.Name, &person.Identifier,
		&person.Metadata, &person.CreatedAt, &person.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting person by identifier: %w", err)
	}
	return person, nil
}

// List returns all people.
func (s *PersonStore) List(ctx context.Context) ([]Person, error) {
	query := `
		SELECT id, name, identifier, metadata, created_at, updated_at
		FROM vault_people ORDER BY name`

	rows, err := s.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing people: %w", err)
	}
	defer rows.Close()

	var people []Person
	for rows.Next() {
		var p Person
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Identifier,
			&p.Metadata, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning person: %w", err)
		}
		people = append(people, p)
	}
	return people, rows.Err()
}

// Update updates a person.
func (s *PersonStore) Update(ctx context.Context, id string, input PersonUpdateInput) (*Person, error) {
	// Build dynamic query
	setParts := []string{}
	args := []interface{}{}
	argCount := 0

	if input.Name != nil {
		argCount++
		setParts = append(setParts, fmt.Sprintf("name = $%d", argCount))
		args = append(args, *input.Name)
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

	query := fmt.Sprintf(`
		UPDATE vault_people SET %s
		WHERE id = $%d
		RETURNING id, name, identifier, metadata, created_at, updated_at`,
		string(setParts[0]),
		argCount,
	)

	// Join setParts properly
	if len(setParts) > 1 {
		setClause := setParts[0]
		for i := 1; i < len(setParts); i++ {
			setClause += ", " + setParts[i]
		}
		query = fmt.Sprintf(`
			UPDATE vault_people SET %s
			WHERE id = $%d
			RETURNING id, name, identifier, metadata, created_at, updated_at`,
			setClause,
			argCount,
		)
	}

	person := &Person{}
	err := s.db.Pool.QueryRow(ctx, query, args...).Scan(
		&person.ID, &person.Name, &person.Identifier,
		&person.Metadata, &person.CreatedAt, &person.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("person not found")
		}
		return nil, fmt.Errorf("updating person: %w", err)
	}
	return person, nil
}

// Delete removes a person.
func (s *PersonStore) Delete(ctx context.Context, id string) error {
	ct, err := s.db.Pool.Exec(ctx, "DELETE FROM vault_people WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("deleting person: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("person not found")
	}
	return nil
}

// Count returns the total number of people.
func (s *PersonStore) Count(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM vault_people").Scan(&count)
	return count, err
}
