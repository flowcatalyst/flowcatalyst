package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Source.Track;
import io.flowcatalyst.framework.config.Secret;
import io.flowcatalyst.framework.http.Json;
import java.math.BigDecimal;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.ResultSetMetaData;
import java.sql.SQLException;
import java.sql.Timestamp;
import java.sql.Types;
import java.time.LocalDate;
import java.time.LocalDateTime;
import java.time.LocalTime;
import java.time.OffsetDateTime;
import java.util.ArrayList;
import java.util.Base64;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Properties;
import org.apache.camel.CamelContext;
import org.apache.camel.Consumer;
import org.apache.camel.Exchange;
import org.apache.camel.Processor;
import org.apache.camel.Producer;
import org.apache.camel.spi.PollingConsumerPollStrategy;
import org.apache.camel.support.DefaultEndpoint;
import org.apache.camel.support.ScheduledPollConsumer;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * A database table polled over JDBC. One poll reads up to {@code maxRows} new rows and offers them
 * as one exchange whose body is the list of rows, each a JSON object of its columns. Only when
 * their dispatch jobs exist are the rows marked as read: the watermark is stored, or the status
 * column is updated. If that last step fails the rows are read again, so a job can be created
 * twice but a row is never skipped.
 *
 * <p>The driver must be on the classpath; PostgreSQL's is.
 */
final class DatabasePollEndpoint extends DefaultEndpoint {

  private static final Logger LOG = LoggerFactory.getLogger(DatabasePollEndpoint.class);

  /** The key the watermark is stored under. */
  static final String WATERMARK = "watermark";

  private static final int QUERY_TIMEOUT_SECONDS = 60;

  private final String ingestionId;
  private final Source.DatabasePoll source;
  private final Secret password;
  private final StateStore state;
  private @Nullable PollingConsumerPollStrategy pollStrategy;

  DatabasePollEndpoint(
      CamelContext camel, String ingestionId, Source.DatabasePoll source, Secret password, StateStore state) {
    this.ingestionId = ingestionId;
    this.source = source;
    this.password = password;
    this.state = state;
    setCamelContext(camel);
  }

  @Override
  protected String createEndpointUri() {
    return "ingest-database:" + ingestionId;
  }

  @Override
  public Producer createProducer() {
    throw new UnsupportedOperationException("an ingestion's source is only consumed from");
  }

  @Override
  public Consumer createConsumer(Processor processor) throws Exception {
    TablePoller consumer = new TablePoller(processor);
    consumer.setDelay(source.every().toMillis());
    consumer.setInitialDelay(Math.min(1000, source.every().toMillis()));
    configureConsumer(consumer);
    PollingConsumerPollStrategy strategy = pollStrategy;
    if (strategy != null) {
      consumer.setPollStrategy(strategy);
    }
    return consumer;
  }

  /** What a failed or successful poll is told to; set before the route starts. */
  void pollStrategy(PollingConsumerPollStrategy strategy) {
    this.pollStrategy = strategy;
  }

  /** The rows of one poll, and what marks them as read. */
  private record Batch(List<String> rows, List<Object> keys, @Nullable String watermark) {}

  private final class TablePoller extends ScheduledPollConsumer {

    TablePoller(Processor processor) {
      super(DatabasePollEndpoint.this, processor);
    }

    @Override
    protected int poll() throws Exception {
      Properties login = new Properties();
      login.setProperty("user", source.username());
      login.setProperty("password", password.reveal());
      try (Connection connection = DriverManager.getConnection(source.jdbcUrl(), login)) {
        // A transaction lets the driver read with a cursor, and makes the status update one step.
        connection.setAutoCommit(false);
        Batch batch =
            switch (source.track()) {
              case Track.Watermark watermark -> readPast(connection, watermark);
              case Track.Status status -> readPending(connection, status);
            };
        if (batch.rows().isEmpty()) {
          return 0;
        }
        if (!dispatched(batch)) {
          return 0;
        }
        switch (source.track()) {
          case Track.Watermark _ -> {
            if (batch.watermark() != null) {
              state.put(ingestionId, WATERMARK, batch.watermark());
            }
          }
          case Track.Status status -> markProcessed(connection, status, batch.keys());
        }
        connection.commit();
        return batch.rows().size();
      }
    }

    /** Runs the rows through the route. False when the platform did not take their jobs. */
    private boolean dispatched(Batch batch) {
      Exchange exchange = createExchange(false);
      try {
        exchange.getMessage().setBody(batch.rows());
        try {
          getProcessor().process(exchange);
        } catch (Exception e) {
          exchange.setException(e);
        }
        if (exchange.getException() != null) {
          LOG.warn("ingestion {} could not take {} row(s); they are read again: {}", ingestionId, batch.rows().size(), exchange.getException().getMessage());
          return false;
        }
        return true;
      } finally {
        releaseExchange(exchange, false);
      }
    }

