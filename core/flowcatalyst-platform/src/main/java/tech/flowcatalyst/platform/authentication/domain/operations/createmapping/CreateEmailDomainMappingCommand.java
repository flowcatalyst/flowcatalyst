package tech.flowcatalyst.platform.authentication.domain.operations.createmapping;

import tech.flowcatalyst.platform.authentication.domain.ScopeType;

import java.util.List;

/**
 * Command to create a new Email Domain Mapping.
 *
 * @param emailDomain           The email domain to map (e.g., "acmecorp.com")
 * @param identityProviderId    ID of the identity provider to use
 * @param scopeType             Scope type (ANCHOR, PARTNER, or CLIENT)
 * @param primaryClientId       Primary client ID (required for CLIENT scope)
 * @param additionalClientIds   Additional client IDs (for CLIENT scope)
 * @param grantedClientIds      Granted client IDs (for PARTNER scope)
 * @param requiredOidcTenantId  Required OIDC tenant ID for multi-tenant IDPs (optional)
 * @param allowedRoleIds        Allowed role IDs for users from this domain (optional)
 * @param syncRolesFromIdp      Whether to sync roles from the external IDP during OIDC login
 */
public record CreateEmailDomainMappingCommand(
    String emailDomain,
    String identityProviderId,
    ScopeType scopeType,
    String primaryClientId,
    List<String> additionalClientIds,
    List<String> grantedClientIds,
    String requiredOidcTenantId,
    List<String> allowedRoleIds,
    boolean syncRolesFromIdp
) {}
