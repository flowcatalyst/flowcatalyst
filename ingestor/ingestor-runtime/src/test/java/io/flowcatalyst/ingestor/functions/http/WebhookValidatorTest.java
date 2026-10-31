package io.flowcatalyst.ingestor.functions.http;

import static io.flowcatalyst.ingestor.functions.http.WebhookValidator.Failure.INVALID_SIGNATURE;
import static io.flowcatalyst.ingestor.functions.http.WebhookValidator.Failure.INVALID_TIMESTAMP;
import static io.flowcatalyst.ingestor.functions.http.WebhookValidator.Failure.MISSING_SECRET;
import static io.flowcatalyst.ingestor.functions.http.WebhookValidator.Failure.MISSING_SIGNATURE;
import static io.flowcatalyst.ingestor.functions.http.WebhookValidator.Failure.MISSING_TIMESTAMP;
import static io.flowcatalyst.ingestor.functions.http.WebhookValidator.Failure.TIMESTAMP_EXPIRED;
import static io.flowcatalyst.ingestor.functions.http.WebhookValidator.Failure.TIMESTAMP_IN_FUTURE;
import static org.assertj.core.api.Assertions.assertThat;

import java.nio.charset.StandardCharsets;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.ZoneOffset;
import java.time.format.DateTimeFormatter;
import org.junit.jupiter.api.Test;

/// The test vectors of `pkg/fcsdk/webhook/validator_test.go`, against the Java port, plus known-answer signatures
/// computed independently (`openssl dgst -sha256 -hmac`) so the two implementations are tied to the same bytes.
class WebhookValidatorTest {

    private static final DateTimeFormatter ISO_MILLIS =
            DateTimeFormatter.ofPattern("yyyy-MM-dd'T'HH:mm:ss.SSS'Z'").withZone(ZoneOffset.UTC);

    private static byte[] bytes(String s) {
        return s.getBytes(StandardCharsets.UTF_8);
    }

    private final WebhookValidator v = new WebhookValidator("test-secret");

    @Test
    void acceptsAFreshSignedRequest() {
        byte[] body = bytes("{\"type\":\"order.created\"}");
        String ts = Long.toString(Instant.now().getEpochSecond());
        assertThat(v.validate(v.computeSignature(ts, body), ts, body)).isEmpty();
    }

    /// The router signs with an ISO-8601 millisecond timestamp, not Unix seconds.
    @Test
    void acceptsAnIso8601MillisecondTimestamp() {
        byte[] body = bytes("{\"type\":\"order.created\"}");
        String ts = ISO_MILLIS.format(Instant.now());
        assertThat(v.validate(v.computeSignature(ts, body), ts, body)).isEmpty();
    }

    @Test
    void rejectsAnExpiredIso8601Timestamp() {
        byte[] body = bytes("{\"x\":1}");
        String ts = ISO_MILLIS.format(Instant.now().minus(Duration.ofHours(1)));
        assertThat(v.validate(v.computeSignature(ts, body), ts, body)).contains(TIMESTAMP_EXPIRED);
    }

    @Test
    void rejectsABadSignature() {
        byte[] body = bytes("{\"type\":\"order.created\"}");
        String ts = Long.toString(Instant.now().getEpochSecond());
        assertThat(v.validate("deadbeef", ts, body)).contains(INVALID_SIGNATURE);
    }

    @Test
    void rejectsASignatureOfAnotherBody() {
        String ts = Long.toString(Instant.now().getEpochSecond());
        String signed = v.computeSignature(ts, bytes("{\"a\":1}"));
        assertThat(v.validate(signed, ts, bytes("{\"a\":2}"))).contains(INVALID_SIGNATURE);
    }

    @Test
    void rejectsASignatureMadeWithAnotherSecret() {
        byte[] body = bytes("x");
        String ts = Long.toString(Instant.now().getEpochSecond());
        String other = new WebhookValidator("another-secret").computeSignature(ts, body);
        assertThat(v.validate(other, ts, body)).contains(INVALID_SIGNATURE);
    }

