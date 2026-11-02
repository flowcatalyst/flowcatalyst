package io.flowcatalyst.ingestor.runtime;

import java.io.IOException;
import java.time.Duration;
import org.jspecify.annotations.Nullable;

/**
 * A call to the platform failed for the platform's sake and not the request's: no answer (connect
 * failed, timed out), a 5xx, 429, 408, or 401 or 403 after the token was renewed. Sending the same
 * request later may well work; every ingestion's is equally likely to fail meanwhile, which is what
 * {@link HoldingJobSink} holds them for (docs/ingestor-failure-pacing.md, R2).
 */
final class PlatformUnavailableException extends IOException {

  private static final long serialVersionUID = 1L;

  private final int status;
  private final @Nullable Duration retryAfter;

  /**
   * Creates the exception.
   *
   * @param message what failed, without a secret
   * @param status the HTTP status, or 0 when there was no answer
   * @param retryAfter how long the platform asked to wait ({@code Retry-After}), or null
   * @param cause the failure under it, or null
   */
  PlatformUnavailableException(String message, int status, @Nullable Duration retryAfter, @Nullable Throwable cause) {
    super(message, cause);
    this.status = status;
    this.retryAfter = retryAfter;
  }

  /** The HTTP status, 0 when there was no answer. */
  int status() {
    return status;
  }

  /** How long the platform asked callers to wait, or null. */
  @Nullable Duration retryAfter() {
    return retryAfter;
  }

  /**
   * Whether a status is the platform's failure and not the request's.
   *
   * @param status the HTTP status
   * @return true for 5xx, 429, 408, 401 and 403
   */
  static boolean platformStatus(int status) {
    return status >= 500 || status == 429 || status == 408 || status == 401 || status == 403;
  }
}
