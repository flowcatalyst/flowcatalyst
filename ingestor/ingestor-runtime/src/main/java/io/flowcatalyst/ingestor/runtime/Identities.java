package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.framework.http.Json;
import io.flowcatalyst.ingestor.contract.JobRecord;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.HexFormat;

/**
 * The keys made from where a payload came from: a record's idempotency key ({@code externalId}),
 * the message group of a payload's jobs, and the size of a record as the platform is sent it.
 */
final class Identities {

  private Identities() {}

  /**
   * A record's idempotency key: the SHA-256 of the payload's identity and the record's position in
   * it, in hex (64 characters, within the platform's 100).
   *
   * @param payload where the payload came from
   * @param record the record's position among the payload's records, from 0
   * @return the key
   */
  static String externalId(String payload, long record) {
    return sha256(payload + "#" + record);
  }

  /**
   * The SHA-256 of the text, in hex.
   *
   * @param text the text
   * @return 64 hex digits
   */
  static String sha256(String text) {
    return sha256(text.getBytes(StandardCharsets.UTF_8));
  }

  /**
   * The SHA-256 of the bytes, in hex.
   *
   * @param bytes the bytes
   * @return 64 hex digits
   */
  static String sha256(byte[] bytes) {
    try {
      return HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256").digest(bytes));
    } catch (NoSuchAlgorithmException e) {
      throw new IllegalStateException("every JVM has SHA-256", e);
    }
  }

  /**
   * How many bytes the record is in the JSON of a jobs request, as the platform's client writes it.
   *
   * @param record the record
   * @return its size
   */
  static int encodedSize(JobRecord record) {
    return Json.mapper().writeValueAsBytes(record).length;
  }
}
