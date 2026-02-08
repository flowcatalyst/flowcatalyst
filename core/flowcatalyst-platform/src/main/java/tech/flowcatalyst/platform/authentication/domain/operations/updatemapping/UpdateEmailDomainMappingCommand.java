package tech.flowcatalyst.platform.authentication.domain.operations.updatemapping;

import tech.flowcatalyst.platform.authentication.domain.ScopeType;

import java.util.List;

/**
 * Command to update an Email Domain Mapping.
 *
 * @param emailDomainMappingId  ID of the mapping to update
 * @param identityProviderId    New identity provider ID (optional)
 * @param scopeType             New scope type (optional)
 * @param primaryClientId       New primary client ID (optional)
 * @param additionalClientIds   New additional client IDs (optional)
 * @param grantedClientIds      New granted client IDs (optional)
 * @param requiredOidcTenantId  New required OIDC tenant ID (optional, use empty string to clear)
 * @param allowedRoleIds        New allowed role IDs (optional)
 * @param syncRolesFromIdp      Whether to sync roles from the external IDP during OIDC login (optional, null = no change)
 */
public record UpdateEmailDomainMappingCommand(
    String emailDomainMappingId,
    String identityProviderId,
    ScopeType scopeType,
    String primaryClientId,
    List<String> additionalClientIds,
    List<String> grantedClientIds,
    String requiredOidcTenantId,
    List<String> allowedRoleIds,
    Boolean syncRolesFromIdp
) {}
