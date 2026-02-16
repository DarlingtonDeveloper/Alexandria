package hermes

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/MikeSquared-Agency/Alexandria/internal/embeddings"
	"github.com/MikeSquared-Agency/Alexandria/internal/store"
)

// Subscriber listens to Hermes events and auto-captures knowledge.
type Subscriber struct {
	client    *Client
	knowledge *store.KnowledgeStore
	embedder  embeddings.Provider
	publisher *Publisher
	logger    *slog.Logger
	subs      []*nats.Subscription
}

// NewSubscriber creates a new Hermes event subscriber.
func NewSubscriber(client *Client, knowledge *store.KnowledgeStore, embedder embeddings.Provider, publisher *Publisher, logger *slog.Logger) *Subscriber {
	return &Subscriber{
		client:    client,
		knowledge: knowledge,
		embedder:  embedder,
		publisher: publisher,
		logger:    logger,
	}
}

// CorrectionSignal represents a Dredd correction event payload.
type CorrectionSignal struct {
	SessionRef     string `json:"session_ref"`
	DecisionID     string `json:"decision_id"`
	AgentID        string `json:"agent_id"`
	ModelID        string `json:"model_id"`
	ModelTier      string `json:"model_tier"`
	CorrectionType string `json:"correction_type"`
	Category       string `json:"category"`
	Severity       string `json:"severity"`
}

