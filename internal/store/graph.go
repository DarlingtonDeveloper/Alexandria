package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// EntityType represents the type of a graph entity.
type EntityType string

const (
	EntityPerson     EntityType = "person"
	EntityAgent      EntityType = "agent"
	EntityService    EntityType = "service"
	EntityProject    EntityType = "project"
	EntityConcept    EntityType = "concept"
	EntityCredential EntityType = "credential"
)

// Entity represents a knowledge graph entity.
type Entity struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	EntityType EntityType     `json:"entity_type"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// Relationship represents a connection between two entities.
type Relationship struct {
	ID               string   `json:"id"`
	SourceEntityID   string   `json:"source_entity_id"`
	TargetEntityID   string   `json:"target_entity_id"`
	RelationshipType string   `json:"relationship_type"`
	Strength         float64  `json:"strength"`
	KnowledgeID      *string  `json:"knowledge_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// GraphStore provides knowledge graph operations.
type GraphStore struct {
	db *DB
}

// NewGraphStore creates a new GraphStore.
func NewGraphStore(db *DB) *GraphStore {
	return &GraphStore{db: db}
}

// CreateEntity inserts a new entity (upserts on name+type).
func (s *GraphStore) CreateEntity(ctx context.Context, name string, entityType EntityType, metadata map[string]any) (*Entity, error) {
	query := `
		INSERT INTO vault_entities (name, entity_type, metadata)
		VALUES ($1, $2, $3)
		ON CONFLICT (name, entity_type) DO UPDATE SET metadata = EXCLUDED.metadata
		RETURNING id, name, entity_type, metadata, created_at, updated_at`

	e := &Entity{}
	err := s.db.Pool.QueryRow(ctx, query, name, entityType, metadata).Scan(
		&e.ID, &e.Name, &e.EntityType, &e.Metadata, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating entity: %w", err)
	}
	return e, nil
}

// GetEntity retrieves an entity by ID.
func (s *GraphStore) GetEntity(ctx context.Context, id string) (*Entity, error) {
	e := &Entity{}
	err := s.db.Pool.QueryRow(ctx,
		`SELECT id, name, entity_type, metadata, created_at, updated_at FROM vault_entities WHERE id = $1`, id,
	).Scan(&e.ID, &e.Name, &e.EntityType, &e.Metadata, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting entity: %w", err)
	}
	return e, nil
}

// ListEntities lists entities with optional type filter.
func (s *GraphStore) ListEntities(ctx context.Context, entityType *EntityType, limit, offset int) ([]Entity, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var query string
	var args []any
	if entityType != nil {
		query = `SELECT id, name, entity_type, metadata, created_at, updated_at
			FROM vault_entities WHERE entity_type = $1 ORDER BY name LIMIT $2 OFFSET $3`
		args = []any{*entityType, limit, offset}
	} else {
		query = `SELECT id, name, entity_type, metadata, created_at, updated_at
			FROM vault_entities ORDER BY name LIMIT $1 OFFSET $2`
		args = []any{limit, offset}
	}

	rows, err := s.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing entities: %w", err)
	}
	defer rows.Close()

	var entities []Entity
	for rows.Next() {
		var e Entity
		if err := rows.Scan(&e.ID, &e.Name, &e.EntityType, &e.Metadata, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning entity: %w", err)
		}
		entities = append(entities, e)
	}
	return entities, rows.Err()
}

// CreateRelationship inserts a relationship (upserts on source+target+type).
func (s *GraphStore) CreateRelationship(ctx context.Context, sourceID, targetID, relType string, strength float64, knowledgeID *string) (*Relationship, error) {
	query := `
		INSERT INTO vault_relationships (source_entity_id, target_entity_id, relationship_type, strength, knowledge_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (source_entity_id, target_entity_id, relationship_type)
		DO UPDATE SET strength = EXCLUDED.strength, knowledge_id = EXCLUDED.knowledge_id
		RETURNING id, source_entity_id, target_entity_id, relationship_type, strength, knowledge_id, created_at`

	r := &Relationship{}
	err := s.db.Pool.QueryRow(ctx, query, sourceID, targetID, relType, strength, knowledgeID).Scan(
		&r.ID, &r.SourceEntityID, &r.TargetEntityID, &r.RelationshipType,
		&r.Strength, &r.KnowledgeID, &r.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating relationship: %w", err)
	}
	return r, nil
}

// GetRelationships retrieves all relationships for an entity (both directions).
func (s *GraphStore) GetRelationships(ctx context.Context, entityID string) ([]Relationship, error) {
	query := `
		SELECT id, source_entity_id, target_entity_id, relationship_type, strength, knowledge_id, created_at
		FROM vault_relationships
		WHERE source_entity_id = $1 OR target_entity_id = $1
		ORDER BY strength DESC`

	rows, err := s.db.Pool.Query(ctx, query, entityID)
	if err != nil {
		return nil, fmt.Errorf("getting relationships: %w", err)
	}
	defer rows.Close()

	var rels []Relationship
	for rows.Next() {
		var r Relationship
		if err := rows.Scan(&r.ID, &r.SourceEntityID, &r.TargetEntityID, &r.RelationshipType,
			&r.Strength, &r.KnowledgeID, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning relationship: %w", err)
		}
		rels = append(rels, r)
	}
	return rels, rows.Err()
}

// GetRelatedEntities returns entities related to a given entity up to maxDepth hops.
func (s *GraphStore) GetRelatedEntities(ctx context.Context, entityID string, maxDepth int) ([]Entity, []Relationship, error) {
	if maxDepth <= 0 || maxDepth > 3 {
		maxDepth = 2
	}

	// BFS traversal
	visited := map[string]bool{entityID: true}
	var allRels []Relationship
	frontier := []string{entityID}

	for depth := 0; depth < maxDepth && len(frontier) > 0; depth++ {
		var nextFrontier []string
		for _, eid := range frontier {
			rels, err := s.GetRelationships(ctx, eid)
			if err != nil {
				return nil, nil, err
			}
			for _, r := range rels {
				allRels = append(allRels, r)
				other := r.TargetEntityID
				if other == eid {
					other = r.SourceEntityID
				}
				if !visited[other] {
					visited[other] = true
					nextFrontier = append(nextFrontier, other)
				}
			}
		}
		frontier = nextFrontier
	}

	// Fetch all visited entities (except the starting one)
	var entities []Entity
	for eid := range visited {
		if eid == entityID {
			continue
		}
		e, err := s.GetEntity(ctx, eid)
		if err != nil {
			return nil, nil, err
		}
		if e != nil {
			entities = append(entities, *e)
		}
	}

	return entities, allRels, nil
}
