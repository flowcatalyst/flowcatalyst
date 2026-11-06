package io.flowcatalyst.ingestor.functions.trace;

import io.flowcatalyst.framework.http.Json;
import io.flowcatalyst.framework.httpclient.HttpCapture;
import io.flowcatalyst.function.TraceOptions;
import io.flowcatalyst.ingestor.functions.context.OutboundClients;
import io.flowcatalyst.ingestor.functions.control.Control;
import io.flowcatalyst.ingestor.runtime.HttpRecords;
import io.flowcatalyst.ingestor.runtime.TraceRedactor;
import java.util.ArrayList;
import java.util.Collection;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.atomic.AtomicInteger;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.node.ArrayNode;
import tools.jackson.databind.node.ObjectNode;
import tools.jackson.databind.node.StringNode;

/// What one invocation writes to the trace: built by [FunctionTraces#begin] only when at least one context is live for
/// the invocation, and bound as a [ScopedValue] ([#CURRENT]) around the worker thread's call into the function, beside
/// [io.flowcatalyst.ingestor.functions.context.InvocationDeadline#CURRENT] — never a mutable field on the shared
/// function context, which concurrent invocations of one loaded version share.
///
/// It holds the invocation's identity, the live contexts (decided once, when it began), the current reference, and the
/// per-invocation caps: 500 `log` records, 500 `trace` records and 100 `outbound` records. The `http` record is written
/// once, when the outcome is known ([#http]).
///
/// Every record's message, data, headers, bodies, query and URL path pass through a [TraceRedactor] built, for that
/// record, from the function's declared secret values and the values concealed for the function
/// ([FunctionTraces#conceal]), so a value concealed by one record is out of the next. Nothing here throws into the
/// invocation: a record that cannot be made is left out.
public final class InvocationTrace {

    private static final Logger LOG = LoggerFactory.getLogger(InvocationTrace.class);

    /// Bound for the duration of one traced invocation's `Function#handle` call; unbound when no context is live.
    public static final ScopedValue<InvocationTrace> CURRENT = ScopedValue.newInstance();

    /// The most `log`, `trace` and `outbound` records one invocation writes.
    public static final int MAX_LOG_RECORDS = 500;
    public static final int MAX_TRACE_RECORDS = 500;
    public static final int MAX_OUTBOUND_RECORDS = 100;

    /// The longest reference, in code points.
    public static final int MAX_REFERENCE = 200;

    /// The `http` record's outcome when the function answered.
    public static final String OUTCOME_OK = Control.TRACE_OUTCOME_OK;

    private final FunctionTraces owner;
    private final String address;
    private final int version;
    private final String invocationId;
    private final String correlationId;
    private final List<String> live;
    private final boolean httpLive;
    private final Collection<String> secrets;
    private volatile String reference = "";

    private final AtomicInteger logs = new AtomicInteger();
    private final AtomicInteger traces = new AtomicInteger();
    private final AtomicInteger outbounds = new AtomicInteger();

    InvocationTrace(FunctionTraces owner, String address, int version, String invocationId, String correlationId,
            List<String> live, Collection<String> secrets) {
        this.owner = owner;
        this.address = address;
        this.version = version;
        this.invocationId = invocationId;
        this.correlationId = correlationId;
        this.live = List.copyOf(live);
        this.httpLive = this.live.contains(FunctionTraces.HTTP);
        this.secrets = List.copyOf(secrets);
    }

    /// [#CURRENT] if bound, else `null`.
    public static @Nullable InvocationTrace current() {
        return CURRENT.isBound() ? CURRENT.get() : null;
    }

    /// Whether `context` is live for this invocation: its window is on and the invocation was sampled in.
    public boolean isLive(String context) {
        return live.contains(context);
    }

    /// Whether the built-in `http` context is live: the call, the function's log lines and its outbound calls are
    /// recorded.
    public boolean httpLive() {
        return httpLive;
    }

    /// The invocation's reference; empty when none.
    public String reference() {
        return reference;
    }

