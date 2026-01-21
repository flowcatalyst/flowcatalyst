package tech.flowcatalyst.messagerouter.vertx.codec;

import com.fasterxml.jackson.databind.ObjectMapper;
import io.vertx.core.Vertx;
import io.vertx.core.eventbus.EventBus;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.vertx.message.RouterMessages.*;

/**
 * Registers Jackson codecs for all message types used on the event bus.
 * <p>
 * Call {@link #registerAll(Vertx, ObjectMapper)} once at startup before
 * deploying any verticles that use typed messages.
 */
public final class CodecRegistry {

    private static final Logger LOG = Logger.getLogger(CodecRegistry.class);

    private CodecRegistry() {}

    /**
     * Register all message codecs on the event bus.
     * Must be called before any verticles are deployed.
     */
    public static void registerAll(Vertx vertx, ObjectMapper objectMapper) {
        EventBus eventBus = vertx.eventBus();

        // Router messages
        register(eventBus, objectMapper, BatchRequest.class);
        register(eventBus, objectMapper, QueuedMessage.class);
        register(eventBus, objectMapper, AckRequest.class);
        register(eventBus, objectMapper, NackRequest.class);
        register(eventBus, objectMapper, InFlightQuery.class);
        register(eventBus, objectMapper, InFlightQueryResult.class);
        register(eventBus, objectMapper, PoolStatsResult.class);

        // Pool messages
        register(eventBus, objectMapper, PoolMessage.class);
        register(eventBus, objectMapper, PoolConfigUpdate.class);

        // Pool results (sealed interface implementations)
        register(eventBus, objectMapper, Accepted.class);
        register(eventBus, objectMapper, Rejected.class);
        register(eventBus, objectMapper, ConfigUpdated.class);
        register(eventBus, objectMapper, ConfigFailed.class);

        // Mediator messages
        register(eventBus, objectMapper, MediationRequest.class);
        register(eventBus, objectMapper, MediationResult.class);

        // Generic
        register(eventBus, objectMapper, OkReply.class);

        LOG.info("Registered Jackson codecs for event bus messages");
    }

    private static <T> void register(EventBus eventBus, ObjectMapper objectMapper, Class<T> clazz) {
        try {
            eventBus.registerDefaultCodec(clazz, new JacksonMessageCodec<>(objectMapper, clazz));
        } catch (IllegalStateException e) {
            // Already registered (can happen in tests)
            LOG.debugf("Codec already registered for %s", clazz.getName());
        }
    }
}
