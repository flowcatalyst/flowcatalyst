package io.flowcatalyst.ingestor.runtime;

import java.util.regex.Matcher;
import java.util.regex.Pattern;
import org.jspecify.annotations.Nullable;
import tools.jackson.databind.JsonNode;

/**
 * The paths an API poll names in an answer: {@code $}, then {@code .field} and {@code [n]} steps; a
 * negative index counts from the end, so {@code $.items[-1].id} is the last item's id. Nothing else
 * of JSONPath (no wildcards, filters or recursion).
 */
final class JsonPaths {

  /** A path this class reads. The platform stores the same shape unchecked. */
  static final Pattern PATH = Pattern.compile("\\$(\\.[A-Za-z0-9_-]+|\\[-?\\d{1,6}])*");

  private static final Pattern STEP = Pattern.compile("\\.([A-Za-z0-9_-]+)|\\[(-?\\d{1,6})]");

  private JsonPaths() {}

  /**
   * The node a path names.
   *
   * @param root the answer
   * @param path the path
   * @return the node, or null when the answer has nothing there
   * @throws UnsupportedIngestionException when the path is not one this class reads
   */
  static @Nullable JsonNode at(JsonNode root, String path) {
    if (!PATH.matcher(path).matches()) {
      throw new UnsupportedIngestionException("the path " + path + " is not $ followed by .field or [n] steps");
    }
    JsonNode node = root;
    Matcher step = STEP.matcher(path.substring(1));
    while (step.find()) {
      if (step.group(1) != null) {
        node = node.isObject() ? node.get(step.group(1)) : null;
      } else {
        int index = Integer.parseInt(step.group(2));
        if (!node.isArray()) {
          return null;
        }
        int at = index < 0 ? node.size() + index : index;
        node = at >= 0 && at < node.size() ? node.get(at) : null;
      }
      if (node == null || node.isMissingNode()) {
        return null;
      }
    }
    return node;
  }

  /** Whether a node counts as results: a non-empty array, or a value that is not null, false or empty. */
  static boolean present(@Nullable JsonNode node) {
    if (node == null || node.isNull() || node.isMissingNode()) {
      return false;
    }
    if (node.isArray() || node.isObject()) {
      return !node.isEmpty();
    }
    if (node.isBoolean()) {
      return node.asBoolean();
    }
    return !node.asString().isEmpty();
  }

  /** A node's text, for a cursor or a token: a string as it is, anything else as JSON. */
  static @Nullable String text(@Nullable JsonNode node) {
    if (node == null || node.isNull() || node.isMissingNode()) {
      return null;
    }
    String text = node.isValueNode() ? node.asString() : node.toString();
    return text.isEmpty() ? null : text;
  }
}
