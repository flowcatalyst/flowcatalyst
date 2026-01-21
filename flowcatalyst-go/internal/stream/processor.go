// Package stream provides MongoDB change stream processing
package stream

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.flowcatalyst.tech/internal/common/health"
)

// ProcessorConfig holds configuration for the stream processor
type ProcessorConfig struct {
	// Database is the MongoDB database name
	Database string

	// EventsEnabled enables the events projection stream
	EventsEnabled bool

	// DispatchJobsEnabled enables the dispatch jobs projection stream
	DispatchJobsEnabled bool

	// BatchMaxSize is the maximum batch size before flushing
	BatchMaxSize int

	// BatchMaxWait is the maximum time to wait before flushing a batch
	BatchMaxWait time.Duration
}

// DefaultProcessorConfig returns sensible defaults
func DefaultProcessorConfig() *ProcessorConfig {
	return &ProcessorConfig{
		Database:            "flowcatalyst",
		EventsEnabled:       true,
		DispatchJobsEnabled: true,
		BatchMaxSize:        100,
		BatchMaxWait:        5 * time.Second,
	}
}

// Processor manages multiple MongoDB change stream watchers
type Processor struct {
	client          *mongo.Client
	config          *ProcessorConfig
	checkpointStore CheckpointStore
	watchers        []*Watcher
	running         bool
	runningMu       sync.Mutex
}

// NewProcessor creates a new stream processor
func NewProcessor(client *mongo.Client, config *ProcessorConfig) *Processor {
	if config == nil {
		config = DefaultProcessorConfig()
	}

	db := client.Database(config.Database)
	checkpointStore := NewMongoCheckpointStore(db)

	return &Processor{
		client:          client,
		config:          config,
		checkpointStore: checkpointStore,
		watchers:        make([]*Watcher, 0),
	}
}

// Start starts all configured stream watchers
func (p *Processor) Start() error {
	p.runningMu.Lock()
	if p.running {
		p.runningMu.Unlock()
		slog.Warn("Stream processor already running")
		return nil
	}
	p.running = true
	p.runningMu.Unlock()

	slog.Info("Starting stream processor")

	// Create events stream watcher
	if p.config.EventsEnabled {
		eventsConfig := &StreamConfig{
			Name:             "events",
			SourceCollection: "events",
			TargetCollection: "events_read",
			WatchOperations:  []string{"insert", "update", "replace"},
			BatchMaxSize:     p.config.BatchMaxSize,
			BatchMaxWait:     p.config.BatchMaxWait,
			CheckpointKey:    "events_projection",
		}

		eventsWatcher := NewWatcher(
			p.client,
			p.config.Database,
			eventsConfig,
			p.checkpointStore,
			NewEventProjectionMapper(),
		)
		p.watchers = append(p.watchers, eventsWatcher)
		eventsWatcher.Start()

		slog.Info("Events stream watcher started",
			"source", eventsConfig.SourceCollection,
			"target", eventsConfig.TargetCollection)
	}

	// Create dispatch jobs stream watcher
	if p.config.DispatchJobsEnabled {
		dispatchJobsConfig := &StreamConfig{
			Name:             "dispatch_jobs",
			SourceCollection: "dispatch_jobs",
			TargetCollection: "dispatch_jobs_read",
			WatchOperations:  []string{"insert", "update", "replace"},
			BatchMaxSize:     p.config.BatchMaxSize,
			BatchMaxWait:     p.config.BatchMaxWait,
			CheckpointKey:    "dispatch_jobs_projection",
		}

		dispatchJobsWatcher := NewWatcher(
			p.client,
			p.config.Database,
			dispatchJobsConfig,
			p.checkpointStore,
			NewDispatchJobProjectionMapper(),
		)
		p.watchers = append(p.watchers, dispatchJobsWatcher)
		dispatchJobsWatcher.Start()

		slog.Info("Dispatch jobs stream watcher started",
			"source", dispatchJobsConfig.SourceCollection,
			"target", dispatchJobsConfig.TargetCollection)
	}

	slog.Info("Stream processor started",
		"watcherCount", len(p.watchers))

	return nil
}

// Stop stops all stream watchers
func (p *Processor) Stop() {
	p.runningMu.Lock()
	if !p.running {
		p.runningMu.Unlock()
		return
	}
	p.running = false
	p.runningMu.Unlock()

	slog.Info("Stopping stream processor")

	// Stop all watchers concurrently
	var wg sync.WaitGroup
	for _, w := range p.watchers {
		wg.Add(1)
		go func(watcher *Watcher) {
			defer wg.Done()
			watcher.Stop()
		}(w)
	}
	wg.Wait()

	p.watchers = make([]*Watcher, 0)

	slog.Info("Stream processor stopped")
}

