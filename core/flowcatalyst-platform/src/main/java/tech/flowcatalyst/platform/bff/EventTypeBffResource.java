package tech.flowcatalyst.platform.bff;

import io.quarkus.runtime.annotations.RegisterForReflection;
import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import tech.flowcatalyst.eventtype.*;
import tech.flowcatalyst.eventtype.events.*;
import tech.flowcatalyst.eventtype.operations.addschema.AddSchemaCommand;
import tech.flowcatalyst.eventtype.operations.archiveeventtype.ArchiveEventTypeCommand;
import tech.flowcatalyst.eventtype.operations.createeventtype.CreateEventTypeCommand;
import tech.flowcatalyst.eventtype.operations.deleteeventtype.DeleteEventTypeCommand;
import tech.flowcatalyst.eventtype.operations.deprecateschema.DeprecateSchemaCommand;
import tech.flowcatalyst.eventtype.operations.finaliseschema.FinaliseSchemaCommand;
import tech.flowcatalyst.eventtype.operations.updateeventtype.UpdateEventTypeCommand;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.authorization.AuthorizationService;
import tech.flowcatalyst.platform.authorization.platform.PlatformMessagingPermissions;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.TracingContext;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.List;
import java.util.Map;

/**
 * BFF (Backend For Frontend) endpoints for Event Types.
 * Returns IDs as strings to preserve precision for JavaScript clients.
 */
@Path("/bff/event-types")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@Tag(name = "BFF - Event Types", description = "Web-optimized event type endpoints with string IDs")
@RegisterForReflection(registerFullHierarchy = true)
public class EventTypeBffResource {

    @Inject
    EventTypeOperations eventTypeOperations;

    @Inject
    AuditContext auditContext;

    @Inject
    TracingContext tracingContext;

    @Inject
    AuthorizationService authorizationService;

