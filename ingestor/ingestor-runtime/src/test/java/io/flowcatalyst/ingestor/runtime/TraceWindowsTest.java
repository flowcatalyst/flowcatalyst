package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.ingestor.contract.TraceRecord;
import io.flowcatalyst.ingestor.contract.TraceWindow;
import io.flowcatalyst.ingestor.extension.PollTrace;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import org.junit.jupiter.api.Test;

/** When a runtime captures: the window it counts from pickup, the hard stop, and the sample interval. */
class TraceWindowsTest {

  static final Instant T0 = Instant.parse("2026-10-30T10:00:00Z");

  final TestClock clock = new TestClock(T0);
  final List<TraceRecord> shipped = new ArrayList<>();
  final TraceWindows windows = new TraceWindows(clock, shipped::add, id -> List.of());
  final Ingestion ingestion = SourceHarness.ingestion("ing_1", Source.Custom.of("ticker", Map.of()), new Split.None());

  private TraceWindow window(String id, int secs, int every, Duration hardStopAfter) {
    return new TraceWindow("ing_1", id, secs, every, clock.instant().plus(hardStopAfter));
  }

  @Test
  void withoutAWindowThereIsNoCapture() {
    assertThat(windows.begin(ingestion)).isNull();
    assertThat(windows.active("ing_1")).isFalse();
  }

  @Test
  void aWindowFirstSeenCountsItsSecondsFromPickupNotFromWhenItWasSwitchedOn() {
    // The platform's hard stop is far away; the runtime captures for windowSecs from when it saw the window.
    windows.update(List.of(window("itw_1", 60, 1, Duration.ofMinutes(10))));

    assertThat(windows.begin(ingestion)).isNotNull();
    clock.advance(Duration.ofSeconds(59));
    assertThat(windows.begin(ingestion)).isNotNull();
    clock.advance(Duration.ofSeconds(1));
    assertThat(windows.begin(ingestion)).isNull();
    assertThat(windows.active("ing_1")).isFalse();
  }

  @Test
  void theHardStopCapsTheWindow() {
    windows.update(List.of(window("itw_1", 900, 1, Duration.ofSeconds(20))));

    clock.advance(Duration.ofSeconds(19));
    assertThat(windows.begin(ingestion)).isNotNull();
    clock.advance(Duration.ofSeconds(1));
    assertThat(windows.begin(ingestion)).isNull();
  }

  @Test
  void theSameWindowIdSeenAgainChangesNothing() {
    windows.update(List.of(window("itw_1", 60, 1, Duration.ofMinutes(10))));
    clock.advance(Duration.ofSeconds(30));
    // A later sync lists it again, with a hard stop later still: the local end stays where it was.
    windows.update(List.of(window("itw_1", 60, 1, Duration.ofMinutes(10))));

    clock.advance(Duration.ofSeconds(31));

    assertThat(windows.begin(ingestion)).isNull();
  }

  @Test
  void aDifferentWindowIdIsANewStartAndForgetsTheSampleInterval() {
    windows.update(List.of(window("itw_1", 60, 60, Duration.ofMinutes(10))));
    Capture first = windows.begin(ingestion);
    clock.advance(Duration.ofSeconds(50));
    assertThat(windows.begin(ingestion)).as("sampled out").isNull();

    windows.update(List.of(window("itw_2", 60, 60, Duration.ofMinutes(10))));
    Capture second = windows.begin(ingestion);
    clock.advance(Duration.ofSeconds(100));

    assertThat(first).isNotNull();
    assertThat(first.windowId()).isEqualTo("itw_1");
    assertThat(second).isNotNull();
    assertThat(second.windowId()).isEqualTo("itw_2");
    // The new window ends 60 s after it was first seen.
    assertThat(windows.begin(ingestion)).isNull();
  }

  @Test
  void anIngestionNoLongerListedHasNoWindow() {
    windows.update(List.of(window("itw_1", 60, 1, Duration.ofMinutes(10))));
    assertThat(windows.begin(ingestion)).isNotNull();

    windows.update(List.of());

    clock.advance(Duration.ofSeconds(2));
    assertThat(windows.begin(ingestion)).isNull();
  }

  @Test
  void aCaptureStartsOnlyWhenTheLastOneStartedTheSampleIntervalAgo() {
    windows.update(List.of(window("itw_1", 900, 5, Duration.ofMinutes(20))));

    Capture a = windows.begin(ingestion);
    clock.advance(Duration.ofSeconds(4));
    Capture tooSoon = windows.begin(ingestion);
    clock.advance(Duration.ofSeconds(1));
    Capture b = windows.begin(ingestion);

    assertThat(a).isNotNull();
    assertThat(tooSoon).isNull();
    assertThat(b).isNotNull();
    assertThat(b.id()).isNotEqualTo(a.id());
  }

  @Test
  void aPollIsCountedWhetherOrNotTracingIsOnAndOnlyASampledPollRecords() throws Exception {
    ExchangeCounters counters = new ExchangeCounters(clock);

    // Off: no capture bound, but the poll is counted.
    PollTrace off = windows.pollStarting(ingestion, counters);
    boolean[] boundOff = new boolean[1];
    off.run(
        () -> {
          boundOff[0] = Capture.CURRENT.isBound();
          return null;
        });
    off.finished(true, null, null);
    assertThat(boundOff[0]).isFalse();
    assertThat(shipped).isEmpty();

    // On: the first poll is sampled, the next one (too soon) is bound to a discarding capture.
    windows.update(List.of(window("itw_1", 900, 5, Duration.ofMinutes(20))));
    PollTrace sampled = windows.pollStarting(ingestion, counters);
    boolean[] recording = new boolean[1];
    sampled.run(
        () -> {
          recording[0] = Capture.CURRENT.get().recording();
          return null;
        });
    sampled.finished(true, null, Duration.ofSeconds(60));
    PollTrace passedOver = windows.pollStarting(ingestion, counters);
    boolean[] discarding = new boolean[1];
    passedOver.run(
        () -> {
          discarding[0] = Capture.CURRENT.isBound() && !Capture.CURRENT.get().recording();
          return null;
        });
    passedOver.finished(true, null, null);

    assertThat(recording[0]).isTrue();
    assertThat(discarding[0]).isTrue();
    assertThat(shipped).singleElement().satisfies(r -> assertThat(r.kind()).isEqualTo("poll"));
    assertThat(counters.drain(clock.instant()).windows()).singleElement().satisfies(w -> {
      assertThat(w.polls()).isEqualTo(3);
      assertThat(w.lastPollAt()).isEqualTo(T0);
    });
  }
}
