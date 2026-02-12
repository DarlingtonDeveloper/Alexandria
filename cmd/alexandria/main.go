// Package main is the entry point for the Alexandria service.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/warrentherabbit/alexandria/internal/config"
	"github.com/warrentherabbit/alexandria/internal/embeddings"
	"github.com/warrentherabbit/alexandria/internal/encryption"
	"github.com/warrentherabbit/alexandria/internal/hermes"
	"github.com/warrentherabbit/alexandria/internal/server"
	"github.com/warrentherabbit/alexandria/internal/store"
)

func main() {
	// Logger
	logLevel := slog.LevelInfo
	if os.Getenv("ALEXANDRIA_LOG_LEVEL") == "debug" {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	// Config
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Database
	db, err := store.NewDB(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	logger.Info("connected to database")

	// Embedding provider
	var embedder embeddings.Provider
	switch cfg.EmbeddingBackend {
	case "openai":
		if cfg.OpenAIAPIKey == "" {
			logger.Error("OpenAI API key required for openai embedding backend")
			os.Exit(1)
		}
		embedder = embeddings.NewOpenAIProvider(cfg.OpenAIAPIKey, cfg.OpenAIModel)
	default:
		embedder = embeddings.NewSimpleProvider()
	}
	logger.Info("embedding provider initialized", "backend", embedder.Name())

	// Encryption
	var encryptor *encryption.Encryptor
	if cfg.EncryptionKey != "" {
		encryptor, err = encryption.NewEncryptor(cfg.EncryptionKey)
		if err != nil {
			logger.Warn("failed to initialize encryptor, secret management disabled", "error", err)
		}
	}
	if encryptor == nil {
		logger.Warn("no encryption key configured, using ephemeral key for development")
		// Generate a temporary key for development
		key, _ := encryption.GenerateKey()
		if key != nil {
			encryptor, _ = encryption.NewEncryptor(key.Encode())
		}
	}

	// Hermes (NATS) — optional, service works without it
	var hermesClient *hermes.Client
	if cfg.NatsURL != "" {
		hermesClient, err = hermes.NewClient(cfg.NatsURL, logger)
		if err != nil {
			logger.Warn("failed to connect to Hermes (NATS), running without event bus", "error", err)
			hermesClient = nil
		} else {
			defer hermesClient.Close()
			logger.Info("connected to Hermes (NATS)", "url", cfg.NatsURL)

			// Start subscriber for auto-capture
			knowledgeStore := store.NewKnowledgeStore(db)
			publisher := hermes.NewPublisher(hermesClient, logger)
			subscriber := hermes.NewSubscriber(hermesClient, knowledgeStore, embedder, publisher, logger)
			if err := subscriber.Start(ctx); err != nil {
				logger.Warn("failed to start Hermes subscriber", "error", err)
			} else {
				defer subscriber.Stop()
			}
		}
	}

	// Server
	srv := server.New(cfg, db, hermesClient, embedder, encryptor, logger)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      srv.Router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		logger.Info("shutting down gracefully...")
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer shutdownCancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown error", "error", err)
		}
	}()

	logger.Info("Alexandria starting", "port", cfg.Port)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}

	logger.Info("Alexandria stopped")
}
