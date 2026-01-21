package tech.flowcatalyst.messagerouter.mediator;

import org.junit.jupiter.api.Test;

import java.time.Duration;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Unit tests for MediationError sealed interface and its implementations.
 */
class MediationErrorTest {

    @Test
    void timeout_isRetryable() {
        var error = new MediationError.Timeout(Duration.ofSeconds(30));
        assertTrue(error.isRetryable(), "Timeout errors should be retryable");
    }

    @Test
    void timeout_hasCorrectMessage() {
        var error = new MediationError.Timeout(Duration.ofSeconds(30));
        assertTrue(error.message().contains("30000"), "Message should contain duration in ms");
        assertTrue(error.message().contains("timed out"), "Message should mention timeout");
    }

    @Test
    void timeout_preservesDuration() {
        var duration = Duration.ofMinutes(5);
        var error = new MediationError.Timeout(duration);
        assertEquals(duration, error.duration());
    }

    @Test
    void circuitOpen_isNotRetryable() {
        var error = new MediationError.CircuitOpen("http-mediator");
        assertFalse(error.isRetryable(), "Circuit open should not be retryable");
    }

    @Test
    void circuitOpen_hasCorrectMessage() {
        var error = new MediationError.CircuitOpen("http-mediator");
        assertTrue(error.message().contains("http-mediator"), "Message should contain circuit name");
        assertTrue(error.message().toLowerCase().contains("circuit"), "Message should mention circuit");
    }

    @Test
    void circuitOpen_preservesCircuitName() {
        var error = new MediationError.CircuitOpen("test-circuit");
        assertEquals("test-circuit", error.circuitName());
    }

    @Test
    void httpError_5xx_isRetryable() {
        var error = new MediationError.HttpError(503, "Service Unavailable");
        assertTrue(error.isRetryable(), "5xx errors should be retryable");
    }

    @Test
    void httpError_502_isRetryable() {
        var error = new MediationError.HttpError(502, "Bad Gateway");
        assertTrue(error.isRetryable(), "502 should be retryable");
    }

    @Test
    void httpError_500_isRetryable() {
        var error = new MediationError.HttpError(500, "Internal Server Error");
        assertTrue(error.isRetryable(), "500 should be retryable");
    }

    @Test
    void httpError_4xx_isNotRetryable() {
        var error = new MediationError.HttpError(400, "Bad Request");
        assertFalse(error.isRetryable(), "4xx errors should not be retryable");
    }

    @Test
    void httpError_401_isNotRetryable() {
        var error = new MediationError.HttpError(401, "Unauthorized");
        assertFalse(error.isRetryable(), "401 should not be retryable");
    }

    @Test
    void httpError_403_isNotRetryable() {
        var error = new MediationError.HttpError(403, "Forbidden");
        assertFalse(error.isRetryable(), "403 should not be retryable");
    }

    @Test
    void httpError_404_isNotRetryable() {
        var error = new MediationError.HttpError(404, "Not Found");
        assertFalse(error.isRetryable(), "404 should not be retryable");
    }

    @Test
    void httpError_hasCorrectMessage() {
        var error = new MediationError.HttpError(503, "Service Unavailable");
        assertTrue(error.message().contains("503"), "Message should contain status code");
    }

    @Test
    void httpError_preservesStatusCode() {
        var error = new MediationError.HttpError(503, "Service Unavailable");
        assertEquals(503, error.statusCode());
    }

    @Test
    void httpError_preservesBody() {
        var error = new MediationError.HttpError(500, "Internal Server Error");
        assertEquals("Internal Server Error", error.body());
    }

    @Test
    void httpError_handlesNullBody() {
        var error = new MediationError.HttpError(500, null);
        assertEquals(500, error.statusCode());
        assertNull(error.body());
        assertNotNull(error.message()); // Should not throw
    }

    @Test
    void httpError_truncatesLongBody() {
        var longBody = "x".repeat(200);
        var error = new MediationError.HttpError(500, longBody);
        // Message should truncate long bodies
        assertTrue(error.message().length() < longBody.length() + 20,
            "Message should truncate long bodies");
    }

    @Test
    void networkError_isRetryable() {
        var error = new MediationError.NetworkError(new RuntimeException("Connection refused"));
        assertTrue(error.isRetryable(), "Network errors should be retryable");
    }

    @Test
    void networkError_hasCorrectMessage() {
        var error = new MediationError.NetworkError(new RuntimeException("Connection refused"));
        assertTrue(error.message().contains("Connection refused"), "Message should contain cause message");
        assertTrue(error.message().toLowerCase().contains("network"), "Message should mention network");
    }

    @Test
    void networkError_preservesCause() {
        var cause = new RuntimeException("Connection refused");
        var error = new MediationError.NetworkError(cause);
        assertEquals(cause, error.cause());
    }

    @Test
    void networkError_handlesNullCause() {
        var error = new MediationError.NetworkError(null);
        assertTrue(error.isRetryable());
        assertNotNull(error.message()); // Should not throw
    }

    @Test
    void rateLimited_isRetryable() {
        var error = new MediationError.RateLimited(Duration.ofSeconds(60));
        assertTrue(error.isRetryable(), "Rate limited should be retryable");
    }

    @Test
    void rateLimited_hasCorrectMessage() {
        var error = new MediationError.RateLimited(Duration.ofSeconds(60));
        assertTrue(error.message().contains("60"), "Message should contain retry delay");
        assertTrue(error.message().toLowerCase().contains("rate"), "Message should mention rate");
    }

    @Test
    void rateLimited_preservesRetryAfter() {
        var retryAfter = Duration.ofMinutes(2);
        var error = new MediationError.RateLimited(retryAfter);
        assertEquals(retryAfter, error.retryAfter());
    }

    @Test
    void sealedInterface_allowsPatternMatching() {
        MediationError error = new MediationError.Timeout(Duration.ofSeconds(30));

        // This should compile and work with exhaustive pattern matching
        String result = switch (error) {
            case MediationError.Timeout t -> "timeout: " + t.duration();
            case MediationError.CircuitOpen c -> "circuit: " + c.circuitName();
            case MediationError.HttpError h -> "http: " + h.statusCode();
            case MediationError.NetworkError n -> "network: " + n.cause();
            case MediationError.RateLimited r -> "rate: " + r.retryAfter();
        };

        assertEquals("timeout: PT30S", result);
    }

    @Test
    void allErrorTypes_haveNonNullMessage() {
        MediationError[] errors = {
            new MediationError.Timeout(Duration.ofSeconds(30)),
            new MediationError.CircuitOpen("test"),
            new MediationError.HttpError(500, "error"),
            new MediationError.NetworkError(new RuntimeException("test")),
            new MediationError.RateLimited(Duration.ofSeconds(30))
        };

        for (MediationError error : errors) {
            assertNotNull(error.message(), "All error types should have non-null message");
            assertFalse(error.message().isEmpty(), "All error types should have non-empty message");
        }
    }
}
