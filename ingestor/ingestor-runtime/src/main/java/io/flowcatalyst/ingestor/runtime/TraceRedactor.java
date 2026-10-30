package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.framework.http.Json;
import io.flowcatalyst.framework.httpclient.HttpCapture;
import java.net.URI;
import java.net.URLDecoder;
import java.net.URLEncoder;
import java.nio.ByteBuffer;
import java.nio.charset.CharacterCodingException;
import java.nio.charset.CodingErrorAction;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Base64;
import java.util.Collection;
import java.util.Comparator;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import java.util.function.Predicate;
import java.util.regex.Matcher;
import java.util.regex.Pattern;
import org.jspecify.annotations.Nullable;
import tools.jackson.core.JacksonException;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.node.ArrayNode;
import tools.jackson.databind.node.ObjectNode;

/**
 * What of an HTTP call or a payload may leave the runtime in a trace (the design's section 6): header
 * values, query values and body fields whose names look like credentials are replaced, the user info
 * of a URL is dropped, and the secret values the runtime fetched for the ingestion are replaced
 * wherever they appear verbatim. Everything a trace carries goes through here before a record exists.
 * One redactor is made for a record, so it holds the secret values that are current then; it is never
 * logged and not kept.
 */
final class TraceRedactor {

  /** The replacement for a value that must not be kept. */
  static final String REDACTED = HttpCapture.REDACTED;

  /** The most bytes of a body a record keeps. */
  static final int BODY_LIMIT = 32 * 1024;

  /** Secrets shorter than this are not searched for: a short value would cut a body to pieces. */
  static final int MIN_SECRET = 4;

  /** Past this a body is not parsed: its start is cut and redacted by pattern. */
  private static final int STRUCTURED_LIMIT = 1024 * 1024;

  private static final Set<String> BODY_FIELDS =
      Set.of("client_secret", "access_token", "refresh_token", "id_token", "assertion", "credential", "private_key");

  private static final Set<String> QUERY_FIELDS =
      Set.of("access_token", "code", "token", "sig", "signature", "key", "apikey", "api_key", "password", "secret");

  /** A JSON member whose value is a string or a bare token; also matches a string cut off by the end. */
  private static final Pattern JSON_MEMBER = Pattern.compile("\"([^\"\\\\]{1,100})\"(\\s*:\\s*)(\"(?:[^\"\\\\]++|\\\\.)*+\"?|[^,}\\]\\s]+)");

  private final List<String> secrets;

  /**
   * Creates a redactor.
   *
   * @param secretValues the secret values fetched for the ingestion, plain
   */
  TraceRedactor(Collection<String> secretValues) {
    List<String> found = new ArrayList<>();
    for (String value : secretValues) {
      if (value.length() >= MIN_SECRET) {
        found.add(value);
        String encoded = URLEncoder.encode(value, StandardCharsets.UTF_8);
        if (!encoded.equals(value)) {
          found.add(encoded);
        }
      }
    }
    // Longest first, so a secret that holds another is replaced whole.
    found.sort(Comparator.comparingInt(String::length).reversed());
    this.secrets = List.copyOf(found);
  }

  /** A redactor with no secret values to look for. */
  static TraceRedactor withoutSecrets() {
    return new TraceRedactor(List.of());
  }

  /** Whether a header name, or a query or body field in general, names a credential. */
  static boolean sensitive(String name) {
    return HttpCapture.isSensitiveHeader(name);
  }

  /**
   * Text with the ingestion's secret values replaced.
   *
   * @param text any text a record would carry
   * @return the text
   */
  String text(String text) {
    String out = text;
    for (String secret : secrets) {
      if (out.contains(secret)) {
        out = out.replace(secret, REDACTED);
      }
    }
    return out;
  }

