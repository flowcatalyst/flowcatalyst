package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;

import com.sun.net.httpserver.HttpExchange;
import io.flowcatalyst.framework.core.App;
import io.flowcatalyst.framework.http.jetty.HttpServer;
import io.flowcatalyst.ingestor.functions.Fx;
import io.flowcatalyst.ingestor.functions.control.Control;
import java.io.IOException;
import java.net.InetAddress;
import java.net.InetSocketAddress;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.nio.file.Path;
import java.time.Duration;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.function.BooleanSupplier;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import tools.jackson.databind.ObjectMapper;

/**
 * The function role of the whole application, against a small stand-in for the platform: the settings
 * that switch it, the control plane as the runtime's service account, the listener on the application's own
 * server, and a functions-only process that takes no ingestion leases.
 */
@Timeout(120)
class FunctionRoleTest {

  record Seen(String method, String path, String query, String authorization, String body) {}

  final List<Seen> seen = new CopyOnWriteArrayList<>();
  final ObjectMapper json = io.flowcatalyst.framework.http.Json.mapper();
  com.sun.net.httpserver.HttpServer platform;
  Fx.Jar jar;
  volatile String desired;

  @BeforeEach
  void start() throws Exception {
    jar = Fx.hello("role");
    String describe = Fx.describe("fixture.app.Hello", List.of(Fx.endpoint("GET", "/hello/{name}", "none")));
    Control.Desired doc = Fx.desired(5, Fx.function("fn_1", "app.hello").version(Fx.version(1, jar, describe, "live")).build());
    desired = json.writeValueAsString(doc);
    platform = com.sun.net.httpserver.HttpServer.create(new InetSocketAddress(InetAddress.getLoopbackAddress(), 0), 0);
    platform.createContext("/", this::handle);
    platform.setExecutor(java.util.concurrent.Executors.newVirtualThreadPerTaskExecutor());
    platform.start();
  }

  @AfterEach
  void stop() {
    platform.stop(0);
  }

  private void handle(HttpExchange e) throws IOException {
    String body = new String(e.getRequestBody().readAllBytes(), StandardCharsets.UTF_8);
    String path = e.getRequestURI().getPath();
    seen.add(new Seen(e.getRequestMethod(), path, String.valueOf(e.getRequestURI().getRawQuery()), e.getRequestHeaders().getFirst("Authorization"), body));
    e.getResponseHeaders().add("Content-Type", "application/json");
    if (path.endsWith("/token")) {
      reply(e, 200, "{\"access_token\":\"tok-role\",\"token_type\":\"Bearer\",\"expires_in\":3600}".getBytes(StandardCharsets.UTF_8));
    } else if (path.equals("/control/functions/desired")) {
      e.getResponseHeaders().add("ETag", "\"e1\"");
      if ("\"e1\"".equals(e.getRequestHeaders().getFirst("If-None-Match"))) {
        // a long-poll that nothing changes: held for a moment, then 304
        try {
          Thread.sleep(300);
        } catch (InterruptedException ex) {
          Thread.currentThread().interrupt();
        }
        reply(e, 304, new byte[0]);
      } else {
        reply(e, 200, desired.getBytes(StandardCharsets.UTF_8));
      }
    } else if (path.startsWith("/control/functions/artifacts/")) {
      reply(e, 200, jar.bytes());
    } else if (path.equals("/control/functions/heartbeat")) {
      reply(e, 204, new byte[0]);
    } else {
      reply(e, 404, "{}".getBytes(StandardCharsets.UTF_8));
    }
    e.close();
  }

  private static void reply(HttpExchange e, int status, byte[] body) throws IOException {
    e.sendResponseHeaders(status, body.length == 0 ? -1 : body.length);
    if (body.length > 0) {
      e.getResponseBody().write(body);
    }
  }

  private Map<String, String> env() {
    Map<String, String> env = new HashMap<>();
    // Plain http to the platform is accepted for APP_ENV test only.
    env.put("APP_ENV", "test");
    env.put("HTTP_PORT", "0");
    env.put("HTTP_HOST", "127.0.0.1");
    env.put("FLOWCATALYST_BASE_URL", "http://127.0.0.1:" + platform.getAddress().getPort());
    env.put("FLOWCATALYST_CLIENT_ID", "oac_test");
    env.put("FLOWCATALYST_CLIENT_SECRET", "test-secret");
    return env;
  }

