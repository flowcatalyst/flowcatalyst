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
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.authentication.JwtKeyService;
import tech.flowcatalyst.platform.authorization.*;
import tech.flowcatalyst.platform.authorization.events.RoleCreated;
import tech.flowcatalyst.platform.authorization.events.RoleDeleted;
import tech.flowcatalyst.platform.authorization.events.RoleUpdated;
import tech.flowcatalyst.platform.authorization.operations.createrole.CreateRoleCommand;
import tech.flowcatalyst.platform.authorization.operations.deleterole.DeleteRoleCommand;
import tech.flowcatalyst.platform.authorization.operations.updaterole.UpdateRoleCommand;
import tech.flowcatalyst.platform.authorization.platform.PlatformIamPermissions;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.TracingContext;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Set;

/**
 * SDK API for managing roles.
 * Uses Bearer token authentication (no BFF session cookie).
 */
@Path("/api/sdk/roles")
@Tag(name = "SDK - Roles", description = "SDK API for role management")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
public class SdkRoleResource {

    private static final Logger LOG = Logger.getLogger(SdkRoleResource.class);

    @Inject
    RoleService roleService;

    @Inject
    RoleOperations roleOperations;

    @Inject
    ApplicationRepository applicationRepository;

    @Inject
    JwtKeyService jwtKeyService;

    @Inject
    TracingContext tracingContext;

    @Inject
    AuthorizationService authorizationService;

    // ==================== CRUD Operations ====================

    @GET
    @Operation(operationId = "sdkListRoles", summary = "List roles")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of roles",
            content = @Content(schema = @Schema(implementation = RoleListResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response listRoles(
            @QueryParam("application") @Parameter(description = "Filter by application code") String application,
            @QueryParam("source") @Parameter(description = "Filter by source (CODE, DATABASE, SDK)") String source,
            @HeaderParam("Authorization") String authHeader) {

        var principalId = requireAuth(authHeader);
        authorizationService.requirePermission(principalId, PlatformIamPermissions.ROLE_VIEW);

        List<AuthRole> roles;
        if (application != null && !application.isBlank()) {
            roles = roleService.getRolesForApplication(application);
        } else {
            roles = roleService.getAllRoles();
        }

        if (source != null && !source.isBlank()) {
            try {
                var sourceEnum = AuthRole.RoleSource.valueOf(source.toUpperCase());
                roles = roles.stream().filter(r -> r.source == sourceEnum).toList();
            } catch (IllegalArgumentException e) {
                return Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ErrorResponse("INVALID_SOURCE", "Invalid source. Must be CODE, DATABASE, or SDK"))
                    .build();
            }
        }

        var dtos = roles.stream()
            .map(this::toRoleDto)
            .sorted((a, b) -> a.name().compareTo(b.name()))
            .toList();

        return Response.ok(new RoleListResponse(dtos, dtos.size())).build();
    }

    @GET
    @Path("/{roleName}")
    @Operation(operationId = "sdkGetRole", summary = "Get role by name")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Role details",
            content = @Content(schema = @Schema(implementation = RoleDto.class))),
        @APIResponse(responseCode = "404", description = "Role not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response getRole(
            @PathParam("roleName") String roleName,
            @HeaderParam("Authorization") String authHeader) {

        var principalId = requireAuth(authHeader);
        authorizationService.requirePermission(principalId, PlatformIamPermissions.ROLE_VIEW);

        return roleService.getRoleByName(roleName)
            .map(role -> Response.ok(toRoleDto(role)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("ROLE_NOT_FOUND", "Role not found: " + roleName))
                .build());
    }

