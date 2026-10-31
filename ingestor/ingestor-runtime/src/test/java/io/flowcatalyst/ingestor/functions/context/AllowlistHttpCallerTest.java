package io.flowcatalyst.ingestor.functions.context;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.sun.net.httpserver.HttpServer;
import io.flowcatalyst.framework.httpclient.HttpCapture;
import io.flowcatalyst.framework.httpclient.HttpClients;
import io.flowcatalyst.function.HttpCall;
import io.flowcatalyst.function.HttpCallRefusedException;
import io.flowcatalyst.function.HttpReply;
import io.flowcatalyst.function.HttpResponseTooLargeException;
import java.net.InetSocketAddress;
import java.net.http.HttpTimeoutException;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.Executors;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/// [AllowlistHttpCaller] over the framework's HTTP client, against real local servers: exact-host allow, `http://`
/// refused off loopback, a 302 handed back unfollowed, the timeout cut to the remaining [InvocationDeadline], the
/// response cap, and the call visible to the framework's capture hook (so a trace sees it).
class AllowlistHttpCallerTest {

    private HttpServer server;
    private HttpClients clients;

    @BeforeEach
    void start() {
        clients = HttpClients.create();
    }

    @AfterEach
    void stop() {
        if (server != null) {
            server.stop(0);
        }
        clients.close();
    }

    private HttpServer serve(String path, com.sun.net.httpserver.HttpHandler handler) throws Exception {
        HttpServer s = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        s.createContext(path, handler);
        s.setExecutor(Executors.newVirtualThreadPerTaskExecutor());
        s.start();
        this.server = s;
        return s;
    }

    private AllowlistHttpCaller caller(String... allow) {
        return new AllowlistHttpCaller(new OutboundClients(clients), List.of(allow), Clock.systemUTC());
    }

    @Test
    void anExactHostOnTheAllowlistIsPermitted() throws Exception {
        HttpServer s = serve("/ok", ex -> {
            byte[] body = "hi".getBytes();
            ex.getResponseHeaders().add("X-Reply", "1");
            ex.getResponseHeaders().add("Set-Cookie", "a=b");
            ex.sendResponseHeaders(200, body.length);
            ex.getResponseBody().write(body);
            ex.close();
        });

        HttpReply reply = caller("localhost").send(new HttpCall("GET", "http://localhost:" + s.getAddress().getPort() + "/ok", Map.of(), new byte[0]));

        assertThat(reply.status()).isEqualTo(200);
        assertThat(new String(reply.body())).isEqualTo("hi");
        assertThat(reply.headers()).containsKey("x-reply").containsKey("set-cookie");
    }

    @Test
    void theRequestIsSentAsTheFunctionBuiltIt() throws Exception {
        List<String> seen = new CopyOnWriteArrayList<>();
        HttpServer s = serve("/", ex -> {
            seen.add(ex.getRequestMethod() + " " + ex.getRequestURI() + " ct=" + ex.getRequestHeaders().getFirst("Content-Type")
                    + " x=" + ex.getRequestHeaders().getFirst("X-Thing") + " body=" + new String(ex.getRequestBody().readAllBytes()));
            ex.sendResponseHeaders(204, -1);
            ex.close();
        });

        HttpReply reply = caller("localhost").send(new HttpCall("post", "http://localhost:" + s.getAddress().getPort() + "/a/b?x=1&y=a%20b",
                Map.of("X-Thing", List.of("v"), "content-type", List.of("application/json")), "{\"n\":1}".getBytes()));

        assertThat(reply.status()).isEqualTo(204);
        assertThat(seen).containsExactly("POST /a/b?x=1&y=a%20b ct=application/json x=v body={\"n\":1}");
    }

