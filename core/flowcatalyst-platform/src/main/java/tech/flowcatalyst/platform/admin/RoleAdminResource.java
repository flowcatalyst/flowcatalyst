package tech.flowcatalyst.platform.admin;

import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.*;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.media.Content;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponses;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.authentication.EmbeddedModeOnly;
import tech.flowcatalyst.platform.authorization.*;
import tech.flowcatalyst.platform.authorization.platform.PlatformIamPermissions;
import tech.flowcatalyst.platform.authorization.PermissionInput;
import tech.flowcatalyst.platform.authorization.events.RoleCreated;
import tech.flowcatalyst.platform.authorization.events.RoleDeleted;
import tech.flowcatalyst.platform.authorization.events.RoleUpdated;
import tech.flowcatalyst.platform.authorization.operations.createrole.CreateRoleCommand;
import tech.flowcatalyst.platform.authorization.operations.deleterole.DeleteRoleCommand;
import tech.flowcatalyst.platform.authorization.operations.updaterole.UpdateRoleCommand;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.TracingContext;
import tech.flowcatalyst.platform.common.api.ApiResponses;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.Collection;
import java.util.List;
import java.util.Set;

/**
 * Admin API for managing roles and viewing permissions.
 *
 * Roles can come from three sources:
 * - CODE: Defined in Java @Role classes (read-only, synced to DB at startup)
 * - DATABASE: Created by administrators through this API
 * - SDK: Registered by external applications via the SDK API
 *
 * Permissions are code-first (defined in Java code) and cannot be created via API.
 * External applications can register their own permissions via SDK.
 */
@Path("/api/admin/roles")
@Tag(name = "BFF - Role Admin", description = "Manage roles and view permissions")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@EmbeddedModeOnly
@jakarta.transaction.Transactional
public class RoleAdminResource {

    @Inject
    PermissionRegistry permissionRegistry;

    @Inject
    RoleService roleService;

    @Inject
    RoleOperations roleOperations;

    @Inject
    ApplicationRepository applicationRepository;

    @Inject
    AuditContext auditContext;

    @Inject
    TracingContext tracingContext;

    @Inject
    AuthorizationService authorizationService;

    // ==================== Roles ====================

    /**
     * List all available roles from the database.
     */
    @GET
    @Operation(operationId = "listRoles", summary = "List all available roles",
        description = "Returns all roles from the database. Filter by application code or source.")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of roles",
            content = @Content(schema = @Schema(implementation = RoleListResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid source filter",
            content = @Content(schema = @Schema(implementation = ApiResponses.ErrorResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response listRoles(
            @QueryParam("application") String application,
            @QueryParam("source") String source) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformIamPermissions.ROLE_VIEW);

        List<AuthRole> roles;
        if (application != null && !application.isBlank()) {
            roles = roleService.getRolesForApplication(application);
        } else {
            roles = roleService.getAllRoles();
        }

        // Filter by source if provided
        if (source != null && !source.isBlank()) {
            try {
                AuthRole.RoleSource sourceEnum = AuthRole.RoleSource.valueOf(source.toUpperCase());
                roles = roles.stream()
                    .filter(r -> r.source == sourceEnum)
                    .toList();
            } catch (IllegalArgumentException e) {
                return Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ApiResponses.ErrorResponse("INVALID_SOURCE", "Invalid source. Must be CODE, DATABASE, or SDK"))
                    .build();
            }
        }

        List<RoleDto> dtos = roles.stream()
            .map(this::toRoleDto)
            .sorted((a, b) -> a.name().compareTo(b.name()))
            .toList();

        return Response.ok(new RoleListResponse(dtos, dtos.size())).build();
    }

    /**
     * Get a specific role by name.
     */
    @GET
    @Path("/{roleName}")
    @Operation(operationId = "getRole", summary = "Get role details by name")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Role details with permissions",
            content = @Content(schema = @Schema(implementation = RoleDto.class))),
        @APIResponse(responseCode = "404", description = "Role not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response getRole(@PathParam("roleName") String roleName) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformIamPermissions.ROLE_VIEW);

        return roleService.getRoleByName(roleName)
            .map(role -> Response.ok(toRoleDto(role)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ApiResponses.NotFoundResponse("role", roleName))
                .build());
    }

