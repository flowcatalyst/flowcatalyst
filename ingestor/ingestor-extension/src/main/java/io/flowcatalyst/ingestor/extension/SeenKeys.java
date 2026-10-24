package io.flowcatalyst.ingestor.extension;

import java.time.Duration;
import java.time.Instant;
import java.time.InstantSource;
import java.util.Iterator;
import java.util.LinkedHashMap;
import java.util.Map;

/**
 * What a poll has dispatched lately, by key, so a record read again (an overlapping time range, a
 * page asked for twice) is not dispatched twice. Held in memory by the runtime that runs the
 * ingestion: on another runtime, after a move, it starts empty. A key is kept for {@code keep}, and
 * at most {@code most} keys are kept, the oldest dropped first.
 */
public final class SeenKeys {

  private final Duration keep;
  private final int most;
  private final InstantSource clock;
  // Insertion order is age order: a key is added once, when first seen.
  private final LinkedHashMap<String, Instant> keys = new LinkedHashMap<>();

  /**
   * An empty set.
   *
   * @param keep how long a key is kept
   * @param most the most keys kept
   * @param clock the time
   */
  public SeenKeys(Duration keep, int most, InstantSource clock) {
    this.keep = keep;
    this.most = most;
    this.clock = clock;
  }

  /**
   * Whether the key was dispatched within {@code keep}.
   *
   * @param key the key
   * @return whether it is held
   */
  public synchronized boolean contains(String key) {
    forgetOld();
    return keys.containsKey(key);
  }

  /**
   * Remembers that the key was dispatched.
   *
   * @param key the key
   */
  public synchronized void add(String key) {
    if (!keys.containsKey(key)) {
      keys.put(key, clock.instant());
    }
    forgetOld();
  }

  /**
   * How many keys are held.
   *
   * @return the count
   */
  public synchronized int size() {
    forgetOld();
    return keys.size();
  }

  private void forgetOld() {
    Instant oldest = clock.instant().minus(keep);
    Iterator<Map.Entry<String, Instant>> it = keys.entrySet().iterator();
    while (it.hasNext()) {
      Map.Entry<String, Instant> next = it.next();
      if (keys.size() > most || next.getValue().isBefore(oldest)) {
        it.remove();
      } else {
        break;
      }
    }
  }
}
