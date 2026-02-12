package store

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	pgvector "github.com/pgvector/pgvector-go"
)

// KnowledgeCategory represents the type of knowledge.
type KnowledgeCategory string

const (
	CategoryDiscovery    KnowledgeCategory = "discovery"
	CategoryLesson       KnowledgeCategory = "lesson"
	CategoryPreference   KnowledgeCategory = "preference"
	CategoryFact         KnowledgeCategory = "fact"
	CategoryEvent        KnowledgeCategory = "event"
	CategoryDecision     KnowledgeCategory = "decision"
	CategoryRelationship KnowledgeCategory = "relationship"
)

// KnowledgeScope represents visibility of a knowledge entry.
type KnowledgeScope string

const (
	ScopePublic  KnowledgeScope = "public"
	ScopePrivate KnowledgeScope = "private"
	ScopeShared  KnowledgeScope = "shared"
)

// RelevanceDecay represents how quickly knowledge loses relevance.
type RelevanceDecay string

const (
	DecayNone      RelevanceDecay = "none"
	DecaySlow      RelevanceDecay = "slow"
	DecayFast      RelevanceDecay = "fast"
	DecayEphemeral RelevanceDecay = "ephemeral"
)

// KnowledgeEntry represents a stored knowledge record.
type KnowledgeEntry struct {
	ID             string            `json:"id"`
	Content        string            `json:"content"`
	Summary        *string           `json:"summary,omitempty"`
	SourceAgent    string            `json:"source_agent"`
	Category       KnowledgeCategory `json:"category"`
	Scope          KnowledgeScope    `json:"scope"`
	SharedWith     []string          `json:"shared_with,omitempty"`
	Tags           []string          `json:"tags,omitempty"`
	Embedding      pgvector.Vector   `json:"-"`
	Metadata       map[string]any    `json:"metadata,omitempty"`
	SourceEventID  *string           `json:"source_event_id,omitempty"`
	Confidence     float64           `json:"confidence"`
	RelevanceDecay RelevanceDecay    `json:"relevance_decay"`
	ExpiresAt      *time.Time        `json:"expires_at,omitempty"`
	SupersededBy   *string           `json:"superseded_by,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// KnowledgeCreateInput is the input for creating a knowledge entry.
type KnowledgeCreateInput struct {
	Content        string            `json:"content"`
	Summary        *string           `json:"summary,omitempty"`
	SourceAgent    string            `json:"source_agent"`
	Category       KnowledgeCategory `json:"category"`
	Scope          KnowledgeScope    `json:"scope"`
	SharedWith     []string          `json:"shared_with,omitempty"`
	Tags           []string          `json:"tags,omitempty"`
	Embedding      pgvector.Vector   `json:"-"`
	Metadata       map[string]any    `json:"metadata,omitempty"`
	SourceEventID  *string           `json:"source_event_id,omitempty"`
	Confidence     float64           `json:"confidence"`
	RelevanceDecay RelevanceDecay    `json:"relevance_decay"`
	ExpiresAt      *time.Time        `json:"expires_at,omitempty"`
}

// KnowledgeUpdateInput is the input for updating a knowledge entry.
type KnowledgeUpdateInput struct {
	Content        *string            `json:"content,omitempty"`
	Summary        *string            `json:"summary,omitempty"`
	Category       *KnowledgeCategory `json:"category,omitempty"`
	Scope          *KnowledgeScope    `json:"scope,omitempty"`
	SharedWith     []string           `json:"shared_with,omitempty"`
	Tags           []string           `json:"tags,omitempty"`
	Embedding      *pgvector.Vector   `json:"-"`
	Metadata       map[string]any     `json:"metadata,omitempty"`
	Confidence     *float64           `json:"confidence,omitempty"`
	RelevanceDecay *RelevanceDecay    `json:"relevance_decay,omitempty"`
	ExpiresAt      *time.Time         `json:"expires_at,omitempty"`
	SupersededBy   *string            `json:"superseded_by,omitempty"`
}

// KnowledgeFilter specifies filter criteria for listing knowledge.
type KnowledgeFilter struct {
	Category    *KnowledgeCategory
	Scope       *KnowledgeScope
	SourceAgent *string
	Tags        []string
	AgentID     string // requesting agent for access control
	Limit       int
	Offset      int
}

// SearchInput represents a semantic search request.
type SearchInput struct {
	QueryEmbedding pgvector.Vector
	Limit          int
	Scope          *KnowledgeScope
	Categories     []KnowledgeCategory
	AgentID        string
	MinRelevance   float64
	IncludeExpired bool
}

// SearchResult is a knowledge entry with relevance score.
type SearchResult struct {
	KnowledgeEntry
	Similarity float64 `json:"relevance"`
}

// KnowledgeStore provides knowledge CRUD operations.
type KnowledgeStore struct {
	db *DB
}

// NewKnowledgeStore creates a new KnowledgeStore.
func NewKnowledgeStore(db *DB) *KnowledgeStore {
	return &KnowledgeStore{db: db}
}

// Create inserts a new knowledge entry.
func (s *KnowledgeStore) Create(ctx context.Context, input KnowledgeCreateInput) (*KnowledgeEntry, error) {
	query := `
		INSERT INTO vault_knowledge (content, summary, source_agent, category, scope, shared_with, tags, embedding, metadata, source_event_id, confidence, relevance_decay, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, content, summary, source_agent, category, scope, shared_with, tags, metadata, source_event_id, confidence, relevance_decay, expires_at, superseded_by, created_at, updated_at`

	entry := &KnowledgeEntry{}
	err := s.db.Pool.QueryRow(ctx, query,
		input.Content, input.Summary, input.SourceAgent, input.Category, input.Scope,
		input.SharedWith, input.Tags, input.Embedding, input.Metadata, input.SourceEventID,
		input.Confidence, input.RelevanceDecay, input.ExpiresAt,
	).Scan(
		&entry.ID, &entry.Content, &entry.Summary, &entry.SourceAgent, &entry.Category,
		&entry.Scope, &entry.SharedWith, &entry.Tags, &entry.Metadata, &entry.SourceEventID,
		&entry.Confidence, &entry.RelevanceDecay, &entry.ExpiresAt, &entry.SupersededBy,
		&entry.CreatedAt, &entry.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating knowledge entry: %w", err)
	}
	return entry, nil
}

// GetByID retrieves a knowledge entry by ID with access control.
func (s *KnowledgeStore) GetByID(ctx context.Context, id, agentID string) (*KnowledgeEntry, error) {
	query := `
		SELECT id, content, summary, source_agent, category, scope, shared_with, tags, metadata,
		       source_event_id, confidence, relevance_decay, expires_at, superseded_by, created_at, updated_at
		FROM vault_knowledge
		WHERE id = $1 AND deleted_at IS NULL`

	entry := &KnowledgeEntry{}
	err := s.db.Pool.QueryRow(ctx, query, id).Scan(
		&entry.ID, &entry.Content, &entry.Summary, &entry.SourceAgent, &entry.Category,
		&entry.Scope, &entry.SharedWith, &entry.Tags, &entry.Metadata, &entry.SourceEventID,
		&entry.Confidence, &entry.RelevanceDecay, &entry.ExpiresAt, &entry.SupersededBy,
		&entry.CreatedAt, &entry.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting knowledge entry: %w", err)
	}

	// Access control
	if !canAccess(entry, agentID) {
		return nil, nil
	}

	return entry, nil
}

// List retrieves knowledge entries with filters and access control.
func (s *KnowledgeStore) List(ctx context.Context, filter KnowledgeFilter) ([]KnowledgeEntry, error) {
	var conditions []string
	var args []any
	argN := 1

	conditions = append(conditions, "deleted_at IS NULL")

	if filter.Category != nil {
		conditions = append(conditions, fmt.Sprintf("category = $%d", argN))
		args = append(args, *filter.Category)
		argN++
	}
	if filter.Scope != nil {
		conditions = append(conditions, fmt.Sprintf("scope = $%d", argN))
		args = append(args, *filter.Scope)
		argN++
	}
	if filter.SourceAgent != nil {
		conditions = append(conditions, fmt.Sprintf("source_agent = $%d", argN))
		args = append(args, *filter.SourceAgent)
		argN++
	}
	if len(filter.Tags) > 0 {
		conditions = append(conditions, fmt.Sprintf("tags && $%d", argN))
		args = append(args, filter.Tags)
		argN++
	}

	// Access control: show public, own entries, or shared with agent
	conditions = append(conditions, fmt.Sprintf(
		"(scope = 'public' OR source_agent = $%d OR (scope = 'shared' AND $%d = ANY(shared_with)))",
		argN, argN+1,
	))
	args = append(args, filter.AgentID, filter.AgentID)
	argN += 2

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`
		SELECT id, content, summary, source_agent, category, scope, shared_with, tags, metadata,
		       source_event_id, confidence, relevance_decay, expires_at, superseded_by, created_at, updated_at
		FROM vault_knowledge
		WHERE %s
		ORDER BY created_at DESC
		LIMIT %d OFFSET %d`,
		strings.Join(conditions, " AND "), limit, offset)

	rows, err := s.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing knowledge: %w", err)
	}
	defer rows.Close()

	var entries []KnowledgeEntry
	for rows.Next() {
		var e KnowledgeEntry
		if err := rows.Scan(
			&e.ID, &e.Content, &e.Summary, &e.SourceAgent, &e.Category,
			&e.Scope, &e.SharedWith, &e.Tags, &e.Metadata, &e.SourceEventID,
			&e.Confidence, &e.RelevanceDecay, &e.ExpiresAt, &e.SupersededBy,
			&e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning knowledge entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Update modifies a knowledge entry. Only the owning agent or admin can update.
func (s *KnowledgeStore) Update(ctx context.Context, id, agentID string, input KnowledgeUpdateInput) (*KnowledgeEntry, error) {
	// Verify ownership
	var owner string
	err := s.db.Pool.QueryRow(ctx, "SELECT source_agent FROM vault_knowledge WHERE id = $1 AND deleted_at IS NULL", id).Scan(&owner)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("checking ownership: %w", err)
	}
	if owner != agentID && agentID != "warren" {
		return nil, fmt.Errorf("access denied: only owner or admin can update")
	}

	var setClauses []string
	var args []any
	argN := 1

	if input.Content != nil {
		setClauses = append(setClauses, fmt.Sprintf("content = $%d", argN))
		args = append(args, *input.Content)
		argN++
	}
	if input.Summary != nil {
		setClauses = append(setClauses, fmt.Sprintf("summary = $%d", argN))
		args = append(args, *input.Summary)
		argN++
	}
	if input.Category != nil {
		setClauses = append(setClauses, fmt.Sprintf("category = $%d", argN))
		args = append(args, *input.Category)
		argN++
	}
	if input.Scope != nil {
		setClauses = append(setClauses, fmt.Sprintf("scope = $%d", argN))
		args = append(args, *input.Scope)
		argN++
	}
	if input.SharedWith != nil {
		setClauses = append(setClauses, fmt.Sprintf("shared_with = $%d", argN))
		args = append(args, input.SharedWith)
		argN++
	}
	if input.Tags != nil {
		setClauses = append(setClauses, fmt.Sprintf("tags = $%d", argN))
		args = append(args, input.Tags)
		argN++
	}
	if input.Embedding != nil {
		setClauses = append(setClauses, fmt.Sprintf("embedding = $%d", argN))
		args = append(args, *input.Embedding)
		argN++
	}
	if input.Metadata != nil {
		setClauses = append(setClauses, fmt.Sprintf("metadata = $%d", argN))
		args = append(args, input.Metadata)
		argN++
	}
	if input.Confidence != nil {
		setClauses = append(setClauses, fmt.Sprintf("confidence = $%d", argN))
		args = append(args, *input.Confidence)
		argN++
	}
	if input.RelevanceDecay != nil {
		setClauses = append(setClauses, fmt.Sprintf("relevance_decay = $%d", argN))
		args = append(args, *input.RelevanceDecay)
		argN++
	}
	if input.ExpiresAt != nil {
		setClauses = append(setClauses, fmt.Sprintf("expires_at = $%d", argN))
		args = append(args, *input.ExpiresAt)
		argN++
	}
	if input.SupersededBy != nil {
		setClauses = append(setClauses, fmt.Sprintf("superseded_by = $%d", argN))
		args = append(args, *input.SupersededBy)
		argN++
	}

	if len(setClauses) == 0 {
		return s.GetByID(ctx, id, agentID)
	}

	query := fmt.Sprintf(`
		UPDATE vault_knowledge SET %s
		WHERE id = $%d AND deleted_at IS NULL
		RETURNING id, content, summary, source_agent, category, scope, shared_with, tags, metadata,
		          source_event_id, confidence, relevance_decay, expires_at, superseded_by, created_at, updated_at`,
		strings.Join(setClauses, ", "), argN)
	args = append(args, id)

	entry := &KnowledgeEntry{}
	err = s.db.Pool.QueryRow(ctx, query, args...).Scan(
		&entry.ID, &entry.Content, &entry.Summary, &entry.SourceAgent, &entry.Category,
		&entry.Scope, &entry.SharedWith, &entry.Tags, &entry.Metadata, &entry.SourceEventID,
		&entry.Confidence, &entry.RelevanceDecay, &entry.ExpiresAt, &entry.SupersededBy,
		&entry.CreatedAt, &entry.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("updating knowledge entry: %w", err)
	}
	return entry, nil
}

// Delete soft-deletes a knowledge entry.
func (s *KnowledgeStore) Delete(ctx context.Context, id, agentID string) error {
	var owner string
	err := s.db.Pool.QueryRow(ctx, "SELECT source_agent FROM vault_knowledge WHERE id = $1 AND deleted_at IS NULL", id).Scan(&owner)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("not found")
		}
		return fmt.Errorf("checking ownership: %w", err)
	}
	if owner != agentID && agentID != "warren" {
		return fmt.Errorf("access denied")
	}

	_, err = s.db.Pool.Exec(ctx, "UPDATE vault_knowledge SET deleted_at = NOW() WHERE id = $1", id)
	return err
}

// Search performs semantic search using pgvector cosine similarity.
func (s *KnowledgeStore) Search(ctx context.Context, input SearchInput) ([]SearchResult, error) {
	var conditions []string
	var args []any
	argN := 1

	// Query embedding
	conditions = append(conditions, "deleted_at IS NULL")
	conditions = append(conditions, "(superseded_by IS NULL)")

	if !input.IncludeExpired {
		conditions = append(conditions, "(expires_at IS NULL OR expires_at > NOW())")
	}

	if input.Scope != nil {
		conditions = append(conditions, fmt.Sprintf("scope = $%d", argN))
		args = append(args, *input.Scope)
		argN++
	}

	if len(input.Categories) > 0 {
		conditions = append(conditions, fmt.Sprintf("category = ANY($%d)", argN))
		args = append(args, input.Categories)
		argN++
	}

	// Access control
	conditions = append(conditions, fmt.Sprintf(
		"(scope = 'public' OR source_agent = $%d OR (scope = 'shared' AND $%d = ANY(shared_with)))",
		argN, argN+1,
	))
	args = append(args, input.AgentID, input.AgentID)
	argN += 2

	// Embedding parameter
	embeddingArgN := argN
	args = append(args, input.QueryEmbedding)
	argN++

	limit := input.Limit
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	minSim := input.MinRelevance
	if minSim <= 0 {
		minSim = 0.5
	}

	query := fmt.Sprintf(`
		SELECT id, content, summary, source_agent, category, scope, shared_with, tags, metadata,
		       source_event_id, confidence, relevance_decay, expires_at, superseded_by, created_at, updated_at,
		       (1 - (embedding <=> $%d))::FLOAT AS similarity
		FROM vault_knowledge
		WHERE %s
		  AND embedding IS NOT NULL
		  AND (1 - (embedding <=> $%d)) >= %f
		ORDER BY embedding <=> $%d
		LIMIT %d`,
		embeddingArgN, strings.Join(conditions, " AND "), embeddingArgN, minSim, embeddingArgN, limit)

	rows, err := s.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("searching knowledge: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(
			&r.ID, &r.Content, &r.Summary, &r.SourceAgent, &r.Category,
			&r.Scope, &r.SharedWith, &r.Tags, &r.Metadata, &r.SourceEventID,
			&r.Confidence, &r.RelevanceDecay, &r.ExpiresAt, &r.SupersededBy,
			&r.CreatedAt, &r.UpdatedAt, &r.Similarity,
		); err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}
		// Apply relevance decay
		r.Similarity = applyDecay(r.Similarity, r.RelevanceDecay, r.CreatedAt)
		results = append(results, r)
	}
	return results, rows.Err()
}

// Count returns the total number of non-deleted knowledge entries.
func (s *KnowledgeStore) Count(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM vault_knowledge WHERE deleted_at IS NULL").Scan(&count)
	return count, err
}

// canAccess checks if an agent can access a knowledge entry.
func canAccess(entry *KnowledgeEntry, agentID string) bool {
	if entry.SourceAgent == agentID || agentID == "warren" {
		return true
	}
	switch entry.Scope {
	case ScopePublic:
		return true
	case ScopePrivate:
		return false
	case ScopeShared:
		for _, a := range entry.SharedWith {
			if a == agentID || a == "*" {
				return true
			}
		}
		return false
	}
	return false
}

// applyDecay applies relevance decay based on entry age.
func applyDecay(similarity float64, decay RelevanceDecay, createdAt time.Time) float64 {
	var halfLifeDays float64
	switch decay {
	case DecayNone:
		return similarity
	case DecaySlow:
		halfLifeDays = 30
	case DecayFast:
		halfLifeDays = 7
	case DecayEphemeral:
		halfLifeDays = 1
	default:
		return similarity
	}

	ageDays := time.Since(createdAt).Hours() / 24
	multiplier := math.Pow(0.5, ageDays/halfLifeDays)
	return similarity * multiplier
}
