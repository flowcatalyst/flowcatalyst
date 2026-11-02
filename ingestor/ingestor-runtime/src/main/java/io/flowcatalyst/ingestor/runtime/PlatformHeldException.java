package io.flowcatalyst.ingestor.runtime;

import java.io.IOException;

/**
 * A submit was not sent because the platform is not taking jobs and every ingestion is held until a
 * time ({@link HoldingJobSink}). The source keeps the payload; nothing is wrong with it.
 */
final class PlatformHeldException extends IOException {

  private static final long serialVersionUID = 1L;

  PlatformHeldException(String message) {
    super(message);
  }
}
