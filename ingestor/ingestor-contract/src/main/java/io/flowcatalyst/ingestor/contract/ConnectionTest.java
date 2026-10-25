package io.flowcatalyst.ingestor.contract;

import com.fasterxml.jackson.annotation.JsonInclude;
import java.util.ArrayList;
import java.util.List;
import org.jspecify.annotations.Nullable;

/**
 * What a runtime found when it tried an ingestion's source once, for "Test connection": it logs in
 * and asks as the route would, with the runtime's secrets and from its network, and dispatches
 * nothing. No secret or token is in it.
 *
 * @param id the test request it answers
 * @param outcome how it went
 * @param steps what happened, step by step, for a person: at most {@value #MAX_STEPS}, each cut to
 *     {@value #MAX_STEP} characters
 * @param records how many records the split makes of the answer, when it answered
 * @param excerpt the start of the answer, only when the ingestion keeps failed payloads
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
public record ConnectionTest(String id, Outcome outcome, List<String> steps, @Nullable Long records, @Nullable String excerpt) {

  /** The most steps the platform takes in one answer. */
  public static final int MAX_STEPS = 50;

  /** The longest step the platform takes, in characters. */
  public static final int MAX_STEP = 500;

  /**
   * Copies the list, within what the platform takes: it refuses the whole report otherwise. A NUL
   * character, which the platform cannot store, is taken out of the steps and the excerpt.
   */
  public ConnectionTest {
    List<String> kept = new ArrayList<>();
    for (String step : steps.subList(0, Math.min(steps.size(), MAX_STEPS))) {
      String clean = Text.withoutNul(step);
      kept.add(clean.length() > MAX_STEP ? Text.cut(clean, MAX_STEP - 3) + "..." : clean);
    }
    steps = List.copyOf(kept);
    excerpt = excerpt == null ? null : Text.withoutNul(excerpt);
  }

  /** How a test went. */
  public enum Outcome {
    /** Logged in, and the answer has results. */
    RESULTS,
    /** Logged in, and the answer has none: the poll would wait for its idle delay. */
    NO_RESULTS,
    /** The login was refused, also with a fresh token. */
    REFUSED,
    /** The source could not be asked, or answered with an error, or a secret is not set. */
    FAILED,
    /** This kind of source cannot be tried yet. */
    NOT_SUPPORTED
  }
}
