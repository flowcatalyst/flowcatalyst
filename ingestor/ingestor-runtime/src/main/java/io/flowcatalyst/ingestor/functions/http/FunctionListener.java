package io.flowcatalyst.ingestor.functions.http;

import io.flowcatalyst.framework.auth.PlatformTokens;
import io.flowcatalyst.framework.auth.TokenResult;
import io.flowcatalyst.framework.http.HttpException;
import io.flowcatalyst.framework.http.Json;
import io.flowcatalyst.framework.http.Request;
import io.flowcatalyst.framework.http.Response;
import io.flowcatalyst.framework.http.Routes;
import io.flowcatalyst.framework.pipeline.Next;
import io.flowcatalyst.function.Caller;
import io.flowcatalyst.function.FunctionContext;
import io.flowcatalyst.function.Result;
import io.flowcatalyst.ingestor.extension.FailureLog;
import io.flowcatalyst.ingestor.functions.control.Control;
import io.flowcatalyst.ingestor.functions.control.Describe;
import io.flowcatalyst.ingestor.functions.load.LoadedFunction;
import io.flowcatalyst.ingestor.functions.reconcile.Reconciler;
import io.flowcatalyst.ingestor.functions.trace.InvocationTrace;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.CharacterCodingException;
import java.nio.charset.CodingErrorAction;
import java.nio.charset.StandardCharsets;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/// The listener for every function: `/fn/{address}[@{alias}|@v{n}]/{path}` on the framework's HTTP server, with the
/// invocation path of the Go function runner (`internal/functions/runner/invoke.go`, docs/function-runner-plan.md
/// section 6.2). One fixed prefix serves all functions; which functions exist behind it changes as the platform's
/// document does. The framework's router has no catch-all pattern, so the listener is global middleware
/// ([#install]) that answers `/fn/...` itself and passes every other path on.
///
/// The order of the checks, and the answer to each:
///
/// | step | answer |
/// |---|---|
/// | address not in the document | `404 FUNCTION_NOT_FOUND` |
/// | alias / explicit version not found | `404 ALIAS_NOT_FOUND` / `404 VERSION_NOT_FOUND` |
/// | version still loading / failed or unloaded | `503 FUNCTION_PREPARING` (`Retry-After: 2`) / `503 FUNCTION_UNAVAILABLE` (`30`) |
/// | explicit `@v{n}` | a platform token (`401`) holding `platform:function:version:invoke` with reach to the function's client (`403`) |
/// | no endpoint matches the path / the method | `404 ROUTE_NOT_FOUND` / `405 METHOD_NOT_ALLOWED` |
/// | `cors` declared | a preflight is answered here; `Access-Control-*` headers are the host's |
/// | `webhook` endpoint and the method is not `POST` | `405` |
/// | `auth: platform` | a platform access token (`401`; `503` when its keys cannot be read), before the body is read |
/// | body over the limit (the endpoint's, else the function's, at most 32 MiB) | `413 BODY_TOO_LARGE` |
/// | `auth: webhook` | the delivery signature over the raw body (`401 INVALID_SIGNATURE`) |
/// | no permit free (`limits.maxConcurrency`) | `429 FUNCTION_BUSY`, `Retry-After: 1`, at once — never queued |
/// | the deadline (the endpoint's `timeoutMs`, at most `limits.timeoutMs`) passes | `504 FUNCTION_TIMEOUT` |
/// | the handler throws | `500 FUNCTION_FAILED` (the exception only in the log) |
/// | the handler answers a status outside 200 to 599 | `502 FUNCTION_BAD_RESPONSE` |
///
/// **Concurrent (R3).** Everything on this path reads immutable or concurrent state: nothing is locked or queued,
/// and each call runs on a virtual thread of its own ([InvocationRunner]). The permit and the version's in-flight
/// count are released by the handler's thread when it really returns — a call past its deadline answers 504 at
/// once, and its permit comes back when the handler ends.
public final class FunctionListener {

    private static final Logger LOG = LoggerFactory.getLogger(FunctionListener.class);

    /// The URL prefix.
    public static final String PREFIX = "/fn/";

    /// The runner-wide ceiling on a request body (`MaxRequestBodyBytes`): the body is buffered in memory.
    public static final long MAX_REQUEST_BODY_BYTES = 32L << 20;

