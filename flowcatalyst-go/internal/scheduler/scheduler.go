// Package scheduler provides dispatch job scheduling
package scheduler

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"log/slog"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.flowcatalyst.tech/internal/common/leader"
	"go.flowcatalyst.tech/internal/common/metrics"
	"go.flowcatalyst.tech/internal/platform/dispatchjob"
	"go.flowcatalyst.tech/internal/queue"
	"go.flowcatalyst.tech/internal/router/model"
)

// SchedulerConfig holds configuration for the dispatch scheduler
type SchedulerConfig struct {
	// Database is the MongoDB database name
	Database string

	// PollInterval is how often to poll for pending jobs
	PollInterval time.Duration

	// BatchSize is the maximum jobs to fetch per poll
	BatchSize int

	// MaxConcurrentPools limits concurrent pool processing
	MaxConcurrentPools int

	// StaleThreshold is how long before a QUEUED job is considered stale
	StaleThreshold time.Duration

	// StaleCheckInterval is how often to check for stale jobs
	StaleCheckInterval time.Duration

	// LeaderElection enables distributed leader election
	LeaderElection LeaderElectionConfig

	// ProcessingEndpoint is the URL the message router calls back to process jobs
	// e.g., "http://localhost:8080/api/dispatch/process"
	ProcessingEndpoint string

	// DefaultDispatchPoolCode is the default pool code when job has none
	DefaultDispatchPoolCode string

	// AppKey is the secret key for HMAC auth token generation
	AppKey string
}

// LeaderElectionConfig holds leader election settings
type LeaderElectionConfig struct {
	// Enabled controls whether leader election is active
	Enabled bool

	// InstanceID uniquely identifies this instance
	InstanceID string

	// TTL is how long the lock is valid before expiring
	TTL time.Duration

	// RefreshInterval is how often to refresh the lock while primary
	RefreshInterval time.Duration
}

// DefaultSchedulerConfig returns sensible defaults
func DefaultSchedulerConfig() *SchedulerConfig {
	return &SchedulerConfig{
		Database:                "flowcatalyst",
		PollInterval:            5 * time.Second,
		BatchSize:               100,
		MaxConcurrentPools:      10,
		StaleThreshold:          15 * time.Minute,
		StaleCheckInterval:      60 * time.Second,
		ProcessingEndpoint:      "http://localhost:8080/api/dispatch/process",
		DefaultDispatchPoolCode: "DEFAULT-POOL",
	}
}

// Scheduler manages dispatch job scheduling
type Scheduler struct {
	client    *mongo.Client
	config    *SchedulerConfig
	publisher queue.Publisher

	collection    *mongo.Collection
	jobRepo       dispatchjob.Repository
	blockChecker  *BlockChecker
	leaderElector *leader.LeaderElector
	authService   *dispatchjob.DispatchAuthService

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	running   bool
	runningMu sync.Mutex
}

// NewScheduler creates a new dispatch scheduler
func NewScheduler(client *mongo.Client, publisher queue.Publisher, config *SchedulerConfig) *Scheduler {
	if config == nil {
		config = DefaultSchedulerConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())
	db := client.Database(config.Database)

	jobRepo := dispatchjob.NewRepository(db)

	// Create auth service for generating HMAC tokens
	authService := dispatchjob.NewDispatchAuthService(config.AppKey, nil)

	s := &Scheduler{
		client:       client,
		config:       config,
		publisher:    publisher,
		collection:   db.Collection("dispatch_jobs"),
		jobRepo:      jobRepo,
		blockChecker: NewBlockChecker(jobRepo),
		authService:  authService,
		ctx:          ctx,
		cancel:       cancel,
	}

	// Initialize leader elector if enabled
	if config.LeaderElection.Enabled {
		electorConfig := &leader.ElectorConfig{
			InstanceID:      config.LeaderElection.InstanceID,
			LockName:        "scheduler-leader",
			TTL:             config.LeaderElection.TTL,
			RefreshInterval: config.LeaderElection.RefreshInterval,
		}

		// Use defaults if not configured
		if electorConfig.TTL == 0 {
			electorConfig.TTL = 30 * time.Second
		}
		if electorConfig.RefreshInterval == 0 {
			electorConfig.RefreshInterval = 10 * time.Second
		}
		if electorConfig.InstanceID == "" {
			defaultCfg := leader.DefaultElectorConfig("scheduler-leader")
			electorConfig.InstanceID = defaultCfg.InstanceID
		}

		s.leaderElector = leader.NewLeaderElector(db, electorConfig)
	}

	return s
}