  /**
   * Headers as a record carries them: sensitive names redacted by the framework's rule (applied
   * again here), the secret values taken out of the rest.
   *
   * @param headers names to values
   * @return a copy
   */
  Map<String, List<String>> headers(Map<String, List<String>> headers) {
    Map<String, List<String>> out = new LinkedHashMap<>();
    for (Map.Entry<String, List<String>> e : HttpCapture.redactHeaders(headers).entrySet()) {
      List<String> values = new ArrayList<>();
      for (String value : e.getValue()) {
        values.add(text(value));
      }
      out.put(e.getKey(), List.copyOf(values));
    }
    return out;
  }

  /**
   * A header or property map of single values, as a source's headers are.
   *
   * @param values names to values
   * @return a copy with the values redacted
   */
  Map<String, String> single(Map<String, String> values) {
    Map<String, String> out = new LinkedHashMap<>();
    values.forEach((name, value) -> out.put(name, sensitive(name) ? REDACTED : text(value)));
    return out;
  }

  /**
   * A URL as a record carries it: scheme, host, port, path and the query with its credentials
   * redacted; the user info and the fragment are dropped.
   *
   * @param uri the URL
   * @return the text
   */
  String url(URI uri) {
    String address = address(uri);
    if (uri.getRawQuery() == null) {
      return address;
    }
    return text(address + "?" + pairs(uri.getRawQuery(), TraceRedactor::sensitiveQuery));
  }

  /**
   * A URL without its query: scheme, host, port and path, as a one-line message names a call.
   *
   * @param uri the URL
   * @return the text
   */
  String address(URI uri) {
    StringBuilder out = new StringBuilder();
    if (uri.getScheme() != null) {
      out.append(uri.getScheme()).append("://");
    }
    String host = uri.getHost();
    if (host != null) {
      out.append(host);
      if (uri.getPort() != -1) {
        out.append(':').append(uri.getPort());
      }
    } else if (uri.getRawAuthority() != null) {
      String authority = uri.getRawAuthority();
      out.append(authority.substring(authority.lastIndexOf('@') + 1));
    }
    if (uri.getRawPath() != null) {
      out.append(uri.getRawPath());
    }
    return text(out.toString());
  }

  /**
   * A body as a record carries it.
   *
   * @param body the bytes, or null for none
   * @param contentType the {@code Content-Type}, or null
   * @return {@code body}, {@code bodyEncoding}, {@code bodyBytes} and {@code bodyTruncated}; the body
   *     is null when there was none
   */
  Map<String, Object> body(byte @Nullable [] body, @Nullable String contentType) {
    Map<String, Object> out = new LinkedHashMap<>();
    if (body == null || body.length == 0) {
      out.put("body", null);
      out.put("bodyEncoding", "utf8");
      out.put("bodyBytes", 0L);
      out.put("bodyTruncated", false);
      return out;
    }
    boolean huge = body.length > STRUCTURED_LIMIT;
    byte[] work = huge ? java.util.Arrays.copyOf(body, BODY_LIMIT + 4) : body;
    String text = huge ? decodeStart(work, BODY_LIMIT) : utf8(work);
    if (text == null) {
      // Not text (or text that holds a NUL, which the platform cannot store): its start, in base64.
      int kept = Math.min(body.length, BODY_LIMIT);
      out.put("body", Base64.getEncoder().encodeToString(java.util.Arrays.copyOf(body, kept)));
      out.put("bodyEncoding", "base64");
      out.put("bodyBytes", (long) body.length);
      out.put("bodyTruncated", body.length > BODY_LIMIT);
      return out;
    }
    String redacted = text(redactText(text, contentType, huge));
    byte[] bytes = redacted.getBytes(StandardCharsets.UTF_8);
    boolean truncated = huge || bytes.length > BODY_LIMIT;
    out.put("body", bytes.length > BODY_LIMIT ? cut(bytes) : redacted);
    out.put("bodyEncoding", "utf8");
    out.put("bodyBytes", (long) body.length);
    out.put("bodyTruncated", truncated);
    return out;
  }

  /** The start of the bytes at most {@link #BODY_LIMIT}, never in the middle of a character. */
  private static String cut(byte[] bytes) {
    int end = BODY_LIMIT;
    // A continuation byte at the cut means the character began before it: leave that one out.
    while (end > 0 && (bytes[end] & 0xC0) == 0x80) {
      end--;
    }
    return new String(bytes, 0, end, StandardCharsets.UTF_8);
  }

