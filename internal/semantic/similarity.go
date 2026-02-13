package semantic

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/MikeSquared-Agency/Alexandria/internal/store"
)

// scanSimilarity finds semantically similar entities and creates auto-edges.
func (w *Worker) scanSimilarity(ctx context.Context) error {
	db := w.db.DBTX()

	entities, err := store.ListEntitiesTx(ctx, db, "")
	if err != nil {
		return err
	}

	created := 0
	for _, entity := range entities {
		if entity.DeletedAt != nil {
			continue
		}

		similar, err := store.FindSimilarToEntity(ctx, db, entity.ID, 10, w.config.EdgeThreshold)
		if err != nil {
			continue // no embedding yet
		}

		for _, s := range similar {
			fromID, toID := canonicalOrder(entity.ID, s.EntityID)
			if fromID != entity.ID {
				continue
			}

			edge := &store.CGEdge{
				FromID:     fromID,
				ToID:       toID,
				Type:       "semantic_similarity",
				Confidence: s.Similarity,
				Source:     "semantic-scanner",
				Metadata:   json.RawMessage(`{"auto_generated":true}`),
			}
			if err := store.UpsertSemanticEdge(ctx, db, edge); err != nil {
				continue
			}
			created++
		}
	}

	if created > 0 {
		w.logger.Info("similarity edges created", "count", created)
	}
	return nil
}

// canonicalOrder returns UUIDs in canonical order (smaller first).
func canonicalOrder(a, b uuid.UUID) (uuid.UUID, uuid.UUID) {
	if a.String() < b.String() {
		return a, b
	}
	return b, a
}
