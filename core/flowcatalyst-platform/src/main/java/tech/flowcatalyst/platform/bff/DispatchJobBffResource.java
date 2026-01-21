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
import tech.flowcatalyst.dispatchjob.read.DispatchJobRead;
import tech.flowcatalyst.dispatchjob.read.DispatchJobReadRepository;
import tech.flowcatalyst.dispatchjob.read.DispatchJobReadRepository.DispatchJobReadFilter;
import tech.flowcatalyst.dispatchjob.read.DispatchJobReadRepository.FilterOptions;
import tech.flowcatalyst.dispatchjob.read.DispatchJobReadRepository.FilterOptionsRequest;
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
 * BFF resource for querying dispatch jobs from the dispatch_jobs_read projection collection.
 *
 * This endpoint is optimized for UI queries with rich filtering and pagination.
 * The underlying dispatch_jobs_read collection has indexes optimized for these queries.
 */
@Path("/api/bff/dispatch-jobs")
@Produces(MediaType.APPLICATION_JSON)
@Tag(name = "Dispatch Jobs (BFF)", description = "Query dispatch jobs from the read-optimized projection")
@RegisterForReflection(registerFullHierarchy = true)
public class DispatchJobBffResource {

    @Inject
    DispatchJobReadRepository dispatchJobReadRepository;

    @Inject
    ClientRepository clientRepository;

    @Inject
    AuditContext auditContext;

    @Inject
    AuthorizationService authorizationService;

