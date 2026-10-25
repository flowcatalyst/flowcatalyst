package io.flowcatalyst.ingestor.runtime;

/**
 * The platform refused a batch of dispatch jobs for good: a client error it would answer again
 * however often the batch is sent. The payload is let go of as if it could not be read (a file moved
 * to {@code .rejected}, a message not requeued, a watermark or cursor moved past it), so the source
 * keeps moving, and the failure is reported with the category {@code rejected} and the platform's
 * message.
 */
final class PlatformRefusedException extends Exception {

  private static final long serialVersionUID = 1L;

  PlatformRefusedException(RejectedException cause) {
    super("the platform refused the dispatch jobs: " + cause.getMessage(), cause);
  }
}
