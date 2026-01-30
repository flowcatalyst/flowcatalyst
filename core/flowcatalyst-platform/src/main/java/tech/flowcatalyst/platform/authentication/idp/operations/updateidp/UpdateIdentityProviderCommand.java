package tech.flowcatalyst.platform.authentication.idp.operations.updateidp;

import java.util.List;

/**
 * Command to update an Identity Provider.
 *
 * @param identityProviderId  ID of the identity provider to update
 * @param name                New name (optional - null means no change)
 * @param oidcIssuerUrl       New OIDC issuer URL (optional)
 * @param oidcClientId        New OIDC client ID (optional)
 * @param oidcClientSecretRef New reference to OIDC client secret (optional)
 * @param oidcMultiTenant     New multi-tenant setting (optional)
 * @param oidcIssuerPattern   New issuer pattern (optional)
 * @param allowedEmailDomains New allowed email domains (optional)
 */
public record UpdateIdentityProviderCommand(
    String identityProviderId,
    String name,
    String oidcIssuerUrl,
    String oidcClientId,
    String oidcClientSecretRef,
    Boolean oidcMultiTenant,
    String oidcIssuerPattern,
    List<String> allowedEmailDomains
) {}
