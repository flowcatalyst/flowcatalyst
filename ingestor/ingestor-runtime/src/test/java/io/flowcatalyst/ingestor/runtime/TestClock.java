package io.flowcatalyst.ingestor.runtime;

import java.time.Duration;
import java.time.Instant;
import java.time.InstantSource;

/** A clock a test moves by hand. */
final class TestClock implements InstantSource {

  private volatile Instant now;

  TestClock(Instant start) {
    this.now = start;
  }

  @Override
  public Instant instant() {
    return now;
  }

  void advance(Duration by) {
    now = now.plus(by);
  }
}
