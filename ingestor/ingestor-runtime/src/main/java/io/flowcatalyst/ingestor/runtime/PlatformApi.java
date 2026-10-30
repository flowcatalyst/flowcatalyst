package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.framework.http.Json;
import io.flowcatalyst.framework.httpclient.ClientResponse;
import io.flowcatalyst.framework.httpclient.RequestBuilder;
import io.flowcatalyst.framework.httpclient.ServiceClient;
import io.flowcatalyst.ingestor.contract.ConnectionTest;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.JobRecord;
import io.flowcatalyst.ingestor.contract.JobResult;
import io.flowcatalyst.ingestor.contract.JobsAnswer;
import io.flowcatalyst.ingestor.contract.PollAnswer;
import io.flowcatalyst.ingestor.contract.RuntimeKind;
import io.flowcatalyst.ingestor.contract.RuntimePoll;
import io.flowcatalyst.ingestor.contract.RuntimeReport;
import io.flowcatalyst.ingestor.contract.SecretValue;
import io.flowcatalyst.ingestor.contract.StateValue;
import io.flowcatalyst.ingestor.contract.TestRequest;
import io.flowcatalyst.ingestor.contract.TraceAccepted;
import io.flowcatalyst.ingestor.contract.TraceBatch;
import io.flowcatalyst.ingestor.contract.TraceRecord;
import io.flowcatalyst.ingestor.contract.TraceWindow;
import java.io.IOException;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.regex.Pattern;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import tools.jackson.core.JacksonException;
import tools.jackson.databind.DeserializationFeature;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;

/**
 * The platform's ingestion runtime API ({@code /api/ingestion-runtime/v1}) over HTTP, as the runtime's
 * service account. The client retries what is safe to repeat (reading and storing a value, revealing
 * a secret); a poll, a report and a jobs request are sent once: the next round is a poll's and a
 * report's retry, and the source keeps what a jobs request did not make.
 */
final class PlatformApi implements RuntimeApi, StateStore, JobSink, SecretSource, TraceUplink {

  private static final Logger LOG = LoggerFactory.getLogger(PlatformApi.class);

  private static final String BASE = "/api/ingestion-runtime/v1";
  private static final String POLL = BASE + "/poll";
  private static final String REPORT = BASE + "/report";
  private static final String TRACES = BASE + "/traces";
  private static final String STATE = BASE + "/state/{ingestion}/{key}";
  private static final String JOBS = BASE + "/ingestions/{ingestion}/jobs";
  private static final String SECRET = BASE + "/ingestions/{ingestion}/secrets/{name}";

  /**
   * Reads the platform's answers. Not the framework's strict mapper: the platform adds a {@code $schema}
   * member to every answer, and the contract grows by additive fields, neither of which may stop a poll.
   */
  private static final ObjectMapper ANSWERS = Json.mapper().rebuild().disable(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES).build();

  /** The longest part of the platform's message repeated in an error. */
  private static final int MESSAGE = 300;

  /** A kind's name as it may be repeated in a reason: nothing else from a definition is. */
  private static final Pattern KIND_NAME = Pattern.compile("[a-z][a-z0-9-]{0,39}");

  /** The reason an ingestion or test this runtime cannot read is given. */
  static final String NOT_SUPPORTED = "definition not supported by this runtime version";

  /** The answer's outline: each ingestion and test is read on its own, so one this runtime cannot read fails alone. */
  private record Outline(
      Instant asOf, Duration leaseFor, @Nullable List<JsonNode> ingestions, @Nullable List<JsonNode> tests, @Nullable List<JsonNode> traces) {}

  private final ServiceClient client;
  private final String runtimeId;
  private final @Nullable String url;
  private final List<RuntimeKind> kinds;
  private final Runnable forgetToken;

  /**
   * Creates the client.
   *
   * @param client the platform, with the runtime's service-account token
   * @param runtimeId names this runtime
   * @param url where this runtime can be reached (informational), or null
   * @param kinds the kinds this runtime adds to the built-in ones
   */
  PlatformApi(ServiceClient client, String runtimeId, @Nullable String url, List<RuntimeKind> kinds) {
    this(client, runtimeId, url, kinds, () -> {});
  }

  /**
   * Creates the client.
   *
   * @param client the platform, with the runtime's service-account token
   * @param runtimeId names this runtime
   * @param url where this runtime can be reached (informational), or null
   * @param kinds the kinds this runtime adds to the built-in ones
   * @param forgetToken drops the token held, so the next call fetches a new one: run after a 401 or
   *     403 (the client itself repeats a 401 once, with a new token)
   */
  PlatformApi(ServiceClient client, String runtimeId, @Nullable String url, List<RuntimeKind> kinds, Runnable forgetToken) {
    this.client = client;
    this.runtimeId = runtimeId;
    this.url = url;
    this.kinds = List.copyOf(kinds);
    this.forgetToken = forgetToken;
  }

