package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
)

// SemanticCluster represents a group of semantically related entities.
type SemanticCluster struct {
	ID          uuid.UUID       `json:"id"`
	Label       string          `json:"label"`
	Centroid    pgvector.Vector `json:"-"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	DissolvedAt *time.Time      `json:"dissolved_at,omitempty"`
}

// ClusterMembership tracks an entity's membership in a cluster.
type ClusterMembership struct {
	EntityID  uuid.UUID  `json:"entity_id"`
	ClusterID uuid.UUID  `json:"cluster_id"`
	Distance  float64    `json:"distance"`
	JoinedAt  time.Time  `json:"joined_at"`
	LeftAt    *time.Time `json:"left_at,omitempty"`
}

// MergeProposal records a proposed merge between two entities or clusters.
type MergeProposal struct {
	ID           uuid.UUID  `json:"id"`
	EntityAID    uuid.UUID  `json:"entity_a_id"`
	EntityBID    uuid.UUID  `json:"entity_b_id"`
	Similarity   float64    `json:"similarity"`
	ProposalType string     `json:"proposal_type"`
	ClusterAID   *uuid.UUID `json:"cluster_a_id,omitempty"`
	ClusterBID   *uuid.UUID `json:"cluster_b_id,omitempty"`
	Status       string     `json:"status"`
	ReviewedBy   *string    `json:"reviewed_by,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
}

// ClusterDistance is returned by nearest-cluster queries.
type ClusterDistance struct {
	ClusterID  uuid.UUID `json:"cluster_id"`
	Distance   float64   `json:"distance"`
	Similarity float64   `json:"similarity"`
}

// --- Cluster CRUD ---

