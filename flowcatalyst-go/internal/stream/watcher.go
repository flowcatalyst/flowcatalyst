// Package stream provides MongoDB change stream processing
package stream

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"log/slog"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.flowcatalyst.tech/internal/common/metrics"
)

// StreamConfig holds configuration for a single stream
type StreamConfig struct {
	// Name is the stream name for logging
	Name string

	// SourceCollection is the collection to watch
	SourceCollection string

	// TargetCollection is the collection to write projections to
	TargetCollection string

	// WatchOperations are the operation types to watch (insert, update, replace)
	WatchOperations []string

	// BatchMaxSize is the maximum batch size before flushing
	BatchMaxSize int

	// BatchMaxWait is the maximum time to wait before flushing a batch
	BatchMaxWait time.Duration

	// CheckpointKey is the key for storing checkpoints
	CheckpointKey string
}

// ProjectionMapper maps source documents to projection documents
type ProjectionMapper interface {
	Map(doc bson.M) bson.M
}

// CheckpointStore stores and retrieves resume tokens
type CheckpointStore interface {
	// GetCheckpoint retrieves the resume token for the given key.
	// Returns nil if no checkpoint exists.
	GetCheckpoint(key string) (bson.Raw, error)
	// SaveCheckpoint saves the resume token for the given key.
	SaveCheckpoint(key string, token bson.Raw) error
}

// Watcher watches a MongoDB change stream
type Watcher struct {
	name              string
	client            *mongo.Client
	database          string
	config            *StreamConfig
	checkpointStore   CheckpointStore
	mapper            ProjectionMapper
	targetCollection  *mongo.Collection

	running    bool
	runningMu  sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup

	// Health metrics (like Java's StreamContext)
	batchSequence    atomic.Int64 // Total batches processed
	checkpointedSeq  atomic.Int64 // Last checkpointed sequence
	inFlightCount    atomic.Int32 // Batches currently processing
	fatalError       atomic.Value // Stores error or nil
	availableSlots   atomic.Int32 // Concurrency slots available (default 1 for single-threaded)
}

// NewWatcher creates a new stream watcher
func NewWatcher(
	client *mongo.Client,
	database string,
	config *StreamConfig,
	checkpointStore CheckpointStore,
	mapper ProjectionMapper,
) *Watcher {
	ctx, cancel := context.WithCancel(context.Background())

	w := &Watcher{
		name:             config.Name,
		client:           client,
		database:         database,
		config:           config,
		checkpointStore:  checkpointStore,
		mapper:           mapper,
		targetCollection: client.Database(database).Collection(config.TargetCollection),
		ctx:              ctx,
		cancel:           cancel,
	}

	// Initialize available slots (single-threaded by default)
	w.availableSlots.Store(1)

	return w
}

// Start starts watching the change stream
func (w *Watcher) Start() {
	w.runningMu.Lock()
	if w.running {
		w.runningMu.Unlock()
		slog.Warn("Watcher already running", "stream", w.name)
		return
	}
	w.running = true
	w.runningMu.Unlock()

	w.wg.Add(1)
	go w.watchLoop()

	slog.Info("Stream watcher started", "stream", w.name)
}

// Stop stops the watcher
func (w *Watcher) Stop() {
	slog.Info("Stopping stream watcher", "stream", w.name)

	w.runningMu.Lock()
	w.running = false
	w.runningMu.Unlock()

	w.cancel()
	w.wg.Wait()

	slog.Info("Stream watcher stopped", "stream", w.name)
}

// IsRunning returns true if the watcher is running
func (w *Watcher) IsRunning() bool {
	w.runningMu.Lock()
	defer w.runningMu.Unlock()
	return w.running
}

// GetCurrentBatchSequence returns the total number of batches processed
func (w *Watcher) GetCurrentBatchSequence() int64 {
	return w.batchSequence.Load()
}

// GetLastCheckpointedSequence returns the last checkpointed batch sequence
func (w *Watcher) GetLastCheckpointedSequence() int64 {
	return w.checkpointedSeq.Load()
}

// GetInFlightBatchCount returns the number of batches currently being processed
func (w *Watcher) GetInFlightBatchCount() int32 {
	return w.inFlightCount.Load()
}

// GetAvailableConcurrencySlots returns the available concurrency slots
func (w *Watcher) GetAvailableConcurrencySlots() int32 {
	return w.availableSlots.Load()
}

// HasFatalError returns true if the watcher has encountered a fatal error
func (w *Watcher) HasFatalError() bool {
	return w.fatalError.Load() != nil
}

// GetFatalError returns the fatal error if one occurred
func (w *Watcher) GetFatalError() error {
	if err := w.fatalError.Load(); err != nil {
		return err.(error)
	}
	return nil
}

// setFatalError sets the fatal error and stops the watcher
func (w *Watcher) setFatalError(err error) {
	w.fatalError.Store(err)
	w.runningMu.Lock()
	w.running = false
	w.runningMu.Unlock()
}

