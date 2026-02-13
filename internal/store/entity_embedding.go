package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
)

// EntityEmbedding represents a stored vector embedding for an entity.
type EntityEmbedding struct {
	EntityID  uuid.UUID       `json:"entity_id"`
	Embedding pgvector.Vector `json:"-"`
	Model     string          `json:"model"`
	TextHash  string          `json:"text_hash"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// SimilarEntity is returned by nearest-neighbor queries.
type SimilarEntity struct {
	EntityID   uuid.UUID `json:"entity_id"`
	Distance   float64   `json:"distance"`
	Similarity float64   `json:"similarity"`
}

// UpsertEntityEmbedding inserts or updates an embedding for an entity.
func UpsertEntityEmbedding(ctx context.Context, db DBTX, e *EntityEmbedding) error {
	return db.QueryRow(ctx, `
		INSERT INTO vault_entity_embeddings (entity_id, embedding, model, text_hash)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (entity_id) DO UPDATE SET
			embedding = EXCLUDED.embedding,
			model = EXCLUDED.model,
			text_hash = EXCLUDED.text_hash,
			updated_at = now()
		RETURNING created_at, updated_at
	`, e.EntityID, e.Embedding, e.Model, e.TextHash).
		Scan(&e.CreatedAt, &e.UpdatedAt)
}

// GetEntityEmbedding fetches an embedding by entity ID.
func GetEntityEmbedding(ctx context.Context, db DBTX, entityID uuid.UUID) (*EntityEmbedding, error) {
	e := &EntityEmbedding{}
	err := db.QueryRow(ctx, `
		SELECT entity_id, embedding, model, text_hash, created_at, updated_at
		FROM vault_entity_embeddings WHERE entity_id = $1
	`, entityID).Scan(&e.EntityID, &e.Embedding, &e.Model, &e.TextHash, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get embedding %s: %w", entityID, err)
	}
	return e, nil
}

// FindSimilarEntities finds the top-N entities nearest to a given embedding.
func FindSimilarEntities(ctx context.Context, db DBTX, embedding pgvector.Vector, limit int, minSimilarity float64) ([]SimilarEntity, error) {
	maxDistance := 1.0 - minSimilarity
	rows, err := db.Query(ctx, `
		SELECT e.entity_id, e.embedding <=> $1 AS distance
		FROM vault_entity_embeddings e
		JOIN vault_entities ent ON ent.id = e.entity_id AND ent.deleted_at IS NULL
		WHERE e.embedding <=> $1 < $2
		ORDER BY distance
		LIMIT $3
	`, embedding, maxDistance, limit)
	if err != nil {
		return nil, fmt.Errorf("find similar: %w", err)
	}
	defer rows.Close()

	var result []SimilarEntity
	for rows.Next() {
		var s SimilarEntity
		if err := rows.Scan(&s.EntityID, &s.Distance); err != nil {
			return nil, fmt.Errorf("scan similar: %w", err)
		}
		s.Similarity = 1.0 - s.Distance
		result = append(result, s)
	}
	return result, rows.Err()
}

// FindSimilarToEntity finds entities nearest to a specific entity's embedding.
func FindSimilarToEntity(ctx context.Context, db DBTX, entityID uuid.UUID, limit int, minSimilarity float64) ([]SimilarEntity, error) {
	maxDistance := 1.0 - minSimilarity
	rows, err := db.Query(ctx, `
		SELECT e2.entity_id, e1.embedding <=> e2.embedding AS distance
		FROM vault_entity_embeddings e1
		JOIN vault_entity_embeddings e2 ON e2.entity_id != e1.entity_id
		JOIN vault_entities ent ON ent.id = e2.entity_id AND ent.deleted_at IS NULL
		WHERE e1.entity_id = $1
		  AND e1.embedding <=> e2.embedding < $2
		ORDER BY distance
		LIMIT $3
	`, entityID, maxDistance, limit)
	if err != nil {
		return nil, fmt.Errorf("find similar to entity: %w", err)
	}
	defer rows.Close()

	var result []SimilarEntity
	for rows.Next() {
		var s SimilarEntity
		if err := rows.Scan(&s.EntityID, &s.Distance); err != nil {
			return nil, fmt.Errorf("scan similar: %w", err)
		}
		s.Similarity = 1.0 - s.Distance
		result = append(result, s)
	}
	return result, rows.Err()
}

// EntitiesWithoutEmbeddings returns entity IDs that have no embedding yet.
func EntitiesWithoutEmbeddings(ctx context.Context, db DBTX, limit int) ([]uuid.UUID, error) {
	rows, err := db.Query(ctx, `
		SELECT e.id FROM vault_entities e
		LEFT JOIN vault_entity_embeddings emb ON emb.entity_id = e.id
		WHERE emb.entity_id IS NULL AND e.deleted_at IS NULL
		ORDER BY e.created_at
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("entities without embeddings: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// EntitiesWithStaleEmbeddings returns entities whose embedding is older than the entity's updated_at.
func EntitiesWithStaleEmbeddings(ctx context.Context, db DBTX, limit int) ([]uuid.UUID, error) {
	rows, err := db.Query(ctx, `
		SELECT e.id FROM vault_entities e
		JOIN vault_entity_embeddings emb ON emb.entity_id = e.id
		WHERE e.updated_at > emb.updated_at AND e.deleted_at IS NULL
		ORDER BY e.updated_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("stale embeddings: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
