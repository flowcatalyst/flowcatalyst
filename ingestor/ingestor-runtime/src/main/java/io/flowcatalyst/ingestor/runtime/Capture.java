package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.framework.httpclient.HttpCapture;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.Text;
import io.flowcatalyst.ingestor.contract.TraceRecord;
import java.time.Duration;
import java.time.Instant;
import java.time.InstantSource;
import java.util.Collection;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicLong;
import java.util.function.Consumer;
import java.util.function.Supplier;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * One capture of an ingestion: a poll cycle, or one exchange, whose HTTP calls, exchanges and end are
 * written as trace records that share an id. Made by {@link TraceWindows#begin(Ingestion)} only while
 * an operator has tracing switched on and the sample interval has passed. A capture bound to {@link
 * #CURRENT} around a poll is the one the route finds when the poll dispatches a payload, so the
 * payload's {@code exchange} record lands in the poll's capture. A {@linkplain #discarding discarding}
 * capture is bound around a poll that was not sampled: it records nothing, and tells the route not to
 * start a capture of its own for that poll's payloads (a capture is a whole poll or one exchange).
 *
 * <p>Everything written is redacted here, before a record exists. Nothing here throws into the
 * ingestion: a record that cannot be made is left out.
 */
final class Capture {

  private static final Logger LOG = LoggerFactory.getLogger(Capture.class);

  /** Bound around a poll whose HTTP calls and exchanges are recorded. */
  static final ScopedValue<Capture> CURRENT = ScopedValue.newInstance();

  /** The name the runtime's own client for the platform is registered under; its calls are not traced. */
  static final String PLATFORM_CLIENT = "platform";

  /** The longest error text a record keeps. */
  private static final int ERROR = 2000;

  private final Ingestion ingestion;
  private final String windowId;
  private final String captureId;
  private final TraceSink sink;
  private final InstantSource clock;
  private final Supplier<Collection<String>> secrets;
  private final Consumer<String> conceal;
  private final boolean recording;

  private final AtomicInteger httpCalls = new AtomicInteger();
  private final AtomicInteger payloads = new AtomicInteger();
  private final AtomicLong records = new AtomicLong();
  private final AtomicLong jobs = new AtomicLong();

  Capture(
      Ingestion ingestion,
      String windowId,
      TraceSink sink,
      InstantSource clock,
      Supplier<Collection<String>> secrets,
      Consumer<String> conceal) {
    this(ingestion, windowId, "c-" + UUID.randomUUID(), sink, clock, secrets, conceal, true);
  }

  private Capture(
      Ingestion ingestion,
      String windowId,
      String captureId,
      TraceSink sink,
      InstantSource clock,
      Supplier<Collection<String>> secrets,
      Consumer<String> conceal,
      boolean recording) {
    this.ingestion = ingestion;
    this.windowId = windowId;
    this.captureId = captureId;
    this.sink = sink;
    this.clock = clock;
    this.secrets = secrets;
    this.conceal = conceal;
    this.recording = recording;
  }

  /**
   * A capture that records nothing, for the polls the sampler passed over.
   *
   * @param ingestion the ingestion
   * @return the capture
   */
  static Capture discarding(Ingestion ingestion) {
    return new Capture(ingestion, "", "", TraceSink.DISCARD, InstantSource.system(), List::of, value -> {}, false);
  }

  /** Whether this capture writes records. */
  boolean recording() {
    return recording;
  }

  /** The id its records share. */
  String id() {
    return captureId;
  }

  /** The window it belongs to. */
  String windowId() {
    return windowId;
  }

  /**
   * One attempt of an outbound call made while the capture is bound ({@link HttpCapture.Sink}). Calls
   * to the platform itself (jobs, state, secrets) are not shown: the exchange record says what they
   * came to.
   *
   * @param call the attempt
   */
  void http(HttpCapture.Exchange call) {
    if (!recording || PLATFORM_CLIENT.equals(call.service())) {
      return;
    }
    try {
      // A credential the answer hands out (a login's session) is kept out of this record and every
      // later one: later calls may carry it where no name says what it is, such as a URL's path.
      for (String credential : TraceRedactor.credentialsIn(call.responseBody(), HttpRecords.contentType(call.responseHeaders()))) {
        conceal.accept(credential);
      }
      HttpRecords.Built built = HttpRecords.build(call, redactor(), TraceRedactor.BODY_LIMIT);
      httpCalls.incrementAndGet();
      write("http", built.level(), built.message(), built.data());
    } catch (RuntimeException e) {
      LOG.debug("ingestion {}: an http call could not be traced", ingestion.id(), e);
    }
  }

  /**
   * One payload through the route, written when it has completed or failed.
   *
   * @param source what the source says about it (file name, message id), by name
   * @param identity where it came from, or null
   * @param body the payload, or null when it is not held in memory
   * @param payloadBytes its size when known, else -1
   * @param recordCount records it was split into
   * @param jobCount jobs created from them
   * @param outcome {@code completed}, {@code rejected} or {@code failed}
   * @param error why it was rejected or failed, or null
   */
  void exchange(
      Map<String, String> source,
      @Nullable String identity,
      byte @Nullable [] body,
      long payloadBytes,
      long recordCount,
      long jobCount,
      String outcome,
      @Nullable String error) {
    if (!recording) {
      return;
    }
    try {
      TraceRedactor redactor = redactor();
      payloads.incrementAndGet();
      records.addAndGet(recordCount);
      jobs.addAndGet(jobCount);
      Map<String, Object> data = new LinkedHashMap<>();
      data.put("source", redactor.single(source));
      if (identity != null) {
        data.put("identity", redactor.text(Text.cut(identity, 500)));
      }
      data.put("payloadBytes", payloadBytes >= 0 ? payloadBytes : body != null ? body.length : 0);
      data.put("payload", body == null ? null : redactor.body(body, null));
      data.put("records", recordCount);
      data.put("jobs", jobCount);
      data.put("outcome", outcome);
      String said = error == null ? null : redactor.text(Text.cut(ExchangeCounters.clean(error), ERROR));
      if (said != null) {
        data.put("error", said);
      }
      String message = "exchange " + outcome + ": " + recordCount + " record(s), " + jobCount + " job(s)" + (said == null ? "" : ": " + said);
      write("exchange", outcome.equals("completed") ? "info" : outcome.equals("rejected") ? "warn" : "error", message, data);
    } catch (RuntimeException e) {
      LOG.debug("ingestion {}: an exchange could not be traced", ingestion.id(), e);
    }
  }

  /**
   * A failure inside the capture that is not an HTTP call, an exchange or the poll's own end.
   *
   * @param message what failed
   * @param kind the failure's kind, or null
   */
  void error(String message, @Nullable String kind) {
    if (!recording) {
      return;
    }
    try {
      TraceRedactor redactor = redactor();
      String said = redactor.text(Text.cut(ExchangeCounters.clean(message), ERROR));
      Map<String, Object> data = new LinkedHashMap<>();
      data.put("error", said);
      if (kind != null) {
        data.put("kind", kind);
      }
      write("error", "error", said, data);
    } catch (RuntimeException e) {
      LOG.debug("ingestion {}: an error could not be traced", ingestion.id(), e);
    }
  }

  /**
   * The end of a captured poll cycle.
   *
   * @param startedAt when it began
   * @param ok whether it ended without a failure
   * @param error why not, or null
   * @param next when it will be polled again, or null
   */
  void poll(Instant startedAt, boolean ok, @Nullable String error, @Nullable Duration next) {
    if (!recording) {
      return;
    }
    try {
      TraceRedactor redactor = redactor();
      long durationMs = Math.max(0, Duration.between(startedAt, clock.instant()).toMillis());
      Map<String, Object> data = new LinkedHashMap<>();
      data.put("startedAt", startedAt.toString());
      data.put("durationMs", durationMs);
      data.put("httpCalls", httpCalls.get());
      data.put("payloads", payloads.get());
      data.put("records", records.get());
      data.put("jobs", jobs.get());
      data.put("outcome", ok ? "ok" : "failed");
      String said = error == null ? null : redactor.text(Text.cut(ExchangeCounters.clean(error), ERROR));
      if (said != null) {
        data.put("error", said);
      }
      if (next != null) {
        data.put("nextDelayMs", next.toMillis());
      }
      String message = "poll: " + httpCalls.get() + " call(s), " + payloads.get() + " payload(s)" + (ok ? "" : " failed" + (said == null ? "" : ": " + said));
      write("poll", ok ? "info" : "error", message, data);
    } catch (RuntimeException e) {
      LOG.debug("ingestion {}: a poll could not be traced", ingestion.id(), e);
    }
  }

  private TraceRedactor redactor() {
    return new TraceRedactor(secrets.get());
  }

  private void write(String kind, String level, String message, Map<String, Object> data) {
    sink.offer(
        new TraceRecord(
            UUID.randomUUID().toString(),
            ingestion.id(),
            ingestion.version(),
            windowId,
            captureId,
            clock.instant().toEpochMilli(),
            kind,
            level,
            message,
            data));
  }
}
