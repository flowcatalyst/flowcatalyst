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
 * @param kinds the kinds the runtime's extensions add, and {@code camel-route} when enabled
 * @param url where the runtime can be reached; informational, the platform never calls it
 * @param builtinKinds the built-in source kinds this runtime can run ({@link #BUILTIN_KINDS}), so
 *     that a platform that knows a kind this runtime does not hands it to a runtime that does. A
 *     platform that predates the field ignores it
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
public record RuntimePoll(
    String runtimeId, int contractVersion, @Nullable List<RuntimeKind> kinds, @Nullable String url, @Nullable List<String> builtinKinds) {

  /** The contract version this runtime speaks. Additions to it (such as {@code builtinKinds}) keep it. */
  public static final int CONTRACT_VERSION = 2;

  /** The built-in kinds this runtime runs: every kind of {@link Source} but the {@code custom} envelope. */
  public static final List<String> BUILTIN_KINDS =
      Source.builtInKinds().stream().filter(kind -> !kind.equals("custom")).toList();

  /** Copies the kinds; none when absent. */
  public RuntimePoll {
    kinds = kinds == null ? List.of() : List.copyOf(kinds);
    builtinKinds = builtinKinds == null ? null : List.copyOf(builtinKinds);
  }

  /**
   * A poll in the contract version this runtime speaks, naming the built-in kinds it runs.
   *
   * @param runtimeId the asking runtime
   * @param kinds the kinds it adds
   * @param url where it can be reached, or null
   */
  public RuntimePoll(String runtimeId, List<RuntimeKind> kinds, @Nullable String url) {
    this(runtimeId, CONTRACT_VERSION, kinds, url, BUILTIN_KINDS);
  }
}
