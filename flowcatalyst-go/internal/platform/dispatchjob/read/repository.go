package read

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var ErrNotFound = errors.New("not found")

// Repository provides read access to denormalized dispatch jobs
type Repository struct {
	collection *mongo.Collection
}

// NewRepository creates a new DispatchJobRead repository
func NewRepository(db *mongo.Database) *Repository {
	return &Repository{
		collection: db.Collection("dispatch_jobs_read"),
	}
}

// FindByID finds a dispatch job by ID
func (r *Repository) FindByID(ctx context.Context, id string) (*DispatchJobRead, error) {
	var job DispatchJobRead
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&job)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &job, nil
}

// FindByClientID finds dispatch jobs by client ID with pagination
func (r *Repository) FindByClientID(ctx context.Context, clientID string, skip, limit int64) ([]*DispatchJobRead, error) {
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	cursor, err := r.collection.Find(ctx, bson.M{"clientId": clientID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []*DispatchJobRead
	if err := cursor.All(ctx, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// FindBySubscription finds dispatch jobs by subscription ID
func (r *Repository) FindBySubscription(ctx context.Context, subscriptionID string, skip, limit int64) ([]*DispatchJobRead, error) {
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	cursor, err := r.collection.Find(ctx, bson.M{"subscriptionId": subscriptionID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []*DispatchJobRead
	if err := cursor.All(ctx, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// FindByDispatchPool finds dispatch jobs by dispatch pool ID
func (r *Repository) FindByDispatchPool(ctx context.Context, dispatchPoolID string, skip, limit int64) ([]*DispatchJobRead, error) {
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	cursor, err := r.collection.Find(ctx, bson.M{"dispatchPoolId": dispatchPoolID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []*DispatchJobRead
	if err := cursor.All(ctx, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// FindByStatus finds dispatch jobs by status
func (r *Repository) FindByStatus(ctx context.Context, status DispatchStatus, skip, limit int64) ([]*DispatchJobRead, error) {
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	cursor, err := r.collection.Find(ctx, bson.M{"status": status}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []*DispatchJobRead
	if err := cursor.All(ctx, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// FindPendingJobs finds all pending jobs that can be processed
func (r *Repository) FindPendingJobs(ctx context.Context, limit int64) ([]*DispatchJobRead, error) {
	opts := options.Find().
		SetLimit(limit).
		SetSort(bson.D{{Key: "scheduledFor", Value: 1}})

	filter := bson.M{
		"status":     DispatchStatusPending,
		"isTerminal": false,
		"$or": []bson.M{
			{"scheduledFor": bson.M{"$exists": false}},
			{"scheduledFor": bson.M{"$lte": time.Now()}},
		},
	}
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []*DispatchJobRead
	if err := cursor.All(ctx, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// FindRetryableJobs finds jobs that can be retried
func (r *Repository) FindRetryableJobs(ctx context.Context, limit int64) ([]*DispatchJobRead, error) {
	opts := options.Find().
		SetLimit(limit).
		SetSort(bson.D{{Key: "lastAttemptAt", Value: 1}})

	filter := bson.M{
		"status":   DispatchStatusError,
		"canRetry": true,
	}
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []*DispatchJobRead
	if err := cursor.All(ctx, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// FindByEventID finds dispatch jobs for a specific event
func (r *Repository) FindByEventID(ctx context.Context, eventID string) ([]*DispatchJobRead, error) {
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}})

	cursor, err := r.collection.Find(ctx, bson.M{"eventId": eventID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []*DispatchJobRead
	if err := cursor.All(ctx, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// FindByCorrelationID finds dispatch jobs by correlation ID
func (r *Repository) FindByCorrelationID(ctx context.Context, correlationID string) ([]*DispatchJobRead, error) {
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}})

	cursor, err := r.collection.Find(ctx, bson.M{"correlationId": correlationID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []*DispatchJobRead
	if err := cursor.All(ctx, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// FindByMessageGroup finds dispatch jobs by message group
func (r *Repository) FindByMessageGroup(ctx context.Context, messageGroup string) ([]*DispatchJobRead, error) {
	opts := options.Find().SetSort(bson.D{{Key: "sequence", Value: 1}})

	cursor, err := r.collection.Find(ctx, bson.M{"messageGroup": messageGroup}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []*DispatchJobRead
	if err := cursor.All(ctx, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// CountByStatus returns count of jobs by status
func (r *Repository) CountByStatus(ctx context.Context, status DispatchStatus) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{"status": status})
}

// CountBySubscription returns count of jobs for a subscription
func (r *Repository) CountBySubscription(ctx context.Context, subscriptionID string) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{"subscriptionId": subscriptionID})
}

// CountTerminalBySubscription returns count of terminal jobs for a subscription
func (r *Repository) CountTerminalBySubscription(ctx context.Context, subscriptionID string) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{
		"subscriptionId": subscriptionID,
		"isTerminal":     true,
	})
}

// CountCompletedBySubscription returns count of completed jobs for a subscription
func (r *Repository) CountCompletedBySubscription(ctx context.Context, subscriptionID string) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{
		"subscriptionId": subscriptionID,
		"isCompleted":    true,
	})
}

// Upsert inserts or updates a dispatch job read projection
func (r *Repository) Upsert(ctx context.Context, job *DispatchJobRead) error {
	filter := bson.M{"_id": job.ID}
	opts := options.Replace().SetUpsert(true)
	_, err := r.collection.ReplaceOne(ctx, filter, job, opts)
	return err
}

// Delete removes a dispatch job read projection
func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// Stats represents statistics for dispatch jobs
type Stats struct {
	Total      int64 `json:"total"`
	Pending    int64 `json:"pending"`
	Queued     int64 `json:"queued"`
	InProgress int64 `json:"inProgress"`
	Completed  int64 `json:"completed"`
	Error      int64 `json:"error"`
	Cancelled  int64 `json:"cancelled"`
}

// GetStatsBySubscription returns statistics for a subscription
func (r *Repository) GetStatsBySubscription(ctx context.Context, subscriptionID string) (*Stats, error) {
	baseFilter := bson.M{"subscriptionId": subscriptionID}

	total, err := r.collection.CountDocuments(ctx, baseFilter)
	if err != nil {
		return nil, err
	}

	pending, _ := r.collection.CountDocuments(ctx, bson.M{"subscriptionId": subscriptionID, "status": DispatchStatusPending})
	queued, _ := r.collection.CountDocuments(ctx, bson.M{"subscriptionId": subscriptionID, "status": DispatchStatusQueued})
	inProgress, _ := r.collection.CountDocuments(ctx, bson.M{"subscriptionId": subscriptionID, "status": DispatchStatusInProgress})
	completed, _ := r.collection.CountDocuments(ctx, bson.M{"subscriptionId": subscriptionID, "status": DispatchStatusCompleted})
	errorCount, _ := r.collection.CountDocuments(ctx, bson.M{"subscriptionId": subscriptionID, "status": DispatchStatusError})
	cancelled, _ := r.collection.CountDocuments(ctx, bson.M{"subscriptionId": subscriptionID, "status": DispatchStatusCancelled})

	return &Stats{
		Total:      total,
		Pending:    pending,
		Queued:     queued,
		InProgress: inProgress,
		Completed:  completed,
		Error:      errorCount,
		Cancelled:  cancelled,
	}, nil
}
