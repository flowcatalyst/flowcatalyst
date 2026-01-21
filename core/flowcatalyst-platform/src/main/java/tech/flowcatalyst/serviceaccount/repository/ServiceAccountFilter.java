package tech.flowcatalyst.serviceaccount.repository;

/**
 * Filter for service account queries.
 */
public record ServiceAccountFilter(
    String clientId,
    Boolean active,
    String applicationId
) {
    public static ServiceAccountFilter all() {
        return new ServiceAccountFilter(null, null, null);
    }

    public static ServiceAccountFilter forClient(String clientId) {
        return new ServiceAccountFilter(clientId, null, null);
    }

    public static ServiceAccountFilter activeOnly() {
        return new ServiceAccountFilter(null, true, null);
    }
}
