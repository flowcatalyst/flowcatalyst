package io.flowcatalyst.ingestor.runtime;

import java.io.IOException;

/**
 * The platform answered a call with a client error that sending it again will not change, such as a
 * report it finds invalid. A report that is rejected is dropped, not kept for the next round: kept,
 * it would be refused every round and nothing after it would ever be reported.
 */
public final class RejectedException extends IOException {

  private static final long serialVersionUID = 1L;

  private final int status;

  /**
   * Creates the exception.
   *
   * @param message what the platform said, without a secret
   * @param status the HTTP status
   */
  public RejectedException(String message, int status) {
    super(message);
    this.status = status;
  }

  /**
   * The status.
   *
   * @return the HTTP status
   */
  public int status() {
    return status;
  }
}
