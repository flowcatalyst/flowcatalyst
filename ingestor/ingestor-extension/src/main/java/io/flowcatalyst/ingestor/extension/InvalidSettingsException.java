package io.flowcatalyst.ingestor.extension;

/** Settings an extension cannot build a source from. The message is shown to operators and holds no secret. */
public final class InvalidSettingsException extends RuntimeException {

  private static final long serialVersionUID = 1L;

  /**
   * Settings that cannot be used.
   *
   * @param message why, for an operator
   */
  public InvalidSettingsException(String message) {
    super(message);
  }
}
