package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.ingestor.contract.JobRecord;
import io.flowcatalyst.ingestor.contract.Target;
import io.flowcatalyst.ingestor.contract.HttpAuth;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RateLimit;
import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.framework.auth.PlatformTokens;
import io.flowcatalyst.framework.auth.TestOidcProvider;
import io.flowcatalyst.framework.http.Routes;
import io.flowcatalyst.framework.http.TestClient;
import io.flowcatalyst.framework.http.TestResponse;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.time.Instant;
import java.time.InstantSource;
import java.util.ArrayList;
import java.util.Base64;
import java.util.List;
import java.util.Map;
import java.util.concurrent.atomic.AtomicLong;
import org.apache.camel.CamelContext;
import org.apache.camel.ProducerTemplate;
import org.apache.camel.impl.DefaultCamelContext;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/** The HTTP listener through the framework's pipeline, into real Camel routes. */
class HttpIngestTest {

  final List<List<JobRecord>> batches = new ArrayList<>();
  final AtomicLong nanos = new AtomicLong();
  boolean platformDown;
  CamelContext camel;
  ProducerTemplate producer;
  Reconciler reconciler;
  TestClient http;

  TestOidcProvider platform;

  @BeforeEach
  void start() {
    platform = TestOidcProvider.start(InstantSource.system());
    camel = new DefaultCamelContext();
    camel.start();
    producer = camel.createProducerTemplate();
    ExchangeCounters counters = new ExchangeCounters(InstantSource.system());
    JobSink sink =
        (ingestion, jobs) -> {
          if (platformDown) {
            throw new IOException("platform unavailable");
          }
          batches.add(jobs);
        };
    reconciler =
        new Reconciler(camel, i -> new IngestionRoute(i, new CamelSourceEndpoints(SourceHarness.secrets(Map.of()), new MemoryState(), null), sink, counters));
    Secrets secrets = SourceHarness.secrets(Map.of("token", "s3cret", "password", "pw"));
    Routes routes = new Routes();
    HttpIngest ingest = new HttpIngest(reconciler, secrets, HttpIngest.limiter(nanos::get), producer, PlatformTokens.createAllowingPlainHttp(platform.issuer(), Duration.ofSeconds(5), InstantSource.system()));
    routes.post(HttpIngest.PATTERN, ingest::handle);
    routes.post(HttpIngest.PATTERN_WITH_APPLICATION, ingest::handleWithApplication);
    http = TestClient.of(routes);

    reconciler.reconcile(
        List.of(
            ingestion("ing_bearer", "orders", new HttpAuth.Bearer(new SecretRef("token")), null),
            ingestion("ing_basic", "returns", new HttpAuth.Basic("acme", new SecretRef("password")), null),
            ingestion("ing_limited", "stock", new HttpAuth.Bearer(new SecretRef("token")), new RateLimit(2, Duration.ofSeconds(10))),
            ingestion("ing_unset", "prices", new HttpAuth.Bearer(new SecretRef("missing")), null),
            ingestion("ing_fc", "users", new HttpAuth.FlowCatalystToken("crm", null), null),
            ingestion("ing_fc_one", "contacts", new HttpAuth.FlowCatalystToken("crm", "prn_SERVICE1"), null),
            // As an ingestion stored before the application was required reads back.
            ingestion("ing_fc_old", "leads", new HttpAuth.FlowCatalystToken(nullApplication(), null), null),
            // The same code in two applications: told apart by the application in the path.
            ingestion("ing_twin_a", "billing", "twin", new HttpAuth.Bearer(new SecretRef("token")), null),
            ingestion("ing_twin_b", "crm", "twin", new HttpAuth.Bearer(new SecretRef("token")), null)));
  }

  @SuppressWarnings("NullAway")
  private static String nullApplication() {
    return null;
  }

  @AfterEach
  void stop() throws IOException {
    producer.close();
    camel.stop();
    platform.close();
  }

