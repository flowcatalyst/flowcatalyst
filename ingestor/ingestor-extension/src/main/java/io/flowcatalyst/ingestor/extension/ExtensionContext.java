package io.flowcatalyst.ingestor.extension;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.framework.httpclient.module.ServiceClients;
import java.io.IOException;
import java.time.InstantSource;
import java.util.Optional;
import org.apache.camel.CamelContext;

/** What the runtime gives an extension to build one ingestion's endpoint with. */
public interface ExtensionContext {

  /**
   * The context the route is added to.
   *
   * @return the context
   */
  CamelContext camel();

  /**
   * The ingestion: its id, version, client and code. Its source is the custom one.
   *
   * @return the ingestion
   */
  Ingestion ingestion();

  /**
   * The ingestion's settings, read by their kind's fields.
   *
   * @return the settings
   */
  Settings settings();

  /**
   * The runtime's HTTP clients: an extension registers its own ({@code registerScoped}) under a
   * name from {@link #clientName(String)}, so their requests are measured with the runtime's, and
   * closes the registration when its endpoint stops.
   *
   * @return the clients
   */
  ServiceClients http();

  /**
   * A name for an HTTP client of this ingestion, unique among the runtime's: the prefix, the
   * ingestion and its version, or a test's number.
   *
   * @param prefix lowercase letters, digits and '-', such as {@code mix}
   * @return the name
   */
  String clientName(String prefix);

  /**
   * The time, which tests set.
   *
   * @return the clock
   */
  default InstantSource clock() {
    return InstantSource.system();
  }

  /**
   * Small values kept between polls, such as a watermark or a cursor. They are kept by the control
   * plane, so they follow the ingestion to whichever runtime runs it next.
   *
   * @return the ingestion's state
   */
  State state();

  /** Small values an ingestion keeps between polls. */
  interface State {

    /**
     * Reads a value.
     *
     * @param key the value's name
     * @return the value, when one has been stored
     * @throws IOException when the platform cannot be reached
     * @throws InterruptedException when interrupted while waiting
     */
    Optional<String> get(String key) throws IOException, InterruptedException;

    /**
     * Stores a value; refused when this runtime no longer runs the ingestion.
     *
     * @param key the value's name
     * @param value the value
     * @throws IOException when the platform cannot be reached or refuses it
     * @throws InterruptedException when interrupted while waiting
     */
    void put(String key, String value) throws IOException, InterruptedException;
  }
}
