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
import tech.flowcatalyst.event.read.EventRead;
import tech.flowcatalyst.event.read.EventReadRepository;
import tech.flowcatalyst.event.read.EventReadRepository.EventFilter;
import tech.flowcatalyst.event.read.EventReadRepository.FilterOptions;
import tech.flowcatalyst.event.read.EventReadRepository.FilterOptionsRequest;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.authorization.AuthorizationService;
import tech.flowcatalyst.platform.authorization.platform.PlatformMessagingPermissions;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.ClientRepository;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TypedId;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;

/**
 * BFF resource for querying events from the events_read projection collection.
 *
 * This endpoint is optimized for UI queries with rich filtering and pagination.
 * The underlying events_read collection has indexes optimized for these queries.
 */
@Path("/api/bff/events")
@Produces(MediaType.APPLICATION_JSON)
@Tag(name = "Events (BFF)", description = "Query events from the read-optimized projection")
@RegisterForReflection(registerFullHierarchy = true)
public class EventBffResource {

    @Inject
    EventReadRepository eventReadRepository;

    @Inject
    ClientRepository clientRepository;

    @Inject
    AuditContext auditContext;

    @Inject
    AuthorizationService authorizationService;

    @GET
    @Operation(summary = "Search events", description = "Search events with optional filters and pagination. " +
        "Multi-value parameters (clientIds, applications, etc.) support comma-separated values for OR filtering. " +
        "Use 'null' in clientIds to include platform events (no client).")
    @APIResponses({
        @APIResponse(
            responseCode = "200",
            description = "Events found",
            content = @Content(mediaType = MediaType.APPLICATION_JSON,
                schema = @Schema(implementation = PagedEventResponse.class))
        )
    })
    public Response searchEvents(
        @QueryParam("clientIds") String clientIds,
        @QueryParam("applications") String applications,
        @QueryParam("subdomains") String subdomains,
        @QueryParam("aggregates") String aggregates,
        @QueryParam("types") String types,
        @QueryParam("source") String source,
        @QueryParam("subject") String subject,
        @QueryParam("correlationId") String correlationId,
        @QueryParam("messageGroup") String messageGroup,
        @QueryParam("timeAfter") Instant timeAfter,
        @QueryParam("timeBefore") Instant timeBefore,
        @QueryParam("page") @DefaultValue("0") Integer page,
        @QueryParam("size") @DefaultValue("20") Integer size
    ) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.EVENT_VIEW);

        EventFilter filter = new EventFilter(
            parseCommaSeparated(clientIds),
            parseCommaSeparated(applications),
            parseCommaSeparated(subdomains),
            parseCommaSeparated(aggregates),
            parseCommaSeparated(types),
            source, subject, correlationId, messageGroup,
            timeAfter, timeBefore, page, size
        );

        List<EventRead> events = eventReadRepository.findWithFilter(filter);
        long totalCount = eventReadRepository.countWithFilter(filter);

        List<EventReadResponse> responses = events.stream()
            .map(EventReadResponse::from)
            .toList();

        return Response.ok(new PagedEventResponse(
            responses,
            page,
            size,
            totalCount,
            (int) Math.ceil((double) totalCount / size)
        )).build();
    }

    @GET
    @Path("/filter-options")
    @Operation(summary = "Get filter options", description = "Get available filter values for cascading filters. " +
        "Each level is narrowed by selections at higher levels (client → application → subdomain → aggregate → type).")
    @APIResponses({
        @APIResponse(
            responseCode = "200",
            description = "Filter options",
            content = @Content(mediaType = MediaType.APPLICATION_JSON,
                schema = @Schema(implementation = FilterOptionsResponse.class))
        )
    })
    public Response getFilterOptions(
        @QueryParam("clientIds") String clientIds,
        @QueryParam("applications") String applications,
        @QueryParam("subdomains") String subdomains,
        @QueryParam("aggregates") String aggregates
    ) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.EVENT_VIEW);

        // Get clients from the clients collection (not from events)
        List<FilterOption> clientOptionsList = new ArrayList<>();
        // Add "Platform" option for events with no client
        clientOptionsList.add(new FilterOption("null", "Platform (No Client)"));
        // Add all clients from the clients collection (dedupe by ID)
        java.util.Map<String, Client> clientsById = new java.util.LinkedHashMap<>();
        for (Client c : clientRepository.listAll()) {
            clientsById.putIfAbsent(c.id, c);
        }
        clientsById.values().stream()
            .map(c -> new FilterOption(
                TypedId.Ops.serialize(EntityType.CLIENT, c.id),
                c.name != null ? c.name : c.identifier))
            .sorted((a, b) -> a.label().compareToIgnoreCase(b.label()))
            .forEach(clientOptionsList::add);

        // Get event-based filter options (applications, subdomains, aggregates, types)
        FilterOptionsRequest request = new FilterOptionsRequest(
            parseCommaSeparated(clientIds),
            parseCommaSeparated(applications),
            parseCommaSeparated(subdomains),
            parseCommaSeparated(aggregates)
        );

        FilterOptions eventOptions = eventReadRepository.getFilterOptions(request);

        // Build response with clients from collection and other options from events
        // Use distinct() to ensure no duplicates (can happen with mixed denormalized/parsed data)
        return Response.ok(new FilterOptionsResponse(
            clientOptionsList,
            eventOptions.applications().stream()
                .filter(a -> a != null && !a.isBlank())
                .map(String::trim)
                .distinct()
                .sorted(String::compareToIgnoreCase)
                .map(a -> new FilterOption(a, a))
                .toList(),
            eventOptions.subdomains().stream()
                .filter(s -> s != null && !s.isBlank())
                .map(String::trim)
                .distinct()
                .sorted(String::compareToIgnoreCase)
                .map(s -> new FilterOption(s, s))
                .toList(),
            eventOptions.aggregates().stream()
                .filter(a -> a != null && !a.isBlank())
                .map(String::trim)
                .distinct()
                .sorted(String::compareToIgnoreCase)
                .map(a -> new FilterOption(a, a))
                .toList(),
            eventOptions.types().stream()
                .filter(t -> t != null && !t.isBlank())
                .map(String::trim)
                .distinct()
                .sorted(String::compareToIgnoreCase)
                .map(t -> new FilterOption(t, t))
                .toList()
        )).build();
    }

    /**
     * Parse comma-separated string into list, trimming whitespace.
     * Returns null if input is null or empty.
     */
    private List<String> parseCommaSeparated(String value) {
        if (value == null || value.isBlank()) {
            return null;
        }
        return java.util.Arrays.stream(value.split(","))
            .map(String::trim)
            .filter(s -> !s.isEmpty())
            .toList();
    }

    @GET
    @Path("/{id}")
    @Operation(summary = "Get event by ID", description = "Get a single event by its ID")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Event found"),
        @APIResponse(responseCode = "404", description = "Event not found")
    })
    public Response getEvent(@PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.EVENT_VIEW);

        return eventReadRepository.findByIdOptional(id)
            .map(event -> Response.ok(EventReadResponse.from(event)).build())
            .orElse(Response.status(404)
                .entity(new ErrorResponse("Event not found: " + id))
                .build());
    }

    // ========================================================================
    // DTOs
    // ========================================================================

    public record EventReadResponse(
        String id,
        String specVersion,
        String type,
        String application,
        String subdomain,
        String aggregate,
        String source,
        String subject,
        String time,
        String data,
        String messageGroup,
        String correlationId,
        String causationId,
        String deduplicationId,
        List<ContextDataResponse> contextData,
        String clientId,
        String projectedAt
    ) {
        public static EventReadResponse from(EventRead event) {
            List<ContextDataResponse> contextDataResponses = null;
            if (event.contextData != null) {
                contextDataResponses = event.contextData.stream()
                    .map(cd -> new ContextDataResponse(cd.key, cd.value))
                    .toList();
            }

            return new EventReadResponse(
                event.id,
                event.specVersion,
                event.type,
                event.application,
                event.subdomain,
                event.aggregate,
                event.source,
                event.subject,
                event.time != null ? event.time.toString() : null,
                event.data,
                event.messageGroup,
                event.correlationId,
                event.causationId,
                event.deduplicationId,
                contextDataResponses,
                event.clientId,
                event.projectedAt != null ? event.projectedAt.toString() : null
            );
        }
    }

    public record ContextDataResponse(String key, String value) {}

    public record PagedEventResponse(
        List<EventReadResponse> items,
        int page,
        int size,
        long totalItems,
        int totalPages
    ) {}

    /**
     * Filter options for cascading filter UI.
     * null values in clients list represent platform events (no client).
     */
    public record FilterOptionsResponse(
        List<FilterOption> clients,
        List<FilterOption> applications,
        List<FilterOption> subdomains,
        List<FilterOption> aggregates,
        List<FilterOption> types
    ) {
        public static FilterOptionsResponse from(FilterOptions options) {
            return new FilterOptionsResponse(
                options.clients().stream()
                    .map(c -> new FilterOption(c == null ? "null" : c, c == null ? "Platform (No Client)" : c))
                    .toList(),
                options.applications().stream()
                    .filter(a -> a != null)  // Skip null applications
                    .map(a -> new FilterOption(a, a))
                    .toList(),
                options.subdomains().stream()
                    .filter(s -> s != null)  // Skip null subdomains
                    .map(s -> new FilterOption(s, s))
                    .toList(),
                options.aggregates().stream()
                    .filter(a -> a != null)  // Skip null aggregates
                    .map(a -> new FilterOption(a, a))
                    .toList(),
                options.types().stream()
                    .filter(t -> t != null)  // Skip null types
                    .map(t -> new FilterOption(t, t))
                    .toList()
            );
        }
    }

    /**
     * A filter option with value and display label.
     */
    public record FilterOption(String value, String label) {}

    public record ErrorResponse(String error) {}
}