  private static void await(BooleanSupplier condition, String what) throws InterruptedException {
    long end = System.nanoTime() + Duration.ofSeconds(30).toNanos();
    while (!condition.getAsBoolean()) {
      if (System.nanoTime() > end) {
        throw new AssertionError("timed out waiting for " + what);
      }
      Thread.sleep(20);
    }
  }

  private boolean saw(String path) {
    return seen.stream().anyMatch(s -> s.path().equals(path));
  }

  private HttpResponse<String> get(App app, String path) throws Exception {
    int port = app.get(HttpServer.class).port();
    return HttpClient.newHttpClient()
        .send(HttpRequest.newBuilder(URI.create("http://127.0.0.1:" + port + path)).GET().build(), HttpResponse.BodyHandlers.ofString());
  }

  @Test
  void aFunctionsOnlyProcessHostsTheFunctionsOfItsPoolAndTakesNoIngestionLeases() throws Exception {
    Map<String, String> env = env();
    env.put("INGEST_INGESTION_ENABLED", "false");
    env.put("INGEST_FUNCTIONS_POOL", "blue");
    env.put("INGEST_RUNTIME_ID", "runtime-fn-1");

    try (App app = IngestRuntime.builder().configDirectory(Path.of("config")).environment(env).start()) {
      assertThat(app.config().get(RuntimeModule.FUNCTIONS_ENABLED)).isTrue();
      assertThat(app.config().get(RuntimeModule.FUNCTIONS_POOL)).isEqualTo("blue");

      await(() -> saw("/control/functions/heartbeat") && seen.stream().anyMatch(s -> s.path().equals("/control/functions/heartbeat") && s.body().contains("\"LOADED\"")), "the function to be reported loaded");

      HttpResponse<String> hello = get(app, "/fn/app.hello/hello/ann");
      assertThat(hello.statusCode()).isEqualTo(200);
      assertThat(json.readTree(hello.body()).path("name").asString()).isEqualTo("ann");

      Seen desiredCall = seen.stream().filter(s -> s.path().equals("/control/functions/desired")).findFirst().orElseThrow();
      assertThat(desiredCall.query()).contains("pool=blue").contains("runtimes=jvm");
      assertThat(desiredCall.authorization()).isEqualTo("Bearer tok-role");
      Seen beat = seen.stream().filter(s -> s.path().equals("/control/functions/heartbeat")).reduce((a, b) -> b).orElseThrow();
      assertThat(json.readTree(beat.body()).path("runnerId").asString()).isEqualTo("runtime-fn-1");
      assertThat(json.readTree(beat.body()).path("capabilities").path("runnerKind").asString()).isEqualTo("ingestor-jvm");
      assertThat(json.readTree(beat.body()).path("revision").asLong()).isEqualTo(5);

      // no ingestion: nothing polled, no lease taken, no ingest endpoint
      Thread.sleep(500);
      assertThat(seen).noneMatch(s -> s.path().startsWith("/api/ingestion-runtime/"));
      assertThat(get(app, "/ingest/acme/orders").statusCode()).isEqualTo(404);
    }
  }

  @Test
  void theFunctionRoleCanBeSwitchedOff() throws Exception {
    Map<String, String> env = env();
    env.put("INGEST_INGESTION_ENABLED", "false");
    env.put("INGEST_FUNCTIONS_ENABLED", "false");

    try (App app = IngestRuntime.builder().configDirectory(Path.of("config")).environment(env).start()) {
      assertThat(app.config().get(RuntimeModule.FUNCTIONS_ENABLED)).isFalse();
      Thread.sleep(800);
      assertThat(seen).noneMatch(s -> s.path().startsWith("/control/"));
      assertThat(get(app, "/fn/app.hello/hello/ann").statusCode()).isEqualTo(404);
    }
  }

  @Test
  void bothRolesAreOnByDefault() {
    App app = IngestRuntime.builder().configDirectory(Path.of("config")).environment(env()).start();
    try {
      assertThat(app.config().get(RuntimeModule.FUNCTIONS_ENABLED)).isTrue();
      assertThat(app.config().get(RuntimeModule.FUNCTIONS_POOL)).isEqualTo("default");
      assertThat(app.config().get(RuntimeModule.INGESTION_ENABLED)).isTrue();
    } finally {
      app.close();
    }
  }
}
