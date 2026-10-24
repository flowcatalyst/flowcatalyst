package io.flowcatalyst.ingestor.runtime;

import java.io.IOException;
import java.util.Optional;

/**
 * Small values an ingestion keeps between polls, such as the watermark of a database poll. They
 * are kept outside the runtime so that they follow the ingestion to whichever runtime runs it.
 */
public interface StateStore {

  /**
   * Reads a value.
   *
   * @param ingestionId the ingestion
   * @param key the value's name
   * @return the value, when one has been stored
   * @throws IOException when the store cannot be reached
   * @throws InterruptedException when interrupted while waiting
   */
  Optional<String> get(String ingestionId, String key) throws IOException, InterruptedException;

  /**
   * Stores a value. The store refuses it when this runtime no longer holds the ingestion.
   *
   * @param ingestionId the ingestion
   * @param key the value's name
   * @param value the value
   * @throws IOException when the store cannot be reached or refuses the value
   * @throws InterruptedException when interrupted while waiting
   */
  void put(String ingestionId, String key, String value) throws IOException, InterruptedException;
}