    private static final String BEARER = "Bearer ";

    private static final List<String> HOP_HEADERS =
            List.of("connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade", "host");

    private final Reconciler reconciler;
    private final PlatformTokens tokens;
    private final Permits permits = new Permits();
    private final Clock clock;

    /// Invocation failures and timeouts (docs/ingestor-failure-pacing.md R3, F6): the first of a streak of one
    /// function version and failure with its stack trace, then a line a minute with a count. No "recovered" call:
    /// the minute limit and the log's stale rule end a streak.
    private final FailureLog failures;

    public FunctionListener(Reconciler reconciler, PlatformTokens tokens, Clock clock) {
        this(reconciler, tokens, clock, new FailureLog((message, cause) -> {
            if (cause == null) {
                LOG.warn(message);
            } else {
                LOG.warn(message, cause);
            }
        }, Duration.ofMinutes(1), clock));
    }

    /// @param failures where invocation failures are logged (tests give one with a clock they move)
    public FunctionListener(Reconciler reconciler, PlatformTokens tokens, Clock clock, FailureLog failures) {
        this.failures = Objects.requireNonNull(failures, "failures");
        this.reconciler = Objects.requireNonNull(reconciler, "reconciler");
        this.tokens = Objects.requireNonNull(tokens, "tokens");
        this.clock = Objects.requireNonNull(clock, "clock");
        reconciler.addPostReconcileListener(() -> permits.retainOnly(reconciler.addresses()));
    }

    public Permits permits() {
        return permits;
    }

    /// Registers the listener on the server's routes.
    public void install(Routes routes) {
        routes.use(this::middleware);
    }

    Response middleware(Request request, Next<Request, Response> next) throws Exception {
        String path = request.path();
        if (path.startsWith(PREFIX) || path.equals("/fn")) {
            return handle(request);
        }
        return next.handle(request);
    }

    // ── the call ──────────────────────────────────────────────────────────────────────────────────

    /// Handles one `/fn/...` request.
    public Response handle(Request request) {
        String raw = request.path();
        String rest = raw.length() > PREFIX.length() ? raw.substring(PREFIX.length()) : "";
        int slash = rest.indexOf('/');
        String rawTarget = slash < 0 ? rest : rest.substring(0, slash);
        String rawPath = slash < 0 ? "/" : rest.substring(slash);
        @Nullable String targetText = decode(rawTarget);
        @Nullable String path = decode(rawPath);
        @Nullable Target target = targetText == null ? null : parseTarget(targetText);
        if (target == null || path == null) {
            return error(404, "FUNCTION_NOT_FOUND", "no such function", 0);
        }

        Reconciler.Resolution resolution = reconciler.resolve(target.address(), target.alias(), target.version());
        return switch (resolution) {
            case Reconciler.Resolution.Unknown u -> error(404, "FUNCTION_NOT_FOUND", "no such function", 0);
            case Reconciler.Resolution.AliasNotFound a -> error(404, "ALIAS_NOT_FOUND", "no such alias", 0);
            case Reconciler.Resolution.VersionNotFound v -> error(404, "VERSION_NOT_FOUND", "no such version on this host", 0);
            case Reconciler.Resolution.Unavailable u -> error(503, u.code(), u.message(), u.retryAfterSeconds());
            case Reconciler.Resolution.Ready ready -> {
                yield invoke(request, target, path, ready);
            }
        };
    }

    /// `ready`'s function is retained; it is released by the invocation's thread once the handler returns, or here on
    /// every path that does not start an invocation.
    private Response invoke(Request request, Target target, String path, Reconciler.Resolution.Ready ready) {
        Control.FunctionDoc doc = ready.doc();
        Reconciler.Served served = ready.served();
        LoadedFunction function = ready.function();
        Handoff handoff = new Handoff();
        try {
            return invokeWith(request, target, path, doc, served, function, handoff);
        } finally {
            if (!handoff.started) {
                function.release();
            }
        }
    }

    /// Set once the invocation's thread owns the release of the function.
    private static final class Handoff {
        boolean started;
    }

