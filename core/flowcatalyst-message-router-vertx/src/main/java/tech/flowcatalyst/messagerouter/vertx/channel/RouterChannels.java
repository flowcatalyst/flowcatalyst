package tech.flowcatalyst.messagerouter.vertx.channel;

import io.vertx.core.Vertx;
import tech.flowcatalyst.messagerouter.vertx.message.RouterMessages.*;

/**
 * Factory for type-safe router channels.
 * <p>
 * Provides compile-time binding between addresses and message types:
 * <pre>
 * RouterChannels.ack(vertx).request(new AckRequest("msg-123"));  // OK
 * RouterChannels.ack(vertx).request(new NackRequest(...));       // Compile error!
 * </pre>
 */
public final class RouterChannels {

    private RouterChannels() {}

    // ==================== Router Addresses ====================

    public static final String BATCH = "router.batch";
    public static final String ACK = "router.ack";
    public static final String NACK = "router.nack";
    public static final String QUERY_IN_FLIGHT = "router.query.in-flight";
    public static final String QUERY_POOL_STATS = "router.query.pool-stats";

    // ==================== Typed Channels ====================

    /**
     * Channel for submitting message batches from queue consumers.
     * <p>
     * Request: BatchRequest → Response: OkReply
     */
    public static TypedChannel<BatchRequest, OkReply> batch(Vertx vertx) {
        return new TypedChannel<>(vertx, BATCH);
    }

    /**
     * Channel for acknowledging successfully processed messages.
     * <p>
     * Request: AckRequest → Response: OkReply
     */
    public static TypedChannel<AckRequest, OkReply> ack(Vertx vertx) {
        return new TypedChannel<>(vertx, ACK);
    }

    /**
     * Channel for negative acknowledgement (retry later).
     * <p>
     * Request: NackRequest → Response: OkReply
     */
    public static TypedChannel<NackRequest, OkReply> nack(Vertx vertx) {
        return new TypedChannel<>(vertx, NACK);
    }

    /**
     * Channel for querying in-flight messages.
     * <p>
     * Request: InFlightQuery → Response: InFlightQueryResult
     */
    public static TypedChannel<InFlightQuery, InFlightQueryResult> queryInFlight(Vertx vertx) {
        return new TypedChannel<>(vertx, QUERY_IN_FLIGHT);
    }

    /**
     * Channel for querying pool statistics.
     * <p>
     * Request: String (ignored) → Response: PoolStatsResult
     */
    public static TypedChannel<String, PoolStatsResult> queryPoolStats(Vertx vertx) {
        return new TypedChannel<>(vertx, QUERY_POOL_STATS);
    }
}
