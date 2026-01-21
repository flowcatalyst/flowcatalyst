package tech.flowcatalyst.platform.application.operations;

/**
 * Command to disable an application for a client.
 *
 * @param applicationId The application to disable
 * @param clientId The client to disable it for
 */
public record DisableApplicationForClientCommand(
    String applicationId,
    String clientId
) {}
