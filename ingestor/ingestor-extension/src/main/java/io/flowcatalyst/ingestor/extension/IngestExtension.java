package io.flowcatalyst.ingestor.extension;

import io.flowcatalyst.ingestor.contract.ConnectionTest;
import io.flowcatalyst.ingestor.contract.ExtensionKind;
import java.util.List;
import org.apache.camel.Endpoint;
import org.jspecify.annotations.Nullable;

/**
 * The runtime's part of a kind of source an extension adds: it turns an ingestion's settings into
 * the Camel endpoint the ingestion's route consumes from, and may try the source for "Test
 * connection". What the kind is configured with is its {@link ExtensionKind}, which the control
 * plane loads too.
 *
 * <p>Each exchange the endpoint produces is one payload: its body is split by the ingestion's
 * split and each record becomes a dispatch job. When the jobs cannot be created the exchange fails,
 * so a source that can keep a payload and offer it again (a message not acknowledged, a file not
 * moved) should do so on a failed exchange. A {@link PolledSource}, or a {@link
 * org.apache.camel.support.ScheduledPollEndpoint}, has its failed polls reported for it and the
 * ingestion shown {@code FAILING}.
 *
 * <p>An implementation has a public no-argument constructor and keeps no state of its own: the
 * runtime makes one instance and asks it for an endpoint per ingestion and version.
 */
public interface IngestExtension {

  /**
   * The kind this extension adds, the same the module lists as its {@link ExtensionKind}. Its type
   * must not be a built-in kind's nor another extension's.
   *
   * @return the kind
   */
  ExtensionKind kind();

  /**
   * The endpoint for one ingestion. Its settings have been checked against {@link #kind()} first.
   *
   * @param context the ingestion, its settings and what the runtime offers it
   * @return a new endpoint, not one shared with another route; the runtime stops it when the
   *     ingestion stops
   * @throws InvalidSettingsException when the settings cannot be used; the ingestion is reported
   *     {@code FAILED} with the message, which must not hold a secret
   * @throws Exception when the endpoint cannot be made otherwise; the runtime tries again on its
   *     next turn
   */
  Endpoint endpoint(ExtensionContext context) throws Exception;

  /**
   * Tries the source once for "Test connection", as the endpoint would, and dispatches nothing: log
   * in, read a little, say what came back. The ingestion may not be saved; its state is not read or
   * written.
   *
   * @param context the ingestion, its settings and what the runtime offers it
   * @param steps what was done, a sentence each without a secret, added to as the test goes, so
   *     the steps before a failure are shown with it
   * @return what it found, or null when this kind offers no test (the default)
   * @throws InvalidSettingsException when the settings cannot be used
   * @throws Exception when the source failed; its message, which must not hold a secret, is the
   *     test's last step, and a refused login ({@link PolledSource#refusedLogin(Throwable)}) is shown
   *     as one
   */
  default @Nullable Tested test(ExtensionContext context, List<String> steps) throws Exception {
    return null;
  }

  /**
   * What a test found.
   *
   * @param outcome whether the source had results or none
   * @param records how many records it read, or null when it cannot say
   */
  record Tested(ConnectionTest.Outcome outcome, @Nullable Long records) {}
}
