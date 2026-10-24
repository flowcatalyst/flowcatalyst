package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.framework.httpclient.module.ServiceClients;
import io.flowcatalyst.ingestor.contract.CamelRoutes;
import io.flowcatalyst.ingestor.contract.ConnectionTest;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.TestRequest;
import io.flowcatalyst.ingestor.extension.IngestExtension;
import io.flowcatalyst.ingestor.extension.PolledSource;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.atomic.AtomicLong;
import java.util.function.Function;
import java.util.function.LongSupplier;
import org.apache.camel.CamelContext;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * "Test connection": the platform hands a runtime that carries the kind a request in its poll, the
 * runtime tries the ingestion's source once, as its route would, with its own secrets and from its
 * own network, dispatches nothing, and answers in its next report. The ingestion is saved or not.
 */
final class TestRunner implements Function<TestRequest, ConnectionTest> {

  private static final Logger LOG = LoggerFactory.getLogger(TestRunner.class);

  private final Secrets secrets;
  private final ServiceClients http;
  private final Extensions extensions;
  private final CamelContext camel;
  private final LongSupplier nanos;
  private final AtomicLong tests = new AtomicLong();

  TestRunner(Secrets secrets, ServiceClients http, Extensions extensions, CamelContext camel, LongSupplier nanos) {
    this.secrets = secrets;
    this.http = http;
    this.extensions = extensions;
    this.camel = camel;
    this.nanos = nanos;
  }

  @Override
  public ConnectionTest apply(TestRequest request) {
    Ingestion ingestion = request.ingestion();
    try {
      return test(request.id(), ingestion);
    } catch (RuntimeException e) {
      // A secret the platform would not give, a client that could not be built: the test's answer, not the loop's failure.
      LOG.warn("test {} of ingestion {} failed: {}", request.id(), ingestion.code(), ExchangeCounters.clean(e.getMessage()));
      return result(request.id(), ConnectionTest.Outcome.FAILED, List.of(ExchangeCounters.clean(e.getMessage()) + "."), null, null);
    }
  }

  /** Tries the ingestion's source once. */
  private ConnectionTest test(String id, Ingestion ingestion) {
    if (CamelRoutes.yaml(ingestion.source()) != null) {
      return route(id, ingestion);
    }
    return switch (ingestion.source()) {
      case Source.ApiPoll api -> apiPoll(id, ingestion, api);
      case Source.Custom custom -> custom(id, ingestion, custom);
      default ->
          result(id, ConnectionTest.Outcome.NOT_SUPPORTED, List.of("Testing is offered for API polls and the extensions that have a test, so far."), null, null);
    };
  }

  /** A route is checked, not run: it would act on its own sources, with this runtime's identity. */
  private ConnectionTest route(String id, Ingestion ingestion) {
    if (!extensions.camelRoutes()) {
      return result(id, ConnectionTest.Outcome.FAILED, List.of("This runtime does not run camel routes (INGEST_CAMEL_ROUTES_ENABLED is not true)."), null, null);
    }
    String problem = CamelRoutes.problem(CamelRoutes.yaml(ingestion.source()));
    if (problem != null) {
      return result(id, ConnectionTest.Outcome.FAILED, List.of("The route " + ExchangeCounters.clean(problem) + "."), null, null);
    }
    return result(id, ConnectionTest.Outcome.NOT_SUPPORTED, List.of("The route is valid. A route is not run for a test."), null, null);
  }

  /** Tries a custom source with its extension, when the extension offers a test. */
  private ConnectionTest custom(String id, Ingestion ingestion, Source.Custom custom) {
    List<String> steps = new ArrayList<>();
    IngestExtension.Tested tested;
    try {
      tested = extensions.test(camel, ingestion, custom, secrets, http, tests.incrementAndGet(), steps);
    } catch (UnsupportedIngestionException e) {
      return result(id, ConnectionTest.Outcome.FAILED, List.of(ExchangeCounters.clean(e.getMessage()) + "."), null, null);
    } catch (InterruptedException e) {
      Thread.currentThread().interrupt();
      return failed(id, steps, e);
    } catch (Exception e) {
      return failed(id, steps, e);
    }
    if (tested == null) {
      return result(id, ConnectionTest.Outcome.NOT_SUPPORTED, List.of("The " + custom.type() + " source offers no test."), null, null);
    }
    return result(id, tested.outcome(), steps, tested.records(), null);
  }

  private ConnectionTest failed(String id, List<String> steps, Exception e) {
    List<String> all = new ArrayList<>(steps);
    all.add(ExchangeCounters.clean(e.getMessage() == null ? e.getClass().getSimpleName() : e.getMessage()) + ".");
    return result(id, PolledSource.refusedLogin(e) ? ConnectionTest.Outcome.REFUSED : ConnectionTest.Outcome.FAILED, all, null, null);
  }

  private ConnectionTest apiPoll(String id, Ingestion ingestion, Source.ApiPoll api) {
    ApiPollClient client;
    try {
      // Its own clients, apart from the route's when the ingestion is running too.
      client = new ApiPollClient(ingestion, api, secrets, ApiPollClient.name("api-test", tests.incrementAndGet()), http, nanos);
    } catch (UnsupportedIngestionException e) {
      return result(id, ConnectionTest.Outcome.FAILED, List.of(ExchangeCounters.clean(e.getMessage()) + "."), null, null);
    }
    Source.ApiPoll.Cursor cursor = api.cursor();
    ApiPollClient.Outcome outcome;
    try (client) {
      outcome = client.poll(cursor == null ? null : cursor.initial());
    }
    return switch (outcome) {
      case ApiPollClient.Refused refused -> result(id, ConnectionTest.Outcome.REFUSED, refused.steps(), null, null);
      case ApiPollClient.Failed failed -> result(id, ConnectionTest.Outcome.FAILED, failed.steps(), null, kept(ingestion, failed.body()));
      case ApiPollClient.Answer answer -> {
        long records = RecordCount.of(ingestion.split(), answer.body());
        List<String> steps = new ArrayList<>(answer.steps());
        steps.add(records < 0 ? "The split cannot make records of this answer." : "The split makes " + records + " record" + (records == 1 ? "" : "s") + " of it.");
        boolean results = answer.hasResults() != null ? answer.hasResults() : records > 0;
        yield result(
            id,
            results ? ConnectionTest.Outcome.RESULTS : ConnectionTest.Outcome.NO_RESULTS,
            steps,
            records < 0 ? null : records,
            kept(ingestion, answer.body()));
      }
    };
  }

  private static ConnectionTest result(String id, ConnectionTest.Outcome outcome, List<String> steps, @Nullable Long records, @Nullable String excerpt) {
    return new ConnectionTest(id, outcome, steps, records, excerpt);
  }

  /** The start of the answer, only when the ingestion keeps payloads: it is the client's data. */
  private static @Nullable String kept(Ingestion ingestion, @Nullable String body) {
    if (!ingestion.keepFailedPayloads() || body == null || body.isEmpty()) {
      return null;
    }
    return body.length() > ExchangeCounters.EXCERPT ? body.substring(0, ExchangeCounters.EXCERPT) : body;
  }
}
