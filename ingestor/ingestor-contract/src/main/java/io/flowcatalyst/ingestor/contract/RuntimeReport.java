package io.flowcatalyst.ingestor.contract;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import java.time.Instant;
import java.util.List;
import org.jspecify.annotations.Nullable;

/**
 * What a runtime sends back in one batch: the state of every route it runs, the exchange counts
 * gathered since its last report, what went wrong, and the answers to its tests.
 *
 * @param runtimeId the reporting runtime
 * @param routes one entry per ingestion the runtime was asked to run
 * @param metrics one entry per ingestion that handled an exchange in the window
 * @param failures what went wrong since the last report, a bounded number per ingestion
 * @param tests the answers to the test requests of earlier polls
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
public record RuntimeReport(
    String runtimeId, List<RouteState> routes, List<ExchangeWindow> metrics, List<Failure> failures, List<ConnectionTest> tests) {

  /** The most routes, windows and tests one report carries; the platform refuses more. */
  public static final int MAX_ROUTES = 2000;

  /** The most windows one report carries. */
  public static final int MAX_WINDOWS = 2000;

  /** The most failures one report carries. */
  public static final int MAX_FAILURES = 500;

  /** The most test answers one report carries. */
  public static final int MAX_TESTS = 20;

  /** The largest report body the platform takes, in bytes of its JSON. */
  public static final int MAX_BYTES = 4 * 1024 * 1024;

  /** Copies the lists. */
  public RuntimeReport {
    routes = List.copyOf(routes);
    metrics = List.copyOf(metrics);
    failures = List.copyOf(failures);
    tests = List.copyOf(tests);
  }

  /**
   * A report without failures or tests.
   *
   * @param runtimeId the reporting runtime
   * @param routes one entry per ingestion the runtime was asked to run
   * @param metrics one entry per ingestion that handled an exchange in the window
   */
  public RuntimeReport(String runtimeId, List<RouteState> routes, List<ExchangeWindow> metrics) {
    this(runtimeId, routes, metrics, List.of(), List.of());
  }

  /**
   * The state of one ingestion's route on the runtime.
   *
   * @param ingestionId the ingestion
   * @param version the version the runtime built
   * @param status whether it is running
   * @param error why it failed, when it did
   */
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public record RouteState(String ingestionId, long version, Status status, @Nullable String error) {

    /** Takes out the NUL characters the platform cannot store. */
    public RouteState {
      error = error == null ? null : Text.withoutNul(error);
    }

    /** Whether a route is running. */
    public enum Status {
      /** The route is running. */
      STARTED,
      /** The route is running, but its source keeps failing: the error says why, such as a login refused. */
      FAILING,
      /** The route could not be built or started. */
      FAILED
    }
  }

  /**
   * One thing that went wrong with an ingestion. The message never holds a secret; the excerpt is
   * set only when the ingestion keeps failed payloads.
   *
   * @param ingestionId the ingestion
   * @param version the version that was running
   * @param at when it happened
   * @param kind what went wrong
   * @param message why, for people
   * @param afterRecords how many of the payload's records were read before it failed, when it failed on a payload
   * @param excerpt the start of the record, payload or answer it failed on, or null
   * @param category for a {@code REJECTED} payload, whose fault it was: the payload's (the
   *     default when absent), or the platform's refusal of its jobs, which the platform shows as a
   *     notice to dismiss
   */
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public record Failure(
      String ingestionId,
      long version,
      Instant at,
      Kind kind,
      String message,
      @Nullable Long afterRecords,
      @Nullable String excerpt,
      @Nullable Category category) {

    /**
     * A failure without a category.
     *
     * @param ingestionId the ingestion
     * @param version the version that was running
     * @param at when it happened
     * @param kind what went wrong
     * @param message why, for people
     * @param afterRecords how many records were read before it failed, or null
     * @param excerpt the start of what it failed on, or null
     */
    public Failure(
        String ingestionId, long version, Instant at, Kind kind, String message, @Nullable Long afterRecords, @Nullable String excerpt) {
      this(ingestionId, version, at, kind, message, afterRecords, excerpt, null);
    }

    /** Takes out the NUL characters the platform cannot store. */
    public Failure {
      message = Text.withoutNul(message);
      excerpt = excerpt == null ? null : Text.withoutNul(excerpt);
    }

    /** Whose fault a rejected payload was. */
    public enum Category {
      /** The payload could not be read. */
      @JsonProperty("payload")
      PAYLOAD,
      /** The platform refused its jobs for good (a client error it would repeat); the message is the platform's. */
      @JsonProperty("rejected")
      REJECTED
    }

    /** What went wrong. */
    public enum Kind {
      /** The payload could not be read; the source let go of it. */
      REJECTED,
      /** The platform did not take the jobs; the source keeps the payload and offers it again. */
      NOT_DISPATCHED,
      /** The source could not be read: no connection, an error answer, a poll that failed. */
      SOURCE,
      /** The source refused the credentials, also with a fresh token; it is tried again less often. */
      AUTH
    }
  }

  /**
   * Exchange counts for one ingestion over a window. A window sent again replaces the counts it sent
   * before.
   *
   * @param ingestionId the ingestion
   * @param from the start of the window
   * @param to the end of the window
   * @param completed exchanges that completed
   * @param failed exchanges that failed
   * @param records records the exchanges were split into
   * @param dispatchJobs dispatch jobs created
   */
  public record ExchangeWindow(
      String ingestionId, Instant from, Instant to, long completed, long failed, long records, long dispatchJobs) {}
}
