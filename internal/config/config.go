// Package config provides environment-based configuration for Alexandria.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the Alexandria service.
type Config struct {
	// Server
	Port     int
	LogLevel string

	// Database (Supabase PostgreSQL)
	DatabaseURL string

	// Supabase REST API (optional, for compatibility)
	SupabaseURL string
	SupabaseKey string

	// Encryption
	EncryptionKeyPath string
	EncryptionKey     string // loaded from file or env

	// NATS / Hermes
	NatsURL string

	// Embeddings
	EmbeddingBackend string // "simple" or "openai"
	OpenAIAPIKey     string
	OpenAIModel      string

	// Rate limiting
	KnowledgeRateLimit int           // requests per minute
	SecretRateLimit    int           // requests per minute
	BriefingRateLimit  int           // requests per minute
	RateWindow         time.Duration // window for rate limiting

	// JWT
	JWTSecret string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
	c := &Config{
		Port:               envInt("ALEXANDRIA_PORT", 8500),
		LogLevel:           envStr("ALEXANDRIA_LOG_LEVEL", "info"),
		DatabaseURL:        envStr("DATABASE_URL", ""),
		SupabaseURL:        envStr("SUPABASE_URL", "https://uaubofpmokvumbqpeymz.supabase.co"),
		SupabaseKey:        envStr("SUPABASE_SERVICE_KEY", ""),
		EncryptionKeyPath:  envStr("ENCRYPTION_KEY_PATH", "/run/secrets/vault_encryption_key"),
		EncryptionKey:      envStr("ENCRYPTION_KEY", ""),
		NatsURL:            envStr("NATS_URL", "nats://localhost:4222"),
		EmbeddingBackend:   envStr("EMBEDDING_BACKEND", "simple"),
		OpenAIAPIKey:       envStr("OPENAI_API_KEY", ""),
		OpenAIModel:        envStr("OPENAI_EMBEDDING_MODEL", "text-embedding-3-small"),
		KnowledgeRateLimit: envInt("KNOWLEDGE_RATE_LIMIT", 100),
		SecretRateLimit:    envInt("SECRET_RATE_LIMIT", 10),
		BriefingRateLimit:  envInt("BRIEFING_RATE_LIMIT", 5),
		RateWindow:         time.Minute,
		JWTSecret:          envStr("JWT_SECRET", ""),
	}

	// Load encryption key from file if not set via env
	if c.EncryptionKey == "" {
		data, err := os.ReadFile(c.EncryptionKeyPath)
		if err == nil {
			c.EncryptionKey = string(data)
		}
		// If still empty, generate a warning but don't fail — tests may not need it
	}

	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	return c, nil
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}
