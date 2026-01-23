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
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.authentication.JwtKeyService;
import tech.flowcatalyst.platform.authorization.AuthRole;
import tech.flowcatalyst.platform.authorization.PermissionInput;
import tech.flowcatalyst.platform.authorization.RoleOperations;
import tech.flowcatalyst.platform.authorization.RoleService;
import tech.flowcatalyst.platform.authorization.events.RoleCreated;
import tech.flowcatalyst.platform.authorization.events.RoleDeleted;
import tech.flowcatalyst.platform.authorization.events.RolesSynced;
import tech.flowcatalyst.platform.authorization.operations.createrole.CreateRoleCommand;
import tech.flowcatalyst.platform.authorization.operations.deleterole.DeleteRoleCommand;
import tech.flowcatalyst.platform.authorization.operations.syncroles.SyncRolesCommand;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.TracingContext;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.List;
import java.util.Map;
import java.util.Set;

/**
 * SDK API for external applications to manage their roles.
 *
 * External applications using the FlowCatalyst SDK can:
 * - Register roles for assignment to users
 * - Sync roles (bulk create/update/delete)
 * - Remove roles
 *
 * Role names are auto-prefixed with the application code.
 * For example, if app code is "myapp" and role name is "admin",
 * the full role name will be "myapp:admin".
 */
@Path("/api/applications/{appCode}/roles")
@Tag(name = "Application Roles SDK", description = "SDK API for external applications to manage their roles")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
public class ApplicationRoleResource {

    @Inject
    ApplicationRepository applicationRepository;

    @Inject
    RoleService roleService;

    @Inject
    RoleOperations roleOperations;

    @Inject
    JwtKeyService jwtKeyService;

    @Inject
    AuditContext auditContext;

    @Inject
    TracingContext tracingContext;

