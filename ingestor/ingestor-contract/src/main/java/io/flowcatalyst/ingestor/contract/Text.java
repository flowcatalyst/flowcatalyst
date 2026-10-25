package io.flowcatalyst.ingestor.contract;

/** Text the platform stores: it cannot store a NUL character, and counts length in characters. */
public final class Text {

  private Text() {}

  /**
   * The text without NUL characters, which PostgreSQL text cannot hold: a report carrying one would
   * fail every time it is sent.
   *
   * @param text the text
   * @return the text without NUL
   */
  public static String withoutNul(String text) {
    return text.indexOf('\u0000') < 0 ? text : text.replace("\u0000", "");
  }

  /**
   * The start of the text, at most {@code max} characters, never splitting a surrogate pair.
   *
   * @param text the text
   * @param max the most characters kept
   * @return the start
   */
  public static String cut(String text, int max) {
    if (text.length() <= max) {
      return text;
    }
    int end = max;
    if (end > 0 && Character.isHighSurrogate(text.charAt(end - 1))) {
      end--;
    }
    return text.substring(0, end);
  }
}
