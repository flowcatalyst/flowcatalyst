package io.flowcatalyst.ingestor.contract;

import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.annotation.JsonSubTypes;
import com.fasterxml.jackson.annotation.JsonTypeInfo;
import java.time.Duration;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import org.jspecify.annotations.Nullable;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.node.JsonNodeFactory;

/**
 * Where an ingestion's data comes from. The fields are a first cut: enough to tell the kinds
 * apart and to build a route from, not every option the underlying component has.
 */
@JsonTypeInfo(use = JsonTypeInfo.Id.NAME, property = "kind")
@JsonSubTypes({
  @JsonSubTypes.Type(value = Source.Sftp.class, name = "sftp"),
  @JsonSubTypes.Type(value = Source.Http.class, name = "http"),
  @JsonSubTypes.Type(value = Source.RabbitMq.class, name = "rabbitmq"),
  @JsonSubTypes.Type(value = Source.Sqs.class, name = "sqs"),
  @JsonSubTypes.Type(value = Source.S3.class, name = "s3"),
  @JsonSubTypes.Type(value = Source.DatabasePoll.class, name = "database-poll"),
  @JsonSubTypes.Type(value = Source.ApiPoll.class, name = "api-poll"),
  @JsonSubTypes.Type(value = Source.Custom.class, name = "custom"),
})
public sealed interface Source {

  /**
   * The kinds built in, as {@code kind} names them; an extension's type is none of these.
   *
   * @return the names, {@code custom} included
   */
  static java.util.Set<String> builtInKinds() {
    java.util.Set<String> names = new java.util.LinkedHashSet<>();
    for (JsonSubTypes.Type type : Source.class.getAnnotation(JsonSubTypes.class).value()) {
      names.add(type.name());
    }
    return java.util.Collections.unmodifiableSet(names);
  }

  /**
   * Whether exactly one runtime may run this source at a time. A source that is polled (a
   * directory, a bucket, a table) would be read twice by two runtimes; a queue or an HTTP endpoint
   * is shared safely.
   *
   * @return true for a polled source
   */
  @JsonIgnore
  default boolean singleOwner() {
    return switch (this) {
      // A custom source may poll; nothing says it can be run twice at once.
      case Sftp _, S3 _, DatabasePoll _, ApiPoll _, Custom _ -> true;
      case Http _, RabbitMq _, Sqs _ -> false;
    };
  }

  /**
   * Files picked up from a directory on an SFTP server.
   *
   * @param host the server
   * @param port the port
   * @param directory the directory polled
   * @param username the login
   * @param credential the password or private key
   * @param knownHosts the server's host key as a {@code known_hosts} line; without it the runtime
   *     refuses to connect unless {@code acceptAnyHostKey} is true
   * @param include a file name pattern such as {@code *.csv}, or null for every file
   * @param every how often the directory is polled
   * @param acceptAnyHostKey true to connect without checking who the server is, when there is no
   *     {@code knownHosts}: anyone who can intercept the connection then gets the login. Null or
   *     false refuses
   */
  record Sftp(
      String host,
      int port,
      String directory,
      String username,
      SecretRef credential,
      @Nullable String knownHosts,
      @Nullable String include,
      Duration every,
      @Nullable Boolean acceptAnyHostKey)
      implements Source {

    /**
     * A source that checks the server's host key when it is given and refuses to connect otherwise.
     *
     * @param host the server
     * @param port the port
     * @param directory the directory polled
     * @param username the login
     * @param credential the password or private key
     * @param knownHosts the server's host key as a {@code known_hosts} line
     * @param include a file name pattern, or null for every file
     * @param every how often the directory is polled
     */
    public Sftp(
        String host,
        int port,
        String directory,
        String username,
        SecretRef credential,
        @Nullable String knownHosts,
        @Nullable String include,
        Duration every) {
      this(host, port, directory, username, credential, knownHosts, include, every, null);
    }

    /**
     * Whether the definition asked to connect without checking the host key.
     *
     * @return true only when {@code acceptAnyHostKey} is true
     */
    @JsonIgnore
    public boolean acceptsAnyHostKey() {
      return Boolean.TRUE.equals(acceptAnyHostKey);
    }
  }

  /**
   * JSON posted to the endpoint the runtime exposes for this ingestion:
   * {@code POST /ingest/{clientId}/{code}}.
   *
   * @param auth how the caller authenticates
   * @param rateLimit how fast requests are accepted, or null for no limit
   */
  record Http(HttpAuth auth, @Nullable RateLimit rateLimit) implements Source {}