  private String redactText(String text, @Nullable String contentType, boolean unparsed) {
    String type = contentType == null ? "" : contentType.toLowerCase(Locale.ROOT);
    if (type.contains("x-www-form-urlencoded")) {
      return pairs(text, name -> sensitive(name) || BODY_FIELDS.contains(name.toLowerCase(Locale.ROOT)));
    }
    String start = text.stripLeading();
    boolean looksJson = start.startsWith("{") || start.startsWith("[");
    boolean json = type.contains("json") || (looksJson && !type.contains("xml") && !type.contains("html") && !type.contains("csv"));
    if (!json) {
      return text;
    }
    if (!unparsed) {
      try {
        JsonNode tree = Json.mapper().readTree(text);
        redactNode(tree);
        return Json.mapper().writeValueAsString(tree);
      } catch (JacksonException e) {
        // Cut off or not quite JSON: fall through to the pattern.
      }
    }
    return members(text);
  }

  private static void redactNode(JsonNode node) {
    if (node instanceof ObjectNode object) {
      for (String name : List.copyOf(object.propertyNames())) {
        if (sensitiveField(name)) {
          object.put(name, REDACTED);
        } else {
          redactNode(object.get(name));
        }
      }
    } else if (node instanceof ArrayNode array) {
      for (JsonNode each : array) {
        redactNode(each);
      }
    }
  }

  /** JSON redacted by pattern: for what cannot be parsed. */
  private static String members(String text) {
    Matcher m = JSON_MEMBER.matcher(text);
    StringBuilder out = new StringBuilder();
    while (m.find()) {
      String replacement = sensitiveField(m.group(1)) ? "\"" + m.group(1) + "\"" + m.group(2) + "\"" + REDACTED + "\"" : m.group();
      m.appendReplacement(out, Matcher.quoteReplacement(replacement));
    }
    m.appendTail(out);
    return out.toString();
  }

  /** {@code a=b&c=d} with the value of every name {@code sensitive} says so of replaced. */
  private static String pairs(String text, Predicate<String> sensitive) {
    StringBuilder out = new StringBuilder(text.length());
    boolean first = true;
    for (String part : text.split("&", -1)) {
      if (!first) {
        out.append('&');
      }
      first = false;
      int eq = part.indexOf('=');
      String rawName = eq < 0 ? part : part.substring(0, eq);
      String name;
      try {
        name = URLDecoder.decode(rawName, StandardCharsets.UTF_8);
      } catch (IllegalArgumentException e) {
        name = rawName;
      }
      if (eq >= 0 && sensitive.test(name)) {
        out.append(rawName).append('=').append(REDACTED);
      } else {
        out.append(part);
      }
    }
    return out.toString();
  }

  private static boolean sensitiveField(String name) {
    return sensitive(name) || BODY_FIELDS.contains(name.toLowerCase(Locale.ROOT));
  }

  private static boolean sensitiveQuery(String name) {
    String lower = name.toLowerCase(Locale.ROOT);
    return sensitive(name) || QUERY_FIELDS.contains(lower) || lower.startsWith("x-amz-");
  }

  /** The bytes as text when they are UTF-8 without a NUL, else null. */
  private static @Nullable String utf8(byte[] bytes) {
    try {
      String text = StandardCharsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT).onUnmappableCharacter(CodingErrorAction.REPORT).decode(ByteBuffer.wrap(bytes)).toString();
      return text.indexOf('\u0000') >= 0 ? null : text;
    } catch (CharacterCodingException e) {
      return null;
    }
  }

  /** The first {@code limit} bytes as text, leaving out a character cut in half; null when not text. */
  private static @Nullable String decodeStart(byte[] bytes, int limit) {
    int end = Math.min(limit, bytes.length);
    while (end > 0 && end < bytes.length && (bytes[end] & 0xC0) == 0x80) {
      end--;
    }
    return utf8(java.util.Arrays.copyOf(bytes, end));
  }
}
