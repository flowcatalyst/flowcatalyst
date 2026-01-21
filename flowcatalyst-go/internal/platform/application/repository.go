package application

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.flowcatalyst.tech/internal/common/tsid"
)

// Repository handles application persistence
type Repository struct {
	collection       *mongo.Collection
	configCollection *mongo.Collection
}

// NewRepository creates a new application repository
func NewRepository(db *mongo.Database) *Repository {
	return &Repository{
		collection:       db.Collection("auth_applications"),
		configCollection: db.Collection("application_client_config"),
	}
}

// FindAll finds all applications
func (r *Repository) FindAll(ctx context.Context) ([]*Application, error) {
	cursor, err := r.collection.Find(ctx, bson.M{}, options.Find().SetSort(bson.M{"code": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var apps []*Application
	if err := cursor.All(ctx, &apps); err != nil {
		return nil, err
	}
	return apps, nil
}

// FindByID finds an application by ID
func (r *Repository) FindByID(ctx context.Context, id string) (*Application, error) {
	var app Application
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&app)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &app, nil
}

// FindByCode finds an application by code
func (r *Repository) FindByCode(ctx context.Context, code string) (*Application, error) {
	var app Application
	err := r.collection.FindOne(ctx, bson.M{"code": code}).Decode(&app)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &app, nil
}

// Insert inserts a new application
func (r *Repository) Insert(ctx context.Context, app *Application) error {
	app.ID = tsid.Generate()
	app.CreatedAt = time.Now()
	app.UpdatedAt = time.Now()
	_, err := r.collection.InsertOne(ctx, app)
	return err
}

// Update updates an application
func (r *Repository) Update(ctx context.Context, app *Application) error {
	app.UpdatedAt = time.Now()
	_, err := r.collection.UpdateByID(ctx, app.ID, bson.M{"$set": app})
	return err
}

// Delete deletes an application
func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// FindClientConfig finds client configuration for an application and client
func (r *Repository) FindClientConfig(ctx context.Context, applicationID, clientID string) (*ApplicationClientConfig, error) {
	var config ApplicationClientConfig
	err := r.configCollection.FindOne(ctx, bson.M{
		"applicationId": applicationID,
		"clientId":      clientID,
	}).Decode(&config)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &config, nil
}

// InsertClientConfig inserts a new client configuration
func (r *Repository) InsertClientConfig(ctx context.Context, config *ApplicationClientConfig) error {
	config.ID = tsid.Generate()
	config.CreatedAt = time.Now()
	config.UpdatedAt = time.Now()
	_, err := r.configCollection.InsertOne(ctx, config)
	return err
}