// IsRunning returns true if the processor is running
func (p *Processor) IsRunning() bool {
	p.runningMu.Lock()
	defer p.runningMu.Unlock()
	return p.running
}

// GetWatcherStatus returns status information for all watchers
func (p *Processor) GetWatcherStatus() []WatcherStatus {
	statuses := make([]WatcherStatus, 0, len(p.watchers))
	for _, w := range p.watchers {
		statuses = append(statuses, WatcherStatus{
			Name:    w.name,
			Running: w.IsRunning(),
		})
	}
	return statuses
}

// WatcherStatus holds status information for a watcher
type WatcherStatus struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
}

// StreamMetrics holds detailed metrics for a stream watcher (like Java's StreamContext)
type StreamMetrics struct {
	WatcherName      string `json:"watcherName"`
	Running          bool   `json:"running"`
	HasFatalError    bool   `json:"hasFatalError"`
	FatalError       string `json:"fatalError,omitempty"`
	BatchesProcessed int64  `json:"batchesProcessed"`
	CheckpointedSeq  int64  `json:"checkpointedSeq"`
	InFlightCount    int32  `json:"inFlightCount"`
	AvailableSlots   int32  `json:"availableSlots"`
}

// HealthCheck returns a health check function for the stream processor
func (p *Processor) HealthCheck() health.CheckFunc {
	return health.StreamProcessorCheckDetailed(
		p.IsRunning,
		func() interface{} {
			// Convert to interface slice to avoid type issues
			metrics := p.GetStreamMetrics()
			result := make([]health.StreamMetricsData, len(metrics))
			for i, m := range metrics {
				result[i] = health.StreamMetricsData{
					WatcherName:      m.WatcherName,
					Running:          m.Running,
					HasFatalError:    m.HasFatalError,
					FatalError:       m.FatalError,
					BatchesProcessed: m.BatchesProcessed,
					CheckpointedSeq:  m.CheckpointedSeq,
					InFlightCount:    m.InFlightCount,
					AvailableSlots:   m.AvailableSlots,
				}
			}
			return result
		},
	)
}

// GetWatcherStatusMap returns a map of watcher names to running status
func (p *Processor) GetWatcherStatusMap() map[string]bool {
	statuses := make(map[string]bool)
	for _, w := range p.watchers {
		statuses[w.name] = w.IsRunning()
	}
	return statuses
}

// GetStreamMetrics returns detailed metrics for all stream watchers
func (p *Processor) GetStreamMetrics() []StreamMetrics {
	result := make([]StreamMetrics, 0, len(p.watchers))
	for _, w := range p.watchers {
		m := StreamMetrics{
			WatcherName:      w.name,
			Running:          w.IsRunning(),
			HasFatalError:    w.HasFatalError(),
			BatchesProcessed: w.GetCurrentBatchSequence(),
			CheckpointedSeq:  w.GetLastCheckpointedSequence(),
			InFlightCount:    w.GetInFlightBatchCount(),
			AvailableSlots:   w.GetAvailableConcurrencySlots(),
		}
		if w.HasFatalError() {
			m.FatalError = w.GetFatalError().Error()
		}
		result = append(result, m)
	}
	return result
}