// Start starts the scheduler
func (s *Scheduler) Start() {
	s.runningMu.Lock()
	if s.running {
		s.runningMu.Unlock()
		slog.Warn("Scheduler already running")
		return
	}
	s.running = true
	s.runningMu.Unlock()

	// Start leader election if enabled
	if s.leaderElector != nil {
		if err := s.leaderElector.Start(s.ctx); err != nil {
			slog.Error("Failed to start leader election", "error", err)
		} else {
			slog.Info("Leader election enabled for scheduler", "instanceId", s.leaderElector.InstanceID(), "leaderElection", true)
		}
	}

	// Start polling goroutine
	s.wg.Add(1)
	go s.pollLoop()

	// Start stale job recovery goroutine
	s.wg.Add(1)
	go s.staleRecoveryLoop()

	slog.Info("Dispatch scheduler started", "pollInterval", s.config.PollInterval, "batchSize", s.config.BatchSize, "leaderElection", s.leaderElector != nil)
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.runningMu.Lock()
	if !s.running {
		s.runningMu.Unlock()
		return
	}
	s.running = false
	s.runningMu.Unlock()

	slog.Info("Stopping dispatch scheduler")

	s.cancel()
	s.wg.Wait()

	// Stop leader election if enabled
	if s.leaderElector != nil {
		s.leaderElector.Stop()
	}

	slog.Info("Dispatch scheduler stopped")
}

// IsRunning returns true if the scheduler is running
func (s *Scheduler) IsRunning() bool {
	s.runningMu.Lock()
	defer s.runningMu.Unlock()
	return s.running
}

// IsPrimary returns true if this instance is the leader (or leader election is disabled)
func (s *Scheduler) IsPrimary() bool {
	if s.leaderElector == nil {
		// No leader election configured, always primary
		return true
	}
	return s.leaderElector.IsPrimary()
}

// pollLoop is the main polling loop
func (s *Scheduler) pollLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.PollInterval)
	defer ticker.Stop()

	// Do an initial poll immediately
	s.pollAndDispatch()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.pollAndDispatch()
		}
	}
}

