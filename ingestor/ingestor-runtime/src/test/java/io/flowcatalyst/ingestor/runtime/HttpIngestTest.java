package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.ingestor.contract.JobRecord;
import io.flowcatalyst.ingestor.contract.JobResult;
import io.flowcatalyst.ingestor.contract.JobsAnswer;
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
import java.util.HashMap;
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
  final Map<String, String> ids = new HashMap<>();
  int issued;
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
        new JobSink() {
          @Override
          public void create(Ingestion ingestion, List<JobRecord> jobs) throws IOException {
            submit(ingestion, jobs);
          }

          // Answers as the platform does: an id per record, the same one for an external id seen before.
          @Override
          public JobsAnswer submit(Ingestion ingestion, List<JobRecord> jobs) throws IOException {
            if (platformDown) {
              throw new IOException("platform unavailable");
            }
            batches.add(jobs);
            boolean event = ingestion.id().startsWith("ing_event");
            List<JobResult> results = new ArrayList<>();
            int created = 0;
            for (int i = 0; i < jobs.size(); i++) {
              String external = jobs.get(i).externalId();
              String known = external == null ? null : ids.get(external);
              String id = known != null ? known : event ? "evt_" + ++issued : "djb_" + ++issued;
              if (known == null) {
                created++;
                if (external != null) {
                  ids.put(external, id);
                }
              }
              results.add(new JobResult(i, id, null, known != null));
            }
            return new JobsAnswer(event ? "EVENT" : "DISPATCH_JOB", created, results);
          }
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
            ingestion("ing_twin_b", "crm", "twin", new HttpAuth.Bearer(new SecretRef("token")), null),
            single("ing_xml", "yard", new Source.Http.Accept(List.of("application/xml", "text/xml"), Source.Http.Body.XML)),
            single("ing_obj", "tower", new Source.Http.Accept(List.of("application/json"), Source.Http.Body.JSON_OBJECT)),
            single("ing_json", "plain", new Source.Http.Accept(List.of(), Source.Http.Body.JSON)),
            single("ing_nonempty", "gate", new Source.Http.Accept(List.of(), Source.Http.Body.NONE)),
            single("ing_single", "one", null),
            single("ing_event_one", "event1", null),
            ingestion("ing_event_many", "events", new HttpAuth.Bearer(new SecretRef("token")), null)));
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

  private static Ingestion single(String id, String code, Source.Http.Accept accept) {
    return SourceHarness.ingestion(
        id,
        "logistics",
        code,
        1,
        new Source.Http(new HttpAuth.Bearer(new SecretRef("token")), null, accept),
        new Split.None(),
        SourceHarness.target(Target.Mode.IMMEDIATE, 100),
        false);
  }

  private TestResponse send(String code, String contentType, String body, String... headers) {
    TestClient.Call call = http.post("/ingest/acme/" + code).header("Authorization", "Bearer s3cret").body(body);
    if (contentType != null) {
      call = call.header("Content-Type", contentType);
    }
    for (int i = 0; i < headers.length; i += 2) {
      call = call.header(headers[i], headers[i + 1]);
    }
    return call.send();
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
  void anAddressThatKeepsFailingToLogInWaitsAndIsNotCheckedMeanwhile() {
    for (int i = 0; i < HttpIngest.AUTH_FAILURES.burst(); i++) {
      assertThat(post("orders", "Bearer wrong").status()).isEqualTo(401);
    }
    TestResponse limited = post("orders", "Bearer wrong");
    assertThat(limited.status()).isEqualTo(429);
    assertThat(limited.header("Retry-After")).isNotNull();
    // Waiting, even with the right credentials; another ingestion is not held up.
    assertThat(post("orders", "Bearer s3cret").status()).isEqualTo(429);
    assertThat(post("stock", "Bearer s3cret").status()).isEqualTo(202);
  }

  @Test
  void aRequestSentAgainUnderTheSameIdempotencyKeyHasTheSameRecordKeys() {
    for (int i = 0; i < 2; i++) {
      assertThat(
              http.post("/ingest/acme/orders")
                  .header("Authorization", "Bearer s3cret")
                  .header(HttpIngest.IDEMPOTENCY_KEY, "order-batch-42")
                  .body("[{\"id\":1},{\"id\":2}]")
                  .send()
                  .status())
          .isEqualTo(202);
    }
    assertThat(post("orders", "Bearer s3cret").status()).isEqualTo(202);

    assertThat(batches).hasSize(3);
    assertThat(batches.get(0)).extracting(JobRecord::externalId)
        .containsExactly(Identities.externalId("order-batch-42", 0), Identities.externalId("order-batch-42", 1));
    assertThat(batches.get(1)).isEqualTo(batches.get(0));
    assertThat(batches.get(2)).extracting(JobRecord::externalId).containsOnlyNulls();
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
  void aPlatformThatCannotCheckTokensDoesNotLockCallersOut() {
    // Each answer is the platform's fault, not the caller's: well past the failed-login limit, the caller
    // is still told to try again (503), never made to wait (429) as if it had guessed credentials.
    String token = token(Map.of("applications", List.of("*")));
    platform.down(true);
    for (int i = 0; i < HttpIngest.AUTH_FAILURES.burst() + 5; i++) {
      assertThat(post("users", token).status()).as("call %d", i + 1).isEqualTo(503);
    }
  }

  @Test
  void anUnsplitRecordKeepsTheTypeItsCallerSent() {
    assertThat(send("yard", "application/xml; charset=UTF-8", "<trip/>").status()).isEqualTo(202);
    assertThat(send("one", null, "{\"id\":1}").status()).isEqualTo(202);
    assertThat(send("one", "x/" + "y".repeat(120), "{\"id\":2}").status()).isEqualTo(202);
    assertThat(send("orders", "text/plain", "[{\"id\":3}]").status()).isEqualTo(202);

    assertThat(batches.get(0)).extracting(JobRecord::payloadContentType).containsExactly("application/xml; charset=UTF-8");
    // None sent, or longer than the platform keeps: JSON, as before.
    assertThat(batches.get(1)).extracting(JobRecord::payloadContentType).containsExactly("application/json");
    assertThat(batches.get(2)).extracting(JobRecord::payloadContentType).containsExactly("application/json");
    // A split payload's records are what the split makes of them, whatever the request says.
    assertThat(batches.get(3)).extracting(JobRecord::payloadContentType).containsExactly("application/json");
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

  @Test
  void aMediaTypeTheIngestionDoesNotTakeIsAnswered415() {
    TestResponse refused = send("yard", "application/json", "<a/>");
    assertThat(refused.status()).isEqualTo(415);
    assertThat(refused.tree().get("error").asString()).isEqualTo("UNSUPPORTED_MEDIA_TYPE");
    assertThat(send("yard", null, "<a/>").status()).isEqualTo(415);
    assertThat(batches).isEmpty();
  }

  @Test
  void theMediaTypeIsComparedWithoutCaseOrParameters() {
    assertThat(send("yard", "Application/XML; charset=UTF-8", "<a/>").status()).isEqualTo(202);
    assertThat(send("yard", "text/xml", "<a/>").status()).isEqualTo(202);
    // An empty list takes any media type.
    assertThat(send("plain", "text/plain", "{}").status()).isEqualTo(202);
    assertThat(send("plain", null, "{}").status()).isEqualTo(202);
  }

  @Test
  void theCredentialsAreCheckedBeforeTheMediaType() {
    TestResponse response = http.post("/ingest/acme/yard").header("Content-Type", "application/json").body("<a/>").send();
    assertThat(response.status()).isEqualTo(401);
  }

  @Test
  void anEmptyBodyIsRefusedOnceTheIngestionHasAnAcceptAndNotBefore() {
    for (String code : List.of("yard", "tower", "plain", "gate")) {
      TestResponse refused = send(code, "application/json".equals(code) ? null : code.equals("yard") ? "application/xml" : "application/json", "");
      assertThat(refused.status()).as(code).isEqualTo(400);
      assertThat(refused.tree().get("error").asString()).as(code).isEqualTo("EMPTY_BODY");
    }
    // No accept: as before, a blank payload makes no jobs and is taken.
    assertThat(send("one", "application/json", "").status()).isEqualTo(202);
    assertThat(batches).isEmpty();
  }

  @Test
  void jsonThatDoesNotParseIsRefused() {
    for (String body : List.of("{\"a\":", "nope", "{\"a\":1} {\"b\":2}", "[1,2", "{\"a\":1}}")) {
      TestResponse refused = send("plain", "application/json", body);
      assertThat(refused.status()).as(body).isEqualTo(400);
      assertThat(refused.tree().get("error").asString()).as(body).isEqualTo("INVALID_JSON");
    }
    for (String body : List.of("{}", "[]", "1", "\"s\"", "null", "{\"a\":[1,{\"b\":2}]}", "  [1] ")) {
      assertThat(send("plain", "application/json", body).status()).as(body).isEqualTo(202);
    }
  }

  @Test
  void onlyANonEmptyJsonObjectPassesJsonObject() {
    for (String body : List.of("{}", "[]", "[{\"a\":1}]", "1", "\"s\"", "null", " { } ")) {
      TestResponse refused = send("tower", "application/json", body);
      assertThat(refused.status()).as(body).isEqualTo(400);
      assertThat(refused.tree().get("error").asString()).as(body).isEqualTo("PAYLOAD_MUST_BE_A_NON_EMPTY_JSON_OBJECT");
    }
    TestResponse broken = send("tower", "application/json", "{\"a\":1");
    assertThat(broken.status()).isEqualTo(400);
    assertThat(broken.tree().get("error").asString()).isEqualTo("INVALID_JSON");
    assertThat(send("tower", "application/json", "{\"a\":{\"b\":[1,2]},\"c\":null}").status()).isEqualTo(202);
    assertThat(batches).hasSize(1);
  }

  @Test
  void xmlThatIsNotWellFormedIsRefusedWithoutEchoingIt() {
    for (String body : List.of("<a><b></a>", "<a>", "plain text SECRETWORD", "<a/><b/>", "<a>&undefined;</a>")) {
      TestResponse refused = send("yard", "application/xml", body);
      assertThat(refused.status()).as(body).isEqualTo(400);
      assertThat(refused.tree().get("error").asString()).as(body).isEqualTo("INVALID_XML");
      assertThat(refused.body()).as(body).doesNotContain("SECRETWORD");
    }
    assertThat(send("yard", "application/xml", "<?xml version=\"1.0\" encoding=\"UTF-8\"?><trip id=\"1\"><leg/>text</trip>").status()).isEqualTo(202);
  }

  @Test
  void xmlWithADoctypeIsRefusedSoNoEntityReachesTheProcessor() {
    String xxe = "<?xml version=\"1.0\"?><!DOCTYPE a [<!ENTITY x SYSTEM \"file:///etc/hostname\">]><a>&x;</a>";
    TestResponse response = send("yard", "application/xml", xxe);

    assertThat(response.status()).isEqualTo(400);
    assertThat(response.tree().get("error").asString()).isEqualTo("INVALID_XML");
    assertThat(response.tree().get("message").asString()).isEqualTo("a DOCTYPE is not accepted");
    assertThat(response.body()).doesNotContain("hostname");
    // Even a DOCTYPE with nothing in it.
    assertThat(send("yard", "application/xml", "<!DOCTYPE a><a/>").status()).isEqualTo(400);
    assertThat(batches).isEmpty();
  }

  @Test
  void theMessageGroupHeaderIsCheckedAndBecomesEveryRecordsGroup() {
    for (String bad : List.of("has space", "a/b", "x".repeat(129), "é", "")) {
      TestResponse refused = send("orders", "application/json", "[{\"id\":1}]", "X-Message-Group", bad);
      assertThat(refused.status()).as(bad).isEqualTo(400);
      assertThat(refused.tree().get("error").asString()).as(bad).isEqualTo("INVALID_MESSAGE_GROUP");
    }
    assertThat(batches).isEmpty();

    assertThat(send("orders", "application/json", "[{\"id\":1},{\"id\":2}]", "X-Message-Group", "trip_1.A:b-2").status()).isEqualTo(202);
    assertThat(send("orders", "application/json", "[{\"id\":3}]", "X-Message-Group", "x".repeat(128)).status()).isEqualTo(202);

    assertThat(batches.get(0)).extracting(JobRecord::messageGroup).containsOnly("ing_bearer:trip_1.A:b-2");
    // The correlation id is the group when the caller names none, as before.
    assertThat(batches.get(0)).extracting(JobRecord::correlationId).containsOnly("ing_bearer:trip_1.A:b-2");
    assertThat(batches.get(1)).extracting(JobRecord::messageGroup).containsOnly("ing_bearer:" + "x".repeat(128));
  }

  @Test
  void theCorrelationIdHeaderIsCheckedAndBecomesEveryRecordsCorrelationId() {
    TestResponse refused = send("orders", "application/json", "[{\"id\":1}]", "X-Correlation-Id", "c".repeat(101));
    assertThat(refused.status()).isEqualTo(400);
    assertThat(refused.tree().get("error").asString()).isEqualTo("INVALID_CORRELATION_ID");
    assertThat(batches).isEmpty();

    assertThat(send("orders", "application/json", "[{\"id\":1},{\"id\":2}]", "X-Correlation-Id", "req 42/é").status()).isEqualTo(202);
    assertThat(send("orders", "application/json", "[{\"id\":3}]", "X-Correlation-Id", "c".repeat(100)).status()).isEqualTo(202);

    assertThat(batches.get(0)).extracting(JobRecord::correlationId).containsOnly("req 42/é");
    // The group is the payload's own, as before.
    assertThat(batches.get(0)).extracting(JobRecord::messageGroup).doesNotContain("req 42/é").doesNotContainNull();
    assertThat(batches.get(1)).extracting(JobRecord::correlationId).containsOnly("c".repeat(100));
  }

  @Test
  void withoutTheHeadersTheGroupAndCorrelationAreTheSamePayloadGroupAsBefore() {
    assertThat(post("orders", "Bearer s3cret").status()).isEqualTo(202);

    JobRecord first = batches.getFirst().getFirst();
    assertThat(first.messageGroup()).startsWith("ing_bearer:c");
    assertThat(first.correlationId()).isEqualTo(first.messageGroup());
    assertThat(batches.getFirst()).extracting(JobRecord::messageGroup).containsOnly(first.messageGroup());
  }

  @Test
  void aPayloadWithoutASplitAnswersItsJobId() {
    TestResponse response = send("one", "application/json", "{\"id\":1}");

    assertThat(response.status()).isEqualTo(202);
    assertThat(response.tree().get("accepted").asBoolean()).isTrue();
    assertThat(response.tree().get("jobId").asString()).isEqualTo("djb_1");
    assertThat(response.tree().has("jobIds")).isFalse();
    assertThat(response.tree().has("eventId")).isFalse();
  }

  @Test
  void aSplitPayloadAnswersItsJobIdsInRecordOrder() {
    TestResponse response = send("orders", "application/json", "[{\"id\":1},{\"id\":2},{\"id\":3}]");

    assertThat(response.status()).isEqualTo(202);
    assertThat(response.tree().get("jobIds")).extracting(n -> n.asString()).containsExactly("djb_1", "djb_2", "djb_3");
    assertThat(response.tree().has("jobId")).isFalse();
  }

  @Test
  void aSplitPayloadWithNoRecordsAnswersAnEmptyList() {
    TestResponse response = send("orders", "application/json", "[]");

    assertThat(response.status()).isEqualTo(202);
    assertThat(response.tree().get("accepted").asBoolean()).isTrue();
    assertThat(response.tree().get("jobIds").isArray()).isTrue();
    assertThat(response.tree().get("jobIds")).isEmpty();
  }

  @Test
  void anEventTargetAnswersEventIds() {
    TestResponse one = send("event1", "application/json", "{\"id\":1}");
    assertThat(one.status()).isEqualTo(202);
    assertThat(one.tree().get("eventId").asString()).isEqualTo("evt_1");
    assertThat(one.tree().has("jobId")).isFalse();

    TestResponse many = send("events", "application/json", "[{\"id\":1},{\"id\":2}]");
    assertThat(many.status()).isEqualTo(202);
    assertThat(many.tree().get("eventIds")).extracting(n -> n.asString()).containsExactly("evt_2", "evt_3");
    assertThat(many.tree().has("jobIds")).isFalse();
  }

  @Test
  void aRequestSentAgainUnderTheSameIdempotencyKeyAnswersTheSameIds() {
    TestResponse first = send("orders", "application/json", "[{\"id\":1},{\"id\":2}]", HttpIngest.IDEMPOTENCY_KEY, "batch-7");
    TestResponse again = send("orders", "application/json", "[{\"id\":1},{\"id\":2}]", HttpIngest.IDEMPOTENCY_KEY, "batch-7");
    TestResponse single = send("one", "application/json", "{\"id\":1}", HttpIngest.IDEMPOTENCY_KEY, "one-1");
    TestResponse singleAgain = send("one", "application/json", "{\"id\":1}", HttpIngest.IDEMPOTENCY_KEY, "one-1");

    assertThat(first.tree().get("jobIds")).extracting(n -> n.asString()).containsExactly("djb_1", "djb_2");
    assertThat(again.tree()).isEqualTo(first.tree());
    assertThat(single.tree().get("jobId").asString()).isEqualTo(singleAgain.tree().get("jobId").asString());
  }

  @Test
  void theMediaTypeComparisonIgnoresCaseAndParameters() {
    assertThat(HttpIngest.accepted(List.of("a/b"), "A/B;x=y")).isTrue();
    assertThat(HttpIngest.accepted(List.of("a/b"), "a/c")).isFalse();
    assertThat(HttpIngest.accepted(List.of("a/b"), null)).isFalse();
  }
}
