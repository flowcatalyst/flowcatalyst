package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.framework.http.Json;
import java.util.regex.Pattern;
import tools.jackson.core.JacksonException;
import tools.jackson.databind.JsonNode;

/**
 * How many records a split makes of an answer, for "Test connection", without dispatching any. A
 * close count, not the route's: blank CSV lines are skipped as the route skips them, and XML is
 * counted by its opening tags.
 */
final class RecordCount {

  private RecordCount() {}

  /**
   * Counts.
   *
   * @param split the ingestion's split
   * @param body the answer
   * @return the records, or -1 when the answer cannot be split so
   */
  static long of(Split split, String body) {
    return switch (split) {
      case Split.None _ -> body.isBlank() ? 0 : 1;
      case Split.Csv csv -> {
        long lines = body.lines().filter(line -> !line.isBlank()).count();
        yield csv.header() ? Math.max(lines - 1, 0) : lines;
      }
      case Split.Xml xml -> Pattern.compile("<" + Pattern.quote(xml.element()) + "[\\s/>]").matcher(body).results().count();
      case Split.Json json -> {
        try {
          JsonNode found = JsonPaths.at(Json.mapper().readTree(body), json.arrayPath());
          yield found != null && found.isArray() ? found.size() : -1;
        } catch (JacksonException | UnsupportedIngestionException e) {
          yield -1;
        }
      }
    };
  }
}
