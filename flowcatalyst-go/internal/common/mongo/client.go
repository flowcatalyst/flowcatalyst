package mongo

import (
	"context"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"go.flowcatalyst.tech/internal/config"
)

// Client wraps the MongoDB client with helper methods
type Client struct {
	client   *mongo.Client
	database *mongo.Database
	dbName   string
}

// Connect establishes a connection to MongoDB
func Connect(ctx context.Context, cfg config.MongoDBConfig) (*Client, error) {
	clientOpts := options.Client().
		ApplyURI(cfg.URI).
		SetMaxPoolSize(100).
		SetMinPoolSize(10).
		SetMaxConnIdleTime(5 * time.Minute).
		SetServerSelectionTimeout(5 * time.Second).
		SetConnectTimeout(10 * time.Second)

	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return nil, err
	}

	// Verify connection
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		return nil, err
	}

	slog.Info("Connected to MongoDB", "database", cfg.Database)

	return &Client{
		client:   client,
		database: client.Database(cfg.Database),
		dbName:   cfg.Database,
	}, nil
}

// Database returns the default database
func (c *Client) Database() *mongo.Database {
	return c.database
}

// Collection returns a collection from the default database
func (c *Client) Collection(name string) *mongo.Collection {
	return c.database.Collection(name)
}

// Ping checks if the connection is alive
func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx, readpref.Primary())
}

// Disconnect closes the MongoDB connection
func (c *Client) Disconnect(ctx context.Context) error {
	return c.client.Disconnect(ctx)
}

// WithTransaction executes a function within a MongoDB transaction
// This implements the Unit of Work pattern for ACID guarantees
func (c *Client) WithTransaction(ctx context.Context, fn func(sessCtx mongo.SessionContext) error) error {
	session, err := c.client.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (interface{}, error) {
		return nil, fn(sessCtx)
	})

	return err
}

// UnitOfWork represents a transactional unit of work
type UnitOfWork struct {
	client  *Client
	session mongo.Session
	ctx     mongo.SessionContext
}

// NewUnitOfWork creates a new unit of work
func (c *Client) NewUnitOfWork(ctx context.Context) (*UnitOfWork, error) {
	session, err := c.client.StartSession()
	if err != nil {
		return nil, err
	}

	return &UnitOfWork{
		client:  c,
		session: session,
	}, nil
}

// Begin starts the transaction
func (uow *UnitOfWork) Begin(ctx context.Context) error {
	return uow.session.StartTransaction()
}

// Commit commits the transaction
func (uow *UnitOfWork) Commit(ctx context.Context) error {
	return uow.session.CommitTransaction(ctx)
}

// Rollback aborts the transaction
func (uow *UnitOfWork) Rollback(ctx context.Context) error {
	return uow.session.AbortTransaction(ctx)
}

// End ends the session
func (uow *UnitOfWork) End(ctx context.Context) {
	uow.session.EndSession(ctx)
}

// Collection returns a collection for use within the unit of work
func (uow *UnitOfWork) Collection(name string) *mongo.Collection {
	return uow.client.Collection(name)
}

// ExecuteInTransaction is a helper that runs a function in a transaction
// automatically handling commit/rollback
func (uow *UnitOfWork) ExecuteInTransaction(ctx context.Context, fn func(sessCtx mongo.SessionContext) error) error {
	defer uow.End(ctx)

	_, err := uow.session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (interface{}, error) {
		return nil, fn(sessCtx)
	})

	return err
}
