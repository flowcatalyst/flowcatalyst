package tech.flowcatalyst.serviceaccount.operations.deleteserviceaccount;

/**
 * Command to delete a service account.
 *
 * @param serviceAccountId The service account ID to delete
 */
public record DeleteServiceAccountCommand(
    String serviceAccountId
) {}
