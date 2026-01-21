package tech.flowcatalyst.platform.bff;

import io.quarkus.runtime.annotations.RegisterForReflection;
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
import tech.flowcatalyst.event.ContextData;
import tech.flowcatalyst.event.Event;
import tech.flowcatalyst.event.EventRepository;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.authorization.AuthorizationService;
import tech.flowcatalyst.platform.authorization.platform.PlatformMessagingPermissions;

import java.util.List;

/**
 * Debug BFF resource for querying raw events from the transactional events collection.
 *
 * This endpoint is for admin/debug purposes only. The raw events collection has
 * minimal indexes optimized for writes. For regular UI queries, use /api/bff/events
 * which queries the events_read projection.
 */
@Path("/api/bff/debug/events")
@Produces(MediaType.APPLICATION_JSON)
@Tag(name = "Raw Events (Debug)", description = "Debug endpoint for raw event store queries")
@RegisterForReflection(registerFullHierarchy = true)
public class RawEventBffResource {

    @Inject
    EventRepository eventRepository;

    @Inject
    AuditContext auditContext;

    @Inject
    AuthorizationService authorizationService;

    @GET
    @Operation(summary = "List raw events", description = "List raw events from the transactional collection (debug/admin only)")
    @APIResponses({
        @APIResponse(
            responseCode = "200",
            description = "Events found",
            content = @Content(mediaType = MediaType.APPLICATION_JSON,
                schema = @Schema(implementation = PagedRawEventResponse.class))
        )
    })
    public Response listRawEvents(
        @QueryParam("page") @DefaultValue("0") Integer page,
        @QueryParam("size") @DefaultValue("20") Integer size
    ) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.EVENT_VIEW_RAW);

        // Validate pagination
        if (page < 0) page = 0;
        if (size < 1 || size > 100) size = 20;

        // Query raw events - limited filtering since this is debug
        List<Event> events = eventRepository.findRecentPaged(page, size);

        long totalCount = eventRepository.count();

        List<RawEventResponse> responses = events.stream()
            .map(RawEventResponse::from)
            .toList();

        return Response.ok(new PagedRawEventResponse(
            responses,
            page,
            size,
            totalCount,
            (int) Math.ceil((double) totalCount / size)
        )).build();
    }

    @GET
    @Path("/{id}")
    @Operation(summary = "Get raw event by ID", description = "Get a single raw event by its ID (debug/admin only)")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Event found"),
        @APIResponse(responseCode = "404", description = "Event not found")
    })
    public Response getRawEvent(@PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.EVENT_VIEW_RAW);

        return eventRepository.findByIdOptional(id)
            .map(event -> Response.ok(RawEventResponse.from(event)).build())
            .orElse(Response.status(404)
                .entity(new ErrorResponse("Event not found: " + id))
                .build());
    }

    // ========================================================================
    // DTOs
    // ========================================================================

    public record RawEventResponse(
        String id,
        String specVersion,
        String type,
        String source,
        String subject,
        String time,
        String data,
        String messageGroup,
        String correlationId,
        String causationId,
        String deduplicationId,
        List<ContextDataResponse> contextData,
        String clientId
    ) {
        public static RawEventResponse from(Event event) {
            List<ContextDataResponse> contextDataResponses = null;
            if (event.contextData != null) {
                contextDataResponses = event.contextData.stream()
                    .map(cd -> new ContextDataResponse(cd.key(), cd.value()))
                    .toList();
            }

            return new RawEventResponse(
                event.id,
                event.specVersion,
                event.type,
                event.source,
                event.subject,
                event.time != null ? event.time.toString() : null,
                event.data,
                event.messageGroup,
                event.correlationId,
                event.causationId,
                event.deduplicationId,
                contextDataResponses,
                event.clientId
            );
        }
    }

    public record ContextDataResponse(String key, String value) {}

    public record PagedRawEventResponse(
        List<RawEventResponse> items,
        int page,
        int size,
        long totalItems,
        int totalPages
    ) {}

    public record ErrorResponse(String error) {}
}