// EnsureIndexes creates necessary indexes on projection collections
// This matches Java's EventProjectionMapper and DispatchJobProjectionMapper index definitions
func (p *Processor) EnsureIndexes(ctx context.Context) error {
	db := p.client.Database(p.config.Database)

	// =========================================================================
	// Events read projection indexes
	// Matches Java's EventProjectionMapper.getIndexDefinitions()
	// =========================================================================
	eventsReadColl := db.Collection("events_read")
	eventsIndexes := []mongo.IndexModel{
		// -----------------------------------------------------------------
		// Global cascading filter indexes (no clientId filter)
		// Supports: app, app+subdomain, app+subdomain+aggregate, full type
		// -----------------------------------------------------------------

		// Global cascading filter - covers all non-client-scoped filter combos
		{
			Keys: bson.D{
				{Key: "application", Value: 1},
				{Key: "subdomain", Value: 1},
				{Key: "aggregate", Value: 1},
				{Key: "type", Value: 1},
				{Key: "time", Value: -1},
			},
		},

		// Global subject + time - for aggregate history across all
		{
			Keys: bson.D{
				{Key: "subject", Value: 1},
				{Key: "time", Value: -1},
			},
		},

		// -----------------------------------------------------------------
		// Client-scoped cascading filter indexes
		// Supports: client, client+app, client+app+subdomain, etc.
		// -----------------------------------------------------------------

		// Client-scoped cascading filter - covers all client-scoped filter combos
		{
			Keys: bson.D{
				{Key: "clientId", Value: 1},
				{Key: "application", Value: 1},
				{Key: "subdomain", Value: 1},
				{Key: "aggregate", Value: 1},
				{Key: "type", Value: 1},
				{Key: "time", Value: -1},
			},
		},

		// Client + subject + time - aggregate history within client
		{
			Keys: bson.D{
				{Key: "clientId", Value: 1},
				{Key: "subject", Value: 1},
				{Key: "time", Value: -1},
			},
		},

		// -----------------------------------------------------------------
		// Tracing and monitoring indexes
		// -----------------------------------------------------------------

		// Correlation ID - for distributed tracing (sparse - truly optional)
		{
			Keys:    bson.D{{Key: "correlationId", Value: 1}},
			Options: options.Index().SetSparse(true),
		},

		// Client + message group - for ordered processing within client context
		{
			Keys: bson.D{
				{Key: "clientId", Value: 1},
				{Key: "messageGroup", Value: 1},
			},
			Options: options.Index().SetSparse(true),
		},

		// Context data key/value lookup - multikey index for querying by contextData entries
		{
			Keys: bson.D{
				{Key: "contextData.key", Value: 1},
				{Key: "contextData.value", Value: 1},
			},
			Options: options.Index().SetSparse(true),
		},

		// Projection lag monitoring
		{Keys: bson.D{{Key: "projectedAt", Value: -1}}},
	}

	if _, err := eventsReadColl.Indexes().CreateMany(ctx, eventsIndexes); err != nil {
		slog.Error("Failed to create events_read indexes", "error", err)
		return err
	}
	slog.Info("Created events_read indexes", "count", len(eventsIndexes))

	// =========================================================================
	// Dispatch jobs read projection indexes
	// Matches Java's DispatchJobProjectionMapper.getIndexDefinitions()
	// =========================================================================
	dispatchJobsReadColl := db.Collection("dispatch_jobs_read")
	dispatchJobsIndexes := []mongo.IndexModel{
		// -----------------------------------------------------------------
		// Status-based indexes for job processing and monitoring
		// -----------------------------------------------------------------

		// Primary job lookup by status with time ordering
		{
			Keys: bson.D{
				{Key: "status", Value: 1},
				{Key: "scheduledFor", Value: 1},
			},
		},

		// Pool + status for pool-level job management
		{
			Keys: bson.D{
				{Key: "dispatchPoolId", Value: 1},
				{Key: "status", Value: 1},
				{Key: "scheduledFor", Value: 1},
			},
		},

		// Subscription + status for subscription-level monitoring
		{
			Keys: bson.D{
				{Key: "subscriptionId", Value: 1},
				{Key: "status", Value: 1},
				{Key: "createdAt", Value: -1},
			},
		},

		// -----------------------------------------------------------------
		// Client-scoped indexes
		// -----------------------------------------------------------------

		// Client cascading filter
		{
			Keys: bson.D{
				{Key: "clientId", Value: 1},
				{Key: "status", Value: 1},
				{Key: "createdAt", Value: -1},
			},
		},

		// Client + application for app-level job views
		{
			Keys: bson.D{
				{Key: "clientId", Value: 1},
				{Key: "applicationCode", Value: 1},
				{Key: "status", Value: 1},
				{Key: "createdAt", Value: -1},
			},
		},

		// -----------------------------------------------------------------
		// Lookup indexes
		// -----------------------------------------------------------------

		// Event ID lookup (find all jobs for an event)
		{Keys: bson.D{{Key: "eventId", Value: 1}}},

		// Correlation ID for tracing (sparse)
		{
			Keys:    bson.D{{Key: "metadata.correlationId", Value: 1}},
			Options: options.Index().SetSparse(true),
		},

		// Message group for ordered processing (sparse)
		{
			Keys: bson.D{
				{Key: "clientId", Value: 1},
				{Key: "messageGroup", Value: 1},
			},
			Options: options.Index().SetSparse(true),
		},

		// Projection lag monitoring
		{Keys: bson.D{{Key: "projectedAt", Value: -1}}},
	}

	if _, err := dispatchJobsReadColl.Indexes().CreateMany(ctx, dispatchJobsIndexes); err != nil {
		slog.Error("Failed to create dispatch_jobs_read indexes", "error", err)
		return err
	}
	slog.Info("Created dispatch_jobs_read indexes", "count", len(dispatchJobsIndexes))

	slog.Info("All projection indexes created successfully")
	return nil
}
