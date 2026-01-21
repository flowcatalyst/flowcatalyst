package serviceaccount

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.flowcatalyst.tech/internal/common/tsid"
)

// Repository handles service account persistence
type Repository struct {
	collection *mongo.Collection
}

// NewRepository creates a new service account repository
func NewRepository(db *mongo.Database) *Repository {
	return &Repository{
		collection: db.Collection("service_accounts"),
	}
}

// FindAll finds all service accounts
func (r *Repository) FindAll(ctx context.Context) ([]*ServiceAccount, error) {
	cursor, err := r.collection.Find(ctx, bson.M{}, options.Find().SetSort(bson.M{"name": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var accounts []*ServiceAccount
	if err := cursor.All(ctx, &accounts); err != nil {
		return nil, err
	}
	return accounts, nil
}

// FindByID finds a service account by ID
func (r *Repository) FindByID(ctx context.Context, id string) (*ServiceAccount, error) {
	var account ServiceAccount
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&account)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &account, nil
}

// FindByCode finds a service account by code
func (r *Repository) FindByCode(ctx context.Context, code string) (*ServiceAccount, error) {
	var account ServiceAccount
	err := r.collection.FindOne(ctx, bson.M{"code": code}).Decode(&account)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &account, nil
}

// FindByCredentialID finds a service account by credential ID
func (r *Repository) FindByCredentialID(ctx context.Context, credentialID string) (*ServiceAccount, error) {
	var account ServiceAccount
	err := r.collection.FindOne(ctx, bson.M{"credentialId": credentialID}).Decode(&account)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &account, nil
}

// Insert inserts a new service account
func (r *Repository) Insert(ctx context.Context, account *ServiceAccount) error {
	account.ID = tsid.Generate()
	account.CreatedAt = time.Now()
	account.UpdatedAt = time.Now()
	_, err := r.collection.InsertOne(ctx, account)
	return err
}

// Update updates a service account
func (r *Repository) Update(ctx context.Context, account *ServiceAccount) error {
	account.UpdatedAt = time.Now()
	_, err := r.collection.UpdateByID(ctx, account.ID, bson.M{"$set": account})
	return err
}

// Delete deletes a service account
func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	return err
}
