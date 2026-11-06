package io.flowcatalyst.ingestor.functions.control;

import io.flowcatalyst.framework.http.Json;
import io.flowcatalyst.framework.httpclient.ClientResponse;
import io.flowcatalyst.framework.httpclient.HttpClientException;
import io.flowcatalyst.framework.httpclient.RequestBuilder;
import io.flowcatalyst.framework.httpclient.ServiceClient;
import io.flowcatalyst.function.DispatchException;
import io.flowcatalyst.function.EmitResult;
import io.flowcatalyst.function.PlatformTokenException;
import java.io.IOException;
import java.io.InputStream;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.time.Duration;
import java.util.HexFormat;
import java.util.regex.Pattern;
import org.jspecify.annotations.Nullable;
import tools.jackson.core.JacksonException;
import tools.jackson.databind.JsonNode;

/// The real [ControlPlane]: the Go runner's `control.Client` (`internal/functions/control/client.go`) over the
/// framework's [ServiceClient], authenticated as the ingestor's service account like every other
/// call it makes to the platform (that account also holds `platform:function-runner`).
///
/// The artifact route answers either the bytes or a 302 to a presigned storage URL. The framework's
/// client never follows a redirect, so it is followed here: a redirect to another origin is fetched
/// with a client of its own and no `Authorization` header (storage never sees the token); one to the
/// platform's own origin is fetched again through the authenticated client. The digest is verified
/// either way.
public final class HttpControlPlane implements ControlPlane {

    /// An artifact is at most 64 MiB (`maxArtifactBytes`).
    public static final long MAX_ARTIFACT_BYTES = 64L << 20;

    private static final Pattern DIGEST = Pattern.compile("[0-9a-f]{64}");
    private static final int MAX_REDIRECTS = 3;
    private static final int MESSAGE = 300;

    private final ServiceClient client;
    private final HttpClient storage;
    private final Duration artifactTimeout;

    /// @param client the platform with the service account's token; its `maxResponseBytes` must allow
    ///                an artifact ([#MAX_ARTIFACT_BYTES] plus one) and its request timeout the long-poll
    public HttpControlPlane(ServiceClient client) {
        this(client, HttpClient.newBuilder().followRedirects(HttpClient.Redirect.NEVER).connectTimeout(Duration.ofSeconds(10)).build(),
                Duration.ofMinutes(2));
    }

    HttpControlPlane(ServiceClient client, HttpClient storage, Duration artifactTimeout) {
        this.client = client;
        this.storage = storage;
        this.artifactTimeout = artifactTimeout;
    }

    @Override
    public Fetched desired(String pool, @Nullable String etag, Duration wait) throws ControlPlaneException {
        long seconds = Math.min(Math.max(0, wait.toSeconds()), Control.MAX_WAIT_SECONDS);
        RequestBuilder request = client.get(Control.PATH_DESIRED)
                .query("pool", pool)
                .query("runtimes", Control.RUNTIME_JVM)
                .retryable(false)
                .timeout(Duration.ofSeconds(seconds).plusSeconds(30));
        if (seconds > 0) {
            request.query("wait", Long.toString(seconds));
        }
        if (etag != null && !etag.isEmpty()) {
            request.header("If-None-Match", etag);
        }
        ClientResponse response = send(request, "desired state");
        if (response.status() == 304) {
            return new Fetched.NotModified();
        }
        if (response.status() != 200) {
            throw statusError(response);
        }
        try {
            return new Fetched.Changed(response.header("ETag"), response.json(Control.Desired.class));
        } catch (HttpClientException e) {
            throw new ControlPlaneException(0, "", "the desired document is unreadable", e);
        }
    }

    @Override
    public void heartbeat(Control.Heartbeat heartbeat) throws ControlPlaneException {
        ClientResponse response = send(client.post(Control.PATH_HEARTBEAT).json(heartbeat).timeout(Duration.ofSeconds(10)), "heartbeat");
        if (!response.isSuccess()) {
            throw statusError(response);
        }
    }

    @Override
    public Control.TraceAck traceLog(Control.TraceBatch batch) throws ControlPlaneException {
        ClientResponse response = send(
                client.post(Control.PATH_TRACE_LOG).json(batch).retryable(false).timeout(Duration.ofSeconds(15)), "trace log");
        if (!response.isSuccess()) {
            throw statusError(response);
        }
        try {
            return response.json(Control.TraceAck.class);
        } catch (HttpClientException e) {
            // The batch was taken; only its counts are unreadable.
            return new Control.TraceAck(batch.records().size(), 0);
        }
    }

