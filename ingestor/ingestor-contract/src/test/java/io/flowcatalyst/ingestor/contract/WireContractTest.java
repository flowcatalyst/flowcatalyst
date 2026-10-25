package io.flowcatalyst.ingestor.contract;

import static org.assertj.core.api.Assertions.assertThat;

import java.time.Duration;
import java.time.Instant;
import java.util.List;
import org.junit.jupiter.api.Test;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.json.JsonMapper;

/**
 * The records against the platform's runtime API (api/openapi.lock.json, docs/ingestion-design.md
 * section 7): an answer as the platform writes it is read, and a poll and a report are written with
 * exactly the fields the platform takes.
 */
class WireContractTest {

  private final ObjectMapper mapper = JsonMapper.builder().build();

  /** A poll answer as the platform's runtimeapi renders it: a custom source, a connection, a test. */
  private static final String ANSWER =
      """
      {"asOf":"2026-10-24T10:00:00Z","leaseFor":"PT3M","ingestions":[
        {"id":"ing_1","version":4,"clientId":"clt_0HZ","clientIdentifier":"acme","applicationCode":"logistics",
         "code":"orders","name":"Orders",
         "source":{"kind":"custom","type":"generator","settings":{"every":"PT10S","count":3}},
         "split":{"kind":"json","arrayPath":"$"},
         "target":{"code":"logistics:orders:record:map","targetUrl":"https://f.example/map","pool":"acme-pool",
                   "connection":"acme-fn","mode":"BLOCK_ON_ERROR","batchSize":50},
         "keepFailedPayloads":true}],
       "tests":[{"id":"tst_1","expiresAt":"2026-10-24T10:05:00Z","ingestion":
        {"id":"ing_test","version":0,"clientId":"clt_0HZ","clientIdentifier":"acme","applicationCode":"logistics",
         "code":"test","name":"",
         "source":{"kind":"api-poll","url":"https://api.example/x","method":"GET","auth":{"kind":"none"},
                   "results":{"kind":"records"},"every":"PT1M","idleDelay":"PT5M"},
         "split":{"kind":"none"},
         "target":{"code":"a:b:c:d","targetUrl":"https://f.example/map","pool":"p","mode":"IMMEDIATE","batchSize":1},
         "keepFailedPayloads":false}}]}
      """;

  @Test
  void aPollAnswerAsThePlatformWritesItIsRead() {
    PollAnswer answer = mapper.readValue(ANSWER, PollAnswer.class);

    assertThat(answer.leaseFor()).isEqualTo(Duration.ofMinutes(3));
    Ingestion ingestion = answer.ingestions().getFirst();
    assertThat(ingestion.clientId()).isEqualTo("clt_0HZ");
    assertThat(ingestion.clientIdentifier()).isEqualTo("acme");
    assertThat(ingestion.applicationCode()).isEqualTo("logistics");
    assertThat(ingestion.source()).isInstanceOf(Source.Custom.class);
    assertThat(ingestion.source().singleOwner()).isTrue();
    assertThat(ingestion.target().pool()).isEqualTo("acme-pool");
    assertThat(ingestion.target().connection()).isEqualTo("acme-fn");
    assertThat(ingestion.target().serviceAccount()).isNull();
    assertThat(ingestion.target().mode()).isEqualTo(Target.Mode.BLOCK_ON_ERROR);
    assertThat(ingestion.keepFailedPayloads()).isTrue();
    TestRequest test = answer.testList().getFirst();
    assertThat(test.ingestion().id()).isEqualTo("ing_test");
    assertThat(test.ingestion().source()).isInstanceOf(Source.ApiPoll.class);
  }

  @Test
  void anAnswerWithoutTestsAndWithFieldsAddedLaterIsStillRead() {
    PollAnswer answer =
        mapper.readValue("{\"asOf\":\"2026-10-24T10:00:00Z\",\"leaseFor\":\"PT3M\",\"ingestions\":[],\"added\":1}", PollAnswer.class);

    assertThat(answer.testList()).isEmpty();
  }

  @Test
  void aCamelRouteIsACustomSourceOfItsTypeAndTheRouteIsItsYamlSetting() {
    Source route = mapper.readValue("{\"kind\":\"custom\",\"type\":\"camel-route\",\"settings\":{\"yaml\":\"from:\\n  uri: timer:x\"}}", Source.class);

    assertThat(CamelRoutes.yaml(route)).isEqualTo("from:\n  uri: timer:x");
    assertThat(CamelRoutes.yaml(new Source.Http(new HttpAuth.Bearer(new SecretRef("t")), null))).isNull();
    assertThat(CamelRoutes.problem(CamelRoutes.yaml(route))).isNull();
  }

  @Test
  void aPollCarriesTheContractVersionAndTheKindsWithTheirPermissionAndSettings() {
    SourceKind generator =
        new SourceKind(
            "generator", "Generator", "Made-up records.", List.of(SourceKind.field("every", "Every", SourceKind.Type.DURATION).required().build()));

    JsonNode poll =
        mapper.readTree(
            mapper.writeValueAsString(
                new RuntimePoll("runtime-a", List.of(RuntimeKind.of(generator, null), CamelRoutes.kind()), "http://runtime-a:8091")));

    assertThat(poll.propertyNames()).containsExactlyInAnyOrder("runtimeId", "contractVersion", "kinds", "url", "builtinKinds");
    assertThat(poll.get("contractVersion").asInt()).isEqualTo(2);
    assertThat(poll.get("builtinKinds").valueStream().map(JsonNode::asString).toList())
        .containsExactlyInAnyOrder("sftp", "http", "rabbitmq", "sqs", "s3", "database-poll", "api-poll");
    JsonNode first = poll.get("kinds").get(0);
    assertThat(first.get("kind").asString()).isEqualTo("generator");
    assertThat(first.has("requiredPermission")).isFalse();
    assertThat(first.get("settings").get(0).get("name").asString()).isEqualTo("every");
    assertThat(first.get("settings").get(0).get("type").asString()).isEqualTo("DURATION");
    JsonNode camel = poll.get("kinds").get(1);
    assertThat(camel.get("kind").asString()).isEqualTo("camel-route");
    assertThat(camel.get("requiredPermission").asString()).isEqualTo("platform:ingestion:ingestion:route");
  }

