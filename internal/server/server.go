// Package server provides the HTTP server setup for Alexandria.
package server

import (
	"time"

	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/MikeSquared-Agency/Alexandria/internal/api"
	"github.com/MikeSquared-Agency/Alexandria/internal/bootctx"
	"github.com/MikeSquared-Agency/Alexandria/internal/briefings"
	"github.com/MikeSquared-Agency/Alexandria/internal/config"
	"github.com/MikeSquared-Agency/Alexandria/internal/embeddings"
	"github.com/MikeSquared-Agency/Alexandria/internal/encryption"
	"github.com/MikeSquared-Agency/Alexandria/internal/hermes"
	"github.com/MikeSquared-Agency/Alexandria/internal/identity"
	"github.com/MikeSquared-Agency/Alexandria/internal/middleware"
	"github.com/MikeSquared-Agency/Alexandria/internal/store"

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
func New(cfg *config.Config, db *store.DB, hermesClient *hermes.Client, embedder embeddings.Provider, encryptor *encryption.Encryptor, resolver *identity.Resolver, logger *slog.Logger) *Server {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))
	r.Use(middleware.RequestLogging(logger))
	r.Use(middleware.APIKeyAuth(cfg.APIKey))
	r.Use(middleware.AgentAuth(cfg.JWTSecret))

	// Stores
	knowledgeStore := store.NewKnowledgeStore(db)
	secretStore := store.NewSecretStore(db)
	graphStore := store.NewGraphStore(db)
	auditStore := store.NewAuditStore(db)

	// New access control stores
	peopleStore := store.NewPersonStore(db)
	devicesStore := store.NewDeviceStore(db)
	grantsStore := store.NewGrantStore(db)

	// Publisher (may be nil if NATS not available)
	var publisher *hermes.Publisher
	if hermesClient != nil {
		publisher = hermes.NewPublisher(hermesClient, logger)
	}

	// Handlers
	healthHandler := api.NewHealthHandler(db, knowledgeStore, secretStore, hermesClient)
	knowledgeHandler := api.NewKnowledgeHandler(knowledgeStore, auditStore, embedder, publisher)
	secretHandler := api.NewSecretHandler(secretStore, grantsStore, auditStore, encryptor, publisher)
	briefingAssembler := briefings.NewAssembler(knowledgeStore, secretStore)
	briefingHandler := api.NewBriefingHandler(briefingAssembler, auditStore, publisher)
	contextAssembler := bootctx.NewAssembler(knowledgeStore, secretStore, graphStore, grantsStore)
	contextHandler := api.NewContextHandler(contextAssembler, auditStore, publisher, logger)
	graphHandler := api.NewGraphHandler(graphStore, auditStore)

	// New access control handlers
	peopleHandler := api.NewPeopleHandler(peopleStore, auditStore)
	devicesHandler := api.NewDevicesHandler(devicesStore, auditStore)
	grantsHandler := api.NewGrantsHandler(grantsStore, auditStore)

	// Identity + Semantic handlers
	identityHandler := api.NewIdentityHandler(resolver, db, auditStore)
	semanticHandler := api.NewSemanticHandler(db)

	// Rate limiters
	knowledgeRL := middleware.NewRateLimiter(cfg.KnowledgeRateLimit, cfg.RateWindow)
	secretRL := middleware.NewRateLimiter(cfg.SecretRateLimit, cfg.RateWindow)
	briefingRL := middleware.NewRateLimiter(cfg.BriefingRateLimit, cfg.RateWindow)

	// Root-level health and info (no auth required)
	r.Get("/health", healthHandler.Health)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"service":"alexandria","version":"0.1.0"}`))
	})

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

		// Boot Context
		r.Route("/context", func(r chi.Router) {
			r.Use(briefingRL.Middleware)
			r.Get("/{agent_id}", contextHandler.Generate)
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

		// Access Control - People
		r.Route("/people", func(r chi.Router) {
			r.Use(knowledgeRL.Middleware)
			r.Post("/", peopleHandler.Create)
			r.Get("/", peopleHandler.List)
			r.Get("/{id}", peopleHandler.Get)
			r.Put("/{id}", peopleHandler.Update)
			r.Delete("/{id}", peopleHandler.Delete)
		})

		// Access Control - Devices
		r.Route("/devices", func(r chi.Router) {
			r.Use(knowledgeRL.Middleware)
			r.Post("/", devicesHandler.Create)
			r.Get("/", devicesHandler.List)
			r.Get("/{id}", devicesHandler.Get)
			r.Put("/{id}", devicesHandler.Update)
			r.Delete("/{id}", devicesHandler.Delete)
		})

		// Access Control - Grants
		r.Route("/grants", func(r chi.Router) {
			r.Use(knowledgeRL.Middleware)
			r.Post("/", grantsHandler.Create)
			r.Get("/", grantsHandler.List)
			r.Get("/check", grantsHandler.CheckAccess)
			r.Get("/{id}", grantsHandler.Get)
			r.Delete("/{id}", grantsHandler.Delete)
		})

		// Identity Resolution
		r.Route("/identity", func(r chi.Router) {
			r.Use(knowledgeRL.Middleware)
			r.Post("/resolve", identityHandler.Resolve)
			r.Post("/merge", identityHandler.Merge)
			r.Get("/pending", identityHandler.Pending)
			r.Post("/aliases/{id}/review", identityHandler.ReviewAlias)
			r.Get("/entities/{id}", identityHandler.EntityLookup)
		})

		// Semantic Layer
		r.Route("/semantic", func(r chi.Router) {
			r.Use(knowledgeRL.Middleware)
			r.Get("/status", semanticHandler.Status)
			r.Get("/similar/{id}", semanticHandler.SimilarEntities)
			r.Get("/clusters", semanticHandler.ListClusters)
			r.Get("/clusters/{id}/members", semanticHandler.ClusterMembers)
			r.Get("/entities/{id}/clusters", semanticHandler.EntityClusters)
			r.Get("/proposals", semanticHandler.Proposals)
			r.Post("/proposals/{id}/review", semanticHandler.ReviewProposal)
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
