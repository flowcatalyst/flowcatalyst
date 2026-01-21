package tech.flowcatalyst.platform.cors.operations.addorigin;

/**
 * Command to add a new CORS allowed origin.
 *
 * @param origin      The origin URL (e.g., "https://app.example.com")
 * @param description Optional description of what this origin is for
 */
public record AddCorsOriginCommand(
    String origin,
    String description
) {}
