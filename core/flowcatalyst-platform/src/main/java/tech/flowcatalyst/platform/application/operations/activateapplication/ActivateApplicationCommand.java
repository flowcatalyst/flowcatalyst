package tech.flowcatalyst.platform.application.operations.activateapplication;

/**
 * Command to activate an Application.
 *
 * @param applicationId The ID of the application to activate
 */
public record ActivateApplicationCommand(String applicationId) {}