  @Override
  public Polled poll(Duration timeout) throws IOException, InterruptedException {
    ClientResponse response =
        succeeded(client.post(POLL).timeout(timeout).json(new RuntimePoll(runtimeId, kinds, url)), "POST " + POLL);
    Outline outline = read(response, Outline.class, "the platform's list of ingestions cannot be read");
    if (outline.asOf() == null || outline.leaseFor() == null) {
      throw new IOException("the platform's list of ingestions cannot be read");
    }
    List<Ingestion> ingestions = new ArrayList<>();
    List<Unreadable> unreadable = new ArrayList<>();
    for (JsonNode node : outline.ingestions() == null ? List.<JsonNode>of() : outline.ingestions()) {
      try {
        ingestions.add(ANSWERS.treeToValue(node, Ingestion.class));
      } catch (JacksonException | IllegalArgumentException e) {
        String id = node.path("id").asString("");
        if (id.isEmpty()) {
          LOG.warn("the platform listed an ingestion without an id; it is skipped");
          continue;
        }
        // Not e's message: it may quote the definition.
        unreadable.add(new Unreadable(id, node.path("version").asLong(0), reason(node)));
      }
    }
    List<TestRequest> tests = new ArrayList<>();
    List<ConnectionTest> unreadableTests = new ArrayList<>();
    for (JsonNode node : outline.tests() == null ? List.<JsonNode>of() : outline.tests()) {
      try {
        tests.add(ANSWERS.treeToValue(node, TestRequest.class));
      } catch (JacksonException | IllegalArgumentException e) {
        String id = node.path("id").asString("");
        if (!id.isEmpty()) {
          unreadableTests.add(
              new ConnectionTest(id, ConnectionTest.Outcome.FAILED, List.of("The " + reason(node.path("ingestion")) + "."), null, null));
        }
      }
    }
    List<TraceWindow> traces = new ArrayList<>();
    for (JsonNode node : outline.traces() == null ? List.<JsonNode>of() : outline.traces()) {
      try {
        TraceWindow window = ANSWERS.treeToValue(node, TraceWindow.class);
        if (window.ingestionId() != null && !window.ingestionId().isBlank() && window.windowId() != null && !window.windowId().isBlank() && window.hardStop() != null) {
          traces.add(window);
        }
      } catch (JacksonException | IllegalArgumentException e) {
        // A window this runtime cannot read is no window: tracing is never worth failing a poll for.
        LOG.debug("a trace window of the platform's answer cannot be read; it is skipped");
      }
    }
    return new Polled(new PollAnswer(outline.asOf(), outline.leaseFor(), ingestions, tests, traces), unreadable, unreadableTests);
  }

  /** Why a definition cannot be read, naming its source kind when it is one this runtime does not know. */
  private static String reason(JsonNode ingestion) {
    JsonNode source = ingestion.path("source");
    String kind = source.path("kind").asString("");
    if (kind.equals("custom")) {
      kind = source.path("type").asString("");
    }
    if (KIND_NAME.matcher(kind).matches() && !RuntimePoll.BUILTIN_KINDS.contains(kind) && !kind.equals("custom")) {
      return NOT_SUPPORTED + " (source kind '" + kind + "')";
    }
    return NOT_SUPPORTED;
  }

  @Override
  public void report(RuntimeReport report, Duration timeout) throws IOException, InterruptedException {
    succeeded(client.post(REPORT).timeout(timeout).json(report), "POST " + REPORT);
  }

  /**
   * Sends a batch of trace records, once: the shipper decides about trying again from the status. A
   * 401 or 403 drops the token held, so the next call fetches a new one.
   *
   * @param records the records, at most {@link TraceBatch#MAX_RECORDS}
   * @param timeout how long to wait for the answer
   * @return the answer, whatever its status
   * @throws IOException when there is no answer
   * @throws InterruptedException when interrupted while waiting
   */
  @Override
  public Result sendTraces(List<TraceRecord> records, Duration timeout) throws IOException, InterruptedException {
    ClientResponse response = ServiceCalls.send(client.post(TRACES).timeout(timeout).json(new TraceBatch(runtimeId, records)));
    int status = response.status();
    if (status == 401 || status == 403) {
      forgetToken.run();
    }
    if (!response.isSuccess()) {
      return new Result(status, 0, 0);
    }
    try {
      TraceAccepted accepted = ANSWERS.readValue(response.bodyBytes(), TraceAccepted.class);
      return new Result(status, accepted.accepted(), accepted.dropped());
    } catch (JacksonException e) {
      // The batch was taken; only its counts are unreadable.
      return new Result(status, records.size(), 0);
    }
  }

