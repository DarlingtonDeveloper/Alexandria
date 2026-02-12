package hermes

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/warrentherabbit/alexandria/internal/store"
)

// Publisher publishes Alexandria events to Hermes.
type Publisher struct {
	client *Client
	logger *slog.Logger
}

// NewPublisher creates a new Hermes event publisher.
func NewPublisher(client *Client, logger *slog.Logger) *Publisher {
	return &Publisher{client: client, logger: logger}
}

// VaultEvent is the standard event envelope published to Hermes.
type VaultEvent struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Source    string    `json:"source"`
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data"`
}

func (p *Publisher) publish(_ context.Context, subject string, event VaultEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshaling event: %w", err)
	}

	if err := p.client.conn.Publish(subject, data); err != nil {
		return fmt.Errorf("publishing to %s: %w", subject, err)
	}

	p.logger.Debug("published event", "subject", subject, "type", event.Type)
	return nil
}

// KnowledgeCreated publishes a knowledge creation event.
func (p *Publisher) KnowledgeCreated(ctx context.Context, entry *store.KnowledgeEntry) error {
	return p.publish(ctx, "swarm.vault.knowledge.created", VaultEvent{
		ID:        entry.ID,
		Type:      "vault.knowledge.created",
		Source:    "alexandria",
		Timestamp: time.Now(),
		Data: map[string]any{
			"id":           entry.ID,
			"category":     entry.Category,
			"source_agent": entry.SourceAgent,
			"summary":      entry.Summary,
		},
	})
}

// KnowledgeUpdated publishes a knowledge update event.
func (p *Publisher) KnowledgeUpdated(ctx context.Context, entry *store.KnowledgeEntry) error {
	return p.publish(ctx, "swarm.vault.knowledge.updated", VaultEvent{
		ID:        entry.ID,
		Type:      "vault.knowledge.updated",
		Source:    "alexandria",
		Timestamp: time.Now(),
		Data: map[string]any{
			"id":           entry.ID,
			"category":     entry.Category,
			"source_agent": entry.SourceAgent,
		},
	})
}

// KnowledgeSearched publishes a search event (for analytics).
func (p *Publisher) KnowledgeSearched(ctx context.Context, agentID, query string, resultCount int) error {
	return p.publish(ctx, "swarm.vault.knowledge.searched", VaultEvent{
		Type:      "vault.knowledge.searched",
		Source:    "alexandria",
		Timestamp: time.Now(),
		Data: map[string]any{
			"agent_id":     agentID,
			"result_count": resultCount,
		},
	})
}

// SecretAccessed publishes a secret access event.
func (p *Publisher) SecretAccessed(ctx context.Context, agentID, secretName string, success bool) error {
	return p.publish(ctx, "swarm.vault.secret.accessed", VaultEvent{
		Type:      "vault.secret.accessed",
		Source:    "alexandria",
		Timestamp: time.Now(),
		Data: map[string]any{
			"agent_id":    agentID,
			"secret_name": secretName,
			"success":     success,
		},
	})
}

// SecretRotated publishes a secret rotation event.
func (p *Publisher) SecretRotated(ctx context.Context, secretName, rotatedBy string) error {
	return p.publish(ctx, "swarm.vault.secret.rotated", VaultEvent{
		Type:      "vault.secret.rotated",
		Source:    "alexandria",
		Timestamp: time.Now(),
		Data: map[string]any{
			"secret_name": secretName,
			"rotated_by":  rotatedBy,
		},
	})
}

// BriefingGenerated publishes a briefing generation event.
func (p *Publisher) BriefingGenerated(ctx context.Context, agentID string, itemCount int) error {
	return p.publish(ctx, "swarm.vault.briefing.generated", VaultEvent{
		Type:      "vault.briefing.generated",
		Source:    "alexandria",
		Timestamp: time.Now(),
		Data: map[string]any{
			"agent_id":   agentID,
			"item_count": itemCount,
		},
	})
}
