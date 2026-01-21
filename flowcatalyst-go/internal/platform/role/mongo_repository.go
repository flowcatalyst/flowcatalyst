package role

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.flowcatalyst.tech/internal/common/tsid"
)

// Role represents an authentication role
type Role struct {
	ID          string    `bson:"_id" json:"id"`
	Code        string    `bson:"code" json:"code"`
	Name        string    `bson:"name" json:"name"`
	Description string    `bson:"description,omitempty" json:"description,omitempty"`
	Scope       string    `bson:"scope" json:"scope"` // ANCHOR, PARTNER, CLIENT
	Permissions []string  `bson:"permissions" json:"permissions"`
	BuiltIn     bool      `bson:"builtIn" json:"builtIn"`
	CreatedAt   time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time `bson:"updatedAt" json:"updatedAt"`
}

// mongoRepository provides MongoDB access to role data
type mongoRepository struct {
	collection *mongo.Collection
}

// NewRepository creates a new role repository with instrumentation
func NewRepository(db *mongo.Database) Repository {
	return newInstrumentedRepository(&mongoRepository{
		collection: db.Collection("auth_roles"),
	})
}

// FindAll finds all roles
func (r *mongoRepository) FindAll(ctx context.Context) ([]*Role, error) {
	cursor, err := r.collection.Find(ctx, bson.M{}, options.Find().SetSort(bson.M{"code": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var roles []*Role
	if err := cursor.All(ctx, &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

// FindByID finds a role by ID
func (r *mongoRepository) FindByID(ctx context.Context, id string) (*Role, error) {
	var role Role
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&role)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &role, nil
}

// FindByCode finds a role by code
func (r *mongoRepository) FindByCode(ctx context.Context, code string) (*Role, error) {
	var role Role
	err := r.collection.FindOne(ctx, bson.M{"code": code}).Decode(&role)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &role, nil
}

// Insert inserts a new role
func (r *mongoRepository) Insert(ctx context.Context, role *Role) error {
	role.ID = tsid.Generate()
	role.CreatedAt = time.Now()
	role.UpdatedAt = time.Now()
	_, err := r.collection.InsertOne(ctx, role)
	return err
}

// Update updates a role
func (r *mongoRepository) Update(ctx context.Context, role *Role) error {
	role.UpdatedAt = time.Now()
	_, err := r.collection.UpdateByID(ctx, role.ID, bson.M{"$set": role})
	return err
}

// Delete deletes a role
func (r *mongoRepository) Delete(ctx context.Context, id string) error {
	_, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	return err
}
