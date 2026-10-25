package io.flowcatalyst.ingestor.runtime;

/**
 * What the runtime does when the JVM has run out of memory where it catches everything: end the
 * process, so that whatever runs it (the container, fcdev) starts a healthy one, instead of a loop
 * or a source silently ending. The image also runs with {@code -XX:+ExitOnOutOfMemoryError}.
 */
final class Fatal {

  /** The exit status. */
  static final int OUT_OF_MEMORY = 3;

  private Fatal() {}

  /**
   * Ends the process at once: shutdown hooks are not run, as they may need memory there is none of.
   *
   * @param where what was running
   * @param e the error
   */
  static void outOfMemory(String where, OutOfMemoryError e) {
    try {
      System.err.println("ingestor: out of memory in " + where + "; exiting");
    } finally {
      Runtime.getRuntime().halt(OUT_OF_MEMORY);
    }
  }
}