    /**
     * List all roles for an application.
     */
    @GET
    @Operation(summary = "List application roles",
        description = "Returns all roles registered for this application.")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of roles",
            content = @Content(schema = @Schema(implementation = RoleListResponse.class))),
        @APIResponse(responseCode = "404", description = "Application not found")
    })
    public Response listRoles(
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

        List<AuthRole> roles = roleService.getRolesForApplication(appCode);

        // Filter by source if provided
        if (source != null && !source.isBlank()) {
            try {
                AuthRole.RoleSource sourceEnum = AuthRole.RoleSource.valueOf(source.toUpperCase());
                roles = roles.stream()
                    .filter(r -> r.source == sourceEnum)
                    .toList();
            } catch (IllegalArgumentException e) {
                return Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ErrorResponse("INVALID_SOURCE", "Invalid source. Must be CODE, DATABASE, or SDK"))
                    .build();
            }
        }

        List<RoleDto> dtos = roles.stream()
            .map(this::toRoleDto)
            .toList();

        return Response.ok(new RoleListResponse(dtos, dtos.size())).build();
    }

    /**
     * Sync roles from an external application.
     * Creates new roles, updates existing SDK roles, and optionally removes unlisted SDK roles.
     *
     * This is the primary method for SDK integration - applications call this endpoint
     * on startup or when their role definitions change.
     */
    @POST
    @Path("/sync")
    @Operation(summary = "Sync application roles",
        description = "Bulk sync roles from an external application. " +
                      "Creates new roles, updates existing SDK roles. " +
                      "Set removeUnlisted=true to remove SDK roles not in the sync list.")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Sync complete",
            content = @Content(schema = @Schema(implementation = SyncResponse.class))),
        @APIResponse(responseCode = "404", description = "Application not found")
    })
    public Response syncRoles(
            @PathParam("appCode") String appCode,
            @QueryParam("removeUnlisted") @DefaultValue("false") boolean removeUnlisted,
            SyncRolesRequest request,
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

        if (request.roles() == null || request.roles().isEmpty()) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse("ROLES_REQUIRED", "roles list is required"))
                .build();
        }

        // Set audit context and create execution context
        auditContext.setPrincipalId(principalId);
        ExecutionContext context = ExecutionContext.from(tracingContext, principalId);

        // Convert request to internal format
        List<SyncRolesCommand.SyncRoleItem> roleItems = request.roles().stream()
            .map(r -> new SyncRolesCommand.SyncRoleItem(
                r.name(),
                r.displayName(),
                r.description(),
                r.permissions() != null
                    ? r.permissions().stream().map(PermissionInputDto::toPermissionInput).toList()
                    : List.of(),
                r.clientManaged() != null ? r.clientManaged() : false
            ))
            .toList();

        SyncRolesCommand command = new SyncRolesCommand(app.id, roleItems, removeUnlisted);
        Result<RolesSynced> result = roleOperations.syncRoles(command, context);

        return switch (result) {
            case Result.Success<RolesSynced> s -> {
                // Return updated role list
                List<AuthRole> roles = roleService.getRolesForApplication(appCode);
                List<RoleDto> dtos = roles.stream()
                    .filter(r -> r.source == AuthRole.RoleSource.SDK)
                    .map(this::toRoleDto)
                    .toList();
                yield Response.ok(new SyncResponse(dtos.size(), dtos)).build();
            }
            case Result.Failure<RolesSynced> f -> mapErrorToResponse(f.error());
        };
    }

    /**
     * Create a single role for an application.
     */
    @POST
    @Operation(summary = "Create application role",
        description = "Creates a single role for the application with source=SDK.")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Role created",
            content = @Content(schema = @Schema(implementation = RoleDto.class))),
        @APIResponse(responseCode = "404", description = "Application not found"),
        @APIResponse(responseCode = "409", description = "Role already exists")
    })
    public Response createRole(
            @PathParam("appCode") String appCode,
            CreateRoleRequest request,
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

        if (request.name() == null || request.name().isBlank()) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse("NAME_REQUIRED", "name is required"))
                .build();
        }

        // Set audit context and create execution context
        auditContext.setPrincipalId(principalId);
        ExecutionContext context = ExecutionContext.from(tracingContext, principalId);

        CreateRoleCommand command = new CreateRoleCommand(
            app.id,
            request.name(),
            request.displayName(),
            request.description(),
            request.permissions() != null
                ? request.permissions().stream().map(PermissionInputDto::toPermissionInput).toList()
                : List.of(),
            AuthRole.RoleSource.SDK,
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
     * Delete an SDK role.
     * Only SDK-sourced roles can be deleted via this endpoint.
     */
    @DELETE
    @Path("/{roleName}")
    @Operation(summary = "Delete application role",
        description = "Deletes an SDK-sourced role. Cannot delete CODE or DATABASE sourced roles.")
    @APIResponses({
        @APIResponse(responseCode = "204", description = "Role deleted"),
        @APIResponse(responseCode = "400", description = "Cannot delete non-SDK role"),
        @APIResponse(responseCode = "404", description = "Role not found")
    })
    public Response deleteRole(
            @PathParam("appCode") String appCode,
            @PathParam("roleName") String roleName,
            @CookieParam("fc_session") String sessionToken,
            @HeaderParam("Authorization") String authHeader) {

        var principalIdOpt = jwtKeyService.extractAndValidatePrincipalId(sessionToken, authHeader);
        if (principalIdOpt.isEmpty()) {
            return Response.status(Response.Status.UNAUTHORIZED)
                .entity(new ErrorResponse("UNAUTHORIZED", "Not authenticated"))
                .build();
        }
        String principalId = principalIdOpt.get();

        // Construct full role name
        String fullRoleName = appCode + ":" + roleName;

        // Verify the role belongs to this application
        var roleOpt = roleService.getRoleByName(fullRoleName);
        if (roleOpt.isEmpty()) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("ROLE_NOT_FOUND", "Role not found: " + fullRoleName))
                .build();
        }

        AuthRole role = roleOpt.get();
        if (role.source != AuthRole.RoleSource.SDK) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse("CANNOT_DELETE_NON_SDK", "Cannot delete non-SDK role via SDK API. Source: " + role.source))
                .build();
        }

        // Set audit context and create execution context
        auditContext.setPrincipalId(principalId);
        ExecutionContext context = ExecutionContext.from(tracingContext, principalId);

        DeleteRoleCommand command = new DeleteRoleCommand(fullRoleName);
        Result<RoleDeleted> result = roleOperations.deleteRole(command, context);

        return switch (result) {
            case Result.Success<RoleDeleted> s -> Response.noContent().build();
            case Result.Failure<RoleDeleted> f -> mapErrorToResponse(f.error());
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

    private RoleDto toRoleDto(AuthRole role) {
        return new RoleDto(
            role.getShortName(),
            role.name,
            role.displayName,
            role.description,
            role.permissions,
            role.source.name(),
            role.clientManaged
        );
    }

    // ==================== DTOs ====================

    public record RoleDto(
        String name,
        String fullName,
        String displayName,
        String description,
        Set<String> permissions,
        String source,
        boolean clientManaged
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
        String name,
        String displayName,
        String description,
        List<PermissionInputDto> permissions,
        Boolean clientManaged
    ) {}

    public record SyncRolesRequest(
        List<SyncRoleItem> roles
    ) {}

    /**
     * Role item for sync request.
     *
     * <p>Permissions are structured with explicit segments to enforce format.
     */
    public record SyncRoleItem(
        String name,
        String displayName,
        String description,
        List<PermissionInputDto> permissions,
        Boolean clientManaged
    ) {}

    /**
     * Structured permission input.
     *
     * <p>Format: {application}:{context}:{aggregate}:{action}
     *
     * @param application Application code (e.g., "myapp")
     * @param context     Bounded context (e.g., "orders")
     * @param aggregate   Resource/entity (e.g., "order")
     * @param action      Operation (e.g., "view", "create", "update", "delete")
     */
    public record PermissionInputDto(
        String application,
        String context,
        String aggregate,
        String action
    ) {
        /**
         * Convert to internal PermissionInput.
         */
        public PermissionInput toPermissionInput() {
            return new PermissionInput(application, context, aggregate, action);
        }
    }

    public record SyncResponse(
        int syncedCount,
        List<RoleDto> roles
    ) {}

    public record ErrorResponse(String code, String message, Map<String, Object> details) {
        public ErrorResponse(String code, String message) {
            this(code, message, Map.of());
        }
    }
}
