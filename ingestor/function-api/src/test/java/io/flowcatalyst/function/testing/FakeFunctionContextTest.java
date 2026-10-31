package io.flowcatalyst.function.testing;

import io.flowcatalyst.function.Caller;
import io.flowcatalyst.function.DispatchException;
import io.flowcatalyst.function.DispatchJob;
import io.flowcatalyst.function.EmitResult;
import io.flowcatalyst.function.Function;
import io.flowcatalyst.function.FunctionAddress;
import io.flowcatalyst.function.OutboundEvent;
import io.flowcatalyst.function.Request;
import io.flowcatalyst.function.Result;
import io.flowcatalyst.function.Webhook;

import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.util.NoSuchElementException;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

/// The test helpers themselves, and a function unit-tested end to end through them.
class FakeFunctionContextTest {

    private static final FunctionAddress ADDRESS = FunctionAddress.parse("orders.fulfil");

    @Test
    void defaultsAndIdentity() {
        FakeFunctionContext ctx = FakeFunctionContext.builder(ADDRESS).build();
        assertThat(ctx.address()).isEqualTo(ADDRESS);
        assertThat(ctx.version()).isEqualTo(1);
        assertThat(ctx.clock().instant()).isEqualTo(Instant.parse("2026-09-20T00:00:00Z"));
        assertThat(ctx.logger().getName()).isEqualTo("fn.orders.fulfil");
        assertThatThrownBy(() -> ctx.http().send(null)).isInstanceOf(UnsupportedOperationException.class);
        assertThatThrownBy(() -> ctx.dataSource("main")).isInstanceOf(UnsupportedOperationException.class);
    }

    @Test
    void configAndSecretsBehaveLikeTheHosts() {
        FakeFunctionContext ctx = FakeFunctionContext.builder(ADDRESS).version(7)
                .config("greeting", "Hi").secret("api-key", "s3cr3t").build();
        assertThat(ctx.version()).isEqualTo(7);
        assertThat(ctx.config().get("greeting")).contains("Hi");
        assertThat(ctx.config().get("missing")).isEmpty();
        assertThatThrownBy(() -> ctx.config().require("missing")).isInstanceOf(NoSuchElementException.class);
        assertThat(ctx.secrets().require("api-key")).isEqualTo("s3cr3t");
        assertThatThrownBy(() -> ctx.secrets().require("nope")).isInstanceOf(NoSuchElementException.class)
                .hasMessageContaining("nope");
        assertThat(ctx.secrets().toString()).contains("api-key").doesNotContain("s3cr3t");
    }

    @Test
    void loggerCapturesFormattedLines() {
        FakeFunctionContext ctx = FakeFunctionContext.builder(ADDRESS).build();
        ctx.logger().log(System.Logger.Level.INFO, "hello {0}", "ann");
        ctx.logger().log(System.Logger.Level.INFO, "plain");
        assertThat(ctx.capturingLogger().lines()).containsExactly("hello ann", "plain");
    }

    @Test
    void eventsAreRecordedAndARefusalIsOneShot() {
        FakeFunctionContext ctx = FakeFunctionContext.builder(ADDRESS).build();
        OutboundEvent event = new OutboundEvent("orders:order:shipped", null, null, null, null, null, null, null, "d1");
        assertThat(ctx.events().emit(event)).isEqualTo(new EmitResult.Emitted("evt_recorded_1"));
        ctx.events().refuseNext(new EmitResult.Refused("EVENT_TYPE_NOT_OWNED", 403, "no"));
        assertThat(ctx.events().emit(event)).isInstanceOf(EmitResult.Refused.class);
        assertThat(ctx.events().emit(event)).isEqualTo(new EmitResult.Emitted("evt_recorded_2"));
        assertThat(ctx.events().emitted()).hasSize(2);
    }