  @Test
  void aReportHasExactlyTheFieldsThePlatformTakes() {
    Instant at = Instant.parse("2026-10-24T10:00:00Z");
    RuntimeReport report =
        new RuntimeReport(
            "runtime-a",
            List.of(new RuntimeReport.RouteState("ing_1", 4, RuntimeReport.RouteState.Status.FAILING, "login refused")),
            List.of(new RuntimeReport.ExchangeWindow("ing_1", at.minusSeconds(60), at, 3, 1, 9, 9)),
            List.of(new RuntimeReport.Failure("ing_1", 4, at, RuntimeReport.Failure.Kind.AUTH, "refused", 2L, null)),
            List.of(new ConnectionTest("tst_1", ConnectionTest.Outcome.RESULTS, List.of("logged in"), 5L, null)));

    JsonNode json = mapper.readTree(mapper.writeValueAsString(report));

    assertThat(json.propertyNames()).containsExactlyInAnyOrder("runtimeId", "routes", "metrics", "failures", "tests");
    assertThat(propertyNames(json.get("routes").get(0))).containsExactlyInAnyOrder("ingestionId", "version", "status", "error");
    assertThat(json.get("routes").get(0).get("status").asString()).isEqualTo("FAILING");
    assertThat(propertyNames(json.get("metrics").get(0)))
        .containsExactlyInAnyOrder("ingestionId", "from", "to", "completed", "failed", "records", "dispatchJobs");
    assertThat(json.get("metrics").get(0).get("to").asString()).isEqualTo("2026-10-24T10:00:00Z");
    assertThat(propertyNames(json.get("failures").get(0)))
        .containsExactlyInAnyOrder("ingestionId", "version", "at", "kind", "message", "afterRecords");
    assertThat(propertyNames(json.get("tests").get(0))).containsExactlyInAnyOrder("id", "outcome", "steps", "records");
  }

  @Test
  void aRejectedPayloadSaysWhoseFaultItWasAndNoTextCarriesANul() {
    Instant at = Instant.parse("2026-10-24T10:00:00Z");
    RuntimeReport.Failure refused =
        new RuntimeReport.Failure(
            "ing_1", 4, at, RuntimeReport.Failure.Kind.REJECTED, "the platform refused\u0000 it", 0L, "a\u0000b", RuntimeReport.Failure.Category.REJECTED);

    JsonNode json = mapper.readTree(mapper.writeValueAsString(refused));

    assertThat(json.get("category").asString()).isEqualTo("rejected");
    assertThat(json.get("message").asString()).isEqualTo("the platform refused it");
    assertThat(json.get("excerpt").asString()).isEqualTo("ab");
    assertThat(mapper.readTree(mapper.writeValueAsString(new RuntimeReport.Failure("ing_1", 4, at, RuntimeReport.Failure.Kind.REJECTED, "x", 0L, null, RuntimeReport.Failure.Category.PAYLOAD))).get("category").asString())
        .isEqualTo("payload");
    assertThat(new RuntimeReport.RouteState("ing_1", 1, RuntimeReport.RouteState.Status.FAILED, "bad\u0000").error()).isEqualTo("bad");
    assertThat(new ConnectionTest("t", ConnectionTest.Outcome.FAILED, List.of("s\u0000tep"), null, "e\u0000x").steps()).containsExactly("step");
  }

  @Test
  void anSftpSourceWithoutAHostKeyAcceptsAnyOnlyWhenItSaysSo() {
    Source.Sftp plain = mapper.readValue(
        "{\"kind\":\"sftp\",\"host\":\"h\",\"port\":22,\"directory\":\"/d\",\"username\":\"u\",\"credential\":{\"name\":\"c\"},\"every\":\"PT1M\"}",
        Source.Sftp.class);
    Source.Sftp open = mapper.readValue(
        "{\"kind\":\"sftp\",\"host\":\"h\",\"port\":22,\"directory\":\"/d\",\"username\":\"u\",\"credential\":{\"name\":\"c\"},\"every\":\"PT1M\",\"acceptAnyHostKey\":true}",
        Source.Sftp.class);

    assertThat(plain.acceptsAnyHostKey()).isFalse();
    assertThat(open.acceptsAnyHostKey()).isTrue();
  }

  @Test
  void aTestAnswerStaysWithinWhatThePlatformTakes() {
    List<String> steps = new java.util.ArrayList<>();
    for (int i = 0; i < 60; i++) {
      steps.add("x".repeat(600));
    }

    ConnectionTest answer = new ConnectionTest("tst_1", ConnectionTest.Outcome.FAILED, steps, null, null);

    assertThat(answer.steps()).hasSize(ConnectionTest.MAX_STEPS);
    assertThat(answer.steps()).allSatisfy(step -> assertThat(step).hasSize(ConnectionTest.MAX_STEP));
  }

  private static List<String> propertyNames(JsonNode node) {
    return node.propertyNames().stream().toList();
  }
}
