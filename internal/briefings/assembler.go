// Package briefings provides context rehydration (wake-up briefing) assembly.
package briefings

import (
	"context"
	"fmt"
	"time"

	"github.com/MikeSquared-Agency/Alexandria/internal/store"
)

// Assembler builds wake-up briefings for agents.
type Assembler struct {
	knowledge *store.KnowledgeStore
	secrets   *store.SecretStore
}

// NewAssembler creates a new briefing assembler.
func NewAssembler(knowledge *store.KnowledgeStore, secrets *store.SecretStore) *Assembler {
	return &Assembler{knowledge: knowledge, secrets: secrets}
}

// Briefing is the full context package for an agent wake-up.
type Briefing struct {
	AgentID     string    `json:"agent_id"`
	GeneratedAt time.Time `json:"generated_at"`
	Content     BriefingContent `json:"briefing"`
}

// BriefingContent holds the structured briefing data.
type BriefingContent struct {
	Summary          string            `json:"summary"`
	Sections         []BriefingSection `json:"sections"`
	SecretsAvailable []string          `json:"secrets_available"`
	PendingTasks     []any             `json:"pending_tasks"`
}

// BriefingSection is a named group of briefing items.
type BriefingSection struct {
	Title string         `json:"title"`
	Items []BriefingItem `json:"items"`
}

// BriefingItem is a single piece of briefing context.
type BriefingItem struct {
	Timestamp *time.Time `json:"timestamp,omitempty"`
	Content   string     `json:"content"`
	Source    string     `json:"source"`
	Relevance float64   `json:"relevance"`
}

// Generate assembles a wake-up briefing for the given agent.
func (a *Assembler) Generate(ctx context.Context, agentID string, since time.Time, maxItems int) (*Briefing, error) {
	if maxItems <= 0 || maxItems > 100 {
		maxItems = 50
	}

	// 1. Recent events — knowledge entries created since the agent last slept
	scopePublic := store.ScopePublic
	recentEvents, err := a.knowledge.List(ctx, store.KnowledgeFilter{
		Scope:   &scopePublic,
		AgentID: agentID,
		Limit:   maxItems / 2,
	})
	if err != nil {
		return nil, fmt.Errorf("listing recent events: %w", err)
	}

	// Filter to entries created since 'since'
	var eventItems []BriefingItem
	for _, e := range recentEvents {
		if e.CreatedAt.Before(since) {
			continue
		}
		summary := e.Content
		if e.Summary != nil && *e.Summary != "" {
			summary = *e.Summary
		}
		ts := e.CreatedAt
		source := "vault:knowledge"
		if e.SourceEventID != nil {
			source = "hermes:" + *e.SourceEventID
		}
		eventItems = append(eventItems, BriefingItem{
			Timestamp: &ts,
			Content:   summary,
			Source:    source,
			Relevance: e.Confidence,
		})
	}

	// 2. Agent's own context — preferences and decisions
	catPref := store.CategoryPreference
	agentContext, err := a.knowledge.List(ctx, store.KnowledgeFilter{
		Category:    &catPref,
		SourceAgent: &agentID,
		AgentID:     agentID,
		Limit:       10,
	})
	if err != nil {
		return nil, fmt.Errorf("listing agent context: %w", err)
	}

	var contextItems []BriefingItem
	for _, e := range agentContext {
		summary := e.Content
		if e.Summary != nil && *e.Summary != "" {
			summary = *e.Summary
		}
		contextItems = append(contextItems, BriefingItem{
			Content:   summary,
			Source:    "vault:knowledge",
			Relevance: 1.0,
		})
	}

	// 3. Secrets available to this agent
	allSecrets, err := a.secrets.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing secrets: %w", err)
	}
	var secretNames []string
	for _, s := range allSecrets {
		if a.secrets.CanAccess(&s, agentID) {
			secretNames = append(secretNames, s.Name)
		}
	}

	// Build summary
	summary := fmt.Sprintf("Briefing for %s. %d new events since %s. %d secrets available.",
		agentID, len(eventItems), since.Format(time.RFC3339), len(secretNames))

	// Assemble sections
	var sections []BriefingSection
	if len(eventItems) > 0 {
		sections = append(sections, BriefingSection{
			Title: "Swarm Events",
			Items: eventItems,
		})
	}
	if len(contextItems) > 0 {
		sections = append(sections, BriefingSection{
			Title: "Your Context",
			Items: contextItems,
		})
	}

	return &Briefing{
		AgentID:     agentID,
		GeneratedAt: time.Now(),
		Content: BriefingContent{
			Summary:          summary,
			Sections:         sections,
			SecretsAvailable: secretNames,
			PendingTasks:     []any{},
		},
	}, nil
}