// CreateCluster inserts a new semantic cluster.
func CreateCluster(ctx context.Context, db DBTX, c *SemanticCluster) error {
	return db.QueryRow(ctx, `
		INSERT INTO vault_semantic_clusters (label, centroid)
		VALUES ($1, $2)
		RETURNING id, created_at, updated_at
	`, c.Label, c.Centroid).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

// UpdateClusterCentroid updates a cluster's centroid vector.
func UpdateClusterCentroid(ctx context.Context, db DBTX, clusterID uuid.UUID, centroid pgvector.Vector) error {
	_, err := db.Exec(ctx, `
		UPDATE vault_semantic_clusters SET centroid = $1, updated_at = now()
		WHERE id = $2 AND dissolved_at IS NULL
	`, centroid, clusterID)
	return err
}

// DissolveCluster marks a cluster as dissolved.
func DissolveCluster(ctx context.Context, db DBTX, clusterID uuid.UUID) error {
	_, err := db.Exec(ctx, `
		UPDATE vault_semantic_clusters SET dissolved_at = now(), updated_at = now()
		WHERE id = $1
	`, clusterID)
	return err
}

// ListActiveClusters returns all non-dissolved clusters.
func ListActiveClusters(ctx context.Context, db DBTX) ([]SemanticCluster, error) {
	rows, err := db.Query(ctx, `
		SELECT id, label, centroid, created_at, updated_at, dissolved_at
		FROM vault_semantic_clusters WHERE dissolved_at IS NULL
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}
	defer rows.Close()

	var result []SemanticCluster
	for rows.Next() {
		var c SemanticCluster
		if err := rows.Scan(&c.ID, &c.Label, &c.Centroid, &c.CreatedAt, &c.UpdatedAt, &c.DissolvedAt); err != nil {
			return nil, fmt.Errorf("scan cluster: %w", err)
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// --- Membership ---

// AddClusterMember adds an entity to a cluster.
func AddClusterMember(ctx context.Context, db DBTX, m *ClusterMembership) error {
	return db.QueryRow(ctx, `
		INSERT INTO vault_cluster_memberships (entity_id, cluster_id, distance)
		VALUES ($1, $2, $3)
		RETURNING joined_at
	`, m.EntityID, m.ClusterID, m.Distance).Scan(&m.JoinedAt)
}

// RemoveClusterMember removes an entity from a cluster.
func RemoveClusterMember(ctx context.Context, db DBTX, entityID, clusterID uuid.UUID) error {
	_, err := db.Exec(ctx, `
		UPDATE vault_cluster_memberships SET left_at = now()
		WHERE entity_id = $1 AND cluster_id = $2 AND left_at IS NULL
	`, entityID, clusterID)
	return err
}

// ClusterMembers returns active members of a cluster.
func ClusterMembers(ctx context.Context, db DBTX, clusterID uuid.UUID) ([]ClusterMembership, error) {
	rows, err := db.Query(ctx, `
		SELECT entity_id, cluster_id, distance, joined_at, left_at
		FROM vault_cluster_memberships
		WHERE cluster_id = $1 AND left_at IS NULL
		ORDER BY distance
	`, clusterID)
	if err != nil {
		return nil, fmt.Errorf("cluster members: %w", err)
	}
	defer rows.Close()

	var result []ClusterMembership
	for rows.Next() {
		var m ClusterMembership
		if err := rows.Scan(&m.EntityID, &m.ClusterID, &m.Distance, &m.JoinedAt, &m.LeftAt); err != nil {
			return nil, fmt.Errorf("scan membership: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// EntityClusters returns active clusters an entity belongs to.
func EntityClusters(ctx context.Context, db DBTX, entityID uuid.UUID) ([]SemanticCluster, error) {
	rows, err := db.Query(ctx, `
		SELECT sc.id, sc.label, sc.centroid, sc.created_at, sc.updated_at, sc.dissolved_at
		FROM vault_semantic_clusters sc
		JOIN vault_cluster_memberships cm ON cm.cluster_id = sc.id
		WHERE cm.entity_id = $1 AND cm.left_at IS NULL AND sc.dissolved_at IS NULL
		ORDER BY sc.created_at
	`, entityID)
	if err != nil {
		return nil, fmt.Errorf("entity clusters: %w", err)
	}
	defer rows.Close()

	var result []SemanticCluster
	for rows.Next() {
		var c SemanticCluster
		if err := rows.Scan(&c.ID, &c.Label, &c.Centroid, &c.CreatedAt, &c.UpdatedAt, &c.DissolvedAt); err != nil {
			return nil, fmt.Errorf("scan cluster: %w", err)
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// NearestClusters finds clusters whose centroids are nearest to a given embedding.
func NearestClusters(ctx context.Context, db DBTX, embedding pgvector.Vector, limit int, minSimilarity float64) ([]ClusterDistance, error) {
	maxDistance := 1.0 - minSimilarity
	rows, err := db.Query(ctx, `
		SELECT id, centroid <=> $1 AS distance
		FROM vault_semantic_clusters
		WHERE dissolved_at IS NULL AND centroid IS NOT NULL
		  AND centroid <=> $1 < $2
		ORDER BY distance
		LIMIT $3
	`, embedding, maxDistance, limit)
	if err != nil {
		return nil, fmt.Errorf("nearest clusters: %w", err)
	}
	defer rows.Close()

	var result []ClusterDistance
	for rows.Next() {
		var cd ClusterDistance
		if err := rows.Scan(&cd.ClusterID, &cd.Distance); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		cd.Similarity = 1.0 - cd.Distance
		result = append(result, cd)
	}
	return result, rows.Err()
}

// --- Merge Proposals ---

// CreateMergeProposal inserts or updates a merge proposal.
func CreateMergeProposal(ctx context.Context, db DBTX, p *MergeProposal) error {
	return db.QueryRow(ctx, `
		INSERT INTO vault_merge_proposals (entity_a_id, entity_b_id, similarity, proposal_type, cluster_a_id, cluster_b_id, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (entity_a_id, entity_b_id) DO UPDATE SET
			similarity = GREATEST(vault_merge_proposals.similarity, EXCLUDED.similarity)
		RETURNING id, created_at
	`, p.EntityAID, p.EntityBID, p.Similarity, p.ProposalType, p.ClusterAID, p.ClusterBID, p.Status).
		Scan(&p.ID, &p.CreatedAt)
}

// PendingMergeProposals returns pending merge proposals ordered by similarity.
func PendingMergeProposals(ctx context.Context, db DBTX) ([]MergeProposal, error) {
	rows, err := db.Query(ctx, `
		SELECT id, entity_a_id, entity_b_id, similarity, proposal_type,
		       cluster_a_id, cluster_b_id, status, reviewed_by, created_at, resolved_at
		FROM vault_merge_proposals WHERE status = 'pending'
		ORDER BY similarity DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("pending proposals: %w", err)
	}
	defer rows.Close()

	var result []MergeProposal
	for rows.Next() {
		var p MergeProposal
		if err := rows.Scan(&p.ID, &p.EntityAID, &p.EntityBID, &p.Similarity, &p.ProposalType,
			&p.ClusterAID, &p.ClusterBID, &p.Status, &p.ReviewedBy, &p.CreatedAt, &p.ResolvedAt); err != nil {
			return nil, fmt.Errorf("scan proposal: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// ResolveMergeProposal approves or rejects a merge proposal.
func ResolveMergeProposal(ctx context.Context, db DBTX, id uuid.UUID, status string, reviewedBy string) error {
	tag, err := db.Exec(ctx, `
		UPDATE vault_merge_proposals SET status = $1, reviewed_by = $2, resolved_at = now()
		WHERE id = $3 AND status = 'pending'
	`, status, reviewedBy, id)
	if err != nil {
		return fmt.Errorf("resolve proposal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("proposal %s not found or already resolved", id)
	}
	return nil
}
