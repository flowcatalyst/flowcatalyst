package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.CamelRoutes;
import io.flowcatalyst.ingestor.contract.JobRecord;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.ingestor.contract.Target;
import io.flowcatalyst.ingestor.contract.RuntimeReport.Failure;
import io.flowcatalyst.ingestor.extension.PolledSource;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;
import org.apache.camel.CamelContext;
import org.apache.camel.Endpoint;
import org.apache.camel.Exchange;
import org.apache.camel.Expression;
import org.apache.camel.builder.RouteBuilder;
import org.apache.camel.model.TryDefinition;
import org.apache.camel.support.PluginHelper;
import org.apache.camel.support.ResourceHelper;
import org.apache.camel.support.ScheduledPollEndpoint;
import org.apache.camel.support.SynchronizationAdapter;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * The route for one ingestion: consume from its source, split the payload into records, and create
 * one dispatch job per record, a batch of records at a time through the platform's runtime API,
 * which builds each job from the ingestion's own target. The route id is the ingestion id.
 *
 * <p>Jobs are created on the thread that consumed the payload, so a batch the platform does not
 * take fails the exchange and the source keeps the payload (the file is not moved, the message is
 * not acknowledged). A payload that cannot be read is rejected instead: the exchange ends, and the
 * source lets go of it.
 */
final class IngestionRoute extends RouteBuilder {

  private static final Logger LOG = LoggerFactory.getLogger(IngestionRoute.class);

  private static final String PENDING = "ingest.pending";

  /** Exchange property: why the payload was rejected. Absent when it was not. */
  static final String REJECTED = "ingestRejected";

  /** Exchange property: how many records the payload was split into, once it has been. */
  static final String RECORDS = "ingestRecords";

  /** Exchange property: the folder a file source moves the file into when the exchange ends. */
  static final String FOLDER = "ingestFolder";

  /** A camel-route ingestion's own route: its id is the ingestion's with this after it. */
  static final String SOURCE_ROUTE = "-source";

  /** The route a camel-route ingestion's failing steps go to, to be reported. */
  static final String FAILURES_ROUTE = "-failures";

  /** The jobs of one inbound payload not yet sent. Used by one thread at a time. */
  private static final class Pending {
    final String messageGroup;
    final List<JobRecord> jobs = new ArrayList<>();
    /** How many records have been read, sent or not. */
    int read;
    boolean rejected;
    /** The record last read, kept only when the ingestion keeps failed payloads. */
    @Nullable String lastRecord;

    Pending(String messageGroup) {
      this.messageGroup = messageGroup;
    }
  }

  private final Ingestion ingestion;
  private final SourceEndpoints endpoints;
  private final JobSink sink;
  private final ExchangeCounters counters;
  private final SourceHealth health;
  /** The endpoint the route consumes from, once configured. */
  private @Nullable Endpoint consumed;

  IngestionRoute(
      Ingestion ingestion, SourceEndpoints endpoints, JobSink sink, ExchangeCounters counters) {
    this(ingestion, endpoints, sink, counters, new SourceHealth());
  }

  IngestionRoute(
      Ingestion ingestion, SourceEndpoints endpoints, JobSink sink, ExchangeCounters counters, SourceHealth health) {
    this.ingestion = ingestion;
    this.endpoints = endpoints;
    this.sink = sink;
    this.counters = counters;
    this.health = health;
  }

  /** Where a polled source's failures go: the counts and the ingestion's health, a secret taken out. */
  private PolledSource.Reporter reporter() {
    return new PolledSource.Reporter() {
      @Override
      public void failed(Failure.Kind kind, String message) {
        String clean = ExchangeCounters.clean(message);
        counters.failure(ingestion, kind, clean, null, null);
        health.failing(ingestion, clean);
      }

      @Override
      public void recovered() {
        health.recovered(ingestion);
      }
    };
  }

