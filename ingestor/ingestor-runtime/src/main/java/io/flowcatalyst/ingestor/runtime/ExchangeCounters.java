package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.ExchangeWindow;
import io.flowcatalyst.ingestor.contract.RuntimeReport.Failure;
import java.time.Instant;
import java.time.InstantSource;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.regex.Pattern;
import org.jspecify.annotations.Nullable;

/**
 * Counts exchanges per ingestion between two reports, and keeps what went
 * wrong: the first {@value #FAILURES_PER_INGESTION} failures of each ingestion in a window (the
 * counts still count every one).
 */
final class ExchangeCounters {

  /** Failures kept per ingestion and window; the rest are counted only. */
  static final int FAILURES_PER_INGESTION = 20;

  /** The longest excerpt kept, in characters. */
  static final int EXCERPT = 2048;

  private static final int MESSAGE = 2000;

  // Credentials that a component's message can carry: a URI's user info, a parameter.
  private static final Pattern USER_INFO = Pattern.compile("//[^/@\\s]+@");
  private static final Pattern SECRET_PARAMETER =
      Pattern.compile("(?i)(password|passwd|secret|token|access_token|apikey|api_key|authorization)=[^&\\s]+");

  private static final class Tally {
    long completed;
    long failed;
    long records;
    long dispatchJobs;
    final List<Failure> failures = new ArrayList<>();
  }

  private final Object lock = new Object();
  private final InstantSource clock;
  private final Map<String, Tally> tallies = new LinkedHashMap<>();
  private Instant windowStart;

  ExchangeCounters(InstantSource clock) {
    this.clock = clock;
    this.windowStart = clock.instant();
  }

  void exchangeCompleted(Ingestion ingestion) {
    synchronized (lock) {
      tally(ingestion).completed++;
    }
  }

  void exchangeFailed(Ingestion ingestion) {
    synchronized (lock) {
      tally(ingestion).failed++;
    }
  }

  /** Records one batch of dispatch jobs created from as many records. */
  void dispatched(Ingestion ingestion, int count) {
    synchronized (lock) {
      Tally t = tally(ingestion);
      t.records += count;
      t.dispatchJobs += count;
    }
  }

  /**
   * Keeps what went wrong, for the next report. Credentials a message may carry are blanked; the
   * excerpt is dropped unless the ingestion keeps failed payloads.
   *
   * @param ingestion the ingestion
   * @param kind what went wrong
   * @param message why
   * @param afterRecords how many records were read before it failed, or null
   * @param excerpt the payload, record or answer it failed on, or null
   */
  void failure(Ingestion ingestion, Failure.Kind kind, @Nullable String message, @Nullable Long afterRecords, @Nullable String excerpt) {
    Failure failure =
        new Failure(
            ingestion.id(),
            ingestion.version(),
            clock.instant(),
            kind,
            clean(message),
            afterRecords,
            ingestion.keepFailedPayloads() && excerpt != null ? excerpt(excerpt) : null);
    synchronized (lock) {
      List<Failure> kept = tally(ingestion).failures;
      if (kept.size() < FAILURES_PER_INGESTION) {
        kept.add(failure);
      }
    }
  }

  /**
   * Returns the counts since the last call and starts a new window.
   *
   * @param now the end of the window
   * @return one window per ingestion that counted anything, and the failures kept
   */
  Drained drain(Instant now) {
    synchronized (lock) {
      List<ExchangeWindow> windows = new ArrayList<>();
      List<Failure> failures = new ArrayList<>();
      for (Map.Entry<String, Tally> e : tallies.entrySet()) {
        Tally t = e.getValue();
        if (t.completed + t.failed + t.records + t.dispatchJobs > 0) {
          windows.add(
              new ExchangeWindow(
                  e.getKey(), windowStart, now, t.completed, t.failed, t.records, t.dispatchJobs));
        }
        failures.addAll(t.failures);
      }
      tallies.clear();
      windowStart = now;
      return new Drained(windows, failures);
    }
  }

  /**
   * What one drain took.
   *
   * @param windows the counts
   * @param failures what went wrong
   */
  record Drained(List<ExchangeWindow> windows, List<Failure> failures) {}

  /** A message for the platform: one line, without the credentials a component may put in it. */
  static String clean(@Nullable String message) {
    if (message == null || message.isBlank()) {
      return "no reason was given";
    }
    String text = USER_INFO.matcher(message).replaceAll("//***@");
    text = SECRET_PARAMETER.matcher(text).replaceAll("$1=***");
    // The platform takes a message of at most MESSAGE characters, the ellipsis included.
    return text.length() > MESSAGE ? text.substring(0, MESSAGE - 3) + "..." : text;
  }

  private static String excerpt(String text) {
    return text.length() > EXCERPT ? text.substring(0, EXCERPT) : text;
  }

  private Tally tally(Ingestion ingestion) {
    return tallies.computeIfAbsent(ingestion.id(), id -> new Tally());
  }
}
