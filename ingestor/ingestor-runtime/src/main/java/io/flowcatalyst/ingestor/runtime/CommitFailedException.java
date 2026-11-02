package io.flowcatalyst.ingestor.runtime;

/**
 * A payload was read and its jobs were made, but the source could not be told so (a row not marked,
 * a watermark or cursor not stored). The payload will be read again; its records' keys keep that
 * from making new jobs where the source gives them. Reported as a {@code SOURCE} failure
 * (docs/ingestor-failure-pacing.md, R4).
 */
final class CommitFailedException extends Exception {

  private static final long serialVersionUID = 1L;

  /**
   * Creates the exception.
   *
   * @param what what could not be done, for example "marked as read"
   * @param cause why
   */
  CommitFailedException(String what, Throwable cause) {
    super(message(what, cause), cause);
  }

  /**
   * The text of a failed commit.
   *
   * @param what what could not be done, for example "moved or deleted"
   * @param cause why
   * @return the text, without a secret as far as the cause holds none
   */
  static String message(String what, Throwable cause) {
    String why = cause.getMessage() == null ? cause.getClass().getSimpleName() : cause.getMessage();
    return "read and sent, but could not be " + what + ": " + why + "; it will be read again";
  }
}
