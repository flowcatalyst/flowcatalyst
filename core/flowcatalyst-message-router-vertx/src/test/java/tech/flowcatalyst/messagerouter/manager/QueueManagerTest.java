package tech.flowcatalyst.messagerouter.manager;

import com.fasterxml.jackson.databind.ObjectMapper;
import io.vertx.core.Vertx;
import io.vertx.core.json.JsonObject;
import io.vertx.junit5.VertxExtension;
import io.vertx.junit5.VertxTestContext;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import tech.flowcatalyst.messagerouter.model.MediationType;
import tech.flowcatalyst.messagerouter.vertx.codec.CodecRegistry;
import tech.flowcatalyst.messagerouter.vertx.message.RouterMessages.*;

import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Unit tests for event bus message types and codec.
 * Tests the typed message passing without deploying actual verticles.
 */
@ExtendWith(VertxExtension.class)
class QueueManagerTest {

    private ObjectMapper objectMapper;

    @BeforeEach
    void setUp(Vertx vertx, VertxTestContext ctx) {
        objectMapper = new ObjectMapper();

        // Register Jackson codecs for typed messages
        CodecRegistry.registerAll(vertx, objectMapper);

        ctx.completeNow();
    }

    @Test
    void shouldPassTypedPoolMessage(Vertx vertx, VertxTestContext ctx) throws Exception {
        // Given
        PoolMessage message = createTestMessage("msg-1", "TEST-POOL");

        // Set up consumer that receives typed message
        vertx.eventBus().<PoolMessage>consumer("pool.TEST-POOL", msg -> {
            PoolMessage received = msg.body();
            assertEquals("msg-1", received.id());
            assertEquals("sqs-msg-1", received.sqsMessageId());
            assertEquals("TEST-POOL", received.poolCode());
            assertEquals(MediationType.HTTP, received.mediationType());
            ctx.completeNow();
        });

        // When - send typed message
        vertx.eventBus().send("pool.TEST-POOL", message);

        // Then
        assertTrue(ctx.awaitCompletion(5, TimeUnit.SECONDS));
    }

    @Test
    void shouldPassTypedAckRequest(Vertx vertx, VertxTestContext ctx) throws Exception {
        // Given
        AckRequest ack = new AckRequest("sqs-msg-123");

        // Set up consumer
        vertx.eventBus().<AckRequest>consumer("router.ack", msg -> {
            AckRequest received = msg.body();
            assertEquals("sqs-msg-123", received.sqsMessageId());
            msg.reply(new OkReply());
            ctx.completeNow();
        });

        // When
        vertx.eventBus().request("router.ack", ack);

        // Then
        assertTrue(ctx.awaitCompletion(5, TimeUnit.SECONDS));
    }

    @Test
    void shouldPassTypedNackRequest(Vertx vertx, VertxTestContext ctx) throws Exception {
        // Given
        NackRequest nack = new NackRequest("sqs-msg-456", 30);

        // Set up consumer
        vertx.eventBus().<NackRequest>consumer("router.nack", msg -> {
            NackRequest received = msg.body();
            assertEquals("sqs-msg-456", received.sqsMessageId());
            assertEquals(30, received.delaySeconds());
            msg.reply(new OkReply());
            ctx.completeNow();
        });

        // When
        vertx.eventBus().request("router.nack", nack);

        // Then
        assertTrue(ctx.awaitCompletion(5, TimeUnit.SECONDS));
    }

    @Test
    void shouldPassTypedMediationRequest(Vertx vertx, VertxTestContext ctx) throws Exception {
        // Given
        MediationRequest request = new MediationRequest(
                "msg-1",
                "sqs-msg-1",
                "test-token",
                MediationType.HTTP,
                "http://localhost:8080/test",
                "group-1"
        );

        // Set up consumer
        vertx.eventBus().<MediationRequest>consumer("mediator.TEST-POOL", msg -> {
            MediationRequest received = msg.body();
            assertEquals("msg-1", received.id());
            assertEquals("sqs-msg-1", received.sqsMessageId());
            assertEquals("test-token", received.authToken());
            assertEquals(MediationType.HTTP, received.mediationType());
            assertEquals("http://localhost:8080/test", received.mediationTarget());
            assertEquals("group-1", received.messageGroupId());

            // Reply with success
            msg.reply(MediationResult.success());
        });

        // When
        vertx.eventBus().<MediationResult>request("mediator.TEST-POOL", request)
                .onComplete(ctx.succeeding(reply -> {
                    MediationResult result = reply.body();
                    assertEquals(MediationResult.Outcome.SUCCESS, result.outcome());
                    ctx.completeNow();
                }));

        // Then
        assertTrue(ctx.awaitCompletion(5, TimeUnit.SECONDS));
    }

    @Test
    void shouldPassMediationNackResult(Vertx vertx, VertxTestContext ctx) throws Exception {
        // Given
        MediationRequest request = new MediationRequest(
                "msg-1", "sqs-msg-1", "token", MediationType.HTTP,
                "http://localhost/fail", null
        );

        // Set up consumer that returns NACK
        vertx.eventBus().<MediationRequest>consumer("mediator.FAIL-POOL", msg -> {
            msg.reply(MediationResult.nack(30, "Rate limited"));
        });

        // When
        vertx.eventBus().<MediationResult>request("mediator.FAIL-POOL", request)
                .onComplete(ctx.succeeding(reply -> {
                    MediationResult result = reply.body();
                    assertEquals(MediationResult.Outcome.NACK, result.outcome());
                    assertEquals(30, result.delaySeconds());
                    assertEquals("Rate limited", result.errorMessage());
                    ctx.completeNow();
                }));

        // Then
        assertTrue(ctx.awaitCompletion(5, TimeUnit.SECONDS));
    }

    @Test
    void shouldPassPoolConfigUpdate(Vertx vertx, VertxTestContext ctx) throws Exception {
        // Given
        PoolConfigUpdate config = new PoolConfigUpdate(20, 100);

        // Set up consumer
        vertx.eventBus().<PoolConfigUpdate>consumer("pool.TEST-POOL.config", msg -> {
            PoolConfigUpdate received = msg.body();
            assertEquals(20, received.concurrency());
            assertEquals(100, received.rateLimitPerMinute());
            ctx.completeNow();
        });

        // When
        vertx.eventBus().send("pool.TEST-POOL.config", config);

        // Then
        assertTrue(ctx.awaitCompletion(5, TimeUnit.SECONDS));
    }

    @Test
    void shouldPassMultipleMessagesInSequence(Vertx vertx, VertxTestContext ctx) throws Exception {
        // Given
        AtomicInteger count = new AtomicInteger(0);

        vertx.eventBus().<PoolMessage>consumer("pool.TEST-POOL", msg -> {
            if (count.incrementAndGet() == 3) {
                ctx.completeNow();
            }
        });

        // When - send 3 messages
        for (int i = 1; i <= 3; i++) {
            vertx.eventBus().send("pool.TEST-POOL", createTestMessage("msg-" + i, "TEST-POOL"));
        }

        // Then
        assertTrue(ctx.awaitCompletion(5, TimeUnit.SECONDS));
        assertEquals(3, count.get());
    }

    // ==================== Helper Methods ====================

    private PoolMessage createTestMessage(String id, String poolCode) {
        return new PoolMessage(
                id,
                "sqs-" + id,
                poolCode,
                "test-token",
                MediationType.HTTP,
                "http://localhost:8080/test",
                null,
                "test-batch"
        );
    }
}
