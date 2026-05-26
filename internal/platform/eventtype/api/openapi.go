package api

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/openapi"
)

// OpenAPI registers the event-type subdomain's OpenAPI ops on doc.
// Called by WirePlatform alongside RegisterRoutes. The two functions
// are paired by convention — when adding a new route, add it here too
// or the spec drifts. New aggregates: prefer the fused
// `Mount(r, doc, state)` pattern once that helper lands (HANDOFF #26).
//
// Schema names use the Go type names (CreateCommand, UpdateCommand,
// EventType) so cross-references read naturally in Swagger UI.
func OpenAPI(doc *openapi.Doc) {
	const tag = "event-types"
	listResp := map[string]any{"items": []eventtype.EventType{}}
	errResp := map[string]string{"code": "", "message": ""}

	doc.Op("GET", "/api/event-types", "listEventTypes", "List event types",
		openapi.Tag(tag),
		openapi.QueryParam("application", "filter by application code", ""),
		openapi.QueryParam("clientId", "filter by client id", ""),
		openapi.QueryParam("status", "filter by status (CURRENT, ARCHIVED)", ""),
		openapi.QueryParam("subdomain", "filter by subdomain", ""),
		openapi.QueryParam("aggregate", "filter by aggregate", ""),
		openapi.Response(200, "Event types matching filters", "EventTypeList", listResp),
		openapi.Response(403, "Forbidden", "ErrorEnvelope", errResp),
	)

	doc.Op("POST", "/api/event-types", "createEventType", "Create an event type",
		openapi.Tag(tag),
		openapi.RequestBody("CreateEventTypeCommand", "Event type to create", &operations.CreateCommand{}),
		openapi.Response(201, "Event type created", "EventType", &eventtype.EventType{}),
		openapi.Response(403, "Forbidden", "ErrorEnvelope", errResp),
		openapi.Response(422, "Validation error", "ErrorEnvelope", errResp),
	)

	doc.Op("GET", "/api/event-types/{id}", "getEventType", "Get an event type by id",
		openapi.Tag(tag),
		openapi.PathParam("id", "Event type id (TSID)"),
		openapi.Response(200, "Event type", "EventType", &eventtype.EventType{}),
		openapi.Response(404, "Not found", "ErrorEnvelope", errResp),
	)

	doc.Op("GET", "/api/event-types/by-code/{code}", "getEventTypeByCode", "Get an event type by code",
		openapi.Tag(tag),
		openapi.PathParam("code", "Event type code (e.g. platform:iam:user:created)"),
		openapi.Response(200, "Event type", "EventType", &eventtype.EventType{}),
		openapi.Response(404, "Not found", "ErrorEnvelope", errResp),
	)

	doc.Op("PUT", "/api/event-types/{id}", "updateEventType", "Update an event type",
		openapi.Tag(tag),
		openapi.PathParam("id", "Event type id"),
		openapi.RequestBody("UpdateEventTypeCommand", "Fields to update", &operations.UpdateCommand{}),
		openapi.Response(204, "Updated", "", nil),
		openapi.Response(404, "Not found", "ErrorEnvelope", errResp),
	)

	doc.Op("DELETE", "/api/event-types/{id}", "deleteEventType", "Archive an event type",
		openapi.Tag(tag),
		openapi.PathParam("id", "Event type id"),
		openapi.Response(204, "Archived", "", nil),
		openapi.Response(404, "Not found", "ErrorEnvelope", errResp),
	)

	doc.Op("POST", "/api/event-types/{id}/schemas", "addEventTypeSchema", "Add a schema version to an event type",
		openapi.Tag(tag),
		openapi.PathParam("id", "Event type id"),
		openapi.RequestBody("AddSchemaCommand", "Schema to add", &operations.AddSchemaCommand{}),
		openapi.Response(201, "Schema added", "EventType", &eventtype.EventType{}),
		openapi.Response(404, "Event type not found", "ErrorEnvelope", errResp),
	)
}
