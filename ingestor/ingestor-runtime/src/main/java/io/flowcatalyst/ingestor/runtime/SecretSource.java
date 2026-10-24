package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.Ingestion;
import java.io.IOException;
import java.util.Optional;

/** Where the secrets an ingestion names are read from: the platform, one ingestion at a time. */
public interface SecretSource {

  /**
   * Reveals a secret the ingestion's live definition names, at the ingestion's client.
   *
   * @param ingestion the ingestion that names it
   * @param name the secret's name
   * @return the value, or empty when the platform has none under that name
   * @throws IOException when the platform cannot be reached or will not give it to this runtime
   * @throws InterruptedException when interrupted while waiting
   */
  Optional<String> reveal(Ingestion ingestion, String name) throws IOException, InterruptedException;
}
