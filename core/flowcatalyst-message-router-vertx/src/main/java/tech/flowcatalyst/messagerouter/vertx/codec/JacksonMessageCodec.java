package tech.flowcatalyst.messagerouter.vertx.codec;

import com.fasterxml.jackson.databind.ObjectMapper;
import io.vertx.core.buffer.Buffer;
import io.vertx.core.eventbus.MessageCodec;
import org.jboss.logging.Logger;

/**
 * Generic Jackson-based message codec for Vert.x event bus.
 * <p>
 * This codec serializes Java objects (including records) to JSON bytes
 * using Jackson and deserializes them back. It can be registered once
 * per message type.
 *
 * @param <T> The message type this codec handles
 */
public class JacksonMessageCodec<T> implements MessageCodec<T, T> {

    private static final Logger LOG = Logger.getLogger(JacksonMessageCodec.class);

    private final ObjectMapper objectMapper;
    private final Class<T> clazz;
    private final String codecName;

    public JacksonMessageCodec(ObjectMapper objectMapper, Class<T> clazz) {
        this.objectMapper = objectMapper;
        this.clazz = clazz;
        this.codecName = clazz.getName();
    }

    @Override
    public void encodeToWire(Buffer buffer, T message) {
        try {
            byte[] bytes = objectMapper.writeValueAsBytes(message);
            buffer.appendInt(bytes.length);
            buffer.appendBytes(bytes);
        } catch (Exception e) {
            LOG.errorf("Failed to encode message of type %s: %s", clazz.getName(), e.getMessage());
            throw new RuntimeException("Failed to encode message", e);
        }
    }

    @Override
    public T decodeFromWire(int pos, Buffer buffer) {
        try {
            int length = buffer.getInt(pos);
            pos += 4;
            byte[] bytes = buffer.getBytes(pos, pos + length);
            return objectMapper.readValue(bytes, clazz);
        } catch (Exception e) {
            LOG.errorf("Failed to decode message of type %s: %s", clazz.getName(), e.getMessage());
            throw new RuntimeException("Failed to decode message", e);
        }
    }

    @Override
    public T transform(T message) {
        // Local delivery - no transformation needed
        return message;
    }

    @Override
    public String name() {
        return codecName;
    }

    @Override
    public byte systemCodecID() {
        return -1; // Custom codec
    }
}
