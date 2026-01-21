package tech.flowcatalyst.messagerouter.vertx.channel;

import io.vertx.core.Vertx;
import tech.flowcatalyst.messagerouter.vertx.message.RouterMessages.*;

/**
 * Factory for type-safe pool channels.
 * <p>
 * Uses a typed address record to prevent string concatenation errors:
 * <pre>
 * PoolChannels.Address addr = PoolChannels.address("high-priority");
 * addr.messages(vertx).request(poolMessage);  // OK - IDE autocomplete
 * addr.config(vertx).request(configUpdate);   // OK - type-safe
 * </pre>
 */
public final class PoolChannels {

    private PoolChannels() {}

    /**
     * Create a typed address for a pool.
     *
     * @param poolCode The pool identifier (e.g., "high-priority", "order-service")
     * @return Type-safe address that provides channel factories
     */
    public static Address address(String poolCode) {
        return new Address(poolCode);
    }

    /**
     * Type-safe pool address that provides channel factories.
     * <p>
     * Eliminates string concatenation and provides IDE autocomplete.
     */
    public record Address(String poolCode) {

        /**
         * Address for submitting messages to this pool.
         */
        public String messagesAddress() {
            return "pool." + poolCode;
        }

        /**
         * Address for configuration updates.
         */
        public String configAddress() {
            return "pool." + poolCode + ".config";
        }

        /**
         * Channel for submitting messages to this pool.
         * <p>
         * Request: PoolMessage → Response: SubmitResult
         */
        public TypedChannel<PoolMessage, SubmitResult> messages(Vertx vertx) {
            return new TypedChannel<>(vertx, messagesAddress());
        }

        /**
         * Channel for submitting messages (fire-and-forget, no backpressure).
         * <p>
         * Use when you don't need confirmation. The pool will handle
         * ACK/NACK via RouterChannels.
         */
        public TypedChannel<PoolMessage, Void> messagesFireAndForget(Vertx vertx) {
            return new TypedChannel<>(vertx, messagesAddress());
        }

        /**
         * Channel for configuration updates.
         * <p>
         * Request: PoolConfigUpdate → Response: ConfigResult
         */
        public TypedChannel<PoolConfigUpdate, ConfigResult> config(Vertx vertx) {
            return new TypedChannel<>(vertx, configAddress());
        }
    }
}
