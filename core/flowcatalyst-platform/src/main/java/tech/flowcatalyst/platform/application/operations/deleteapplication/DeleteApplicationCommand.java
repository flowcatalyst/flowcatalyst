package tech.flowcatalyst.platform.application.operations.deleteapplication;

/**
 * Command to delete an Application.
 *
 * @param applicationId The ID of the application to delete
 */
public record DeleteApplicationCommand(String applicationId) {}
