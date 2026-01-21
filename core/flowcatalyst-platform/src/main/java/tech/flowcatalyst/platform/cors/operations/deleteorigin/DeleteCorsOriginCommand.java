package tech.flowcatalyst.platform.cors.operations.deleteorigin;

/**
 * Command to delete a CORS allowed origin.
 *
 * @param originId The ID of the CORS entry to delete
 */
public record DeleteCorsOriginCommand(
    String originId
) {}
