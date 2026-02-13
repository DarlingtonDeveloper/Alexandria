package semantic

import (
	"context"
	"log/slog"
	"time"

	"github.com/warrentherabbit/alexandria/internal/store"
)

// Worker runs background semantic processing goroutines.
type Worker struct {
	db       *store.DB
	provider EmbeddingProvider
	config   Config
	logger   *slog.Logger
}

// NewWorker creates a semantic worker.
func NewWorker(db *store.DB, provider EmbeddingProvider, cfg Config, logger *slog.Logger) *Worker {
	return &Worker{
		db:       db,
		provider: provider,
		config:   cfg,
		logger:   logger,
	}
}

// Start launches background goroutines. They run until ctx is cancelled.
func (w *Worker) Start(ctx context.Context) {
	w.logger.Info("semantic worker starting")

	go w.runLoop(ctx, "embedder", w.config.EmbedInterval, w.embedBatch)
	go w.runLoop(ctx, "similarity-scanner", w.config.ScanInterval, w.scanSimilarity)
	go w.runLoop(ctx, "cluster-detector", w.config.ClusterInterval, w.detectClusters)
}

func (w *Worker) runLoop(ctx context.Context, name string, interval time.Duration, fn func(ctx context.Context) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run once immediately
	if err := fn(ctx); err != nil {
		w.logger.Warn("semantic initial run", "worker", name, "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("semantic worker shutting down", "worker", name)
			return
		case <-ticker.C:
			if err := fn(ctx); err != nil {
				w.logger.Warn("semantic worker error", "worker", name, "error", err)
			}
		}
	}
}
