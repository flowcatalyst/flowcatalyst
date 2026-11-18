package io.flowcatalyst.sdk.http;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import io.flowcatalyst.sdk.FlowCatalystClient;
import io.flowcatalyst.sdk.StubServer;
import io.flowcatalyst.sdk.error.FlowCatalystException;
import io.flowcatalyst.sdk.error.SdkError;
import io.flowcatalyst.sdk.generated.model.CreateApplicationRequest;
import io.flowcatalyst.sdk.generated.model.UpdateApplicationRequest;
import io.flowcatalyst.sdk.outbox.CreateDispatchJobDto;
import io.flowcatalyst.sdk.resources.DispatchJobsResource;
import java.io.BufferedReader;
import java.io.EOFException;
import java.io.IOException;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.net.ConnectException;
import java.net.InetAddress;
import java.net.ServerSocket;
import java.net.Socket;
import java.net.SocketException;
import java.net.UnknownHostException;
import java.net.http.HttpConnectTimeoutException;
import java.net.http.HttpTimeoutException;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.atomic.AtomicInteger;
import javax.net.ssl.SSLHandshakeException;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/**
 * The retry rule (README, "Retries"): what is sent again, by method and by the
 * per-call repeatable mark, after a gateway status or a transport failure.
 */
class RetryRuleTest {

    private static final String SYNC_OK =
            "{\"applicationCode\":\"orders\",\"created\":0,\"updated\":0,\"deleted\":0,\"syncedCodes\":[]}";

    private StubServer server;
    private final List<RawServer> rawServers = new CopyOnWriteArrayList<>();

    @BeforeEach
    void setUp() throws Exception {
        server = new StubServer();
    }

    @AfterEach
    void tearDown() {
        server.close();
        rawServers.forEach(RawServer::close);
    }

    private FlowCatalystClient client(String baseUrl) {
        return FlowCatalystClient.builder()
                .baseUrl(baseUrl)
                .accessToken("tok")
                .retryDelay(Duration.ofMillis(1))
                .build();
    }

    private FlowCatalystClient client() {
        return client(server.baseUrl());
    }

    /** Answers the scripted replies in order, the last one for every later request. */
    private static java.util.function.Function<StubServer.Recorded, StubServer.Reply> script(
            AtomicInteger calls, StubServer.Reply... replies) {
        return r -> replies[Math.min(calls.getAndIncrement(), replies.length - 1)];
    }

    private long requestsTo(String path) {
        return server.requests.stream().filter(r -> r.pathAndQuery().startsWith(path)).count();
    }

    // ── by method ────────────────────────────────────────────────────

    @Test
    void getAnswering502ThenOkIsRetried() {
        AtomicInteger calls = new AtomicInteger();
        server.on("GET", "/api/applications/app_1/service-account", script(calls,
                StubServer.Reply.json(502, "bad gateway"),
                StubServer.Reply.json(200, "{\"id\":\"sa_1\"}")));

        assertNotNull(client().applications().getServiceAccount("app_1"));
        assertEquals(2, calls.get());
    }

    @Test
    void putAnswering503ThenOkIsRetried() {
        AtomicInteger calls = new AtomicInteger();
        server.on("PUT", "/api/applications/app_1", script(calls,
                StubServer.Reply.json(503, "{\"error\":\"UNAVAILABLE\"}"),
                StubServer.Reply.json(204, null)));

        client().applications().update("app_1", new UpdateApplicationRequest());
        assertEquals(2, calls.get());
    }

    @Test
    void deleteAnswering502Then404SucceedsAfterTwoRequests() {
        AtomicInteger calls = new AtomicInteger();
        server.on("DELETE", "/api/applications/app_1", script(calls,
                StubServer.Reply.json(502, "bad gateway"),
                StubServer.Reply.json(404, "{\"error\":\"NOT_FOUND\",\"message\":\"application not found\"}")));

        client().applications().delete("app_1");
        assertEquals(2, calls.get());
    }

