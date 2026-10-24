package io.flowcatalyst.ingestor.runtime;

import static io.flowcatalyst.ingestor.runtime.SourceHarness.await;
import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.flowcatalyst.ingestor.contract.ConnectionTest;
import io.flowcatalyst.ingestor.contract.ExtensionKind;
import io.flowcatalyst.ingestor.contract.RuntimeKind;
import io.flowcatalyst.ingestor.contract.RuntimeReport.RouteState;
import io.flowcatalyst.ingestor.contract.JobRecord;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.SourceKind;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.ingestor.contract.TestRequest;
import io.flowcatalyst.ingestor.extension.ExtensionContext;
import io.flowcatalyst.ingestor.extension.IngestExtension;
import io.flowcatalyst.framework.config.Secret;
import java.io.IOException;
import java.time.Instant;
import java.time.InstantSource;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicReference;
import java.util.function.Supplier;
import org.apache.camel.CamelContext;
import org.apache.camel.Endpoint;
import org.apache.camel.impl.DefaultCamelContext;
import org.jspecify.annotations.Nullable;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;

/** Extensions: found on the class path, checked at start, and run as custom sources. */
@Timeout(60)
class ExtensionsTest {

  @Test
  void theRuntimeFindsTheExtensionsBuiltIntoItAndReportsTheirKinds() {
    assertThat(Extensions.load(null, false).kinds()).extracting(RuntimeKind::kind).containsExactly("generator");
  }

  @Test
  void aGeneratorIngestionRunsLikeAnyOtherAndNumbersOnFromWhereItLeftOff() throws Exception {
    List<List<JobRecord>> batches = new CopyOnWriteArrayList<>();
    MemoryState state = new MemoryState();
    // As if another runtime had run it up to record 41.
    state.values.put("ing_gen/next", "42");
    CamelContext camel = new DefaultCamelContext();
    camel.start();
    try {
      SourceEndpoints endpoints = new CamelSourceEndpoints(SourceHarness.secrets(Map.of()), state, null, null, Extensions.load(null, false));
      ExchangeCounters counters = new ExchangeCounters(InstantSource.system());
      Reconciler reconciler = new Reconciler(camel, i -> new IngestionRoute(i, endpoints, (ingestion, jobs) -> batches.add(jobs), counters));
      Source.Custom generator = Source.Custom.of("generator", Map.of("every", "PT1S", "count", 3, "template", "{\"n\":${n}}"));
      reconciler.reconcile(List.of(SourceHarness.ingestion("ing_gen", generator, new Split.Json("$"))));

      await(() -> batches.stream().mapToLong(List::size).sum() >= 3);

      assertThat(batches.stream().flatMap(List::stream).map(JobRecord::payload)).startsWith("{\"n\":42}", "{\"n\":43}", "{\"n\":44}");
      await(() -> Long.parseLong(state.values.get("ing_gen/next")) >= 45);
      assertThat(reconciler.states()).singleElement().extracting(RouteState::status).isEqualTo(RouteState.Status.STARTED);
    } finally {
      camel.stop();
    }
  }

  @Test
  void settingsTheKindRefusesAndATypeNoExtensionHasLeaveTheIngestionFailedWithWhy() {
    CamelContext camel = new DefaultCamelContext();
    camel.start();
    try {
      SourceEndpoints endpoints =
          new CamelSourceEndpoints(SourceHarness.secrets(Map.of()), new MemoryState(), null, null, Extensions.load(null, false));
      ExchangeCounters counters = new ExchangeCounters(InstantSource.system());
      Reconciler reconciler = new Reconciler(camel, i -> new IngestionRoute(i, endpoints, (ingestion, jobs) -> {}, counters));
      reconciler.reconcile(
          List.of(
              SourceHarness.ingestion("ing_a", Source.Custom.of("generator", Map.of("every", "soon", "colour", "red")), new Split.Json("$")),
              SourceHarness.ingestion("ing_b", Source.Custom.of("acme-orders", Map.of()), new Split.None()),
              SourceHarness.ingestion("ing_c", Source.Custom.of("generator", Map.of("every", "PT1S", "count", 5000)), new Split.Json("$"))));

      assertThat(reconciler.states()).extracting(RouteState::status).containsOnly(RouteState.Status.FAILED);
      assertThat(reconciler.states()).extracting(RouteState::error).containsExactly(
          "the settings of generator cannot be used: colour is not a setting of generator; every must be a duration such as PT30S",
          "this runtime has no extension for the source type 'acme-orders'",
          "the settings of generator cannot be used: count must be 1 to 1000");
    } finally {
      camel.stop();
    }
  }

