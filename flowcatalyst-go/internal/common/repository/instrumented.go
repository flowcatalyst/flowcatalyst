package repository

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// dbOperationDuration tracks the duration of database operations
	dbOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "flowcatalyst",
			Subsystem: "db",
			Name:      "operation_duration_seconds",
			Help:      "Database operation duration in seconds",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"collection", "operation"},
	)

	// dbOperationTotal counts total database operations
	dbOperationTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "flowcatalyst",
			Subsystem: "db",
			Name:      "operations_total",
			Help:      "Total database operations",
		},
		[]string{"collection", "operation", "result"},
	)

	// dbOperationErrors counts database operation errors by type
	dbOperationErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "flowcatalyst",
			Subsystem: "db",
			Name:      "operation_errors_total",
			Help:      "Database operation errors by type",
		},
		[]string{"collection", "operation", "error_type"},
	)
)

// SlowQueryThreshold defines when a query is considered slow
const SlowQueryThreshold = 100 * time.Millisecond

// Instrument wraps a repository operation with metrics and logging.
// It records duration, success/failure counts, and logs slow queries.
func Instrument[T any](
	ctx context.Context,
	collection string,
	operation string,
	fn func() (T, error),
) (T, error) {
	start := time.Now()

	result, err := fn()

	duration := time.Since(start)

	// Record duration metric
	dbOperationDuration.WithLabelValues(collection, operation).Observe(duration.Seconds())

	if err != nil {
		// Record error metrics
		dbOperationTotal.WithLabelValues(collection, operation, "error").Inc()
		dbOperationErrors.WithLabelValues(collection, operation, classifyError(err)).Inc()

		// Always log errors
		slog.Error("Database operation failed",
			"collection", collection,
			"operation", operation,
			"duration_ms", duration.Milliseconds(),
			"error", err)
	} else {
		// Record success
		dbOperationTotal.WithLabelValues(collection, operation, "success").Inc()

		// Log slow queries
		if duration > SlowQueryThreshold {
			slog.Warn("Slow database operation",
				"collection", collection,
				"operation", operation,
				"duration_ms", duration.Milliseconds())
		}
	}

	return result, err
}

// InstrumentVoid wraps a repository operation that returns only an error.
func InstrumentVoid(
	ctx context.Context,
	collection string,
	operation string,
	fn func() error,
) error {
	_, err := Instrument(ctx, collection, operation, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}

// classifyError returns a label-safe error type for metrics
func classifyError(err error) string {
	if errors.Is(err, ErrNotFound) {
		return "not_found"
	}
	if errors.Is(err, ErrDuplicateKey) {
		return "duplicate_key"
	}
	if errors.Is(err, ErrOptimisticLock) {
		return "optimistic_lock"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	return "internal"
}
