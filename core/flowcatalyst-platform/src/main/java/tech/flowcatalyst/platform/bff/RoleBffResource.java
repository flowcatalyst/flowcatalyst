package tech.flowcatalyst.platform.bff;

import io.quarkus.runtime.annotations.RegisterForReflection;
import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.authentication.JwtKeyService;
import tech.flowcatalyst.platform.authorization.AuthRole;
import tech.flowcatalyst.platform.authorization.PermissionDefinition;
import tech.flowcatalyst.platform.authorization.PermissionRegistry;
import tech.flowcatalyst.platform.authorization.RoleOperations;
import tech.flowcatalyst.platform.authorization.events.RoleCreated;
import tech.flowcatalyst.platform.authorization.events.RoleDeleted;
import tech.flowcatalyst.platform.authorization.events.RoleUpdated;
import tech.flowcatalyst.platform.authorization.operations.createrole.CreateRoleCommand;
import tech.flowcatalyst.platform.authorization.operations.deleterole.DeleteRoleCommand;
import tech.flowcatalyst.platform.authorization.operations.updaterole.UpdateRoleCommand;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.TracingContext;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.List;
import java.util.Map;
import java.util.Set;

/**
 * BFF (Backend For Frontend) endpoints for Roles.
 * Returns IDs as strings to preserve precision for JavaScript clients.
 */
@Path("/bff/roles")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@Tag(name = "BFF - Roles", description = "Web-optimized role endpoints with string IDs")
@RegisterForReflection(registerFullHierarchy = true)
public class RoleBffResource {

    @Inject
    RoleOperations roleOperations;

    @Inject
    PermissionRegistry permissionRegistry;

    @Inject
    ApplicationRepository applicationRepository;

    @Inject
    JwtKeyService jwtKeyService;

    @Inject
    AuditContext auditContext;

    @Inject
    TracingContext tracingContext;

    @GET
    @Operation(summary = "List all roles (BFF)")
    public Response listRoles(
        @QueryParam("application") String applicationCode,
        @QueryParam("source") String source,
        @CookieParam("fc_session") String sessionToken,
        @HeaderParam("Authorization") String authHeader
    ) {
        List<AuthRole> roles;

        if (applicationCode != null && !applicationCode.isBlank()) {
            roles = roleOperations.findByApplicationCode(applicationCode);
        } else {
            roles = roleOperations.findAll();
        }

        // Filter by source if provided
        if (source != null && !source.isBlank()) {
            try {
                AuthRole.RoleSource sourceEnum = AuthRole.RoleSource.valueOf(source.toUpperCase());
                roles = roles.stream()
                    .filter(r -> r.source == sourceEnum)
                    .toList();
            } catch (IllegalArgumentException e) {
                return Response.status(400)
                    .entity(new ErrorResponse("INVALID_SOURCE", "Invalid source. Must be CODE, DATABASE, or SDK"))
                    .build();
            }
        }

        List<BffRoleResponse> responses = roles.stream()
            .map(BffRoleResponse::from)
            .toList();

        return Response.ok(new BffRoleListResponse(responses, responses.size())).build();
    }

    @GET
    @Path("/{roleName}")
    @Operation(summary = "Get role by name (BFF)")
    public Response getRole(@PathParam("roleName") String roleName) {
        return roleOperations.findByName(roleName)
            .map(role -> Response.ok(BffRoleResponse.from(role)).build())
            .orElse(Response.status(404)
                .entity(new ErrorResponse("ROLE_NOT_FOUND", "Role not found: " + roleName))
                .build());
    }

    @GET
    @Path("/filters/applications")
    @Operation(summary = "Get applications for role filter")
    public Response getApplications() {
        List<Application> apps = applicationRepository.listAll();
        List<BffApplicationOption> options = apps.stream()
            .filter(a -> a.active)
            .map(a -> new BffApplicationOption(a.id.toString(), a.code, a.name))
            .toList();
        return Response.ok(new BffApplicationOptionsResponse(options)).build();
    }

    @POST
    @Operation(summary = "Create a new role (BFF)")
    public Response createRole(
        CreateRoleRequest request,
        @CookieParam("fc_session") String sessionToken,
        @HeaderParam("Authorization") String authHeader
    ) {
        var principalIdOpt = jwtKeyService.extractAndValidatePrincipalId(sessionToken, authHeader);
        if (principalIdOpt.isEmpty()) {
            return Response.status(401).entity(new ErrorResponse("UNAUTHORIZED", "Not authenticated")).build();
        }
        String principalId = principalIdOpt.get();

        if (request.applicationCode() == null || request.applicationCode().isBlank()) {
            return Response.status(400).entity(new ErrorResponse("APPLICATION_CODE_REQUIRED", "applicationCode is required")).build();
        }
        if (request.name() == null || request.name().isBlank()) {
            return Response.status(400).entity(new ErrorResponse("NAME_REQUIRED", "name is required")).build();
        }

        Application app = applicationRepository.findByCode(request.applicationCode()).orElse(null);
        if (app == null) {
            return Response.status(404)
                .entity(new ErrorResponse("APPLICATION_NOT_FOUND", "Application not found: " + request.applicationCode()))
                .build();
        }

        auditContext.setPrincipalId(principalId);
        ExecutionContext context = ExecutionContext.from(tracingContext, principalId);

        CreateRoleCommand command = new CreateRoleCommand(
            app.id,
            request.name(),
            request.displayName(),
            request.description(),
            request.permissions(),
            AuthRole.RoleSource.DATABASE,
            request.clientManaged() != null ? request.clientManaged() : false
        );

        Result<RoleCreated> result = roleOperations.createRole(command, context);

        return switch (result) {
            case Result.Success<RoleCreated> s -> {
                AuthRole role = roleOperations.findByName(s.value().roleName()).orElseThrow();
                yield Response.status(201).entity(BffRoleResponse.from(role)).build();
            }
            case Result.Failure<RoleCreated> f -> mapErrorToResponse(f.error());
        };
    }

