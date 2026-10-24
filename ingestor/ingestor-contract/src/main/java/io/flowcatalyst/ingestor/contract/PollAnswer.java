package io.flowcatalyst.ingestor.contract;

import java.time.Duration;
import java.time.Instant;
import java.util.List;
import org.jspecify.annotations.Nullable;

/**
 * The platform's answer to one runtime's poll: the live version of every ingestion that runtime
 * should be running, in full, and the connection tests waiting for it. An ingestion with a {@linkplain
 * Source#singleOwner() single owner} is listed for one runtime only, and only for as long as the
 * lease lasts.
 *
 * @param asOf when the platform read them
 * @param leaseFor how long, counted from when the runtime sent the poll, it may run the
 *     single-owner ingestions listed here without another answer
 * @param ingestions the ingestions this runtime should run
 * @param tests the "test connection" requests for kinds this runtime carries; none when absent
 */
public record PollAnswer(Instant asOf, Duration leaseFor, List<Ingestion> ingestions, @Nullable List<TestRequest> tests) {

  /** Copies the lists; no tests when absent. */
  public PollAnswer {
    ingestions = List.copyOf(ingestions);
    tests = tests == null ? List.of() : List.copyOf(tests);
  }

  /**
   * An answer without tests.
   *
   * @param asOf when the platform read the ingestions
   * @param leaseFor how long the single-owner ingestions may run
   * @param ingestions the ingestions to run
   */
  public PollAnswer(Instant asOf, Duration leaseFor, List<Ingestion> ingestions) {
    this(asOf, leaseFor, ingestions, null);
  }

  /**
   * The tests.
   *
   * @return the pending "test connection" requests
   */
  public List<TestRequest> testList() {
    List<TestRequest> list = tests;
    return list == null ? List.of() : list;
  }
}
