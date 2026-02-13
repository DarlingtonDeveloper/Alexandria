package semantic

import (
	"context"

	"github.com/pgvector/pgvector-go"

	"github.com/warrentherabbit/alexandria/internal/store"
)

// embedBatch finds entities needing embeddings and computes them in a batch.
func (w *Worker) embedBatch(ctx context.Context) error {
	db := w.db.DBTX()

	// Phase 1: entities with no embedding
	ids, err := store.EntitiesWithoutEmbeddings(ctx, db, w.config.EmbedBatchSize)
	if err != nil {
		return err
	}

	// Phase 2: stale embeddings (entity updated after embedding)
	remaining := w.config.EmbedBatchSize - len(ids)
	if remaining > 0 {
		staleIDs, err := store.EntitiesWithStaleEmbeddings(ctx, db, remaining)
		if err != nil {
			return err
		}
		ids = append(ids, staleIDs...)
	}

	if len(ids) == 0 {
		return nil
	}

	w.logger.Info("embedding entities", "count", len(ids))

	// Fetch entities and build text
	texts := make([]string, 0, len(ids))
	entities := make([]*store.CGEntity, 0, len(ids))
	for _, id := range ids {
		entity, err := store.GetEntityTx(ctx, db, id)
		if err != nil {
			w.logger.Warn("embed skip entity", "id", id, "error", err)
			continue
		}
		entities = append(entities, entity)
		texts = append(texts, EntityText(entity))
	}

	if len(texts) == 0 {
		return nil
	}

	// Call embedding provider
	vectors, err := w.provider.Embed(ctx, texts)
	if err != nil {
		return err
	}

	// Store embeddings
	stored := 0
	for i, entity := range entities {
		if i >= len(vectors) || vectors[i] == nil {
			continue
		}
		emb := &store.EntityEmbedding{
			EntityID:  entity.ID,
			Embedding: pgvector.NewVector(vectors[i]),
			Model:     w.provider.Model(),
			TextHash:  TextHash(texts[i]),
		}
		if err := store.UpsertEntityEmbedding(ctx, db, emb); err != nil {
			w.logger.Warn("embed store failed", "id", entity.ID, "error", err)
			continue
		}
		stored++
	}

	w.logger.Info("embedded entities", "stored", stored)
	return nil
}
