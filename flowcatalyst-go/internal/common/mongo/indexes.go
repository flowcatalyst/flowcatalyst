package mongo

import (
	"context"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// IndexDefinition defines a MongoDB index
type IndexDefinition struct {
	Collection string
	Keys       bson.D
	Options    *options.IndexOptions
}

// IndexInitializer creates indexes on startup
type IndexInitializer struct {
	client *Client
}

// NewIndexInitializer creates a new index initializer
func NewIndexInitializer(client *Client) *IndexInitializer {
	return &IndexInitializer{client: client}
}

// Initialize creates all required indexes
func (i *IndexInitializer) Initialize(ctx context.Context) error {
	indexes := i.getIndexDefinitions()

	for _, idx := range indexes {
		if err := i.createIndex(ctx, idx); err != nil {
			slog.Warn("Failed to create index (may already exist)",
				"error", err,
				"collection", idx.Collection)
		}
	}

	slog.Info("Index initialization complete", "count", len(indexes))
	return nil
}

func (i *IndexInitializer) createIndex(ctx context.Context, idx IndexDefinition) error {
	collection := i.client.Collection(idx.Collection)

	indexModel := mongo.IndexModel{
		Keys:    idx.Keys,
		Options: idx.Options,
	}

	_, err := collection.Indexes().CreateOne(ctx, indexModel)
	return err
}

func (i *IndexInitializer) getIndexDefinitions() []IndexDefinition {
	return []IndexDefinition{
		// auth_principals
		{
			Collection: "auth_principals",
			Keys:       bson.D{{Key: "clientId", Value: 1}},
		},
		{
			Collection: "auth_principals",
			Keys:       bson.D{{Key: "type", Value: 1}},
		},
		{
			Collection: "auth_principals",
			Keys:       bson.D{{Key: "userIdentity.email", Value: 1}},
			Options:    options.Index().SetUnique(true).SetSparse(true),
		},

		// auth_clients
		{
			Collection: "auth_clients",
			Keys:       bson.D{{Key: "identifier", Value: 1}},
			Options:    options.Index().SetUnique(true),
		},
		{
			Collection: "auth_clients",
			Keys:       bson.D{{Key: "status", Value: 1}},
		},

		// client_access_grants
		{
			Collection: "client_access_grants",
			Keys:       bson.D{{Key: "principalId", Value: 1}},
		},
		{
			Collection: "client_access_grants",
			Keys:       bson.D{{Key: "clientId", Value: 1}},
		},
		{
			Collection: "client_access_grants",
			Keys:       bson.D{{Key: "principalId", Value: 1}, {Key: "clientId", Value: 1}},
			Options:    options.Index().SetUnique(true),
		},

		// client_auth_config
		{
			Collection: "client_auth_config",
			Keys:       bson.D{{Key: "emailDomain", Value: 1}},
			Options:    options.Index().SetUnique(true),
		},

		// auth_applications
		{
			Collection: "auth_applications",
			Keys:       bson.D{{Key: "code", Value: 1}},
			Options:    options.Index().SetUnique(true),
		},
		{
			Collection: "auth_applications",
			Keys:       bson.D{{Key: "active", Value: 1}},
		},

		// application_client_config
		{
			Collection: "application_client_config",
			Keys:       bson.D{{Key: "applicationId", Value: 1}},
		},
		{
			Collection: "application_client_config",
			Keys:       bson.D{{Key: "clientId", Value: 1}},
		},
		{
			Collection: "application_client_config",
			Keys:       bson.D{{Key: "applicationId", Value: 1}, {Key: "clientId", Value: 1}},
			Options:    options.Index().SetUnique(true),
		},

		// auth_roles
		{
			Collection: "auth_roles",
			Keys:       bson.D{{Key: "name", Value: 1}},
			Options:    options.Index().SetUnique(true),
		},
		{
			Collection: "auth_roles",
			Keys:       bson.D{{Key: "applicationId", Value: 1}},
		},
		{
			Collection: "auth_roles",
			Keys:       bson.D{{Key: "source", Value: 1}},
		},

		// auth_permissions
		{
			Collection: "auth_permissions",
			Keys:       bson.D{{Key: "name", Value: 1}},
			Options:    options.Index().SetUnique(true),
		},
		{
			Collection: "auth_permissions",
			Keys:       bson.D{{Key: "applicationId", Value: 1}},
		},

		// idp_role_mappings
		{
			Collection: "idp_role_mappings",
			Keys:       bson.D{{Key: "idpRoleName", Value: 1}},
			Options:    options.Index().SetUnique(true),
		},
		{
			Collection: "idp_role_mappings",
			Keys:       bson.D{{Key: "internalRoleName", Value: 1}},
		},

		// oauth_clients
		{
			Collection: "oauth_clients",
			Keys:       bson.D{{Key: "clientId", Value: 1}},
			Options:    options.Index().SetUnique(true),
		},

		// authorization_codes (TTL index)
		{
			Collection: "authorization_codes",
			Keys:       bson.D{{Key: "code", Value: 1}},
			Options:    options.Index().SetUnique(true),
		},
		{
			Collection: "authorization_codes",
			Keys:       bson.D{{Key: "expiresAt", Value: 1}},
			Options:    options.Index().SetExpireAfterSeconds(0),
		},

		// refresh_tokens
		{
			Collection: "refresh_tokens",
			Keys:       bson.D{{Key: "tokenHash", Value: 1}},
			Options:    options.Index().SetUnique(true),
		},
		{
			Collection: "refresh_tokens",
			Keys:       bson.D{{Key: "principalId", Value: 1}},
		},
		{
			Collection: "refresh_tokens",
			Keys:       bson.D{{Key: "tokenFamily", Value: 1}},
		},
		{
			Collection: "refresh_tokens",
			Keys:       bson.D{{Key: "expiresAt", Value: 1}},
		},

		// events
		{
			Collection: "events",
			Keys:       bson.D{{Key: "deduplicationId", Value: 1}},
			Options:    options.Index().SetUnique(true).SetSparse(true),
		},
		{
			Collection: "events",
			Keys:       bson.D{{Key: "time", Value: 1}},
			Options:    options.Index().SetExpireAfterSeconds(int32(30 * 24 * time.Hour / time.Second)),
		},

		// dispatch_jobs
		{
			Collection: "dispatch_jobs",
			Keys:       bson.D{{Key: "idempotencyKey", Value: 1}},
			Options:    options.Index().SetUnique(true).SetSparse(true),
		},
		{
			Collection: "dispatch_jobs",
			Keys:       bson.D{{Key: "status", Value: 1}, {Key: "scheduledFor", Value: 1}, {Key: "clientId", Value: 1}},
		},
		{
			Collection: "dispatch_jobs",
			Keys:       bson.D{{Key: "clientId", Value: 1}, {Key: "messageGroup", Value: 1}, {Key: "status", Value: 1}},
			Options:    options.Index().SetSparse(true),
		},
		{
			Collection: "dispatch_jobs",
			Keys:       bson.D{{Key: "createdAt", Value: 1}},
			Options:    options.Index().SetExpireAfterSeconds(int32(30 * 24 * time.Hour / time.Second)),
		},

		// event_types
		{
			Collection: "event_types",
			Keys:       bson.D{{Key: "code", Value: 1}},
			Options:    options.Index().SetUnique(true),
		},
		{
			Collection: "event_types",
			Keys:       bson.D{{Key: "status", Value: 1}},
		},

		// subscriptions
		{
			Collection: "subscriptions",
			Keys:       bson.D{{Key: "code", Value: 1}},
			Options:    options.Index().SetUnique(true),
		},
		{
			Collection: "subscriptions",
			Keys:       bson.D{{Key: "clientId", Value: 1}},
		},
		{
			Collection: "subscriptions",
			Keys:       bson.D{{Key: "status", Value: 1}},
		},
		{
			Collection: "subscriptions",
			Keys:       bson.D{{Key: "dispatchPoolId", Value: 1}},
		},

		// dispatch_pools
		{
			Collection: "dispatch_pools",
			Keys:       bson.D{{Key: "code", Value: 1}},
			Options:    options.Index().SetUnique(true),
		},
		{
			Collection: "dispatch_pools",
			Keys:       bson.D{{Key: "status", Value: 1}},
		},

		// audit_logs
		{
			Collection: "audit_logs",
			Keys:       bson.D{{Key: "entityType", Value: 1}, {Key: "entityId", Value: 1}},
		},
		{
			Collection: "audit_logs",
			Keys:       bson.D{{Key: "principalId", Value: 1}},
		},
		{
			Collection: "audit_logs",
			Keys:       bson.D{{Key: "performedAt", Value: -1}},
		},

		// anchor_domains
		{
			Collection: "anchor_domains",
			Keys:       bson.D{{Key: "domain", Value: 1}},
			Options:    options.Index().SetUnique(true),
		},

		// oidc_login_state (TTL index)
		{
			Collection: "oidc_login_state",
			Keys:       bson.D{{Key: "expiresAt", Value: 1}},
			Options:    options.Index().SetExpireAfterSeconds(0),
		},

		// service_accounts
		{
			Collection: "service_accounts",
			Keys:       bson.D{{Key: "code", Value: 1}},
			Options:    options.Index().SetUnique(true),
		},
	}
}