    @Test
    void aHostNotOnTheAllowlistIsRefused() throws Exception {
        HttpServer s = serve("/ok", ex -> {
            ex.sendResponseHeaders(200, -1);
            ex.close();
        });
        assertThatThrownBy(() -> caller("some-other-host.test").send(new HttpCall("GET", "http://localhost:" + s.getAddress().getPort() + "/ok", Map.of(), new byte[0])))
                .isInstanceOf(HttpCallRefusedException.class);
    }

    @Test
    void httpToANonLoopbackHostIsRefusedBeforeAnyNetworkCall() {
        assertThatThrownBy(() -> caller("external.example.test").send(new HttpCall("GET", "http://external.example.test/x", Map.of(), new byte[0])))
                .isInstanceOf(HttpCallRefusedException.class);
        assertThatThrownBy(() -> caller("external.example.test").send(new HttpCall("GET", "ftp://external.example.test/x", Map.of(), new byte[0])))
                .isInstanceOf(HttpCallRefusedException.class);
    }

    @Test
    void credentialsInTheUrlAndRefusedHeadersAndMethodsAreRefused() throws Exception {
        HttpServer s = serve("/", ex -> {
            ex.sendResponseHeaders(200, -1);
            ex.close();
        });
        String url = "http://localhost:" + s.getAddress().getPort() + "/";
        assertThatThrownBy(() -> caller("localhost").send(new HttpCall("GET", "http://user:pw@localhost:" + s.getAddress().getPort() + "/", Map.of(), new byte[0])))
                .isInstanceOf(HttpCallRefusedException.class);
        assertThatThrownBy(() -> caller("localhost").send(new HttpCall("GET", url, Map.of("Host", List.of("evil.test")), new byte[0])))
                .isInstanceOf(HttpCallRefusedException.class);
        assertThatThrownBy(() -> caller("localhost").send(new HttpCall("TRACE", url, Map.of(), new byte[0])))
                .isInstanceOf(HttpCallRefusedException.class);
    }

    @Test
    void aRedirectIsHandedBackNotFollowed() throws Exception {
        HttpServer s = serve("/redirect", ex -> {
            ex.getResponseHeaders().add("Location", "http://localhost:1/elsewhere");
            ex.sendResponseHeaders(302, -1);
            ex.close();
        });

        HttpReply reply = caller("localhost").send(new HttpCall("GET", "http://localhost:" + s.getAddress().getPort() + "/redirect", Map.of(), new byte[0]));

        assertThat(reply.status()).isEqualTo(302);
        assertThat(reply.headers().get("location")).contains("http://localhost:1/elsewhere");
    }

    @Test
    void theTimeoutIsCutToTheRemainingInvocationDeadline() throws Exception {
        HttpServer s = serve("/slow", ex -> {
            try {
                Thread.sleep(5000);
            } catch (InterruptedException ignored) {
                Thread.currentThread().interrupt();
            }
            try {
                ex.sendResponseHeaders(200, -1);
            } catch (Exception ignored) {
                // the client has given up
            }
            ex.close();
        });
        AllowlistHttpCaller caller = caller("localhost");
        HttpCall call = new HttpCall("GET", "http://localhost:" + s.getAddress().getPort() + "/slow", Map.of(), new byte[0]);

        Instant deadline = Clock.systemUTC().instant().plusMillis(300);
        long start = System.nanoTime();
        assertThatThrownBy(() -> ScopedValue.where(InvocationDeadline.CURRENT, deadline).call(() -> caller.send(call)))
                .isInstanceOf(HttpTimeoutException.class);
        assertThat(Duration.ofNanos(System.nanoTime() - start)).as("the deadline, not the 30 s default or the server's 5 s").isLessThan(Duration.ofSeconds(3));
    }

