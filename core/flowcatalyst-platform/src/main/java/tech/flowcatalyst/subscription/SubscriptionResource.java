package tech.flowcatalyst.subscription;

import jakarta.inject.Inject;
import jakarta.validation.Valid;
import jakarta.validation.constraints.Min;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotEmpty;
import jakarta.validation.constraints.Size;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.*;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.media.Content;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.eclipse.microprofile.openapi.annotations.parameters.Parameter;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponses;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import org.jboss.logging.Logger;
import tech.flowcatalyst.dispatch.DispatchMode;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.authentication.EmbeddedModeOnly;
import tech.flowcatalyst.platform.authorization.AuthorizationService;
import tech.flowcatalyst.platform.authorization.platform.PlatformMessagingPermissions;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.subscription.events.SubscriptionCreated;
import tech.flowcatalyst.subscription.events.SubscriptionDeleted;
import tech.flowcatalyst.subscription.events.SubscriptionUpdated;
import tech.flowcatalyst.subscription.operations.createsubscription.CreateSubscriptionCommand;
import tech.flowcatalyst.subscription.operations.deletesubscription.DeleteSubscriptionCommand;
import tech.flowcatalyst.subscription.operations.updatesubscription.UpdateSubscriptionCommand;

import java.time.Instant;
import java.util.List;

/**
 * Admin API for subscription management.
 *
 * Provides CRUD operations for subscriptions including:
 * - Create, read, update, delete subscriptions
 * - Status management (pause, resume)
 * - Filtering by client, status, source, dispatch pool
 *
 * All operations require messaging-level permissions.
 */
@Path("/api/admin/subscriptions")
@Tag(name = "BFF - Subscription Admin", description = "Administrative operations for subscription management")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@EmbeddedModeOnly
public class SubscriptionResource {

    private static final Logger LOG = Logger.getLogger(SubscriptionResource.class);

    @Inject
    SubscriptionOperations subscriptionOperations;

    @Inject
    AuditContext auditContext;

    @Inject
    AuthorizationService authorizationService;

    // ==================== List & Get ====================

    /**
     * List subscriptions with optional filters.
     */
    @GET
    @Operation(summary = "List subscriptions", description = "Returns subscriptions with optional filtering")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of subscriptions",
            content = @Content(schema = @Schema(implementation = SubscriptionListResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response listSubscriptions(
            @QueryParam("clientId") @Parameter(description = "Filter by client ID") String clientId,
            @QueryParam("status") @Parameter(description = "Filter by status") SubscriptionStatus status,
            @QueryParam("source") @Parameter(description = "Filter by source") SubscriptionSource source,
            @QueryParam("dispatchPoolId") @Parameter(description = "Filter by dispatch pool") String dispatchPoolId,
            @QueryParam("anchorLevel") @Parameter(description = "If true, return only anchor-level subscriptions") Boolean anchorLevel) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.SUBSCRIPTION_VIEW);

        List<Subscription> subscriptions;
        if (Boolean.TRUE.equals(anchorLevel)) {
            subscriptions = subscriptionOperations.findAnchorLevel();
            if (status != null) {
                subscriptions = subscriptions.stream().filter(s -> s.status() == status).toList();
            }
            if (source != null) {
                subscriptions = subscriptions.stream().filter(s -> s.source() == source).toList();
            }
            if (dispatchPoolId != null) {
                subscriptions = subscriptions.stream().filter(s -> dispatchPoolId.equals(s.dispatchPoolId())).toList();
            }
        } else {
            subscriptions = subscriptionOperations.findWithFilters(clientId, status, source, dispatchPoolId);
        }

        List<SubscriptionDto> dtos = subscriptions.stream()
            .map(this::toDto)
            .toList();

        return Response.ok(new SubscriptionListResponse(dtos, dtos.size())).build();
    }