    /// Stamps `ref` (cut to [#MAX_REFERENCE] code points) on every record written from now on. Empty clears it.
    public void setReference(String ref) {
        reference = cutCodePoints(ref == null ? "" : ref, MAX_REFERENCE);
    }

    /// What to bind as [HttpCapture#CURRENT] around the call into the function while `http` is live: turns each
    /// exchange of the function's outbound client into an `outbound` record.
    public HttpCapture.Sink outboundSink() {
        return this::outbound;
    }

    // ── log ──────────────────────────────────────────────────────────────────────────────────────

    /// The record level of a `System.Logger` level (`TRACE`, `DEBUG` and `ALL` are `debug`, `WARNING` is `warn`).
    public static String levelOf(System.Logger.Level level) {
        return switch (level) {
            case TRACE, DEBUG, ALL -> "debug";
            case INFO -> "info";
            case WARNING -> "warn";
            case ERROR, OFF -> "error";
        };
    }

    /// Records one line the function logged, if `http` is live and the invocation's 500 are not used up.
    ///
    /// @param level one of `debug`, `info`, `warn`, `error`
    public void log(String level, @Nullable String message) {
        if (!httpLive || logs.incrementAndGet() > MAX_LOG_RECORDS) {
            return;
        }
        try {
            emit(FunctionTraces.HTTP, Control.TRACE_KIND_LOG, level, message(redactor(), message), "", null);
        } catch (RuntimeException e) {
            LOG.debug("a log line of {} could not be traced", address, e);
        }
    }

    // ── trace ────────────────────────────────────────────────────────────────────────────────────

    /// Records what the function wrote with `ctx.trace`, if `context` is live for this invocation, the invocation's
    /// 500 are not used up and the data fits (over [Control#MAX_TRACE_DATA] bytes the record is not written).
    public void trace(String context, @Nullable String message, @Nullable Map<String, ?> data, @Nullable TraceOptions options) {
        if (!isLive(context) || traces.incrementAndGet() > MAX_TRACE_RECORDS) {
            return;
        }
        try {
            TraceRedactor redactor = redactor();
            JsonNode out = null;
            if (data != null) {
                JsonNode tree = Json.mapper().valueToTree(data);
                out = replacing(tree, redactor);
                int size = Json.mapper().writeValueAsBytes(out).length;
                if (size > Control.MAX_TRACE_DATA) {
                    LOG.debug("a trace record of {} in '{}' has {} bytes of data (over {}); it is not written", address, context, size,
                            Control.MAX_TRACE_DATA);
                    return;
                }
            }
            String level = "info";
            String ref = "";
            if (options != null) {
                String asked = options.level();
                if (asked != null && (asked.equals("debug") || asked.equals("info") || asked.equals("warn") || asked.equals("error"))) {
                    level = asked;
                }
                String own = options.reference();
                if (own != null) {
                    ref = cutCodePoints(redactor.text(own), MAX_REFERENCE);
                }
            }
            emit(context, Control.TRACE_KIND_TRACE, level, message(redactor, message), ref, out);
        } catch (RuntimeException | StackOverflowError e) {
            // Data the function built that cannot be serialized (a cycle, a failing getter) costs the record, not the call.
            LOG.debug("a trace record of {} in '{}' could not be written", address, context, e);
        }
    }

    /// A copy of `node` with the redactor's values replaced in every string, keys included.
    private static JsonNode replacing(JsonNode node, TraceRedactor redactor) {
        if (node instanceof ObjectNode object) {
            ObjectNode out = object.objectNode();
            for (Map.Entry<String, JsonNode> e : object.properties()) {
                out.set(redactor.text(e.getKey()), replacing(e.getValue(), redactor));
            }
            return out;
        }
        if (node instanceof ArrayNode array) {
            ArrayNode out = array.arrayNode();
            for (JsonNode each : array) {
                out.add(replacing(each, redactor));
            }
            return out;
        }
        if (node.isString()) {
            return StringNode.valueOf(redactor.text(node.asString()));
        }
        return node;
    }