    private Response invokeWith(
            Request request, Target target, String path, Control.FunctionDoc doc, Reconciler.Served served,
            LoadedFunction function, Handoff handoff) {
        Control.Limits limits = doc.effectiveLimits();
        String method = request.method().name();

        // Explicit versions are an operator tool: a platform token with version:invoke and reach; the endpoint's
        // own auth is not applied.
        Caller.@Nullable Principal explicit = null;
        if (target.version() > 0) {
            VerifiedToken verified = verify(request);
            if (verified.refusal() != null) {
                return verified.refusal();
            }
            Caller.Principal principal = Objects.requireNonNull(verified.principal());
            if (!principal.hasPermission(CallerMapper.PERM_VERSION_INVOKE) || !CallerMapper.canReach(principal, doc.clientId())) {
                return error(403, "FORBIDDEN", "invoking an explicit version needs " + CallerMapper.PERM_VERSION_INVOKE, 0);
            }
            explicit = principal;
        }

        // A CORS preflight is matched by the method it asks about.
        String matchMethod = method;
        if (method.equals("OPTIONS")) {
            String want = request.header("Access-Control-Request-Method");
            if (want != null && !want.isEmpty()) {
                matchMethod = want;
            }
        }
        EndpointRouter.Matched matched = served.router().match(matchMethod, path);
        if (matched instanceof EndpointRouter.Matched.MethodNotAllowed) {
            return error(405, "METHOD_NOT_ALLOWED", "the endpoint does not accept " + method, 0);
        }
        if (!(matched instanceof EndpointRouter.Matched.Found found)) {
            return error(404, "ROUTE_NOT_FOUND", "no endpoint matches " + path, 0);
        }
        Describe.Endpoint ep = found.endpoint();

        Map<String, String> corsHeaders = Map.of();
        if (ep.cors() != null) {
            CorsPolicy.Outcome cors = CorsPolicy.evaluate(
                    ep.cors(), method, request.header("Origin"), request.header("Access-Control-Request-Method"),
                    request.header("Access-Control-Request-Headers"));
            if (cors.answered()) {
                return withHeaders(cors.status() == 204 ? Response.noContent() : Response.bytes(cors.status(), "text/plain; charset=utf-8", new byte[0]), cors.headers());
            }
            corsHeaders = cors.headers();
        }
        Response response = invokeEndpoint(request, target, path, doc, served, function, handoff, limits, explicit, found, ep, corsHeaders);
        return corsHeaders.isEmpty() ? response : withHeaders(response, corsHeaders);
    }