    /**
     * Create a new role for an application.
     * Role name will be auto-prefixed with the application code.
     */
    @POST
    @Operation(operationId = "createRole", summary = "Create a new role",
        description = "Creates a new role with source=DATABASE. Role name is auto-prefixed with application code.")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Role created",
            content = @Content(schema = @Schema(implementation = RoleDto.class))),
        @APIResponse(responseCode = "400", description = "Invalid request",
            content = @Content(schema = @Schema(implementation = ApiResponses.ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Application not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class))),
        @APIResponse(responseCode = "409", description = "Role already exists",
            content = @Content(schema = @Schema(implementation = ApiResponses.ConflictResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response createRole(CreateRoleRequest request) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformIamPermissions.ROLE_CREATE);

        // Validate request
        if (request.applicationCode() == null || request.applicationCode().isBlank()) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ApiResponses.ErrorResponse("APPLICATION_CODE_REQUIRED", "applicationCode is required"))
                .build();
        }
        if (request.name() == null || request.name().isBlank()) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ApiResponses.ErrorResponse("NAME_REQUIRED", "name is required"))
                .build();
        }

        // Find application
        Application app = applicationRepository.findByCode(request.applicationCode())
            .orElse(null);
        if (app == null) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ApiResponses.NotFoundResponse("application", request.applicationCode()))
                .build();
        }

        // Create execution context (audit context already set by filter)
        var context = ExecutionContext.from(tracingContext, principalId);

        CreateRoleCommand command = new CreateRoleCommand(
            app.id,
            request.name(),
            request.displayName(),
            request.description(),
            request.permissions() != null
                ? request.permissions().stream().map(PermissionInputDto::toPermissionInput).toList()
                : List.of(),
            AuthRole.RoleSource.DATABASE,
            request.clientManaged() != null ? request.clientManaged() : false
        );

        Result<RoleCreated> result = roleOperations.createRole(command, context);

        return switch (result) {
            case Result.Success<RoleCreated> s -> {
                AuthRole role = roleOperations.findByName(s.value().roleName()).orElseThrow();
                yield Response.status(Response.Status.CREATED).entity(toRoleDto(role)).build();
            }
            case Result.Failure<RoleCreated> f -> mapErrorToResponse(f.error());
        };
    }

    /**
     * Update an existing role.
     * CODE-sourced roles can only have clientManaged flag updated.
     */
    @PUT
    @Path("/{roleName}")
    @Operation(operationId = "updateRole", summary = "Update a role",
        description = "Updates a role. CODE-sourced roles can only have clientManaged updated.")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Role updated",
            content = @Content(schema = @Schema(implementation = RoleDto.class))),
        @APIResponse(responseCode = "400", description = "Invalid request",
            content = @Content(schema = @Schema(implementation = ApiResponses.ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Role not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response updateRole(
            @PathParam("roleName") String roleName,
            UpdateRoleRequest request) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformIamPermissions.ROLE_UPDATE);

        // Create execution context (audit context already set by filter)
        var context = ExecutionContext.from(tracingContext, principalId);

        UpdateRoleCommand command = new UpdateRoleCommand(
            roleName,
            request.displayName(),
            request.description(),
            request.permissions() != null
                ? request.permissions().stream().map(PermissionInputDto::toPermissionInput).toList()
                : null,
            request.clientManaged()
        );

        var result = roleOperations.updateRole(command, context);

        return switch (result) {
            case Result.Success<RoleUpdated> s -> {
                AuthRole role = roleOperations.findByName(s.value().roleName()).orElseThrow();
                yield Response.ok(toRoleDto(role)).build();
            }
            case Result.Failure<RoleUpdated> f -> mapErrorToResponse(f.error());
        };
    }

    /**
     * Delete a role. Only DATABASE and SDK sourced roles can be deleted.
     */
    @DELETE
    @Path("/{roleName}")
    @Operation(operationId = "deleteRole", summary = "Delete a role",
        description = "Deletes a role. Only DATABASE and SDK sourced roles can be deleted.")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Role deleted",
            content = @Content(schema = @Schema(implementation = ApiResponses.DeleteResponse.class))),
        @APIResponse(responseCode = "400", description = "Cannot delete CODE-defined role",
            content = @Content(schema = @Schema(implementation = ApiResponses.ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Role not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response deleteRole(@PathParam("roleName") String roleName) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformIamPermissions.ROLE_DELETE);

        // Create execution context (audit context already set by filter)
        ExecutionContext context = ExecutionContext.from(tracingContext, principalId);

        var command = new DeleteRoleCommand(roleName);

        var result = roleOperations.deleteRole(command, context);

        return switch (result) {
            case Result.Success<RoleDeleted> s ->
                Response.ok(new ApiResponses.DeleteResponse(s.value().roleName(), "role")).build();
            case Result.Failure<RoleDeleted> f -> mapErrorToResponse(f.error());
        };
    }

    // ==================== Permissions ====================

    /**
     * List all available permissions.
     * Includes both code-defined and database permissions.
     */
    @GET
    @Path("/permissions")
    @Operation(operationId = "listPermissions", summary = "List all available permissions",
        description = "Returns all permissions from code and database.")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of permissions",
            content = @Content(schema = @Schema(implementation = PermissionListResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response listPermissions() {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformIamPermissions.PERMISSION_VIEW);

        Collection<PermissionDefinition> permissions = permissionRegistry.getAllPermissions();

        List<PermissionDto> dtos = permissions.stream()
            .map(this::toPermissionDto)
            .sorted((a, b) -> a.permission().compareTo(b.permission()))
            .toList();

        return Response.ok(new PermissionListResponse(dtos, dtos.size())).build();
    }

    /**
     * Get a specific permission by string.
     */
    @GET
    @Path("/permissions/{permission}")
    @Operation(operationId = "getPermission", summary = "Get permission details")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Permission details",
            content = @Content(schema = @Schema(implementation = PermissionDto.class))),
        @APIResponse(responseCode = "404", description = "Permission not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response getPermission(@PathParam("permission") String permission) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformIamPermissions.PERMISSION_VIEW);

        return permissionRegistry.getPermission(permission)
            .map(perm -> Response.ok(toPermissionDto(perm)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ApiResponses.NotFoundResponse("permission", permission))
                .build());
    }

    // ==================== Helper Methods ====================

    private Response mapErrorToResponse(UseCaseError error) {
        return switch (error) {
            case UseCaseError.ValidationError v -> Response.status(Response.Status.BAD_REQUEST)
                .entity(new ApiResponses.ErrorResponse(v.code(), v.message(), v.details()))
                .build();
            case UseCaseError.NotFoundError n -> Response.status(Response.Status.NOT_FOUND)
                .entity(new ApiResponses.NotFoundResponse(n.code(), n.message(), "resource", null))
                .build();
            case UseCaseError.BusinessRuleViolation b -> Response.status(Response.Status.CONFLICT)
                .entity(new ApiResponses.ConflictResponse(b.code(), b.message(), b.details()))
                .build();
            case UseCaseError.ConcurrencyError c -> Response.status(Response.Status.CONFLICT)
                .entity(new ApiResponses.ConflictResponse(c.code(), c.message()))
                .build();
            case UseCaseError.AuthorizationError a -> Response.status(Response.Status.FORBIDDEN)
                .entity(new ApiResponses.ForbiddenResponse(a.message()))
                .build();
        };
    }

    private RoleDto toRoleDto(AuthRole role) {
        return new RoleDto(
            role.name,
            role.applicationCode,
            role.displayName,
            role.getShortName(),
            role.description,
            role.permissions,
            role.source.name(),
            role.clientManaged,
            role.createdAt,
            role.updatedAt
        );
    }

    private PermissionDto toPermissionDto(PermissionDefinition perm) {
        return new PermissionDto(
            perm.toPermissionString(),
            perm.application(),
            perm.context(),
            perm.aggregate(),
            perm.action(),
            perm.description()
        );
    }

    // ==================== DTOs ====================

    public record RoleDto(
        String name,
        String applicationCode,
        String displayName,
        String shortName,
        String description,
        Set<String> permissions,
        String source,
        boolean clientManaged,
        java.time.Instant createdAt,
        java.time.Instant updatedAt
    ) {}

    public record RoleListResponse(
        List<RoleDto> roles,
        int total
    ) {}

    /**
     * Request to create a role.
     *
     * <p>Permissions are structured with explicit segments to enforce format.
     */
    public record CreateRoleRequest(
        String applicationCode,
        String name,
        String displayName,
        String description,
        List<PermissionInputDto> permissions,
        Boolean clientManaged
    ) {}

    /**
     * Request to update a role.
     *
     * <p>Permissions are structured with explicit segments to enforce format.
     */
    public record UpdateRoleRequest(
        String displayName,
        String description,
        List<PermissionInputDto> permissions,
        Boolean clientManaged
    ) {}

    /**
     * Structured permission input.
     *
     * <p>Format: {application}:{context}:{aggregate}:{action}
     */
    public record PermissionInputDto(
        String application,
        String context,
        String aggregate,
        String action
    ) {
        public PermissionInput toPermissionInput() {
            return new PermissionInput(application, context, aggregate, action);
        }
    }

    public record PermissionDto(
        String permission,
        String application,
        String context,
        String aggregate,
        String action,
        String description
    ) {}

    public record PermissionListResponse(
        List<PermissionDto> permissions,
        int total
    ) {}
}
