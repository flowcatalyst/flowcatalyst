package io.flowcatalyst.ingestor.runtime;

import java.time.Duration;
import java.util.Optional;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * Ends the runtime when the process that started it is gone. fcdev sets INGEST_PARENT_PID to its own
 * process id: on Linux the kernel ends the runtime with fcdev, but elsewhere (macOS) a runtime would
 * outlive an fcdev that is killed hard, holding its port for the next one. A deployed runtime never
 * has it set, and nothing is watched.
 */
final class ParentWatch {

  private static final Logger LOG = LoggerFactory.getLogger(ParentWatch.class);

  static final String ENV = "INGEST_PARENT_PID";
  private static final Duration EVERY = Duration.ofSeconds(2);

  private ParentWatch() {}

  /**
   * Watches the process INGEST_PARENT_PID names, when it is set, and exits (running the shutdown
   * hooks, so the runtime stops as on SIGTERM) once that process has ended.
   */
  static void start() {
    @Nullable String value = System.getenv(ENV);
    if (value == null || value.isBlank()) {
      return;
    }
    long pid;
    try {
      pid = Long.parseLong(value.trim());
    } catch (NumberFormatException e) {
      LOG.warn("{} is not a process id ('{}'); not watching a parent", ENV, value);
      return;
    }
    // The handle carries the process's start time, so a reused id is not mistaken for the parent.
    Optional<ProcessHandle> parent = ProcessHandle.of(pid);
    if (parent.isEmpty() || !parent.get().isAlive()) {
      LOG.warn("the parent process {} is already gone; stopping", pid);
      System.exit(0);
    }
    ProcessHandle watched = parent.get();
    Thread.ofPlatform()
        .daemon()
        .name("parent-watch")
        .start(
            () -> {
              while (watched.isAlive()) {
                try {
                  Thread.sleep(EVERY);
                } catch (InterruptedException e) {
                  Thread.currentThread().interrupt();
                  return;
                }
              }
              LOG.warn("the parent process {} has ended; stopping", pid);
              System.exit(0);
            });
  }
}