    @Override
    public byte[] artifact(String digest) throws ControlPlaneException {
        if (!DIGEST.matcher(digest).matches()) {
            throw new ControlPlaneException(0, "", "digest is not a sha256 hex digest", null, true);
        }
        ClientResponse response = send(
                client.get(Control.PATH_ARTIFACTS + "/{digest}").pathParam("digest", digest).retryable(false).timeout(artifactTimeout), "artifact");
        for (int hops = 0; response.status() >= 300 && response.status() < 400; hops++) {
            String location = response.header("Location");
            if (location == null || hops >= MAX_REDIRECTS) {
                throw new ControlPlaneException(response.status(), "", "the artifact redirect cannot be followed", null);
            }
            URI target = client.options().baseUrl().resolve(location);
            if (sameOrigin(target, client.options().baseUrl())) {
                response = send(sameOriginRequest(target), "artifact");
                continue;
            }
            return verified(fromStorage(target), digest);
        }
        if (response.status() != 200) {
            throw statusError(response);
        }
        return verified(response.bodyBytes(), digest);
    }

    @Override
    public EmitResult emit(Control.EmitRequest request) {
        ClientResponse response;
        try {
            response = client.post(Control.PATH_EVENTS).json(request).timeout(Duration.ofSeconds(30)).send();
        } catch (HttpClientException e) {
            interruptedFlag(e);
            return new EmitResult.Refused(io.flowcatalyst.function.DispatchException.UNAVAILABLE, 503, "the platform could not be reached");
        }
        if (response.isSuccess()) {
            try {
                String id = response.json(Control.EmitResponse.class).eventId();
                if (id != null && !id.isBlank()) {
                    return new EmitResult.Emitted(id);
                }
            } catch (HttpClientException e) {
                // fall through
            }
            return new EmitResult.Refused(DispatchException.UNAVAILABLE, 503, "the platform's answer was unreadable");
        }
        Said said = said(response);
        // The platform's own code when it gave one (EVENT_NOT_DECLARED, EVENT_TYPE_NOT_OWNED, ...); else the
        // Go runner's classification of the status.
        String code = response.status() >= 500 ? DispatchException.UNAVAILABLE : said.code().isEmpty() ? classify(response.status()) : said.code();
        return new EmitResult.Refused(code, response.status(), said.message());
    }

    @Override
    public String dispatch(Control.DispatchRequest request) throws DispatchException {
        ClientResponse response;
        try {
            response = client.post(Control.PATH_DISPATCH_JOBS).json(request).timeout(Duration.ofSeconds(30)).send();
        } catch (HttpClientException e) {
            interruptedFlag(e);
            throw new DispatchException(DispatchException.UNAVAILABLE, "the platform could not be reached");
        }
        if (response.isSuccess()) {
            try {
                String id = response.json(Control.DispatchResponse.class).jobId();
                if (id != null && !id.isBlank()) {
                    return id;
                }
            } catch (HttpClientException e) {
                // fall through
            }
            throw new DispatchException(DispatchException.UNAVAILABLE, "the platform's answer was unreadable");
        }
        Said said = said(response);
        String code = response.status() >= 500
                ? DispatchException.UNAVAILABLE
                : "DISPATCH_NOT_DECLARED".equals(said.code()) ? "NOT_DECLARED" : classify(response.status());
        throw new DispatchException(code, said.message());
    }

    /// As the Go client does: 5xx, 429, 408 and 401 (the runner's own credential, refreshed once by the client
    /// already, is not accepted: an operational fault, not the function's) and an unreachable platform are
    /// `UNAVAILABLE`; any other refusal (403, 409, 400, 404 ...) is `NOT_ALLOWED` and its message names the
    /// platform's code. The token never reaches a log or a message.
    @Override
    public Control.TokenResponse platformToken(Control.TokenRequest request) throws PlatformTokenException {
        ClientResponse response;
        try {
            response = client.post(Control.PATH_TOKEN).json(request).timeout(Duration.ofSeconds(30)).send();
        } catch (HttpClientException e) {
            interruptedFlag(e);
            throw new PlatformTokenException(PlatformTokenException.UNAVAILABLE, "the platform could not be reached");
        }
        if (response.isSuccess()) {
            try {
                Control.TokenResponse token = response.json(Control.TokenResponse.class);
                String value = token.accessToken();
                if (value != null && !value.isBlank()) {
                    return token;
                }
            } catch (HttpClientException e) {
                // fall through
            }
            throw new PlatformTokenException(PlatformTokenException.UNAVAILABLE, "the platform's answer was unreadable");
        }
        Said said = said(response);
        int status = response.status();
        boolean transientStatus = status >= 500 || status == 429 || status == 408 || status == 401;
        if (transientStatus) {
            throw new PlatformTokenException(PlatformTokenException.UNAVAILABLE, said.message());
        }
        String detail = said.code().isEmpty() ? said.message() : said.code() + ": " + said.message();
        throw new PlatformTokenException("NOT_ALLOWED", detail);
    }

    /// The Go runner's classification of a non-2xx answer to a write made on a function's behalf.
    private static String classify(int status) {
        if (status >= 500) {
            return DispatchException.UNAVAILABLE;
        }
        return status == 403 ? "NOT_ALLOWED" : "BAD_REQUEST";
    }

