package io.flowcatalyst.ingestor.contract;

/**
 * The platform's answer to a batch of trace records (202).
 *
 * @param accepted records stored
 * @param dropped records refused (a window that is over, a rate limit, a record already held)
 */
public record TraceAccepted(int accepted, int dropped) {}