// pollAndDispatch polls for pending jobs and dispatches them
func (s *Scheduler) pollAndDispatch() {
	// Skip if not the leader
	if !s.IsPrimary() {
		slog.Debug("Skipping poll - not the leader")
		return
	}

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	// Find pending jobs that are ready to be scheduled
	now := time.Now()
	filter := bson.M{
		"status": "PENDING",
		"$or": []bson.M{
			{"scheduledFor": bson.M{"$lte": now}},
			{"scheduledFor": bson.M{"$exists": false}},
		},
	}

	opts := options.Find().
		SetSort(bson.M{"scheduledFor": 1, "createdAt": 1}).
		SetLimit(int64(s.config.BatchSize))

	cursor, err := s.collection.Find(ctx, filter, opts)
	if err != nil {
		slog.Error("Failed to poll for pending jobs", "error", err)
		return
	}
	defer cursor.Close(ctx)

	// Group jobs by dispatch pool
	jobsByPool := make(map[string][]*DispatchJob)

	for cursor.Next(ctx) {
		var job DispatchJob
		if err := cursor.Decode(&job); err != nil {
			slog.Error("Failed to decode dispatch job", "error", err)
			continue
		}

		poolCode := job.DispatchPoolID
		if poolCode == "" {
			poolCode = "default"
		}

		jobsByPool[poolCode] = append(jobsByPool[poolCode], &job)
	}

	if err := cursor.Err(); err != nil {
		slog.Error("Cursor error while polling jobs", "error", err)
		return
	}

	if len(jobsByPool) == 0 {
		return
	}

	totalJobs := 0
	for _, jobs := range jobsByPool {
		totalJobs += len(jobs)
	}

	// Record pending jobs metric
	metrics.SchedulerJobsPending.Set(float64(totalJobs))

	slog.Debug("Polled pending dispatch jobs", "jobCount", totalJobs, "poolCount", len(jobsByPool))

	// Process pools concurrently with limit
	sem := make(chan struct{}, s.config.MaxConcurrentPools)
	var wg sync.WaitGroup

	for poolCode, jobs := range jobsByPool {
		sem <- struct{}{}
		wg.Add(1)

		go func(pool string, poolJobs []*DispatchJob) {
			defer wg.Done()
			defer func() { <-sem }()

			s.dispatchPoolJobs(ctx, pool, poolJobs)
		}(poolCode, jobs)
	}

	wg.Wait()
}

// dispatchPoolJobs dispatches jobs for a specific pool
func (s *Scheduler) dispatchPoolJobs(ctx context.Context, poolCode string, jobs []*DispatchJob) {
	// Collect message groups from BLOCK_ON_ERROR jobs
	blockOnErrorGroups := make([]string, 0)
	groupSet := make(map[string]struct{})

	for _, job := range jobs {
		if job.IsBlockOnErrorMode() && job.MessageGroup != "" {
			if _, exists := groupSet[job.MessageGroup]; !exists {
				groupSet[job.MessageGroup] = struct{}{}
				blockOnErrorGroups = append(blockOnErrorGroups, job.MessageGroup)
			}
		}
	}

	// Get blocked groups if there are any BLOCK_ON_ERROR jobs
	blockedGroups := make(map[string]bool)
	if len(blockOnErrorGroups) > 0 {
		blockedGroups = s.blockChecker.GetBlockedGroups(ctx, blockOnErrorGroups)
	}

	// Dispatch jobs, skipping blocked ones
	dispatched := 0
	blocked := 0

	for _, job := range jobs {
		// Check if job should be blocked
		if job.IsBlockOnErrorMode() && blockedGroups[job.MessageGroup] {
			slog.Debug("Job blocked due to ERROR jobs in group", "jobId", job.ID, "pool", poolCode, "messageGroup", job.MessageGroup)
			blocked++
			continue
		}

		if err := s.dispatchJob(ctx, job); err != nil {
			slog.Error("Failed to dispatch job", "error", err, "jobId", job.ID, "pool", poolCode)
			continue
		}
		dispatched++
	}

	if blocked > 0 {
		slog.Info("Dispatched jobs with BLOCK_ON_ERROR filtering", "pool", poolCode, "dispatched", dispatched, "blocked", blocked, "blockedGroups", len(blockedGroups))
	}
}

