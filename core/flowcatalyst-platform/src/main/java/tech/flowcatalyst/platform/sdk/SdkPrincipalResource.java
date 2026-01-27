package tech.flowcatalyst.platform.sdk;

import jakarta.inject.Inject;
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
import tech.flowcatalyst.platform.authentication.IdpType;
import tech.flowcatalyst.platform.authentication.JwtKeyService;
import tech.flowcatalyst.platform.authorization.AuthorizationService;
import tech.flowcatalyst.platform.authorization.PrincipalRole;
import tech.flowcatalyst.platform.authorization.RoleService;
import tech.flowcatalyst.platform.authorization.platform.PlatformIamPermissions;
import tech.flowcatalyst.platform.client.ClientAccessGrant;
import tech.flowcatalyst.platform.client.ClientAccessGrantRepository;
import tech.flowcatalyst.platform.client.ClientAccessService;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.TracingContext;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.principal.*;
import tech.flowcatalyst.platform.principal.events.*;
import tech.flowcatalyst.platform.principal.operations.activateuser.ActivateUserCommand;
import tech.flowcatalyst.platform.principal.operations.createuser.CreateUserCommand;
import tech.flowcatalyst.platform.principal.operations.deactivateuser.DeactivateUserCommand;
import tech.flowcatalyst.platform.principal.operations.updateuser.UpdateUserCommand;
import tech.flowcatalyst.platform.principal.operations.assignroles.AssignRolesCommand;
import tech.flowcatalyst.platform.principal.operations.grantclientaccess.GrantClientAccessCommand;
import tech.flowcatalyst.platform.principal.operations.revokeclientaccess.RevokeClientAccessCommand;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TypedId;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.stream.Collectors;

/**
 * SDK API for managing principals (users and service accounts).
 * Uses Bearer token authentication (no BFF session cookie).
 */
@Path("/api/sdk/principals")
@Tag(name = "SDK - Principals", description = "SDK API for principal management")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
public class SdkPrincipalResource {

    private static final Logger LOG = Logger.getLogger(SdkPrincipalResource.class);

    @Inject
    UserOperations userOperations;

    @Inject
    UserService userService;

    @Inject
    RoleService roleService;

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    ClientAccessGrantRepository grantRepo;

    @Inject
    ClientAccessService clientAccessService;

    @Inject
    JwtKeyService jwtKeyService;

    @Inject
    TracingContext tracingContext;

    @Inject
    AuthorizationService authorizationService;

    // ==================== CRUD Operations ====================

