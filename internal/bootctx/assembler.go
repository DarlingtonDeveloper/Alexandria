// Package bootctx assembles agent-specific boot context as markdown.
package bootctx

import (
	"context"
	"fmt"
	"strings"

	"github.com/MikeSquared-Agency/Alexandria/internal/store"
)

// AccessLevel controls what an agent can see.
type AccessLevel int

const (
	// ScopedAccess means the agent sees only its owner's info plus shared resources.
	ScopedAccess AccessLevel = iota
	// FullAccess means the agent sees everything (e.g. kai).
	FullAccess
)

// AgentProfile defines per-agent scope rules for context generation.
type AgentProfile struct {
	Access    AccessLevel
	ExtraTags []string // additional knowledge tags to include
}

// agentProfiles maps known agent names to their scope rules.
var agentProfiles = map[string]AgentProfile{
	"kai":         {Access: FullAccess},
	"lily":        {Access: ScopedAccess},
	"scout":       {Access: ScopedAccess},
	"dutybound":   {ExtraTags: []string{"ci", "workflow", "repo"}},
	"celebrimbor": {ExtraTags: []string{"souls", "promptforge", "design"}},
}

// Assembler builds boot-context markdown for agents.
type Assembler struct {
	knowledge *store.KnowledgeStore
	secrets   *store.SecretStore
	graph     *store.GraphStore
	grants    *store.GrantStore
}

// NewAssembler creates a new boot-context assembler.
func NewAssembler(
	knowledge *store.KnowledgeStore,
	secrets *store.SecretStore,
	graph *store.GraphStore,
	grants *store.GrantStore,
) *Assembler {
	return &Assembler{
		knowledge: knowledge,
		secrets:   secrets,
		graph:     graph,
		grants:    grants,
	}
}

// Generate produces the full boot-context markdown for agentID.
func (a *Assembler) Generate(ctx context.Context, agentID string) (string, error) {
	profile := agentProfiles[agentID] // zero-value is fine for unknown agents

	// Derive owner from graph: agent entity → "owns" relationship → person entity
	var ownerEntity *store.Entity
	agentEntity, err := a.graph.GetEntityByName(ctx, agentID, store.EntityAgent)
	if err != nil {
		return "", fmt.Errorf("looking up agent entity: %w", err)
	}
	if agentEntity != nil {
		rels, err := a.graph.GetRelationships(ctx, agentEntity.ID)
		if err != nil {
			return "", fmt.Errorf("looking up agent relationships: %w", err)
		}
		for _, r := range rels {
			if r.RelationshipType == "owns" && r.TargetEntityID == agentEntity.ID {
				ownerEntity, err = a.graph.GetEntity(ctx, r.SourceEntityID)
				if err != nil {
					return "", fmt.Errorf("looking up owner entity: %w", err)
				}
				break
			}
		}
	}

	var b strings.Builder
	b.WriteString("# Boot Context\n\n")
	b.WriteString(fmt.Sprintf("Agent: **%s**\n\n", agentID))

	if err := a.writeOwnerSection(ctx, &b, ownerEntity); err != nil {
		return "", fmt.Errorf("owner section: %w", err)
	}
	if err := a.writePeopleSection(ctx, &b, profile, ownerEntity); err != nil {
		return "", fmt.Errorf("people section: %w", err)
	}
	if err := a.writeAgentsSection(ctx, &b); err != nil {
		return "", fmt.Errorf("agents section: %w", err)
	}
	if err := a.writeAccessSection(ctx, &b, agentID); err != nil {
		return "", fmt.Errorf("access section: %w", err)
	}
	if err := a.writeRulesSection(ctx, &b, agentID, profile); err != nil {
		return "", fmt.Errorf("rules section: %w", err)
	}
	if err := a.writeInfraSection(ctx, &b); err != nil {
		return "", fmt.Errorf("infra section: %w", err)
	}

	return b.String(), nil
}

