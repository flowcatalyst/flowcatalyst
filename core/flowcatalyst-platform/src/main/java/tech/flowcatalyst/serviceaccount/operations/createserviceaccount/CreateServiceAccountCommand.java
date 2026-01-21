package tech.flowcatalyst.serviceaccount.operations.createserviceaccount;

import java.util.List;

/**
 * Command to create a new service account.
 *
 * @param code        Unique identifier code (e.g., "tms-service")
 * @param name        Human-readable display name
 * @param description Optional description of the service account's purpose
 * @param clientIds   List of client IDs this service account can access (empty = no restrictions)
 * @param applicationId Optional application ID if created for an application
 */
public record CreateServiceAccountCommand(
    String code,
    String name,
    String description,
    List<String> clientIds,
    String applicationId
) {}
