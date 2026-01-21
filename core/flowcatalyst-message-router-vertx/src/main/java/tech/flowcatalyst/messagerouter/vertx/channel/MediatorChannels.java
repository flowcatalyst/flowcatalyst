package tech.flowcatalyst.messagerouter.vertx.channel;

import io.vertx.core.Vertx;
import tech.flowcatalyst.messagerouter.vertx.message.RouterMessages.*;

/**
 * Factory for type-safe mediator channels.
 * <p>
 * Each pool has its own mediator that handles HTTP delivery:
 * <pre>
 * MediatorChannels.Address addr = MediatorChannels.address("high-priority");
 * MediationResult result = addr.mediate(vertx).requestBlocking(request);
 * </pre>
 */
public final class MediatorChannels {

    private MediatorChannels() {}

    /**
     * Create a typed address for a pool's mediator.
     *
     * @param poolCode The pool identifier
     * @return Type-safe address that provides channel factory
     */
    public static Address address(String poolCode) {
        return new Address(poolCode);
    }

    /**
     * Type-safe mediator address.
     */
    public record Address(String poolCode) {

        /**
         * Address for mediation requests.
         */
        public String mediateAddress() {
            return "mediator." + poolCode;
        }

        /**
         * Channel for HTTP mediation requests.
         * <p>
         * Request: MediationRequest → Response: MediationResult
         * <p>
         * Uses longer timeout (2 minutes) for HTTP calls.
         */
        public TypedChannel<MediationRequest, MediationResult> mediate(Vertx vertx) {
            return new TypedChannel<>(vertx, mediateAddress(), 120_000);
        }
    }
}
