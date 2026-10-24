package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.sun.net.httpserver.HttpServer;
import io.flowcatalyst.framework.http.Json;
import io.flowcatalyst.framework.httpclient.Auth;
import io.flowcatalyst.framework.httpclient.HttpClients;
import io.flowcatalyst.framework.httpclient.RetryPolicy;
import io.flowcatalyst.framework.httpclient.ServiceOptions;
import io.flowcatalyst.ingestor.contract.CamelRoutes;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.JobRecord;
import io.flowcatalyst.ingestor.contract.PollAnswer;
import io.flowcatalyst.ingestor.contract.RuntimeReport;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.ingestor.contract.Target;
import java.io.IOException;
import java.net.InetAddress;
import java.net.InetSocketAddress;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import tools.jackson.databind.JsonNode;

/** The runtime API calls against a fake platform that records what it is sent and answers as the platform does. */
class PlatformApiTest {

  /** One request as the fake platform saw it. */
  record Seen(String method, String target, String authorization, String body) {}

  HttpServer platform;
  final List<Seen> seen = new ArrayList<>();
  final List<Integer> statuses = new ArrayList<>();
  String answer = "";
  HttpClients clients;
  PlatformApi api;
  final Ingestion ingestion =
      SourceHarness.ingestion("ing_1", 1, new Source.Sqs("eu-west-1", "q", null), new Split.None(), SourceHarness.target(Target.Mode.IMMEDIATE, 10), false);

  @BeforeEach
  void start() throws IOException {
    platform = HttpServer.create(new InetSocketAddress(InetAddress.getLoopbackAddress(), 0), 0);
    platform.createContext("/", exchange -> {
      seen.add(
          new Seen(
              exchange.getRequestMethod(),
              exchange.getRequestURI().getRawPath() + (exchange.getRequestURI().getRawQuery() == null ? "" : "?" + exchange.getRequestURI().getRawQuery()),
              exchange.getRequestHeaders().getFirst("Authorization"),
              new String(exchange.getRequestBody().readAllBytes(), StandardCharsets.UTF_8)));
      int status = statuses.isEmpty() ? 200 : statuses.removeFirst();
      byte[] out = answer.getBytes(StandardCharsets.UTF_8);
      exchange.getResponseHeaders().add("Content-Type", "application/json");
      exchange.sendResponseHeaders(status, out.length == 0 ? -1 : out.length);
      if (out.length > 0) {
        exchange.getResponseBody().write(out);
      }
      exchange.close();
    });
    platform.start();
    clients = HttpClients.create();
    api =
        new PlatformApi(
            clients.service(
                ServiceOptions.builder("platform", URI.create("http://127.0.0.1:" + platform.getAddress().getPort()))
                    .requestTimeout(Duration.ofSeconds(5))
                    .retry(RetryPolicy.defaults().withBackoff(Duration.ofMillis(1), Duration.ofMillis(1)))
                    .auth(Auth.bearer(() -> "runtime-token"))
                    .build()),
            "runtime 1",
            "http://runtime-1:8091",
            List.of(CamelRoutes.kind()));
  }

  @AfterEach
  void stop() {
    clients.close();
    platform.stop(0);
  }

  @Test
  void aPollSendsTheRuntimeIdTheContractVersionAndTheKindsAndReadsTheAnswer() throws Exception {
    answer = "{\"asOf\":\"2026-10-24T10:00:00Z\",\"leaseFor\":\"PT3M\",\"ingestions\":[],\"tests\":[]}";

    PollAnswer polled = api.poll();

    assertThat(polled.leaseFor()).isEqualTo(Duration.ofMinutes(3));
    Seen poll = seen.getFirst();
    assertThat(poll.method()).isEqualTo("POST");
    assertThat(poll.target()).isEqualTo("/api/ingestion-runtime/v1/poll");
    assertThat(poll.authorization()).isEqualTo("Bearer runtime-token");
    JsonNode body = Json.mapper().readTree(poll.body());
    assertThat(body.get("runtimeId").asString()).isEqualTo("runtime 1");
    assertThat(body.get("contractVersion").asInt()).isEqualTo(2);
    assertThat(body.get("url").asString()).isEqualTo("http://runtime-1:8091");
    assertThat(body.get("kinds").get(0).get("kind").asString()).isEqualTo("camel-route");
    assertThat(body.get("kinds").get(0).get("requiredPermission").asString()).isEqualTo("platform:ingestion:ingestion:route");
  }

