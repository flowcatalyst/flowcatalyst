package tech.flowcatalyst.serviceaccount.api;

import jakarta.inject.Inject;
import jakarta.validation.Valid;
import jakarta.validation.constraints.NotBlank;
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
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.authentication.EmbeddedModeOnly;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.TracingContext;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.serviceaccount.entity.ServiceAccount;
import tech.flowcatalyst.serviceaccount.entity.WebhookAuthType;
import tech.flowcatalyst.serviceaccount.operations.ServiceAccountOperations;
import tech.flowcatalyst.serviceaccount.operations.assignroles.RolesAssigned;
import tech.flowcatalyst.serviceaccount.operations.createserviceaccount.CreateServiceAccountCommand;
import tech.flowcatalyst.serviceaccount.operations.createserviceaccount.CreateServiceAccountResult;
import tech.flowcatalyst.serviceaccount.operations.deleteserviceaccount.ServiceAccountDeleted;
import tech.flowcatalyst.serviceaccount.operations.regenerateauthtoken.RegenerateAuthTokenResult;
import tech.flowcatalyst.serviceaccount.operations.regeneratesigningsecret.RegenerateSigningSecretResult;
import tech.flowcatalyst.serviceaccount.operations.updateserviceaccount.ServiceAccountUpdated;
import tech.flowcatalyst.serviceaccount.operations.updateserviceaccount.UpdateServiceAccountCommand;
import tech.flowcatalyst.serviceaccount.repository.ServiceAccountFilter;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TypedId;

import java.time.Instant;
import java.util.List;

/**
 * Admin API for service account management.
 *
 * Provides CRUD operations for service accounts including:
 * - Create, read, update, delete service accounts
 * - Credential management (regenerate token, regenerate secret)
 * - Role assignments
 */
@Path("/api/admin/service-accounts")
@Tag(name = "BFF - Service Account Admin", description = "Administrative operations for service account management")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@EmbeddedModeOnly
public class ServiceAccountAdminResource {

    private static final Logger LOG = Logger.getLogger(ServiceAccountAdminResource.class);

    @Inject
    ServiceAccountOperations operations;

    @Inject
    AuditContext auditContext;

    @Inject
    TracingContext tracingContext;

    // ==================== CRUD Operations ====================

    @GET
    @Operation(summary = "List service accounts", description = "List all service accounts with optional filters")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of service accounts",
            content = @Content(schema = @Schema(implementation = ServiceAccountListResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated")
    })
    public Response list(
            @QueryParam("clientId") @Parameter(description = "Filter by client ID") String clientId,
            @QueryParam("applicationId") @Parameter(description = "Filter by application ID") String applicationId,
            @QueryParam("active") @Parameter(description = "Filter by active status") Boolean active) {

        auditContext.requirePrincipalId();

        ServiceAccountFilter filter = new ServiceAccountFilter(clientId, active, applicationId);
        List<ServiceAccount> accounts = operations.findWithFilter(filter);

        List<ServiceAccountDto> dtos = accounts.stream()
            .map(this::toDto)
            .toList();

        return Response.ok(new ServiceAccountListResponse(dtos, dtos.size())).build();
    }

