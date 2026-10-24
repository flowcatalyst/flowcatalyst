package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.Ingestion;
import org.apache.camel.CamelContext;
import org.apache.camel.Endpoint;

/** Turns an ingestion's source into the Camel endpoint its route consumes from. */
public interface SourceEndpoints {

  /**
   * The endpoint for an ingestion's source.
   *
   * @param camel the context the route is added to
   * @param ingestion the ingestion
   * @return the endpoint to consume from
   * @throws UnsupportedIngestionException when the runtime cannot run this source
   */
  Endpoint endpoint(CamelContext camel, Ingestion ingestion);
}