  @Test
  void anExtensionReadsItsSecretsForTheIngestionsOwnClientOnly() {
    AtomicReference<String> seen = new AtomicReference<>();
    IngestExtension reader =
        new Recording(
            () -> new SourceKind("secretive", "Secretive", "Reads a secret.", List.of(new SourceKind.Field("key", "Key", null, SourceKind.Type.SECRET, true))),
            context -> seen.set(context.settings().secret("key").map(Secret::reveal).orElse("none")));
    CamelContext camel = new DefaultCamelContext();
    camel.start();
    try {
      Extensions extensions = Extensions.of(List.of(reader));
      SourceEndpoints endpoints = new CamelSourceEndpoints(SourceHarness.secrets(Map.of("api-key", "k3y")), new MemoryState(), null, null, extensions);
      Reconciler reconciler = new Reconciler(camel, i -> new IngestionRoute(i, endpoints, (ingestion, jobs) -> {}, new ExchangeCounters(InstantSource.system())));
      reconciler.reconcile(List.of(SourceHarness.ingestion("ing_s", Source.Custom.of("secretive", Map.of("key", "api-key")), new Split.None())));

      assertThat(seen).hasValue("k3y");
      assertThat(reconciler.states()).singleElement().extracting(RouteState::status).isEqualTo(RouteState.Status.STARTED);

      reconciler.reconcile(List.of(SourceHarness.ingestion("ing_t", Source.Custom.of("secretive", Map.of("key", "other-key")), new Split.None())));
      assertThat(reconciler.states()).singleElement().extracting(RouteState::error).asString().contains("secret 'other-key' is not set for client acme");
    } finally {
      camel.stop();
    }
  }

  @Test
  void aBrokenImageDoesNotStart() {
    assertThatThrownBy(() -> Extensions.of(List.of(new Recording(() -> new SourceKind("sftp", "Mine", "", List.of()), c -> {}))))
        .isInstanceOf(IllegalStateException.class)
        .hasMessageContaining("not a built-in kind");

    Supplier<SourceKind> twice = () -> new SourceKind("twice", "Twice", "", List.of());
    assertThatThrownBy(() -> Extensions.of(List.of(new Recording(twice, c -> {}), new Recording(twice, c -> {}))))
        .hasMessageContaining("is taken by");

    Supplier<SourceKind> badDefault =
        () -> new SourceKind("bad", "Bad", "", List.of(SourceKind.field("every", "Every", SourceKind.Type.DURATION).required().defaultValue("often").build()));
    assertThatThrownBy(() -> Extensions.of(List.of(new Recording(badDefault, c -> {}))))
        .hasMessageContaining("the default of 'every' must be a duration");
  }

  @Test
  void testConnectionRunsTheExtensionsTestAndKeepsItsStepsBeforeAFailure() {
    IngestExtension failing =
        new Recording(() -> new SourceKind("tried", "Tried", "", List.of()), c -> {}).withTest(
            (context, steps) -> {
              steps.add("Logged in.");
              throw new IOException("the server said no");
            });
    CamelContext camel = new DefaultCamelContext();
    camel.start();
    try {
      TestRunner probe =
          new TestRunner(SourceHarness.secrets(Map.of()), null, Extensions.of(List.of(failing, new Recording(() -> new SourceKind("untried", "Untried", "", List.of()), c -> {}))), camel, System::nanoTime);

      ConnectionTest failed = probe.apply(new TestRequest("tst_ing_t", Instant.now().plusSeconds(60), SourceHarness.ingestion("ing_t", Source.Custom.of("tried", Map.of()), new Split.None())));
      assertThat(failed.outcome()).isEqualTo(ConnectionTest.Outcome.FAILED);
      assertThat(failed.steps()).containsExactly("Logged in.", "the server said no.");

      ConnectionTest none = probe.apply(new TestRequest("tst_ing_u", Instant.now().plusSeconds(60), SourceHarness.ingestion("ing_u", Source.Custom.of("untried", Map.of()), new Split.None())));
      assertThat(none.outcome()).isEqualTo(ConnectionTest.Outcome.NOT_SUPPORTED);
      assertThat(none.steps()).containsExactly("The untried source offers no test.");

      ConnectionTest refused = probe.apply(new TestRequest("tst_ing_v", Instant.now().plusSeconds(60), SourceHarness.ingestion("ing_v", Source.Custom.of("tried", Map.of("colour", "red")), new Split.None())));
      assertThat(refused.outcome()).isEqualTo(ConnectionTest.Outcome.FAILED);
      assertThat(refused.steps()).containsExactly("the settings of tried cannot be used: colour is not a setting of tried.");
    } finally {
      camel.stop();
    }
  }

  /** An extension that runs a check on its context and makes an endpoint nothing sends to; it may offer a test. */
  private record Recording(Supplier<SourceKind> source, Check check, @Nullable Tester tester) implements IngestExtension {

    Recording(Supplier<SourceKind> source, Check check) {
      this(source, check, null);
    }

    interface Check {
      void on(ExtensionContext context) throws IOException;
    }

    interface Tester {
      Tested on(ExtensionContext context, List<String> steps) throws Exception;
    }

    Recording withTest(Tester test) {
      return new Recording(source, check, test);
    }

    @Override
    public ExtensionKind kind() {
      return source::get;
    }

    @Override
    public Endpoint endpoint(ExtensionContext context) throws Exception {
      check.on(context);
      return context.camel().getEndpoint("direct:never-" + context.ingestion().id());
    }

    @Override
    public @Nullable Tested test(ExtensionContext context, List<String> steps) throws Exception {
      return tester == null ? null : tester.on(context, steps);
    }
  }
}
