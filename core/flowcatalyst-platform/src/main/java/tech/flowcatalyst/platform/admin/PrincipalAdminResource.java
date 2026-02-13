package tech.flowcatalyst.platform.admin;

import jakarta.inject.Inject;
import jakarta.validation.Valid;
import jakarta.validation.constraints.Email;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
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

import tech.flowcatalyst.platform.authentication.IdpType;
import tech.flowcatalyst.platform.authorization.AuthorizationService;
import tech.flowcatalyst.platform.authorization.PrincipalRole;
import tech.flowcatalyst.platform.authorization.RoleService;
import tech.flowcatalyst.platform.authorization.platform.PlatformIamPermissions;
import tech.flowcatalyst.platform.authentication.idp.IdentityProvider;
import tech.flowcatalyst.platform.authentication.idp.IdentityProviderService;
import tech.flowcatalyst.platform.authentication.idp.IdentityProviderType;
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
import tech.flowcatalyst.platform.principal.operations.deleteuser.DeleteUserCommand;
import tech.flowcatalyst.platform.principal.operations.grantclientaccess.GrantClientAccessCommand;
import tech.flowcatalyst.platform.principal.operations.revokeclientaccess.RevokeClientAccessCommand;
import tech.flowcatalyst.platform.principal.operations.updateuser.UpdateUserCommand;
import tech.flowcatalyst.platform.principal.operations.assignroles.AssignRolesCommand;
import tech.flowcatalyst.platform.principal.operations.assignapplicationaccess.AssignApplicationAccessCommand;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.application.ApplicationClientConfigRepository;
import tech.flowcatalyst.platform.authentication.domain.EmailDomainMappingRepository;
import tech.flowcatalyst.platform.principal.entity.PrincipalApplicationAccessEntity;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TypedId;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.stream.Collectors;

/**
 * Admin API for principal (user/service account) management.
 *
 * Provides CRUD operations for principals including:
 * - Create, read, update users
 * - Activate/deactivate principals
 * - Password management (reset)
 * - Role assignments
 * - Client access grants
 *
 * All operations require admin-level permissions.
 */
@Path("/api/admin/principals")
@Tag(name = "BFF - Principal Admin", description = "Administrative operations for user and service account management")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@jakarta.transaction.Transactional
public class PrincipalAdminResource {

    private static final Logger LOG = Logger.getLogger(PrincipalAdminResource.class);

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
    IdentityProviderService idpService;

    @Inject
    EmailDomainMappingRepository emailDomainMappingRepo;

    @Inject
    ApplicationRepository applicationRepo;

    @Inject
    ApplicationClientConfigRepository appConfigRepo;

    @Inject
    jakarta.persistence.EntityManager em;

    @Inject
    AuditContext auditContext;

    @Inject
    TracingContext tracingContext;

    @Inject
    AuthorizationService authorizationService;

    // ==================== CRUD Operations ====================

