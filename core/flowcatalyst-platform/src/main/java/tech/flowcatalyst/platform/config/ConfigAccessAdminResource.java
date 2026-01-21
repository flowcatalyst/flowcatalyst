package tech.flowcatalyst.platform.config;

import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponses;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.authentication.EmbeddedModeOnly;
import tech.flowcatalyst.platform.authorization.RoleService;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.util.List;
import java.util.Map;
import java.util.Set;

/**
 * REST resource for managing config access grants.
 *
 * Allows platform admins to grant/revoke config access to roles.
 * Only platform:super-admin can manage access grants.
 */
@Path("/api/admin/config-access")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@Tag(name = "Platform Config Access", description = "Manage role-based access to platform configurations")
@EmbeddedModeOnly
public class ConfigAccessAdminResource {

    @Inject
    PlatformConfigAccessRepository accessRepo;

    @Inject
    RoleService roleService;

    @Inject
    AuditContext auditContext;

    @GET
    @Path("/{appCode}")
    @Operation(summary = "List access grants for an application")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of access grants"),
        @APIResponse(responseCode = "403", description = "Access denied")
    })
    public Response listAccessGrants(@PathParam("appCode") String appCode) {
        if (!isSuperAdmin()) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("ACCESS_DENIED", "Only super admins can manage config access"))
                .build();
        }

        List<PlatformConfigAccess> grants = accessRepo.findByApplicationCode(appCode);
        List<AccessGrantResponse> responses = grants.stream()
            .map(AccessGrantResponse::from)
            .toList();

        return Response.ok(new AccessGrantListResponse(responses)).build();
    }

    @POST
    @Path("/{appCode}")
    @Operation(summary = "Grant config access to a role")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Access granted"),
        @APIResponse(responseCode = "400", description = "Invalid request"),
        @APIResponse(responseCode = "403", description = "Access denied"),
        @APIResponse(responseCode = "409", description = "Grant already exists")
    })
    public Response grantAccess(
            @PathParam("appCode") String appCode,
            GrantAccessRequest request) {

        if (!isSuperAdmin()) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("ACCESS_DENIED", "Only super admins can manage config access"))
                .build();
        }

        if (request.roleCode() == null || request.roleCode().isBlank()) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse("ROLE_CODE_REQUIRED", "Role code is required"))
                .build();
        }

        // Check if grant already exists
        if (accessRepo.findByApplicationAndRole(appCode, request.roleCode()).isPresent()) {
            return Response.status(Response.Status.CONFLICT)
                .entity(new ErrorResponse("GRANT_EXISTS",
                    "Access grant already exists for " + request.roleCode() + " on " + appCode))
                .build();
        }

        PlatformConfigAccess access = new PlatformConfigAccess();
        access.id = TsidGenerator.generate(EntityType.CONFIG_ACCESS);
        access.applicationCode = appCode;
        access.roleCode = request.roleCode();
        access.canRead = request.canRead() != null ? request.canRead() : true;
        access.canWrite = request.canWrite() != null ? request.canWrite() : false;

        accessRepo.persist(access);

        return Response.status(Response.Status.CREATED)
            .entity(AccessGrantResponse.from(access))
            .build();
    }

    @PUT
    @Path("/{appCode}/{roleCode}")
    @Operation(summary = "Update config access for a role")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Access updated"),
        @APIResponse(responseCode = "403", description = "Access denied"),
        @APIResponse(responseCode = "404", description = "Grant not found")
    })
    public Response updateAccess(
            @PathParam("appCode") String appCode,
            @PathParam("roleCode") String roleCode,
            UpdateAccessRequest request) {

        if (!isSuperAdmin()) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("ACCESS_DENIED", "Only super admins can manage config access"))
                .build();
        }

        return accessRepo.findByApplicationAndRole(appCode, roleCode)
            .map(access -> {
                if (request.canRead() != null) {
                    access.canRead = request.canRead();
                }
                if (request.canWrite() != null) {
                    access.canWrite = request.canWrite();
                }
                accessRepo.update(access);
                return Response.ok(AccessGrantResponse.from(access)).build();
            })
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("GRANT_NOT_FOUND",
                    "Access grant not found for " + roleCode + " on " + appCode))
                .build());
    }

    @DELETE
    @Path("/{appCode}/{roleCode}")
    @Operation(summary = "Revoke config access from a role")
    @APIResponses({
        @APIResponse(responseCode = "204", description = "Access revoked"),
        @APIResponse(responseCode = "403", description = "Access denied"),
        @APIResponse(responseCode = "404", description = "Grant not found")
    })
    public Response revokeAccess(
            @PathParam("appCode") String appCode,
            @PathParam("roleCode") String roleCode) {

        if (!isSuperAdmin()) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("ACCESS_DENIED", "Only super admins can manage config access"))
                .build();
        }

        boolean deleted = accessRepo.deleteByApplicationAndRole(appCode, roleCode);

        if (!deleted) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("GRANT_NOT_FOUND",
                    "Access grant not found for " + roleCode + " on " + appCode))
                .build();
        }

        return Response.noContent().build();
    }

    private boolean isSuperAdmin() {
        String principalId = auditContext.requirePrincipalId();
        Set<String> roles = roleService.findRoleNamesByPrincipal(principalId);
        return roles.contains(PlatformConfigService.SUPER_ADMIN_ROLE);
    }

    // ========================================================================
    // DTOs
    // ========================================================================

    public record AccessGrantListResponse(List<AccessGrantResponse> items) {}

    public record AccessGrantResponse(
        String id,
        String applicationCode,
        String roleCode,
        boolean canRead,
        boolean canWrite,
        String createdAt
    ) {
        public static AccessGrantResponse from(PlatformConfigAccess access) {
            return new AccessGrantResponse(
                access.id,
                access.applicationCode,
                access.roleCode,
                access.canRead,
                access.canWrite,
                access.createdAt != null ? access.createdAt.toString() : null
            );
        }
    }

    public record GrantAccessRequest(
        String roleCode,
        Boolean canRead,
        Boolean canWrite
    ) {}

    public record UpdateAccessRequest(
        Boolean canRead,
        Boolean canWrite
    ) {}

    public record ErrorResponse(String code, String message, Map<String, Object> details) {
        public ErrorResponse(String code, String message) {
            this(code, message, Map.of());
        }
    }
}