  @Override
  public Optional<String> get(String ingestionId, String key) throws IOException, InterruptedException {
    ClientResponse response =
        ServiceCalls.send(client.get(STATE).pathParam("ingestion", ingestionId).pathParam("key", key).query("runtimeId", runtimeId));
    if (response.status() == 404 && "STATE_NOT_FOUND".equals(code(response))) {
      return Optional.empty();
    }
    return Optional.of(read(failed(response, "GET " + BASE + "/state"), StateValue.class, "the platform's stored value cannot be read").value());
  }

  @Override
  public void put(String ingestionId, String key, String value) throws IOException, InterruptedException {
    succeeded(
        client.put(STATE).pathParam("ingestion", ingestionId).pathParam("key", key).query("runtimeId", runtimeId).json(new StateValue(value)),
        "PUT " + BASE + "/state");
  }

  @Override
  public void create(Ingestion ingestion, List<JobRecord> records) throws IOException, InterruptedException {
    ClientResponse response =
        succeeded(
            client.post(JOBS).pathParam("ingestion", ingestion.id()).query("runtimeId", runtimeId).json(Map.of("records", records)),
            "POST " + BASE + "/ingestions/{id}/jobs");
    JobsAnswer answer = read(response, JobsAnswer.class, "the platform's answer to a jobs request cannot be read");
    if (answer.results().size() != records.size()) {
      throw new IOException("the platform answered for a different number of records than were sent");
    }
    for (JobResult result : answer.results()) {
      if (result.error() != null) {
        // A record the platform finds invalid is refused the same way however often it is sent.
        throw new RejectedException("the platform refused record " + result.index() + ": " + result.error(), response.status());
      }
    }
  }

  @Override
  public Optional<String> reveal(Ingestion ingestion, String name) throws IOException, InterruptedException {
    ClientResponse response =
        ServiceCalls.send(client.get(SECRET).pathParam("ingestion", ingestion.id()).pathParam("name", name).query("runtimeId", runtimeId));
    if (response.status() == 404 && "SECRET_NOT_FOUND".equals(code(response))) {
      return Optional.empty();
    }
    return Optional.of(read(failed(response, "GET " + BASE + "/ingestions/{id}/secrets/{name}"), SecretValue.class, "the platform's secret cannot be read").value());
  }

  private static <T> T read(ClientResponse response, Class<T> type, String otherwise) throws IOException {
    try {
      return ANSWERS.readValue(response.bodyBytes(), type);
    } catch (JacksonException e) {
      // Not e's message: it may quote a secret's value.
      throw new IOException(otherwise);
    }
  }

  private ClientResponse succeeded(RequestBuilder request, String what) throws IOException, InterruptedException {
    return failed(ServiceCalls.send(request), what);
  }

  /**
   * Whether the platform will answer a request the same way however often it is sent: a client
   * error other than 401 and 403 (a token to renew), 408 and 429 (try later), 409 (a lease or a
   * paused target, which change) and 425.
   *
   * @param status the HTTP status
   * @return true for a refusal for good
   */
  static boolean permanent(int status) {
    return status >= 400
        && status < 500
        && status != 401
        && status != 403
        && status != 408
        && status != 409
        && status != 425
        && status != 429;
  }

  /** The response, or the failure its status is: a client error the platform will repeat is a {@link RejectedException}. */
  private ClientResponse failed(ClientResponse response, String what) throws IOException {
    if (response.isSuccess()) {
      return response;
    }
    int status = response.status();
    if (status == 401 || status == 403) {
      // The next call fetches a new token; a role granted since, or a rotated secret, is then in it.
      forgetToken.run();
    }
    String message = what + " answered " + status + said(response);
    throw permanent(status) ? new RejectedException(message, status) : new IOException(message);
  }

  /** What the platform said, from its error envelope: {@code : CODE, the message}. */
  private static String said(ClientResponse response) {
    JsonNode body = envelope(response);
    if (body == null) {
      return "";
    }
    String code = body.path("error").asString("");
    String message = body.path("message").asString("");
    String said = code.isEmpty() ? message : message.isEmpty() ? code : code + ", " + message;
    if (said.isEmpty()) {
      return "";
    }
    return ": " + (said.length() > MESSAGE ? said.substring(0, MESSAGE) + "..." : said);
  }

  private static String code(ClientResponse response) {
    JsonNode body = envelope(response);
    return body == null ? "" : body.path("error").asString("");
  }

  private static @Nullable JsonNode envelope(ClientResponse response) {
    byte[] bytes = response.bodyBytes();
    if (bytes.length == 0) {
      return null;
    }
    try {
      JsonNode body = Json.mapper().readTree(bytes);
      return body.isObject() ? body : null;
    } catch (JacksonException e) {
      return null;
    }
  }
}