    /**
     * List all principals with optional filters.
     */
    @GET
    @Operation(operationId = "listPrincipals", summary = "List principals", description = "List users and service accounts with optional filters")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of principals",
            content = @Content(schema = @Schema(implementation = PrincipalListResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated")
    })
    public Response listPrincipals(
            @QueryParam("clientId") @Parameter(description = "Filter by client ID") String clientId,
            @QueryParam("type") @Parameter(description = "Filter by type (USER/SERVICE)") PrincipalType type,
            @QueryParam("active") @Parameter(description = "Filter by active status") Boolean active,
            @QueryParam("email") @Parameter(description = "Filter by exact email match") String email) {

        String principalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(principalId, PlatformIamPermissions.USER_VIEW.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.USER_VIEW.toPermissionString()))
                .build();
        }

        // If email is provided, filter by exact email match (takes precedence over other filters)
        if (email != null && !email.isBlank()) {
            return principalRepo.findByEmail(email)
                .map(p -> Response.ok(new PrincipalListResponse(List.of(toDto(p)), 1)).build())
                .orElse(Response.ok(new PrincipalListResponse(List.of(), 0)).build());
        }

        List<Principal> principals;

        // Build query based on filters
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

        List<PrincipalDto> dtos = principals.stream()
            .map(this::toDto)
            .toList();

        return Response.ok(new PrincipalListResponse(dtos, dtos.size())).build();
    }

    /**
     * Get a specific principal by ID.
     */
    @GET
    @Path("/{id}")
    @Operation(operationId = "getPrincipal", summary = "Get principal by ID")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Principal details",
            content = @Content(schema = @Schema(implementation = PrincipalDetailDto.class))),
        @APIResponse(responseCode = "404", description = "Principal not found")
    })
    public Response getPrincipal(@PathParam("id") String id) {

        String principalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(principalId, PlatformIamPermissions.USER_VIEW.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.USER_VIEW.toPermissionString()))
                .build();
        }

        // Validate ID has correct type prefix
        TypedId.Ops.validate(EntityType.PRINCIPAL, id);

        return principalRepo.findByIdOptional(id)
            .map(principal -> {
                // Include roles and client access grants
                Set<String> roles = roleService.findRoleNamesByPrincipal(id);
                List<ClientAccessGrant> grants = grantRepo.findByPrincipalId(id);
                Set<String> grantedClientIds = grants.stream()
                    .map(g -> g.clientId)
                    .collect(Collectors.toSet());

                return Response.ok(toDetailDto(principal, roles, grantedClientIds)).build();
            })
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("Principal not found"))
                .build());
    }

    /**
     * Create a new internal user.
     */
    @POST
    @Path("/users")
    @Operation(operationId = "createUser", summary = "Create a new internal user")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "User created",
            content = @Content(schema = @Schema(implementation = PrincipalDto.class))),
        @APIResponse(responseCode = "400", description = "Invalid request or email already exists")
    })
    public Response createUser(
            @Valid CreateUserRequest request,
            @Context UriInfo uriInfo) {

        String adminPrincipalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(adminPrincipalId, PlatformIamPermissions.USER_CREATE.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.USER_CREATE.toPermissionString()))
                .build();
        }

        ExecutionContext context = ExecutionContext.from(tracingContext, adminPrincipalId);

        CreateUserCommand command = new CreateUserCommand(
            request.email(),
            request.password(),
            request.name(),
            request.clientId()
        );

        Result<UserCreated> result = userOperations.createUser(command, context);

        return switch (result) {
            case Result.Success<UserCreated> s -> {
                Principal principal = userOperations.findById(s.value().userId()).orElseThrow();
                LOG.infof("User created: %s by principal %s", request.email(), adminPrincipalId);
                yield Response.status(Response.Status.CREATED)
                    .entity(toDto(principal))
                    .location(uriInfo.getBaseUriBuilder()
                        .path(PrincipalAdminResource.class)
                        .path(String.valueOf(principal.id))
                        .build())
                    .build();
            }
            case Result.Failure<UserCreated> f -> mapErrorToResponse(f.error());
        };
    }

    /**
     * Update a principal's name.
     */
    @PUT
    @Path("/{id}")
    @Operation(operationId = "updatePrincipal", summary = "Update principal details")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Principal updated"),
        @APIResponse(responseCode = "404", description = "Principal not found")
    })
    public Response updatePrincipal(
            @PathParam("id") String id,
            @Valid UpdatePrincipalRequest request) {

        String adminPrincipalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(adminPrincipalId, PlatformIamPermissions.USER_UPDATE.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.USER_UPDATE.toPermissionString()))
                .build();
        }

        ExecutionContext context = ExecutionContext.from(tracingContext, adminPrincipalId);

        // Validate ID has correct type prefix
        TypedId.Ops.validate(EntityType.PRINCIPAL, id);

        UpdateUserCommand command = new UpdateUserCommand(id, request.name(), null);
        Result<UserUpdated> result = userOperations.updateUser(command, context);

        return switch (result) {
            case Result.Success<UserUpdated> s -> {
                Principal principal = userOperations.findById(s.value().userId()).orElseThrow();
                LOG.infof("Principal %s updated by principal %s", id, adminPrincipalId);
                yield Response.ok(toDto(principal)).build();
            }
            case Result.Failure<UserUpdated> f -> mapErrorToResponse(f.error());
        };
    }

    // ==================== Status Management ====================

    /**
     * Activate a principal.
     */
    @POST
    @Path("/{id}/activate")
    @Operation(operationId = "activatePrincipal", summary = "Activate a principal")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Principal activated"),
        @APIResponse(responseCode = "404", description = "Principal not found")
    })
    public Response activatePrincipal(@PathParam("id") String id) {

        String adminPrincipalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(adminPrincipalId, PlatformIamPermissions.USER_UPDATE.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.USER_UPDATE.toPermissionString()))
                .build();
        }

        ExecutionContext context = ExecutionContext.from(tracingContext, adminPrincipalId);

        // Validate ID has correct type prefix
        TypedId.Ops.validate(EntityType.PRINCIPAL, id);

        ActivateUserCommand command = new ActivateUserCommand(id);
        Result<UserActivated> result = userOperations.activateUser(command, context);

        return switch (result) {
            case Result.Success<UserActivated> s -> {
                LOG.infof("Principal %s activated by principal %s", id, adminPrincipalId);
                yield Response.ok(new StatusResponse("Principal activated")).build();
            }
            case Result.Failure<UserActivated> f -> mapErrorToResponse(f.error());
        };
    }

    /**
     * Deactivate a principal.
     */
    @POST
    @Path("/{id}/deactivate")
    @Operation(operationId = "deactivatePrincipal", summary = "Deactivate a principal")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Principal deactivated"),
        @APIResponse(responseCode = "404", description = "Principal not found")
    })
    public Response deactivatePrincipal(@PathParam("id") String id) {

        String adminPrincipalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(adminPrincipalId, PlatformIamPermissions.USER_UPDATE.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.USER_UPDATE.toPermissionString()))
                .build();
        }

        ExecutionContext context = ExecutionContext.from(tracingContext, adminPrincipalId);

        DeactivateUserCommand command = new DeactivateUserCommand(id, null);
        Result<UserDeactivated> result = userOperations.deactivateUser(command, context);

        return switch (result) {
            case Result.Success<UserDeactivated> s -> {
                LOG.infof("Principal %s deactivated by principal %s", id, adminPrincipalId);
                yield Response.ok(new StatusResponse("Principal deactivated")).build();
            }
            case Result.Failure<UserDeactivated> f -> mapErrorToResponse(f.error());
        };
    }

    // ==================== Password Management ====================

    /**
     * Reset a user's password (admin action).
     */
    @POST
    @Path("/{id}/reset-password")
    @Operation(operationId = "resetPrincipalPassword", summary = "Reset user password")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Password reset"),
        @APIResponse(responseCode = "400", description = "User is not internal auth or password doesn't meet requirements"),
        @APIResponse(responseCode = "404", description = "User not found")
    })
    public Response resetPassword(
            @PathParam("id") String id,
            @Valid ResetPasswordRequest request) {

        String adminPrincipalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(adminPrincipalId, PlatformIamPermissions.USER_UPDATE.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.USER_UPDATE.toPermissionString()))
                .build();
        }

        try {
            userService.resetPassword(id, request.newPassword());
            LOG.infof("Password reset for principal %s by principal %s", id, adminPrincipalId);
            return Response.ok(new StatusResponse("Password reset successfully")).build();
        } catch (NotFoundException e) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("User not found"))
                .build();
        } catch (IllegalArgumentException e) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse(e.getMessage()))
                .build();
        }
    }

    // ==================== Role Management ====================

    /**
     * Get roles assigned to a principal.
     */
    @GET
    @Path("/{id}/roles")
    @Operation(operationId = "getPrincipalRoles", summary = "Get principal's roles")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of roles"),
        @APIResponse(responseCode = "404", description = "Principal not found")
    })
    public Response getPrincipalRoles(@PathParam("id") String id) {

        String principalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(principalId, PlatformIamPermissions.USER_VIEW.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.USER_VIEW.toPermissionString()))
                .build();
        }

        if (!principalRepo.findByIdOptional(id).isPresent()) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("Principal not found"))
                .build();
        }

        List<PrincipalRole> assignments = roleService.findAssignmentsByPrincipal(id);
        List<RoleAssignmentDto> dtos = assignments.stream()
            .map(pr -> new RoleAssignmentDto(pr.roleName, pr.assignmentSource, pr.assignedAt))
            .toList();

        return Response.ok(new RoleListResponse(dtos)).build();
    }

    /**
     * Assign a role to a principal.
     */
    @POST
    @Path("/{id}/roles")
    @Operation(operationId = "assignPrincipalRole", summary = "Assign role to principal")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Role assigned"),
        @APIResponse(responseCode = "400", description = "Role already assigned or not defined"),
        @APIResponse(responseCode = "404", description = "Principal not found")
    })
    public Response assignRole(
            @PathParam("id") String id,
            @Valid AssignRoleRequest request) {

        String adminPrincipalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(adminPrincipalId, PlatformIamPermissions.USER_UPDATE.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.USER_UPDATE.toPermissionString()))
                .build();
        }

        try {
            PrincipalRole assignment = roleService.assignRole(id, request.roleName(), "MANUAL");
            LOG.infof("Role %s assigned to principal %s by principal %s",
                request.roleName(), id, adminPrincipalId);

            return Response.status(Response.Status.CREATED)
                .entity(new RoleAssignmentDto(
                    assignment.roleName,
                    assignment.assignmentSource,
                    assignment.assignedAt
                ))
                .build();
        } catch (NotFoundException e) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("Principal not found"))
                .build();
        } catch (BadRequestException e) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse(e.getMessage()))
                .build();
        }
    }

    /**
     * Remove a role from a principal.
     */
    @DELETE
    @Path("/{id}/roles/{roleName}")
    @Operation(operationId = "removePrincipalRole", summary = "Remove role from principal")
    @APIResponses({
        @APIResponse(responseCode = "204", description = "Role removed"),
        @APIResponse(responseCode = "404", description = "Principal or role assignment not found")
    })
    public Response removeRole(
            @PathParam("id") String id,
            @PathParam("roleName") String roleName) {

        String adminPrincipalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(adminPrincipalId, PlatformIamPermissions.USER_UPDATE.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.USER_UPDATE.toPermissionString()))
                .build();
        }

        try {
            roleService.removeRole(id, roleName);
            LOG.infof("Role %s removed from principal %s by principal %s",
                roleName, id, adminPrincipalId);
            return Response.noContent().build();
        } catch (NotFoundException e) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("Role assignment not found"))
                .build();
        }
    }

    /**
     * Batch assign roles to a principal.
     *
     * <p>This is a declarative operation - the provided list represents the complete
     * set of roles the user should have. Roles not in the list will be removed,
     * new roles will be added.
     */
    @PUT
    @Path("/{id}/roles")
    @Operation(operationId = "assignPrincipalRoles", summary = "Batch assign roles to principal",
        description = "Sets the complete list of roles for a principal. Roles not in the list will be removed.")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Roles assigned"),
        @APIResponse(responseCode = "400", description = "Invalid role names"),
        @APIResponse(responseCode = "404", description = "Principal not found")
    })
    public Response assignRoles(
            @PathParam("id") String id,
            @Valid AssignRolesRequest request) {

        String adminPrincipalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(adminPrincipalId, PlatformIamPermissions.USER_UPDATE.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.USER_UPDATE.toPermissionString()))
                .build();
        }

        ExecutionContext context = ExecutionContext.from(tracingContext, adminPrincipalId);

        // Build command
        AssignRolesCommand command = new AssignRolesCommand(id, request.roles());

        // Execute use case
        Result<RolesAssigned> result = userOperations.assignRoles(command, context);

        return switch (result) {
            case Result.Success<RolesAssigned> s -> {
                LOG.infof("Roles assigned to principal %s by principal %s: added=%s, removed=%s",
                    id, adminPrincipalId, s.value().added(), s.value().removed());

                // Return updated role assignments
                List<RoleAssignmentDto> assignments = roleService.findAssignmentsByPrincipal(id).stream()
                    .map(pr -> new RoleAssignmentDto(pr.roleName, pr.assignmentSource, pr.assignedAt))
                    .toList();

                yield Response.ok(new RolesAssignedResponse(
                    assignments,
                    s.value().added(),
                    s.value().removed()
                )).build();
            }
            case Result.Failure<RolesAssigned> f -> mapErrorToResponse(f.error());
        };
    }

    // ==================== Client Access Grants ====================

    /**
     * Get client access grants for a principal.
     */
    @GET
    @Path("/{id}/client-access")
    @Operation(operationId = "getPrincipalClientAccess", summary = "Get principal's client access grants")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of client access grants"),
        @APIResponse(responseCode = "404", description = "Principal not found")
    })
    public Response getClientAccessGrants(@PathParam("id") String id) {

        String principalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(principalId, PlatformIamPermissions.USER_VIEW.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.USER_VIEW.toPermissionString()))
                .build();
        }

        if (!principalRepo.findByIdOptional(id).isPresent()) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("Principal not found"))
                .build();
        }

        List<ClientAccessGrant> grants = grantRepo.findByPrincipalId(id);
        List<ClientAccessGrantDto> dtos = grants.stream()
            .map(g -> new ClientAccessGrantDto(g.id, g.clientId, g.grantedAt, g.expiresAt))
            .toList();

        return Response.ok(new ClientAccessListResponse(dtos)).build();
    }

    /**
     * Grant client access to a principal.
     */
    @POST
    @Path("/{id}/client-access")
    @Operation(operationId = "grantPrincipalClientAccess", summary = "Grant client access to principal")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Access granted"),
        @APIResponse(responseCode = "400", description = "Grant already exists or principal belongs to client"),
        @APIResponse(responseCode = "404", description = "Principal or client not found")
    })
    public Response grantClientAccess(
            @PathParam("id") String id,
            @Valid GrantClientAccessRequest request) {

        String adminPrincipalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(adminPrincipalId, PlatformIamPermissions.USER_UPDATE.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.USER_UPDATE.toPermissionString()))
                .build();
        }

        ExecutionContext context = ExecutionContext.from(tracingContext, adminPrincipalId);

        GrantClientAccessCommand command = new GrantClientAccessCommand(id, request.clientId(), null);
        Result<ClientAccessGranted> result = userOperations.grantClientAccess(command, context);

        return switch (result) {
            case Result.Success<ClientAccessGranted> s -> {
                LOG.infof("Client access to %s granted to principal %s by principal %s",
                    request.clientId(), id, adminPrincipalId);
                yield Response.status(Response.Status.CREATED)
                    .entity(new ClientAccessGrantDto(
                        s.value().grantId(),
                        s.value().clientId(),
                        s.value().time(),
                        s.value().expiresAt()
                    ))
                    .build();
            }
            case Result.Failure<ClientAccessGranted> f -> mapErrorToResponse(f.error());
        };
    }

    /**
     * Revoke client access from a principal.
     */
    @DELETE
    @Path("/{id}/client-access/{clientId}")
    @Operation(operationId = "revokePrincipalClientAccess", summary = "Revoke client access from principal")
    @APIResponses({
        @APIResponse(responseCode = "204", description = "Access revoked"),
        @APIResponse(responseCode = "404", description = "Grant not found")
    })
    public Response revokeClientAccess(
            @PathParam("id") String id,
            @PathParam("clientId") String clientId) {

        String adminPrincipalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(adminPrincipalId, PlatformIamPermissions.USER_UPDATE.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.USER_UPDATE.toPermissionString()))
                .build();
        }

        ExecutionContext context = ExecutionContext.from(tracingContext, adminPrincipalId);

        RevokeClientAccessCommand command = new RevokeClientAccessCommand(id, clientId);
        Result<ClientAccessRevoked> result = userOperations.revokeClientAccess(command, context);

        return switch (result) {
            case Result.Success<ClientAccessRevoked> s -> {
                LOG.infof("Client access to %s revoked from principal %s by principal %s",
                    clientId, id, adminPrincipalId);
                yield Response.noContent().build();
            }
            case Result.Failure<ClientAccessRevoked> f -> mapErrorToResponse(f.error());
        };
    }

    // ==================== Application Access ====================

    /**
     * Get application access grants for a principal.
     */
    @GET
    @Path("/{id}/application-access")
    @Operation(operationId = "getPrincipalApplicationAccess", summary = "Get principal's application access grants")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of application access grants"),
        @APIResponse(responseCode = "404", description = "Principal not found")
    })
    public Response getApplicationAccess(@PathParam("id") String id) {

        String principalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(principalId, PlatformIamPermissions.USER_VIEW.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.USER_VIEW.toPermissionString()))
                .build();
        }

        var principalOpt = principalRepo.findByIdOptional(id);
        if (principalOpt.isEmpty()) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("Principal not found"))
                .build();
        }

        // Load application access grants from junction table
        List<PrincipalApplicationAccessEntity> accessEntities = em.createQuery(
                "SELECT a FROM PrincipalApplicationAccessEntity a WHERE a.principalId = :id",
                PrincipalApplicationAccessEntity.class)
            .setParameter("id", id)
            .getResultList();

        // Enrich with application details
        Set<String> appIds = accessEntities.stream()
            .map(a -> a.applicationId)
            .collect(Collectors.toSet());

        Map<String, Application> appsById = applicationRepo.findByIds(appIds).stream()
            .collect(Collectors.toMap(a -> a.id, a -> a));

        List<ApplicationAccessDto> dtos = accessEntities.stream()
            .map(entity -> {
                Application app = appsById.get(entity.applicationId);
                return new ApplicationAccessDto(
                    entity.applicationId,
                    app != null ? app.code : null,
                    app != null ? app.name : null,
                    entity.grantedAt
                );
            })
            .toList();

        return Response.ok(new ApplicationAccessListResponse(dtos)).build();
    }

    /**
     * Batch assign application access to a principal.
     *
     * <p>This is a declarative operation - the provided list represents the complete
     * set of applications the user should have access to. Applications not in the list
     * will be removed, new applications will be added.
     */
    @PUT
    @Path("/{id}/application-access")
    @Operation(operationId = "assignPrincipalApplicationAccess", summary = "Batch assign application access to principal",
        description = "Sets the complete list of applications for a principal. Applications not in the list will be removed.")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Application access assigned"),
        @APIResponse(responseCode = "400", description = "Invalid application IDs or applications not accessible"),
        @APIResponse(responseCode = "404", description = "Principal not found")
    })
    public Response assignApplicationAccess(
            @PathParam("id") String id,
            @Valid AssignApplicationAccessRequest request) {

        String adminPrincipalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(adminPrincipalId, PlatformIamPermissions.USER_UPDATE.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.USER_UPDATE.toPermissionString()))
                .build();
        }

        ExecutionContext context = ExecutionContext.from(tracingContext, adminPrincipalId);

        // Build command
        AssignApplicationAccessCommand command = new AssignApplicationAccessCommand(id, request.applicationIds());

        // Execute use case
        Result<ApplicationAccessAssigned> result = userOperations.assignApplicationAccess(command, context);

        return switch (result) {
            case Result.Success<ApplicationAccessAssigned> s -> {
                LOG.infof("Application access assigned to principal %s by principal %s: added=%s, removed=%s",
                    id, adminPrincipalId, s.value().added(), s.value().removed());

                // Return updated application access
                List<PrincipalApplicationAccessEntity> accessEntities = em.createQuery(
                        "SELECT a FROM PrincipalApplicationAccessEntity a WHERE a.principalId = :id",
                        PrincipalApplicationAccessEntity.class)
                    .setParameter("id", id)
                    .getResultList();

                Set<String> appIds = accessEntities.stream()
                    .map(a -> a.applicationId)
                    .collect(Collectors.toSet());

                Map<String, Application> appsById = applicationRepo.findByIds(appIds).stream()
                    .collect(Collectors.toMap(a -> a.id, a -> a));

                List<ApplicationAccessDto> dtos = accessEntities.stream()
                    .map(entity -> {
                        Application app = appsById.get(entity.applicationId);
                        return new ApplicationAccessDto(
                            entity.applicationId,
                            app != null ? app.code : null,
                            app != null ? app.name : null,
                            entity.grantedAt
                        );
                    })
                    .toList();

                yield Response.ok(new ApplicationAccessAssignedResponse(
                    dtos,
                    s.value().added(),
                    s.value().removed()
                )).build();
            }
            case Result.Failure<ApplicationAccessAssigned> f -> mapErrorToResponse(f.error());
        };
    }

    /**
     * Get available applications for a principal (filtered to client-accessible apps).
     */
    @GET
    @Path("/{id}/available-applications")
    @Operation(operationId = "getPrincipalAvailableApplications", summary = "Get applications available to grant to principal",
        description = "Returns applications that are enabled for at least one of the user's accessible clients")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of available applications"),
        @APIResponse(responseCode = "404", description = "Principal not found")
    })
    public Response getAvailableApplications(@PathParam("id") String id) {

        String principalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(principalId, PlatformIamPermissions.USER_VIEW.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.USER_VIEW.toPermissionString()))
                .build();
        }

        var principalOpt = principalRepo.findByIdOptional(id);
        if (principalOpt.isEmpty()) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("Principal not found"))
                .build();
        }

        Principal principal = principalOpt.get();

        // Get user's accessible clients
        Set<String> accessibleClientIds = clientAccessService.getAccessibleClients(principal);

        // Find all active applications that are enabled for at least one accessible client
        List<Application> allApps = applicationRepo.findAllActive();
        List<AvailableApplicationDto> availableApps = allApps.stream()
            .filter(app -> {
                for (String clientId : accessibleClientIds) {
                    if (appConfigRepo.isApplicationEnabledForClient(app.id, clientId)) {
                        return true;
                    }
                }
                return false;
            })
            .map(app -> new AvailableApplicationDto(app.id, app.code, app.name))
            .toList();

        return Response.ok(new AvailableApplicationsResponse(availableApps)).build();
    }

    // ==================== Helper Methods ====================

    private PrincipalDto toDto(Principal principal) {
        String email = null;
        IdpType idpType = null;
        if (principal.userIdentity != null) {
            email = principal.userIdentity.email;
            idpType = principal.userIdentity.idpType;
        }

        boolean isAnchorUser = clientAccessService.isAnchorDomainUser(principal);

        // Get granted client IDs (already prefixed in database)
        List<ClientAccessGrant> grants = grantRepo.findByPrincipalId(principal.id);
        Set<String> grantedClientIds = grants.stream()
            .map(g -> g.clientId)
            .collect(Collectors.toSet());

        return new PrincipalDto(
            principal.id,  // Already prefixed
            principal.type,
            principal.scope,
            principal.clientId,  // Already prefixed
            principal.name,
            principal.active,
            email,
            idpType,
            principal.getRoleNames(),
            isAnchorUser,
            grantedClientIds,
            principal.createdAt,
            principal.updatedAt
        );
    }

    private PrincipalDetailDto toDetailDto(Principal principal, Set<String> roles, Set<String> grantedClientIds) {
        String email = null;
        IdpType idpType = null;
        Instant lastLoginAt = null;
        if (principal.userIdentity != null) {
            email = principal.userIdentity.email;
            idpType = principal.userIdentity.idpType;
            lastLoginAt = principal.userIdentity.lastLoginAt;
        }

        boolean isAnchorUser = clientAccessService.isAnchorDomainUser(principal);

        // Client IDs are already prefixed in the database
        return new PrincipalDetailDto(
            principal.id,  // Already prefixed
            principal.type,
            principal.scope,
            principal.clientId,  // Already prefixed
            principal.name,
            principal.active,
            email,
            idpType,
            lastLoginAt,
            roles,
            isAnchorUser,
            grantedClientIds,  // Already prefixed
            principal.createdAt,
            principal.updatedAt
        );
    }

    // ==================== Email Domain Check ====================

    /**
     * Check email domain configuration.
     * Returns the authentication provider configured for the domain,
     * whether it's an anchor domain, and any warnings.
     */
    @GET
    @Path("/check-email-domain")
    @Operation(operationId = "checkEmailDomain", summary = "Check email domain configuration",
        description = "Returns auth provider info and warnings for an email domain")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Domain info returned",
            content = @Content(schema = @Schema(implementation = EmailDomainCheckResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid email format")
    })
    public Response checkEmailDomain(
            @QueryParam("email") @Parameter(description = "Email address to check") String email) {

        String principalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(principalId, PlatformIamPermissions.USER_VIEW.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.USER_VIEW.toPermissionString()))
                .build();
        }

        if (email == null || !email.contains("@")) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse("Invalid email format"))
                .build();
        }

        String domain = email.substring(email.indexOf('@') + 1).toLowerCase();

        // Check if email already exists
        boolean emailExists = principalRepo.findByEmail(email).isPresent();

        // Check if anchor domain
        boolean isAnchorDomain = emailDomainMappingRepo.isAnchorDomain(domain);

        // Check identity provider configuration
        var idpOpt = idpService.findByEmailDomain(domain);

        String authProvider = null;
        String warning = null;
        String info = null;

        if (emailExists) {
            warning = "A user with this email address already exists.";
            // Still determine auth provider for display
            if (isAnchorDomain) {
                authProvider = "INTERNAL";
            } else if (idpOpt.isPresent()) {
                authProvider = idpOpt.get().type.name();
            } else {
                authProvider = "INTERNAL";
            }
        } else if (isAnchorDomain) {
            info = "This is an anchor domain. User will have access to all clients.";
            authProvider = "INTERNAL";
        } else if (idpOpt.isPresent()) {
            IdentityProvider idp = idpOpt.get();
            authProvider = idp.type.name();
            if (idp.type == IdentityProviderType.OIDC) {
                info = "This domain uses external OIDC authentication. User will authenticate via their organization's identity provider.";
            } else {
                info = "This domain uses internal authentication.";
            }
        } else {
            warning = "No authentication configuration found for this email domain. The user will be created with internal authentication but may not be linked to any client.";
            authProvider = "INTERNAL";
        }

        return Response.ok(new EmailDomainCheckResponse(
            domain,
            authProvider,
            isAnchorDomain,
            idpOpt.isPresent(),
            emailExists,
            info,
            warning
        )).build();
    }

    // ==================== DTOs ====================

    public record PrincipalDto(
        String id,
        PrincipalType type,
        UserScope scope,
        String clientId,
        String name,
        boolean active,
        String email,
        IdpType idpType,
        Set<String> roles,
        boolean isAnchorUser,
        Set<String> grantedClientIds,
        Instant createdAt,
        Instant updatedAt
    ) {}

    public record PrincipalDetailDto(
        String id,
        PrincipalType type,
        UserScope scope,
        String clientId,
        String name,
        boolean active,
        String email,
        IdpType idpType,
        Instant lastLoginAt,
        Set<String> roles,
        boolean isAnchorUser,
        Set<String> grantedClientIds,
        Instant createdAt,
        Instant updatedAt
    ) {}

    public record PrincipalListResponse(
        List<PrincipalDto> principals,
        int total
    ) {}

    public record CreateUserRequest(
        @NotBlank(message = "Email is required")
        @Email(message = "Invalid email format")
        String email,

        // Password is optional - only required for INTERNAL auth users
        // OIDC users will authenticate via their identity provider
        @Size(min = 12, message = "Password must be at least 12 characters")
        String password,

        @NotBlank(message = "Name is required")
        String name,

        String clientId
    ) {}

    public record UpdatePrincipalRequest(
        @NotBlank(message = "Name is required")
        String name
    ) {}

    public record ResetPasswordRequest(
        @NotBlank(message = "New password is required")
        @Size(min = 12, message = "Password must be at least 12 characters")
        String newPassword
    ) {}

    public record AssignRoleRequest(
        @NotBlank(message = "Role name is required")
        String roleName
    ) {}

    public record GrantClientAccessRequest(
        @NotNull(message = "Client ID is required")
        String clientId
    ) {}

    public record RoleAssignmentDto(
        String roleName,
        String assignmentSource,
        Instant assignedAt
    ) {}

    public record RoleListResponse(
        List<RoleAssignmentDto> roles
    ) {}

    public record AssignRolesRequest(
        @NotNull(message = "Roles list is required")
        List<String> roles
    ) {}

    public record RolesAssignedResponse(
        List<RoleAssignmentDto> roles,
        List<String> added,
        List<String> removed
    ) {}

    public record ClientAccessGrantDto(
        String id,
        String clientId,
        Instant grantedAt,
        Instant expiresAt
    ) {}

    public record ClientAccessListResponse(
        List<ClientAccessGrantDto> grants
    ) {}

    // ========== Application Access DTOs ==========

    public record ApplicationAccessDto(
        String applicationId,
        String applicationCode,
        String applicationName,
        Instant grantedAt
    ) {}

    public record ApplicationAccessListResponse(
        List<ApplicationAccessDto> applications
    ) {}

    public record AssignApplicationAccessRequest(
        @NotNull(message = "Application IDs list is required")
        List<String> applicationIds
    ) {}

    public record ApplicationAccessAssignedResponse(
        List<ApplicationAccessDto> applications,
        List<String> added,
        List<String> removed
    ) {}

    public record AvailableApplicationDto(
        String id,
        String code,
        String name
    ) {}

    public record AvailableApplicationsResponse(
        List<AvailableApplicationDto> applications
    ) {}

    public record StatusResponse(
        String message
    ) {}

    public record EmailDomainCheckResponse(
        String domain,
        String authProvider,
        boolean isAnchorDomain,
        boolean hasIdpConfig,
        boolean emailExists,
        String info,
        String warning
    ) {}

    public record ErrorResponse(
        String code,
        String message,
        Map<String, Object> details
    ) {
        public ErrorResponse(String message) {
            this("ERROR", message, Map.of());
        }
    }

    // ==================== Error Mapping ====================

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
}
