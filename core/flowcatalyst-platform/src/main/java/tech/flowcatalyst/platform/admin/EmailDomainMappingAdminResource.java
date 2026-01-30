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
import tech.flowcatalyst.platform.authentication.domain.EmailDomainMapping;
import tech.flowcatalyst.platform.authentication.domain.EmailDomainMappingOperations;
import tech.flowcatalyst.platform.authentication.domain.ScopeType;
import tech.flowcatalyst.platform.authentication.domain.events.EmailDomainMappingCreated;
import tech.flowcatalyst.platform.authentication.domain.events.EmailDomainMappingDeleted;
import tech.flowcatalyst.platform.authentication.domain.events.EmailDomainMappingUpdated;
import tech.flowcatalyst.platform.authentication.domain.operations.createmapping.CreateEmailDomainMappingCommand;
import tech.flowcatalyst.platform.authentication.domain.operations.deletemapping.DeleteEmailDomainMappingCommand;
import tech.flowcatalyst.platform.authentication.domain.operations.updatemapping.UpdateEmailDomainMappingCommand;
import tech.flowcatalyst.platform.authentication.idp.IdentityProviderOperations;
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
 * Admin API for managing Email Domain Mappings.
 *
 * <p>Email Domain Mappings connect email domains to identity providers and define
 * the user scope (ANCHOR, PARTNER, or CLIENT) for users from that domain.
 */
@Path("/api/admin/email-domain-mappings")
@Tag(name = "BFF - Email Domain Mapping Admin", description = "Manage email domain to IDP mappings")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@EmbeddedModeOnly
@jakarta.transaction.Transactional
public class EmailDomainMappingAdminResource {

    @Inject
    EmailDomainMappingOperations operations;

    @Inject
    IdentityProviderOperations idpOperations;

    @Inject
    AuditContext auditContext;

    @Inject
    TracingContext tracingContext;

    @Inject
    AuthorizationService authorizationService;

    // ==================== List ====================