    private Response invokeEndpoint(
            Request request, Target target, String path, Control.FunctionDoc doc, Reconciler.Served served,
            LoadedFunction function, Handoff handoff, Control.Limits limits, Caller.@Nullable Principal explicit,
            EndpointRouter.Matched.Found found, Describe.Endpoint ep, Map<String, String> corsHeaders) {
        String method = request.method().name();

        // Deliveries are always POST. A webhook endpoint that declares no method matches every method, so refuse the
        // rest here: a captured signed delivery cannot be replayed as a GET/PUT/DELETE.
        if (Describe.AUTH_WEBHOOK.equals(ep.auth()) && !method.equals("POST")) {
            return error(405, "METHOD_NOT_ALLOWED", "the endpoint does not accept " + method, 0);
        }

        // A platform bearer token is checked before the body is touched, so an unauthenticated caller cannot make
        // the host buffer anything. (Webhook auth signs the body, so that path has to read first; it is bounded by
        // the same ceiling.)
        @Nullable Caller caller = explicit;
        if (caller == null && Describe.AUTH_PLATFORM.equals(ep.auth())) {
            VerifiedToken verified = verify(request);
            if (verified.refusal() != null) {
                return verified.refusal();
            }
            caller = verified.principal();
        }

        long maxBody = limits.maxBodyBytes();
        if (ep.maxBodyBytes() != null) {
            maxBody = ep.maxBodyBytes();
        }
        maxBody = Math.min(maxBody, MAX_REQUEST_BODY_BYTES);
        if (request.contentLength() > maxBody) {
            return error(413, "BODY_TOO_LARGE", "the body is over " + maxBody + " bytes", 0);
        }
        byte[] body;
        try (InputStream in = request.body()) {
            body = in.readNBytes((int) maxBody + 1);
        } catch (HttpException e) {
            if (e.status() == 413) {
                return error(413, "BODY_TOO_LARGE", "the body is over the server's limit", 0);
            }
            return error(400, "BODY_UNREADABLE", "the request body could not be read", 0);
        } catch (IOException | RuntimeException e) {
            LOG.debug("request body could not be read: {}", e.getClass().getSimpleName());
            return error(400, "BODY_UNREADABLE", "the request body could not be read", 0);
        }
        if (body.length > maxBody) {
            return error(413, "BODY_TOO_LARGE", "the body is over " + maxBody + " bytes", 0);
        }

        if (caller == null) {
            if (Describe.AUTH_WEBHOOK.equals(ep.auth())) {
                // No secret yet (the signing account came after this host's last desired document) is "not
                // ready", not "bad signature": a 401 is never retried and the delivery would be lost, a 503 is
                // retried once the next document has the secret. As the Go runner.
                String secret = doc.webhookSecret();
                if (secret == null || secret.isEmpty()) {
                    return error(503, "WEBHOOK_SECRET_PENDING", "the function cannot verify deliveries yet", 5);
                }
                if (!verifyWebhook(secret, request, body)) {
                    return error(401, "INVALID_SIGNATURE", "the delivery signature did not verify", 0);
                }
                caller = Caller.Platform.INSTANCE;
            } else {
                caller = Caller.Anonymous.INSTANCE;
            }
        }

        Permits.Grant grant = permits.tryAcquire(doc.address(), limits.maxConcurrency());
        if (grant == null) {
            return error(429, "FUNCTION_BUSY", "the function is at its concurrency limit", 1);
        }

        // The trace gate runs after the permit, so a busy refusal takes no sampling turn; with no window on the function
        // it reads one field.
        long startNanos = System.nanoTime();
        String invocationId = UUID.randomUUID().toString().replace("-", "");
        @Nullable InvocationTrace trace = reconciler.traces().begin(doc, served.number(), invocationId, request.header("X-Correlation-Id"));
        Map<String, List<String>> headers = requestHeaders(request, Describe.AUTH_PLATFORM.equals(ep.auth()) || target.version() > 0);
        Admitted admitted = new Admitted(request, doc, served, function, handoff, grant, ep, limits, caller, method, path, body, headers,
                found.pathParams(), invocationId, trace);
        Outcome outcome = execute(admitted);
        if (trace != null && trace.httpLive()) {
            try {
                trace.http(inbound(request, method, path, caller, headers, body), sent(outcome, corsHeaders), outcome.outcome(),
                        TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - startNanos));
            } catch (RuntimeException e) {
                LOG.debug("the http record of an invocation of {} could not be made", doc.address(), e);
            }
        }
        return outcome.response();
    }

    /// Everything the invocation needs once its permit is held.
    private record Admitted(
            Request request, Control.FunctionDoc doc, Reconciler.Served served, LoadedFunction function, Handoff handoff,
            Permits.Grant grant, Describe.Endpoint ep, Control.Limits limits, Caller caller, String method, String path, byte[] body,
            Map<String, List<String>> headers, Map<String, String> pathParams, String invocationId, @Nullable InvocationTrace trace) {
    }

    /// What the call came to: the response, how the trace names the outcome (`ok`, `timeout`, `bad_response`, `error`) and
    /// the response body (only kept when the call is traced; the framework's [Response] does not give its bytes back).
    private record Outcome(Response response, String outcome, byte[] body) {
    }

    private static final byte[] NO_BODY = new byte[0];

    /// Runs the admitted call: builds the function's request, runs it on its own virtual thread with the deadline, and
    /// maps the way it ended. The permit is released here on every path that does not hand it to the invocation's thread.
    private Outcome execute(Admitted a) {
        Control.FunctionDoc doc = a.doc();
        Reconciler.Served served = a.served();
        Permits.Grant grant = a.grant();
        @Nullable InvocationTrace trace = a.trace();
        boolean traced = trace != null;
        FunctionContext ctx = a.function().context();
        if (ctx == null) {
            grant.release();
            LOG.error("a loaded function has no context: {}", doc.address());
            return failure(500, "FUNCTION_FAILED", "the function failed", 0, Control.TRACE_OUTCOME_ERROR, traced);
        }

        long timeoutMs = a.limits().timeoutMs();
        if (a.ep().timeoutMs() != null) {
            timeoutMs = Math.min(timeoutMs, a.ep().timeoutMs());
        }
        Request request = a.request();
        Instant deadline = clock.instant().plusMillis(timeoutMs);
        io.flowcatalyst.function.Request fnRequest;
        try {
            fnRequest = new io.flowcatalyst.function.Request(
                    a.invocationId(),
                    a.function().address(),
                    served.number(),
                    a.method(),
                    a.path(),
                    request.rawQuery() == null ? "" : request.rawQuery(),
                    a.headers(),
                    a.ep().pattern(),
                    a.pathParams(),
                    a.caller(),
                    deadline,
                    request.header("Host") == null ? "" : request.header("Host"),
                    request.remoteAddress() == null ? "" : request.remoteAddress(),
                    false,
                    a.body());
        } catch (RuntimeException e) {
            grant.release();
            LOG.warn("a request could not be built for {}: {}", doc.address(), e.getMessage());
            return failure(500, "FUNCTION_FAILED", "the request could not be encoded", 0, Control.TRACE_OUTCOME_ERROR, traced);
        }

        InvocationRunner.Invocation invocation;
        try {
            invocation = InvocationRunner.start(a.function(), fnRequest, ctx, deadline, grant::release, trace);
            a.handoff().started = true; // the worker releases the function; the permit comes back with it
        } catch (RuntimeException | Error e) {
            grant.release();
            throw e;
        }

        Result result;
        try {
            result = invocation.future().get(timeoutMs, TimeUnit.MILLISECONDS);
        } catch (TimeoutException e) {
            invocation.worker().interrupt();
            failures.failed(doc.address() + "@" + served.number() + ":timeout",
                    "function " + doc.address() + " v" + served.number() + " did not finish before its deadline (invocation " + a.invocationId() + ")", null);
            return failure(504, "FUNCTION_TIMEOUT", "the function did not finish before its deadline", 0, Control.TRACE_OUTCOME_TIMEOUT, traced);
        } catch (InterruptedException e) {
            // The request thread was interrupted (the server is stopping, or the client went away).
            invocation.worker().interrupt();
            Thread.currentThread().interrupt();
            return failure(503, "FUNCTION_UNAVAILABLE", "the call was interrupted", 1, Control.TRACE_OUTCOME_ERROR, traced);
        } catch (ExecutionException e) {
            Throwable cause = e.getCause() == null ? e : e.getCause();
            // The exception is the function's own: its message goes to the log, never to the caller.
            failures.failed(doc.address() + "@" + served.number() + ":" + cause.getClass().getName(),
                    "function " + doc.address() + " v" + served.number() + " failed with " + cause.getClass().getName()
                            + " (invocation " + a.invocationId() + ")",
                    cause);
            return failure(500, "FUNCTION_FAILED", "the function failed", 0, Control.TRACE_OUTCOME_ERROR, traced);
        }
        return answer(result, a.ep().cors() != null, a.invocationId(), traced);
    }

    /// A refusal or failure as an outcome; its body is built only when `traced`.
    private static Outcome failure(int status, String code, String message, int retryAfterSeconds, String outcome, boolean traced) {
        return new Outcome(error(status, code, message, retryAfterSeconds), outcome, traced ? errorBody(code, message) : NO_BODY);
    }

    /// The request as the trace records it.
    private static InvocationTrace.Inbound inbound(
            Request request, String method, String path, Caller caller, Map<String, List<String>> headers, byte[] body) {
        String kind;
        String id = "";
        switch (caller) {
            case Caller.Platform p -> kind = "webhook";
            case Caller.Principal p -> {
                kind = "principal";
                id = p.id();
            }
            case Caller.Anonymous a -> kind = "anonymous";
        }
        return new InvocationTrace.Inbound(method, path, request.rawQuery() == null ? "" : request.rawQuery(), headers, body, kind, id);
    }

    /// The response as the trace records it: what the host sent, `Access-Control-*` headers included.
    private static InvocationTrace.Sent sent(Outcome outcome, Map<String, String> corsHeaders) {
        Response response = outcome.response();
        Map<String, List<String>> headers = new LinkedHashMap<>();
        for (Response.Header h : response.headers()) {
            headers.computeIfAbsent(h.name(), n -> new ArrayList<>()).add(h.value());
        }
        String contentType = response.contentType();
        if (contentType != null && headers.keySet().stream().noneMatch(n -> n.equalsIgnoreCase("content-type"))) {
            headers.put("Content-Type", List.of(contentType));
        }
        corsHeaders.forEach((name, value) -> headers.put(name, List.of(value)));
        return new InvocationTrace.Sent(response.status(), headers, outcome.body());
    }

    // ── authentication ────────────────────────────────────────────────────────────────────────

    private record VerifiedToken(Caller.@Nullable Principal principal, @Nullable Response refusal) {
    }

    private VerifiedToken verify(Request request) {
        String header = request.header("Authorization");
        if (header == null || !header.regionMatches(true, 0, BEARER, 0, BEARER.length())) {
            return new VerifiedToken(null, error(401, "UNAUTHENTICATED", "a platform bearer token is required", 0));
        }
        return switch (tokens.verify(header.substring(BEARER.length()).strip())) {
            case TokenResult.Valid valid -> new VerifiedToken(CallerMapper.principal(valid.token().principal()), null);
            case TokenResult.Invalid invalid -> {
                LOG.debug("a function call presented a refused token: {}", invalid.reason());
                yield new VerifiedToken(null, error(401, "UNAUTHENTICATED", "a platform bearer token is required", 0));
            }
            case TokenResult.Unavailable unavailable -> {
                LOG.warn("the platform's signing keys could not be read: {}", unavailable.reason());
                yield new VerifiedToken(null, error(503, "UNAVAILABLE", "tokens cannot be checked right now; send it again", 1));
            }
        };
    }

    private static boolean verifyWebhook(@Nullable String secret, Request request, byte[] body) {
        if (secret == null || secret.isEmpty()) {
            return false;
        }
        return new WebhookValidator(secret)
                .validate(request.header(WebhookValidator.SIGNATURE_HEADER), request.header(WebhookValidator.TIMESTAMP_HEADER), body)
                .isEmpty();
    }

    // ── request and response ─────────────────────────────────────────────────────────────────────

    /// The request's headers in `Canonical-Case` (as Go's `http.Header`), without the hop-by-hop headers and, once the
    /// caller is proven by a platform token, without the credential: a function must never be handed one it could
    /// replay against the platform.
    private static Map<String, List<String>> requestHeaders(Request request, boolean dropAuthorization) {
        Map<String, List<String>> out = new LinkedHashMap<>();
        for (Map.Entry<String, List<String>> e : request.headers().entrySet()) {
            String name = e.getKey().toLowerCase(Locale.ROOT);
            if (HOP_HEADERS.contains(name) || (dropAuthorization && name.equals("authorization"))) {
                continue;
            }
            out.put(canonical(name), e.getValue());
        }
        return out;
    }

    /// `textproto.CanonicalMIMEHeaderKey` for a lower-case name.
    static String canonical(String lower) {
        StringBuilder sb = new StringBuilder(lower.length());
        boolean upper = true;
        for (int i = 0; i < lower.length(); i++) {
            char c = lower.charAt(i);
            sb.append(upper ? Character.toUpperCase(c) : c);
            upper = c == '-';
        }
        return sb.toString();
    }

    /// The function's answer as the HTTP response. When the endpoint has a CORS policy the host already set the
    /// `Access-Control-*` headers from it; the function cannot add to or override them.
    private static Outcome answer(Result result, boolean hostCors, String invocationId, boolean traced) {
        int status = result.status();
        if (status < 200 || status > 599) {
            LOG.warn("a function returned an out-of-range status {} (invocation {})", status, invocationId);
            return failure(502, "FUNCTION_BAD_RESPONSE", "the function returned an invalid response", 0, Control.TRACE_OUTCOME_BAD_RESPONSE, traced);
        }
        String contentType = null;
        List<String[]> others = new ArrayList<>();
        for (Map.Entry<String, List<String>> e : result.headers().entrySet()) {
            String name = e.getKey();
            String lower = name.toLowerCase(Locale.ROOT);
            if (HOP_HEADERS.contains(lower) || lower.equals("content-length")) {
                continue;
            }
            if (hostCors && lower.startsWith("access-control-")) {
                continue;
            }
            if (lower.equals("content-type")) {
                if (!e.getValue().isEmpty()) {
                    contentType = e.getValue().get(0);
                }
                continue;
            }
            for (String v : e.getValue()) {
                others.add(new String[] {name, v});
            }
        }
        byte[] body = result.body();
        Response response;
        if (contentType == null && body.length == 0 && status == 204) {
            response = Response.noContent();
        } else {
            response = Response.bytes(status, contentType == null ? "application/octet-stream" : contentType, body);
        }
        try {
            for (String[] h : others) {
                response = response.addHeader(h[0], h[1]);
            }
        } catch (IllegalArgumentException e) {
            LOG.warn("a function returned a header the server refuses (invocation {}): {}", invocationId, e.getMessage());
            return failure(502, "FUNCTION_BAD_RESPONSE", "the function returned an invalid response", 0, Control.TRACE_OUTCOME_BAD_RESPONSE, traced);
        }
        return new Outcome(response.header("X-FlowCatalyst-Invocation", invocationId), Control.TRACE_OUTCOME_OK, traced ? body : NO_BODY);
    }

    private static Map<String, String> errorEnvelope(String code, String message) {
        Map<String, String> body = new LinkedHashMap<>();
        body.put("error", code);
        body.put("message", message);
        return body;
    }

    /// The JSON body [#error] sends.
    static byte[] errorBody(String code, String message) {
        return Json.mapper().writeValueAsBytes(errorEnvelope(code, message));
    }

    static Response error(int status, String code, String message, int retryAfterSeconds) {
        Response response = Response.json(status, errorEnvelope(code, message));
        return retryAfterSeconds > 0 ? response.header("Retry-After", Integer.toString(retryAfterSeconds)) : response;
    }

    private static Response withHeaders(Response response, Map<String, String> headers) {
        Response out = response;
        for (Map.Entry<String, String> h : headers.entrySet()) {
            out = out.header(h.getKey(), h.getValue());
        }
        return out;
    }

    // ── the target ───────────────────────────────────────────────────────────────────────────────

    /// The parsed `{target}` segment.
    record Target(String address, @Nullable String alias, int version) {
    }

    /// `parseTarget`: `address`, `address@alias`, `address@v{n}` (n a positive integer).
    static @Nullable Target parseTarget(String s) {
        int at = s.indexOf('@');
        if (at < 0) {
            return s.isEmpty() ? null : new Target(s, null, 0);
        }
        String address = s.substring(0, at);
        String selector = s.substring(at + 1);
        if (address.isEmpty() || selector.isEmpty()) {
            return null;
        }
        if (selector.startsWith("v")) {
            try {
                int n = Integer.parseInt(selector.substring(1));
                if (n > 0) {
                    return new Target(address, null, n);
                }
            } catch (NumberFormatException e) {
                // an alias that starts with v
            }
        }
        return new Target(address, selector, 0);
    }

    /// Percent-decodes a path text as UTF-8 (`+` stays a plus); `null` on a malformed escape.
    static @Nullable String decode(String s) {
        if (s.indexOf('%') < 0) {
            return s;
        }
        ByteArrayOutputStream out = new ByteArrayOutputStream(s.length());
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            if (c == '%') {
                if (i + 2 >= s.length()) {
                    return null;
                }
                int hi = Character.digit(s.charAt(i + 1), 16);
                int lo = Character.digit(s.charAt(i + 2), 16);
                if (hi < 0 || lo < 0) {
                    return null;
                }
                out.write((hi << 4) | lo);
                i += 2;
            } else if (c < 0x80) {
                out.write(c);
            } else {
                byte[] bytes = String.valueOf(c).getBytes(StandardCharsets.UTF_8);
                out.write(bytes, 0, bytes.length);
            }
        }
        try {
            return StandardCharsets.UTF_8.newDecoder()
                    .onMalformedInput(CodingErrorAction.REPORT)
                    .onUnmappableCharacter(CodingErrorAction.REPORT)
                    .decode(java.nio.ByteBuffer.wrap(out.toByteArray()))
                    .toString();
        } catch (CharacterCodingException e) {
            return null;
        }
    }
}