// CorrectionEnvelope is the Hermes envelope wrapping a correction signal.
type CorrectionEnvelope struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Source    string          `json:"source"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// HermesEvent represents an incoming event from Hermes.
type HermesEvent struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Source string `json:"source"`
	Data   struct {
		Content string   `json:"content"`
		Tags    []string `json:"tags"`
		Summary string   `json:"summary,omitempty"`
	} `json:"data"`
}

// Start begins subscribing to Hermes event subjects.
func (s *Subscriber) Start(ctx context.Context) error {
	subjects := map[string]func(msg *nats.Msg){
		"swarm.discovery.>":       s.handleDiscovery,
		"swarm.task.*.completed":  s.handleTaskCompleted,
		"swarm.task.*.failed":     s.handleTaskFailed,
		"swarm.agent.*.started":   s.handleAgentStarted,
		"swarm.agent.*.stopped":   s.handleAgentStopped,
		"swarm.dredd.correction":  s.handleCorrection,
	}

	for subject, handler := range subjects {
		// Try JetStream durable consumer first, fall back to core NATS
		sub, err := s.client.js.Subscribe(subject, handler,
			nats.Durable("alexandria-"+sanitizeSubject(subject)),
			nats.DeliverAll(),
			nats.AckExplicit(),
			nats.MaxDeliver(3),
		)
		if err != nil {
			// JetStream might not have the stream; use core NATS
			s.logger.Warn("JetStream subscribe failed, using core NATS", "subject", subject, "error", err)
			sub, err = s.client.conn.Subscribe(subject, handler)
			if err != nil {
				return fmt.Errorf("subscribing to %s: %w", subject, err)
			}
		}
		s.subs = append(s.subs, sub)
		s.logger.Info("subscribed to Hermes subject", "subject", subject)
	}

	return nil
}

// Stop unsubscribes from all subjects.
func (s *Subscriber) Stop() {
	for _, sub := range s.subs {
		_ = sub.Unsubscribe()
	}
}

func (s *Subscriber) handleDiscovery(msg *nats.Msg) {
	s.captureEvent(msg, store.CategoryDiscovery, store.DecaySlow, 0.8)
}

func (s *Subscriber) handleTaskCompleted(msg *nats.Msg) {
	s.captureEvent(msg, store.CategoryEvent, store.DecayFast, 0.9)
}

func (s *Subscriber) handleTaskFailed(msg *nats.Msg) {
	s.captureEvent(msg, store.CategoryLesson, store.DecaySlow, 0.7)
}

func (s *Subscriber) handleAgentStarted(msg *nats.Msg) {
	// Just log, don't persist as knowledge
	s.logger.Info("agent started event", "subject", msg.Subject)
	s.ack(msg)
}

func (s *Subscriber) handleAgentStopped(msg *nats.Msg) {
	s.logger.Info("agent stopped event", "subject", msg.Subject)
	s.ack(msg)
}

func (s *Subscriber) handleCorrection(msg *nats.Msg) {
	// Parse the Hermes envelope
	var envelope CorrectionEnvelope
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		s.logger.Error("failed to parse correction envelope", "error", err, "subject", msg.Subject)
		s.ack(msg)
		return
	}

	// Parse the correction signal from the Data field
	var signal CorrectionSignal
	if err := json.Unmarshal(envelope.Data, &signal); err != nil {
		s.logger.Error("failed to parse correction signal data", "error", err, "event_id", envelope.ID)
		s.ack(msg)
		return
	}

	// Only store rejected decisions as lessons — confirmed decisions need no correction
	if signal.CorrectionType != "rejected" {
		s.logger.Debug("skipping non-rejected correction", "type", signal.CorrectionType, "decision_id", signal.DecisionID)
		s.ack(msg)
		return
	}

	ctx := context.Background()

	content := fmt.Sprintf("Dredd rejected decision %s by agent %s (model: %s, tier: %s). Category: %s, severity: %s. Session: %s",
		signal.DecisionID, signal.AgentID, signal.ModelID, signal.ModelTier, signal.Category, signal.Severity, signal.SessionRef)

	summary := fmt.Sprintf("Rejected %s decision (%s) for %s/%s",
		signal.Category, signal.Severity, signal.AgentID, signal.ModelTier)

	// Build tags for later retrieval: agent role, model tier, correction category
	tags := []string{
		"correction",
		"agent:" + signal.AgentID,
		"model_tier:" + signal.ModelTier,
		"category:" + signal.Category,
		"severity:" + signal.Severity,
	}

	// Generate embedding for the lesson content
	embedding, err := s.embedder.Embed(ctx, content)
	if err != nil {
		s.logger.Error("failed to generate embedding for correction", "error", err)
		// Continue without embedding
	}

	eventID := envelope.ID
	input := store.KnowledgeCreateInput{
		Content:        content,
		Summary:        &summary,
		SourceAgent:    "dredd",
		Category:       store.CategoryLesson,
		Scope:          store.ScopePublic,
		Tags:           tags,
		Embedding:      embedding,
		Metadata: map[string]any{
			"decision_id":     signal.DecisionID,
			"agent_id":        signal.AgentID,
			"model_id":        signal.ModelID,
			"model_tier":      signal.ModelTier,
			"correction_type": signal.CorrectionType,
			"category":        signal.Category,
			"severity":        signal.Severity,
			"session_ref":     signal.SessionRef,
		},
		SourceEventID:  &eventID,
		Confidence:     0.9,
		RelevanceDecay: store.DecaySlow,
	}

	entry, err := s.knowledge.Create(ctx, input)
	if err != nil {
		s.logger.Error("failed to persist correction as lesson", "error", err, "event_id", envelope.ID, "decision_id", signal.DecisionID)
		s.ack(msg)
		return
	}

	s.logger.Info("captured Dredd correction as lesson",
		"knowledge_id", entry.ID,
		"event_id", envelope.ID,
		"agent_id", signal.AgentID,
		"category", signal.Category,
		"severity", signal.Severity,
	)

	// Publish vault.knowledge.created event
	if s.publisher != nil {
		_ = s.publisher.KnowledgeCreated(ctx, entry)
	}

	s.ack(msg)
}

func (s *Subscriber) captureEvent(msg *nats.Msg, category store.KnowledgeCategory, decay store.RelevanceDecay, confidence float64) {
	var event HermesEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		s.logger.Error("failed to parse Hermes event", "error", err, "subject", msg.Subject)
		s.ack(msg)
		return
	}

	ctx := context.Background()

	// Generate embedding
	embedding, err := s.embedder.Embed(ctx, event.Data.Content)
	if err != nil {
		s.logger.Error("failed to generate embedding", "error", err)
		// Still persist without embedding
	}

	summary := event.Data.Summary
	if summary == "" && len(event.Data.Content) > 100 {
		summary = event.Data.Content[:100] + "..."
	} else if summary == "" {
		summary = event.Data.Content
	}

	input := store.KnowledgeCreateInput{
		Content:        event.Data.Content,
		Summary:        &summary,
		SourceAgent:    event.Source,
		Category:       category,
		Scope:          store.ScopePublic,
		Tags:           event.Data.Tags,
		Embedding:      embedding,
		SourceEventID:  &event.ID,
		Confidence:     confidence,
		RelevanceDecay: decay,
	}

	entry, err := s.knowledge.Create(ctx, input)
	if err != nil {
		s.logger.Error("failed to persist Hermes event as knowledge", "error", err, "event_id", event.ID)
		s.ack(msg)
		return
	}

	s.logger.Info("auto-captured knowledge from Hermes",
		"knowledge_id", entry.ID,
		"event_id", event.ID,
		"category", category,
		"source", event.Source,
	)

	// Publish vault.knowledge.created event
	if s.publisher != nil {
		_ = s.publisher.KnowledgeCreated(ctx, entry)
	}

	s.ack(msg)
}

func (s *Subscriber) ack(msg *nats.Msg) {
	if msg.Reply != "" {
		_ = msg.Ack()
	}
}

func sanitizeSubject(subject string) string {
	r := ""
	for _, c := range subject {
		switch c {
		case '.', '>', '*':
			r += "-"
		default:
			r += string(c)
		}
	}
	return r
}