    @GET
    @Path("/{id}")
    @Operation(summary = "Get service account by ID")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Service account details",
            content = @Content(schema = @Schema(implementation = ServiceAccountDto.class))),
        @APIResponse(responseCode = "404", description = "Service account not found")
    })
    public Response getById(@PathParam("id") String id) {

        auditContext.requirePrincipalId();

        return operations.findById(id)
            .map(sa -> Response.ok(toDto(sa)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("Service account not found"))
                .build());
    }

    @GET
    @Path("/code/{code}")
    @Operation(summary = "Get service account by code")
    public Response getByCode(@PathParam("code") String code) {

        auditContext.requirePrincipalId();

        return operations.findByCode(code)
            .map(sa -> Response.ok(toDto(sa)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("Service account not found"))
                .build());
    }

    @POST
    @Operation(summary = "Create a new service account",
        description = "Creates a service account and returns credentials. Credentials are shown only once.")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Service account created",
            content = @Content(schema = @Schema(implementation = CreateServiceAccountResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid request or code already exists")
    })
    public Response create(
            @Valid CreateServiceAccountRequest request,
            @Context UriInfo uriInfo) {

        String adminPrincipalId = auditContext.requirePrincipalId();
        ExecutionContext context = ExecutionContext.from(tracingContext, adminPrincipalId);

        CreateServiceAccountCommand command = new CreateServiceAccountCommand(
            request.code(),
            request.name(),
            request.description(),
            request.clientIds(),
            request.applicationId()
        );

        CreateServiceAccountResult createResult = operations.create(command, context);

        return switch (createResult.result()) {
            case Result.Success<?> s -> {
                LOG.infof("Service account created: %s by principal %s", request.code(), adminPrincipalId);
                yield Response.status(Response.Status.CREATED)
                    .entity(new CreateServiceAccountResponse(
                        toDto(createResult.serviceAccount()),
                        createResult.authToken(),
                        createResult.signingSecret()
                    ))
                    .location(uriInfo.getBaseUriBuilder()
                        .path(ServiceAccountAdminResource.class)
                        .path(createResult.serviceAccount().id)
                        .build())
                    .build();
            }
            case Result.Failure<?> f -> mapErrorToResponse(f.error());
        };
    }

    @PUT
    @Path("/{id}")
    @Operation(summary = "Update service account metadata")
    public Response update(
            @PathParam("id") String id,
            @Valid UpdateServiceAccountRequest request) {

        String adminPrincipalId = auditContext.requirePrincipalId();
        ExecutionContext context = ExecutionContext.from(tracingContext, adminPrincipalId);

        UpdateServiceAccountCommand command = new UpdateServiceAccountCommand(
            id,
            request.name(),
            request.description(),
            request.clientIds()
        );

        Result<ServiceAccountUpdated> result = operations.update(command, context);

        return switch (result) {
            case Result.Success<ServiceAccountUpdated> s -> {
                ServiceAccount sa = operations.findById(id).orElseThrow();
                yield Response.ok(toDto(sa)).build();
            }
            case Result.Failure<ServiceAccountUpdated> f -> mapErrorToResponse(f.error());
        };
    }

    @DELETE
    @Path("/{id}")
    @Operation(summary = "Delete service account")
    public Response delete(@PathParam("id") String id) {

        String adminPrincipalId = auditContext.requirePrincipalId();
        ExecutionContext context = ExecutionContext.from(tracingContext, adminPrincipalId);

        Result<ServiceAccountDeleted> result = operations.delete(id, context);

        return switch (result) {
            case Result.Success<?> s -> Response.noContent().build();
            case Result.Failure<?> f -> mapErrorToResponse(f.error());
        };
    }

    // ==================== Credential Management ====================

    @PUT
    @Path("/{id}/auth-token")
    @Operation(summary = "Update auth token", description = "Replace the auth token with a custom value")
    public Response updateAuthToken(
            @PathParam("id") String id,
            @Valid UpdateAuthTokenRequest request) {

        String adminPrincipalId = auditContext.requirePrincipalId();
        ExecutionContext context = ExecutionContext.from(tracingContext, adminPrincipalId);

        RegenerateAuthTokenResult result = operations.updateAuthToken(id, request.authToken(), context);

        return switch (result.result()) {
            case Result.Success<?> s -> {
                ServiceAccount sa = operations.findById(id).orElseThrow();
                yield Response.ok(toDto(sa)).build();
            }
            case Result.Failure<?> f -> mapErrorToResponse(f.error());
        };
    }

    @POST
    @Path("/{id}/regenerate-token")
    @Operation(summary = "Regenerate auth token", description = "Generate a new random auth token. Returns the new token (shown only once).")
    public Response regenerateToken(@PathParam("id") String id) {

        String adminPrincipalId = auditContext.requirePrincipalId();
        ExecutionContext context = ExecutionContext.from(tracingContext, adminPrincipalId);

        RegenerateAuthTokenResult result = operations.regenerateAuthToken(id, context);

        return switch (result.result()) {
            case Result.Success<?> s -> Response.ok(new RegenerateTokenResponse(result.authToken())).build();
            case Result.Failure<?> f -> mapErrorToResponse(f.error());
        };
    }

    @POST
    @Path("/{id}/regenerate-secret")
    @Operation(summary = "Regenerate signing secret", description = "Generate a new signing secret. Returns the new secret (shown only once).")
    public Response regenerateSecret(@PathParam("id") String id) {

        String adminPrincipalId = auditContext.requirePrincipalId();
        ExecutionContext context = ExecutionContext.from(tracingContext, adminPrincipalId);

        RegenerateSigningSecretResult result = operations.regenerateSigningSecret(id, context);

        return switch (result.result()) {
            case Result.Success<?> s -> Response.ok(new RegenerateSecretResponse(result.signingSecret())).build();
            case Result.Failure<?> f -> mapErrorToResponse(f.error());
        };
    }

    // ==================== Role Management ====================

    @GET
    @Path("/{id}/roles")
    @Operation(summary = "Get assigned roles")
    public Response getRoles(@PathParam("id") String id) {

        auditContext.requirePrincipalId();

        return operations.findById(id)
            .map(sa -> {
                List<RoleAssignmentDto> roles = sa.roles.stream()
                    .map(r -> new RoleAssignmentDto(r.roleName, r.assignmentSource, r.assignedAt))
                    .toList();
                return Response.ok(new RolesResponse(roles)).build();
            })
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("Service account not found"))
                .build());
    }

    @PUT
    @Path("/{id}/roles")
    @Operation(summary = "Assign roles", description = "Replace all roles with the provided list (declarative assignment)")
    public Response assignRoles(
            @PathParam("id") String id,
            @Valid AssignRolesRequest request) {

        String adminPrincipalId = auditContext.requirePrincipalId();
        ExecutionContext context = ExecutionContext.from(tracingContext, adminPrincipalId);

        Result<RolesAssigned> result = operations.assignRoles(id, request.roles(), context);

        return switch (result) {
            case Result.Success<RolesAssigned> s -> {
                ServiceAccount sa = operations.findById(id).orElseThrow();
                List<RoleAssignmentDto> roles = sa.roles.stream()
                    .map(r -> new RoleAssignmentDto(r.roleName, r.assignmentSource, r.assignedAt))
                    .toList();
                yield Response.ok(new RolesAssignedResponse(roles, s.value().addedRoles(), s.value().removedRoles())).build();
            }
            case Result.Failure<RolesAssigned> f -> mapErrorToResponse(f.error());
        };
    }

    // ==================== DTOs ====================

    private ServiceAccountDto toDto(ServiceAccount sa) {
        return new ServiceAccountDto(
            TypedId.Ops.serialize(EntityType.PRINCIPAL, sa.id),
            sa.code,
            sa.name,
            sa.description,
            sa.clientIds != null
                ? sa.clientIds.stream()
                    .map(id -> TypedId.Ops.serialize(EntityType.CLIENT, id))
                    .toList()
                : List.of(),
            TypedId.Ops.serialize(EntityType.APPLICATION, sa.applicationId),
            sa.active,
            sa.webhookCredentials != null ? sa.webhookCredentials.authType : null,
            sa.roles.stream().map(r -> r.roleName).toList(),
            sa.lastUsedAt,
            sa.createdAt,
            sa.updatedAt
        );
    }

    private Response mapErrorToResponse(UseCaseError error) {
        return switch (error) {
            case UseCaseError.ValidationError e -> Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse(e.message(), e.code(), e.details()))
                .build();
            case UseCaseError.BusinessRuleViolation e -> Response.status(Response.Status.CONFLICT)
                .entity(new ErrorResponse(e.message(), e.code(), e.details()))
                .build();
            case UseCaseError.NotFoundError e -> Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse(e.message(), e.code(), e.details()))
                .build();
            case UseCaseError.ConcurrencyError e -> Response.status(Response.Status.CONFLICT)
                .entity(new ErrorResponse(e.message(), e.code(), e.details()))
                .build();
            case UseCaseError.AuthorizationError e -> Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse(e.message(), e.code(), e.details()))
                .build();
            default -> Response.status(Response.Status.INTERNAL_SERVER_ERROR)
                .entity(new ErrorResponse("Internal error"))
                .build();
        };
    }

    // ==================== Request/Response Records ====================

    public record CreateServiceAccountRequest(
        @NotBlank @Size(max = 50) String code,
        @NotBlank @Size(max = 100) String name,
        @Size(max = 500) String description,
        List<String> clientIds,
        String applicationId
    ) {}

    public record UpdateServiceAccountRequest(
        @Size(max = 100) String name,
        @Size(max = 500) String description,
        List<String> clientIds
    ) {}

    public record UpdateAuthTokenRequest(
        @NotBlank String authToken
    ) {}

    public record AssignRolesRequest(
        List<String> roles
    ) {}

    public record ServiceAccountDto(
        String id,
        String code,
        String name,
        String description,
        List<String> clientIds,
        String applicationId,
        boolean active,
        WebhookAuthType authType,
        List<String> roles,
        Instant lastUsedAt,
        Instant createdAt,
        Instant updatedAt
    ) {}

    public record ServiceAccountListResponse(
        List<ServiceAccountDto> serviceAccounts,
        int total
    ) {}

    public record CreateServiceAccountResponse(
        ServiceAccountDto serviceAccount,
        String authToken,
        String signingSecret
    ) {}

    public record RegenerateTokenResponse(String authToken) {}

    public record RegenerateSecretResponse(String signingSecret) {}

    public record RoleAssignmentDto(
        String roleName,
        String assignmentSource,
        Instant assignedAt
    ) {}

    public record RolesResponse(List<RoleAssignmentDto> roles) {}

    public record RolesAssignedResponse(
        List<RoleAssignmentDto> roles,
        List<String> addedRoles,
        List<String> removedRoles
    ) {}

    public record ErrorResponse(String message, String code, Object details) {
        public ErrorResponse(String message) {
            this(message, null, null);
        }
    }
}
