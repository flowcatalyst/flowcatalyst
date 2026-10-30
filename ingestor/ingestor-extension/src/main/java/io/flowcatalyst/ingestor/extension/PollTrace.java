package io.flowcatalyst.ingestor.extension;

import java.time.Duration;
import java.util.concurrent.Callable;
import org.jspecify.annotations.Nullable;

/**
 * One poll as the runtime watches it: {@link PolledSource} asks its {@link PolledSource.Reporter}
 * for one before each poll, runs the poll inside {@link #run(Callable)} and says how it ended with
 * {@link #finished(boolean, String, Duration)}. When an operator has switched tracing on for the
 * ingestion, the runtime binds what the poll's HTTP calls and exchanges are recorded into around the
 * poll; otherwise {@link #run(Callable)} just runs it. Either way the runtime counts the poll. An
 * extension never implements or calls this: it is the seam between the runtime-provided {@code
 * PolledSource} and the runtime, which the extension API cannot name.
 */
public interface PollTrace {

  /** A poll nobody watches: runs it as it is and counts nothing. */
  PollTrace NONE =
      new PollTrace() {
        @Override
        public <T> T run(Callable<T> poll) throws Exception {
          return poll.call();
        }

        @Override
        public void finished(boolean ok, @Nullable String error, @Nullable Duration next) {}
      };

  /**
   * Runs the poll with whatever the runtime records into bound around it.
   *
   * @param poll the poll
   * @param <T> what it returns
   * @return what the poll returned
   * @throws Exception whatever the poll threw, unchanged
   */
  <T> T run(Callable<T> poll) throws Exception;

  /**
   * The poll has ended; called once, on the polling thread, whether or not it threw.
   *
   * @param ok false when it threw or reported a failure of one of its parts
   * @param error why it failed, without a secret, or null
   * @param next how soon it will be polled again, or null when not known
   */
  void finished(boolean ok, @Nullable String error, @Nullable Duration next);
}
