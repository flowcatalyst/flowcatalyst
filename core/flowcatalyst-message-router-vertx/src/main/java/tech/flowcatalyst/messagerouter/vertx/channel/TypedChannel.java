package tech.flowcatalyst.messagerouter.vertx.channel;

import io.vertx.core.Future;
import io.vertx.core.Handler;
import io.vertx.core.Vertx;
import io.vertx.core.eventbus.DeliveryOptions;
import io.vertx.core.eventbus.Message;
import io.vertx.core.eventbus.MessageConsumer;

import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeUnit;

/**
 * Type-safe wrapper around Vert.x event bus addresses.
 * <p>
 * Enforces at compile time that a specific address only accepts messages
 * of type {@code Req} and returns responses of type {@code Res}.
 * <p>
 * Example:
 * <pre>
 * TypedChannel&lt;AckRequest, OkReply&gt; ackChannel = RouterChannels.ack(vertx);
 * ackChannel.request(new AckRequest("msg-123"));  // OK
 * ackChannel.request(new NackRequest(...));       // Compile error!
 * </pre>
 *
 * @param <Req> The request message type this channel accepts
 * @param <Res> The response message type this channel returns
 */
public final class TypedChannel<Req, Res> {

    private final Vertx vertx;
    private final String address;
    private final long timeoutMs;

    /**
     * Create a typed channel with default timeout (30 seconds).
     */
    public TypedChannel(Vertx vertx, String address) {
        this(vertx, address, 30_000);
    }

    /**
     * Create a typed channel with custom timeout.
     */
    public TypedChannel(Vertx vertx, String address, long timeoutMs) {
        this.vertx = vertx;
        this.address = address;
        this.timeoutMs = timeoutMs;
    }

    /**
     * Send a request and wait for a typed response.
     * <p>
     * Use this when you need confirmation or a result from the handler.
     *
     * @param message The request message
     * @return Future containing the typed response
     */
    public Future<Res> request(Req message) {
        DeliveryOptions options = new DeliveryOptions().setSendTimeout(timeoutMs);
        return vertx.eventBus()
                .<Res>request(address, message, options)
                .map(Message::body);
    }

    /**
     * Send a request and block until response (for use in virtual threads).
     * <p>
     * Only use this from virtual thread verticles or blocking code.
     *
     * @param message The request message
     * @return The typed response
     * @throws RuntimeException if the request fails or times out
     */
    public Res requestBlocking(Req message) {
        try {
            return request(message)
                    .toCompletionStage()
                    .toCompletableFuture()
                    .get(timeoutMs, TimeUnit.MILLISECONDS);
        } catch (Exception e) {
            throw new RuntimeException("Request to " + address + " failed: " + e.getMessage(), e);
        }
    }

    /**
     * Fire-and-forget send (no response expected).
     * <p>
     * Use sparingly - prefer {@link #request} for confirmation.
     *
     * @param message The message to send
     */
    public void send(Req message) {
        vertx.eventBus().send(address, message);
    }

    /**
     * Register a consumer that handles requests and sends typed responses.
     *
     * @param handler Handler that receives typed messages
     * @return The message consumer (for unregistration)
     */
    public MessageConsumer<Req> consumer(Handler<Message<Req>> handler) {
        return vertx.eventBus().<Req>consumer(address, handler);
    }

    /**
     * Get the underlying address (for logging/debugging).
     */
    public String address() {
        return address;
    }

    @Override
    public String toString() {
        return "TypedChannel[" + address + "]";
    }
}
