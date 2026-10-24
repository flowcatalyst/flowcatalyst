package io.flowcatalyst.ingestor.contract;

import com.fasterxml.jackson.annotation.JsonInclude;
import java.util.List;
import org.jspecify.annotations.Nullable;

/**
 * A runtime asking what it should run. The call also tells the platform the runtime is alive and
 * renews the leases it holds.
 *
 * @param runtimeId the asking runtime; unique among running runtimes
 * @param contractVersion the runtime contract version the runtime speaks; the platform serves the
 *     current one and the one before
 * @param kinds the kinds the runtime's extensions add, and {@code camel-route} when enabled; the
 *     built-in kinds are all carried
 * @param url where the runtime can be reached; informational, the platform never calls it
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
public record RuntimePoll(String runtimeId, int contractVersion, @Nullable List<RuntimeKind> kinds, @Nullable String url) {

  /** The contract version this runtime speaks. */
  public static final int CONTRACT_VERSION = 2;

  /** Copies the kinds; none when absent. */
  public RuntimePoll {
    kinds = kinds == null ? List.of() : List.copyOf(kinds);
  }

  /**
   * A poll in the contract version this runtime speaks.
   *
   * @param runtimeId the asking runtime
   * @param kinds the kinds it adds
   * @param url where it can be reached, or null
   */
  public RuntimePoll(String runtimeId, List<RuntimeKind> kinds, @Nullable String url) {
    this(runtimeId, CONTRACT_VERSION, kinds, url);
  }
}