  private static Ingestion ingestion(String id, String code, HttpAuth auth, RateLimit limit) {
    return ingestion(id, "logistics", code, auth, limit);
  }

  private static Ingestion ingestion(String id, String application, String code, HttpAuth auth, RateLimit limit) {
    return SourceHarness.ingestion(
        id, application, code, 1, new Source.Http(auth, limit), new Split.Json("$"), SourceHarness.target(Target.Mode.NEXT_ON_ERROR, 100), false);
  }

  private TestResponse post(String code, String authorization) {
    return post(code, authorization, "[{\"id\":1},{\"id\":2}]");
  }

  private TestResponse post(String code, String authorization, String body) {
    TestClient.Call call =
        http.post("/ingest/acme/" + code).header("Content-Type", "application/json").body(body);
    return (authorization == null ? call : call.header("Authorization", authorization)).send();
  }

  @Test
  void anAuthenticatedPayloadIsSplitIntoJobsBeforeTheAnswer() {
    assertThat(post("orders", "Bearer s3cret").status()).isEqualTo(202);

    assertThat(batches).singleElement().satisfies(b -> assertThat(b).extracting(JobRecord::payload).containsExactly("{\"id\":1}", "{\"id\":2}"));
  }

  @Test
  void basicCredentialsAreChecked() {
    String good = "Basic " + Base64.getEncoder().encodeToString("acme:pw".getBytes(StandardCharsets.UTF_8));
    String bad = "Basic " + Base64.getEncoder().encodeToString("acme:nope".getBytes(StandardCharsets.UTF_8));

    assertThat(post("returns", good).status()).isEqualTo(202);
    assertThat(post("returns", bad).status()).isEqualTo(401);
    assertThat(post("returns", "Basic !!!").status()).isEqualTo(401);
  }

  @Test
  void missingOrWrongCredentialsAndAnUnsetSecretAreRefused() {
    assertThat(post("orders", null).status()).isEqualTo(401);
    assertThat(post("orders", "Bearer wrong").status()).isEqualTo(401);
    assertThat(post("prices", "Bearer anything").status()).isEqualTo(401);
    assertThat(batches).isEmpty();
  }

  @Test
  void theClientIsNamedByItsIdentifierOrItsPlatformId() {
    assertThat(http.post("/ingest/clt_acme/orders").header("Authorization", "Bearer s3cret").body("[]").send().status()).isEqualTo(202);
    assertThat(http.post("/ingest/other/orders").header("Authorization", "Bearer s3cret").body("[]").send().status()).isEqualTo(404);
  }

  @Test
  void aCodeTwoApplicationsShareIsRefusedUntilThePathNamesTheApplication() {
    TestResponse ambiguous = post("twin", "Bearer s3cret");
    assertThat(ambiguous.status()).isEqualTo(409);

    TestResponse billing = http.post("/ingest/billing/acme/twin").header("Authorization", "Bearer s3cret").body("[{\"id\":7}]").send();
    assertThat(billing.status()).isEqualTo(202);
    assertThat(http.post("/ingest/crm/acme/twin").header("Authorization", "Bearer s3cret").body("[{\"id\":8}]").send().status()).isEqualTo(202);
    assertThat(http.post("/ingest/nothing/acme/twin").header("Authorization", "Bearer s3cret").body("[]").send().status()).isEqualTo(404);
    assertThat(batches.stream().flatMap(List::stream).map(JobRecord::payload)).containsExactly("{\"id\":7}", "{\"id\":8}");
  }

  @Test
  void anUnknownIngestionIsNotFound() {
    assertThat(post("nothing", "Bearer s3cret").status()).isEqualTo(404);
  }

