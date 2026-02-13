package hermes

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

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
