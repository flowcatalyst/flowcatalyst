package dispatchjob

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.flowcatalyst.tech/internal/common/tsid"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrDuplicateJob = errors.New("duplicate job")
)

// mongoRepository provides MongoDB access to dispatch job data
type mongoRepository struct {
	jobs *mongo.Collection
}

// NewRepository creates a new dispatch job repository with instrumentation
func NewRepository(db *mongo.Database) Repository {
	return newInstrumentedRepository(&mongoRepository{
		jobs: db.Collection("dispatch_jobs"),
	})
}

// FindByID finds a dispatch job by ID
func (r *mongoRepository) FindByID(ctx context.Context, id string) (*DispatchJob, error) {
	var job DispatchJob
	err := r.jobs.FindOne(ctx, bson.M{"_id": id}).Decode(&job)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &job, nil
}

// FindByIdempotencyKey finds a dispatch job by idempotency key
func (r *mongoRepository) FindByIdempotencyKey(ctx context.Context, key string) (*DispatchJob, error) {
	var job DispatchJob
	err := r.jobs.FindOne(ctx, bson.M{"idempotencyKey": key}).Decode(&job)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &job, nil
}

// FindByEventID finds dispatch jobs for an event
func (r *mongoRepository) FindByEventID(ctx context.Context, eventID string) ([]*DispatchJob, error) {
	cursor, err := r.jobs.Find(ctx, bson.M{"eventId": eventID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []*DispatchJob
	if err := cursor.All(ctx, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// FindBySubscription finds dispatch jobs for a subscription
func (r *mongoRepository) FindBySubscription(ctx context.Context, subscriptionID string, skip, limit int64) ([]*DispatchJob, error) {
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	cursor, err := r.jobs.Find(ctx, bson.M{"subscriptionId": subscriptionID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []*DispatchJob
	if err := cursor.All(ctx, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// FindPending finds all pending jobs ready for dispatch
func (r *mongoRepository) FindPending(ctx context.Context, limit int64) ([]*DispatchJob, error) {
	filter := bson.M{
		"status": DispatchStatusPending,
		"$or": []bson.M{
			{"scheduledFor": bson.M{"$exists": false}},
			{"scheduledFor": bson.M{"$lte": time.Now()}},
		},
	}

	opts := options.Find().
		SetLimit(limit).
		SetSort(bson.D{{Key: "scheduledFor", Value: 1}, {Key: "createdAt", Value: 1}})

	cursor, err := r.jobs.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []*DispatchJob
	if err := cursor.All(ctx, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// FindPendingByPool finds pending jobs for a specific dispatch pool
func (r *mongoRepository) FindPendingByPool(ctx context.Context, poolID string, limit int64) ([]*DispatchJob, error) {
	filter := bson.M{
		"status":         DispatchStatusPending,
		"dispatchPoolId": poolID,
		"$or": []bson.M{
			{"scheduledFor": bson.M{"$exists": false}},
			{"scheduledFor": bson.M{"$lte": time.Now()}},
		},
	}

	opts := options.Find().
		SetLimit(limit).
		SetSort(bson.D{{Key: "scheduledFor", Value: 1}, {Key: "createdAt", Value: 1}})

	cursor, err := r.jobs.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []*DispatchJob
	if err := cursor.All(ctx, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// FindStaleQueued finds jobs that have been queued too long (stuck)
func (r *mongoRepository) FindStaleQueued(ctx context.Context, threshold time.Duration) ([]*DispatchJob, error) {
	staleTime := time.Now().Add(-threshold)

	filter := bson.M{
		"status":    DispatchStatusQueued,
		"updatedAt": bson.M{"$lt": staleTime},
	}

	cursor, err := r.jobs.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []*DispatchJob
	if err := cursor.All(ctx, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// Insert creates a new dispatch job
func (r *mongoRepository) Insert(ctx context.Context, job *DispatchJob) error {
	if job.ID == "" {
		job.ID = tsid.Generate()
	}
	now := time.Now()
	job.CreatedAt = now
	job.UpdatedAt = now

	if job.Status == "" {
		job.Status = DispatchStatusPending
	}

	_, err := r.jobs.InsertOne(ctx, job)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicateJob
	}
	return err
}

// InsertMany creates multiple dispatch jobs
func (r *mongoRepository) InsertMany(ctx context.Context, jobs []*DispatchJob) error {
	now := time.Now()
	docs := make([]interface{}, len(jobs))
	for i, job := range jobs {
		if job.ID == "" {
			job.ID = tsid.Generate()
		}
		job.CreatedAt = now
		job.UpdatedAt = now
		if job.Status == "" {
			job.Status = DispatchStatusPending
		}
		docs[i] = job
	}

	_, err := r.jobs.InsertMany(ctx, docs)
	return err
}

// Update updates an existing dispatch job
func (r *mongoRepository) Update(ctx context.Context, job *DispatchJob) error {
	job.UpdatedAt = time.Now()

	result, err := r.jobs.ReplaceOne(ctx, bson.M{"_id": job.ID}, job)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateStatus updates a job's status
func (r *mongoRepository) UpdateStatus(ctx context.Context, id string, status DispatchStatus) error {
	update := bson.M{
		"$set": bson.M{
			"status":    status,
			"updatedAt": time.Now(),
		},
	}

	result, err := r.jobs.UpdateOne(ctx, bson.M{"_id": id}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkQueued marks a job as queued
func (r *mongoRepository) MarkQueued(ctx context.Context, id string) error {
	return r.UpdateStatus(ctx, id, DispatchStatusQueued)
}

// MarkInProgress marks a job as in progress
func (r *mongoRepository) MarkInProgress(ctx context.Context, id string) error {
	return r.UpdateStatus(ctx, id, DispatchStatusInProgress)
}

// MarkCompleted marks a job as completed
func (r *mongoRepository) MarkCompleted(ctx context.Context, id string, durationMillis int64) error {
	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"status":         DispatchStatusCompleted,
			"completedAt":    now,
			"durationMillis": durationMillis,
			"updatedAt":      now,
		},
	}

	result, err := r.jobs.UpdateOne(ctx, bson.M{"_id": id}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkError marks a job as errored
func (r *mongoRepository) MarkError(ctx context.Context, id string, errorMsg string) error {
	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"status":    DispatchStatusError,
			"lastError": errorMsg,
			"updatedAt": now,
		},
	}

	result, err := r.jobs.UpdateOne(ctx, bson.M{"_id": id}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordAttempt records a delivery attempt
func (r *mongoRepository) RecordAttempt(ctx context.Context, id string, attempt DispatchAttempt) error {
	if attempt.ID == "" {
		attempt.ID = tsid.Generate()
	}
	now := time.Now()
	attempt.CreatedAt = now

	update := bson.M{
		"$push": bson.M{"attempts": attempt},
		"$set": bson.M{
			"lastAttemptAt": attempt.AttemptedAt,
			"updatedAt":     now,
		},
		"$inc": bson.M{"attemptCount": 1},
	}

	result, err := r.jobs.UpdateOne(ctx, bson.M{"_id": id}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// ResetToPending resets a job to pending status (for retry)
func (r *mongoRepository) ResetToPending(ctx context.Context, id string, scheduledFor time.Time) error {
	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"status":       DispatchStatusPending,
			"scheduledFor": scheduledFor,
			"updatedAt":    now,
		},
	}

	result, err := r.jobs.UpdateOne(ctx, bson.M{"_id": id}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// CountByStatus counts jobs by status
func (r *mongoRepository) CountByStatus(ctx context.Context, status DispatchStatus) (int64, error) {
	return r.jobs.CountDocuments(ctx, bson.M{"status": status})
}

// CountByGroupAndStatus counts jobs by message group and status
func (r *mongoRepository) CountByGroupAndStatus(ctx context.Context, messageGroup string, status DispatchStatus) (int64, error) {
	filter := bson.M{
		"messageGroup": messageGroup,
		"status":       status,
	}
	return r.jobs.CountDocuments(ctx, filter)
}

// HasErrorJobsInGroup returns true if there are any ERROR jobs in the message group
func (r *mongoRepository) HasErrorJobsInGroup(ctx context.Context, messageGroup string) (bool, error) {
	count, err := r.CountByGroupAndStatus(ctx, messageGroup, DispatchStatusError)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetBlockedMessageGroups returns message groups that have ERROR status jobs
// from the provided list of groups
func (r *mongoRepository) GetBlockedMessageGroups(ctx context.Context, groups []string) (map[string]bool, error) {
	if len(groups) == 0 {
		return map[string]bool{}, nil
	}

	// Query for ERROR status jobs in any of the provided groups
	pipeline := []bson.M{
		{
			"$match": bson.M{
				"messageGroup": bson.M{"$in": groups},
				"status":       DispatchStatusError,
			},
		},
		{
			"$group": bson.M{
				"_id": "$messageGroup",
			},
		},
	}

	cursor, err := r.jobs.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	blocked := make(map[string]bool)
	for cursor.Next(ctx) {
		var result struct {
			ID string `bson:"_id"`
		}
		if err := cursor.Decode(&result); err != nil {
			continue
		}
		blocked[result.ID] = true
	}

	return blocked, cursor.Err()
}

// Delete removes a dispatch job
func (r *mongoRepository) Delete(ctx context.Context, id string) error {
	result, err := r.jobs.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}
