package common

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.flowcatalyst.tech/internal/common/tsid"
)

// MongoUnitOfWork implements UnitOfWork using MongoDB transactions.
// It ensures that aggregate persistence, domain event creation, and
// audit logging all happen atomically within a single transaction.
type MongoUnitOfWork struct {
	client *mongo.Client
	db     *mongo.Database
}

// NewMongoUnitOfWork creates a new MongoDB-backed UnitOfWork.
func NewMongoUnitOfWork(client *mongo.Client, db *mongo.Database) *MongoUnitOfWork {
	return &MongoUnitOfWork{
		client: client,
		db:     db,
	}
}

// Commit persists an aggregate with its domain event atomically.
func (uow *MongoUnitOfWork) Commit(
	ctx context.Context,
	aggregate any,
	event DomainEvent,
	command any,
) Result[DomainEvent] {
	return uow.CommitWithClientID(ctx, aggregate, event, command, "")
}

// CommitWithClientID persists an aggregate with client-scoped event.
func (uow *MongoUnitOfWork) CommitWithClientID(
	ctx context.Context,
	aggregate any,
	event DomainEvent,
	command any,
	clientID string,
) Result[DomainEvent] {
	session, err := uow.client.StartSession()
	if err != nil {
		return Failure[DomainEvent](InternalError(
			ErrCodeCommitFailed,
			"Failed to start session: "+err.Error(),
			nil,
		))
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (any, error) {
		// 1. Persist aggregate (upsert)
		if err := uow.persistAggregate(sessCtx, aggregate); err != nil {
			return nil, fmt.Errorf("persist aggregate: %w", err)
		}

		// 2. Create domain event
		if err := uow.createEvent(sessCtx, event, clientID); err != nil {
			return nil, fmt.Errorf("create event: %w", err)
		}

		// 3. Create audit log
		if err := uow.createAuditLog(sessCtx, event, command); err != nil {
			return nil, fmt.Errorf("create audit log: %w", err)
		}

		return nil, nil
	})

	if err != nil {
		return Failure[DomainEvent](BusinessRuleError(
			ErrCodeCommitFailed,
			"Transaction failed: "+err.Error(),
			nil,
		))
	}

	// ONLY HERE can we return success - via unexported constructor
	return newSuccess[DomainEvent](event)
}

// CommitDelete deletes an aggregate with its domain event atomically.
func (uow *MongoUnitOfWork) CommitDelete(
	ctx context.Context,
	aggregate any,
	event DomainEvent,
	command any,
) Result[DomainEvent] {
	session, err := uow.client.StartSession()
	if err != nil {
		return Failure[DomainEvent](InternalError(
			ErrCodeCommitFailed,
			"Failed to start session: "+err.Error(),
			nil,
		))
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (any, error) {
		// 1. Delete aggregate
		if err := uow.deleteAggregate(sessCtx, aggregate); err != nil {
			return nil, fmt.Errorf("delete aggregate: %w", err)
		}

		// 2. Create domain event
		if err := uow.createEvent(sessCtx, event, ""); err != nil {
			return nil, fmt.Errorf("create event: %w", err)
		}

		// 3. Create audit log
		if err := uow.createAuditLog(sessCtx, event, command); err != nil {
			return nil, fmt.Errorf("create audit log: %w", err)
		}

		return nil, nil
	})

	if err != nil {
		return Failure[DomainEvent](BusinessRuleError(
			ErrCodeCommitFailed,
			"Transaction failed: "+err.Error(),
			nil,
		))
	}

	return newSuccess[DomainEvent](event)
}

// CommitAll persists multiple aggregates with a domain event atomically.
func (uow *MongoUnitOfWork) CommitAll(
	ctx context.Context,
	aggregates []any,
	event DomainEvent,
	command any,
) Result[DomainEvent] {
	session, err := uow.client.StartSession()
	if err != nil {
		return Failure[DomainEvent](InternalError(
			ErrCodeCommitFailed,
			"Failed to start session: "+err.Error(),
			nil,
		))
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (any, error) {
		// 1. Persist all aggregates
		for i, aggregate := range aggregates {
			if err := uow.persistAggregate(sessCtx, aggregate); err != nil {
				return nil, fmt.Errorf("persist aggregate %d: %w", i, err)
			}
		}

		// 2. Create domain event
		if err := uow.createEvent(sessCtx, event, ""); err != nil {
			return nil, fmt.Errorf("create event: %w", err)
		}

		// 3. Create audit log
		if err := uow.createAuditLog(sessCtx, event, command); err != nil {
			return nil, fmt.Errorf("create audit log: %w", err)
		}

		return nil, nil
	})

	if err != nil {
		return Failure[DomainEvent](BusinessRuleError(
			ErrCodeCommitFailed,
			"Transaction failed: "+err.Error(),
			nil,
		))
	}

	return newSuccess[DomainEvent](event)
}

// persistAggregate upserts an aggregate to its collection.
func (uow *MongoUnitOfWork) persistAggregate(ctx mongo.SessionContext, aggregate any) error {
	collectionName := uow.getCollectionName(aggregate)
	id := uow.extractID(aggregate)

	if collectionName == "" {
		return fmt.Errorf("cannot determine collection name for aggregate type %T", aggregate)
	}
	if id == "" {
		return fmt.Errorf("aggregate has no ID field")
	}

	collection := uow.db.Collection(collectionName)

	_, err := collection.ReplaceOne(
		ctx,
		bson.M{"_id": id},
		aggregate,
		options.Replace().SetUpsert(true),
	)

	return err
}

