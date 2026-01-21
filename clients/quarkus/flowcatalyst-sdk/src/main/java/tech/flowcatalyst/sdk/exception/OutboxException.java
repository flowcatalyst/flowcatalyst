package tech.flowcatalyst.sdk.exception;

/**
 * Exception thrown when outbox operations fail.
 */
public class OutboxException extends FlowCatalystException {

    public OutboxException(String message) {
        super(message);
    }

    public OutboxException(String message, Throwable cause) {
        super(message, cause);
    }

    public static OutboxException missingTenantId() {
        return new OutboxException(
            "Tenant ID not configured. Set flowcatalyst.outbox.tenant-id"
        );
    }

    public static OutboxException disabled() {
        return new OutboxException(
            "Outbox is disabled. Set flowcatalyst.outbox.enabled=true"
        );
    }

    public static OutboxException insertFailed(Throwable cause) {
        return new OutboxException("Failed to insert message into outbox", cause);
    }
}
