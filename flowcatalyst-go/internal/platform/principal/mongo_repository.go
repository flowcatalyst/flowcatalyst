package principal

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.flowcatalyst.tech/internal/common/tsid"
)

const collectionName = "auth_principals"

var (
	ErrNotFound       = errors.New("principal not found")
	ErrDuplicateEmail = errors.New("email already exists")
)

// mongoRepository provides MongoDB access to principal data
type mongoRepository struct {
	collection *mongo.Collection
}

// NewRepository creates a new principal repository with instrumentation
func NewRepository(db *mongo.Database) Repository {
	return newInstrumentedRepository(&mongoRepository{
		collection: db.Collection(collectionName),
	})
}

// FindByID finds a principal by ID
func (r *mongoRepository) FindByID(ctx context.Context, id string) (*Principal, error) {
	var principal Principal
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&principal)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &principal, nil
}

// FindByEmail finds a principal by email address
func (r *mongoRepository) FindByEmail(ctx context.Context, email string) (*Principal, error) {
	var principal Principal
	err := r.collection.FindOne(ctx, bson.M{"userIdentity.email": email}).Decode(&principal)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &principal, nil
}

// FindByClientID finds all principals for a client with pagination
func (r *mongoRepository) FindByClientID(ctx context.Context, clientID string, skip, limit int64) ([]*Principal, error) {
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	cursor, err := r.collection.Find(ctx, bson.M{"clientId": clientID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var principals []*Principal
	if err := cursor.All(ctx, &principals); err != nil {
		return nil, err
	}
	return principals, nil
}

// FindByType finds all principals of a specific type with pagination
func (r *mongoRepository) FindByType(ctx context.Context, principalType PrincipalType, skip, limit int64) ([]*Principal, error) {
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	cursor, err := r.collection.Find(ctx, bson.M{"type": principalType}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var principals []*Principal
	if err := cursor.All(ctx, &principals); err != nil {
		return nil, err
	}
	return principals, nil
}

// FindActive finds all active principals with pagination
func (r *mongoRepository) FindActive(ctx context.Context, skip, limit int64) ([]*Principal, error) {
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	cursor, err := r.collection.Find(ctx, bson.M{"active": true}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var principals []*Principal
	if err := cursor.All(ctx, &principals); err != nil {
		return nil, err
	}
	return principals, nil
}

// FindAll returns all principals with optional pagination
func (r *mongoRepository) FindAll(ctx context.Context, skip, limit int64) ([]*Principal, error) {
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	cursor, err := r.collection.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var principals []*Principal
	if err := cursor.All(ctx, &principals); err != nil {
		return nil, err
	}
	return principals, nil
}

// Insert creates a new principal
func (r *mongoRepository) Insert(ctx context.Context, principal *Principal) error {
	if principal.ID == "" {
		principal.ID = tsid.Generate()
	}
	now := time.Now()
	principal.CreatedAt = now
	principal.UpdatedAt = now

	_, err := r.collection.InsertOne(ctx, principal)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicateEmail
	}
	return err
}

// Update updates an existing principal
func (r *mongoRepository) Update(ctx context.Context, principal *Principal) error {
	principal.UpdatedAt = time.Now()

	result, err := r.collection.ReplaceOne(ctx, bson.M{"_id": principal.ID}, principal)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateRoles updates the roles for a principal
func (r *mongoRepository) UpdateRoles(ctx context.Context, id string, roles []RoleAssignment) error {
	result, err := r.collection.UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{
			"$set": bson.M{
				"roles":     roles,
				"updatedAt": time.Now(),
			},
		},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateLastLogin updates the last login timestamp
func (r *mongoRepository) UpdateLastLogin(ctx context.Context, id string) error {
	_, err := r.collection.UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{
			"$set": bson.M{
				"userIdentity.lastLoginAt": time.Now(),
				"updatedAt":                time.Now(),
			},
		},
	)
	return err
}

// SetActive activates or deactivates a principal
func (r *mongoRepository) SetActive(ctx context.Context, id string, active bool) error {
	result, err := r.collection.UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{
			"$set": bson.M{
				"active":    active,
				"updatedAt": time.Now(),
			},
		},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a principal
func (r *mongoRepository) Delete(ctx context.Context, id string) error {
	result, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// Count returns the total number of principals
func (r *mongoRepository) Count(ctx context.Context) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{})
}

// CountByType returns the count of principals by type
func (r *mongoRepository) CountByType(ctx context.Context, principalType PrincipalType) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{"type": principalType})
}

// ExistsByEmail checks if a principal with the given email exists
func (r *mongoRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	count, err := r.collection.CountDocuments(ctx, bson.M{"userIdentity.email": email})
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
