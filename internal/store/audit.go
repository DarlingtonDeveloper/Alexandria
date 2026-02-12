package store

import (
	"context"
	"fmt"
	"time"
)

// AccessAction represents the type of audited action.
type AccessAction string

const (
	ActionKnowledgeRead   AccessAction = "knowledge.read"
	ActionKnowledgeWrite  AccessAction = "knowledge.write"
	ActionKnowledgeSearch AccessAction = "knowledge.search"
	ActionKnowledgeDelete AccessAction = "knowledge.delete"
	ActionSecretRead      AccessAction = "secret.read"
	ActionSecretWrite     AccessAction = "secret.write"
	ActionSecretDelete    AccessAction = "secret.delete"
	ActionSecretRotate    AccessAction = "secret.rotate"
	ActionBriefingGen     AccessAction = "briefing.generate"
	ActionGraphRead       AccessAction = "graph.read"
	ActionGraphWrite      AccessAction = "graph.write"
)

// AccessLogEntry represents an audit log record.
type AccessLogEntry struct {
	ID         string         `json:"id"`
	Action     AccessAction   `json:"action"`
	AgentID    string         `json:"agent_id"`
	ResourceID *string        `json:"resource_id,omitempty"`
	IPAddress  *string        `json:"ip_address,omitempty"`
	Success    bool           `json:"success"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

// AuditStore provides audit logging operations.
type AuditStore struct {
	db *DB
}

// NewAuditStore creates a new AuditStore.
func NewAuditStore(db *DB) *AuditStore {
	return &AuditStore{db: db}
}

// Log writes an audit log entry.
func (s *AuditStore) Log(ctx context.Context, action AccessAction, agentID string, resourceID *string, ipAddress *string, success bool, metadata map[string]any) error {
	_, err := s.db.Pool.Exec(ctx,
		`INSERT INTO vault_access_log (action, agent_id, resource_id, ip_address, success, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		action, agentID, resourceID, ipAddress, success, metadata,
	)
	if err != nil {
		return fmt.Errorf("writing audit log: %w", err)
	}
	return nil
}

// Query retrieves audit log entries with filters.
func (s *AuditStore) Query(ctx context.Context, agentID *string, action *AccessAction, limit int) ([]AccessLogEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	query := `SELECT id, action, agent_id, resource_id, ip_address, success, metadata, created_at
		FROM vault_access_log WHERE 1=1`
	var args []any
	argN := 1

	if agentID != nil {
		query += fmt.Sprintf(" AND agent_id = $%d", argN)
		args = append(args, *agentID)
		argN++
	}
	if action != nil {
		query += fmt.Sprintf(" AND action = $%d", argN)
		args = append(args, *action)
		argN++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", argN)
	args = append(args, limit)

	rows, err := s.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying audit log: %w", err)
	}
	defer rows.Close()

	var entries []AccessLogEntry
	for rows.Next() {
		var e AccessLogEntry
		if err := rows.Scan(&e.ID, &e.Action, &e.AgentID, &e.ResourceID, &e.IPAddress,
			&e.Success, &e.Metadata, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning audit entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