  /**
   * Messages consumed from a RabbitMQ queue.
   *
   * @param uri the broker address, without credentials
   * @param queue the queue
   * @param username the login
   * @param password the password
   */
  record RabbitMq(String uri, String queue, String username, SecretRef password) implements Source {}

  /**
   * Messages consumed from an SQS queue. A message is deleted once its records are dispatch jobs,
   * or when it cannot be read; otherwise it becomes visible again and is read again.
   *
   * @param region the AWS region
   * @param queue the queue name
   * @param credential the access key to read with, or null for the runtime's own identity
   */
  record Sqs(String region, String queue, @Nullable AwsCredential credential) implements Source {}

  /**
   * Objects picked up from an S3 bucket: those directly under the prefix, not the ones in deeper
   * "folders". An object is moved under {@code .done/} once its records are dispatch jobs, or under
   * {@code .rejected/} when it cannot be read; otherwise it stays and is read again.
   *
   * @param region the AWS region
   * @param bucket the bucket
   * @param prefix a key prefix ending in {@code /}, or null for the top of the bucket
   * @param every how often the bucket is polled
   * @param credential the access key to read with, or null for the runtime's own identity
   */
  record S3(String region, String bucket, @Nullable String prefix, Duration every, @Nullable AwsCredential credential)
      implements Source {}

  /**
   * An AWS access key.
   *
   * @param accessKeyId the key's id
   * @param secretAccessKey the key's secret
   */
  record AwsCredential(String accessKeyId, SecretRef secretAccessKey) {}

  /**
   * Rows polled from a database table. Each row is one record, a JSON object of its columns, so
   * the ingestion's split is {@code none}.
   *
   * @param jdbcUrl the database
   * @param username the login
   * @param password the password
   * @param table the table or view read, optionally {@code schema.table}
   * @param track how rows already read are told from new ones
   * @param maxRows how many rows one poll reads
   * @param every how often the table is polled
   */
  record DatabasePoll(
      String jdbcUrl, String username, SecretRef password, String table, Track track, int maxRows, Duration every)
      implements Source {}

  /**
   * An HTTP API polled: one request per poll, its answer split into records like any payload. After
   * an answer with results the next poll comes after {@code every}; after one without, after
   * {@code idleDelay}. When the API refuses the login, also with a fresh token, the ingestion is
   * reported {@code FAILING} and tried every five minutes until it answers or is saved again.
   *
   * @param url the address, with any fixed query parameters
   * @param method {@code GET} or {@code POST}
   * @param headers headers sent on every request, none of them secret; may be empty
   * @param body for a {@code POST}, the JSON sent; or null
   * @param auth how it logs in
   * @param results how an answer is told to have results
   * @param cursor what in an answer moves the next request on, or null when the API itself
   *     returns only what is new
   * @param every how soon to poll again after an answer with results
   * @param idleDelay how soon to poll again after an answer without
   */
  record ApiPoll(
      String url,
      Method method,
      @Nullable List<Header> headers,
      @Nullable String body,
      ApiAuth auth,
      ResultCheck results,
      @Nullable Cursor cursor,
      Duration every,
      Duration idleDelay)
      implements Source {

    /** The headers; none when absent. */
    public List<Header> headerList() {
      List<Header> list = headers;
      return list == null ? List.of() : list;
    }

    /** How an API poll asks. */
    public enum Method {
      /** A {@code GET}. */
      GET,
      /** A {@code POST} with the body. */
      POST
    }

    /**
     * A header sent on every request.
     *
     * @param name the name
     * @param value the value, which is not secret
     */
    public record Header(String name, String value) {}

    /**
     * What in an answer moves the next request on: a next-page token, or the last item's id or
     * time. It is kept by the platform between polls, like a database poll's watermark. An
     * answer without it keeps the one before.
     *
     * @param from where it is in the answer, such as {@code $.next} or {@code $.items[-1].updatedAt}
     * @param param the query parameter it is sent in
     * @param initial what is sent before any answer has given one, or null to send nothing
     */
    public record Cursor(String from, String param, @Nullable String initial) {}

    /** How an answer is told to have results. */
    @JsonTypeInfo(use = JsonTypeInfo.Id.NAME, property = "kind")
    @JsonSubTypes({
      @JsonSubTypes.Type(value = ResultCheck.Records.class, name = "records"),
      @JsonSubTypes.Type(value = ResultCheck.JsonPath.class, name = "json-path"),
      @JsonSubTypes.Type(value = ResultCheck.Header.class, name = "header"),
    })
    public sealed interface ResultCheck {

      /** The answer has results when the split makes at least one record of it. */
      record Records() implements ResultCheck {}

      /**
       * The answer has results when a path in it holds a non-empty array, or a value that is not
       * null, false or empty. An answer without results is not dispatched.
       *
       * @param path such as {@code $.items} or {@code $.hasMore}
       */
      record JsonPath(String path) implements ResultCheck {}

      /**
       * The answer has results when it carries a header, optionally with a value (ignoring case).
       * An answer without results is not dispatched.
       *
       * @param name the header
       * @param equals the value it must have, or null for any
       */
      record Header(String name, @Nullable String equals) implements ResultCheck {}
    }
  }

