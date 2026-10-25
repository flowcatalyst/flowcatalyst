package io.flowcatalyst.ingestor.extension;

import org.apache.camel.Exchange;

/**
 * Where a payload came from, said so that reading it again says the same: a file's path, size and
 * time, a message's id, the page asked for. The runtime makes each record's idempotency key (the
 * job's {@code externalId}) from it and the record's position in the payload, so a payload read
 * twice (a retry, a source that offers it again, a move to another runtime) answers the jobs it made
 * first instead of making them again; and the jobs of one payload share a message group however
 * often it is read.
 *
 * <p>It must name the payload, not describe its content: two different payloads must never share
 * one, and one payload must keep it every time it is read. When a source cannot say, it gives none
 * and its jobs carry no key.
 *
 * <p>A {@link PolledSource} gives it with {@link PolledSource.Payloads#dispatch(String, String)}; any
 * other endpoint sets the exchange property {@link #PROPERTY} on the exchange it hands the route
 * ({@link #set(Exchange, String)}). Added in extension API 1; a runtime that predates it ignores it.
 */
public final class PayloadIdentity {

  /** The exchange property that holds the identity, a string of any length. */
  public static final String PROPERTY = "ingestPayloadIdentity";

  private PayloadIdentity() {}

  /**
   * Says where the exchange's payload came from.
   *
   * @param exchange the exchange handed to the route
   * @param identity the payload's identity; blank for none
   */
  public static void set(Exchange exchange, String identity) {
    if (!identity.isBlank()) {
      exchange.setProperty(PROPERTY, identity);
    }
  }
}
