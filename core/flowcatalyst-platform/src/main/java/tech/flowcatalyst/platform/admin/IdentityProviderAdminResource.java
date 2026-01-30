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
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.authentication.EmbeddedModeOnly;
import tech.flowcatalyst.platform.authentication.idp.IdentityProvider;
import tech.flowcatalyst.platform.authentication.idp.IdentityProviderOperations;
import tech.flowcatalyst.platform.authentication.idp.IdentityProviderType;
import tech.flowcatalyst.platform.authentication.idp.events.IdentityProviderCreated;
import tech.flowcatalyst.platform.authentication.idp.events.IdentityProviderDeleted;
import tech.flowcatalyst.platform.authentication.idp.events.IdentityProviderUpdated;
import tech.flowcatalyst.platform.authentication.idp.operations.createidp.CreateIdentityProviderCommand;
import tech.flowcatalyst.platform.authentication.idp.operations.deleteidp.DeleteIdentityProviderCommand;
import tech.flowcatalyst.platform.authentication.idp.operations.updateidp.UpdateIdentityProviderCommand;
import tech.flowcatalyst.platform.authorization.AuthorizationService;
import tech.flowcatalyst.platform.authorization.platform.PlatformIamPermissions;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.TracingContext;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.time.Instant;
import java.util.List;
import java.util.Map;

/**
 * Admin API for managing Identity Providers.
 *
 * <p>Identity Providers define how users authenticate - either via internal
 * password authentication or through external OIDC providers like Okta or Entra ID.
 */
@Path("/api/admin/identity-providers")
@Tag(name = "BFF - Identity Provider Admin", description = "Manage identity provider configurations")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@EmbeddedModeOnly
@jakarta.transaction.Transactional
public class IdentityProviderAdminResource {

    @Inject
    IdentityProviderOperations operations;

    @Inject
    AuditContext auditContext;

    @Inject
    TracingContext tracingContext;

    @Inject
    AuthorizationService authorizationService;

    // ==================== List ====================