// deleteAggregate removes an aggregate from its collection.
func (uow *MongoUnitOfWork) deleteAggregate(ctx mongo.SessionContext, aggregate any) error {
	collectionName := uow.getCollectionName(aggregate)
	id := uow.extractID(aggregate)

	if collectionName == "" {
		return fmt.Errorf("cannot determine collection name for aggregate type %T", aggregate)
	}
	if id == "" {
		return fmt.Errorf("aggregate has no ID field")
	}

	collection := uow.db.Collection(collectionName)
	_, err := collection.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// createEvent inserts the domain event into the events collection.
func (uow *MongoUnitOfWork) createEvent(ctx mongo.SessionContext, event DomainEvent, clientID string) error {
	persistedEvent := ToPersistedEvent(event, clientID)

	collection := uow.db.Collection("events")
	_, err := collection.InsertOne(ctx, persistedEvent)
	return err
}

// createAuditLog creates an audit log entry for the operation.
func (uow *MongoUnitOfWork) createAuditLog(ctx mongo.SessionContext, event DomainEvent, command any) error {
	// Serialize the command for audit trail
	var operationJSON string
	if auditable, ok := command.(Auditable); ok {
		operationJSON = auditable.ToAuditJSON()
	} else {
		bytes, err := json.Marshal(command)
		if err != nil {
			operationJSON = "{}"
		} else {
			operationJSON = string(bytes)
		}
	}

	// Extract entity type from subject (e.g., "platform.eventtype.123" -> "EventType")
	entityType := extractEntityType(event.Subject())
	entityID := extractEntityID(event.Subject())

	// Extract operation name from command type
	operation := extractOperationName(command)

	auditLog := bson.M{
		"_id":           tsid.Generate(),
		"entityType":    entityType,
		"entityId":      entityID,
		"operation":     operation,
		"operationJson": operationJSON,
		"principalId":   event.PrincipalID(),
		"performedAt":   event.Time(),
	}

	collection := uow.db.Collection("audit_logs")
	_, err := collection.InsertOne(ctx, auditLog)
	return err
}

// getCollectionName determines the MongoDB collection for an aggregate.
func (uow *MongoUnitOfWork) getCollectionName(aggregate any) string {
	// Check if aggregate implements AggregateRoot
	if ar, ok := aggregate.(AggregateRoot); ok {
		return ar.CollectionName()
	}

	// Fall back to convention-based naming from type name
	t := reflect.TypeOf(aggregate)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	typeName := t.Name()

	// Map common type names to collection names
	collectionMap := map[string]string{
		"EventType":     "event_types",
		"Subscription":  "subscriptions",
		"DispatchPool":  "dispatch_pools",
		"DispatchJob":   "dispatch_jobs",
		"Event":         "events",
		"Principal":     "auth_principals",
		"Client":        "auth_clients",
		"Application":   "auth_applications",
		"Role":          "auth_roles",
		"Permission":    "auth_permissions",
		"ServiceAccount": "service_accounts",
		"OAuthClient":   "oauth_clients",
		"AnchorDomain":  "anchor_domains",
		"ClientAuthConfig": "client_auth_configs",
		"IdpRoleMapping": "idp_role_mappings",
	}

	if collection, ok := collectionMap[typeName]; ok {
		return collection
	}

	// Default: convert PascalCase to snake_case and pluralize
	return toSnakeCase(typeName) + "s"
}

// extractID gets the ID from an aggregate using reflection.
func (uow *MongoUnitOfWork) extractID(aggregate any) string {
	// Check if aggregate implements AggregateRoot
	if ar, ok := aggregate.(AggregateRoot); ok {
		return ar.AggregateID()
	}

	v := reflect.ValueOf(aggregate)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return ""
	}

	// Try common ID field names
	for _, fieldName := range []string{"ID", "Id", "id"} {
		field := v.FieldByName(fieldName)
		if field.IsValid() && field.Kind() == reflect.String {
			return field.String()
		}
	}

	return ""
}

// extractEntityType extracts entity type from subject.
// Subject format: {domain}.{aggregate}.{id}
// Returns the aggregate name in PascalCase.
func extractEntityType(subject string) string {
	parts := strings.Split(subject, ".")
	if len(parts) >= 2 {
		// Convert to PascalCase: "eventtype" -> "EventType"
		return toPascalCase(parts[1])
	}
	return "Unknown"
}

// extractEntityID extracts entity ID from subject.
// Subject format: {domain}.{aggregate}.{id}
func extractEntityID(subject string) string {
	parts := strings.Split(subject, ".")
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}

// extractOperationName extracts operation name from command type.
// Returns the full type name including "Command" suffix to match Java behavior.
func extractOperationName(command any) string {
	t := reflect.TypeOf(command)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

// toSnakeCase converts PascalCase to snake_case.
func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

// toPascalCase converts a lowercase string to PascalCase.
func toPascalCase(s string) string {
	if s == "" {
		return ""
	}
	// Capitalize first letter
	return strings.ToUpper(s[:1]) + s[1:]
}
