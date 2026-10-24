package io.flowcatalyst.ingestor.runtime;

import java.io.IOException;
import java.util.Map;
import java.util.Optional;
import java.util.concurrent.ConcurrentHashMap;

/** A state store in memory, which a test can make refuse writes. */
final class MemoryState implements StateStore {

  final Map<String, String> values = new ConcurrentHashMap<>();
  volatile boolean refuseWrites;

  @Override
  public Optional<String> get(String ingestionId, String key) {
    return Optional.ofNullable(values.get(ingestionId + "/" + key));
  }

  @Override
  public void put(String ingestionId, String key, String value) throws IOException {
    if (refuseWrites) {
      throw new IOException("PUT /runtime/v1/state answered 409");
    }
    values.put(ingestionId + "/" + key, value);
  }
}
