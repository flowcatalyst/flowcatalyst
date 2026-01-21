package tech.flowcatalyst.eventtype;

import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.media.Content;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponses;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import tech.flowcatalyst.eventtype.events.*;
import tech.flowcatalyst.eventtype.operations.addschema.AddSchemaCommand;
import tech.flowcatalyst.eventtype.operations.archiveeventtype.ArchiveEventTypeCommand;
import tech.flowcatalyst.eventtype.operations.createeventtype.CreateEventTypeCommand;
import tech.flowcatalyst.eventtype.operations.deleteeventtype.DeleteEventTypeCommand;
import tech.flowcatalyst.eventtype.operations.deprecateschema.DeprecateSchemaCommand;
import tech.flowcatalyst.eventtype.operations.finaliseschema.FinaliseSchemaCommand;
import tech.flowcatalyst.eventtype.operations.updateeventtype.UpdateEventTypeCommand;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.TracingContext;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.List;
import java.util.Map;

/**
 * REST resource for EventType operations.
 *
 * <p>This resource uses the domain-driven pattern with:
 * <ul>
 *   <li>Commands for write operations</li>
 *   <li>ExecutionContext for tracing and principal info</li>
 *   <li>Result type for success/failure handling</li>
 *   <li>Atomic commits via UnitOfWork</li>
 * </ul>
 */
@Path("/api/event-types")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@Tag(name = "Event Types", description = "Manage event type definitions")
public class EventTypeResource {

    @Inject
    EventTypeOperations eventTypeOperations;

    @Inject
    AuditContext auditContext;

    @Inject
    TracingContext tracingContext;

    // ========================================================================
    // Read Operations
    // ========================================================================

    @GET
    @Operation(summary = "List all event types", description = "Returns all event types with optional filtering. Supports multi-value filtering with comma-separated values.")
    @APIResponses({
        @APIResponse(
            responseCode = "200",
            description = "List of event types",
            content = @Content(mediaType = MediaType.APPLICATION_JSON,
                schema = @Schema(implementation = EventTypeListResponse.class))
        )
    })
    public Response listEventTypes(
        @QueryParam("status") EventTypeStatus status,
        @QueryParam("application") List<String> applications,
        @QueryParam("subdomain") List<String> subdomains,
        @QueryParam("aggregate") List<String> aggregates
    ) {
        List<String> filteredApps = filterEmpty(applications);
        List<String> filteredSubs = filterEmpty(subdomains);
        List<String> filteredAggs = filterEmpty(aggregates);

        List<EventType> eventTypes = eventTypeOperations.findWithFilters(
            filteredApps.isEmpty() ? null : filteredApps,
            filteredSubs.isEmpty() ? null : filteredSubs,
            filteredAggs.isEmpty() ? null : filteredAggs,
            status
        );

        List<EventTypeResponse> responses = eventTypes.stream()
            .map(EventTypeResponse::from)
            .toList();

        return Response.ok(new EventTypeListResponse(responses)).build();
    }

