package tech.flowcatalyst.platform.admin;

import jakarta.inject.Inject;
import jakarta.validation.Valid;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
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
import tech.flowcatalyst.platform.authentication.AuthProvider;
import tech.flowcatalyst.platform.authentication.EmbeddedModeOnly;
import tech.flowcatalyst.platform.authorization.AuthorizationService;
import tech.flowcatalyst.platform.authorization.platform.PlatformIamPermissions;
import tech.flowcatalyst.platform.client.AuthConfigType;
import tech.flowcatalyst.platform.client.ClientAuthConfig;
import tech.flowcatalyst.platform.client.ClientAuthConfigService;
import tech.flowcatalyst.platform.security.secrets.SecretProvider.ValidationResult;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TypedId;

import java.time.Instant;
import java.util.List;

/**
 * Admin API for managing domain authentication configurations.
 *
 * Provides CRUD operations for ClientAuthConfig including:
 * - Configure email domains for INTERNAL or OIDC authentication
 * - Manage OIDC settings (issuer URL, client ID, secret reference)
 * - Validate secret references without exposing values
 *
 * All operations require admin-level permissions.
 * Secret resolution is restricted to Super Admin only.
 */
@Path("/api/admin/auth-configs")
@Tag(name = "BFF - Auth Config Admin", description = "Administrative operations for domain authentication configuration")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@EmbeddedModeOnly
public class ClientAuthConfigAdminResource {

    private static final Logger LOG = Logger.getLogger(ClientAuthConfigAdminResource.class);

    @Inject
    ClientAuthConfigService authConfigService;

    @Inject
    AuditContext auditContext;

    @Inject
    AuthorizationService authorizationService;

    // ==================== List & Get Operations ====================