  @Test
  void aPollAnswerWithMembersTheRuntimeDoesNotKnowIsStillRead() throws Exception {
    answer = "{\"$schema\":\"http://platform/PollResponse.json\",\"asOf\":\"2026-10-24T10:00:00Z\",\"leaseFor\":\"PT3M\",\"ingestions\":[],\"tests\":[],\"later\":1}";

    assertThat(api.poll().leaseFor()).isEqualTo(Duration.ofMinutes(3));
  }

  @Test
  void aPollOrAReportThatFailsIsSentOnce() {
    statuses.addAll(List.of(503, 503));
    assertThatThrownBy(() -> api.poll()).isInstanceOf(IOException.class).hasMessageContaining("503");
    assertThatThrownBy(() -> api.report(new RuntimeReport("runtime 1", List.of(), List.of())))
        .isInstanceOf(IOException.class)
        .hasMessageContaining("503");
    assertThat(seen).extracting(Seen::target).containsExactly("/api/ingestion-runtime/v1/poll", "/api/ingestion-runtime/v1/report");
  }

  @Test
  void aContractVersionThePlatformDoesNotServeSaysSoWithItsReason() {
    statuses.add(426);
    answer = "{\"error\":\"CONTRACT_VERSION_UNSUPPORTED\",\"message\":\"contract version 2 is not supported; the platform serves 3 to 4\"}";

    assertThatThrownBy(() -> api.poll())
        .isInstanceOf(RejectedException.class)
        .hasMessageContaining("426")
        .hasMessageContaining("CONTRACT_VERSION_UNSUPPORTED")
        .hasMessageContaining("serves 3 to 4");
  }

  @Test
  void aReportThePlatformFindsInvalidIsRejectedAndOneItMightTakeLaterIsNot() {
    statuses.addAll(List.of(422, 401, 429));
    answer = "{\"error\":\"REPORT_WINDOW_INVALID\",\"message\":\"no\"}";
    RuntimeReport report = new RuntimeReport("runtime 1", List.of(), List.of());

    assertThatThrownBy(() -> api.report(report)).isInstanceOf(RejectedException.class);
    assertThatThrownBy(() -> api.report(report)).isNotInstanceOf(RejectedException.class).isInstanceOf(IOException.class);
    assertThatThrownBy(() -> api.report(report)).isNotInstanceOf(RejectedException.class).isInstanceOf(IOException.class);
  }

  @Test
  void aStoredValueIsReadAsThisRuntimeAndMissingOneIsEmpty() throws Exception {
    answer = "{\"value\":\"42\"}";
    assertThat(api.get("ing_1", "watermark")).contains("42");
    assertThat(seen.getFirst().target()).isEqualTo("/api/ingestion-runtime/v1/state/ing_1/watermark?runtimeId=runtime%201");

    statuses.add(404);
    answer = "{\"error\":\"STATE_NOT_FOUND\",\"message\":\"nothing is stored under that key\"}";
    assertThat(api.get("ing_1", "watermark")).isEmpty();

    // An ingestion the platform does not know is not "no value".
    statuses.add(404);
    answer = "{\"error\":\"INGESTION_NOT_FOUND\",\"message\":\"no\"}";
    assertThatThrownBy(() -> api.get("ing_x", "watermark")).isInstanceOf(IOException.class).hasMessageContaining("INGESTION_NOT_FOUND");
  }

