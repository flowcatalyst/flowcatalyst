package io.flowcatalyst.ingestor.runtime;

import java.io.IOException;

/**
 * The platform refused a poll or a lease release with {@code 409 RUNTIME_ID_IN_USE}: another live
 * process polls under this runtime's id. Nothing was renewed or released. For the leases it is the
 * same as a platform that cannot be reached: they run out, and the single-owner routes are stopped
 * before they do; the runtime keeps polling, and takes over once the other process is gone.
 */
final class RuntimeIdInUseException extends IOException {

  private static final long serialVersionUID = 1L;

  /** The error code of the platform's answer. */
  static final String CODE = "RUNTIME_ID_IN_USE";

  /**
   * Creates the exception.
   *
   * @param message what the platform said
   */
  RuntimeIdInUseException(String message) {
    super(message);
  }
}
