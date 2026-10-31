package io.flowcatalyst.ingestor.functions.http;

import java.nio.charset.StandardCharsets;
import java.security.InvalidKeyException;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.time.Clock;
import java.time.Duration;
import java.time.OffsetDateTime;
import java.time.format.DateTimeParseException;
import java.util.HexFormat;
import java.util.Objects;
import java.util.Optional;
import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;
import org.jspecify.annotations.Nullable;

/// Validates the platform's webhook delivery signature: a port of `pkg/fcsdk/webhook/validator.go`
/// (`webhook.Validator`), same headers, same freshness window, same HMAC.
///
/// The signed message is `timestamp || payload` (the timestamp string exactly as sent in the header, then the raw
/// body), HMAC-SHA256 with the function's `webhookSecret`, lower-case hex in `X-FlowCatalyst-Signature`. The
/// timestamp (`X-FlowCatalyst-Timestamp`) is the ISO-8601 millisecond value the router emits (e.g.
/// `2026-05-24T08:30:00.123Z`); any RFC 3339 fractional precision and a bare Unix-seconds integer are also
/// accepted. A timestamp older than the tolerance (300 s) or more than 60 s in the future is refused. The
/// signature comparison is constant-time. Thread-safe and stateless.
public final class WebhookValidator {

    /// The signature header.
    public static final String SIGNATURE_HEADER = "X-FlowCatalyst-Signature";
    /// The timestamp header.
    public static final String TIMESTAMP_HEADER = "X-FlowCatalyst-Timestamp";

    /// `DefaultToleranceSecs`.
    public static final Duration DEFAULT_TOLERANCE = Duration.ofSeconds(300);
    /// `FutureGraceSecs`: how far into the future a timestamp may be (clock skew).
    public static final Duration FUTURE_GRACE = Duration.ofSeconds(60);

    /// Why a delivery was refused (the Go sentinel errors).
    public enum Failure {
        MISSING_SIGNATURE,
        MISSING_TIMESTAMP,
        INVALID_TIMESTAMP,
        INVALID_SIGNATURE,
        TIMESTAMP_EXPIRED,
        TIMESTAMP_IN_FUTURE,
        MISSING_SECRET
    }

    private final byte[] secret;
    private final Duration tolerance;
    private final Clock clock;

    public WebhookValidator(String secret) {
        this(secret, DEFAULT_TOLERANCE, Clock.systemUTC());
    }

    public WebhookValidator(String secret, Duration tolerance, Clock clock) {
        this.secret = Objects.requireNonNull(secret, "secret").getBytes(StandardCharsets.UTF_8);
        this.tolerance = Objects.requireNonNull(tolerance, "tolerance");
        this.clock = Objects.requireNonNull(clock, "clock");
    }

    /// Checks the signature against the body and timestamp. `null` is a header that was not sent.
    ///
    /// @return empty on success, else the reason; never throws
    public Optional<Failure> validate(@Nullable String signature, @Nullable String timestamp, byte[] payload) {
        if (secret.length == 0) {
            return Optional.of(Failure.MISSING_SECRET);
        }
        if (signature == null || signature.isEmpty()) {
            return Optional.of(Failure.MISSING_SIGNATURE);
        }
        if (timestamp == null || timestamp.isEmpty()) {
            return Optional.of(Failure.MISSING_TIMESTAMP);
        }
        long seconds;
        try {
            seconds = parseTimestamp(timestamp);
        } catch (IllegalArgumentException e) {
            return Optional.of(Failure.INVALID_TIMESTAMP);
        }
        long now = clock.instant().getEpochSecond();
        if (seconds < now - tolerance.toSeconds()) {
            return Optional.of(Failure.TIMESTAMP_EXPIRED);
        }
        if (seconds > now + FUTURE_GRACE.toSeconds()) {
            return Optional.of(Failure.TIMESTAMP_IN_FUTURE);
        }
        String expected = computeSignature(timestamp, payload);
        // hmac.Equal on the hex strings: a constant-time comparison of the bytes.
        if (!MessageDigest.isEqual(signature.getBytes(StandardCharsets.UTF_8), expected.getBytes(StandardCharsets.UTF_8))) {
            return Optional.of(Failure.INVALID_SIGNATURE);
        }
        return Optional.empty();
    }

    /// The hex-encoded HMAC-SHA256 of `(timestamp, payload)` (`ComputeSignature`), so a consumer can sign
    /// outbound callbacks, and a test can sign a delivery, with the same algorithm.
    public String computeSignature(String timestamp, byte[] payload) {
        try {
            Mac mac = Mac.getInstance("HmacSHA256");
            mac.init(new SecretKeySpec(secret.length == 0 ? new byte[1] : secret, "HmacSHA256"));
            mac.update(timestamp.getBytes(StandardCharsets.UTF_8));
            mac.update(payload);
            return HexFormat.of().formatHex(mac.doFinal());
        } catch (NoSuchAlgorithmException | InvalidKeyException e) {
            throw new IllegalStateException("HmacSHA256 unavailable", e);
        }
    }

    /// `parseTimestamp`: the router's ISO-8601 millisecond value, any RFC 3339 fractional precision, or a bare
    /// Unix-seconds integer. Returns Unix seconds.
    static long parseTimestamp(String s) {
        try {
            return OffsetDateTime.parse(s).toEpochSecond();
        } catch (DateTimeParseException e) {
            // not RFC 3339
        }
        try {
            return Long.parseLong(s);
        } catch (NumberFormatException e) {
            throw new IllegalArgumentException("unparseable timestamp");
        }
    }
}
