// Package stream provides MongoDB change stream processing
package stream

import (
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// EventProjectionMapper maps event documents to event read projections
type EventProjectionMapper struct{}

// NewEventProjectionMapper creates a new event projection mapper
func NewEventProjectionMapper() *EventProjectionMapper {
	return &EventProjectionMapper{}
}

// Map maps an event document to a read projection
// This matches Java's EventProjectionMapper including type parsing for cascading filters
func (m *EventProjectionMapper) Map(doc bson.M) bson.M {
	if doc == nil {
		return nil
	}

	projection := bson.M{}

	// Use eventId as _id for automatic unique index and idempotency
	if id, ok := doc["_id"]; ok {
		projection["_id"] = id
		projection["eventId"] = id
	}

	// CloudEvents core fields
	copyField(doc, projection, "specVersion")
	copyField(doc, projection, "source")
	copyField(doc, projection, "subject")
	copyField(doc, projection, "time")
	copyField(doc, projection, "data")

	// Copy type and parse into denormalized filter fields: {app}:{subdomain}:{aggregate}:{event}
	// This enables cascading compound index queries (app only, app+subdomain, etc.)
	if eventType, ok := doc["type"].(string); ok {
		projection["type"] = eventType

		segments := strings.SplitN(eventType, ":", 4)
		if len(segments) > 0 {
			projection["application"] = segments[0]
		}
		if len(segments) > 1 {
			projection["subdomain"] = segments[1]
		}
		if len(segments) > 2 {
			projection["aggregate"] = segments[2]
		}
	}

	// Tracing and correlation
	copyField(doc, projection, "messageGroup")
	copyField(doc, projection, "correlationId")
	copyField(doc, projection, "causationId")
	copyField(doc, projection, "deduplicationId")

	// Context data for filtering
	copyField(doc, projection, "contextData")

	// Denormalize client context for efficient querying
	if contextData, ok := doc["contextData"].(bson.M); ok {
		if clientId, ok := contextData["clientId"]; ok {
			projection["clientId"] = clientId
		}
		if applicationCode, ok := contextData["applicationCode"]; ok {
			projection["applicationCode"] = applicationCode
		}
	}

	// Copy audit timestamps
	copyField(doc, projection, "createdAt")
	copyField(doc, projection, "updatedAt")

	// Add projection timestamp
	projection["projectedAt"] = primitive.NewDateTimeFromTime(time.Now())

	return projection
}

// DispatchJobProjectionMapper maps dispatch job documents to read projections
type DispatchJobProjectionMapper struct{}

// NewDispatchJobProjectionMapper creates a new dispatch job projection mapper
func NewDispatchJobProjectionMapper() *DispatchJobProjectionMapper {
	return &DispatchJobProjectionMapper{}
}

// Map maps a dispatch job document to a read projection
func (m *DispatchJobProjectionMapper) Map(doc bson.M) bson.M {
	if doc == nil {
		return nil
	}

	projection := bson.M{}

	// Copy ID
	if id, ok := doc["_id"]; ok {
		projection["_id"] = id
	}

	// Copy basic fields
	copyField(doc, projection, "eventId")
	copyField(doc, projection, "eventType")
	copyField(doc, projection, "subscriptionId")
	copyField(doc, projection, "dispatchPoolId")
	copyField(doc, projection, "status")
	copyField(doc, projection, "targetUrl")
	copyField(doc, projection, "payload")
	copyField(doc, projection, "contentType")
	copyField(doc, projection, "messageGroup")

	// Copy scheduling fields
	copyField(doc, projection, "scheduledFor")
	copyField(doc, projection, "startedAt")
	copyField(doc, projection, "completedAt")

	// Copy retry configuration
	copyField(doc, projection, "maxRetries")
	copyField(doc, projection, "attemptCount")
	copyField(doc, projection, "timeoutSeconds")

	// Copy metadata
	if metadata, ok := doc["metadata"].(bson.M); ok {
		projMetadata := bson.M{}
		copyField(metadata, projMetadata, "clientId")
		copyField(metadata, projMetadata, "applicationCode")
		copyField(metadata, projMetadata, "correlationId")
		copyField(metadata, projMetadata, "traceId")
		projection["metadata"] = projMetadata

		// Denormalize for efficient querying
		if clientId, ok := metadata["clientId"]; ok {
			projection["clientId"] = clientId
		}
		if applicationCode, ok := metadata["applicationCode"]; ok {
			projection["applicationCode"] = applicationCode
		}
	}

	// Copy attempts array for detailed history
	if attempts, ok := doc["attempts"].(primitive.A); ok {
		projAttempts := make([]bson.M, 0, len(attempts))
		for _, attempt := range attempts {
			if attemptDoc, ok := attempt.(bson.M); ok {
				projAttempt := bson.M{}
				copyField(attemptDoc, projAttempt, "attemptNumber")
				copyField(attemptDoc, projAttempt, "startedAt")
				copyField(attemptDoc, projAttempt, "completedAt")
				copyField(attemptDoc, projAttempt, "status")
				copyField(attemptDoc, projAttempt, "statusCode")
				copyField(attemptDoc, projAttempt, "errorMessage")
				copyField(attemptDoc, projAttempt, "durationMs")
				projAttempts = append(projAttempts, projAttempt)
			}
		}
		projection["attempts"] = projAttempts
	}

	// Copy last attempt summary
	copyField(doc, projection, "lastAttemptAt")
	copyField(doc, projection, "lastStatusCode")
	copyField(doc, projection, "lastErrorMessage")

	// Copy audit timestamps
	copyField(doc, projection, "createdAt")
	copyField(doc, projection, "updatedAt")

	// Add projection timestamp
	projection["projectedAt"] = primitive.NewDateTimeFromTime(time.Now())

	return projection
}

// copyField copies a field from source to destination if it exists
func copyField(src, dst bson.M, field string) {
	if val, ok := src[field]; ok {
		dst[field] = val
	}
}
