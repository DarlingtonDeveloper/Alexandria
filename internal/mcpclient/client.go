// Package mcpclient provides an HTTP client for the Alexandria API,
// used by the MCP stdio server to forward tool calls.
package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Client is an HTTP client for the Alexandria API.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithAPIKey sets the API key sent via X-API-Key header.
func WithAPIKey(key string) Option {
	return func(c *Client) { c.apiKey = key }
}

// New creates a Client. baseURL should be like "http://localhost:8500".
func New(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// --- Request/Response types ---

// ResolveRequest is the input for identity resolution.
type ResolveRequest struct {
	AliasType   string `json:"alias_type"`
	AliasValue  string `json:"alias_value"`
	Source      string `json:"source"`
	EntityType  string `json:"entity_type"`
	DisplayName string `json:"display_name"`
}

// ResolveResult is the output of identity resolution.
type ResolveResult struct {
	EntityID uuid.UUID `json:"entity_id"`
	AliasID  uuid.UUID `json:"alias_id"`
	Outcome  string    `json:"outcome"`
}

// Entity represents a graph entity.
type Entity struct {
	ID          uuid.UUID       `json:"id"`
	Type        string          `json:"type"`
	Key         string          `json:"key"`
	DisplayName string          `json:"display_name"`
	Summary     string          `json:"summary"`
	Metadata    json.RawMessage `json:"metadata"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	DeletedAt   *time.Time      `json:"deleted_at,omitempty"`
}

// Alias represents an identity alias.
type Alias struct {
	ID          uuid.UUID `json:"id"`
	AliasType   string    `json:"alias_type"`
	AliasValue  string    `json:"alias_value"`
	CanonicalID uuid.UUID `json:"canonical_id"`
	Confidence  float64   `json:"confidence"`
	Source      string    `json:"source"`
	Reviewed    bool      `json:"reviewed"`
	CreatedAt   time.Time `json:"created_at"`
}

// Edge represents a graph edge.
type Edge struct {
	ID         uuid.UUID       `json:"id"`
	FromID     uuid.UUID       `json:"from_id"`
	ToID       uuid.UUID       `json:"to_id"`
	Type       string          `json:"type"`
	Confidence float64         `json:"confidence"`
	Source     string          `json:"source"`
	ValidFrom  time.Time       `json:"valid_from"`
	ValidTo    *time.Time      `json:"valid_to,omitempty"`
	Metadata   json.RawMessage `json:"metadata"`
	CreatedAt  time.Time       `json:"created_at"`
}

// EntityDetail includes the entity plus aliases and edges.
type EntityDetail struct {
	Entity    Entity  `json:"entity"`
	Aliases   []Alias `json:"aliases"`
	EdgesFrom []Edge  `json:"edges_from"`
	EdgesTo   []Edge  `json:"edges_to"`
}

// CreateEdgeRequest is the input for creating an edge.
type CreateEdgeRequest struct {
	SourceEntityID string  `json:"source_entity_id"`
	TargetEntityID string  `json:"target_entity_id"`
	RelationshipType string `json:"relationship_type"`
	Strength       float64 `json:"strength,omitempty"`
	Source         string  `json:"source,omitempty"`
}

// SimilarEntity is returned by similarity queries.
type SimilarEntity struct {
	EntityID   uuid.UUID `json:"entity_id"`
	Distance   float64   `json:"distance"`
	Similarity float64   `json:"similarity"`
}

// SemanticCluster represents a cluster of related entities.
type SemanticCluster struct {
	ID          uuid.UUID  `json:"id"`
	Label       string     `json:"label"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DissolvedAt *time.Time `json:"dissolved_at,omitempty"`
}

// SemanticStatus holds semantic layer metrics.
type SemanticStatus struct {
	EntitiesTotal    int `json:"entities_total"`
	EntitiesEmbedded int `json:"entities_embedded"`
	ClustersActive   int `json:"clusters_active"`
	ProposalsPending int `json:"proposals_pending"`
	EmbeddingGap     int `json:"embedding_gap"`
}

// apiEnvelope wraps Alexandria's standard response format.
type apiEnvelope struct {
	Data  json.RawMessage `json:"data"`
	Error json.RawMessage `json:"error,omitempty"`
}

// --- Methods ---

// Resolve calls the identity resolution endpoint.
func (c *Client) Resolve(ctx context.Context, req ResolveRequest) (*ResolveResult, error) {
	var result ResolveResult
	if err := c.post(ctx, "/api/v1/identity/resolve", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetEntity retrieves an entity with its aliases and edges.
func (c *Client) GetEntity(ctx context.Context, id uuid.UUID) (*EntityDetail, error) {
	var result EntityDetail
	if err := c.get(ctx, fmt.Sprintf("/api/v1/identity/entities/%s", id), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListEntities lists graph entities.
func (c *Client) ListEntities(ctx context.Context, entityType string) ([]json.RawMessage, error) {
	path := "/api/v1/graph/entities"
	if entityType != "" {
		path += "?type=" + entityType
	}
	var result []json.RawMessage
	if err := c.get(ctx, path, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// CreateEdge creates a graph relationship.
func (c *Client) CreateEdge(ctx context.Context, req CreateEdgeRequest) (json.RawMessage, error) {
	var result json.RawMessage
	if err := c.post(ctx, "/api/v1/graph/relationships", req, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// PendingReviews lists aliases pending review.
func (c *Client) PendingReviews(ctx context.Context) ([]Alias, error) {
	var result []Alias
	if err := c.get(ctx, "/api/v1/identity/pending", &result); err != nil {
		return nil, err
	}
	return result, nil
}

// SimilarEntities finds entities similar to a given entity.
func (c *Client) SimilarEntities(ctx context.Context, entityID uuid.UUID, limit int, minSimilarity float64) ([]SimilarEntity, error) {
	path := fmt.Sprintf("/api/v1/semantic/similar/%s?limit=%d&min_similarity=%f", entityID, limit, minSimilarity)
	var result []SimilarEntity
	if err := c.get(ctx, path, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Clusters lists active semantic clusters.
func (c *Client) Clusters(ctx context.Context) ([]SemanticCluster, error) {
	var result []SemanticCluster
	if err := c.get(ctx, "/api/v1/semantic/clusters", &result); err != nil {
		return nil, err
	}
	return result, nil
}

// SemanticStatusFn returns semantic layer metrics.
func (c *Client) SemanticStatusFn(ctx context.Context) (*SemanticStatus, error) {
	var result SemanticStatus
	if err := c.get(ctx, "/api/v1/semantic/status", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// --- HTTP helpers ---

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s %s: %d %s", req.Method, req.URL.Path, resp.StatusCode, string(body))
	}

	if out != nil {
		// Alexandria wraps responses in {"data": ...}
		var envelope apiEnvelope
		if err := json.Unmarshal(body, &envelope); err == nil && envelope.Data != nil {
			return json.Unmarshal(envelope.Data, out)
		}
		// Fallback: try direct unmarshal
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