    @GET
    @Operation(operationId = "sdkListPrincipals", summary = "List principals")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of principals",
            content = @Content(schema = @Schema(implementation = PrincipalListResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response listPrincipals(
            @QueryParam("clientId") @Parameter(description = "Filter by client ID") String clientId,
            @QueryParam("type") @Parameter(description = "Filter by type (USER/SERVICE)") PrincipalType type,
            @QueryParam("active") @Parameter(description = "Filter by active status") Boolean active,
            @QueryParam("email") @Parameter(description = "Filter by exact email match") String email,
            @HeaderParam("Authorization") String authHeader) {

        var principalId = requireAuth(authHeader);
        authorizationService.requirePermission(principalId, PlatformIamPermissions.USER_VIEW);

        if (email != null && !email.isBlank()) {
            return principalRepo.findByEmail(email)
                .map(p -> Response.ok(new PrincipalListResponse(List.of(toDto(p)), 1)).build())
                .orElse(Response.ok(new PrincipalListResponse(List.of(), 0)).build());
        }

        List<Principal> principals;
        if (clientId != null && type != null && active != null) {
            principals = principalRepo.findByClientIdAndTypeAndActive(clientId, type, active);
        } else if (clientId != null && type != null) {
            principals = principalRepo.findByClientIdAndType(clientId, type);
        } else if (clientId != null && active != null) {
            principals = principalRepo.findByClientIdAndActive(clientId, active);
        } else if (clientId != null) {
            principals = principalRepo.findByClientId(clientId);
        } else if (type != null) {
            principals = principalRepo.findByType(type);
        } else if (active != null) {
            principals = principalRepo.findByActive(active);
        } else {
            principals = principalRepo.listAll();
        }

        var dtos = principals.stream().map(this::toDto).toList();
        return Response.ok(new PrincipalListResponse(dtos, dtos.size())).build();
    }

    @GET
    @Path("/{id}")
    @Operation(operationId = "sdkGetPrincipal", summary = "Get principal by ID")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Principal details",
            content = @Content(schema = @Schema(implementation = PrincipalDto.class))),
        @APIResponse(responseCode = "404", description = "Principal not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response getPrincipal(
            @PathParam("id") String id,
            @HeaderParam("Authorization") String authHeader) {

        var principalId = requireAuth(authHeader);
        authorizationService.requirePermission(principalId, PlatformIamPermissions.USER_VIEW);

        return principalRepo.findByIdOptional(id)
            .map(p -> Response.ok(toDto(p)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("PRINCIPAL_NOT_FOUND", "Principal not found"))
                .build());
    }

    @POST
    @Path("/user")
    @Operation(operationId = "sdkCreateUserPrincipal", summary = "Create a new user principal")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "User created",
            content = @Content(schema = @Schema(implementation = PrincipalDto.class))),
        @APIResponse(responseCode = "400", description = "Invalid request"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response createUserPrincipal(
            CreateUserRequest request,
            @HeaderParam("Authorization") String authHeader) {

        var adminPrincipalId = requireAuth(authHeader);
        authorizationService.requirePermission(adminPrincipalId, PlatformIamPermissions.USER_CREATE);

        var context = ExecutionContext.from(tracingContext, adminPrincipalId);

        var command = new CreateUserCommand(
            request.email(),
            request.password(),
            request.name(),
            request.clientId()
        );

        Result<UserCreated> result = userOperations.createUser(command, context);

        return switch (result) {
            case Result.Success<UserCreated> s -> {
                var principal = userOperations.findById(s.value().userId()).orElseThrow();
                yield Response.status(Response.Status.CREATED).entity(toDto(principal)).build();
            }
            case Result.Failure<UserCreated> f -> mapErrorToResponse(f.error());
        };
    }

    @POST
    @Path("/service")
    @Operation(operationId = "sdkCreateServicePrincipal", summary = "Create a new service account principal")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Service account created",
            content = @Content(schema = @Schema(implementation = StatusResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid request"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response createServicePrincipal(
            CreateServicePrincipalRequest request,
            @HeaderParam("Authorization") String authHeader) {

        var adminPrincipalId = requireAuth(authHeader);
        authorizationService.requirePermission(adminPrincipalId, PlatformIamPermissions.SERVICE_ACCOUNT_CREATE);

        // Service account creation is handled by a separate operations class.
        // This endpoint is a placeholder for SDK access.
        return Response.status(Response.Status.NOT_IMPLEMENTED)
            .entity(new ErrorResponse("NOT_IMPLEMENTED", "Service account creation via SDK is not yet supported"))
            .build();
    }

    @PUT
    @Path("/{id}")
    @Operation(operationId = "sdkUpdatePrincipal", summary = "Update principal details")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Principal updated",
            content = @Content(schema = @Schema(implementation = PrincipalDto.class))),
        @APIResponse(responseCode = "404", description = "Principal not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response updatePrincipal(
            @PathParam("id") String id,
            UpdatePrincipalRequest request,
            @HeaderParam("Authorization") String authHeader) {

        var adminPrincipalId = requireAuth(authHeader);
        authorizationService.requirePermission(adminPrincipalId, PlatformIamPermissions.USER_UPDATE);

        var context = ExecutionContext.from(tracingContext, adminPrincipalId);

        var command = new UpdateUserCommand(id, request.name(), null);
        Result<UserUpdated> result = userOperations.updateUser(command, context);

        return switch (result) {
            case Result.Success<UserUpdated> s -> {
                var principal = userOperations.findById(s.value().userId()).orElseThrow();
                yield Response.ok(toDto(principal)).build();
            }
            case Result.Failure<UserUpdated> f -> mapErrorToResponse(f.error());
        };
    }

    // ==================== Status Management ====================

    @POST
    @Path("/{id}/activate")
    @Operation(operationId = "sdkActivatePrincipal", summary = "Activate a principal")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Principal activated",
            content = @Content(schema = @Schema(implementation = StatusResponse.class))),
        @APIResponse(responseCode = "404", description = "Principal not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response activatePrincipal(
            @PathParam("id") String id,
            @HeaderParam("Authorization") String authHeader) {

        var adminPrincipalId = requireAuth(authHeader);
        authorizationService.requirePermission(adminPrincipalId, PlatformIamPermissions.USER_UPDATE);

        var context = ExecutionContext.from(tracingContext, adminPrincipalId);
        var command = new ActivateUserCommand(id);
        Result<UserActivated> result = userOperations.activateUser(command, context);

        return switch (result) {
            case Result.Success<UserActivated> s -> Response.ok(new StatusResponse("Principal activated")).build();
            case Result.Failure<UserActivated> f -> mapErrorToResponse(f.error());
        };
    }