    @POST
    @Operation(operationId = "sdkCreateRole", summary = "Create a new role")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Role created",
            content = @Content(schema = @Schema(implementation = RoleDto.class))),
        @APIResponse(responseCode = "400", description = "Invalid request"),
        @APIResponse(responseCode = "404", description = "Application not found"),
        @APIResponse(responseCode = "409", description = "Role already exists"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response createRole(
            CreateRoleRequest request,
            @HeaderParam("Authorization") String authHeader) {

        var principalId = requireAuth(authHeader);
        authorizationService.requirePermission(principalId, PlatformIamPermissions.ROLE_CREATE);

        if (request.applicationCode() == null || request.applicationCode().isBlank()) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse("APPLICATION_CODE_REQUIRED", "applicationCode is required"))
                .build();
        }
        if (request.name() == null || request.name().isBlank()) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse("NAME_REQUIRED", "name is required"))
                .build();
        }

        Application app = applicationRepository.findByCode(request.applicationCode()).orElse(null);
        if (app == null) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("APPLICATION_NOT_FOUND", "Application not found: " + request.applicationCode()))
                .build();
        }

        var context = ExecutionContext.from(tracingContext, principalId);

        var command = new CreateRoleCommand(
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
                var role = roleOperations.findByName(s.value().roleName()).orElseThrow();
                yield Response.status(Response.Status.CREATED).entity(toRoleDto(role)).build();
            }
            case Result.Failure<RoleCreated> f -> mapErrorToResponse(f.error());
        };
    }

    @PUT
    @Path("/{roleName}")
    @Operation(operationId = "sdkUpdateRole", summary = "Update a role")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Role updated",
            content = @Content(schema = @Schema(implementation = RoleDto.class))),
        @APIResponse(responseCode = "404", description = "Role not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response updateRole(
            @PathParam("roleName") String roleName,
            UpdateRoleRequest request,
            @HeaderParam("Authorization") String authHeader) {

        var principalId = requireAuth(authHeader);
        authorizationService.requirePermission(principalId, PlatformIamPermissions.ROLE_UPDATE);

        var context = ExecutionContext.from(tracingContext, principalId);

        var command = new UpdateRoleCommand(
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
                var role = roleOperations.findByName(s.value().roleName()).orElseThrow();
                yield Response.ok(toRoleDto(role)).build();
            }
            case Result.Failure<RoleUpdated> f -> mapErrorToResponse(f.error());
        };
    }

    @DELETE
    @Path("/{roleName}")
    @Operation(operationId = "sdkDeleteRole", summary = "Delete a role")
    @APIResponses({
        @APIResponse(responseCode = "204", description = "Role deleted"),
        @APIResponse(responseCode = "400", description = "Cannot delete CODE-defined role"),
        @APIResponse(responseCode = "404", description = "Role not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response deleteRole(
            @PathParam("roleName") String roleName,
            @HeaderParam("Authorization") String authHeader) {

        var principalId = requireAuth(authHeader);
        authorizationService.requirePermission(principalId, PlatformIamPermissions.ROLE_DELETE);

        var context = ExecutionContext.from(tracingContext, principalId);
        var command = new DeleteRoleCommand(roleName);
        var result = roleOperations.deleteRole(command, context);

        return switch (result) {
            case Result.Success<RoleDeleted> s -> Response.noContent().build();
            case Result.Failure<RoleDeleted> f -> mapErrorToResponse(f.error());
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

    // ==================== DTOs ====================

    @Schema(description = "Role details")
    public record RoleDto(
        @Schema(description = "Full role name (e.g. app:admin)") String name,
        @Schema(description = "Application code") String applicationCode,
        @Schema(description = "Display name") String displayName,
        @Schema(description = "Short name without app prefix") String shortName,
        @Schema(description = "Role description") String description,
        @Schema(description = "Permission strings") Set<String> permissions,
        @Schema(description = "Source (CODE, DATABASE, SDK)") String source,
        @Schema(description = "Whether clients can manage assignment") boolean clientManaged,
        @Schema(description = "Creation timestamp") Instant createdAt,
        @Schema(description = "Last update timestamp") Instant updatedAt
    ) {}

    @Schema(description = "List of roles")
    public record RoleListResponse(
        @Schema(description = "Role list") List<RoleDto> roles,
        @Schema(description = "Total count") int total
    ) {}

    @Schema(description = "Create role request")
    public record CreateRoleRequest(
        @Schema(required = true, description = "Application code") String applicationCode,
        @Schema(required = true, description = "Role name (will be prefixed with app code)") String name,
        @Schema(description = "Display name") String displayName,
        @Schema(description = "Role description") String description,
        @Schema(description = "Structured permission inputs") List<PermissionInputDto> permissions,
        @Schema(description = "Whether clients can manage assignment") Boolean clientManaged
    ) {}

    @Schema(description = "Update role request")
    public record UpdateRoleRequest(
        @Schema(description = "Display name") String displayName,
        @Schema(description = "Role description") String description,
        @Schema(description = "Structured permission inputs") List<PermissionInputDto> permissions,
        @Schema(description = "Whether clients can manage assignment") Boolean clientManaged
    ) {}

    @Schema(description = "Structured permission input")
    public record PermissionInputDto(
        @Schema(description = "Application code") String application,
        @Schema(description = "Bounded context") String context,
        @Schema(description = "Resource/entity") String aggregate,
        @Schema(description = "Operation") String action
    ) {
        public PermissionInput toPermissionInput() {
            return new PermissionInput(application, context, aggregate, action);
        }
    }

    @Schema(description = "Error response")
    public record ErrorResponse(
        @Schema(description = "Error code") String code,
        @Schema(description = "Error message") String message
    ) {}
}