// writeOwnerSection writes the "Your Owner" section from the graph entity.
func (a *Assembler) writeOwnerSection(ctx context.Context, b *strings.Builder, owner *store.Entity) error {
	if owner == nil {
		return nil
	}

	b.WriteString("## Your Owner\n\n")
	b.WriteString(fmt.Sprintf("- **Name**: %s\n", owner.DisplayName))
	b.WriteString(fmt.Sprintf("- **Identifier**: %s\n", metaString(owner.Metadata, "identifier", owner.Name)))

	if v := metaString(owner.Metadata, "phone", ""); v != "" {
		b.WriteString(fmt.Sprintf("- **Phone**: %s\n", v))
	}
	if v := metaString(owner.Metadata, "timezone", ""); v != "" {
		b.WriteString(fmt.Sprintf("- **Timezone**: %s\n", v))
	}
	if v := metaString(owner.Metadata, "preferences", ""); v != "" {
		b.WriteString(fmt.Sprintf("- **Preferences**: %s\n", v))
	}
	b.WriteString("\n")
	return nil
}

// writePeopleSection writes a markdown table of all known people from vault_entities.
func (a *Assembler) writePeopleSection(ctx context.Context, b *strings.Builder, profile AgentProfile, owner *store.Entity) error {
	personType := store.EntityPerson
	entities, err := a.graph.ListEntities(ctx, &personType, 50, 0)
	if err != nil {
		return err
	}
	if len(entities) == 0 {
		return nil
	}

	b.WriteString("## People\n\n")
	b.WriteString("| Name | Identifier | Timezone |\n")
	b.WriteString("|------|------------|----------|\n")

	for _, e := range entities {
		identifier := metaString(e.Metadata, "identifier", e.Name)
		tz := metaString(e.Metadata, "timezone", "—")
		// Scoped agents only see their owner
		if profile.Access != FullAccess && owner != nil && e.ID != owner.ID {
			continue
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", e.DisplayName, identifier, tz))
	}

	b.WriteString("\n")
	return nil
}

// writeAgentsSection writes a markdown table of all known agents from the graph.
// When entity summary is empty, falls back to knowledge entries tagged ['agent', '<name>', 'config'].
func (a *Assembler) writeAgentsSection(ctx context.Context, b *strings.Builder) error {
	agentType := store.EntityAgent
	entities, err := a.graph.ListEntities(ctx, &agentType, 50, 0)
	if err != nil {
		return err
	}
	if len(entities) == 0 {
		return nil
	}

	// Pre-load agent config knowledge entries for summary fallback
	catFact := store.CategoryFact
	agentKnowledge, err := a.knowledge.List(ctx, store.KnowledgeFilter{
		Category: &catFact,
		Tags:     []string{"agent", "config"},
		AgentID:  "kai", // use kai for full access to public entries
		Limit:    50,
	})
	if err != nil {
		return err
	}

	// Index by agent name (lowercase) for quick lookup
	knowledgeByAgent := make(map[string]string)
	for _, ke := range agentKnowledge {
		for _, tag := range ke.Tags {
			// Tags are like ['agent', 'kai', 'config'] — the middle tag is the agent name
			if tag != "agent" && tag != "config" && tag != "capabilities" {
				knowledgeByAgent[tag] = ke.Content
			}
		}
	}

	b.WriteString("## Agents\n\n")
	b.WriteString("| Name | Summary |\n")
	b.WriteString("|------|---------|\n")

	for _, e := range entities {
		summary := e.Summary
		if summary == "" {
			// Fallback: look up from knowledge entries
			agentKey := strings.ToLower(e.Name)
			if desc, ok := knowledgeByAgent[agentKey]; ok {
				summary = desc
			} else {
				summary = "—"
			}
		}
		b.WriteString(fmt.Sprintf("| %s | %s |\n", e.DisplayName, summary))
	}

	b.WriteString("\n")
	return nil
}

// writeAccessSection writes the secrets and channels the agent can access.
func (a *Assembler) writeAccessSection(ctx context.Context, b *strings.Builder, agentID string) error {
	// Secrets
	allSecrets, err := a.secrets.List(ctx)
	if err != nil {
		return err
	}
	var secretNames []string
	for _, s := range allSecrets {
		if a.secrets.CanAccess(&s, agentID) {
			secretNames = append(secretNames, s.Name)
		}
	}

	// Channels from knowledge tagged "channel"
	catFact := store.CategoryFact
	channelEntries, err := a.knowledge.List(ctx, store.KnowledgeFilter{
		Category: &catFact,
		Tags:     []string{"channel"},
		AgentID:  agentID,
		Limit:    50,
	})
	if err != nil {
		return err
	}

	if len(secretNames) == 0 && len(channelEntries) == 0 {
		return nil
	}

	b.WriteString("## Access\n\n")

	if len(secretNames) > 0 {
		b.WriteString("### Secrets Available\n\n")
		for _, name := range secretNames {
			b.WriteString(fmt.Sprintf("- `%s`\n", name))
		}
		b.WriteString("\n")
	}

	if len(channelEntries) > 0 {
		b.WriteString("### Channels\n\n")
		for _, e := range channelEntries {
			summary := e.Content
			if e.Summary != nil && *e.Summary != "" {
				summary = *e.Summary
			}
			b.WriteString(fmt.Sprintf("- %s\n", summary))
		}
		b.WriteString("\n")
	}

	return nil
}

// writeRulesSection writes operational rules from knowledge entries.
func (a *Assembler) writeRulesSection(ctx context.Context, b *strings.Builder, agentID string, profile AgentProfile) error {
	catDecision := store.CategoryDecision
	ruleEntries, err := a.knowledge.List(ctx, store.KnowledgeFilter{
		Category: &catDecision,
		Tags:     []string{"rules"},
		AgentID:  agentID,
		Limit:    50,
	})
	if err != nil {
		return err
	}

	// Extra-tag rules for agents like dutybound/celebrimbor
	var extraEntries []store.KnowledgeEntry
	for _, tag := range profile.ExtraTags {
		entries, err := a.knowledge.List(ctx, store.KnowledgeFilter{
			Tags:    []string{tag},
			AgentID: agentID,
			Limit:   20,
		})
		if err != nil {
			return err
		}
		extraEntries = append(extraEntries, entries...)
	}

	if len(ruleEntries) == 0 && len(extraEntries) == 0 {
		return nil
	}

	b.WriteString("## Rules\n\n")

	// Deduplicate by ID
	seen := make(map[string]bool)
	for _, e := range ruleEntries {
		if seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		content := e.Content
		if e.Summary != nil && *e.Summary != "" {
			content = *e.Summary
		}
		b.WriteString(fmt.Sprintf("- %s\n", content))
	}
	for _, e := range extraEntries {
		if seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		content := e.Content
		if e.Summary != nil && *e.Summary != "" {
			content = *e.Summary
		}
		b.WriteString(fmt.Sprintf("- %s\n", content))
	}

	b.WriteString("\n")
	return nil
}

// writeInfraSection writes known services from the graph.
func (a *Assembler) writeInfraSection(ctx context.Context, b *strings.Builder) error {
	serviceType := store.EntityService
	entities, err := a.graph.ListEntities(ctx, &serviceType, 50, 0)
	if err != nil {
		return err
	}
	if len(entities) == 0 {
		return nil
	}

	b.WriteString("## Infrastructure\n\n")

	for _, e := range entities {
		endpoint := metaString(e.Metadata, "endpoint", "")
		if endpoint != "" {
			b.WriteString(fmt.Sprintf("- **%s** — %s\n", e.DisplayName, endpoint))
		} else {
			b.WriteString(fmt.Sprintf("- **%s**", e.DisplayName))
			if e.Summary != "" {
				b.WriteString(fmt.Sprintf(" — %s", e.Summary))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	return nil
}

// metaString extracts a string value from a metadata map, returning dflt if missing.
func metaString(m map[string]any, key, dflt string) string {
	if m == nil {
		return dflt
	}
	v, ok := m[key]
	if !ok {
		return dflt
	}
	s, ok := v.(string)
	if !ok {
		return dflt
	}
	return s
}
