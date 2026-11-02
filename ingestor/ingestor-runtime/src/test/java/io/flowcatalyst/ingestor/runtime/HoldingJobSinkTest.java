package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.JobRecord;
import io.flowcatalyst.ingestor.contract.JobsAnswer;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.ingestor.extension.Backoff;
import io.flowcatalyst.ingestor.extension.FailureLog;
import java.io.IOException;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.atomic.AtomicInteger;
import org.apache.camel.CamelContext;
import org.apache.camel.ProducerTemplate;
import org.apache.camel.impl.DefaultCamelContext;
import org.jspecify.annotations.Nullable;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import java.util.concurrent.TimeUnit;

/** The runtime-wide hold on every ingestion while the platform is not taking jobs (docs/ingestor-failure-pacing.md, R2). */
class HoldingJobSinkTest {

  private record Line(String message, @Nullable Throwable cause) {}

  private final TestClock clock = new TestClock(Instant.parse("2026-11-02T10:00:00Z"));
  private final List<Line> lines = new ArrayList<>();
  private final List<String> closedLines = new ArrayList<>();
  private final FailureLog failures = new FailureLog((m, c) -> lines.add(new Line(m, c)), Duration.ofMinutes(1), clock);
  private final AtomicInteger platformCalls = new AtomicInteger();
  private volatile @Nullable IOException platform;

  private final JobSink delegate =
      (ingestion, jobs) -> {
        platformCalls.incrementAndGet();
        IOException down = platform;
        if (down != null) {
          throw down;
        }
      };
  private final HoldingJobSink sink = new HoldingJobSink(delegate, clock, Backoff.FAILURES, failures, closedLines::add);

  private static final PlatformUnavailableException DOWN = new PlatformUnavailableException("POST /jobs answered 503", 503, null, null);

  private void batch(Ingestion ingestion) throws Exception {
    try {
      sink.admit();
      sink.submit(ingestion, List.of());
      sink.settled(null);
    } catch (PlatformHeldException e) {
      throw e;
    } catch (Exception e) {
      sink.settled(e);
      throw e;
    }
  }

  private Ingestion ingestion(String id) {
    return SourceHarness.ingestion(id, new Source.Sqs("eu-west-1", "q", null), new Split.None());
  }

  @Test
  void whenThePlatformFailsEveryIngestionIsHeldAndOnlyOneProbeCallIsMadePerHold() throws Exception {
    platform = DOWN;
    assertThatThrownBy(() -> batch(ingestion("a"))).isSameAs(DOWN);
    int callsThatOpenedIt = platformCalls.get();

    for (String id : List.of("b", "c", "d", "e", "f")) {
      assertThatThrownBy(() -> batch(ingestion(id))).isInstanceOf(PlatformHeldException.class).hasMessageContaining("held until");
    }
    assertThat(platformCalls.get()).isEqualTo(callsThatOpenedIt);

    // After the hold the first caller is the probe; the rest stay held until it ends.
    clock.advance(Backoff.FAILURES.ceiling(1).plusSeconds(1));
    sink.admit();
    assertThatThrownBy(sink::admit).isInstanceOf(PlatformHeldException.class);
    sink.settled(DOWN);
    // The probe failed: held again, for longer.
    assertThatThrownBy(sink::admit).isInstanceOf(PlatformHeldException.class);
    clock.advance(Backoff.FAILURES.ceiling(2).plusSeconds(1));
    sink.admit();
  }

  @Test
  void aRetryAfterLongerThanTheBackoffIsHonouredUpToFifteenMinutes() throws Exception {
    sink.settled(new PlatformUnavailableException("429", 429, Duration.ofMinutes(10), null));
    clock.advance(Duration.ofMinutes(9));
    assertThatThrownBy(sink::admit).isInstanceOf(PlatformHeldException.class);
    clock.advance(Duration.ofMinutes(1).plusSeconds(1));
    sink.admit();
    sink.settled(null);

    sink.settled(new PlatformUnavailableException("503", 503, Duration.ofHours(6), null));
    clock.advance(Duration.ofMinutes(14));
    assertThatThrownBy(sink::admit).isInstanceOf(PlatformHeldException.class);
    clock.advance(Duration.ofMinutes(1).plusSeconds(1));
    sink.admit();
  }

  @Test
  void aShorterRetryAfterDoesNotShortenTheBackoff() throws Exception {
    sink.settled(new PlatformUnavailableException("429", 429, Duration.ofMillis(1), null));
    clock.advance(Duration.ofMillis(2400));
    assertThatThrownBy(sink::admit).isInstanceOf(PlatformHeldException.class);
  }

