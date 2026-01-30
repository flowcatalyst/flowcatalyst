package tech.flowcatalyst.platform.authentication.idp.operations.deleteidp;

/**
 * Command to delete an Identity Provider.
 *
 * @param identityProviderId ID of the identity provider to delete
 */
public record DeleteIdentityProviderCommand(
    String identityProviderId
) {}