    // ── outbound ─────────────────────────────────────────────────────────────────────────────────

    /// One attempt of an outbound call the function made while `http` is live. Calls to the platform (the control
    /// plane client) are not recorded: only the function's own client is.
    private void outbound(HttpCapture.Exchange call) {
        if (!OutboundClients.SERVICE_NAME.equals(call.service())) {
            return;
        }
        try {
            // A credential the answer hands out (a login's session) is kept out of this record and every later one:
            // later calls may carry it where no name says what it is, such as a URL's path.
            for (String credential : TraceRedactor.credentialsIn(call.responseBody(), HttpRecords.contentType(call.responseHeaders()))) {
                owner.conceal(address, credential);
            }
            if (outbounds.incrementAndGet() > MAX_OUTBOUND_RECORDS) {
                return;
            }
            TraceRedactor redactor = redactor();
            int limit = TraceRedactor.BODY_LIMIT;
            HttpRecords.Built built = HttpRecords.build(call, redactor, limit);
            // The platform replaces the data of a record over the limit with a marker, which would lose the whole
            // capture: shrink the bodies until it fits.
            while (Json.mapper().writeValueAsBytes(built.data()).length > Control.MAX_TRACE_HTTP_DATA && limit > 0) {
                limit /= 2;
                built = HttpRecords.build(call, redactor, limit);
            }
            emit(FunctionTraces.HTTP, Control.TRACE_KIND_OUTBOUND, built.level(), message(redactor, built.message()), "", built.data());
        } catch (RuntimeException e) {
            LOG.debug("an outbound call of {} could not be traced", address, e);
        }
    }

    // ── http ─────────────────────────────────────────────────────────────────────────────────────

    /// The request as the function was given it.
    ///
    /// @param method     the HTTP method
    /// @param path       the decoded path
    /// @param rawQuery   the query as sent, without the `?`; empty when none
    /// @param headers    the headers the function saw
    /// @param body       the body
    /// @param callerKind `webhook`, `anonymous` or `principal`
    /// @param callerId   the principal's id; empty for the others
    public record Inbound(String method, String path, String rawQuery, Map<String, List<String>> headers, byte[] body,
            String callerKind, String callerId) {
    }

    /// The response the host sent.
    ///
    /// @param status  the status
    /// @param headers the headers, `Content-Type` included
    /// @param body    the body
    public record Sent(int status, Map<String, List<String>> headers, byte[] body) {
    }

    /// Writes the `http` record of the call, if `http` is live: the request, the response, how long it took and how it
    /// ended. Called once, on every outcome after the permit was held.
    ///
    /// @param outcome    `ok`, `timeout`, `bad_response` or `error` ([Control]'s `TRACE_OUTCOME_*`)
    /// @param durationMs from the start of the invocation to the answer
    public void http(Inbound in, Sent out, String outcome, long durationMs) {
        if (!httpLive) {
            return;
        }
        try {
            TraceRedactor redactor = redactor();
            int status = out.status();
            String level = status >= 500 || !OUTCOME_OK.equals(outcome) ? "error" : status >= 400 ? "warn" : "info";
            String path = redactor.text(in.path());
            Map<String, List<String>> requestHeaders = lowerCased(redactor.headers(in.headers()));
            Map<String, List<String>> responseHeaders = lowerCased(redactor.headers(out.headers()));
            String requestType = HttpRecords.contentType(in.headers());
            String responseType = HttpRecords.contentType(out.headers());
            int limit = owner.bodyBytes();
            Map<String, Object> data = httpData(in, out, outcome, durationMs, redactor, path, requestHeaders, responseHeaders, requestType, responseType, limit);
            // The platform replaces the data of a record over the limit with a marker, which would lose the whole
            // capture: shrink the bodies until it fits.
            while (Json.mapper().writeValueAsBytes(data).length > Control.MAX_TRACE_HTTP_DATA && limit > 0) {
                limit /= 2;
                data = httpData(in, out, outcome, durationMs, redactor, path, requestHeaders, responseHeaders, requestType, responseType, limit);
            }
            String message = in.method() + " " + path + " -> " + status + " (" + durationMs + " ms)";
            emit(FunctionTraces.HTTP, Control.TRACE_KIND_HTTP, level, message(redactor, message), "", data);
        } catch (RuntimeException e) {
            LOG.debug("the http record of an invocation of {} could not be written", address, e);
        }
    }

