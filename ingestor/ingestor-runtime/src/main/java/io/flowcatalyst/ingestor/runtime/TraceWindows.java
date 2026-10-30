package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.framework.httpclient.HttpCapture;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.TraceWindow;
import io.flowcatalyst.ingestor.extension.PollTrace;
import java.time.Duration;
import java.time.Instant;
import java.time.InstantSource;
import java.util.Collection;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.function.Function;
import org.jspecify.annotations.Nullable;

/**
 * The trace windows this runtime has been told of, and the sampler that decides when a capture starts.
 * The platform lists the windows in each poll answer ({@link #update(List)}); the runtime counts a
 * window from when it first sees it. With no window for an ingestion, asking for a capture is one map
 * lookup and nothing else.
 *
 * <p>The rules (the design's sections 2 and 9): a window id seen for the first time ends at the
 * earlier of {@code now + windowSecs} and the platform's hard stop; the same id seen again changes
 * nothing; another id for the ingestion replaces the window, a new start; an ingestion no longer listed
 * has no window. The local clock and the hard stop are compared on every use. A capture starts only when
 * the last one of the ingestion began {@code sampleEverySecs} or more ago.
 */
final class TraceWindows {

  /** The most a runtime captures for one window, whatever the platform lists. */
  private static final int MAX_WINDOW_SECS = 900;

  private static final class State {
    final String windowId;
    final Instant localUntil;
    final Instant hardStop;
    final Duration sampleEvery;
    /** Guarded by {@code this}. */
    @Nullable Instant lastCaptureStart;

    State(String windowId, Instant localUntil, Instant hardStop, Duration sampleEvery) {
      this.windowId = windowId;
      this.localUntil = localUntil;
      this.hardStop = hardStop;
      this.sampleEvery = sampleEvery;
    }
  }

  private final InstantSource clock;
  private final TraceSink sink;
  private final Function<String, Collection<String>> secrets;
  private final Map<String, State> windows = new ConcurrentHashMap<>();

  /**
   * Creates the store.
   *
   * @param clock the time, local and the platform's hard stops compared on it
   * @param sink where a capture's records go
   * @param secrets the secret values fetched for an ingestion, by its id, which records must not carry
   */
  TraceWindows(InstantSource clock, TraceSink sink, Function<String, Collection<String>> secrets) {
    this.clock = clock;
    this.sink = sink;
    this.secrets = secrets;
  }

  /** A store that never captures: tracing is not wired. */
  static TraceWindows disabled() {
    return new TraceWindows(InstantSource.system(), TraceSink.DISCARD, id -> List.of());
  }

  /**
   * Takes the windows of a poll answer: the whole list, so an ingestion not on it has none.
   *
   * @param listed the windows the platform lists
   */
  void update(List<TraceWindow> listed) {
    Instant now = clock.instant();
    java.util.Set<String> present = new java.util.HashSet<>();
    for (TraceWindow w : listed) {
      present.add(w.ingestionId());
      State known = windows.get(w.ingestionId());
      if (known != null && known.windowId.equals(w.windowId())) {
        continue;
      }
      long secs = Math.clamp(w.windowSecs(), 1, MAX_WINDOW_SECS);
      Instant until = now.plusSeconds(secs);
      if (w.hardStop().isBefore(until)) {
        until = w.hardStop();
      }
      windows.put(w.ingestionId(), new State(w.windowId(), until, w.hardStop(), Duration.ofSeconds(Math.max(1, w.sampleEverySecs()))));
    }
    windows.keySet().removeIf(id -> !present.contains(id));
  }

  /**
   * Whether tracing is on for the ingestion right now, ignoring the sample interval.
   *
   * @param ingestionId the ingestion
   * @return true while its window is live
   */
  boolean active(String ingestionId) {
    State state = windows.get(ingestionId);
    return state != null && live(state, clock.instant());
  }

  /**
   * Starts a capture if tracing is on for the ingestion and its last capture began long enough ago.
   *
   * @param ingestion the ingestion
   * @return the capture, or null
   */
  @Nullable Capture begin(Ingestion ingestion) {
    State state = windows.get(ingestion.id());
    if (state == null) {
      return null;
    }
    Instant now = clock.instant();
    if (!live(state, now)) {
      return null;
    }
    synchronized (state) {
      Instant last = state.lastCaptureStart;
      if (last != null && now.isBefore(last.plus(state.sampleEvery))) {
        return null;
      }
      state.lastCaptureStart = now;
    }
    return new Capture(ingestion, state.windowId, sink, clock, () -> secrets.apply(ingestion.id()));
  }

  /**
   * What a polled source's poll is told: counted always, recorded when sampled.
   *
   * @param ingestion the ingestion polled
   * @param counters where the poll is counted
   * @return the poll's trace
   */
  PollTrace pollStarting(Ingestion ingestion, ExchangeCounters counters) {
    Instant started = clock.instant();
    if (!active(ingestion.id())) {
      return new PollWatch(ingestion, counters, started, null);
    }
    Capture capture = begin(ingestion);
    // A poll the sampler passed over still binds a capture, so its payloads do not start their own.
    return new PollWatch(ingestion, counters, started, capture != null ? capture : Capture.discarding(ingestion));
  }

  private static boolean live(State state, Instant now) {
    return now.isBefore(state.localUntil) && now.isBefore(state.hardStop);
  }

  /** One poll: counted when it ends, recorded when it has a capture. */
  private static final class PollWatch implements PollTrace {

    private final Ingestion ingestion;
    private final ExchangeCounters counters;
    private final Instant started;
    private final @Nullable Capture capture;
    private boolean finished;

    PollWatch(Ingestion ingestion, ExchangeCounters counters, Instant started, @Nullable Capture capture) {
      this.ingestion = ingestion;
      this.counters = counters;
      this.started = started;
      this.capture = capture;
    }

    @Override
    public <T> T run(java.util.concurrent.Callable<T> poll) throws Exception {
      Capture bound = capture;
      if (bound == null) {
        return poll.call();
      }
      if (!bound.recording()) {
        return ScopedValue.where(Capture.CURRENT, bound).call(poll::call);
      }
      HttpCapture.Sink sink = bound::http;
      return ScopedValue.where(Capture.CURRENT, bound).where(HttpCapture.CURRENT, sink).call(poll::call);
    }

    @Override
    public void finished(boolean ok, @Nullable String error, @Nullable Duration next) {
      if (finished) {
        return;
      }
      finished = true;
      counters.polled(ingestion);
      Capture ended = capture;
      if (ended != null) {
        ended.poll(started, ok, error, next);
      }
    }
  }
}
