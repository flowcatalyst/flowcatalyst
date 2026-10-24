package io.flowcatalyst.ingestor.runtime;

/** The runtime cannot run an ingestion as configured. Its route is reported as failed. */
public final class UnsupportedIngestionException extends RuntimeException {

  private static final long serialVersionUID = 1L;

  /**
   * Creates the exception.
   *
   * @param message what is not supported
   */
  public UnsupportedIngestionException(String message) {
    super(message);
  }
}
