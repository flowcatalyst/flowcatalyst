package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.framework.httpclient.HttpCapture;
import java.time.Duration;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import org.jspecify.annotations.Nullable;

/**
 * The record of one outbound HTTP attempt, as an ingestion's {@code http} record and a function's
 * {@code outbound} record both carry it: one shape, so one rendering. Everything in it goes through
 * the {@link TraceRedactor} given.
 */
public final class HttpRecords {

  private HttpRecords() {}

  /**
   * An attempt as a record.
   *
   * @param level {@code info}, {@code warn} (status 400 or more) or {@code error} (a transport failure)
   * @param message {@code METHOD scheme://host[:port]/path -> status (N ms)}, or {@code ... failed: KIND (N ms)}
   * @param data {@code client}, {@code request}, {@code response} (null for a transport failure),
   *     {@code durationMs}, {@code attempt} and, for a failure, {@code failure}
   */
  public record Built(String level, String message, Map<String, Object> data) {}

  /**
   * Builds the record of an attempt.
   *
   * @param call the attempt
   * @param redactor what may leave
   * @param bodyLimit the most bytes of each body to keep
   * @return the record's parts
   */
  public static Built build(HttpCapture.Exchange call, TraceRedactor redactor, int bodyLimit) {
    Map<String, Object> request = new LinkedHashMap<>();
    request.put("method", call.method().name());
    request.put("url", redactor.url(call.url()));
    request.put("headers", redactor.headers(call.requestHeaders()));
    request.putAll(redactor.body(call.requestBody(), contentType(call.requestHeaders()), bodyLimit));
    Map<String, Object> response = null;
    if (call.failure() == null || call.status() != 0) {
      response = new LinkedHashMap<>();
      response.put("status", call.status());
      response.put("headers", redactor.headers(call.responseHeaders()));
      response.putAll(redactor.body(call.responseBody(), contentType(call.responseHeaders()), bodyLimit));
    }
    long millis = Duration.ofNanos(call.nanos()).toMillis();
    Map<String, Object> data = new LinkedHashMap<>();
    data.put("client", call.service());
    data.put("request", request);
    data.put("response", response);
    data.put("durationMs", millis);
    data.put("attempt", call.attempt());
    String failure = call.failure() == null ? null : call.failure().name();
    if (failure != null) {
      data.put("failure", failure);
    }
    String where = redactor.address(call.url());
    String message = call.method().name() + " " + where + (failure != null ? " failed: " + failure : " -> " + call.status()) + " (" + millis + " ms)";
    String level = failure != null ? "error" : call.status() >= 400 ? "warn" : "info";
    return new Built(level, message, data);
  }

  /**
   * The {@code Content-Type} of a header map.
   *
   * @param headers names to values
   * @return the first value, or null
   */
  public static @Nullable String contentType(Map<String, List<String>> headers) {
    for (Map.Entry<String, List<String>> e : headers.entrySet()) {
      if (e.getKey().equalsIgnoreCase("content-type") && !e.getValue().isEmpty()) {
        return e.getValue().getFirst();
      }
    }
    return null;
  }
}