  @Test
  void aPlatformTokenIsAcceptedOnlyForAPrincipalWithAccessToTheClientAndTheApplication() {
    String everything = token(Map.of("applications", List.of("*")));
    String thisClientAndApp = token(Map.of("clients", List.of("other", "clt_acme:acme"), "applications", List.of("app_1:billing", "app_2:crm")));
    String otherClient = token(Map.of("clients", List.of("other"), "applications", List.of("*")));
    String otherApp = token(Map.of("applications", List.of("app_1:billing")));
    String noApps = token(Map.of());
    String expired = token(Map.of("applications", List.of("*"), "exp", Instant.now().minusSeconds(3700).getEpochSecond()));
    String identity = token(Map.of("applications", List.of("*"), "token_use", "identity"));

    assertThat(post("users", everything).status()).isEqualTo(202);
    assertThat(post("users", thisClientAndApp).status()).isEqualTo(202);
    assertThat(post("users", otherClient).status()).isEqualTo(403);
    assertThat(post("users", otherApp).status()).isEqualTo(403);
    assertThat(post("users", noApps).status()).isEqualTo(403);
    assertThat(post("users", expired).status()).isEqualTo(401);
    assertThat(post("users", identity).status()).isEqualTo(401);
    assertThat(post("users", "Bearer s3cret").status()).isEqualTo(401);
    assertThat(post("users", null).status()).isEqualTo(401);
    assertThat(batches).hasSize(2);
    // A platform token does not open an ingestion that asks for its own credential.
    assertThat(post("orders", everything).status()).isEqualTo(401);
  }

  @Test
  void anIngestionNamingAPrincipalAcceptsOnlyThatPrincipal() {
    // The provider's tokens are for prn_SERVICE1 unless told otherwise.
    assertThat(post("contacts", token(Map.of("applications", List.of("*")))).status()).isEqualTo(202);
    assertThat(post("contacts", token(Map.of("applications", List.of("*"), "sub", "prn_OTHER"))).status()).isEqualTo(403);
  }

  @Test
  void anIngestionWithoutAnApplicationIsClosedToEveryToken() {
    assertThat(post("leads", token(Map.of("applications", List.of("*")))).status()).isEqualTo(403);
    assertThat(batches).isEmpty();
  }

  private String token(Map<String, Object> claims) {
    return "Bearer " + platform.accessToken(claims);
  }

  @Test
  void whenThePlatformsKeysCannotBeReadTheCallerIsToldToTryAgain() {
    String token = token(Map.of("applications", List.of("*")));
    platform.down(true);

    assertThat(post("users", token).status()).isEqualTo(503);
    assertThat(batches).isEmpty();
  }

  @Test
  void aCallerOverTheLimitIsToldToSlowDownAndWhenToRetry() {
    assertThat(post("stock", "Bearer s3cret").status()).isEqualTo(202);
    assertThat(post("stock", "Bearer s3cret").status()).isEqualTo(202);

    TestResponse limited = post("stock", "Bearer s3cret");
    assertThat(limited.status()).isEqualTo(429);
    assertThat(limited.header("Retry-After")).isEqualTo("5");
    assertThat(batches).hasSize(2);

    nanos.addAndGet(Duration.ofSeconds(5).toNanos());
    assertThat(post("stock", "Bearer s3cret").status()).isEqualTo(202);
  }

  @Test
  void aCallerWithoutTheCredentialsDoesNotUseUpTheLimit() {
    for (int i = 0; i < 5; i++) {
      assertThat(post("stock", "Bearer wrong").status()).isEqualTo(401);
    }

    assertThat(post("stock", "Bearer s3cret").status()).isEqualTo(202);
    assertThat(post("stock", "Bearer s3cret").status()).isEqualTo(202);
    assertThat(post("stock", "Bearer s3cret").status()).isEqualTo(429);
  }

  @Test
  void whenThePlatformRefusesTheJobsTheCallerIsToldToSendAgain() {
    platformDown = true;

    assertThat(post("orders", "Bearer s3cret").status()).isEqualTo(503);
  }

  @Test
  void aPayloadThatCannotBeReadIsRefusedForGood() {
    assertThat(post("orders", "Bearer s3cret", "[{\"id\":1},").status()).isEqualTo(422);
    assertThat(post("orders", "Bearer s3cret", "{\"not\": \"an array\"}").status()).isEqualTo(422);
  }
}