// Reconnection settings
const (
	initialBackoff = 5 * time.Second
	maxBackoff     = 60 * time.Second
	backoffMultiplier = 2.0
)

// watchLoop is the main watch loop with automatic reconnection
func (w *Watcher) watchLoop() {
	defer w.wg.Done()
	defer func() {
		w.runningMu.Lock()
		w.running = false
		w.runningMu.Unlock()
	}()

	sourceCollection := w.client.Database(w.database).Collection(w.config.SourceCollection)

	// Build pipeline to filter by operation types
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"operationType": bson.M{"$in": w.config.WatchOperations},
		}}},
	}

	consecutiveFailures := 0
	backoff := initialBackoff

	// Outer loop for reconnection
	for {
		// Check if we should stop
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		// Configure change stream options
		opts := options.ChangeStream().
			SetFullDocument(options.UpdateLookup).
			SetBatchSize(int32(w.config.BatchMaxSize))

		// Resume from checkpoint if available
		if w.checkpointStore != nil {
			if resumeToken, err := w.checkpointStore.GetCheckpoint(w.config.CheckpointKey); err == nil && resumeToken != nil {
				opts.SetResumeAfter(resumeToken)
				slog.Info("Resuming from checkpoint", "stream", w.name)
			} else if err != nil {
				slog.Warn("Failed to load checkpoint, starting from current position", "error", err, "stream", w.name)
			} else {
				slog.Info("No checkpoint found, starting from current position", "stream", w.name)
			}
		}

		slog.Info("Opening change stream",
			"stream", w.name,
			"source", w.config.SourceCollection,
			"target", w.config.TargetCollection,
			"operations", w.config.WatchOperations)

		// Open change stream
		stream, err := sourceCollection.Watch(w.ctx, pipeline, opts)
		if err != nil {
			consecutiveFailures++
			slog.Error("Failed to open change stream, will retry",
				"error", err,
				"stream", w.name,
				"attempt", consecutiveFailures,
				"backoff", backoff)

			// Wait before retry
			select {
			case <-w.ctx.Done():
				return
			case <-time.After(backoff):
			}

			// Increase backoff for next attempt
			backoff = time.Duration(float64(backoff) * backoffMultiplier)
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		// Reset backoff on successful connection
		consecutiveFailures = 0
		backoff = initialBackoff

		slog.Info("Change stream opened - waiting for documents", "stream", w.name)

		// Process the stream (inner loop)
		streamErr := w.processStream(stream)
		stream.Close(w.ctx)

		// Check why we exited
		if w.ctx.Err() != nil {
			// Context cancelled - clean shutdown
			return
		}

		if streamErr != nil {
			consecutiveFailures++
			slog.Warn("Change stream error, reconnecting",
				"error", streamErr,
				"stream", w.name,
				"attempt", consecutiveFailures,
				"backoff", backoff)

			// Check for stale resume token error
			if isStaleResumeTokenError(streamErr) {
				slog.Error("Resume token expired - clearing checkpoint and starting from current position. EVENTS MAY BE MISSED.",
					"stream", w.name)
				if w.checkpointStore != nil {
					w.clearCheckpoint()
				}
				backoff = initialBackoff // Don't backoff for stale token
				continue
			}

			// Wait before retry
			select {
			case <-w.ctx.Done():
				return
			case <-time.After(backoff):
			}

			// Increase backoff for next attempt
			backoff = time.Duration(float64(backoff) * backoffMultiplier)
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// processStream processes events from the change stream until an error or context cancellation
func (w *Watcher) processStream(stream *mongo.ChangeStream) error {
	batch := make([]bson.M, 0, w.config.BatchMaxSize)
	var lastToken bson.Raw
	batchStartTime := time.Now()

	for {
		select {
		case <-w.ctx.Done():
			// Flush remaining batch before exit
			if len(batch) > 0 {
				w.processBatch(batch, lastToken)
			}
			return nil

		default:
			// Check for next event with timeout
			ctx, cancel := context.WithTimeout(w.ctx, 100*time.Millisecond)
			hasNext := stream.TryNext(ctx)
			cancel()

			// Check for stream errors
			if err := stream.Err(); err != nil {
				// Flush batch before returning error
				if len(batch) > 0 {
					w.processBatch(batch, lastToken)
				}
				return err
			}

			if hasNext {
				var event bson.M
				if err := stream.Decode(&event); err != nil {
					slog.Error("Failed to decode change event", "error", err, "stream", w.name)
					continue
				}

				// Extract full document
				if fullDoc, ok := event["fullDocument"].(bson.M); ok {
					batch = append(batch, fullDoc)
					lastToken = stream.ResumeToken()
				}
			}

			// Check if we should flush the batch
			batchFull := len(batch) >= w.config.BatchMaxSize
			timeoutReached := time.Since(batchStartTime) >= w.config.BatchMaxWait

			if len(batch) > 0 && (batchFull || timeoutReached) {
				w.processBatch(batch, lastToken)
				batch = make([]bson.M, 0, w.config.BatchMaxSize)
				batchStartTime = time.Now()
			}
		}
	}
}

// isStaleResumeTokenError checks if the error is due to a stale/expired resume token
func isStaleResumeTokenError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// MongoDB error code 286 is ChangeStreamHistoryLost
	return contains(errStr, "ChangeStreamHistoryLost") ||
		contains(errStr, "resume token") ||
		contains(errStr, "oplog") ||
		contains(errStr, "invalidate")
}

// contains checks if s contains substr
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// clearCheckpoint clears the checkpoint (for stale resume token recovery)
func (w *Watcher) clearCheckpoint() {
	if store, ok := w.checkpointStore.(*MongoCheckpointStore); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := store.collection.DeleteOne(ctx, bson.M{"_id": w.config.CheckpointKey})
		if err != nil {
			slog.Warn("Failed to clear checkpoint", "error", err, "stream", w.name)
		} else {
			slog.Info("Checkpoint cleared", "stream", w.name)
		}
	}
}

// processBatch processes a batch of documents
func (w *Watcher) processBatch(batch []bson.M, resumeToken bson.Raw) {
	if len(batch) == 0 {
		return
	}

	batchStartTime := time.Now()

	// Track batch metrics (like Java's StreamContext)
	currentSeq := w.batchSequence.Add(1)
	w.inFlightCount.Add(1)
	w.availableSlots.Add(-1)
	defer func() {
		w.inFlightCount.Add(-1)
		w.availableSlots.Add(1)
	}()

	slog.Debug("Processing batch",
		"stream", w.name,
		"batchSize", len(batch),
		"batchSeq", currentSeq)

	// Track success/failure counts for metrics
	successCount := 0
	failureCount := 0

	// Map and upsert documents
	for _, doc := range batch {
		docStartTime := time.Now()

		projected := w.mapper.Map(doc)
		if projected == nil {
			continue
		}

		// Get ID for upsert
		id, ok := projected["_id"]
		if !ok {
			slog.Warn("Projected document has no _id", "stream", w.name)
			failureCount++
			metrics.StreamEventsProcessed.WithLabelValues(w.name, "failed").Inc()
			continue
		}

		// Upsert to target collection
		filter := bson.M{"_id": id}
		update := bson.M{"$set": projected}
		opts := options.Update().SetUpsert(true)

		_, err := w.targetCollection.UpdateOne(w.ctx, filter, update, opts)
		if err != nil {
			slog.Error("Failed to upsert projection",
				"error", err,
				"stream", w.name,
				"id", id)
			failureCount++
			metrics.StreamEventsProcessed.WithLabelValues(w.name, "failed").Inc()
		} else {
			successCount++
			metrics.StreamEventsProcessed.WithLabelValues(w.name, "success").Inc()
		}

		// Record per-document processing duration
		metrics.StreamProcessingDuration.WithLabelValues(w.name).Observe(time.Since(docStartTime).Seconds())
	}

	// Save checkpoint and update checkpointed sequence
	if w.checkpointStore != nil && resumeToken != nil {
		if err := w.checkpointStore.SaveCheckpoint(w.config.CheckpointKey, resumeToken); err != nil {
			slog.Error("Failed to save checkpoint", "error", err, "stream", w.name)
		} else {
			w.checkpointedSeq.Store(currentSeq)
		}
	}

	// Update stream lag metric (difference between current and checkpointed sequence)
	lag := currentSeq - w.checkpointedSeq.Load()
	metrics.StreamLag.WithLabelValues(w.name).Set(float64(lag))

	slog.Debug("Batch processed",
		"stream", w.name,
		"processed", len(batch),
		"success", successCount,
		"failed", failureCount,
		"batchSeq", currentSeq,
		"duration", time.Since(batchStartTime))
}

// MongoCheckpointStore stores checkpoints in MongoDB
type MongoCheckpointStore struct {
	collection *mongo.Collection
}

// NewMongoCheckpointStore creates a new MongoDB checkpoint store
func NewMongoCheckpointStore(db *mongo.Database) *MongoCheckpointStore {
	return &MongoCheckpointStore{
		collection: db.Collection("stream_checkpoints"),
	}
}

// GetCheckpoint retrieves a checkpoint (resume token)
func (s *MongoCheckpointStore) GetCheckpoint(key string) (bson.Raw, error) {
	var doc struct {
		ResumeToken bson.Raw `bson:"resumeToken"`
	}

	err := s.collection.FindOne(context.Background(), bson.M{"_id": key}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	// Return nil if no token stored (empty bson.Raw)
	if len(doc.ResumeToken) == 0 {
		return nil, nil
	}

	return doc.ResumeToken, nil
}

// SaveCheckpoint saves a checkpoint
func (s *MongoCheckpointStore) SaveCheckpoint(key string, token bson.Raw) error {
	filter := bson.M{"_id": key}
	update := bson.M{
		"$set": bson.M{
			"resumeToken": token,
			"updatedAt":   time.Now(),
		},
	}
	opts := options.Update().SetUpsert(true)

	_, err := s.collection.UpdateOne(context.Background(), filter, update, opts)
	return err
}