    /**
     * List all auth configurations.
     */
    @GET
    @Operation(summary = "List all auth configurations", description = "Returns all domain auth configurations")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of auth configurations",
            content = @Content(schema = @Schema(implementation = AuthConfigListResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response listAuthConfigs(
            @QueryParam("clientId") @Parameter(description = "Filter by client ID") String clientId) {

        auditContext.requirePrincipalId();

        List<ClientAuthConfig> configs;
        if (clientId != null) {
            configs = authConfigService.findByClientId(clientId);
        } else {
            configs = authConfigService.listAll();
        }

        List<AuthConfigDto> dtos = configs.stream()
            .map(this::toDto)
            .toList();

        return Response.ok(new AuthConfigListResponse(dtos, dtos.size())).build();
    }

    /**
     * Get a specific auth configuration by ID.
     */
    @GET
    @Path("/{id}")
    @Operation(summary = "Get auth configuration by ID")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Auth configuration details",
            content = @Content(schema = @Schema(implementation = AuthConfigDto.class))),
        @APIResponse(responseCode = "404", description = "Auth configuration not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated")
    })
    public Response getAuthConfig(@PathParam("id") String id) {

        auditContext.requirePrincipalId();

        return authConfigService.findById(id)
            .map(config -> Response.ok(toDto(config)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("Auth configuration not found"))
                .build());
    }

    /**
     * Get auth configuration by email domain.
     */
    @GET
    @Path("/by-domain/{domain}")
    @Operation(summary = "Get auth configuration by email domain")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Auth configuration details"),
        @APIResponse(responseCode = "404", description = "No configuration for this domain")
    })
    public Response getAuthConfigByDomain(@PathParam("domain") String domain) {

        auditContext.requirePrincipalId();

        return authConfigService.findByEmailDomain(domain)
            .map(config -> Response.ok(toDto(config)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("No configuration for domain: " + domain))
                .build());
    }

    // ==================== Create Operations ====================

    /**
     * Create an INTERNAL auth configuration.
     */
    @POST
    @Path("/internal")
    @Operation(summary = "Create internal auth configuration",
        description = "Configure a domain to use internal (password) authentication")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Configuration created",
            content = @Content(schema = @Schema(implementation = AuthConfigDto.class))),
        @APIResponse(responseCode = "400", description = "Invalid request or domain already configured"),
        @APIResponse(responseCode = "401", description = "Not authenticated")
    })
    public Response createInternalConfig(
            @Valid CreateInternalConfigRequest request,
            @Context UriInfo uriInfo) {

        String principalId = auditContext.requirePrincipalId();

        // Require IDP manage permission
        if (!authorizationService.hasPermission(principalId, PlatformIamPermissions.IDP_MANAGE.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.IDP_MANAGE.toPermissionString()))
                .build();
        }

        try {
            ClientAuthConfig config = authConfigService.createInternal(
                request.emailDomain(),
                request.effectiveConfigType(),
                request.effectivePrimaryClientId());
            LOG.infof("Created INTERNAL auth config for domain: %s (type: %s) by principal %s",
                config.emailDomain, config.configType, principalId);

            return Response.status(Response.Status.CREATED)
                .entity(toDto(config))
                .location(uriInfo.getAbsolutePathBuilder()
                    .path("../" + config.id).build())
                .build();
        } catch (IllegalArgumentException | IllegalStateException e) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse(e.getMessage()))
                .build();
        }
    }

    /**
     * Create an OIDC auth configuration.
     */
    @POST
    @Path("/oidc")
    @Operation(summary = "Create OIDC auth configuration",
        description = "Configure a domain to use external OIDC identity provider")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Configuration created",
            content = @Content(schema = @Schema(implementation = AuthConfigDto.class))),
        @APIResponse(responseCode = "400", description = "Invalid request or domain already configured"),
        @APIResponse(responseCode = "401", description = "Not authenticated")
    })
    public Response createOidcConfig(
            @Valid CreateOidcConfigRequest request,
            @Context UriInfo uriInfo) {

        String principalId = auditContext.requirePrincipalId();

        // Require IDP manage permission
        if (!authorizationService.hasPermission(principalId, PlatformIamPermissions.IDP_MANAGE.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.IDP_MANAGE.toPermissionString()))
                .build();
        }

        try {
            ClientAuthConfig config = authConfigService.createOidc(
                request.emailDomain(),
                request.effectiveConfigType(),
                request.effectivePrimaryClientId(),
                request.oidcIssuerUrl(),
                request.oidcClientId(),
                request.oidcClientSecretRef(),
                request.oidcMultiTenant() != null && request.oidcMultiTenant(),
                request.oidcIssuerPattern());

            LOG.infof("Created OIDC auth config for domain: %s (type: %s, multiTenant: %s) by principal %s",
                config.emailDomain, config.configType, config.oidcMultiTenant, principalId);

            return Response.status(Response.Status.CREATED)
                .entity(toDto(config))
                .location(uriInfo.getAbsolutePathBuilder()
                    .path("../" + config.id).build())
                .build();
        } catch (IllegalArgumentException | IllegalStateException e) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse(e.getMessage()))
                .build();
        }
    }

    // ==================== Update Operations ====================

    /**
     * Update an OIDC auth configuration.
     */
    @PUT
    @Path("/{id}/oidc")
    @Operation(summary = "Update OIDC configuration")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Configuration updated"),
        @APIResponse(responseCode = "404", description = "Configuration not found"),
        @APIResponse(responseCode = "400", description = "Invalid request or not an OIDC config")
    })
    public Response updateOidcConfig(
            @PathParam("id") String id,
            @Valid UpdateOidcConfigRequest request) {

        String principalId = auditContext.requirePrincipalId();

        // Require IDP manage permission
        if (!authorizationService.hasPermission(principalId, PlatformIamPermissions.IDP_MANAGE.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.IDP_MANAGE.toPermissionString()))
                .build();
        }

        try {
            ClientAuthConfig config = authConfigService.updateOidc(
                id,
                request.oidcIssuerUrl(),
                request.oidcClientId(),
                request.oidcClientSecretRef(),
                request.oidcMultiTenant(),
                request.oidcIssuerPattern());

            LOG.infof("Updated OIDC auth config for domain: %s (multiTenant: %s) by principal %s",
                config.emailDomain, config.oidcMultiTenant, principalId);

            return Response.ok(toDto(config)).build();
        } catch (IllegalArgumentException e) {
            if (e.getMessage().contains("not found")) {
                return Response.status(Response.Status.NOT_FOUND)
                    .entity(new ErrorResponse(e.getMessage()))
                    .build();
            }
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse(e.getMessage()))
                .build();
        }
    }

    // ==================== Update Client Binding ====================

    /**
     * Update the client binding for an auth configuration.
     */
    @PUT
    @Path("/{id}/client-binding")
    @Operation(summary = "Update client binding",
        description = "Change which client this domain is bound to, or remove binding for platform-wide access")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Client binding updated"),
        @APIResponse(responseCode = "404", description = "Configuration not found"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response updateClientBinding(
            @PathParam("id") String id,
            @Valid UpdateClientBindingRequest request) {

        String principalId = auditContext.requirePrincipalId();

        // Require IDP manage permission
        if (!authorizationService.hasPermission(principalId, PlatformIamPermissions.IDP_MANAGE.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.IDP_MANAGE.toPermissionString()))
                .build();
        }

        try {
            ClientAuthConfig config = authConfigService.updateClientBinding(id, request.clientId());

            LOG.infof("Updated client binding for domain: %s to %s by principal %s",
                config.emailDomain, request.clientId() != null ? request.clientId() : "platform-wide", principalId);

            return Response.ok(toDto(config)).build();
        } catch (IllegalArgumentException e) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse(e.getMessage()))
                .build();
        }
    }

    /**
     * Update the config type for an auth configuration.
     */
    @PUT
    @Path("/{id}/config-type")
    @Operation(summary = "Update config type",
        description = "Change the config type (ANCHOR, PARTNER, CLIENT). This will reset client associations.")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Config type updated"),
        @APIResponse(responseCode = "404", description = "Configuration not found"),
        @APIResponse(responseCode = "400", description = "Invalid config type or constraint violation")
    })
    public Response updateConfigType(
            @PathParam("id") String id,
            @Valid UpdateConfigTypeRequest request) {

        String principalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(principalId, PlatformIamPermissions.IDP_MANAGE.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.IDP_MANAGE.toPermissionString()))
                .build();
        }

        try {
            ClientAuthConfig config = authConfigService.updateConfigType(id, request.configType(), request.primaryClientId());

            LOG.infof("Updated config type for domain: %s to %s by principal %s",
                config.emailDomain, config.configType, principalId);

            return Response.ok(toDto(config)).build();
        } catch (IllegalArgumentException | IllegalStateException e) {
            if (e.getMessage().contains("not found")) {
                return Response.status(Response.Status.NOT_FOUND)
                    .entity(new ErrorResponse(e.getMessage()))
                    .build();
            }
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse(e.getMessage()))
                .build();
        }
    }

    /**
     * Update additional clients for a CLIENT type configuration.
     */
    @PUT
    @Path("/{id}/additional-clients")
    @Operation(summary = "Update additional clients",
        description = "Set additional client IDs for CLIENT type configurations")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Additional clients updated"),
        @APIResponse(responseCode = "404", description = "Configuration not found"),
        @APIResponse(responseCode = "400", description = "Invalid request or not a CLIENT type config")
    })
    public Response updateAdditionalClients(
            @PathParam("id") String id,
            @Valid UpdateAdditionalClientsRequest request) {

        String principalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(principalId, PlatformIamPermissions.IDP_MANAGE.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.IDP_MANAGE.toPermissionString()))
                .build();
        }

        try {
            ClientAuthConfig config = authConfigService.updateAdditionalClients(id, request.clientIds());

            LOG.infof("Updated additional clients for domain: %s to %s by principal %s",
                config.emailDomain, config.additionalClientIds, principalId);

            return Response.ok(toDto(config)).build();
        } catch (IllegalArgumentException | IllegalStateException e) {
            if (e.getMessage().contains("not found")) {
                return Response.status(Response.Status.NOT_FOUND)
                    .entity(new ErrorResponse(e.getMessage()))
                    .build();
            }
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse(e.getMessage()))
                .build();
        }
    }

    /**
     * Update granted clients for a PARTNER type configuration.
     */
    @PUT
    @Path("/{id}/granted-clients")
    @Operation(summary = "Update granted clients",
        description = "Set granted client IDs for PARTNER type configurations")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Granted clients updated"),
        @APIResponse(responseCode = "404", description = "Configuration not found"),
        @APIResponse(responseCode = "400", description = "Invalid request or not a PARTNER type config")
    })
    public Response updateGrantedClients(
            @PathParam("id") String id,
            @Valid UpdateGrantedClientsRequest request) {

        String principalId = auditContext.requirePrincipalId();

        if (!authorizationService.hasPermission(principalId, PlatformIamPermissions.IDP_MANAGE.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.IDP_MANAGE.toPermissionString()))
                .build();
        }

        try {
            ClientAuthConfig config = authConfigService.updateGrantedClients(id, request.clientIds());

            LOG.infof("Updated granted clients for domain: %s to %s by principal %s",
                config.emailDomain, config.grantedClientIds, principalId);

            return Response.ok(toDto(config)).build();
        } catch (IllegalArgumentException | IllegalStateException e) {
            if (e.getMessage().contains("not found")) {
                return Response.status(Response.Status.NOT_FOUND)
                    .entity(new ErrorResponse(e.getMessage()))
                    .build();
            }
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse(e.getMessage()))
                .build();
        }
    }

    // ==================== Delete Operations ====================

    /**
     * Delete an auth configuration.
     */
    @DELETE
    @Path("/{id}")
    @Operation(summary = "Delete auth configuration")
    @APIResponses({
        @APIResponse(responseCode = "204", description = "Configuration deleted"),
        @APIResponse(responseCode = "404", description = "Configuration not found")
    })
    public Response deleteAuthConfig(@PathParam("id") String id) {

        String principalId = auditContext.requirePrincipalId();

        // Require IDP manage permission
        if (!authorizationService.hasPermission(principalId, PlatformIamPermissions.IDP_MANAGE.toPermissionString())) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Missing required permission: " + PlatformIamPermissions.IDP_MANAGE.toPermissionString()))
                .build();
        }

        try {
            authConfigService.delete(id);
            LOG.infof("Deleted auth config %s by principal %s", id, principalId);
            return Response.noContent().build();
        } catch (IllegalArgumentException e) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse(e.getMessage()))
                .build();
        }
    }

    // ==================== Secret Validation ====================

    /**
     * Validate a secret reference without resolving the actual value.
     * This checks that the secret exists and is accessible.
     */
    @POST
    @Path("/validate-secret")
    @Operation(summary = "Validate secret reference",
        description = "Checks that a secret reference is valid and accessible without returning the value")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Validation result",
            content = @Content(schema = @Schema(implementation = SecretValidationResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated")
    })
    public Response validateSecretReference(@Valid ValidateSecretRequest request) {

        auditContext.requirePrincipalId();

        ValidationResult result = authConfigService.validateSecretReference(request.secretRef());

        return Response.ok(new SecretValidationResponse(
            result.valid(),
            result.message()
        )).build();
    }

    // ==================== Helper Methods ====================

    private AuthConfigDto toDto(ClientAuthConfig config) {
        return new AuthConfigDto(
            TypedId.Ops.serialize(EntityType.CLIENT_AUTH_CONFIG, config.id),
            config.emailDomain,
            config.getEffectiveConfigType(),
            TypedId.Ops.serialize(EntityType.CLIENT, config.getEffectivePrimaryClientId()),
            config.additionalClientIds != null
                ? config.additionalClientIds.stream()
                    .map(id -> TypedId.Ops.serialize(EntityType.CLIENT, id))
                    .toList()
                : List.of(),
            config.grantedClientIds != null
                ? config.grantedClientIds.stream()
                    .map(id -> TypedId.Ops.serialize(EntityType.CLIENT, id))
                    .toList()
                : List.of(),
            TypedId.Ops.serialize(EntityType.CLIENT, config.clientId), // Backwards compatibility
            config.authProvider,
            config.oidcIssuerUrl,
            config.oidcClientId,
            config.hasClientSecret(),
            config.oidcMultiTenant,
            config.oidcIssuerPattern,
            config.createdAt,
            config.updatedAt
        );
    }

    // ==================== DTOs ====================

    public record AuthConfigDto(
        String id,
        String emailDomain,
        AuthConfigType configType,
        String primaryClientId,
        List<String> additionalClientIds,
        List<String> grantedClientIds,
        @Deprecated String clientId, // Backwards compatibility
        AuthProvider authProvider,
        String oidcIssuerUrl,
        String oidcClientId,
        boolean hasClientSecret,
        boolean oidcMultiTenant,
        String oidcIssuerPattern,
        Instant createdAt,
        Instant updatedAt
    ) {}

    public record AuthConfigListResponse(
        List<AuthConfigDto> configs,
        int total
    ) {}

    public record CreateInternalConfigRequest(
        @NotBlank(message = "Email domain is required")
        String emailDomain,

        @NotNull(message = "Config type is required")
        @Parameter(description = "Config type: ANCHOR, PARTNER, or CLIENT")
        AuthConfigType configType,

        @Parameter(description = "Primary client ID (required for CLIENT type)")
        String primaryClientId,

        @Deprecated
        @Parameter(description = "Client ID (deprecated, use primaryClientId)")
        String clientId
    ) {
        // Allow backwards compatibility: if clientId is set but primaryClientId is not, use clientId
        public String effectivePrimaryClientId() {
            return primaryClientId != null ? primaryClientId : clientId;
        }

        // Derive config type from clientId if not explicitly set (backwards compatibility)
        public AuthConfigType effectiveConfigType() {
            if (configType != null) return configType;
            return clientId != null ? AuthConfigType.CLIENT : AuthConfigType.ANCHOR;
        }
    }

    public record CreateOidcConfigRequest(
        @NotBlank(message = "Email domain is required")
        String emailDomain,

        @NotNull(message = "Config type is required")
        @Parameter(description = "Config type: ANCHOR, PARTNER, or CLIENT")
        AuthConfigType configType,

        @Parameter(description = "Primary client ID (required for CLIENT type)")
        String primaryClientId,

        @Deprecated
        @Parameter(description = "Client ID (deprecated, use primaryClientId)")
        String clientId,

        @NotBlank(message = "OIDC issuer URL is required")
        String oidcIssuerUrl,

        @NotBlank(message = "OIDC client ID is required")
        String oidcClientId,

        @Parameter(description = "Secret reference URI (e.g., aws-sm://secret-name)")
        String oidcClientSecretRef,

        @Parameter(description = "Whether this is a multi-tenant OIDC configuration (e.g., Entra ID)")
        Boolean oidcMultiTenant,

        @Parameter(description = "Pattern for validating multi-tenant issuers (e.g., https://login.microsoftonline.com/{tenantId}/v2.0)")
        String oidcIssuerPattern
    ) {
        // Allow backwards compatibility: if clientId is set but primaryClientId is not, use clientId
        public String effectivePrimaryClientId() {
            return primaryClientId != null ? primaryClientId : clientId;
        }

        // Derive config type from clientId if not explicitly set (backwards compatibility)
        public AuthConfigType effectiveConfigType() {
            if (configType != null) return configType;
            return clientId != null ? AuthConfigType.CLIENT : AuthConfigType.ANCHOR;
        }
    }

    public record UpdateOidcConfigRequest(
        @NotBlank(message = "OIDC issuer URL is required")
        String oidcIssuerUrl,

        @NotBlank(message = "OIDC client ID is required")
        String oidcClientId,

        @Parameter(description = "Secret reference URI (leave empty to keep existing)")
        String oidcClientSecretRef,

        @Parameter(description = "Whether this is a multi-tenant OIDC configuration (null to keep existing)")
        Boolean oidcMultiTenant,

        @Parameter(description = "Pattern for validating multi-tenant issuers (null to keep existing)")
        String oidcIssuerPattern
    ) {}

    public record ValidateSecretRequest(
        @NotBlank(message = "Secret reference is required")
        String secretRef
    ) {}

    public record UpdateClientBindingRequest(
        @Parameter(description = "Client ID to bind to, or null for platform-wide (anchor) access")
        String clientId
    ) {}

    public record UpdateConfigTypeRequest(
        @NotNull(message = "Config type is required")
        @Parameter(description = "New config type: ANCHOR, PARTNER, or CLIENT")
        AuthConfigType configType,

        @Parameter(description = "Primary client ID (required for CLIENT type, must be null for others)")
        String primaryClientId
    ) {}

    public record UpdateAdditionalClientsRequest(
        @Parameter(description = "List of additional client IDs (for CLIENT type configs)")
        List<String> clientIds
    ) {}

    public record UpdateGrantedClientsRequest(
        @Parameter(description = "List of granted client IDs (for PARTNER type configs)")
        List<String> clientIds
    ) {}

    public record SecretValidationResponse(
        boolean valid,
        String message
    ) {}

    public record ErrorResponse(
        String error
    ) {}
}
