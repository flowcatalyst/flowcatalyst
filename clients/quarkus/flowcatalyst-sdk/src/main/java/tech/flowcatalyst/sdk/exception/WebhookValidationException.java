package tech.flowcatalyst.sdk.exception;

/**
 * Exception thrown when webhook signature validation fails.
 */
public class WebhookValidationException extends FlowCatalystException {

    public WebhookValidationException(String message) {
        super(message, 401);
    }

    public static WebhookValidationException invalidSignature() {
        return new WebhookValidationException("Invalid webhook signature");
    }

    public static WebhookValidationException missingSignature() {
        return new WebhookValidationException("Missing X-FlowCatalyst-Signature header");
    }

    public static WebhookValidationException missingTimestamp() {
        return new WebhookValidationException("Missing X-FlowCatalyst-Timestamp header");
    }

    public static WebhookValidationException timestampExpired(int tolerance) {
        return new WebhookValidationException(
            "Webhook timestamp expired (older than " + tolerance + " seconds)"
        );
    }

    public static WebhookValidationException timestampInFuture() {
        return new WebhookValidationException("Webhook timestamp is in the future");
    }

    public static WebhookValidationException missingSigningSecret() {
        return new WebhookValidationException(
            "Signing secret not configured. Set flowcatalyst.signing-secret"
        );
    }
}
