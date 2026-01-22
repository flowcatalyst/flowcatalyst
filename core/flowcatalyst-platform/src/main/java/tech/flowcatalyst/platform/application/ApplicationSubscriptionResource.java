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
import tech.flowcatalyst.dispatch.DispatchMode;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.authentication.JwtKeyService;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.TracingContext;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.subscription.*;
import tech.flowcatalyst.subscription.events.SubscriptionsSynced;
import tech.flowcatalyst.subscription.operations.syncsubscriptions.SyncSubscriptionsCommand;

import java.util.List;
import java.util.Map;

/**
 * SDK API for external applications to manage their subscriptions.
 *
 * <p>External applications using the FlowCatalyst SDK can:
 * <ul>
 *   <li>List anchor-level subscriptions</li>
 *   <li>Sync subscriptions (bulk create/update/delete)</li>
 * </ul>
 *
 * <p>Subscriptions are created as anchor-level (not client-scoped) with
 * the application's service account for webhook credentials.
 */
@Path("/api/applications/{appCode}/subscriptions")
@Tag(name = "Application Subscriptions SDK", description = "SDK API for external applications to manage their subscriptions")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
public class ApplicationSubscriptionResource {

    @Inject
    ApplicationRepository applicationRepository;

    @Inject
    SubscriptionOperations subscriptionOperations;

    @Inject
    JwtKeyService jwtKeyService;

    @Inject
    AuditContext auditContext;

    @Inject
    TracingContext tracingContext;

