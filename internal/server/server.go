// Package server provides the HTTP server setup for Alexandria.
package server

import (
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/warrentherabbit/alexandria/internal/api"
	"github.com/warrentherabbit/alexandria/internal/briefings"
	"github.com/warrentherabbit/alexandria/internal/config"
	"github.com/warrentherabbit/alexandria/internal/embeddings"
	"github.com/warrentherabbit/alexandria/internal/encryption"
	"github.com/warrentherabbit/alexandria/internal/hermes"
	"github.com/warrentherabbit/alexandria/internal/middleware"
	"github.com/warrentherabbit/alexandria/internal/store"

	"log/slog"
)

// Server holds all dependencies for the Alexandria HTTP server.
type Server struct {
	Router    *chi.Mux
	Config    *config.Config
	DB        *store.DB
	Hermes    *hermes.Client
	Publisher *hermes.Publisher
	Logger    *slog.Logger
}

// New creates a new Server with all routes configured.
func New(cfg *config.Config, db *store.DB, hermesClient *hermes.Client, embedder embeddings.Provider, encryptor *encryption.Encryptor, logger *slog.Logger) *Server {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))
	r.Use(middleware.RequestLogging(logger))
	r.Use(middleware.AgentAuth(cfg.JWTSecret))

	// Stores
	knowledgeStore := store.NewKnowledgeStore(db)
	secretStore := store.NewSecretStore(db)
	graphStore := store.NewGraphStore(db)
	auditStore := store.NewAuditStore(db)

	// Publisher (may be nil if NATS not available)
	var publisher *hermes.Publisher
	if hermesClient != nil {
		publisher = hermes.NewPublisher(hermesClient, logger)
	}

	// Handlers
	healthHandler := api.NewHealthHandler(db, knowledgeStore, secretStore, hermesClient)
	knowledgeHandler := api.NewKnowledgeHandler(knowledgeStore, auditStore, embedder, publisher)
	secretHandler := api.NewSecretHandler(secretStore, auditStore, encryptor, publisher)
	briefingAssembler := briefings.NewAssembler(knowledgeStore, secretStore)
	briefingHandler := api.NewBriefingHandler(briefingAssembler, auditStore, publisher)
	graphHandler := api.NewGraphHandler(graphStore, auditStore)

	// Rate limiters
	knowledgeRL := middleware.NewRateLimiter(cfg.KnowledgeRateLimit, cfg.RateWindow)
	secretRL := middleware.NewRateLimiter(cfg.SecretRateLimit, cfg.RateWindow)
	briefingRL := middleware.NewRateLimiter(cfg.BriefingRateLimit, cfg.RateWindow)

	// Routes
	r.Route("/api/v1", func(r chi.Router) {
		// Health (no rate limit)
		r.Get("/health", healthHandler.Health)
		r.Get("/stats", healthHandler.Stats)

		// Knowledge
		r.Route("/knowledge", func(r chi.Router) {
			r.Use(knowledgeRL.Middleware)
			r.Post("/", knowledgeHandler.Create)
			r.Get("/", knowledgeHandler.List)
			r.Post("/search", knowledgeHandler.Search)
			r.Post("/batch", knowledgeHandler.BatchCreate)
			r.Get("/{id}", knowledgeHandler.Get)
			r.Put("/{id}", knowledgeHandler.Update)
			r.Delete("/{id}", knowledgeHandler.Delete)
		})

		// Secrets
		r.Route("/secrets", func(r chi.Router) {
			r.Use(secretRL.Middleware)
			r.Post("/", secretHandler.Create)
			r.Get("/", secretHandler.List)
			r.Get("/{name}", secretHandler.Get)
			r.Put("/{name}", secretHandler.Update)
			r.Delete("/{name}", secretHandler.Delete)
			r.Post("/{name}/rotate", secretHandler.Rotate)
		})

		// Briefings
		r.Route("/briefings", func(r chi.Router) {
			r.Use(briefingRL.Middleware)
			r.Get("/{agent_id}", briefingHandler.Generate)
		})

		// Knowledge Graph
		r.Route("/graph", func(r chi.Router) {
			r.Use(knowledgeRL.Middleware)
			r.Get("/entities", graphHandler.ListEntities)
			r.Post("/entities", graphHandler.CreateEntity)
			r.Get("/entities/{id}", graphHandler.GetEntity)
			r.Get("/entities/{id}/related", graphHandler.GetRelatedEntities)
			r.Post("/relationships", graphHandler.CreateRelationship)
		})
	})

	return &Server{
		Router:    r,
		Config:    cfg,
		DB:        db,
		Hermes:    hermesClient,
		Publisher: publisher,
		Logger:    logger,
	}
}