    @Test
    void deleteAnswering404FirstIsNotFound() {
        server.on("DELETE", "/api/applications/app_1", 404,
                "{\"error\":\"NOT_FOUND\",\"message\":\"application not found\"}");

        FlowCatalystException e = assertThrows(FlowCatalystException.class,
                () -> client().applications().delete("app_1"));
        assertInstanceOf(SdkError.NotFound.class, e.error());
        assertEquals(1, requestsTo("/api/applications/app_1"));
    }

    @Test
    void postAnswering502IsNotRepeatedAndIsOutcomeUnknown() {
        server.on("POST", "/api/applications", 502, "bad gateway");

        FlowCatalystException e = assertThrows(FlowCatalystException.class,
                () -> client().applications().create(new CreateApplicationRequest()));
        SdkError.OutcomeUnknown unknown = assertInstanceOf(SdkError.OutcomeUnknown.class, e.error());
        assertEquals(502, unknown.status());
        assertEquals("POST", unknown.method());
        assertEquals(server.baseUrl() + "/api/applications", unknown.url());
        assertEquals("bad gateway", unknown.body());
        assertNull(unknown.cause());
        assertTrue(unknown.message().contains("may have been applied and was not repeated"),
                unknown.message());
        assertEquals(1, requestsTo("/api/applications"));
    }

    @Test
    void postAnswering503WithRetriesOffIsStillOutcomeUnknown() {
        server.on("POST", "/api/applications", 503, "{\"error\":\"UNAVAILABLE\"}");
        FlowCatalystClient client = FlowCatalystClient.builder()
                .baseUrl(server.baseUrl()).accessToken("tok").retryAttempts(0).build();

        FlowCatalystException e = assertThrows(FlowCatalystException.class,
                () -> client.applications().create(new CreateApplicationRequest()));
        assertEquals(503, assertInstanceOf(SdkError.OutcomeUnknown.class, e.error()).status());
    }

    @Test
    void postAnswering429ThenOkIsRetried() {
        AtomicInteger calls = new AtomicInteger();
        server.on("POST", "/api/applications", script(calls,
                StubServer.Reply.json(429, "{\"error\":\"RATE_LIMITED\"}"),
                StubServer.Reply.json(201, "{\"id\":\"app_1\"}")));

        assertNotNull(client().applications().create(new CreateApplicationRequest()));
        assertEquals(2, calls.get());
    }

    @Test
    void postAnswering408ThenOkIsRetried() {
        AtomicInteger calls = new AtomicInteger();
        server.on("POST", "/api/applications", script(calls,
                StubServer.Reply.json(408, "{\"error\":\"REQUEST_TIMEOUT\"}"),
                StubServer.Reply.json(201, "{\"id\":\"app_1\"}")));

        assertNotNull(client().applications().create(new CreateApplicationRequest()));
        assertEquals(2, calls.get());
    }

    // ── the reported case ────────────────────────────────────────────

    @Test
    void provisioningAnswering502IsSentOnceAndTheScripted409IsNeverRequested() {
        AtomicInteger calls = new AtomicInteger();
        server.on("POST", "/api/applications/app_1/provision-service-account", script(calls,
                // The platform committed; the gateway lost the answer (and its one-time secrets).
                StubServer.Reply.json(502, "<html>502 Bad Gateway</html>"),
                StubServer.Reply.json(409,
                        "{\"error\":\"ALREADY_PROVISIONED\",\"message\":\"already provisioned\"}")));

        FlowCatalystException e = assertThrows(FlowCatalystException.class,
                () -> client().applications().provisionServiceAccount("app_1"));

        SdkError.OutcomeUnknown unknown = assertInstanceOf(SdkError.OutcomeUnknown.class, e.error());
        assertEquals(502, unknown.status());
        assertEquals(1, calls.get(), "exactly one request; the 409 is never consumed");
        assertEquals(1, requestsTo("/api/applications/app_1/provision-service-account"));
    }

    // ── POSTs marked repeatable ──────────────────────────────────────

    @Test
    void syncEndpointAnswering502ThenOkIsRetried() {
        AtomicInteger calls = new AtomicInteger();
        server.on("POST", "/api/applications/orders/event-types/sync", script(calls,
                StubServer.Reply.json(502, "bad gateway"),
                StubServer.Reply.json(200, SYNC_OK)));

        assertNotNull(client().eventTypes().sync("orders", List.of(), false));
        assertEquals(2, calls.get());
    }