  /**
   * A source a runtime extension adds (a jar in the extensions directory, or on the class path), or
   * {@code camel-route}, which the runtime carries when its routes are enabled. The platform stores
   * the settings and knows nothing else about them; the runtime checks them against the kind's {@link
   * ExtensionKind} and its extension turns them into the Camel endpoint the route consumes from. Only
   * one runtime runs it at a time. A definition file writes it flat, as {@code kind: <type>} with the
   * settings beside it; the platform's API has {@code {kind: custom, type, settings}}.
   *
   * @param type the kind's {@link SourceKind#type() type}, such as {@code generator}
   * @param settings its settings by {@linkplain SourceKind.Field#name() name}: strings, numbers,
   *     booleans and lists of strings; a secret's is its name. May be empty
   */
  record Custom(String type, @Nullable Map<String, JsonNode> settings) implements Source {

    /** Copies the settings, leaving out any without a value. */
    public Custom {
      Map<String, JsonNode> copy = new LinkedHashMap<>();
      if (settings != null) {
        settings.forEach(
            (key, value) -> {
              if (value != null && !value.isNull() && !value.isMissingNode()) {
                copy.put(key, value);
              }
            });
      }
      settings = Collections.unmodifiableMap(copy);
    }

    /**
     * The settings; none when absent.
     *
     * @return the settings
     */
    public Map<String, JsonNode> settingMap() {
      Map<String, JsonNode> map = settings;
      return map == null ? Map.of() : map;
    }

    /**
     * A custom source from plain values: strings, numbers, booleans and lists of them.
     *
     * @param type the kind's type
     * @param settings the settings
     * @return the source
     */
    public static Custom of(String type, Map<String, ?> settings) {
      Map<String, JsonNode> nodes = new LinkedHashMap<>();
      settings.forEach((key, value) -> nodes.put(key, node(value)));
      return new Custom(type, nodes);
    }

    private static JsonNode node(@Nullable Object value) {
      return switch (value) {
        case null -> JsonNodeFactory.instance.nullNode();
        case String text -> JsonNodeFactory.instance.stringNode(text);
        case Boolean flag -> JsonNodeFactory.instance.booleanNode(flag);
        case Integer number -> JsonNodeFactory.instance.numberNode(number);
        case Long number -> JsonNodeFactory.instance.numberNode(number);
        case List<?> items -> {
          tools.jackson.databind.node.ArrayNode array = JsonNodeFactory.instance.arrayNode();
          items.forEach(item -> array.add(node(item)));
          yield array;
        }
        default -> JsonNodeFactory.instance.stringNode(value.toString());
      };
    }
  }

  /** How a database poll tells rows it has read from rows it has not. */
  @JsonTypeInfo(use = JsonTypeInfo.Id.NAME, property = "kind")
  @JsonSubTypes({
    @JsonSubTypes.Type(value = Track.Watermark.class, name = "watermark"),
    @JsonSubTypes.Type(value = Track.Status.class, name = "status"),
  })
  sealed interface Track {

    /**
     * Rows are read in the order of a column that only grows, and the highest value read is
     * remembered outside the table. Nothing is written to the table. A row committed later with a
     * lower value than one already read is never seen.
     *
     * @param column the column
     * @param type what the column holds
     */
    record Watermark(String column, Type type) implements Track {

      /** What a watermark column holds. */
      public enum Type {
        /** An incrementing number, such as an identity column. */
        NUMBER,
        /** A date and time, such as a last-modified column. */
        TIMESTAMP
      }
    }

    /**
     * Rows carry their own state: those whose status column holds the pending value are read, and
     * the column is set to the processed value once they are dispatch jobs.
     *
     * @param column the status column
     * @param keyColumn the column that identifies a row, usually the primary key
     * @param pending the value of a row not yet read, such as {@code 0}
     * @param processed the value written once it has been, such as {@code 1}
     */
    record Status(String column, String keyColumn, String pending, String processed) implements Track {}
  }
}