    @GET
    @Operation(operationId = "listEmailDomainMappings", summary = "List all email domain mappings",
        description = "Returns all email domain mappings. Filter by identity provider or scope type.")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of email domain mappings",
            content = @Content(schema = @Schema(implementation = EmailDomainMappingListResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated")
    })
    public Response listEmailDomainMappings(
            @QueryParam("identityProviderId") String identityProviderId,
            @QueryParam("scopeType") String scopeType) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformIamPermissions.DOMAIN_MAPPING_VIEW);

        List<EmailDomainMapping> mappings;

        if (identityProviderId != null && !identityProviderId.isBlank()) {
            mappings = operations.findByIdentityProviderId(identityProviderId);
        } else if (scopeType != null && !scopeType.isBlank()) {
            try {
                var scopeTypeEnum = ScopeType.valueOf(scopeType.toUpperCase());
                mappings = operations.findByScopeType(scopeTypeEnum);
            } catch (IllegalArgumentException e) {
                return Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ErrorResponse("INVALID_SCOPE_TYPE", "Invalid scopeType. Must be ANCHOR, PARTNER, or CLIENT"))
                    .build();
            }
        } else {
            mappings = operations.findAll();
        }

        var dtos = mappings.stream()
            .map(this::toDto)
            .sorted((a, b) -> a.emailDomain().compareTo(b.emailDomain()))
            .toList();

        return Response.ok(new EmailDomainMappingListResponse(dtos, dtos.size())).build();
    }

    // ==================== Get by ID ====================

    @GET
    @Path("/{id}")
    @Operation(operationId = "getEmailDomainMapping", summary = "Get email domain mapping by ID")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Email domain mapping details",
            content = @Content(schema = @Schema(implementation = EmailDomainMappingDto.class))),
        @APIResponse(responseCode = "404", description = "Mapping not found")
    })
    public Response getEmailDomainMapping(@PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformIamPermissions.DOMAIN_MAPPING_VIEW);

        return operations.findById(id)
            .map(mapping -> Response.ok(toDto(mapping)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("NOT_FOUND", "Email domain mapping not found"))
                .build());
    }

    // ==================== Get by Domain ====================

    @GET
    @Path("/by-domain/{domain}")
    @Operation(operationId = "getEmailDomainMappingByDomain", summary = "Get email domain mapping by domain")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Email domain mapping details",
            content = @Content(schema = @Schema(implementation = EmailDomainMappingDto.class))),
        @APIResponse(responseCode = "404", description = "Mapping not found")
    })
    public Response getEmailDomainMappingByDomain(@PathParam("domain") String domain) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformIamPermissions.DOMAIN_MAPPING_VIEW);

        return operations.findByEmailDomain(domain.toLowerCase())
            .map(mapping -> Response.ok(toDto(mapping)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("NOT_FOUND", "Email domain mapping not found"))
                .build());
    }

    // ==================== Create ====================

    @POST
    @Operation(operationId = "createEmailDomainMapping", summary = "Create a new email domain mapping")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Email domain mapping created",
            content = @Content(schema = @Schema(implementation = EmailDomainMappingDto.class))),
        @APIResponse(responseCode = "400", description = "Invalid request"),
        @APIResponse(responseCode = "404", description = "Identity provider or client not found"),
        @APIResponse(responseCode = "409", description = "Email domain mapping already exists")
    })
    public Response createEmailDomainMapping(CreateEmailDomainMappingRequest request) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformIamPermissions.DOMAIN_MAPPING_CREATE);

        // Parse scope type
        ScopeType scopeType;
        try {
            scopeType = ScopeType.valueOf(request.scopeType().toUpperCase());
        } catch (IllegalArgumentException | NullPointerException e) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse("INVALID_SCOPE_TYPE", "scopeType must be ANCHOR, PARTNER, or CLIENT"))
                .build();
        }

        var context = ExecutionContext.from(tracingContext, principalId);

        var command = new CreateEmailDomainMappingCommand(
            request.emailDomain(),
            request.identityProviderId(),
            scopeType,
            request.primaryClientId(),
            request.additionalClientIds(),
            request.grantedClientIds(),
            request.requiredOidcTenantId(),
            request.allowedRoleIds()
        );

        Result<EmailDomainMappingCreated> result = operations.createMapping(command, context);

        return switch (result) {
            case Result.Success<EmailDomainMappingCreated> s -> {
                var mapping = operations.findById(s.value().emailDomainMappingId()).orElseThrow();
                yield Response.status(Response.Status.CREATED).entity(toDto(mapping)).build();
            }
            case Result.Failure<EmailDomainMappingCreated> f -> mapErrorToResponse(f.error());
        };
    }

    // ==================== Update ====================

    @PUT
    @Path("/{id}")
    @Operation(operationId = "updateEmailDomainMapping", summary = "Update an email domain mapping")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Email domain mapping updated",
            content = @Content(schema = @Schema(implementation = EmailDomainMappingDto.class))),
        @APIResponse(responseCode = "400", description = "Invalid request"),
        @APIResponse(responseCode = "404", description = "Mapping, identity provider, or client not found")
    })
    public Response updateEmailDomainMapping(@PathParam("id") String id, UpdateEmailDomainMappingRequest request) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformIamPermissions.DOMAIN_MAPPING_UPDATE);

        // Parse scope type if provided
        ScopeType scopeType = null;
        if (request.scopeType() != null && !request.scopeType().isBlank()) {
            try {
                scopeType = ScopeType.valueOf(request.scopeType().toUpperCase());
            } catch (IllegalArgumentException e) {
                return Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ErrorResponse("INVALID_SCOPE_TYPE", "scopeType must be ANCHOR, PARTNER, or CLIENT"))
                    .build();
            }
        }

        var context = ExecutionContext.from(tracingContext, principalId);

        var command = new UpdateEmailDomainMappingCommand(
            id,
            request.identityProviderId(),
            scopeType,
            request.primaryClientId(),
            request.additionalClientIds(),
            request.grantedClientIds(),
            request.requiredOidcTenantId(),
            request.allowedRoleIds()
        );

        Result<EmailDomainMappingUpdated> result = operations.updateMapping(command, context);

        return switch (result) {
            case Result.Success<EmailDomainMappingUpdated> s -> {
                var mapping = operations.findById(s.value().emailDomainMappingId()).orElseThrow();
                yield Response.ok(toDto(mapping)).build();
            }
            case Result.Failure<EmailDomainMappingUpdated> f -> mapErrorToResponse(f.error());
        };
    }

    // ==================== Delete ====================

    @DELETE
    @Path("/{id}")
    @Operation(operationId = "deleteEmailDomainMapping", summary = "Delete an email domain mapping")
    @APIResponses({
        @APIResponse(responseCode = "204", description = "Email domain mapping deleted"),
        @APIResponse(responseCode = "404", description = "Mapping not found")
    })
    public Response deleteEmailDomainMapping(@PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformIamPermissions.DOMAIN_MAPPING_DELETE);

        var context = ExecutionContext.from(tracingContext, principalId);
        var command = new DeleteEmailDomainMappingCommand(id);

        Result<EmailDomainMappingDeleted> result = operations.deleteMapping(command, context);

        return switch (result) {
            case Result.Success<EmailDomainMappingDeleted> s -> Response.noContent().build();
            case Result.Failure<EmailDomainMappingDeleted> f -> mapErrorToResponse(f.error());
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

    private EmailDomainMappingDto toDto(EmailDomainMapping mapping) {
        // Get identity provider details for display
        var idp = idpOperations.findById(mapping.identityProviderId).orElse(null);
        String idpCode = idp != null ? idp.code : null;
        String idpName = idp != null ? idp.name : null;

        return new EmailDomainMappingDto(
            mapping.id,
            mapping.emailDomain,
            mapping.identityProviderId,
            idpCode,
            idpName,
            mapping.scopeType.name(),
            mapping.primaryClientId,
            mapping.additionalClientIds,
            mapping.grantedClientIds,
            mapping.requiredOidcTenantId,
            mapping.allowedRoleIds,
            mapping.getAllAccessibleClientIds(),
            mapping.createdAt,
            mapping.updatedAt
        );
    }

    // ==================== DTOs ====================

    public record EmailDomainMappingDto(
        String id,
        String emailDomain,
        String identityProviderId,
        String identityProviderCode,
        String identityProviderName,
        String scopeType,
        String primaryClientId,
        List<String> additionalClientIds,
        List<String> grantedClientIds,
        String requiredOidcTenantId,
        List<String> allowedRoleIds,
        List<String> allAccessibleClientIds,
        Instant createdAt,
        Instant updatedAt
    ) {}

    public record EmailDomainMappingListResponse(
        List<EmailDomainMappingDto> mappings,
        int total
    ) {}

    public record CreateEmailDomainMappingRequest(
        String emailDomain,
        String identityProviderId,
        String scopeType,
        String primaryClientId,
        List<String> additionalClientIds,
        List<String> grantedClientIds,
        String requiredOidcTenantId,
        List<String> allowedRoleIds
    ) {}

    public record UpdateEmailDomainMappingRequest(
        String identityProviderId,
        String scopeType,
        String primaryClientId,
        List<String> additionalClientIds,
        List<String> grantedClientIds,
        String requiredOidcTenantId,
        List<String> allowedRoleIds
    ) {}

    public record ErrorResponse(String code, String message, Map<String, Object> details) {
        public ErrorResponse(String code, String message) {
            this(code, message, Map.of());
        }
    }
}