    @Test
    void dispatchBatchIsRetriedOnlyWhenEveryJobCarriesAnIdempotencyKey() {
        AtomicInteger calls = new AtomicInteger();
        server.on("POST", "/api/dispatch-jobs/batch", script(calls,
                StubServer.Reply.json(503, "unavailable"),
                StubServer.Reply.json(201, "{\"results\":[{\"id\":\"j1\",\"status\":\"SUCCESS\"}]}")));
        CreateDispatchJobDto keyed = CreateDispatchJobDto.of("orders:record", "https://t.example.com/r")
                .withIdempotencyKey("order-42");
        CreateDispatchJobDto unkeyed = CreateDispatchJobDto.of("orders:record", "https://t.example.com/r");

        List<DispatchJobsResource.BatchResult> results = client().dispatchJobs().createBatch(List.of(keyed));
        assertEquals(1, results.size());
        assertEquals(2, calls.get());

        calls.set(0);
        FlowCatalystException e = assertThrows(FlowCatalystException.class,
                () -> client().dispatchJobs().createBatch(List.of(keyed, unkeyed)));
        assertInstanceOf(SdkError.OutcomeUnknown.class, e.error());
        assertEquals(1, calls.get());
    }

    @Test
    void routerCheckBatchIsRetried() {
        AtomicInteger calls = new AtomicInteger();
        server.on("POST", "/monitoring/in-flight-messages/check-batch", script(calls,
                StubServer.Reply.json(504, "gateway timeout"),
                StubServer.Reply.json(200, "{\"m1\":true}")));

        assertEquals(Map.of("m1", true), client().router().inPipelineBatch(List.of("m1")));
        assertEquals(2, calls.get());
    }

    // ── transport failures ───────────────────────────────────────────

    @Test
    void postToARefusedConnectionIsATransportErrorNotOutcomeUnknown() throws Exception {
        int port = freePort();
        FlowCatalystClient client = FlowCatalystClient.builder()
                .baseUrl("http://127.0.0.1:" + port).accessToken("tok").retryAttempts(0).build();

        FlowCatalystException e = assertThrows(FlowCatalystException.class,
                () -> client.applications().create(new CreateApplicationRequest()));
        // Nothing was sent, so the error is the plain network failure.
        assertInstanceOf(SdkError.Network.class, e.error());
    }

    @Test
    void postAfterConnectionRefusedThenOkIsRetried() throws Exception {
        int port = freePort();
        Duration delay = Duration.ofMillis(1500);
        FlowCatalystClient client = FlowCatalystClient.builder()
                .baseUrl("http://127.0.0.1:" + port).accessToken("tok")
                .retryAttempts(1).retryDelay(delay).build();
        // Nothing listens on the port for the first attempt; the server is up before the retry.
        Thread starter = Thread.ofVirtual().start(() -> {
            try {
                Thread.sleep(300);
                rawServers.add(new RawServer(port, List.of(Behaviour.respond(201, "{\"id\":\"app_1\"}"))));
            } catch (Exception ex) {
                throw new RuntimeException(ex);
            }
        });

        long started = System.nanoTime();
        assertNotNull(client.applications().create(new CreateApplicationRequest()));
        long elapsedMs = (System.nanoTime() - started) / 1_000_000;
        starter.join();

        assertTrue(elapsedMs >= delay.toMillis(), "the first attempt failed and was retried");
        assertEquals(1, rawServers.getFirst().requests.size());
    }

    @Test
    void postResetAfterSendIsSentOnceAndIsOutcomeUnknown() throws Exception {
        RawServer raw = raw(Behaviour.RESET, Behaviour.respond(201, "{\"id\":\"app_1\"}"));

        FlowCatalystException e = assertThrows(FlowCatalystException.class,
                () -> client(raw.baseUrl()).applications().create(new CreateApplicationRequest()));

        SdkError.OutcomeUnknown unknown = assertInstanceOf(SdkError.OutcomeUnknown.class, e.error());
        assertNull(unknown.status());
        assertInstanceOf(IOException.class, unknown.cause());
        assertEquals(unknown.cause(), e.getCause());
        assertEquals(1, raw.requests.size());
    }