    @GET
    @Operation(operationId = "listIdentityProviders", summary = "List all identity providers",
        description = "Returns all identity providers. Filter by type if needed.")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of identity providers",
            content = @Content(schema = @Schema(implementation = IdentityProviderListResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated")
    })
    public Response listIdentityProviders(@QueryParam("type") String type) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformIamPermissions.IDP_VIEW);

        List<IdentityProvider> providers;
        if (type != null && !type.isBlank()) {
            try {
                var typeEnum = IdentityProviderType.valueOf(type.toUpperCase());
                providers = operations.findByType(typeEnum);
            } catch (IllegalArgumentException e) {
                return Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ErrorResponse("INVALID_TYPE", "Invalid type. Must be INTERNAL or OIDC"))
                    .build();
            }
        } else {
            providers = operations.findAll();
        }

        var dtos = providers.stream()
            .map(this::toDto)
            .sorted((a, b) -> a.name().compareTo(b.name()))
            .toList();

        return Response.ok(new IdentityProviderListResponse(dtos, dtos.size())).build();
    }

    // ==================== Get by ID ====================

    @GET
    @Path("/{id}")
    @Operation(operationId = "getIdentityProvider", summary = "Get identity provider by ID")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Identity provider details",
            content = @Content(schema = @Schema(implementation = IdentityProviderDto.class))),
        @APIResponse(responseCode = "404", description = "Identity provider not found")
    })
    public Response getIdentityProvider(@PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformIamPermissions.IDP_VIEW);

        return operations.findById(id)
            .map(idp -> Response.ok(toDto(idp)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("NOT_FOUND", "Identity provider not found"))
                .build());
    }

    // ==================== Get by Code ====================

    @GET
    @Path("/by-code/{code}")
    @Operation(operationId = "getIdentityProviderByCode", summary = "Get identity provider by code")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Identity provider details",
            content = @Content(schema = @Schema(implementation = IdentityProviderDto.class))),
        @APIResponse(responseCode = "404", description = "Identity provider not found")
    })
    public Response getIdentityProviderByCode(@PathParam("code") String code) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformIamPermissions.IDP_VIEW);

        return operations.findByCode(code)
            .map(idp -> Response.ok(toDto(idp)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("NOT_FOUND", "Identity provider not found"))
                .build());
    }

    // ==================== Create ====================

    @POST
    @Operation(operationId = "createIdentityProvider", summary = "Create a new identity provider")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Identity provider created",
            content = @Content(schema = @Schema(implementation = IdentityProviderDto.class))),
        @APIResponse(responseCode = "400", description = "Invalid request"),
        @APIResponse(responseCode = "409", description = "Identity provider with this code already exists")
    })
    public Response createIdentityProvider(CreateIdentityProviderRequest request) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformIamPermissions.IDP_CREATE);

        // Parse type
        IdentityProviderType type;
        try {
            type = IdentityProviderType.valueOf(request.type().toUpperCase());
        } catch (IllegalArgumentException | NullPointerException e) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse("INVALID_TYPE", "Type must be INTERNAL or OIDC"))
                .build();
        }

        var context = ExecutionContext.from(tracingContext, principalId);

        var command = new CreateIdentityProviderCommand(
            request.code(),
            request.name(),
            type,
            request.oidcIssuerUrl(),
            request.oidcClientId(),
            request.oidcClientSecretRef(),
            request.oidcMultiTenant() != null ? request.oidcMultiTenant() : false,
            request.oidcIssuerPattern(),
            request.allowedEmailDomains()
        );

        Result<IdentityProviderCreated> result = operations.createIdentityProvider(command, context);

        return switch (result) {
            case Result.Success<IdentityProviderCreated> s -> {
                var idp = operations.findById(s.value().identityProviderId()).orElseThrow();
                yield Response.status(Response.Status.CREATED).entity(toDto(idp)).build();
            }
            case Result.Failure<IdentityProviderCreated> f -> mapErrorToResponse(f.error());
        };
    }

    // ==================== Update ====================

    @PUT
    @Path("/{id}")
    @Operation(operationId = "updateIdentityProvider", summary = "Update an identity provider")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Identity provider updated",
            content = @Content(schema = @Schema(implementation = IdentityProviderDto.class))),
        @APIResponse(responseCode = "404", description = "Identity provider not found")
    })
    public Response updateIdentityProvider(@PathParam("id") String id, UpdateIdentityProviderRequest request) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformIamPermissions.IDP_UPDATE);

        var context = ExecutionContext.from(tracingContext, principalId);

        var command = new UpdateIdentityProviderCommand(
            id,
            request.name(),
            request.oidcIssuerUrl(),
            request.oidcClientId(),
            request.oidcClientSecretRef(),
            request.oidcMultiTenant(),
            request.oidcIssuerPattern(),
            request.allowedEmailDomains()
        );

        Result<IdentityProviderUpdated> result = operations.updateIdentityProvider(command, context);

        return switch (result) {
            case Result.Success<IdentityProviderUpdated> s -> {
                var idp = operations.findById(s.value().identityProviderId()).orElseThrow();
                yield Response.ok(toDto(idp)).build();
            }
            case Result.Failure<IdentityProviderUpdated> f -> mapErrorToResponse(f.error());
        };
    }

    // ==================== Delete ====================

    @DELETE
    @Path("/{id}")
    @Operation(operationId = "deleteIdentityProvider", summary = "Delete an identity provider")
    @APIResponses({
        @APIResponse(responseCode = "204", description = "Identity provider deleted"),
        @APIResponse(responseCode = "400", description = "Cannot delete - has dependent mappings"),
        @APIResponse(responseCode = "404", description = "Identity provider not found")
    })
    public Response deleteIdentityProvider(@PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformIamPermissions.IDP_DELETE);

        var context = ExecutionContext.from(tracingContext, principalId);
        var command = new DeleteIdentityProviderCommand(id);

        Result<IdentityProviderDeleted> result = operations.deleteIdentityProvider(command, context);

        return switch (result) {
            case Result.Success<IdentityProviderDeleted> s -> Response.noContent().build();
            case Result.Failure<IdentityProviderDeleted> f -> mapErrorToResponse(f.error());
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

    private IdentityProviderDto toDto(IdentityProvider idp) {
        return new IdentityProviderDto(
            idp.id,
            idp.code,
            idp.name,
            idp.type.name(),
            idp.oidcIssuerUrl,
            idp.oidcClientId,
            idp.hasClientSecret(),
            idp.oidcMultiTenant,
            idp.oidcIssuerPattern,
            idp.allowedEmailDomains,
            idp.createdAt,
            idp.updatedAt
        );
    }

    // ==================== DTOs ====================

    public record IdentityProviderDto(
        String id,
        String code,
        String name,
        String type,
        String oidcIssuerUrl,
        String oidcClientId,
        boolean hasClientSecret,
        boolean oidcMultiTenant,
        String oidcIssuerPattern,
        List<String> allowedEmailDomains,
        Instant createdAt,
        Instant updatedAt
    ) {}

    public record IdentityProviderListResponse(
        List<IdentityProviderDto> identityProviders,
        int total
    ) {}

    public record CreateIdentityProviderRequest(
        String code,
        String name,
        String type,
        String oidcIssuerUrl,
        String oidcClientId,
        String oidcClientSecretRef,
        Boolean oidcMultiTenant,
        String oidcIssuerPattern,
        List<String> allowedEmailDomains
    ) {}

    public record UpdateIdentityProviderRequest(
        String name,
        String oidcIssuerUrl,
        String oidcClientId,
        String oidcClientSecretRef,
        Boolean oidcMultiTenant,
        String oidcIssuerPattern,
        List<String> allowedEmailDomains
    ) {}

    public record ErrorResponse(String code, String message, Map<String, Object> details) {
        public ErrorResponse(String code, String message) {
            this(code, message, Map.of());
        }
    }
}