    @GET
    @Operation(summary = "List all event types (BFF)")
    public Response listEventTypes(
        @QueryParam("status") EventTypeStatus status,
        @QueryParam("application") List<String> applications,
        @QueryParam("subdomain") List<String> subdomains,
        @QueryParam("aggregate") List<String> aggregates
    ) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.EVENT_TYPE_VIEW);

        List<String> filteredApps = filterEmpty(applications);
        List<String> filteredSubs = filterEmpty(subdomains);
        List<String> filteredAggs = filterEmpty(aggregates);

        List<EventType> eventTypes = eventTypeOperations.findWithFilters(
            filteredApps.isEmpty() ? null : filteredApps,
            filteredSubs.isEmpty() ? null : filteredSubs,
            filteredAggs.isEmpty() ? null : filteredAggs,
            status
        );

        List<BffEventTypeResponse> responses = eventTypes.stream()
            .map(BffEventTypeResponse::from)
            .toList();

        return Response.ok(new BffEventTypeListResponse(responses)).build();
    }

    @GET
    @Path("/{id}")
    @Operation(summary = "Get event type by ID (BFF)")
    public Response getEventType(@PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.EVENT_TYPE_VIEW);

        return eventTypeOperations.findById(id)
            .map(et -> Response.ok(BffEventTypeResponse.from(et)).build())
            .orElse(Response.status(404)
                .entity(new ErrorResponse("EVENT_TYPE_NOT_FOUND", "Event type not found: " + id))
                .build());
    }

    @GET
    @Path("/filters/applications")
    @Operation(summary = "Get distinct application names")
    public Response getApplications() {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.EVENT_TYPE_VIEW);

        List<String> applications = eventTypeOperations.getDistinctApplications();
        return Response.ok(new FilterOptionsResponse(applications)).build();
    }

    @GET
    @Path("/filters/subdomains")
    @Operation(summary = "Get distinct subdomains")
    public Response getSubdomains(@QueryParam("application") List<String> applications) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.EVENT_TYPE_VIEW);

        List<String> filteredApps = filterEmpty(applications);
        List<String> subdomains = eventTypeOperations.getDistinctSubdomains(filteredApps);
        return Response.ok(new FilterOptionsResponse(subdomains)).build();
    }

    @GET
    @Path("/filters/aggregates")
    @Operation(summary = "Get distinct aggregates")
    public Response getAggregates(
        @QueryParam("application") List<String> applications,
        @QueryParam("subdomain") List<String> subdomains
    ) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.EVENT_TYPE_VIEW);

        List<String> filteredApps = filterEmpty(applications);
        List<String> filteredSubs = filterEmpty(subdomains);
        List<String> aggregates = eventTypeOperations.getDistinctAggregates(
            filteredApps.isEmpty() ? null : filteredApps,
            filteredSubs.isEmpty() ? null : filteredSubs
        );
        return Response.ok(new FilterOptionsResponse(aggregates)).build();
    }

    @POST
    @Operation(summary = "Create a new event type (BFF)")
    public Response createEventType(CreateEventTypeRequest request) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.EVENT_TYPE_CREATE);

        ExecutionContext context = ExecutionContext.from(tracingContext, principalId);

        CreateEventTypeCommand command = new CreateEventTypeCommand(
            request.application(),
            request.subdomain(),
            request.aggregate(),
            request.event(),
            request.name(),
            request.description(),
            request.clientScoped()
        );

        Result<EventTypeCreated> result = eventTypeOperations.createEventType(command, context);

        return switch (result) {
            case Result.Success<EventTypeCreated> s -> {
                EventType eventType = eventTypeOperations.findById(s.value().eventTypeId())
                    .orElseThrow();
                yield Response.status(201).entity(BffEventTypeResponse.from(eventType)).build();
            }
            case Result.Failure<EventTypeCreated> f -> mapErrorToResponse(f.error());
        };
    }

    @PATCH
    @Path("/{id}")
    @Operation(summary = "Update an event type (BFF)")
    public Response updateEventType(@PathParam("id") String id, UpdateEventTypeRequest request) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.EVENT_TYPE_UPDATE);

        ExecutionContext context = ExecutionContext.from(tracingContext, principalId);

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
                yield Response.ok(BffEventTypeResponse.from(eventType)).build();
            }
            case Result.Failure<EventTypeUpdated> f -> mapErrorToResponse(f.error());
        };
    }

    @POST
    @Path("/{id}/schemas")
    @Operation(summary = "Add a schema version (BFF)")
    public Response addSchema(@PathParam("id") String id, AddSchemaRequest request) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.EVENT_TYPE_UPDATE);

        ExecutionContext context = ExecutionContext.from(tracingContext, principalId);

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
                yield Response.status(201).entity(BffEventTypeResponse.from(eventType)).build();
            }
            case Result.Failure<SchemaAdded> f -> mapErrorToResponse(f.error());
        };
    }

    @POST
    @Path("/{id}/schemas/{version}/finalise")
    @Operation(summary = "Finalise a schema version (BFF)")
    public Response finaliseSchema(@PathParam("id") String id, @PathParam("version") String version) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.EVENT_TYPE_UPDATE);

        ExecutionContext context = ExecutionContext.from(tracingContext, principalId);

        FinaliseSchemaCommand command = new FinaliseSchemaCommand(id, version);

        Result<SchemaFinalised> result = eventTypeOperations.finaliseSchema(command, context);

        return switch (result) {
            case Result.Success<SchemaFinalised> s -> {
                EventType eventType = eventTypeOperations.findById(s.value().eventTypeId())
                    .orElseThrow();
                yield Response.ok(BffEventTypeResponse.from(eventType)).build();
            }
            case Result.Failure<SchemaFinalised> f -> mapErrorToResponse(f.error());
        };
    }

    @POST
    @Path("/{id}/schemas/{version}/deprecate")
    @Operation(summary = "Deprecate a schema version (BFF)")
    public Response deprecateSchema(@PathParam("id") String id, @PathParam("version") String version) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.EVENT_TYPE_UPDATE);

        ExecutionContext context = ExecutionContext.from(tracingContext, principalId);

        DeprecateSchemaCommand command = new DeprecateSchemaCommand(id, version);

        Result<SchemaDeprecated> result = eventTypeOperations.deprecateSchema(command, context);

        return switch (result) {
            case Result.Success<SchemaDeprecated> s -> {
                EventType eventType = eventTypeOperations.findById(s.value().eventTypeId())
                    .orElseThrow();
                yield Response.ok(BffEventTypeResponse.from(eventType)).build();
            }
            case Result.Failure<SchemaDeprecated> f -> mapErrorToResponse(f.error());
        };
    }

    @POST
    @Path("/{id}/archive")
    @Operation(summary = "Archive an event type (BFF)")
    public Response archiveEventType(@PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.EVENT_TYPE_UPDATE);

        ExecutionContext context = ExecutionContext.from(tracingContext, principalId);

        ArchiveEventTypeCommand command = new ArchiveEventTypeCommand(id);

        Result<EventTypeArchived> result = eventTypeOperations.archiveEventType(command, context);

        return switch (result) {
            case Result.Success<EventTypeArchived> s -> {
                EventType eventType = eventTypeOperations.findById(s.value().eventTypeId())
                    .orElseThrow();
                yield Response.ok(BffEventTypeResponse.from(eventType)).build();
            }
            case Result.Failure<EventTypeArchived> f -> mapErrorToResponse(f.error());
        };
    }

    @DELETE
    @Path("/{id}")
    @Operation(summary = "Delete an event type (BFF)")
    public Response deleteEventType(@PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.EVENT_TYPE_DELETE);

        ExecutionContext context = ExecutionContext.from(tracingContext, principalId);

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
    // BFF DTOs - IDs as Strings for JavaScript precision
    // ========================================================================

    public record BffEventTypeListResponse(List<BffEventTypeResponse> items) {}

    public record BffEventTypeResponse(
        String id,
        String code,
        String name,
        String description,
        EventTypeStatus status,
        boolean clientScoped,
        String application,
        String subdomain,
        String aggregate,
        String event,
        List<BffSpecVersionResponse> specVersions,
        String createdAt,
        String updatedAt
    ) {
        public static BffEventTypeResponse from(EventType et) {
            var parts = et.code().split(":");
            return new BffEventTypeResponse(
                et.id() != null ? et.id().toString() : null,
                et.code(),
                et.name(),
                et.description(),
                et.status(),
                et.clientScoped(),
                et.application(),
                et.subdomain(),
                et.aggregate(),
                parts.length > 3 ? parts[3] : null,
                et.specVersions() != null
                    ? et.specVersions().stream().map(BffSpecVersionResponse::from).toList()
                    : List.of(),
                et.createdAt() != null ? et.createdAt().toString() : null,
                et.updatedAt() != null ? et.updatedAt().toString() : null
            );
        }
    }

    public record BffSpecVersionResponse(
        String version,
        String mimeType,
        String schemaType,
        String status
    ) {
        public static BffSpecVersionResponse from(SpecVersion sv) {
            return new BffSpecVersionResponse(
                sv.version(),
                sv.mimeType(),
                sv.schemaType() != null ? sv.schemaType().name() : null,
                sv.status() != null ? sv.status().name() : null
            );
        }
    }

    public record CreateEventTypeRequest(
        String application,
        String subdomain,
        String aggregate,
        String event,
        String name,
        String description,
        boolean clientScoped
    ) {}
    public record UpdateEventTypeRequest(String name, String description) {}
    public record AddSchemaRequest(String version, String mimeType, String schema, SchemaType schemaType) {}
    public record FilterOptionsResponse(List<String> options) {}
    public record ErrorResponse(String code, String message, Map<String, Object> details) {
        public ErrorResponse(String code, String message) {
            this(code, message, Map.of());
        }
    }
}
