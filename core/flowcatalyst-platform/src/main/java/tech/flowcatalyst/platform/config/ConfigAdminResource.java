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

import tech.flowcatalyst.platform.authorization.RoleService;

import java.util.List;
import java.util.Map;
import java.util.Set;

/**
 * REST resource for managing platform configurations.
 *
 * Provides CRUD operations for configuration entries organized by:
 * - applicationCode: References Application.code (e.g., "platform", "tms", "wms")
 * - section: Logical grouping (e.g., "login", "ui", "api")
 * - property: Specific setting (e.g., "brandName", "primaryColor")
 *
 * Access control:
 * - platform:super-admin can access all configs
 * - Other roles require explicit grants via PlatformConfigAccess
 */
@Path("/api/admin/config")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@Tag(name = "Platform Configuration Admin", description = "Manage platform configuration settings")
public class ConfigAdminResource {

    @Inject
    PlatformConfigService configService;

    @Inject
    RoleService roleService;

    @Inject
    AuditContext auditContext;

    @GET
    @Path("/{appCode}")
    @Operation(summary = "List configs for an application")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of configs"),
        @APIResponse(responseCode = "403", description = "Access denied")
    })
    public Response listConfigs(
            @PathParam("appCode") String appCode,
            @QueryParam("scope") @DefaultValue("GLOBAL") ConfigScope scope,
            @QueryParam("clientId") String clientId) {

        if (!checkReadAccess(appCode)) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("ACCESS_DENIED", "No read access to " + appCode + " configs"))
                .build();
        }

        List<PlatformConfig> configs = configService.getConfigs(appCode, scope, clientId);
        List<ConfigResponse> responses = configs.stream()
            .map(ConfigResponse::from)
            .toList();

        return Response.ok(new ConfigListResponse(responses)).build();
    }

    @GET
    @Path("/{appCode}/{section}")
    @Operation(summary = "List configs for a section")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Map of property names to values"),
        @APIResponse(responseCode = "403", description = "Access denied")
    })
    public Response listSectionConfigs(
            @PathParam("appCode") String appCode,
            @PathParam("section") String section,
            @QueryParam("scope") @DefaultValue("GLOBAL") ConfigScope scope,
            @QueryParam("clientId") String clientId) {

        if (!checkReadAccess(appCode)) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("ACCESS_DENIED", "No read access to " + appCode + " configs"))
                .build();
        }

        Map<String, String> values = configService.getSection(appCode, section, scope, clientId, false);
        return Response.ok(new SectionResponse(appCode, section, scope.name(), clientId, values)).build();
    }

    @GET
    @Path("/{appCode}/{section}/{property}")
    @Operation(summary = "Get a single config value")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Config value"),
        @APIResponse(responseCode = "403", description = "Access denied"),
        @APIResponse(responseCode = "404", description = "Config not found")
    })
    public Response getConfig(
            @PathParam("appCode") String appCode,
            @PathParam("section") String section,
            @PathParam("property") String property,
            @QueryParam("scope") @DefaultValue("GLOBAL") ConfigScope scope,
            @QueryParam("clientId") String clientId) {

        if (!checkReadAccess(appCode)) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("ACCESS_DENIED", "No read access to " + appCode + " configs"))
                .build();
        }

        var configOpt = configService.getValue(appCode, section, property, scope, clientId, false);
        return configOpt.map(value ->
            Response.ok(new ValueResponse(appCode, section, property, scope.name(), clientId, value)).build()
        ).orElse(
            Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("CONFIG_NOT_FOUND",
                    "Config not found: " + appCode + "." + section + "." + property))
                .build()
        );
    }

    @PUT
    @Path("/{appCode}/{section}/{property}")
    @Operation(summary = "Set a config value")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Config updated"),
        @APIResponse(responseCode = "201", description = "Config created"),
        @APIResponse(responseCode = "400", description = "Invalid request"),
        @APIResponse(responseCode = "403", description = "Access denied")
    })
    public Response setConfig(
            @PathParam("appCode") String appCode,
            @PathParam("section") String section,
            @PathParam("property") String property,
            @QueryParam("scope") @DefaultValue("GLOBAL") ConfigScope scope,
            @QueryParam("clientId") String clientId,
            SetConfigRequest request) {

        if (!checkWriteAccess(appCode)) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("ACCESS_DENIED", "No write access to " + appCode + " configs"))
                .build();
        }

        if (request.value() == null) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse("VALUE_REQUIRED", "Config value is required"))
                .build();
        }

        ConfigValueType valueType = request.valueType() != null
            ? request.valueType()
            : ConfigValueType.PLAIN;

        // Check if this is an update or create
        boolean exists = configService.getValue(appCode, section, property, scope, clientId, false).isPresent();

        PlatformConfig config = configService.setValue(
            appCode, section, property, scope, clientId,
            request.value(), valueType, request.description()
        );

        return Response.status(exists ? Response.Status.OK : Response.Status.CREATED)
            .entity(ConfigResponse.from(config))
            .build();
    }

    @DELETE
    @Path("/{appCode}/{section}/{property}")
    @Operation(summary = "Delete a config")
    @APIResponses({
        @APIResponse(responseCode = "204", description = "Config deleted"),
        @APIResponse(responseCode = "403", description = "Access denied"),
        @APIResponse(responseCode = "404", description = "Config not found")
    })
    public Response deleteConfig(
            @PathParam("appCode") String appCode,
            @PathParam("section") String section,
            @PathParam("property") String property,
            @QueryParam("scope") @DefaultValue("GLOBAL") ConfigScope scope,
            @QueryParam("clientId") String clientId) {

        if (!checkWriteAccess(appCode)) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("ACCESS_DENIED", "No write access to " + appCode + " configs"))
                .build();
        }

        boolean deleted = configService.delete(appCode, section, property, scope, clientId);

        if (!deleted) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("CONFIG_NOT_FOUND",
                    "Config not found: " + appCode + "." + section + "." + property))
                .build();
        }

        return Response.noContent().build();
    }

    private boolean checkReadAccess(String appCode) {
        var principalId = auditContext.requirePrincipalId();
        var roles = roleService.findRoleNamesByPrincipal(principalId);
        var log = org.jboss.logging.Logger.getLogger(ConfigAdminResource.class);
        log.infof("checkReadAccess: principalId=%s, roles=%s, appCode=%s", principalId, roles, appCode);
        return configService.canAccess(appCode, roles, false);
    }

    private boolean checkWriteAccess(String appCode) {
        var principalId = auditContext.requirePrincipalId();
        var roles = roleService.findRoleNamesByPrincipal(principalId);
        var log = org.jboss.logging.Logger.getLogger(ConfigAdminResource.class);
        log.infof("checkWriteAccess: principalId=%s, roles=%s, appCode=%s", principalId, roles, appCode);
        return configService.canAccess(appCode, roles, true);
    }

    // ========================================================================
    // DTOs
    // ========================================================================

    public record ConfigListResponse(List<ConfigResponse> items) {}

    public record ConfigResponse(
        String id,
        String applicationCode,
        String section,
        String property,
        String scope,
        String clientId,
        String valueType,
        String value,
        String description,
        String createdAt,
        String updatedAt
    ) {
        public static ConfigResponse from(PlatformConfig config) {
            return new ConfigResponse(
                config.id,
                config.applicationCode,
                config.section,
                config.property,
                config.scope.name(),
                config.clientId,
                config.valueType.name(),
                config.valueType == ConfigValueType.SECRET ? "***" : config.value,
                config.description,
                config.createdAt != null ? config.createdAt.toString() : null,
                config.updatedAt != null ? config.updatedAt.toString() : null
            );
        }
    }

    public record SectionResponse(
        String applicationCode,
        String section,
        String scope,
        String clientId,
        Map<String, String> values
    ) {}

    public record ValueResponse(
        String applicationCode,
        String section,
        String property,
        String scope,
        String clientId,
        String value
    ) {}

    public record SetConfigRequest(
        String value,
        ConfigValueType valueType,
        String description
    ) {}

    public record ErrorResponse(String code, String message, Map<String, Object> details) {
        public ErrorResponse(String code, String message) {
            this(code, message, Map.of());
        }
    }
}