    /**
     * Get a specific subscription by ID.
     */
    @GET
    @Path("/{id}")
    @Operation(summary = "Get subscription by ID")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Subscription details",
            content = @Content(schema = @Schema(implementation = SubscriptionDto.class))),
        @APIResponse(responseCode = "404", description = "Subscription not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response getSubscription(@PathParam("id") String id) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.SUBSCRIPTION_VIEW);

        return subscriptionOperations.findById(id)
            .map(subscription -> Response.ok(toDto(subscription)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("Subscription not found"))
                .build());
    }

    // ==================== Create ====================

    /**
     * Create a new subscription.
     */
    @POST
    @Operation(summary = "Create a new subscription")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Subscription created",
            content = @Content(schema = @Schema(implementation = SubscriptionDto.class))),
        @APIResponse(responseCode = "400", description = "Invalid request or code already exists"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response createSubscription(@Valid CreateSubscriptionRequest request, @Context UriInfo uriInfo) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.SUBSCRIPTION_CREATE);

        CreateSubscriptionCommand command = new CreateSubscriptionCommand(
            request.code(),
            request.name(),
            request.description(),
            request.clientId(),
            request.eventTypes(),
            request.target(),
            request.queue(),
            request.customConfig(),
            request.source(),
            request.maxAgeSeconds(),
            request.dispatchPoolId(),
            request.delaySeconds(),
            request.sequence(),
            request.mode(),
            request.timeoutSeconds(),
            request.maxRetries(),
            request.serviceAccountId(),
            request.dataOnly()
        );

        ExecutionContext ctx = ExecutionContext.create(principalId);
        Result<SubscriptionCreated> result = subscriptionOperations.createSubscription(command, ctx);

        if (result instanceof Result.Success<SubscriptionCreated> success) {
            SubscriptionCreated event = success.value();
            LOG.infof("Subscription created: %s (%s) by principal %s",
                event.subscriptionId(), event.code(), principalId);

            return subscriptionOperations.findById(event.subscriptionId())
                .map(sub -> Response.status(Response.Status.CREATED)
                    .entity(toDto(sub))
                    .location(uriInfo.getAbsolutePathBuilder().path(sub.id()).build())
                    .build())
                .orElse(Response.status(Response.Status.CREATED)
                    .entity(new StatusResponse("Subscription created", event.subscriptionId()))
                    .build());
        } else {
            Result.Failure<SubscriptionCreated> failure = (Result.Failure<SubscriptionCreated>) result;
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse(failure.error().message()))
                .build();
        }
    }

    // ==================== Update ====================

    /**
     * Update a subscription.
     */
    @PUT
    @Path("/{id}")
    @Operation(summary = "Update subscription")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Subscription updated",
            content = @Content(schema = @Schema(implementation = SubscriptionDto.class))),
        @APIResponse(responseCode = "404", description = "Subscription not found"),
        @APIResponse(responseCode = "400", description = "Invalid request"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response updateSubscription(@PathParam("id") String id, @Valid UpdateSubscriptionRequest request) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.SUBSCRIPTION_UPDATE);

        UpdateSubscriptionCommand command = new UpdateSubscriptionCommand(
            id,
            request.name(),
            request.description(),
            request.eventTypes(),
            request.target(),
            request.queue(),
            request.customConfig(),
            request.status(),
            request.maxAgeSeconds(),
            request.dispatchPoolId(),
            request.delaySeconds(),
            request.sequence(),
            request.mode(),
            request.timeoutSeconds(),
            request.maxRetries(),
            request.serviceAccountId(),
            request.dataOnly()
        );

        ExecutionContext ctx = ExecutionContext.create(principalId);
        Result<SubscriptionUpdated> result = subscriptionOperations.updateSubscription(command, ctx);

        if (result instanceof Result.Success<SubscriptionUpdated> success) {
            LOG.infof("Subscription updated: %s by principal %s", id, principalId);

            return subscriptionOperations.findById(id)
                .map(sub -> Response.ok(toDto(sub)).build())
                .orElse(Response.ok(new StatusResponse("Subscription updated", id)).build());
        } else {
            Result.Failure<SubscriptionUpdated> failure = (Result.Failure<SubscriptionUpdated>) result;
            String errorCode = failure.error().code();
            if ("SUBSCRIPTION_NOT_FOUND".equals(errorCode)) {
                return Response.status(Response.Status.NOT_FOUND)
                    .entity(new ErrorResponse(failure.error().message()))
                    .build();
            }
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse(failure.error().message()))
                .build();
        }
    }

    // ==================== Delete ====================

    /**
     * Delete a subscription.
     */
    @DELETE
    @Path("/{id}")
    @Operation(summary = "Delete subscription")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Subscription deleted"),
        @APIResponse(responseCode = "404", description = "Subscription not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response deleteSubscription(@PathParam("id") String id) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.SUBSCRIPTION_DELETE);

        DeleteSubscriptionCommand command = new DeleteSubscriptionCommand(id);
        ExecutionContext ctx = ExecutionContext.create(principalId);
        Result<SubscriptionDeleted> result = subscriptionOperations.deleteSubscription(command, ctx);

        if (result instanceof Result.Success<SubscriptionDeleted>) {
            LOG.infof("Subscription deleted: %s by principal %s", id, principalId);
            return Response.ok(new StatusResponse("Subscription deleted", id)).build();
        } else {
            Result.Failure<SubscriptionDeleted> failure = (Result.Failure<SubscriptionDeleted>) result;
            String errorCode = failure.error().code();
            if ("SUBSCRIPTION_NOT_FOUND".equals(errorCode)) {
                return Response.status(Response.Status.NOT_FOUND)
                    .entity(new ErrorResponse(failure.error().message()))
                    .build();
            }
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse(failure.error().message()))
                .build();
        }
    }

    // ==================== Status Management ====================

    /**
     * Pause a subscription.
     */
    @POST
    @Path("/{id}/pause")
    @Operation(summary = "Pause subscription", description = "Stop creating dispatch jobs for this subscription")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Subscription paused"),
        @APIResponse(responseCode = "404", description = "Subscription not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response pauseSubscription(@PathParam("id") String id) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.SUBSCRIPTION_UPDATE);

        UpdateSubscriptionCommand command = new UpdateSubscriptionCommand(
            id, null, null, null, null, null, null,
            SubscriptionStatus.PAUSED, null, null, null, null, null, null, null, null, null
        );

        ExecutionContext ctx = ExecutionContext.create(principalId);
        Result<SubscriptionUpdated> result = subscriptionOperations.updateSubscription(command, ctx);

        if (result instanceof Result.Success<SubscriptionUpdated>) {
            LOG.infof("Subscription paused: %s by principal %s", id, principalId);
            return Response.ok(new StatusResponse("Subscription paused", id)).build();
        } else {
            Result.Failure<SubscriptionUpdated> failure = (Result.Failure<SubscriptionUpdated>) result;
            String errorCode = failure.error().code();
            if ("SUBSCRIPTION_NOT_FOUND".equals(errorCode)) {
                return Response.status(Response.Status.NOT_FOUND)
                    .entity(new ErrorResponse(failure.error().message()))
                    .build();
            }
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse(failure.error().message()))
                .build();
        }
    }

    /**
     * Resume a subscription.
     */
    @POST
    @Path("/{id}/resume")
    @Operation(summary = "Resume subscription", description = "Re-enable dispatch job creation")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Subscription resumed"),
        @APIResponse(responseCode = "404", description = "Subscription not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response resumeSubscription(@PathParam("id") String id) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.SUBSCRIPTION_UPDATE);

        UpdateSubscriptionCommand command = new UpdateSubscriptionCommand(
            id, null, null, null, null, null, null,
            SubscriptionStatus.ACTIVE, null, null, null, null, null, null, null, null, null
        );

        ExecutionContext ctx = ExecutionContext.create(principalId);
        Result<SubscriptionUpdated> result = subscriptionOperations.updateSubscription(command, ctx);

        if (result instanceof Result.Success<SubscriptionUpdated>) {
            LOG.infof("Subscription resumed: %s by principal %s", id, principalId);
            return Response.ok(new StatusResponse("Subscription resumed", id)).build();
        } else {
            Result.Failure<SubscriptionUpdated> failure = (Result.Failure<SubscriptionUpdated>) result;
            String errorCode = failure.error().code();
            if ("SUBSCRIPTION_NOT_FOUND".equals(errorCode)) {
                return Response.status(Response.Status.NOT_FOUND)
                    .entity(new ErrorResponse(failure.error().message()))
                    .build();
            }
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse(failure.error().message()))
                .build();
        }
    }

    // ==================== Helper Methods ====================

    private SubscriptionDto toDto(Subscription subscription) {
        return new SubscriptionDto(
            subscription.id(),
            subscription.code(),
            subscription.name(),
            subscription.description(),
            subscription.clientId(),
            subscription.clientIdentifier(),
            subscription.eventTypes(),
            subscription.target(),
            subscription.queue(),
            subscription.customConfig(),
            subscription.source(),
            subscription.status(),
            subscription.maxAgeSeconds(),
            subscription.dispatchPoolId(),
            subscription.dispatchPoolCode(),
            subscription.delaySeconds(),
            subscription.sequence(),
            subscription.mode(),
            subscription.timeoutSeconds(),
            subscription.maxRetries(),
            subscription.serviceAccountId(),
            subscription.dataOnly(),
            subscription.createdAt(),
            subscription.updatedAt()
        );
    }

    // ==================== DTOs ====================

    public record SubscriptionDto(
        String id,
        String code,
        String name,
        String description,
        String clientId,
        String clientIdentifier,
        List<EventTypeBinding> eventTypes,
        String target,
        String queue,
        List<ConfigEntry> customConfig,
        SubscriptionSource source,
        SubscriptionStatus status,
        int maxAgeSeconds,
        String dispatchPoolId,
        String dispatchPoolCode,
        int delaySeconds,
        int sequence,
        DispatchMode mode,
        int timeoutSeconds,
        int maxRetries,
        String serviceAccountId,
        boolean dataOnly,
        Instant createdAt,
        Instant updatedAt
    ) {}

    public record SubscriptionListResponse(
        List<SubscriptionDto> subscriptions,
        int total
    ) {}

    public record CreateSubscriptionRequest(
        @NotBlank(message = "Code is required")
        @Size(min = 2, max = 100, message = "Code must be 2-100 characters")
        String code,

        @NotBlank(message = "Name is required")
        @Size(max = 255, message = "Name must be less than 255 characters")
        String name,

        String description,

        String clientId,

        @NotEmpty(message = "At least one event type is required")
        List<EventTypeBinding> eventTypes,

        @NotBlank(message = "Target URL is required")
        String target,

        @NotBlank(message = "Queue name is required")
        String queue,

        List<ConfigEntry> customConfig,

        SubscriptionSource source,

        @Min(value = 1, message = "Max age must be at least 1 second")
        Integer maxAgeSeconds,

        @NotBlank(message = "Dispatch pool ID is required")
        String dispatchPoolId,

        @Min(value = 0, message = "Delay cannot be negative")
        Integer delaySeconds,

        @Min(value = 1, message = "Sequence must be at least 1")
        Integer sequence,

        DispatchMode mode,

        @Min(value = 1, message = "Timeout must be at least 1 second")
        Integer timeoutSeconds,

        @Min(value = 0, message = "Max retries cannot be negative")
        Integer maxRetries,

        @NotBlank(message = "Service account ID is required")
        String serviceAccountId,

        Boolean dataOnly
    ) {}

    public record UpdateSubscriptionRequest(
        @Size(max = 255, message = "Name must be less than 255 characters")
        String name,

        String description,

        List<EventTypeBinding> eventTypes,

        String target,

        String queue,

        List<ConfigEntry> customConfig,

        SubscriptionStatus status,

        @Min(value = 1, message = "Max age must be at least 1 second")
        Integer maxAgeSeconds,

        String dispatchPoolId,

        @Min(value = 0, message = "Delay cannot be negative")
        Integer delaySeconds,

        @Min(value = 1, message = "Sequence must be at least 1")
        Integer sequence,

        DispatchMode mode,

        @Min(value = 1, message = "Timeout must be at least 1 second")
        Integer timeoutSeconds,

        @Min(value = 0, message = "Max retries cannot be negative")
        Integer maxRetries,

        String serviceAccountId,

        Boolean dataOnly
    ) {}

    public record StatusResponse(
        String message,
        String subscriptionId
    ) {}

    public record ErrorResponse(
        String error
    ) {}
}