    /**
     * Rows past the stored watermark, in the column's order. Reading stops after {@code maxRows}
     * only where the column's value changes, so rows that share a value are never split between
     * two polls and the "greater than" of the next poll skips none of them.
     */
    private Batch readPast(Connection connection, Track.Watermark watermark) throws Exception {
      String column = watermark.column();
      @Nullable String stored = state.get(ingestionId, WATERMARK).orElse(null);
      String sql =
          "SELECT * FROM " + source.table()
              + " WHERE " + column + (stored == null ? " IS NOT NULL" : " > ?")
              + " ORDER BY " + column;
      try (PreparedStatement statement = connection.prepareStatement(sql)) {
        statement.setQueryTimeout(QUERY_TIMEOUT_SECONDS);
        statement.setFetchSize(source.maxRows());
        if (stored != null) {
          switch (watermark.type()) {
            case NUMBER -> statement.setBigDecimal(1, new BigDecimal(stored));
            case TIMESTAMP -> statement.setTimestamp(1, Timestamp.valueOf(stored));
          }
        }
        List<String> rows = new ArrayList<>();
        @Nullable String last = null;
        try (ResultSet results = statement.executeQuery()) {
          while (results.next()) {
            String value =
                switch (watermark.type()) {
                  case NUMBER -> results.getBigDecimal(column).toPlainString();
                  case TIMESTAMP -> results.getTimestamp(column).toString();
                };
            if (rows.size() >= source.maxRows() && !value.equals(last)) {
              break;
            }
            rows.add(json(results));
            last = value;
          }
        }
        return new Batch(rows, List.of(), last);
      }
    }

    private Batch readPending(Connection connection, Track.Status status) throws SQLException {
      String sql =
          "SELECT * FROM " + source.table() + " WHERE " + status.column() + " = ? ORDER BY " + status.keyColumn();
      try (PreparedStatement statement = connection.prepareStatement(sql)) {
        statement.setQueryTimeout(QUERY_TIMEOUT_SECONDS);
        statement.setMaxRows(source.maxRows());
        bind(statement, 1, status.pending());
        List<String> rows = new ArrayList<>();
        List<Object> keys = new ArrayList<>();
        try (ResultSet results = statement.executeQuery()) {
          while (results.next()) {
            rows.add(json(results));
            keys.add(results.getObject(status.keyColumn()));
          }
        }
        return new Batch(rows, keys, null);
      }
    }

    private void markProcessed(Connection connection, Track.Status status, List<Object> keys) throws SQLException {
      String sql =
          "UPDATE " + source.table() + " SET " + status.column() + " = ? WHERE " + status.keyColumn() + " = ? AND "
              + status.column() + " = ?";
      try (PreparedStatement statement = connection.prepareStatement(sql)) {
        statement.setQueryTimeout(QUERY_TIMEOUT_SECONDS);
        for (Object key : keys) {
          bind(statement, 1, status.processed());
          statement.setObject(2, key);
          bind(statement, 3, status.pending());
          statement.addBatch();
        }
        statement.executeBatch();
      }
    }
  }

  /**
   * Binds a status value by what it looks like, since the column may be a number, a boolean or
   * text: {@code 0} as a number, {@code true} as a boolean, anything else as text.
   */
  private static void bind(PreparedStatement statement, int index, String value) throws SQLException {
    if (value.matches("-?\\d{1,18}")) {
      statement.setLong(index, Long.parseLong(value));
    } else if (value.equals("true") || value.equals("false")) {
      statement.setBoolean(index, Boolean.parseBoolean(value));
    } else {
      statement.setString(index, value);
    }
  }

  /** One row as a JSON object, columns in the table's order under the names the database gives. */
  private static String json(ResultSet results) throws SQLException {
    ResultSetMetaData columns = results.getMetaData();
    Map<String, @Nullable Object> row = new LinkedHashMap<>();
    for (int i = 1; i <= columns.getColumnCount(); i++) {
      row.put(columns.getColumnLabel(i), value(results, columns, i));
    }
    return Json.mapper().writeValueAsString(row);
  }

  /** Dates and times as ISO text, binary as base64, numbers and booleans as they are. */
  private static @Nullable Object value(ResultSet results, ResultSetMetaData columns, int i) throws SQLException {
    int type = columns.getColumnType(i);
    boolean zoned = type == Types.TIMESTAMP_WITH_TIMEZONE || "timestamptz".equalsIgnoreCase(columns.getColumnTypeName(i));
    if (zoned) {
      OffsetDateTime at = results.getObject(i, OffsetDateTime.class);
      return at == null ? null : at.toInstant().toString();
    }
    return switch (type) {
      case Types.TIMESTAMP -> text(results.getObject(i, LocalDateTime.class));
      case Types.DATE -> text(results.getObject(i, LocalDate.class));
      case Types.TIME -> text(results.getObject(i, LocalTime.class));
      default ->
          switch (results.getObject(i)) {
            case null -> null;
            case Number number -> number;
            case Boolean bool -> bool;
            case byte[] bytes -> Base64.getEncoder().encodeToString(bytes);
            case Object other -> other.toString();
          };
    };
  }

  private static @Nullable String text(@Nullable Object value) {
    return value == null ? null : value.toString();
  }
}