    private ClientResponse send(RequestBuilder request, String what) throws ControlPlaneException {
        try {
            return request.send();
        } catch (HttpClientException e) {
            interruptedFlag(e);
            if (e.kind() == HttpClientException.Kind.INTERRUPTED) {
                Thread.interrupted();
                throw new ControlPlaneException(0, "INTERRUPTED", what + ": interrupted", e);
            }
            throw new ControlPlaneException(0, e.kind().name(), what + " could not be fetched: " + e.getMessage(), e);
        }
    }

    private static void interruptedFlag(HttpClientException e) {
        if (e.kind() == HttpClientException.Kind.INTERRUPTED) {
            Thread.currentThread().interrupt();
        }
    }

    private byte[] fromStorage(URI target) throws ControlPlaneException {
        HttpRequest request = HttpRequest.newBuilder(target).timeout(artifactTimeout).GET().build();
        try {
            HttpResponse<InputStream> response = storage.send(request, HttpResponse.BodyHandlers.ofInputStream());
            try (InputStream in = response.body()) {
                if (response.statusCode() != 200) {
                    throw new ControlPlaneException(response.statusCode(), "", "the artifact store answered " + response.statusCode(), null);
                }
                byte[] body = in.readNBytes((int) MAX_ARTIFACT_BYTES + 1);
                return body;
            }
        } catch (IOException e) {
            throw new ControlPlaneException(0, "IO", "the artifact could not be downloaded: " + e.getClass().getSimpleName(), e);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            throw new ControlPlaneException(0, "INTERRUPTED", "artifact download interrupted", e);
        }
    }

    /// Size and digest, both permanent failures: the bytes arrived, they are just not the artifact named.
    private static byte[] verified(byte[] bytes, String digest) throws ControlPlaneException {
        if (bytes.length > MAX_ARTIFACT_BYTES) {
            throw new ControlPlaneException(0, "", "artifact is over 64 MiB", null, true);
        }
        if (!sha256Hex(bytes).equals(digest)) {
            throw new ControlPlaneException(0, "", "artifact does not match its digest", null, true);
        }
        return bytes;
    }

    /// The lowercase hex sha256 of `bytes`.
    public static String sha256Hex(byte[] bytes) {
        try {
            return HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256").digest(bytes));
        } catch (NoSuchAlgorithmException e) {
            throw new IllegalStateException("SHA-256 unavailable", e);
        }
    }

    private static boolean sameOrigin(URI a, URI b) {
        return a.getScheme() != null
                && a.getScheme().equalsIgnoreCase(b.getScheme())
                && a.getHost() != null
                && a.getHost().equalsIgnoreCase(b.getHost())
                && port(a) == port(b);
    }

    private static int port(URI u) {
        if (u.getPort() >= 0) {
            return u.getPort();
        }
        return "https".equalsIgnoreCase(u.getScheme()) ? 443 : 80;
    }

    private RequestBuilder sameOriginRequest(URI target) throws ControlPlaneException {
        String base = client.options().baseUrl().getRawPath();
        String path = target.getRawPath() == null ? "/" : target.getRawPath();
        if (base != null && !base.isEmpty() && !base.equals("/") && path.startsWith(base)) {
            path = path.substring(base.length());
        }
        try {
            RequestBuilder request = client.get(path).retryable(false).timeout(artifactTimeout);
            String query = target.getQuery();
            if (query != null && !query.isEmpty()) {
                for (String pair : query.split("&")) {
                    int eq = pair.indexOf('=');
                    request.query(eq < 0 ? pair : pair.substring(0, eq), eq < 0 ? "" : pair.substring(eq + 1));
                }
            }
            return request;
        } catch (IllegalArgumentException e) {
            throw new ControlPlaneException(0, "", "the artifact redirect cannot be followed", e);
        }
    }

    private ControlPlaneException statusError(ClientResponse response) {
        Said said = said(response);
        return new ControlPlaneException(response.status(), said.code(), said.message(), null);
    }

    private record Said(String code, String message) {
    }

    /// What the platform said, from its error envelope `{"error": CODE, "message": ...}` or, failing that, its text.
    private static Said said(ClientResponse response) {
        String text = response.bodyText().strip();
        String code = "";
        String message = text;
        try {
            JsonNode tree = Json.mapper().readTree(response.bodyBytes());
            if (tree != null && tree.isObject()) {
                code = tree.path("error").asString("");
                String m = tree.path("message").asString("");
                message = m.isEmpty() ? text : m;
            }
        } catch (JacksonException e) {
            // plain text
        }
        if (message.length() > MESSAGE) {
            message = message.substring(0, MESSAGE) + "...";
        }
        if (!code.matches("[A-Z][A-Z0-9_]{0,99}")) {
            code = "";
        }
        return new Said(code, message.isEmpty() ? "no detail" : new String(message.getBytes(StandardCharsets.UTF_8), StandardCharsets.UTF_8));
    }
}
