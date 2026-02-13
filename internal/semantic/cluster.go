package semantic

import (
	"context"
	"math"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"

	"github.com/MikeSquared-Agency/Alexandria/internal/store"
)

// detectClusters assigns entities to clusters, recomputes centroids, and detects convergence.
func (w *Worker) detectClusters(ctx context.Context) error {
	db := w.db.DBTX()

	// Step 1: Assign unassigned entities to nearest cluster or create new ones
	entities, err := store.ListEntitiesTx(ctx, db, "")
	if err != nil {
		return err
	}

	for _, entity := range entities {
		if entity.DeletedAt != nil {
			continue
		}
		emb, err := store.GetEntityEmbedding(ctx, db, entity.ID)
		if err != nil {
			continue // no embedding yet
		}

		clusters, err := store.EntityClusters(ctx, db, entity.ID)
		if err != nil {
			continue
		}
		if len(clusters) > 0 {
			continue // already assigned
		}

		nearest, err := store.NearestClusters(ctx, db, emb.Embedding, 1, w.config.ClusterJoinThreshold)
		if err != nil {
			continue
		}

		if len(nearest) > 0 {
			membership := &store.ClusterMembership{
				EntityID:  entity.ID,
				ClusterID: nearest[0].ClusterID,
				Distance:  nearest[0].Distance,
			}
			if err := store.AddClusterMember(ctx, db, membership); err != nil {
				w.logger.Warn("cluster add member", "entity", entity.ID, "cluster", nearest[0].ClusterID, "error", err)
			}
		} else {
			cluster := &store.SemanticCluster{
				Label:    entity.DisplayName,
				Centroid: emb.Embedding,
			}
			if err := store.CreateCluster(ctx, db, cluster); err != nil {
				w.logger.Warn("cluster create", "error", err)
				continue
			}
			membership := &store.ClusterMembership{
				EntityID:  entity.ID,
				ClusterID: cluster.ID,
				Distance:  0,
			}
			if err := store.AddClusterMember(ctx, db, membership); err != nil {
				w.logger.Warn("cluster add seed", "entity", entity.ID, "error", err)
			}
		}
	}

	// Step 2: Recompute centroids
	if err := w.recomputeCentroids(ctx); err != nil {
		w.logger.Warn("recompute centroids", "error", err)
	}

	// Step 3: Detect convergence
	if err := w.detectConvergence(ctx); err != nil {
		w.logger.Warn("convergence detection", "error", err)
	}

	return nil
}

func (w *Worker) recomputeCentroids(ctx context.Context) error {
	db := w.db.DBTX()
	clusters, err := store.ListActiveClusters(ctx, db)
	if err != nil {
		return err
	}

	for _, cluster := range clusters {
		members, err := store.ClusterMembers(ctx, db, cluster.ID)
		if err != nil {
			continue
		}
		if len(members) == 0 {
			store.DissolveCluster(ctx, db, cluster.ID)
			continue
		}

		var vectors [][]float32
		for _, m := range members {
			emb, err := store.GetEntityEmbedding(ctx, db, m.EntityID)
			if err != nil {
				continue
			}
			vectors = append(vectors, emb.Embedding.Slice())
		}
		if len(vectors) == 0 {
			continue
		}

		centroid := averageVectors(vectors)
		store.UpdateClusterCentroid(ctx, db, cluster.ID, pgvector.NewVector(centroid))
	}
	return nil
}

func (w *Worker) detectConvergence(ctx context.Context) error {
	db := w.db.DBTX()
	clusters, err := store.ListActiveClusters(ctx, db)
	if err != nil {
		return err
	}

	for i := 0; i < len(clusters); i++ {
		for j := i + 1; j < len(clusters); j++ {
			ci, cj := clusters[i], clusters[j]
			if ci.Centroid.Slice() == nil || cj.Centroid.Slice() == nil {
				continue
			}
			similarity := cosineSimilarity(ci.Centroid.Slice(), cj.Centroid.Slice())

			if similarity >= w.config.AutoMergeThreshold {
				w.logger.Info("auto-merging clusters", "a", ci.ID, "b", cj.ID, "similarity", similarity)
				if err := w.mergeClusters(ctx, ci.ID, cj.ID); err != nil {
					w.logger.Warn("cluster merge failed", "error", err)
				}
			} else if similarity >= w.config.MergeProposalThreshold {
				membersA, _ := store.ClusterMembers(ctx, db, ci.ID)
				membersB, _ := store.ClusterMembers(ctx, db, cj.ID)
				if len(membersA) > 0 && len(membersB) > 0 {
					proposal := &store.MergeProposal{
						EntityAID:    membersA[0].EntityID,
						EntityBID:    membersB[0].EntityID,
						Similarity:   similarity,
						ProposalType: "cluster",
						ClusterAID:   &ci.ID,
						ClusterBID:   &cj.ID,
						Status:       "pending",
					}
					store.CreateMergeProposal(ctx, db, proposal)
				}
			}
		}
	}
	return nil
}

func (w *Worker) mergeClusters(ctx context.Context, keepID, dissolveID uuid.UUID) error {
	db := w.db.DBTX()
	members, err := store.ClusterMembers(ctx, db, dissolveID)
	if err != nil {
		return err
	}

	for _, m := range members {
		store.RemoveClusterMember(ctx, db, m.EntityID, dissolveID)
		newMembership := &store.ClusterMembership{
			EntityID:  m.EntityID,
			ClusterID: keepID,
			Distance:  m.Distance,
		}
		store.AddClusterMember(ctx, db, newMembership)
	}

	return store.DissolveCluster(ctx, db, dissolveID)
}

func averageVectors(vectors [][]float32) []float32 {
	if len(vectors) == 0 {
		return nil
	}
	dim := len(vectors[0])
	avg := make([]float32, dim)
	for _, v := range vectors {
		for i := range avg {
			if i < len(v) {
				avg[i] += v[i]
			}
		}
	}
	n := float32(len(vectors))
	for i := range avg {
		avg[i] /= n
	}
	return avg
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