  @Override
  public void configure() {
    Endpoint source = endpoints.endpoint(getContext(), ingestion);
    consumed = source;
    boolean route = CamelRoutes.yaml(ingestion.source()) != null;
    ReportingPollStrategy polls = new ReportingPollStrategy(ingestion, counters, health);
    switch (source) {
      case ScheduledPollEndpoint polled -> polled.setPollStrategy(polls);
      case DatabasePollEndpoint table -> table.pollStrategy(polls);
      case ApiPollEndpoint api -> api.reportTo(counters, health);
      case PolledSource polled -> polled.reportTo(reporter());
      default -> {}
    }
    if (route) {
      from(failures(source)).routeId(ingestion.id() + FAILURES_ROUTE).process(this::sourceFailed);
    }
    TryDefinition work = from(source).routeId(ingestion.id()).process(this::begin).doTry();
    if (ingestion.source() instanceof Source.DatabasePoll) {
      // The body is already the list of rows.
      each(work, body());
    } else {
      split(work);
    }
    work.process(this::finish)
        .doCatch(Exception.class)
        .onWhen(IngestionRoute::payloadIsAtFault)
        .process(this::reject)
        .end();
  }

  /**
   * Adds this ingestion's routes; for a camel-route ingestion, then its own route from the YAML,
   * which hands each payload to them.
   */
  @Override
  public void addRoutesToCamelContext(CamelContext context) throws Exception {
    super.addRoutesToCamelContext(context);
    Endpoint source = consumed;
    String route = CamelRoutes.yaml(ingestion.source());
    if (route != null && source != null) {
      String tail = source.getEndpointUri();
      String yaml = CamelRoutes.prepare(route, ingestion.id() + SOURCE_ROUTE, tail, failures(tail));
      PluginHelper.getRoutesLoader(context).loadRoutes(ResourceHelper.fromString("ingestion-" + ingestion.id() + ".yaml", yaml));
    }
  }

  /** Where a camel-route ingestion's failing steps are sent: beside its tail, as unguessable. */
  private static String failures(Endpoint tail) {
    return failures(tail.getEndpointUri());
  }

  private static String failures(String tail) {
    // Camel writes it direct://ingest-tail-..., so the name is replaced, not the scheme.
    return tail.replace("ingest-tail-", "ingest-failures-");
  }

  /** A step of a camel-route ingestion threw: reported as the source's failure, and the exchange fails. */
  private void sourceFailed(Exchange exchange) throws Exception {
    Throwable cause = exchange.getProperty(Exchange.EXCEPTION_CAUGHT, Throwable.class);
    boolean dispatch = false;
    for (Throwable t = cause; t != null; t = t.getCause()) {
      dispatch |= t instanceof DispatchFailedException;
    }
    // Jobs the platform did not take were reported by the ingestion's route already.
    if (!dispatch) {
      String message = cause == null || cause.getMessage() == null ? "a step of the route failed" : cause.getMessage();
      LOG.warn("ingestion {}: a step of its route failed: {}", ingestion.id(), message);
      counters.failure(ingestion, Failure.Kind.SOURCE, message, null, null);
      if (exchange.getProperty(PENDING) == null) {
        // It failed before the ingestion's route began, so nothing counted it.
        counters.exchangeFailed(ingestion);
      }
      health.failing(ingestion, message);
    }
    if (cause instanceof Exception e) {
      throw e;
    }
    throw new IllegalStateException("a step of the route failed", cause);
  }

  private void split(TryDefinition work) {
    switch (ingestion.split()) {
      case Split.None _ -> work.process(this::add);
      case Split.Csv csv -> each(work, body().tokenize("\n", 1, csv.header()));
      case Split.Xml xml -> each(work, body().tokenizeXML(xml.element(), null));
      case Split.Json json -> each(work, new JsonRecords(json.arrayPath()));
    }
  }

  /** Splits in order on the consuming thread; the first failing record fails the payload. */
  private void each(TryDefinition work, Expression records) {
    work.split(records).streaming().stopOnException().process(this::add).end();
  }

  /**
   * Whether a failure is the payload's and not the platform's. A payload that cannot be read will
   * not read better next time, so it is rejected and the source lets go of it; a batch the platform
   * did not take fails the exchange, and the source keeps the payload to try again.
   */
  private static boolean payloadIsAtFault(Exchange exchange) {
    // While the catch is being chosen the exception is still on the exchange; afterwards it is a property.
    Throwable thrown = exchange.getException();
    if (thrown == null) {
      thrown = exchange.getProperty(Exchange.EXCEPTION_CAUGHT, Throwable.class);
    }
    for (Throwable t = thrown; t != null; t = t.getCause()) {
      if (t instanceof DispatchFailedException || t instanceof InterruptedException) {
        return false;
      }
    }
    return true;
  }