    @GET
    @Path("/{id}")
    @Operation(summary = "Get event type by ID")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Event type found"),
        @APIResponse(responseCode = "404", description = "Event type not found")
    })
    public Response getEventType(@PathParam("id") String id) {
        return eventTypeOperations.findById(id)
            .map(et -> Response.ok(EventTypeResponse.from(et)).build())
            .orElse(Response.status(404)
                .entity(new ErrorResponse("EVENT_TYPE_NOT_FOUND", "Event type not found: " + id))
                .build());
    }

    @GET
    @Path("/filters/applications")
    @Operation(summary = "Get distinct application names for filtering")
    public Response getApplications() {
        List<String> applications = eventTypeOperations.getDistinctApplications();
        return Response.ok(new FilterOptionsResponse(applications)).build();
    }

    @GET
    @Path("/filters/subdomains")
    @Operation(summary = "Get distinct subdomains, optionally filtered by applications")
    public Response getSubdomains(@QueryParam("application") List<String> applications) {
        List<String> filteredApps = filterEmpty(applications);
        List<String> subdomains = eventTypeOperations.getDistinctSubdomains(filteredApps);
        return Response.ok(new FilterOptionsResponse(subdomains)).build();
    }

    @GET
    @Path("/filters/aggregates")
    @Operation(summary = "Get distinct aggregates, optionally filtered by applications and subdomains")
    public Response getAggregates(
        @QueryParam("application") List<String> applications,
        @QueryParam("subdomain") List<String> subdomains
    ) {
        List<String> filteredApps = filterEmpty(applications);
        List<String> filteredSubs = filterEmpty(subdomains);
        List<String> aggregates = eventTypeOperations.getDistinctAggregates(
            filteredApps.isEmpty() ? null : filteredApps,
            filteredSubs.isEmpty() ? null : filteredSubs
        );
        return Response.ok(new FilterOptionsResponse(aggregates)).build();
    }

    // ========================================================================
    // Write Operations
    // ========================================================================

    @POST
    @Operation(summary = "Create a new event type")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Event type created"),
        @APIResponse(responseCode = "400", description = "Invalid request"),
        @APIResponse(responseCode = "409", description = "Business rule violation")
    })
    public Response createEventType(CreateEventTypeRequest request) {
        ExecutionContext context = createExecutionContext();

        CreateEventTypeCommand command = new CreateEventTypeCommand(
            request.code(),
            request.name(),
            request.description()
        );

        Result<EventTypeCreated> result = eventTypeOperations.createEventType(command, context);

        return switch (result) {
            case Result.Success<EventTypeCreated> s -> {
                // Fetch the created entity to return full response
                EventType eventType = eventTypeOperations.findById(s.value().eventTypeId())
                    .orElseThrow();
                yield Response.status(201)
                    .entity(EventTypeResponse.from(eventType))
                    .build();
            }
            case Result.Failure<EventTypeCreated> f -> mapErrorToResponse(f.error());
        };
    }

    @PATCH
    @Path("/{id}")
    @Operation(summary = "Update an event type's name or description")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Event type updated"),
        @APIResponse(responseCode = "400", description = "Invalid request"),
        @APIResponse(responseCode = "404", description = "Event type not found")
    })
    public Response updateEventType(@PathParam("id") String id, UpdateEventTypeRequest request) {
        ExecutionContext context = createExecutionContext();

        UpdateEventTypeCommand command = new UpdateEventTypeCommand(
            id,
            request.name(),
            request.description()
        );

        Result<EventTypeUpdated> result = eventTypeOperations.updateEventType(command, context);

        return switch (result) {
            case Result.Success<EventTypeUpdated> s -> {
                EventType eventType = eventTypeOperations.findById(s.value().eventTypeId())
                    .orElseThrow();
                yield Response.ok(EventTypeResponse.from(eventType)).build();
            }
            case Result.Failure<EventTypeUpdated> f -> mapErrorToResponse(f.error());
        };
    }

    @POST
    @Path("/{id}/schemas")
    @Operation(summary = "Add a new schema version to an event type")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Schema added"),
        @APIResponse(responseCode = "400", description = "Invalid request"),
        @APIResponse(responseCode = "404", description = "Event type not found"),
        @APIResponse(responseCode = "409", description = "Version already exists")
    })
    public Response addSchema(@PathParam("id") String id, AddSchemaRequest request) {
        ExecutionContext context = createExecutionContext();

        AddSchemaCommand command = new AddSchemaCommand(
            id,
            request.version(),
            request.mimeType(),
            request.schema(),
            request.schemaType()
        );

        Result<SchemaAdded> result = eventTypeOperations.addSchema(command, context);

        return switch (result) {
            case Result.Success<SchemaAdded> s -> {
                EventType eventType = eventTypeOperations.findById(s.value().eventTypeId())
                    .orElseThrow();
                yield Response.status(201).entity(EventTypeResponse.from(eventType)).build();
            }
            case Result.Failure<SchemaAdded> f -> mapErrorToResponse(f.error());
        };
    }

    @POST
    @Path("/{id}/schemas/{version}/finalise")
    @Operation(summary = "Finalise a schema version (FINALISING → CURRENT)")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Schema finalised"),
        @APIResponse(responseCode = "400", description = "Invalid request"),
        @APIResponse(responseCode = "404", description = "Event type or version not found")
    })
    public Response finaliseSchema(@PathParam("id") String id, @PathParam("version") String version) {
        ExecutionContext context = createExecutionContext();

        FinaliseSchemaCommand command = new FinaliseSchemaCommand(id, version);

        Result<SchemaFinalised> result = eventTypeOperations.finaliseSchema(command, context);

        return switch (result) {
            case Result.Success<SchemaFinalised> s -> {
                EventType eventType = eventTypeOperations.findById(s.value().eventTypeId())
                    .orElseThrow();
                yield Response.ok(EventTypeResponse.from(eventType)).build();
            }
            case Result.Failure<SchemaFinalised> f -> mapErrorToResponse(f.error());
        };
    }

    @POST
    @Path("/{id}/schemas/{version}/deprecate")
    @Operation(summary = "Deprecate a schema version (CURRENT → DEPRECATED)")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Schema deprecated"),
        @APIResponse(responseCode = "400", description = "Invalid request"),
        @APIResponse(responseCode = "404", description = "Event type or version not found")
    })
    public Response deprecateSchema(@PathParam("id") String id, @PathParam("version") String version) {
        ExecutionContext context = createExecutionContext();

        DeprecateSchemaCommand command = new DeprecateSchemaCommand(id, version);

        Result<SchemaDeprecated> result = eventTypeOperations.deprecateSchema(command, context);

        return switch (result) {
            case Result.Success<SchemaDeprecated> s -> {
                EventType eventType = eventTypeOperations.findById(s.value().eventTypeId())
                    .orElseThrow();
                yield Response.ok(EventTypeResponse.from(eventType)).build();
            }
            case Result.Failure<SchemaDeprecated> f -> mapErrorToResponse(f.error());
        };
    }

    @POST
    @Path("/{id}/archive")
    @Operation(summary = "Archive an event type (CURRENT → ARCHIVE)")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Event type archived"),
        @APIResponse(responseCode = "400", description = "Invalid request - all schemas must be deprecated first"),
        @APIResponse(responseCode = "404", description = "Event type not found")
    })
    public Response archiveEventType(@PathParam("id") String id) {
        ExecutionContext context = createExecutionContext();

        ArchiveEventTypeCommand command = new ArchiveEventTypeCommand(id);

        Result<EventTypeArchived> result = eventTypeOperations.archiveEventType(command, context);

        return switch (result) {
            case Result.Success<EventTypeArchived> s -> {
                EventType eventType = eventTypeOperations.findById(s.value().eventTypeId())
                    .orElseThrow();
                yield Response.ok(EventTypeResponse.from(eventType)).build();
            }
            case Result.Failure<EventTypeArchived> f -> mapErrorToResponse(f.error());
        };
    }

    @DELETE
    @Path("/{id}")
    @Operation(summary = "Delete an event type")
    @APIResponses({
        @APIResponse(responseCode = "204", description = "Event type deleted"),
        @APIResponse(responseCode = "400", description = "Invalid request - must be archived or have no finalized schemas"),
        @APIResponse(responseCode = "404", description = "Event type not found")
    })
    public Response deleteEventType(@PathParam("id") String id) {
        ExecutionContext context = createExecutionContext();

        DeleteEventTypeCommand command = new DeleteEventTypeCommand(id);

        Result<EventTypeDeleted> result = eventTypeOperations.deleteEventType(command, context);

        return switch (result) {
            case Result.Success<EventTypeDeleted> s -> Response.noContent().build();
            case Result.Failure<EventTypeDeleted> f -> mapErrorToResponse(f.error());
        };
    }

    // ========================================================================
    // Helper Methods
    // ========================================================================

    private ExecutionContext createExecutionContext() {
        return ExecutionContext.from(tracingContext, auditContext.requirePrincipalId());
    }

    private List<String> filterEmpty(List<String> list) {
        if (list == null) return List.of();
        return list.stream()
            .filter(s -> s != null && !s.isBlank())
            .toList();
    }

    private Response mapErrorToResponse(UseCaseError error) {
        Response.Status status = switch (error) {
            case UseCaseError.ValidationError v -> Response.Status.BAD_REQUEST;
            case UseCaseError.NotFoundError n -> Response.Status.NOT_FOUND;
            case UseCaseError.BusinessRuleViolation b -> Response.Status.CONFLICT;
            case UseCaseError.ConcurrencyError c -> Response.Status.CONFLICT;
            case UseCaseError.AuthorizationError a -> Response.Status.FORBIDDEN;
        };

        return Response.status(status)
            .entity(new ErrorResponse(error.code(), error.message(), error.details()))
            .build();
    }

    // ========================================================================
    // DTOs
    // ========================================================================

    public record EventTypeListResponse(List<EventTypeResponse> items) {}

    public record EventTypeResponse(
        String id,
        String code,
        String name,
        String description,
        EventTypeStatus status,
        String application,
        String subdomain,
        String aggregate,
        String event,
        List<SpecVersionResponse> specVersions,
        String createdAt,
        String updatedAt
    ) {
        public static EventTypeResponse from(EventType et) {
            String[] parts = et.code().split(":");
            return new EventTypeResponse(
                et.id(),
                et.code(),
                et.name(),
                et.description(),
                et.status(),
                parts.length > 0 ? parts[0] : null,
                parts.length > 1 ? parts[1] : null,
                parts.length > 2 ? parts[2] : null,
                parts.length > 3 ? parts[3] : null,
                et.specVersions() != null
                    ? et.specVersions().stream().map(SpecVersionResponse::from).toList()
                    : List.of(),
                et.createdAt() != null ? et.createdAt().toString() : null,
                et.updatedAt() != null ? et.updatedAt().toString() : null
            );
        }
    }

    public record SpecVersionResponse(
        String version,
        String mimeType,
        String schemaType,
        String status
    ) {
        public static SpecVersionResponse from(SpecVersion sv) {
            return new SpecVersionResponse(
                sv.version(),
                sv.mimeType(),
                sv.schemaType() != null ? sv.schemaType().name() : null,
                sv.status() != null ? sv.status().name() : null
            );
        }
    }

    public record CreateEventTypeRequest(
        String code,
        String name,
        String description
    ) {}

    public record UpdateEventTypeRequest(
        String name,
        String description
    ) {}

    public record AddSchemaRequest(
        String version,
        String mimeType,
        String schema,
        SchemaType schemaType
    ) {}

    public record FilterOptionsResponse(List<String> options) {}

    public record ErrorResponse(String code, String message, Map<String, Object> details) {
        public ErrorResponse(String code, String message) {
            this(code, message, Map.of());
        }
    }
}
