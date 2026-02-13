// Package semantic provides background semantic processing for entity embeddings,
// similarity scanning, and clustering.
package semantic

import (
	"os"
	"strconv"
	"time"
)

// Config holds semantic layer configuration loaded from environment variables.
type Config struct {
	Enabled bool

	// Similarity thresholds
	EdgeThreshold          float64 // min similarity to create an auto-edge
	ClusterJoinThreshold   float64 // min similarity to join a cluster
	AutoMergeThreshold     float64 // auto-merge clusters above this
	MergeProposalThreshold float64 // propose merge above this

	// Worker intervals
	EmbedInterval   time.Duration
	ScanInterval    time.Duration
	ClusterInterval time.Duration
	EmbedBatchSize  int
}

// ConfigFromEnv loads semantic configuration from environment variables.
func ConfigFromEnv() Config {
	return Config{
		Enabled:                envOrDefault("SEMANTIC_ENABLED", "") == "true",
		EdgeThreshold:          envFloatOrDefault("SEMANTIC_EDGE_THRESHOLD", 0.75),
		ClusterJoinThreshold:   envFloatOrDefault("SEMANTIC_CLUSTER_THRESHOLD", 0.70),
		AutoMergeThreshold:     envFloatOrDefault("SEMANTIC_AUTO_MERGE_THRESHOLD", 0.95),
		MergeProposalThreshold: envFloatOrDefault("SEMANTIC_MERGE_PROPOSAL_THRESHOLD", 0.85),
		EmbedInterval:          envDurationOrDefault("SEMANTIC_EMBED_INTERVAL", 30*time.Second),
		ScanInterval:           envDurationOrDefault("SEMANTIC_SCAN_INTERVAL", 5*time.Minute),
		ClusterInterval:        envDurationOrDefault("SEMANTIC_CLUSTER_INTERVAL", 15*time.Minute),
		EmbedBatchSize:         envIntOrDefault("SEMANTIC_EMBED_BATCH_SIZE", 50),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOrDefault(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envFloatOrDefault(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