    private static Map<String, Object> httpData(Inbound in, Sent out, String outcome, long durationMs, TraceRedactor redactor, String path,
            Map<String, List<String>> requestHeaders, Map<String, List<String>> responseHeaders, @Nullable String requestType,
            @Nullable String responseType, int limit) {
        Map<String, Object> request = new LinkedHashMap<>();
        request.put("method", in.method());
        request.put("path", path);
        if (!in.rawQuery().isEmpty()) {
            request.put("rawQuery", redactor.query(in.rawQuery()));
        }
        request.put("headers", requestHeaders);
        request.putAll(redactor.body(in.body(), requestType, limit));
        Map<String, Object> caller = new LinkedHashMap<>();
        caller.put("kind", in.callerKind());
        caller.put("id", in.callerId());
        request.put("caller", caller);
        Map<String, Object> response = new LinkedHashMap<>();
        response.put("status", out.status());
        response.put("headers", responseHeaders);
        response.putAll(redactor.body(out.body(), responseType, limit));
        Map<String, Object> data = new LinkedHashMap<>();
        data.put("request", request);
        data.put("response", response);
        data.put("durationMs", durationMs);
        data.put("outcome", outcome);
        return data;
    }

    // ── shared ───────────────────────────────────────────────────────────────────────────────────

    private void emit(String context, String kind, String level, String message, String ref, @Nullable Object data) {
        String stamped = ref.isEmpty() ? reference : ref;
        owner.sink().offer(new Control.TraceRecord(UUID.randomUUID().toString(), owner.clock().millis(), address, version,
                invocationId, correlationId, stamped, context, kind, level, message, data));
    }

    /// The redactor for the next record: the function's declared secret values and the values concealed for it now.
    private TraceRedactor redactor() {
        List<String> concealed = owner.concealed(address);
        if (concealed.isEmpty()) {
            return new TraceRedactor(secrets);
        }
        List<String> all = new ArrayList<>(secrets);
        all.addAll(concealed);
        return new TraceRedactor(all);
    }

    /// A message with the values replaced, no NUL (the platform cannot store one), and at most
    /// [Control#MAX_TRACE_MESSAGE] characters.
    private static String message(TraceRedactor redactor, @Nullable String message) {
        if (message == null) {
            return "";
        }
        String text = redactor.text(message);
        if (text.indexOf('\0') >= 0) {
            text = text.replace("\0", "");
        }
        if (text.length() <= Control.MAX_TRACE_MESSAGE) {
            return text;
        }
        int end = Control.MAX_TRACE_MESSAGE;
        if (Character.isHighSurrogate(text.charAt(end - 1))) {
            end--;
        }
        return text.substring(0, end);
    }

    private static String cutCodePoints(String text, int max) {
        if (text.codePointCount(0, text.length()) <= max) {
            return text;
        }
        return text.substring(0, text.offsetByCodePoints(0, max));
    }

    /// The headers with lower-case names, as the Go runner records them.
    private static Map<String, List<String>> lowerCased(Map<String, List<String>> headers) {
        Map<String, List<String>> out = new LinkedHashMap<>();
        for (Map.Entry<String, List<String>> e : headers.entrySet()) {
            out.merge(e.getKey().toLowerCase(Locale.ROOT), e.getValue(), (a, b) -> {
                List<String> both = new ArrayList<>(a);
                both.addAll(b);
                return both;
            });
        }
        return out;
    }
}
