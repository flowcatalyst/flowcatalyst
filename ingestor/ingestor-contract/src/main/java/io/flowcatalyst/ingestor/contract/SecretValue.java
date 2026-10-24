package io.flowcatalyst.ingestor.contract;

import java.time.Instant;

/**
 * A revealed secret, as the platform answers {@code GET /ingestions/{id}/secrets/{name}}. Never log
 * it; {@code toString} hides the value.
 *
 * @param value the secret
 * @param updatedAt when it was last set
 */
public record SecretValue(String value, Instant updatedAt) {

  @Override
  public String toString() {
    return "SecretValue[updatedAt=" + updatedAt + "]";
  }
}