    @PUT
    @Path("/{roleName}")
    @Operation(summary = "Update a role (BFF)")
    public Response updateRole(
        @PathParam("roleName") String roleName,
        UpdateRoleRequest request,
        @CookieParam("fc_session") String sessionToken,
        @HeaderParam("Authorization") String authHeader
    ) {
        var principalIdOpt = jwtKeyService.extractAndValidatePrincipalId(sessionToken, authHeader);
        if (principalIdOpt.isEmpty()) {
            return Response.status(401).entity(new ErrorResponse("UNAUTHORIZED", "Not authenticated")).build();
        }
        String principalId = principalIdOpt.get();

        auditContext.setPrincipalId(principalId);
        ExecutionContext context = ExecutionContext.from(tracingContext, principalId);

        UpdateRoleCommand command = new UpdateRoleCommand(
            roleName,
            request.displayName(),
            request.description(),
            request.permissions(),
            request.clientManaged()
        );

        Result<RoleUpdated> result = roleOperations.updateRole(command, context);

        return switch (result) {
            case Result.Success<RoleUpdated> s -> {
                AuthRole role = roleOperations.findByName(s.value().roleName()).orElseThrow();
                yield Response.ok(BffRoleResponse.from(role)).build();
            }
            case Result.Failure<RoleUpdated> f -> mapErrorToResponse(f.error());
        };
    }

    @DELETE
    @Path("/{roleName}")
    @Operation(summary = "Delete a role (BFF)")
    public Response deleteRole(
        @PathParam("roleName") String roleName,
        @CookieParam("fc_session") String sessionToken,
        @HeaderParam("Authorization") String authHeader
    ) {
        var principalIdOpt = jwtKeyService.extractAndValidatePrincipalId(sessionToken, authHeader);
        if (principalIdOpt.isEmpty()) {
            return Response.status(401).entity(new ErrorResponse("UNAUTHORIZED", "Not authenticated")).build();
        }
        String principalId = principalIdOpt.get();

        auditContext.setPrincipalId(principalId);
        ExecutionContext context = ExecutionContext.from(tracingContext, principalId);

        DeleteRoleCommand command = new DeleteRoleCommand(roleName);

        Result<RoleDeleted> result = roleOperations.deleteRole(command, context);

        return switch (result) {
            case Result.Success<RoleDeleted> s -> Response.noContent().build();
            case Result.Failure<RoleDeleted> f -> mapErrorToResponse(f.error());
        };
    }

    // ========================================================================
    // Permissions Endpoints
    // ========================================================================

    @GET
    @Path("/permissions")
    @Operation(summary = "List all permissions (BFF)")
    public Response listPermissions() {
        List<BffPermissionResponse> responses = permissionRegistry.getAllPermissions().stream()
            .map(BffPermissionResponse::from)
            .sorted((a, b) -> a.permission().compareTo(b.permission()))
            .toList();

        return Response.ok(new BffPermissionListResponse(responses, responses.size())).build();
    }

    @GET
    @Path("/permissions/{permission}")
    @Operation(summary = "Get permission by string (BFF)")
    public Response getPermission(@PathParam("permission") String permission) {
        return permissionRegistry.getPermission(permission)
            .map(perm -> Response.ok(BffPermissionResponse.from(perm)).build())
            .orElse(Response.status(404)
                .entity(new ErrorResponse("PERMISSION_NOT_FOUND", "Permission not found: " + permission))
                .build());
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

    // ========================================================================
    // BFF DTOs - IDs as Strings for JavaScript precision
    // ========================================================================

    public record BffRoleListResponse(List<BffRoleResponse> items, int total) {}

    public record BffRoleResponse(
        String id,
        String name,
        String shortName,
        String displayName,
        String description,
        Set<String> permissions,
        String applicationCode,
        String source,
        boolean clientManaged,
        String createdAt,
        String updatedAt
    ) {
        public static BffRoleResponse from(AuthRole role) {
            return new BffRoleResponse(
                role.id != null ? role.id.toString() : null,
                role.name,
                role.getShortName(),
                role.displayName,
                role.description,
                role.permissions,
                role.applicationCode,
                role.source != null ? role.source.name() : null,
                role.clientManaged,
                role.createdAt != null ? role.createdAt.toString() : null,
                role.updatedAt != null ? role.updatedAt.toString() : null
            );
        }
    }

    public record BffApplicationOption(String id, String code, String name) {}
    public record BffApplicationOptionsResponse(List<BffApplicationOption> options) {}

    public record CreateRoleRequest(
        String applicationCode,
        String name,
        String displayName,
        String description,
        Set<String> permissions,
        Boolean clientManaged
    ) {}

    public record UpdateRoleRequest(
        String displayName,
        String description,
        Set<String> permissions,
        Boolean clientManaged
    ) {}

    public record ErrorResponse(String code, String message, Map<String, Object> details) {
        public ErrorResponse(String code, String message) {
            this(code, message, Map.of());
        }
    }

    public record BffPermissionListResponse(List<BffPermissionResponse> items, int total) {}

    public record BffPermissionResponse(
        String permission,
        String application,
        String context,
        String aggregate,
        String action,
        String description
    ) {
        public static BffPermissionResponse from(PermissionDefinition perm) {
            return new BffPermissionResponse(
                perm.toPermissionString(),
                perm.application(),
                perm.context(),
                perm.aggregate(),
                perm.action(),
                perm.description()
            );
        }
    }
}