    @POST
    @Path("/{id}/deactivate")
    @Operation(operationId = "sdkDeactivatePrincipal", summary = "Deactivate a principal")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Principal deactivated",
            content = @Content(schema = @Schema(implementation = StatusResponse.class))),
        @APIResponse(responseCode = "404", description = "Principal not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response deactivatePrincipal(
            @PathParam("id") String id,
            @HeaderParam("Authorization") String authHeader) {

        var adminPrincipalId = requireAuth(authHeader);
        authorizationService.requirePermission(adminPrincipalId, PlatformIamPermissions.USER_UPDATE);

        var context = ExecutionContext.from(tracingContext, adminPrincipalId);
        var command = new DeactivateUserCommand(id, null);
        Result<UserDeactivated> result = userOperations.deactivateUser(command, context);

        return switch (result) {
            case Result.Success<UserDeactivated> s -> Response.ok(new StatusResponse("Principal deactivated")).build();
            case Result.Failure<UserDeactivated> f -> mapErrorToResponse(f.error());
        };
    }

    // ==================== Role Management ====================

    @GET
    @Path("/{id}/roles")
    @Operation(operationId = "sdkGetPrincipalRoles", summary = "Get principal's roles")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of role assignments",
            content = @Content(schema = @Schema(implementation = RoleListResponse.class))),
        @APIResponse(responseCode = "404", description = "Principal not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response getPrincipalRoles(
            @PathParam("id") String id,
            @HeaderParam("Authorization") String authHeader) {

        var principalId = requireAuth(authHeader);
        authorizationService.requirePermission(principalId, PlatformIamPermissions.USER_VIEW);

        if (principalRepo.findByIdOptional(id).isEmpty()) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("PRINCIPAL_NOT_FOUND", "Principal not found"))
                .build();
        }

        var assignments = roleService.findAssignmentsByPrincipal(id);
        var dtos = assignments.stream()
            .map(pr -> new RoleAssignmentDto(pr.roleName, pr.assignmentSource, pr.assignedAt))
            .toList();

        return Response.ok(new RoleListResponse(dtos)).build();
    }

    @PUT
    @Path("/{id}/roles")
    @Operation(operationId = "sdkAssignPrincipalRoles", summary = "Batch assign roles to principal (declarative)")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Roles assigned",
            content = @Content(schema = @Schema(implementation = RolesAssignedResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid role names"),
        @APIResponse(responseCode = "404", description = "Principal not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response assignPrincipalRoles(
            @PathParam("id") String id,
            AssignRolesRequest request,
            @HeaderParam("Authorization") String authHeader) {

        var adminPrincipalId = requireAuth(authHeader);
        authorizationService.requirePermission(adminPrincipalId, PlatformIamPermissions.USER_UPDATE);

        var context = ExecutionContext.from(tracingContext, adminPrincipalId);
        var command = new AssignRolesCommand(id, request.roles());
        Result<RolesAssigned> result = userOperations.assignRoles(command, context);

        return switch (result) {
            case Result.Success<RolesAssigned> s -> {
                var assignments = roleService.findAssignmentsByPrincipal(id).stream()
                    .map(pr -> new RoleAssignmentDto(pr.roleName, pr.assignmentSource, pr.assignedAt))
                    .toList();
                yield Response.ok(new RolesAssignedResponse(assignments, s.value().added(), s.value().removed())).build();
            }
            case Result.Failure<RolesAssigned> f -> mapErrorToResponse(f.error());
        };
    }

    @POST
    @Path("/{id}/roles/{roleName}")
    @Operation(operationId = "sdkAddPrincipalRole", summary = "Add a single role to principal")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Role assigned",
            content = @Content(schema = @Schema(implementation = RoleAssignmentDto.class))),
        @APIResponse(responseCode = "400", description = "Role already assigned or not defined"),
        @APIResponse(responseCode = "404", description = "Principal not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response addPrincipalRole(
            @PathParam("id") String id,
            @PathParam("roleName") String roleName,
            @HeaderParam("Authorization") String authHeader) {

        var adminPrincipalId = requireAuth(authHeader);
        authorizationService.requirePermission(adminPrincipalId, PlatformIamPermissions.USER_UPDATE);

        try {
            var assignment = roleService.assignRole(id, roleName, "MANUAL");
            return Response.status(Response.Status.CREATED)
                .entity(new RoleAssignmentDto(assignment.roleName, assignment.assignmentSource, assignment.assignedAt))
                .build();
        } catch (NotFoundException e) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("NOT_FOUND", "Principal not found"))
                .build();
        } catch (BadRequestException e) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse("BAD_REQUEST", e.getMessage()))
                .build();
        }
    }

    @DELETE
    @Path("/{id}/roles/{roleName}")
    @Operation(operationId = "sdkRemovePrincipalRole", summary = "Remove a role from principal")
    @APIResponses({
        @APIResponse(responseCode = "204", description = "Role removed"),
        @APIResponse(responseCode = "404", description = "Principal or role assignment not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response removePrincipalRole(
            @PathParam("id") String id,
            @PathParam("roleName") String roleName,
            @HeaderParam("Authorization") String authHeader) {

        var adminPrincipalId = requireAuth(authHeader);
        authorizationService.requirePermission(adminPrincipalId, PlatformIamPermissions.USER_UPDATE);

        try {
            roleService.removeRole(id, roleName);
            return Response.noContent().build();
        } catch (NotFoundException e) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("NOT_FOUND", "Role assignment not found"))
                .build();
        }
    }

    // ==================== Client Access ====================

    @GET
    @Path("/{id}/clients")
    @Operation(operationId = "sdkGetPrincipalClients", summary = "Get principal's granted clients")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of client access grants",
            content = @Content(schema = @Schema(implementation = ClientAccessListResponse.class))),
        @APIResponse(responseCode = "404", description = "Principal not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response getPrincipalClients(
            @PathParam("id") String id,
            @HeaderParam("Authorization") String authHeader) {

        var principalId = requireAuth(authHeader);
        authorizationService.requirePermission(principalId, PlatformIamPermissions.USER_VIEW);

        if (principalRepo.findByIdOptional(id).isEmpty()) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("PRINCIPAL_NOT_FOUND", "Principal not found"))
                .build();
        }

        var grants = grantRepo.findByPrincipalId(id);
        var dtos = grants.stream()
            .map(g -> new ClientAccessGrantDto(g.id, g.clientId, g.grantedAt, g.expiresAt))
            .toList();

        return Response.ok(new ClientAccessListResponse(dtos)).build();
    }

    @POST
    @Path("/{id}/clients/{clientId}")
    @Operation(operationId = "sdkGrantClientAccess", summary = "Grant client access to principal")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Access granted",
            content = @Content(schema = @Schema(implementation = ClientAccessGrantDto.class))),
        @APIResponse(responseCode = "400", description = "Grant already exists"),
        @APIResponse(responseCode = "404", description = "Principal or client not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response grantClientAccess(
            @PathParam("id") String id,
            @PathParam("clientId") String clientId,
            @HeaderParam("Authorization") String authHeader) {

        var adminPrincipalId = requireAuth(authHeader);
        authorizationService.requirePermission(adminPrincipalId, PlatformIamPermissions.USER_UPDATE);

        var context = ExecutionContext.from(tracingContext, adminPrincipalId);
        var command = new GrantClientAccessCommand(id, clientId, null);
        Result<ClientAccessGranted> result = userOperations.grantClientAccess(command, context);

        return switch (result) {
            case Result.Success<ClientAccessGranted> s -> Response.status(Response.Status.CREATED)
                .entity(new ClientAccessGrantDto(
                    s.value().grantId(), s.value().clientId(), s.value().time(), s.value().expiresAt()))
                .build();
            case Result.Failure<ClientAccessGranted> f -> mapErrorToResponse(f.error());
        };
    }

    @DELETE
    @Path("/{id}/clients/{clientId}")
    @Operation(operationId = "sdkRevokeClientAccess", summary = "Revoke client access from principal")
    @APIResponses({
        @APIResponse(responseCode = "204", description = "Access revoked"),
        @APIResponse(responseCode = "404", description = "Grant not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response revokeClientAccess(
            @PathParam("id") String id,
            @PathParam("clientId") String clientId,
            @HeaderParam("Authorization") String authHeader) {

        var adminPrincipalId = requireAuth(authHeader);
        authorizationService.requirePermission(adminPrincipalId, PlatformIamPermissions.USER_UPDATE);

        var context = ExecutionContext.from(tracingContext, adminPrincipalId);
        var command = new RevokeClientAccessCommand(id, clientId);
        Result<ClientAccessRevoked> result = userOperations.revokeClientAccess(command, context);

        return switch (result) {
            case Result.Success<ClientAccessRevoked> s -> Response.noContent().build();
            case Result.Failure<ClientAccessRevoked> f -> mapErrorToResponse(f.error());
        };
    }

    // ==================== Helpers ====================

    private String requireAuth(String authHeader) {
        var principalId = jwtKeyService.extractAndValidatePrincipalId(null, authHeader);
        if (principalId.isEmpty()) {
            throw new NotAuthorizedException("Bearer");
        }
        return principalId.get();
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
            .entity(new ErrorResponse(error.code(), error.message()))
            .build();
    }

    private PrincipalDto toDto(Principal principal) {
        String email = null;
        IdpType idpType = null;
        if (principal.userIdentity != null) {
            email = principal.userIdentity.email;
            idpType = principal.userIdentity.idpType;
        }

        boolean isAnchorUser = clientAccessService.isAnchorDomainUser(principal);

        List<ClientAccessGrant> grants = grantRepo.findByPrincipalId(principal.id);
        Set<String> grantedClientIds = grants.stream()
            .map(g -> g.clientId)
            .collect(Collectors.toSet());

        return new PrincipalDto(
            principal.id,
            principal.type != null ? principal.type.name() : null,
            principal.scope != null ? principal.scope.name() : null,
            principal.clientId,
            principal.name,
            principal.active,
            email,
            idpType != null ? idpType.name() : null,
            principal.getRoleNames(),
            isAnchorUser,
            grantedClientIds,
            principal.createdAt,
            principal.updatedAt
        );
    }

    // ==================== DTOs ====================

    @Schema(description = "Principal details")
    public record PrincipalDto(
        @Schema(description = "Principal ID") String id,
        @Schema(description = "Principal type (USER/SERVICE)") String type,
        @Schema(description = "User scope (ANCHOR/PARTNER/CLIENT)") String scope,
        @Schema(description = "Home client ID") String clientId,
        @Schema(description = "Display name") String name,
        @Schema(description = "Whether principal is active") boolean active,
        @Schema(description = "Email address (USER type only)") String email,
        @Schema(description = "Identity provider type") String idpType,
        @Schema(description = "Assigned role names") Set<String> roles,
        @Schema(description = "Whether user is from anchor domain") boolean isAnchorUser,
        @Schema(description = "Granted client IDs") Set<String> grantedClientIds,
        @Schema(description = "Creation timestamp") Instant createdAt,
        @Schema(description = "Last update timestamp") Instant updatedAt
    ) {}

    @Schema(description = "List of principals")
    public record PrincipalListResponse(
        @Schema(description = "Principal list") List<PrincipalDto> principals,
        @Schema(description = "Total count") int total
    ) {}

    @Schema(description = "Create user request")
    public record CreateUserRequest(
        @Schema(required = true, description = "Email address") String email,
        @Schema(description = "Password (required for internal auth)") String password,
        @Schema(required = true, description = "Display name") String name,
        @Schema(description = "Client ID to associate with") String clientId
    ) {}

    @Schema(description = "Create service principal request")
    public record CreateServicePrincipalRequest(
        @Schema(required = true, description = "Service account name") String name,
        @Schema(description = "Description") String description
    ) {}

    @Schema(description = "Update principal request")
    public record UpdatePrincipalRequest(
        @Schema(required = true, description = "Display name") String name
    ) {}

    @Schema(description = "Assign roles request (declarative)")
    public record AssignRolesRequest(
        @Schema(required = true, description = "Complete list of role names") List<String> roles
    ) {}

    @Schema(description = "Role assignment details")
    public record RoleAssignmentDto(
        @Schema(description = "Role name") String roleName,
        @Schema(description = "How the role was assigned") String assignmentSource,
        @Schema(description = "When the role was assigned") Instant assignedAt
    ) {}

    @Schema(description = "List of role assignments")
    public record RoleListResponse(
        @Schema(description = "Role assignments") List<RoleAssignmentDto> roles
    ) {}

    @Schema(description = "Roles assigned response")
    public record RolesAssignedResponse(
        @Schema(description = "Current role assignments") List<RoleAssignmentDto> roles,
        @Schema(description = "Roles that were added") List<String> added,
        @Schema(description = "Roles that were removed") List<String> removed
    ) {}

    @Schema(description = "Client access grant")
    public record ClientAccessGrantDto(
        @Schema(description = "Grant ID") String id,
        @Schema(description = "Client ID") String clientId,
        @Schema(description = "When access was granted") Instant grantedAt,
        @Schema(description = "When access expires") Instant expiresAt
    ) {}

    @Schema(description = "List of client access grants")
    public record ClientAccessListResponse(
        @Schema(description = "Client access grants") List<ClientAccessGrantDto> grants
    ) {}

    @Schema(description = "Status response")
    public record StatusResponse(
        @Schema(description = "Status message") String message
    ) {}

    @Schema(description = "Error response")
    public record ErrorResponse(
        @Schema(description = "Error code") String code,
        @Schema(description = "Error message") String message
    ) {}
}
