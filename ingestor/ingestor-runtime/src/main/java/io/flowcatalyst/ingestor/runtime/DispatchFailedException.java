package io.flowcatalyst.ingestor.runtime;

/**
 * The platform did not take a batch of dispatch jobs. Nothing is wrong with the payload: the source
 * keeps it and it is tried again.
 */
final class DispatchFailedException extends Exception {

  private static final long serialVersionUID = 1L;

  DispatchFailedException(Throwable cause) {
    super("the platform did not take the dispatch jobs: " + cause.getMessage(), cause);
  }
}
