package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.framework.http.Json;
import io.flowcatalyst.framework.httpclient.ClientResponse;
import io.flowcatalyst.framework.httpclient.RequestBuilder;
import io.flowcatalyst.framework.httpclient.ServiceClient;
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
import java.io.IOException;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import org.jspecify.annotations.Nullable;
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
final class PlatformApi implements RuntimeApi, StateStore, JobSink, SecretSource {

  private static final String BASE = "/api/ingestion-runtime/v1";
  private static final String POLL = BASE + "/poll";
  private static final String REPORT = BASE + "/report";
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

  private final ServiceClient client;
  private final String runtimeId;
  private final @Nullable String url;
  private final List<RuntimeKind> kinds;

  /**
   * Creates the client.
   *
   * @param client the platform, with the runtime's service-account token
   * @param runtimeId names this runtime
   * @param url where this runtime can be reached (informational), or null
   * @param kinds the kinds this runtime adds to the built-in ones
   */
  PlatformApi(ServiceClient client, String runtimeId, @Nullable String url, List<RuntimeKind> kinds) {
    this.client = client;
    this.runtimeId = runtimeId;
    this.url = url;
    this.kinds = List.copyOf(kinds);
  }

  @Override
  public PollAnswer poll() throws IOException, InterruptedException {
    ClientResponse response = succeeded(client.post(POLL).json(new RuntimePoll(runtimeId, kinds, url)), "POST " + POLL);
    return read(response, PollAnswer.class, "the platform's list of ingestions cannot be read");
  }

  @Override
  public void report(RuntimeReport report) throws IOException, InterruptedException {
    succeeded(client.post(REPORT).json(report), "POST " + REPORT);
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
        throw new IOException("the platform refused record " + result.index() + ": " + result.error());
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

  private static ClientResponse succeeded(RequestBuilder request, String what) throws IOException, InterruptedException {
    return failed(ServiceCalls.send(request), what);
  }

  /** The response, or the failure its status is: a client error the platform will repeat is a {@link RejectedException}. */
  private static ClientResponse failed(ClientResponse response, String what) throws IOException {
    if (response.isSuccess()) {
      return response;
    }
    int status = response.status();
    String message = what + " answered " + status + said(response);
    boolean again = status == 401 || status == 403 || status == 408 || status == 425 || status == 429 || status >= 500;
    throw status >= 400 && status < 500 && !again ? new RejectedException(message, status) : new IOException(message);
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