  /** Jobs already sent for earlier records of the payload stay sent; the rest are dropped. */
  private void reject(Exchange exchange) {
    Throwable cause = exchange.getProperty(Exchange.EXCEPTION_CAUGHT, Throwable.class);
    String reason = cause == null || cause.getMessage() == null ? "the payload could not be read" : cause.getMessage();
    Pending pending = pending(exchange);
    pending.rejected = true;
    pending.jobs.clear();
    exchange.setProperty(RECORDS, (long) pending.read);
    exchange.setProperty(REJECTED, reason);
    exchange.setProperty(FOLDER, ".rejected");
    LOG.warn("ingestion {} rejected a payload after {} record(s): {}", ingestion.id(), pending.read, reason);
    counters.failure(
        ingestion, Failure.Kind.REJECTED, reason, (long) pending.read, excerpt(exchange, pending));
  }

  /** The record read last, or else the payload when it is text; only asked for when payloads are kept. */
  private @Nullable String excerpt(Exchange exchange, Pending pending) {
    if (!ingestion.keepFailedPayloads()) {
      return null;
    }
    if (pending.lastRecord != null) {
      return pending.lastRecord;
    }
    Object body = exchange.getMessage().getBody();
    return switch (body) {
      case String text -> text;
      case byte[] bytes -> new String(bytes, 0, Math.min(bytes.length, ExchangeCounters.EXCERPT * 4), StandardCharsets.UTF_8);
      case null, default -> null;
    };
  }

  private void begin(Exchange exchange) {
    Pending pending = new Pending(ingestion.id() + ":" + exchange.getExchangeId());
    exchange.setProperty(PENDING, pending);
    exchange.setProperty(FOLDER, ".done");
    exchange
        .getExchangeExtension()
        .addOnCompletion(
            new SynchronizationAdapter() {
              @Override
              public void onComplete(Exchange done) {
                if (pending.rejected) {
                  counters.exchangeFailed(ingestion);
                } else {
                  counters.exchangeCompleted(ingestion);
                }
              }

              @Override
              public void onFailure(Exchange done) {
                counters.exchangeFailed(ingestion);
                // Camel wraps it; the platform's own reason is under the DispatchFailedException.
                Throwable cause = done.getException();
                for (Throwable t = cause; t != null; t = t.getCause()) {
                  if (t instanceof DispatchFailedException && t.getCause() != null) {
                    cause = t.getCause();
                    break;
                  }
                }
                counters.failure(
                    ingestion,
                    Failure.Kind.NOT_DISPATCHED,
                    cause == null ? "the payload was not taken" : cause.getMessage(),
                    (long) pending.read,
                    excerpt(done, pending));
              }
            });
  }

  private void add(Exchange exchange) throws Exception {
    String record = exchange.getMessage().getBody(String.class);
    if (record == null || record.isBlank()) {
      return;
    }
    Pending pending = pending(exchange);
    if (ingestion.keepFailedPayloads()) {
      pending.lastRecord = record;
    }
    Target target = ingestion.target();
    pending.read++;
    pending.jobs.add(new JobRecord(record.strip(), contentType(), pending.messageGroup, null, pending.messageGroup, null));
    if (pending.jobs.size() >= Math.clamp(target.batchSize(), 1, JobRecord.MAX_PER_REQUEST)) {
      flush(pending);
    }
  }

  private String contentType() {
    return switch (ingestion.split()) {
      case Split.Csv _ -> "text/csv";
      case Split.Xml _ -> "application/xml";
      case Split.Json _, Split.None _ -> "application/json";
    };
  }

  private void finish(Exchange exchange) throws Exception {
    Pending pending = pending(exchange);
    flush(pending);
    exchange.setProperty(RECORDS, (long) pending.read);
    if (CamelRoutes.yaml(ingestion.source()) != null) {
      // A payload made it through the route's steps: it is no longer failing.
      health.recovered(ingestion);
    }
  }

  private void flush(Pending pending) throws Exception {
    if (pending.jobs.isEmpty()) {
      return;
    }
    List<JobRecord> batch = List.copyOf(pending.jobs);
    pending.jobs.clear();
    try {
      sink.create(ingestion, batch);
    } catch (InterruptedException e) {
      throw e;
    } catch (Exception e) {
      throw new DispatchFailedException(e);
    }
    counters.dispatched(ingestion, batch.size());
  }

  private static Pending pending(Exchange exchange) {
    Pending pending = exchange.getProperty(PENDING, Pending.class);
    if (pending == null) {
      throw new IllegalStateException("the exchange did not enter through the ingestion route");
    }
    return pending;
  }
}