  @Test
  void aFailureThatIsTheIngestionsDoesNotHoldAnyIngestion() throws Exception {
    platform = new IOException("POST /jobs answered 409: the target is paused");
    assertThatThrownBy(() -> batch(ingestion("paused"))).hasMessageContaining("409");
    assertThatThrownBy(() -> batch(ingestion("paused"))).hasMessageContaining("409");

    platform = null;
    batch(ingestion("other"));

    assertThat(platformCalls.get()).isEqualTo(3);
    assertThat(lines).isEmpty();
  }

  @Test
  void aRefusalIsTheIngestionsAndNeverOpensItButClosesAnOpenOneBecauseThePlatformAnswered() throws Exception {
    sink.settled(DOWN);
    clock.advance(Duration.ofMinutes(10));
    sink.admit(); // the probe
    sink.settled(new RejectedException("POST /jobs answered 400", 400));

    sink.admit();
    sink.admit();
    assertThat(closedLines).hasSize(1);
  }

  @Test
  void theHoldClosesOnTheFirstSuccessAndOpeningAndClosingAreEachLoggedOnce() throws Exception {
    // The platform asked for ten minutes, so the hold outlasts the minute the log is checked at.
    platform = new PlatformUnavailableException("POST /jobs answered 503", 503, Duration.ofMinutes(10), null);
    assertThatThrownBy(() -> batch(ingestion("a"))).isSameAs(platform);
    for (int i = 0; i < 50; i++) {
      assertThatThrownBy(() -> batch(ingestion("b"))).isInstanceOf(PlatformHeldException.class);
    }
    // One WARN when it opened, nothing more inside the minute.
    assertThat(lines).hasSize(1);
    assertThat(lines.getFirst().message()).contains("holding every ingestion until").contains("503");
    assertThat(lines.getFirst().cause()).isNull();

    clock.advance(Duration.ofMinutes(1));
    assertThatThrownBy(() -> batch(ingestion("b"))).isInstanceOf(PlatformHeldException.class);
    // A line a minute while it stays open.
    assertThat(lines).hasSize(2);
    assertThat(lines.getLast().message()).contains("still not taking jobs");

    platform = null;
    clock.advance(Duration.ofMinutes(10));
    batch(ingestion("c")); // the probe succeeds
    batch(ingestion("d"));
    assertThat(closedLines).singleElement().asString().contains("takes jobs again");
    assertThat(lines).hasSize(2);
  }

  @Test
  void aFailureThatArrivesWhileItIsAlreadyHeldDoesNotLengthenTheHold() throws Exception {
    sink.settled(DOWN);
    for (int i = 0; i < 20; i++) {
      sink.settled(DOWN); // batches that began before it opened
    }
    clock.advance(Backoff.FAILURES.ceiling(1).plusSeconds(1));
    sink.admit();
  }

  /** The route asks before each attempt and tells how the batch ended: through real routes, the platform is called for the first batch's attempts only. */
  @Test
  @Timeout(value = 60, threadMode = Timeout.ThreadMode.SEPARATE_THREAD)
  void throughTheRouteFiveIngestionsMakeTheFirstOnesAttemptsAndNoOtherCallWhileHeld() throws Exception {
    platform = DOWN;
    CamelContext camel = new DefaultCamelContext();
    camel.start();
    try {
      ExchangeCounters counters = new ExchangeCounters(clock);
      Reconciler reconciler = new Reconciler(camel, i -> new IngestionRoute(i, (c, x) -> c.getEndpoint("direct:" + x.id()), sink, counters));
      List<Ingestion> all = new ArrayList<>();
      for (int i = 0; i < 5; i++) {
        all.add(SourceHarness.ingestion("ing_" + i, Source.Custom.of("direct", Map.of()), new Split.None()));
      }
      reconciler.reconcile(all);
      ProducerTemplate producer = camel.createProducerTemplate();

      for (Ingestion each : all) {
        var exchange = producer.send("direct:" + each.id(), e -> e.getMessage().setBody("{\"a\":1}"));
        assertThat(exchange.isFailed()).isTrue();
      }

      assertThat(platformCalls.get()).as("the first batch's %d attempts, nothing for the other four", IngestionRoute.ATTEMPTS).isEqualTo(IngestionRoute.ATTEMPTS);
    } finally {
      camel.stop();
    }
  }
}