    @GET
    @Operation(summary = "Search dispatch jobs", description = "Search dispatch jobs with optional filters and pagination. " +
        "Multi-value parameters (clientIds, applications, etc.) support comma-separated values for OR filtering. " +
        "Use 'null' in clientIds to include platform jobs (no client).")
    @APIResponses({
        @APIResponse(
            responseCode = "200",
            description = "Dispatch jobs found",
            content = @Content(mediaType = MediaType.APPLICATION_JSON,
                schema = @Schema(implementation = PagedDispatchJobReadResponse.class))
        )
    })
    public Response searchDispatchJobs(
        @QueryParam("clientIds") String clientIds,
        @QueryParam("statuses") String statuses,
        @QueryParam("applications") String applications,
        @QueryParam("subdomains") String subdomains,
        @QueryParam("aggregates") String aggregates,
        @QueryParam("codes") String codes,
        @QueryParam("source") String source,
        @QueryParam("kind") String kind,
        @QueryParam("subscriptionId") String subscriptionId,
        @QueryParam("dispatchPoolId") String dispatchPoolId,
        @QueryParam("messageGroup") String messageGroup,
        @QueryParam("createdAfter") Instant createdAfter,
        @QueryParam("createdBefore") Instant createdBefore,
        @QueryParam("page") @DefaultValue("0") Integer page,
        @QueryParam("size") @DefaultValue("20") Integer size
    ) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.DISPATCH_JOB_VIEW);

        DispatchJobReadFilter filter = new DispatchJobReadFilter(
            parseCommaSeparated(clientIds),
            parseCommaSeparated(statuses),
            parseCommaSeparated(applications),
            parseCommaSeparated(subdomains),
            parseCommaSeparated(aggregates),
            parseCommaSeparated(codes),
            source, kind, subscriptionId, dispatchPoolId, messageGroup,
            createdAfter, createdBefore, page, size
        );

        List<DispatchJobRead> jobs = dispatchJobReadRepository.findWithFilter(filter);
        long totalCount = dispatchJobReadRepository.countWithFilter(filter);

        List<DispatchJobReadResponse> responses = jobs.stream()
            .map(DispatchJobReadResponse::from)
            .toList();

        return Response.ok(new PagedDispatchJobReadResponse(
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
        "Each level is narrowed by selections at higher levels (client → application → subdomain → aggregate → code).")
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
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.DISPATCH_JOB_VIEW);

        // Get clients from the clients collection (not from dispatch jobs)
        List<FilterOption> clientOptionsList = new ArrayList<>();
        // Add "Platform" option for jobs with no client
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

        // Get job-based filter options (applications, subdomains, aggregates, codes, statuses)
        FilterOptionsRequest request = new FilterOptionsRequest(
            parseCommaSeparated(clientIds),
            parseCommaSeparated(applications),
            parseCommaSeparated(subdomains),
            parseCommaSeparated(aggregates)
        );

        FilterOptions jobOptions = dispatchJobReadRepository.getFilterOptions(request);

        // Build response with clients from collection and other options from jobs
        return Response.ok(new FilterOptionsResponse(
            clientOptionsList,
            jobOptions.applications().stream()
                .filter(a -> a != null && !a.isBlank())
                .map(String::trim)
                .distinct()
                .sorted(String::compareToIgnoreCase)
                .map(a -> new FilterOption(a, a))
                .toList(),
            jobOptions.subdomains().stream()
                .filter(s -> s != null && !s.isBlank())
                .map(String::trim)
                .distinct()
                .sorted(String::compareToIgnoreCase)
                .map(s -> new FilterOption(s, s))
                .toList(),
            jobOptions.aggregates().stream()
                .filter(a -> a != null && !a.isBlank())
                .map(String::trim)
                .distinct()
                .sorted(String::compareToIgnoreCase)
                .map(a -> new FilterOption(a, a))
                .toList(),
            jobOptions.codes().stream()
                .filter(c -> c != null && !c.isBlank())
                .map(String::trim)
                .distinct()
                .sorted(String::compareToIgnoreCase)
                .map(c -> new FilterOption(c, c))
                .toList(),
            jobOptions.statuses().stream()
                .filter(s -> s != null && !s.isBlank())
                .map(String::trim)
                .distinct()
                .sorted(String::compareToIgnoreCase)
                .map(s -> new FilterOption(s, s))
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
    @Operation(summary = "Get dispatch job by ID", description = "Get a single dispatch job by its ID")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Dispatch job found"),
        @APIResponse(responseCode = "404", description = "Dispatch job not found")
    })
    public Response getDispatchJob(@PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.DISPATCH_JOB_VIEW);

        return dispatchJobReadRepository.findByIdOptional(id)
            .map(job -> Response.ok(DispatchJobReadResponse.from(job)).build())
            .orElse(Response.status(404)
                .entity(new ErrorResponse("Dispatch job not found: " + id))
                .build());
    }

    // ========================================================================
    // DTOs
    // ========================================================================

    public record DispatchJobReadResponse(
        String id,
        String externalId,
        String source,
        String kind,
        String code,
        String subject,
        String eventId,
        String correlationId,
        String targetUrl,
        String protocol,
        String clientId,
        String subscriptionId,
        String serviceAccountId,
        String dispatchPoolId,
        String messageGroup,
        String mode,
        Integer sequence,
        String status,
        Integer attemptCount,
        Integer maxRetries,
        String lastError,
        Integer timeoutSeconds,
        String retryStrategy,
        String createdAt,
        String updatedAt,
        String scheduledFor,
        String expiresAt,
        String completedAt,
        String lastAttemptAt,
        Long durationMillis,
        String idempotencyKey,
        Boolean isCompleted,
        Boolean isTerminal,
        String projectedAt
    ) {
        public static DispatchJobReadResponse from(DispatchJobRead job) {
            return new DispatchJobReadResponse(
                job.id,
                job.externalId,
                job.source,
                job.kind,
                job.code,
                job.subject,
                job.eventId,
                job.correlationId,
                job.targetUrl,
                job.protocol,
                job.clientId,
                job.subscriptionId,
                job.serviceAccountId,
                job.dispatchPoolId,
                job.messageGroup,
                job.mode,
                job.sequence,
                job.status,
                job.attemptCount,
                job.maxRetries,
                job.lastError,
                job.timeoutSeconds,
                job.retryStrategy,
                job.createdAt != null ? job.createdAt.toString() : null,
                job.updatedAt != null ? job.updatedAt.toString() : null,
                job.scheduledFor != null ? job.scheduledFor.toString() : null,
                job.expiresAt != null ? job.expiresAt.toString() : null,
                job.completedAt != null ? job.completedAt.toString() : null,
                job.lastAttemptAt != null ? job.lastAttemptAt.toString() : null,
                job.durationMillis,
                job.idempotencyKey,
                job.isCompleted,
                job.isTerminal,
                job.projectedAt != null ? job.projectedAt.toString() : null
            );
        }
    }

    public record PagedDispatchJobReadResponse(
        List<DispatchJobReadResponse> items,
        int page,
        int size,
        long totalItems,
        int totalPages
    ) {}

    /**
     * Filter options for cascading filter UI.
     */
    public record FilterOptionsResponse(
        List<FilterOption> clients,
        List<FilterOption> applications,
        List<FilterOption> subdomains,
        List<FilterOption> aggregates,
        List<FilterOption> codes,
        List<FilterOption> statuses
    ) {}

    /**
     * A filter option with value and display label.
     */
    public record FilterOption(String value, String label) {}

    public record ErrorResponse(String error) {}
}
