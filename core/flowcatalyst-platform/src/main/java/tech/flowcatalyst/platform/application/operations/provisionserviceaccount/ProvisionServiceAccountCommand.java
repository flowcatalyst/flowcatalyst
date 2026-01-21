package tech.flowcatalyst.platform.application.operations.provisionserviceaccount;

/**
 * Command to provision a service account for an application.
 *
 * @param applicationId The ID of the application to provision a service account for
 */
public record ProvisionServiceAccountCommand(String applicationId) {}