    /**
     * List all anchor-level subscriptions.
     */
    @GET
    @Operation(summary = "List anchor-level subscriptions",
        description = "Returns all anchor-level subscriptions (clientId = null).")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of subscriptions",
            content = @Content(schema = @Schema(implementation = SubscriptionListResponse.class))),
        @APIResponse(responseCode = "404", description = "Application not found")
    })
    public Response listSubscriptions(
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

        Application app = applicationRepository.findByCode(appCode).orElse(null);
        if (app == null) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("APPLICATION_NOT_FOUND", "Application not found: " + appCode))
                .build();
        }

        // Get anchor-level subscriptions
        List<Subscription> subscriptions = subscriptionOperations.findAnchorLevel();

        // Filter by source if provided
        if (source != null && !source.isBlank()) {
            try {
                SubscriptionSource sourceEnum = SubscriptionSource.valueOf(source.toUpperCase());
                subscriptions = subscriptions.stream()
                    .filter(s -> s.source() == sourceEnum)
                    .toList();
            } catch (IllegalArgumentException e) {
                return Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ErrorResponse("INVALID_SOURCE", "Invalid source. Must be API or UI"))
                    .build();
            }
        }

        List<SubscriptionDto> dtos = subscriptions.stream()
            .map(this::toSubscriptionDto)
            .toList();

        return Response.ok(new SubscriptionListResponse(dtos, dtos.size())).build();
    }

    /**
     * Sync subscriptions from an external application.
     */
    @POST
    @Path("/sync")
    @Operation(summary = "Sync application subscriptions",
        description = "Bulk sync subscriptions from an external application. " +
                      "Creates new subscriptions, updates existing API-sourced subscriptions. " +
                      "Set removeUnlisted=true to remove API-sourced subscriptions not in the sync list.")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Sync complete",
            content = @Content(schema = @Schema(implementation = SyncResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid request"),
        @APIResponse(responseCode = "404", description = "Application not found")
    })
    public Response syncSubscriptions(
            @PathParam("appCode") String appCode,
            @QueryParam("removeUnlisted") @DefaultValue("false") boolean removeUnlisted,
            SyncSubscriptionsRequest request,
            @CookieParam("fc_session") String sessionToken,
            @HeaderParam("Authorization") String authHeader) {

        var principalIdOpt = jwtKeyService.extractAndValidatePrincipalId(sessionToken, authHeader);
        if (principalIdOpt.isEmpty()) {
            return Response.status(Response.Status.UNAUTHORIZED)
                .entity(new ErrorResponse("UNAUTHORIZED", "Not authenticated"))
                .build();
        }
        String principalId = principalIdOpt.get();

        Application app = applicationRepository.findByCode(appCode).orElse(null);
        if (app == null) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("APPLICATION_NOT_FOUND", "Application not found: " + appCode))
                .build();
        }

        if (request.subscriptions() == null || request.subscriptions().isEmpty()) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse("SUBSCRIPTIONS_REQUIRED", "subscriptions list is required"))
                .build();
        }

        // Set audit context and create execution context
        auditContext.setPrincipalId(principalId);
        ExecutionContext context = ExecutionContext.from(tracingContext, principalId);

        // Convert request to internal format
        List<SyncSubscriptionsCommand.SyncSubscriptionItem> items = request.subscriptions().stream()
            .map(this::toSyncItem)
            .toList();

        SyncSubscriptionsCommand command = new SyncSubscriptionsCommand(appCode, items, removeUnlisted);
        Result<SubscriptionsSynced> result = subscriptionOperations.syncSubscriptions(command, context);

        return switch (result) {
            case Result.Success<SubscriptionsSynced> s -> {
                // Return updated subscription list
                List<Subscription> subscriptions = subscriptionOperations.findAnchorLevel();
                List<SubscriptionDto> dtos = subscriptions.stream()
                    .filter(sub -> sub.source() == SubscriptionSource.API)
                    .map(this::toSubscriptionDto)
                    .toList();
                yield Response.ok(new SyncResponse(
                    s.value().subscriptionsCreated(),
                    s.value().subscriptionsUpdated(),
                    s.value().subscriptionsDeleted(),
                    dtos
                )).build();
            }
            case Result.Failure<SubscriptionsSynced> f -> mapErrorToResponse(f.error());
        };
    }

    // ==================== Helper Methods ====================

    private SyncSubscriptionsCommand.SyncSubscriptionItem toSyncItem(SyncSubscriptionItem item) {
        List<SyncSubscriptionsCommand.EventTypeBindingItem> bindings = null;
        if (item.eventTypes() != null) {
            bindings = item.eventTypes().stream()
                .map(b -> new SyncSubscriptionsCommand.EventTypeBindingItem(
                    b.eventTypeCode(),
                    b.specVersion()
                ))
                .toList();
        }

        return new SyncSubscriptionsCommand.SyncSubscriptionItem(
            item.code(),
            item.name(),
            item.description(),
            bindings,
            item.target(),
            item.queue(),
            item.customConfig(),
            item.maxAgeSeconds(),
            item.dispatchPoolCode(),
            item.delaySeconds(),
            item.sequence(),
            item.mode(),
            item.timeoutSeconds(),
            item.maxRetries(),
            item.dataOnly()
        );
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

    private SubscriptionDto toSubscriptionDto(Subscription sub) {
        List<EventTypeBindingDto> eventTypes = sub.eventTypes() != null
            ? sub.eventTypes().stream()
                .map(b -> new EventTypeBindingDto(b.eventTypeId(), b.eventTypeCode(), b.specVersion()))
                .toList()
            : List.of();

        return new SubscriptionDto(
            sub.id(),
            sub.code(),
            sub.name(),
            sub.description(),
            eventTypes,
            sub.target(),
            sub.queue(),
            sub.customConfig(),
            sub.source() != null ? sub.source().name() : "API",
            sub.status() != null ? sub.status().name() : "ACTIVE",
            sub.maxAgeSeconds(),
            sub.dispatchPoolId(),
            sub.dispatchPoolCode(),
            sub.delaySeconds(),
            sub.sequence(),
            sub.mode(),
            sub.timeoutSeconds(),
            sub.maxRetries(),
            sub.serviceAccountId(),
            sub.dataOnly()
        );
    }

    // ==================== DTOs ====================

    public record SubscriptionDto(
        String id,
        String code,
        String name,
        String description,
        List<EventTypeBindingDto> eventTypes,
        String target,
        String queue,
        List<ConfigEntry> customConfig,
        String source,
        String status,
        int maxAgeSeconds,
        String dispatchPoolId,
        String dispatchPoolCode,
        int delaySeconds,
        int sequence,
        DispatchMode mode,
        int timeoutSeconds,
        int maxRetries,
        String serviceAccountId,
        boolean dataOnly
    ) {}

    public record EventTypeBindingDto(
        String eventTypeId,
        String eventTypeCode,
        String specVersion
    ) {}

    public record SubscriptionListResponse(
        List<SubscriptionDto> subscriptions,
        int total
    ) {}

    public record SyncSubscriptionItem(
        String code,
        String name,
        String description,
        List<EventTypeBindingItem> eventTypes,
        String target,
        String queue,
        List<ConfigEntry> customConfig,
        Integer maxAgeSeconds,
        String dispatchPoolCode,
        Integer delaySeconds,
        Integer sequence,
        DispatchMode mode,
        Integer timeoutSeconds,
        Integer maxRetries,
        Boolean dataOnly
    ) {}

    public record EventTypeBindingItem(
        String eventTypeCode,
        String specVersion
    ) {}

    public record SyncSubscriptionsRequest(
        List<SyncSubscriptionItem> subscriptions
    ) {}

    public record SyncResponse(
        int created,
        int updated,
        int deleted,
        List<SubscriptionDto> subscriptions
    ) {}

    public record ErrorResponse(String code, String message, Map<String, Object> details) {
        public ErrorResponse(String code, String message) {
            this(code, message, Map.of());
        }
    }
}
