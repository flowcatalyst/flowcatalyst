package permission

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.flowcatalyst.tech/internal/common/tsid"
)

// mongoRepository provides MongoDB access to permission data
type mongoRepository struct {
	collection *mongo.Collection
}

// NewRepository creates a new permission repository with instrumentation
func NewRepository(db *mongo.Database) Repository {
	return newInstrumentedRepository(&mongoRepository{
		collection: db.Collection("auth_permissions"),
	})
}

// FindAll finds all permissions
func (r *mongoRepository) FindAll(ctx context.Context) ([]*Permission, error) {
	cursor, err := r.collection.Find(ctx, bson.M{}, options.Find().SetSort(bson.M{"code": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var permissions []*Permission
	if err := cursor.All(ctx, &permissions); err != nil {
		return nil, err
	}
	return permissions, nil
}

// FindByID finds a permission by ID
func (r *mongoRepository) FindByID(ctx context.Context, id string) (*Permission, error) {
	var perm Permission
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&perm)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &perm, nil
}

// FindByCode finds a permission by code
func (r *mongoRepository) FindByCode(ctx context.Context, code string) (*Permission, error) {
	var perm Permission
	err := r.collection.FindOne(ctx, bson.M{"code": code}).Decode(&perm)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &perm, nil
}

// Insert inserts a new permission
func (r *mongoRepository) Insert(ctx context.Context, perm *Permission) error {
	perm.ID = tsid.Generate()
	perm.CreatedAt = time.Now()
	perm.UpdatedAt = time.Now()
	_, err := r.collection.InsertOne(ctx, perm)
	return err
}