// dispatchJob dispatches a single job to the queue
func (s *Scheduler) dispatchJob(ctx context.Context, job *DispatchJob) error {
	// Generate HMAC auth token for this dispatch job
	authToken, err := s.authService.GenerateAuthToken(job.ID)
	if err != nil {
		slog.Warn("Failed to generate auth token, using empty token", "error", err, "jobId", job.ID)
		authToken = ""
	}

	// Determine pool code
	poolCode := job.DispatchPoolID
	if poolCode == "" {
		poolCode = s.config.DefaultDispatchPoolCode
	}

	// Create MessagePointer matching Java format exactly
	// This is what gets serialized and sent through the queue
	pointer := &model.MessagePointer{
		ID:              job.ID,
		PoolCode:        poolCode,
		AuthToken:       authToken,
		MediationType:   model.MediationTypeHTTP,
		MediationTarget: s.config.ProcessingEndpoint,
		MessageGroupID:  job.MessageGroup,
		// BatchID is populated by router, not scheduler
	}

	// Serialize message as JSON (matching Java's ObjectMapper.writeValueAsString)
	data, err := json.Marshal(pointer)
	if err != nil {
		return err
	}

	// Determine subject
	subject := "dispatch." + poolCode
	if poolCode == "" {
		subject = "dispatch.default"
	}

	// Publish to queue
	if err := s.publisher.Publish(ctx, subject, data); err != nil {
		return err
	}

	// Update job status to QUEUED
	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"status":    "QUEUED",
			"queuedAt":  now,
			"updatedAt": now,
		},
	}

	_, err = s.collection.UpdateByID(ctx, job.ID, update)
	if err != nil {
		slog.Error("Failed to update job status to QUEUED", "error", err, "jobId", job.ID)
		// Job is already in queue, log the error but don't fail
	}

	// Record metrics
	metrics.SchedulerJobsScheduled.Inc()

	slog.Debug("Dispatched job to queue", "jobId", job.ID, "pool", poolCode, "subject", subject)

	return nil
}

// staleRecoveryLoop checks for and recovers stale jobs
func (s *Scheduler) staleRecoveryLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.StaleCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.recoverStaleJobs()
		}
	}
}

// recoverStaleJobs finds QUEUED jobs older than threshold and resets them
func (s *Scheduler) recoverStaleJobs() {
	// Skip if not the leader
	if !s.IsPrimary() {
		return
	}

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	staleThreshold := time.Now().Add(-s.config.StaleThreshold)

	filter := bson.M{
		"status":   "QUEUED",
		"queuedAt": bson.M{"$lt": staleThreshold},
	}

	update := bson.M{
		"$set": bson.M{
			"status":    "PENDING",
			"updatedAt": time.Now(),
		},
		"$unset": bson.M{
			"queuedAt": "",
		},
	}

	result, err := s.collection.UpdateMany(ctx, filter, update)
	if err != nil {
		slog.Error("Failed to recover stale jobs", "error", err)
		return
	}

	if result.ModifiedCount > 0 {
		// Record stale recovery metric
		metrics.SchedulerStaleJobs.Add(float64(result.ModifiedCount))

		slog.Warn("Recovered stale QUEUED jobs", "count", result.ModifiedCount, "threshold", s.config.StaleThreshold)
	}
}

// DispatchJob represents a dispatch job document
type DispatchJob struct {
	ID              string            `bson:"_id"`
	EventID         string            `bson:"eventId"`
	EventType       string            `bson:"eventType"`
	SubscriptionID  string            `bson:"subscriptionId"`
	DispatchPoolID  string            `bson:"dispatchPoolId"`
	Status          string            `bson:"status"`
	TargetURL       string            `bson:"targetUrl"`
	Headers         map[string]string `bson:"headers,omitempty"`
	Payload         string            `bson:"payload"`
	ContentType     string            `bson:"contentType"`
	MessageGroup    string            `bson:"messageGroup"`
	Mode            string            `bson:"mode,omitempty"` // IMMEDIATE, NEXT_ON_ERROR, BLOCK_ON_ERROR
	ScheduledFor    *time.Time        `bson:"scheduledFor,omitempty"`
	MaxRetries      int               `bson:"maxRetries"`
	AttemptCount    int               `bson:"attemptCount"`
	TimeoutSeconds  int               `bson:"timeoutSeconds"`
	CreatedAt       time.Time         `bson:"createdAt"`
}

// IsBlockOnErrorMode returns true if the job uses BLOCK_ON_ERROR dispatch mode
func (j *DispatchJob) IsBlockOnErrorMode() bool {
	return j.Mode == string(dispatchjob.DispatchModeBlockOnError)
}