    @Test
    void postClosedAfterSendIsSentOnceAndIsOutcomeUnknown() throws Exception {
        RawServer raw = raw(Behaviour.CLOSE, Behaviour.respond(201, "{\"id\":\"app_1\"}"));

        FlowCatalystException e = assertThrows(FlowCatalystException.class,
                () -> client(raw.baseUrl()).applications().create(new CreateApplicationRequest()));

        assertInstanceOf(SdkError.OutcomeUnknown.class, e.error());
        assertEquals(1, raw.requests.size());
    }

    @Test
    void postReadTimeoutIsSentOnceAndIsOutcomeUnknown() throws Exception {
        RawServer raw = raw(Behaviour.HOLD, Behaviour.respond(201, "{\"id\":\"app_1\"}"));
        FlowCatalystClient client = FlowCatalystClient.builder()
                .baseUrl(raw.baseUrl()).accessToken("tok")
                .timeout(Duration.ofMillis(300)).retryDelay(Duration.ofMillis(1)).build();

        FlowCatalystException e = assertThrows(FlowCatalystException.class,
                () -> client.applications().create(new CreateApplicationRequest()));

        SdkError.OutcomeUnknown unknown = assertInstanceOf(SdkError.OutcomeUnknown.class, e.error());
        assertInstanceOf(HttpTimeoutException.class, unknown.cause());
        assertEquals(1, raw.requests.size());
    }

    @Test
    void syncPostResetAfterSendIsRetried() throws Exception {
        RawServer raw = raw(Behaviour.RESET, Behaviour.respond(200, SYNC_OK));

        assertNotNull(client(raw.baseUrl()).eventTypes().sync("orders", List.of(), false));
        assertEquals(2, raw.requests.size());
    }

    @Test
    void getReadTimeoutThenOkIsRetried() throws Exception {
        RawServer raw = raw(Behaviour.HOLD, Behaviour.respond(200, "{\"id\":\"sa_1\"}"));
        FlowCatalystClient client = FlowCatalystClient.builder()
                .baseUrl(raw.baseUrl()).accessToken("tok")
                .timeout(Duration.ofMillis(300)).retryDelay(Duration.ofMillis(1)).build();

        assertNotNull(client.applications().getServiceAccount("app_1"));
        assertEquals(2, raw.requests.size());
    }

    @Test
    void deleteResetAfterSendThen404Succeeds() throws Exception {
        RawServer raw = raw(Behaviour.RESET,
                Behaviour.respond(404, "{\"error\":\"NOT_FOUND\",\"message\":\"application not found\"}"));

        client(raw.baseUrl()).applications().delete("app_1");
        assertEquals(2, raw.requests.size());
    }

    @Test
    void getTimingOutOnEveryAttemptStillEndsAsTimeout() throws Exception {
        RawServer raw = raw(Behaviour.HOLD);
        FlowCatalystClient client = FlowCatalystClient.builder()
                .baseUrl(raw.baseUrl()).accessToken("tok").retryAttempts(1)
                .timeout(Duration.ofMillis(200)).retryDelay(Duration.ofMillis(1)).build();

        FlowCatalystException e = assertThrows(FlowCatalystException.class,
                () -> client.applications().getServiceAccount("app_1"));
        assertInstanceOf(SdkError.Timeout.class, e.error());
        assertEquals(2, raw.requests.size());
    }

    @Test
    void nothingSentRecognisesConnectPhaseFailuresAnywhereInTheCauseChain() {
        assertTrue(Transport.nothingSent(new ConnectException()));
        assertTrue(Transport.nothingSent(new HttpConnectTimeoutException("HTTP connect timed out")));
        assertTrue(Transport.nothingSent(new SSLHandshakeException("handshake")));
        assertTrue(Transport.nothingSent(new IOException("wrapped", new UnknownHostException("nohost"))));
        assertTrue(Transport.nothingSent(
                new IOException("wrapped", new java.nio.channels.UnresolvedAddressException())));

        assertFalse(Transport.nothingSent(new HttpTimeoutException("request timed out")));
        assertFalse(Transport.nothingSent(new SocketException("Connection reset")));
        assertFalse(Transport.nothingSent(
                new IOException("HTTP/1.1 header parser received no bytes", new EOFException("EOF"))));
    }

