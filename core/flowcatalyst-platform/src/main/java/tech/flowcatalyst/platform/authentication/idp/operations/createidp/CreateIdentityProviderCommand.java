package tech.flowcatalyst.platform.authentication.idp.operations.createidp;

import tech.flowcatalyst.platform.authentication.idp.IdentityProviderType;

import java.util.List;

/**
 * Command to create a new Identity Provider.
 *
 * @param code                Unique code identifier for this IDP
 * @param name                Human-readable name
 * @param type                Type of IDP (INTERNAL or OIDC)
 * @param oidcIssuerUrl       OIDC issuer URL (required for OIDC type)
 * @param oidcClientId        OIDC client ID (required for OIDC type)
 * @param oidcClientSecretRef Reference to the OIDC client secret
 * @param oidcMultiTenant     Whether this is a multi-tenant OIDC configuration
 * @param oidcIssuerPattern   Pattern for validating multi-tenant issuers
 * @param allowedEmailDomains Email domains allowed to use this IDP
 */
public record CreateIdentityProviderCommand(
    String code,
    String name,
    IdentityProviderType type,
    String oidcIssuerUrl,
    String oidcClientId,
    String oidcClientSecretRef,
    boolean oidcMultiTenant,
    String oidcIssuerPattern,
    List<String> allowedEmailDomains
) {}
