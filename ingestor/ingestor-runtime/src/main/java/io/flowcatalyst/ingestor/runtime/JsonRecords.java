package io.flowcatalyst.ingestor.runtime;

import java.io.Closeable;
import java.io.InputStream;
import java.util.Arrays;
import java.util.Iterator;
import java.util.List;
import java.util.NoSuchElementException;
import org.apache.camel.Exchange;
import org.apache.camel.support.ExpressionAdapter;
import org.jspecify.annotations.Nullable;
import tools.jackson.core.JsonParser;
import tools.jackson.core.JsonToken;
import tools.jackson.databind.DeserializationFeature;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.json.JsonMapper;

/**
 * Splits a JSON document into the elements of one array, reading the document as a stream: only
 * the element being handed on is held in memory.
 */
final class JsonRecords extends ExpressionAdapter {

  /** Its own mapper: elements are read one at a time from a parser that has more to give. */
  private static final ObjectMapper MAPPER = JsonMapper.builder().disable(DeserializationFeature.FAIL_ON_TRAILING_TOKENS).build();

  private final List<String> fields;

  /**
   * Creates the splitter.
   *
   * @param arrayPath {@code $} or {@code $.field.field}
   * @throws UnsupportedIngestionException when the path is anything else
   */
  JsonRecords(String arrayPath) {
    if (!arrayPath.matches("\\$(\\.[A-Za-z_][A-Za-z0-9_-]*)*")) {
      throw new UnsupportedIngestionException(
          "json split path '" + arrayPath + "' is not supported: use $ or $.field.field");
    }
    this.fields = arrayPath.equals("$") ? List.of() : Arrays.asList(arrayPath.substring(2).split("\\."));
  }

  @Override
  public Object evaluate(Exchange exchange) {
    InputStream body = exchange.getMessage().getBody(InputStream.class);
    if (body == null) {
      throw new IllegalArgumentException("the payload cannot be read as a stream");
    }
    JsonParser parser = MAPPER.createParser(body);
    try {
      seekArray(parser);
    } catch (RuntimeException e) {
      parser.close();
      throw e;
    }
    return new Elements(parser);
  }

  /** Leaves the parser on the opening bracket of the array the path names. */
  private void seekArray(JsonParser parser) {
    JsonToken token = parser.nextToken();
    for (String field : fields) {
      if (token != JsonToken.START_OBJECT) {
        throw new IllegalArgumentException("expected an object above '" + field + "'");
      }
      while (true) {
        token = parser.nextToken();
        if (token != JsonToken.PROPERTY_NAME) {
          throw new IllegalArgumentException("the document has no '" + field + "' where the split path expects it");
        }
        boolean found = field.equals(parser.currentName());
        token = parser.nextToken();
        if (found) {
          break;
        }
        parser.skipChildren();
      }
    }
    if (token != JsonToken.START_ARRAY) {
      throw new IllegalArgumentException("the split path does not lead to an array");
    }
  }

  /** The array's elements, each as compact JSON. Closed by Camel when the split ends. */
  private static final class Elements implements Iterator<String>, Closeable {

    private final JsonParser parser;
    private @Nullable String next;
    private boolean done;

    Elements(JsonParser parser) {
      this.parser = parser;
    }

    @Override
    public boolean hasNext() {
      if (next == null && !done) {
        try {
          advance();
        } catch (RuntimeException e) {
          // Asked again after a failure (Camel does, when it lets go of the exchange): no more records.
          done = true;
          throw e;
        }
      }
      return next != null;
    }

    private void advance() {
      JsonToken token = parser.nextToken();
      if (token == null) {
        throw new IllegalArgumentException("the document ends before its array is closed");
      }
      if (token == JsonToken.END_ARRAY) {
        done = true;
      } else {
        next = MAPPER.readTree(parser).toString();
      }
    }

    @Override
    public String next() {
      if (!hasNext()) {
        throw new NoSuchElementException();
      }
      String value = next;
      next = null;
      if (value == null) {
        throw new NoSuchElementException();
      }
      return value;
    }

    @Override
    public void close() {
      try {
        parser.close();
      } catch (RuntimeException e) {
        // A document cut short was already rejected while it was read.
      }
    }
  }
}
