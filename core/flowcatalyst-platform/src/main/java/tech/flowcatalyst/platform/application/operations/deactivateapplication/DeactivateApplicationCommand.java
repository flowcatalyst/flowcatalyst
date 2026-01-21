package tech.flowcatalyst.platform.application.operations.deactivateapplication;

/**
 * Command to deactivate an Application.
 *
 * @param applicationId The ID of the application to deactivate
 */
public record DeactivateApplicationCommand(String applicationId) {}