    @Test
    void rejectsAnExpiredUnixTimestamp() {
        byte[] body = bytes("{\"x\":1}");
        String old = Long.toString(Instant.now().minus(Duration.ofHours(1)).getEpochSecond());
        assertThat(v.validate(v.computeSignature(old, body), old, body)).contains(TIMESTAMP_EXPIRED);
    }

    @Test
    void rejectsAFutureTimestamp() {
        byte[] body = bytes("{\"x\":1}");
        String future = Long.toString(Instant.now().plus(Duration.ofHours(2)).getEpochSecond());
        assertThat(v.validate(v.computeSignature(future, body), future, body)).contains(TIMESTAMP_IN_FUTURE);
    }

    @Test
    void rejectsMissingHeaders() {
        assertThat(v.validate("", "1234", new byte[0])).contains(MISSING_SIGNATURE);
        assertThat(v.validate(null, "1234", new byte[0])).contains(MISSING_SIGNATURE);
        assertThat(v.validate("sig", "", new byte[0])).contains(MISSING_TIMESTAMP);
        assertThat(v.validate("sig", null, new byte[0])).contains(MISSING_TIMESTAMP);
        assertThat(v.validate("sig", "notanumber", new byte[0])).contains(INVALID_TIMESTAMP);
    }

    @Test
    void anEmptySecretVerifiesNothing() {
        WebhookValidator none = new WebhookValidator("");
        String ts = Long.toString(Instant.now().getEpochSecond());
        assertThat(none.validate(none.computeSignature(ts, bytes("x")), ts, bytes("x"))).contains(MISSING_SECRET);
    }

    @Test
    void theSignatureIsStable() {
        String a = v.computeSignature("100", bytes("hello"));
        assertThat(a).isEqualTo(v.computeSignature("100", bytes("hello")));
        assertThat(a).isNotEqualTo(v.computeSignature("101", bytes("hello")));
    }

    /// Known answers, computed with `openssl dgst -sha256 -hmac test-secret` over `timestamp || payload`.
    @Test
    void knownAnswers() {
        assertThat(v.computeSignature("100", bytes("hello")))
                .isEqualTo("f549f044e881e44ba426e68fb3cbc3da3a97231d55795e3920db50e75bebf7f8");
        assertThat(v.computeSignature("2026-05-24T08:30:00.123Z", bytes("{\"type\":\"order.created\"}")))
                .isEqualTo("e48113ba238e237172ab4e125e515d6cbc3201d877f5c74c821b2b97694077bf");
    }

    @Test
    void theClockAndToleranceAreTheValidators() {
        Clock fixed = Clock.fixed(Instant.parse("2026-05-24T08:35:00Z"), ZoneOffset.UTC);
        WebhookValidator at = new WebhookValidator("test-secret", Duration.ofSeconds(300), fixed);
        byte[] body = bytes("{\"type\":\"order.created\"}");
        String ts = "2026-05-24T08:30:00.123Z"; // 4m59.877s before: inside the window (whole seconds: 300 s)
        assertThat(at.validate(at.computeSignature(ts, body), ts, body)).isEmpty();
        String older = "2026-05-24T08:29:59.000Z"; // 301 s before
        assertThat(at.validate(at.computeSignature(older, body), older, body)).contains(TIMESTAMP_EXPIRED);
        String ahead = "2026-05-24T08:36:00.000Z"; // exactly the 60 s grace
        assertThat(at.validate(at.computeSignature(ahead, body), ahead, body)).isEmpty();
        String tooFar = "2026-05-24T08:36:01.000Z";
        assertThat(at.validate(at.computeSignature(tooFar, body), tooFar, body)).contains(TIMESTAMP_IN_FUTURE);
    }

    @Test
    void anyRfc3339FractionAndOffsetIsAccepted() {
        Clock fixed = Clock.fixed(Instant.parse("2026-05-24T08:30:30Z"), ZoneOffset.UTC);
        WebhookValidator at = new WebhookValidator("test-secret", Duration.ofSeconds(300), fixed);
        byte[] body = bytes("b");
        for (String ts : new String[] {"2026-05-24T08:30:00Z", "2026-05-24T08:30:00.123456789Z", "2026-05-24T10:30:00+02:00"}) {
            assertThat(at.validate(at.computeSignature(ts, body), ts, body)).as(ts).isEmpty();
        }
    }
}
