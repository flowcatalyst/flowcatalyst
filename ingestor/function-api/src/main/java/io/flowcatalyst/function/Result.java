package io.flowcatalyst.function;

import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.Arrays;
import java.util.List;
import java.util.Map;
import java.util.Objects;

/// The outcome of one [Function#handle]: the HTTP response the host sends
/// back to whoever called it. Every invocation is an HTTP request, so every
/// outcome is an HTTP response; the factories below spell out the delivery
/// conventions so a function author never memorises a status code or a
/// header name. Statuses and bodies are exactly those of the platform SDKs
/// (`clients/fn-ts` `respond.ts`).
///
/// | Helper | Response | What the platform's delivery paths (subscription / dispatch job / scheduled job) do with it |
/// |---|---|---|
/// | [#ack()] | `200`, empty body | any 2xx is a delivery success |
/// | [#retry(Duration)] | `429` + `Retry-After: <ceil-seconds>` | a deferral: rescheduled after `Retry-After`, **spending no retry budget** |
/// | [#reject(String)] | `422` + `FlowCatalyst-Outcome: reject`, plain-text reason as the body (none when empty) | the terminal "do not retry" answer: the job is not retried |
/// | [#fail(String)] | `500` + `{"error": "<reason>"}` | an ordinary failure: retried on the platform's schedule until its attempts run out |
///
/// For a call that is not a delivery (a browser, an API client) the same
/// responses are simply what the caller receives; use [#json], [#text],
/// [#status] and [#http] for those.
///
/// Built only through the static factories below (or the canonical
/// constructor), which enforce the invariants each response depends on.
/// `headers` is deep-copied and unmodifiable; `body` is cloned in the
/// constructor and again by [#body()].
///
/// @param status  the HTTP status code; 100-599
/// @param headers response headers, in insertion order
/// @param body    the response body
public record Result(int status, Map<String, List<String>> headers, byte[] body) {

    /// The header a terminal answer carries (`FlowCatalyst-Outcome`).
    public static final String OUTCOME_HEADER = "FlowCatalyst-Outcome";
    /// The [#OUTCOME_HEADER] value meaning "do not retry".
    public static final String OUTCOME_REJECT = "reject";

    private static final int DEFAULT_FAIL_STATUS = 500;

    public Result {
        if (status < 100 || status > 599) {
            throw new IllegalArgumentException("status must be 100-599, was " + status);
        }
        headers = Copies.multiMap(headers);
        body = Copies.bytes(body);
    }

    /// The invocation succeeded; nothing further to say: `200`, empty body.
    public static Result ack() {
        return new Result(200, Map.of(), new byte[0]);
    }

    /// The invocation should be redelivered after `after`: `429` with
    /// `Retry-After`, rounded **up** to a whole second (the header is integer
    /// seconds; rounding down would ask for less delay than requested) and
    /// never below `1`, since the platform reads `Retry-After: 0` as "absent"
    /// and applies its 30 s default (fn-ts has the same floor of 1).
    ///
    /// @throws IllegalArgumentException if `after` is `null` or negative
    public static Result retry(Duration after) {
        if (after == null || after.isNegative()) {
            throw new IllegalArgumentException("after must be a non-null, non-negative Duration");
        }
        long seconds = ceilSeconds(after);
        return new Result(429, Map.of("Retry-After", List.of(Long.toString(seconds))), new byte[0]);
    }

    /// The invocation failed and may be tried again. `reason` becomes the
    /// audit/metrics detail, carried in a small hand-escaped
    /// `{"error": "<reason>"}` body. This is an ordinary failure: use
    /// [#reject(String)] for one that must not be retried.
    ///
    /// @throws IllegalArgumentException if `reason` is blank or `null`
    public static Result fail(String reason) {
        if (reason == null || reason.isBlank()) {
            throw new IllegalArgumentException("reason must not be blank");
        }
        String json = "{\"error\":\"" + JsonEscape.escape(reason) + "\"}";
        return new Result(DEFAULT_FAIL_STATUS, Map.of("Content-Type", List.of("application/json")),
                json.getBytes(StandardCharsets.UTF_8));
    }

    /// The terminal "do not retry" answer: `422` with `FlowCatalyst-Outcome:
    /// reject`. `reason` is written as a plain-text body for diagnostics (no
    /// JSON envelope); `null` or empty writes no body.
    public static Result reject(String reason) {
        Map<String, List<String>> headers = Map.of(OUTCOME_HEADER, List.of(OUTCOME_REJECT));
        byte[] body = reason == null || reason.isEmpty() ? new byte[0] : reason.getBytes(StandardCharsets.UTF_8);
        return new Result(422, headers, body);
    }

    /// [#reject(String)] without a reason.
    public static Result reject() {
        return reject(null);
    }

    /// A plain-text answer: `Content-Type: text/plain; charset=utf-8`.
    ///
    /// @throws IllegalArgumentException if `status` is outside 100-599
    public static Result text(int status, String text) {
        Objects.requireNonNull(text, "text");
        return new Result(status, Map.of("Content-Type", List.of("text/plain; charset=utf-8")),
                text.getBytes(StandardCharsets.UTF_8));
    }

    /// A bare status: no headers, no body.
    ///
    /// @throws IllegalArgumentException if `status` is outside 100-599
    public static Result status(int status) {
        return new Result(status, Map.of(), new byte[0]);
    }

    /// A direct HTTP answer, verbatim.
    ///
    /// @throws IllegalArgumentException if `status` is outside 100-599
    public static Result http(int status, Map<String, List<String>> headers, byte[] body) {
        return new Result(status, headers, body);
    }

    /// A direct HTTP answer whose body is `json` verbatim, with
    /// `Content-Type: application/json`. `json` is not validated or
    /// parsed — the caller built it, or already has it as text.
    ///
    /// @throws IllegalArgumentException if `status` is outside 100-599
    public static Result json(int status, String json) {
        Objects.requireNonNull(json, "json");
        return new Result(status, Map.of("Content-Type", List.of("application/json")),
                json.getBytes(StandardCharsets.UTF_8));
    }

    @Override
    public byte[] body() {
        return Copies.bytes(body);
    }

    /// Ceiling division to whole seconds, at least `1`, and clamped to
    /// `Integer.MAX_VALUE` so an enormous duration cannot overflow the
    /// `Retry-After` header into a negative number.
    private static long ceilSeconds(Duration after) {
        long seconds = after.getSeconds();
        if (after.getNano() > 0) {
            seconds++;
        }
        return Math.max(1, Math.min(seconds, Integer.MAX_VALUE));
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (!(o instanceof Result other)) return false;
        return status == other.status && headers.equals(other.headers) && Arrays.equals(body, other.body);
    }

    @Override
    public int hashCode() {
        int result = Objects.hash(status, headers);
        return 31 * result + Arrays.hashCode(body);
    }

    @Override
    public String toString() {
        return "Result[status=" + status + ", headers=" + headers
                + ", body.length=" + (body == null ? 0 : body.length) + "]";
    }
}