    // ── helpers ──────────────────────────────────────────────────────

    private RawServer raw(Behaviour... behaviours) throws IOException {
        RawServer raw = new RawServer(0, List.of(behaviours));
        rawServers.add(raw);
        return raw;
    }

    private static int freePort() throws IOException {
        try (ServerSocket s = new ServerSocket(0, 1, InetAddress.getLoopbackAddress())) {
            return s.getLocalPort();
        }
    }

    /** What the raw server does with one connection, after reading its request. */
    record Behaviour(String kind, int status, String body) {
        static final Behaviour RESET = new Behaviour("reset", 0, null);
        static final Behaviour CLOSE = new Behaviour("close", 0, null);
        static final Behaviour HOLD = new Behaviour("hold", 0, null);

        static Behaviour respond(int status, String body) {
            return new Behaviour("respond", status, body);
        }
    }

    /**
     * An HTTP/1.1 server on a raw socket, one connection per request: it reads the
     * whole request (so a failure is after sending), then resets the connection,
     * closes it, holds it without answering, or answers, by its script.
     */
    static final class RawServer implements AutoCloseable {
        final List<String> requests = new CopyOnWriteArrayList<>();
        private final ServerSocket socket;
        private final List<Behaviour> script;
        private final CountDownLatch closed = new CountDownLatch(1);
        private final AtomicInteger next = new AtomicInteger();

        RawServer(int port, List<Behaviour> script) throws IOException {
            this.socket = new ServerSocket(port, 50, InetAddress.getLoopbackAddress());
            this.script = script;
            Thread.ofVirtual().start(this::acceptLoop);
        }

        String baseUrl() {
            return "http://127.0.0.1:" + socket.getLocalPort();
        }

        private void acceptLoop() {
            while (!socket.isClosed()) {
                try {
                    Socket connection = socket.accept();
                    Thread.ofVirtual().start(() -> handle(connection));
                } catch (IOException e) {
                    return;
                }
            }
        }

        private void handle(Socket connection) {
            try (connection) {
                String requestLine = readRequest(connection.getInputStream());
                if (requestLine == null) return;
                requests.add(requestLine);
                Behaviour b = script.get(Math.min(next.getAndIncrement(), script.size() - 1));
                switch (b.kind()) {
                    case "reset" -> connection.setSoLinger(true, 0);
                    case "close" -> { }
                    case "hold" -> closed.await();
                    default -> {
                        byte[] payload = b.body() == null ? new byte[0] : b.body().getBytes(StandardCharsets.UTF_8);
                        OutputStream out = connection.getOutputStream();
                        out.write(("HTTP/1.1 " + b.status() + " X\r\n"
                                + "Content-Type: application/json\r\n"
                                + "Content-Length: " + payload.length + "\r\n"
                                + "Connection: close\r\n\r\n").getBytes(StandardCharsets.US_ASCII));
                        out.write(payload);
                        out.flush();
                    }
                }
            } catch (IOException | InterruptedException ignored) {
                // the client went away
            }
        }

        /** Reads the request line, the headers and a Content-Length body; null on EOF. */
        private static String readRequest(InputStream in) throws IOException {
            BufferedReader reader = new BufferedReader(new InputStreamReader(in, StandardCharsets.ISO_8859_1));
            String requestLine = reader.readLine();
            if (requestLine == null) return null;
            int length = 0;
            for (String line = reader.readLine(); line != null && !line.isEmpty(); line = reader.readLine()) {
                String lower = line.toLowerCase(Locale.ROOT);
                if (lower.startsWith("content-length:")) {
                    length = Integer.parseInt(line.substring("content-length:".length()).trim());
                }
            }
            for (int i = 0; i < length; i++) {
                if (reader.read() < 0) break;
            }
            return requestLine;
        }

        @Override
        public void close() {
            closed.countDown();
            try {
                socket.close();
            } catch (IOException ignored) {
                // closing
            }
        }
    }
}
