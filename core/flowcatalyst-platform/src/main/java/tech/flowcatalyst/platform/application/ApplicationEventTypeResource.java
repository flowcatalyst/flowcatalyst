package tech.flowcatalyst.platform.application;

import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.*;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.media.Content;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponses;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import tech.flowcatalyst.eventtype.EventType;
import tech.flowcatalyst.eventtype.EventTypeOperations;
import tech.flowcatalyst.eventtype.EventTypeSource;
import tech.flowcatalyst.eventtype.events.EventTypesSynced;
import tech.flowcatalyst.eventtype.operations.synceventtypes.SyncEventTypesCommand;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.authentication.JwtKeyService;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.TracingContext;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.List;
import java.util.Map;

/**
 * SDK API for external applications to manage their event types.
 *
 * <p>External applications using the FlowCatalyst SDK can:
 * <ul>
 *   <li>List event types for their application</li>
 *   <li>Sync event types (bulk create/update/delete)</li>
 * </ul>
 *
 * <p>Event type codes follow the format: {application}:{subdomain}:{aggregate}:{event}
 * For example: operant:execution:trip:started
 *
 * <p>When syncing, the application code is taken from the URL path, and
 * each event type specifies subdomain, aggregate, and event segments separately.
 */
@Path("/api/applications/{appCode}/event-types")
@Tag(name = "Application Event Types SDK", description = "SDK API for external applications to manage their event types")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
public class ApplicationEventTypeResource {

    @Inject
    EventTypeOperations eventTypeOperations;

    @Inject
    JwtKeyService jwtKeyService;

    @Inject
    AuditContext auditContext;

    @Inject
    TracingContext tracingContext;