  @Test
  void aValueIsStoredAsThisRuntimeAndALostLeaseIsAFailure() throws Exception {
    api.put("ing/1", "water mark", "2026-10-13T10:00:00Z");

    Seen put = seen.getFirst();
    assertThat(put.method()).isEqualTo("PUT");
    assertThat(put.target()).isEqualTo("/api/ingestion-runtime/v1/state/ing%2F1/water%20mark?runtimeId=runtime%201");
    assertThat(Json.mapper().readTree(put.body()).get("value").asString()).isEqualTo("2026-10-13T10:00:00Z");

    statuses.add(409);
    answer = "{\"error\":\"LEASE_NOT_HELD\",\"message\":\"another runtime holds it\"}";
    assertThatThrownBy(() -> api.put("ing_1", "watermark", "5")).isInstanceOf(IOException.class).hasMessageContaining("LEASE_NOT_HELD");
  }

  @Test
  void storingAndReadingAreTriedAgainWhenThePlatformIsBrieflyUnavailable() throws Exception {
    statuses.add(503);
    api.put("ing_1", "watermark", "5");
    assertThat(seen).hasSize(2);
  }

  @Test
  void recordsAreSentToTheIngestionsOwnJobsCallCarryingOnlyTheRecords() throws Exception {
    answer = "{\"created\":2,\"results\":[{\"index\":0,\"id\":\"djb_1\"},{\"index\":1,\"id\":\"djb_2\"}]}";

    api.create(ingestion, List.of(new JobRecord("{\"id\":1}", "application/json", "g1", null, "g1", null), new JobRecord("{\"id\":2}", "application/json", "g1", null, "g1", null)));

    Seen call = seen.getFirst();
    assertThat(call.method()).isEqualTo("POST");
    assertThat(call.target()).isEqualTo("/api/ingestion-runtime/v1/ingestions/ing_1/jobs?runtimeId=runtime%201");
    assertThat(call.authorization()).isEqualTo("Bearer runtime-token");
    JsonNode body = Json.mapper().readTree(call.body());
    assertThat(body.propertyNames()).containsExactly("records");
    JsonNode first = body.get("records").get(0);
    // Nothing that names the target, the client or a signer: the platform refuses a field for any.
    assertThat(first.propertyNames()).containsExactlyInAnyOrder("payload", "payloadContentType", "messageGroup", "correlationId");
    assertThat(first.get("payload").asString()).isEqualTo("{\"id\":1}");
  }

  @Test
  void aRecordTheCallRefusesOrAnAnswerThatDoesNotCoverEveryRecordIsAFailureAndIsNotSentAgain() {
    List<JobRecord> two = List.of(new JobRecord("a", "text/plain", null, null, null, null), new JobRecord("b", "text/plain", null, null, null, null));

    answer = "{\"created\":1,\"results\":[{\"index\":0,\"id\":\"djb_1\"}]}";
    assertThatThrownBy(() -> api.create(ingestion, two)).isInstanceOf(IOException.class);

    answer = "{\"created\":1,\"results\":[{\"index\":0,\"id\":\"djb_1\"},{\"index\":1,\"error\":\"payload is required\"}]}";
    assertThatThrownBy(() -> api.create(ingestion, two)).isInstanceOf(IOException.class).hasMessageContaining("record 1").hasMessageContaining("payload is required");

    statuses.add(409);
    answer = "{\"error\":\"LEASE_NOT_HELD\",\"message\":\"no\"}";
    assertThatThrownBy(() -> api.create(ingestion, two)).hasMessageContaining("LEASE_NOT_HELD");
    assertThat(seen).hasSize(3);
  }

  @Test
  void aSecretIsRevealedForTheIngestionAndOneItDoesNotNameIsEmpty() throws Exception {
    answer = "{\"value\":\"s3cret\",\"updatedAt\":\"2026-10-24T09:00:00Z\"}";
    assertThat(api.reveal(ingestion, "sftp-password")).contains("s3cret");
    assertThat(seen.getFirst().target()).isEqualTo("/api/ingestion-runtime/v1/ingestions/ing_1/secrets/sftp-password?runtimeId=runtime%201");

    statuses.add(404);
    answer = "{\"error\":\"SECRET_NOT_FOUND\",\"message\":\"the ingestion names no secret other\"}";
    assertThat(api.reveal(ingestion, "other")).isEmpty();
  }

  @Test
  void aPlatformThatIsNotThereIsAFailure() {
    platform.stop(0);
    assertThatThrownBy(() -> api.poll()).isInstanceOf(IOException.class);
  }
}