    @Test
    void aDeadlineThatHasPassedRefusesTheCall() throws Exception {
        HttpServer s = serve("/", ex -> {
            ex.sendResponseHeaders(200, -1);
            ex.close();
        });
        AllowlistHttpCaller caller = caller("localhost");
        Instant past = Clock.systemUTC().instant().minusSeconds(1);
        assertThatThrownBy(() -> ScopedValue.where(InvocationDeadline.CURRENT, past)
                .call(() -> caller.send(new HttpCall("GET", "http://localhost:" + s.getAddress().getPort() + "/", Map.of(), new byte[0]))))
                .isInstanceOf(HttpCallRefusedException.class);
    }

    @Test
    void theCallsOwnTimeoutIsHonouredWhenTighterThanTheDefault() throws Exception {
        HttpServer s = serve("/slow", ex -> {
            try {
                Thread.sleep(5000);
            } catch (InterruptedException ignored) {
                Thread.currentThread().interrupt();
            }
            try {
                ex.sendResponseHeaders(200, -1);
            } catch (Exception ignored) {
                // the client has given up
            }
            ex.close();
        });
        HttpCall call = new HttpCall("GET", "http://localhost:" + s.getAddress().getPort() + "/slow", Map.of(), new byte[0], Duration.ofMillis(300));

        long start = System.nanoTime();
        assertThatThrownBy(() -> caller("localhost").send(call)).isInstanceOf(HttpTimeoutException.class);
        assertThat(Duration.ofNanos(System.nanoTime() - start)).isLessThan(Duration.ofSeconds(3));
    }

    /// A response past 16 MiB fails the call with RESPONSE_TOO_LARGE, whether or not it declared its length; one at the
    /// cap is delivered whole.
    @Test
    void aResponseLargerThanTheCapFailsTheCallAndOneAtTheCapIsDelivered() throws Exception {
        int cap = HttpResponseTooLargeException.MAX_BYTES;
        HttpServer s = serve("/", ex -> {
            boolean chunked = ex.getRequestURI().getPath().endsWith("chunked");
            int size = ex.getRequestURI().getPath().startsWith("/over") ? cap + 1 : cap;
            ex.sendResponseHeaders(200, chunked ? 0 : size);
            byte[] block = new byte[64 * 1024];
            int left = size;
            try (var out = ex.getResponseBody()) {
                while (left > 0) {
                    int n = Math.min(block.length, left);
                    out.write(block, 0, n);
                    left -= n;
                }
            } catch (java.io.IOException ignored) {
                // the caller hung up at the cap
            }
        });
        AllowlistHttpCaller caller = caller("localhost");
        String base = "http://localhost:" + s.getAddress().getPort();
        for (String path : List.of("/over", "/over-chunked")) {
            assertThatThrownBy(() -> caller.send(new HttpCall("GET", base + path, Map.of(), new byte[0])))
                    .as(path).isInstanceOf(HttpResponseTooLargeException.class).hasMessageContaining("RESPONSE_TOO_LARGE");
        }
        assertThat(caller.send(new HttpCall("GET", base + "/at-chunked", Map.of(), new byte[0])).body()).hasSize(cap);
    }

    /// The calls go through the framework's client, so its capture hook sees them (what a trace window reads).
    @Test
    void theCallIsVisibleToTheFrameworksCaptureHook() throws Exception {
        HttpServer s = serve("/ok", ex -> {
            ex.sendResponseHeaders(200, -1);
            ex.close();
        });
        List<HttpCapture.Exchange> captured = new ArrayList<>();

        ScopedValue.where(HttpCapture.CURRENT, (HttpCapture.Sink) captured::add)
                .call(() -> caller("localhost").send(new HttpCall("GET", "http://localhost:" + s.getAddress().getPort() + "/ok", Map.of("Authorization", List.of("Bearer abc")), new byte[0])));

        assertThat(captured).hasSize(1);
        assertThat(captured.get(0).service()).isEqualTo(OutboundClients.SERVICE_NAME);
        assertThat(captured.get(0).status()).isEqualTo(200);
        assertThat(captured.get(0).requestHeaders().get("Authorization")).containsExactly("[redacted]");
    }
}