    /**
     * List all event types for an application.
     */
    @GET
    @Operation(operationId = "listApplicationEventTypes", summary = "List application event types",
        description = "Returns all event types registered for this application (matching code prefix).")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of event types",
            content = @Content(schema = @Schema(implementation = EventTypeListResponse.class))),
        @APIResponse(responseCode = "404", description = "Application not found")
    })
    public Response listEventTypes(
            @PathParam("appCode") String appCode,
            @QueryParam("source") String source,
            @CookieParam("fc_session") String sessionToken,
            @HeaderParam("Authorization") String authHeader) {

        var principalIdOpt = jwtKeyService.extractAndValidatePrincipalId(sessionToken, authHeader);
        if (principalIdOpt.isEmpty()) {
            return Response.status(Response.Status.UNAUTHORIZED)
                .entity(new ErrorResponse("UNAUTHORIZED", "Not authenticated"))
                .build();
        }

        // Note: Application entity is not required - event types can exist for modules
        // that are not registered applications

        // Find event types by code prefix
        String codePrefix = appCode + ":";
        List<EventType> eventTypes = eventTypeOperations.findByCodePrefix(codePrefix);

        // Filter by source if provided
        if (source != null && !source.isBlank()) {
            try {
                EventTypeSource sourceEnum = EventTypeSource.valueOf(source.toUpperCase());
                eventTypes = eventTypes.stream()
                    .filter(et -> et.source() == sourceEnum)
                    .toList();
            } catch (IllegalArgumentException e) {
                return Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ErrorResponse("INVALID_SOURCE", "Invalid source. Must be API or UI"))
                    .build();
            }
        }

        List<EventTypeDto> dtos = eventTypes.stream()
            .map(this::toEventTypeDto)
            .toList();

        return Response.ok(new EventTypeListResponse(dtos, dtos.size())).build();
    }

    /**
     * Sync event types from an external application.
     * Creates new event types, updates existing API-sourced event types,
     * and optionally removes unlisted API-sourced event types.
     */
    @POST
    @Path("/sync")
    @Operation(operationId = "syncApplicationEventTypes", summary = "Sync application event types",
        description = "Bulk sync event types from an external application. " +
                      "Creates new event types, updates existing API-sourced event types. " +
                      "Set removeUnlisted=true to remove API-sourced event types not in the sync list.")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Sync complete",
            content = @Content(schema = @Schema(implementation = SyncResponse.class))),
        @APIResponse(responseCode = "404", description = "Application not found")
    })
    public Response syncEventTypes(
            @PathParam("appCode") String appCode,
            @QueryParam("removeUnlisted") @DefaultValue("false") boolean removeUnlisted,
            SyncEventTypesRequest request,
            @CookieParam("fc_session") String sessionToken,
            @HeaderParam("Authorization") String authHeader) {

        var principalIdOpt = jwtKeyService.extractAndValidatePrincipalId(sessionToken, authHeader);
        if (principalIdOpt.isEmpty()) {
            return Response.status(Response.Status.UNAUTHORIZED)
                .entity(new ErrorResponse("UNAUTHORIZED", "Not authenticated"))
                .build();
        }
        String principalId = principalIdOpt.get();

        // Note: Application entity is not required - event types can exist for modules
        // that are not registered applications

        if (request.eventTypes() == null || request.eventTypes().isEmpty()) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse("EVENT_TYPES_REQUIRED", "eventTypes list is required"))
                .build();
        }

        // Set audit context and create execution context
        auditContext.setPrincipalId(principalId);
        ExecutionContext context = ExecutionContext.from(tracingContext, principalId);

        // Convert request to internal format
        List<SyncEventTypesCommand.SyncEventTypeItem> items = request.eventTypes().stream()
            .map(et -> new SyncEventTypesCommand.SyncEventTypeItem(
                et.subdomain(),
                et.aggregate(),
                et.event(),
                et.name(),
                et.description(),
                et.clientScoped()
            ))
            .toList();

        SyncEventTypesCommand command = new SyncEventTypesCommand(appCode, items, removeUnlisted);
        Result<EventTypesSynced> result = eventTypeOperations.syncEventTypes(command, context);

        return switch (result) {
            case Result.Success<EventTypesSynced> s -> {
                // Return updated event type list
                String codePrefix = appCode + ":";
                List<EventType> eventTypes = eventTypeOperations.findByCodePrefix(codePrefix);
                List<EventTypeDto> dtos = eventTypes.stream()
                    .filter(et -> et.source() == EventTypeSource.API)
                    .map(this::toEventTypeDto)
                    .toList();
                yield Response.ok(new SyncResponse(
                    s.value().eventTypesCreated(),
                    s.value().eventTypesUpdated(),
                    s.value().eventTypesDeleted(),
                    dtos
                )).build();
            }
            case Result.Failure<EventTypesSynced> f -> mapErrorToResponse(f.error());
        };
    }

    // ==================== Helper Methods ====================

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

    private EventTypeDto toEventTypeDto(EventType eventType) {
        // Extract short code by removing app prefix
        String[] parts = eventType.code().split(":", 2);
        String shortCode = parts.length > 1 ? parts[1] : eventType.code();

        return new EventTypeDto(
            eventType.id(),
            shortCode,
            eventType.code(),
            eventType.name(),
            eventType.description(),
            eventType.source() != null ? eventType.source().name() : "UI",
            eventType.status() != null ? eventType.status().name() : "CURRENT"
        );
    }

    // ==================== DTOs ====================

    public record EventTypeDto(
        String id,
        String code,
        String fullCode,
        String name,
        String description,
        String source,
        String status
    ) {}

    public record EventTypeListResponse(
        List<EventTypeDto> eventTypes,
        int total
    ) {}

    /**
     * Event type item for sync request.
     *
     * <p>The full event type code is composed as:
     * {applicationCode}:{subdomain}:{aggregate}:{event}
     *
     * @param subdomain    Subdomain within the application (e.g., "execution", "orders")
     * @param aggregate    Aggregate name (e.g., "trip", "order")
     * @param event        Event name (e.g., "started", "created")
     * @param name         Human-friendly name
     * @param description  Optional description
     * @param clientScoped Whether events of this type are scoped to clients (optional, defaults to false)
     */
    public record SyncEventTypeItem(
        String subdomain,
        String aggregate,
        String event,
        String name,
        String description,
        Boolean clientScoped
    ) {}

    public record SyncEventTypesRequest(
        List<SyncEventTypeItem> eventTypes
    ) {}

    public record SyncResponse(
        int created,
        int updated,
        int deleted,
        List<EventTypeDto> eventTypes
    ) {}

    public record ErrorResponse(String code, String message, Map<String, Object> details) {
        public ErrorResponse(String code, String message) {
            this(code, message, Map.of());
        }
    }
}