    @Test
    void dispatchesAreRecordedAndAFailureIsOneShot() {
        FakeFunctionContext ctx = FakeFunctionContext.builder(ADDRESS).build();
        DispatchJob job = DispatchJob.to("orders.pack", "/pack").build();
        assertThat(ctx.dispatcher().dispatch(job)).isEqualTo("job_recorded_1");
        ctx.dispatcher().failNext(new DispatchException("NOT_DECLARED", "x"));
        assertThatThrownBy(() -> ctx.dispatcher().dispatch(job)).isInstanceOf(DispatchException.class);
        assertThat(ctx.dispatcher().dispatch(job)).isEqualTo("job_recorded_2");
        assertThat(ctx.dispatcher().dispatched()).containsExactly(job, job);
    }

    @Test
    void fakeRequestDefaultsAndOverrides() {
        Request in = FakeRequest.get("/hello/ann").build();
        assertThat(in.method()).isEqualTo("GET");
        assertThat(in.path()).isEqualTo("/hello/ann");
        assertThat(in.caller()).isEqualTo(Caller.Anonymous.INSTANCE);
        assertThat(in.deadline()).isAfter(Instant.parse("2090-01-01T00:00:00Z"));

        Request post = FakeRequest.post("/e").header("X-A", "1").header("X-A", "2").pathParam("n", "v")
                .rawQuery("q=1").caller(Caller.Platform.INSTANCE).isPublic(true).body("hi").build();
        assertThat(post.headers("x-a")).containsExactly("1", "2");
        assertThat(post.pathParams()).containsEntry("n", "v");
        assertThat(post.queryParam("q")).contains("1");
        assertThat(post.isPublic()).isTrue();
        assertThat(post.bodyText()).isEqualTo("hi");
    }

    /// A function written against the API, driven with no host at all.
    static final class Fulfil implements Function {
        @Override
        public Result handle(Request in, io.flowcatalyst.function.FunctionContext ctx) {
            if (!(in.caller() instanceof Caller.Platform)) {
                return Result.status(401);
            }
            var event = Webhook.event(in);
            if (event.dataJson() == null) {
                return Result.reject("no data");
            }
            try {
                ctx.dispatcher().dispatch(DispatchJob.to("orders.pack", "/pack")
                        .payloadJson(event.dataJson()).messageGroup(event.messageGroup()).build());
            } catch (DispatchException e) {
                return e.retryable() ? Result.retry(java.time.Duration.ofSeconds(5)) : Result.reject(e.getMessage());
            }
            return Result.ack();
        }
    }

    @Test
    void aFunctionIsUnitTestableWithoutAHost() {
        FakeFunctionContext ctx = FakeFunctionContext.builder(ADDRESS).build();
        Request in = FakeRequest.post("/fulfil").caller(Caller.Platform.INSTANCE)
                .body("{\"id\":\"j1\",\"type\":\"orders:order:paid\",\"attemptNumber\":1,\"messageGroup\":\"o-1\","
                        + "\"data\":{\"order\":42}}").build();
        Fulfil fn = new Fulfil();

        assertThat(fn.handle(in, ctx)).isEqualTo(Result.ack());
        assertThat(ctx.dispatcher().dispatched()).singleElement().satisfies(job -> {
            assertThat(new String(job.payload(), StandardCharsets.UTF_8)).isEqualTo("{\"order\":42}");
            assertThat(job.messageGroup()).isEqualTo("o-1");
            assertThat(job.contentType()).isEqualTo("application/json");
        });

        ctx.dispatcher().failNext(new DispatchException("UNAVAILABLE", "down"));
        assertThat(fn.handle(in, ctx).status()).isEqualTo(429);
        ctx.dispatcher().failNext(new DispatchException("NOT_ALLOWED", "no"));
        Result rejected = fn.handle(in, ctx);
        assertThat(rejected.status()).isEqualTo(422);
        assertThat(rejected.headers().get(Result.OUTCOME_HEADER)).containsExactly(Result.OUTCOME_REJECT);

        assertThat(fn.handle(FakeRequest.post("/fulfil").build(), ctx).status()).isEqualTo(401);
    }
}
