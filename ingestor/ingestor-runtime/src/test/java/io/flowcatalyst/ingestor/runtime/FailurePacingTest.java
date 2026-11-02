package io.flowcatalyst.ingestor.runtime;

import static io.flowcatalyst.ingestor.runtime.SourceHarness.await;
import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.Failure;
import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.ingestor.extension.FailureLog;
import java.time.Duration;
import java.time.Instant;
import java.time.InstantSource;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.TimeUnit;
import org.apache.camel.CamelContext;
import org.apache.camel.builder.RouteBuilder;
import org.apache.camel.component.aws2.sqs.Sqs2Endpoint;
import org.apache.camel.component.file.remote.SftpEndpoint;
import org.apache.camel.impl.DefaultCamelContext;
import org.jspecify.annotations.Nullable;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;

/** Source configuration and routes that follow docs/ingestor-failure-pacing.md (R1, R3). */
@Timeout(value = 60, threadMode = Timeout.ThreadMode.SEPARATE_THREAD)
class FailurePacingTest {

  private record Line(String message, @Nullable Throwable cause) {}

  private static Source.Sftp sftp(String host, int port) {
    return new Source.Sftp(host, port, "in", "u", new SecretRef("sftp"), null, null, Duration.ofSeconds(1), true);
  }

  @Test
  void anSqsSourceLongPollsForTwentySecondsAndStillPausesAHundredMillisecondsBetweenReceives() throws Exception {
    CamelContext camel = new DefaultCamelContext();
    camel.start();
    try {
      CamelSourceEndpoints endpoints = new CamelSourceEndpoints(SourceHarness.secrets(Map.of()), new MemoryState(), null);
      Ingestion ingestion = SourceHarness.ingestion("ing_q", new Source.Sqs("eu-west-1", "orders", null), new Split.None());

      Sqs2Endpoint endpoint = endpoints.sqsEndpoint(camel, ingestion, (Source.Sqs) ingestion.source());

      assertThat(endpoint.getConfiguration().getWaitTimeSeconds()).isEqualTo(20);
      assertThat(endpoint.getDelay()).isEqualTo(100);
    } finally {
      camel.stop();
    }
  }

  @Test
  void anSftpSourceFailsAPollWhenTheLoginIsRefusedOrTheHostIsDown() throws Exception {
    CamelContext camel = new DefaultCamelContext();
    camel.start();
    try {
      CamelSourceEndpoints endpoints = new CamelSourceEndpoints(SourceHarness.secrets(Map.of("sftp", "pw")), new MemoryState(), null);
      Ingestion ingestion = SourceHarness.ingestion("ing_f", sftp("files.example", 22), new Split.None());

      SftpEndpoint endpoint = (SftpEndpoint) endpoints.endpoint(camel, ingestion);

      assertThat(endpoint.getConfiguration().isThrowExceptionOnConnectFailed()).isTrue();
    } finally {
      camel.stop();
    }
  }

  @Test
  void anSftpHostThatIsDownShowsAsFailingIsReportedOnceAndIsNotPolledAtItsIntervalMeanwhile() throws Exception {
    try (SourceHarness harness = new SourceHarness(SourceHarness.secrets(Map.of("sftp", "pw")), null)) {
      harness.reconciler.reconcile(List.of(SourceHarness.ingestion("ing_down", sftp("127.0.0.1", 1), new Split.None())));

      // The poll fails (the connection is refused) and is reported; without the setting Camel would skip it silently.
      List<Failure> first = new ArrayList<>();
      await(() -> first.addAll(harness.counters.drain(InstantSource.system().instant()).failures()) && !first.isEmpty());
      assertThat(first).allSatisfy(f -> assertThat(f.kind()).isEqualTo(Failure.Kind.SOURCE));
      // At its 1 s interval it would fail twice more in this time; after one failure it waits 2.5 s at least.
      TimeUnit.MILLISECONDS.sleep(1800);
      assertThat(harness.counters.drain(InstantSource.system().instant()).failures()).isEmpty();
    }
  }

  @Test
  void aRouteThatKeepsFailingToStartIsLoggedInFullOnceThenSummarisedAndSaysWhenItStarts() {
    TestClock clock = new TestClock(Instant.parse("2026-11-02T10:00:00Z"));
    List<Line> lines = new ArrayList<>();
    FailureLog failures = new FailureLog((m, c) -> lines.add(new Line(m, c)), Duration.ofMinutes(1), clock);
    CamelContext camel = new DefaultCamelContext();
    boolean[] works = {false};
    Reconciler reconciler =
        new Reconciler(
            camel,
            i -> {
              if (!works[0]) {
                throw new IllegalStateException("cannot build the route");
              }
              return new RouteBuilder() {
                @Override
                public void configure() {
                  from("direct:" + i.id()).routeId(i.id()).to("log:none");
                }
              };
            },
            new SourceHealth(),
            id -> {},
            failures);
    Ingestion ingestion = SourceHarness.ingestion("ing_start", new Source.Sqs("eu-west-1", "q", null), new Split.None());

    for (int i = 0; i < 600; i++) {
      reconciler.reconcile(List.of(ingestion));
      clock.advance(Duration.ofMillis(100));
    }
    reconciler.reconcile(List.of(ingestion));

    assertThat(lines).hasSize(2);
    assertThat(lines.get(0).cause()).isInstanceOf(IllegalStateException.class);
    assertThat(lines.get(0).message()).contains("ing_start version 1 failed to start");
    assertThat(lines.get(1).cause()).isNull();
    assertThat(failures.streak("ing_start:start")).isEqualTo(601);

    works[0] = true;
    try {
      camel.start();
      reconciler.reconcile(List.of(ingestion));
    } finally {
      camel.stop();
    }
    assertThat(failures.streak("ing_start:start")).isZero();
  }
}
