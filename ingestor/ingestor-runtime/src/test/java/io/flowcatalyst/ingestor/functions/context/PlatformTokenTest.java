package io.flowcatalyst.ingestor.functions.context;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.flowcatalyst.framework.httpclient.HttpClients;
import io.flowcatalyst.function.PlatformTokenException;
import io.flowcatalyst.ingestor.functions.FakeControlPlane;
import io.flowcatalyst.ingestor.functions.Harness.MovableClock;
import io.flowcatalyst.ingestor.functions.control.Control;
import io.flowcatalyst.ingestor.functions.control.Describe;
import java.io.ByteArrayOutputStream;
import java.io.PrintStream;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.atomic.AtomicInteger;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;

/// A function's platform token (docs/function-platform-token.md §5-6): refused unless declared, before any platform
/// call; cached while valid and renewed near expiry on the host's clock; one fetch for concurrent callers; a
/// refresh replaces; a failure is not cached; and the token is never in any log output. Tokens here are fake strings.
@Timeout(value = 60, threadMode = Timeout.ThreadMode.SEPARATE_THREAD)
class PlatformTokenTest {

    HttpClients clients;
    FakeControlPlane plane;
    DbPools pools;
    MovableClock clock;
    ContextFactory factory;
    final AtomicInteger issued = new AtomicInteger();

    @BeforeEach
    void start() {
        clients = HttpClients.create();
        plane = new FakeControlPlane();
        pools = new DbPools(DbPools.Limits.DEFAULTS, java.time.Clock.systemUTC());
        clock = new MovableClock();
        factory = new ContextFactory(new OutboundClients(clients), clock, plane, pools);
        // A new token on every fetch, valid for an hour.
        plane.tokenAnswer = r -> new Control.TokenResponse("fake-token-" + issued.incrementAndGet(), "Bearer", 3600);
    }

    @AfterEach
    void stop() {
        pools.close();
        clients.close();
    }

    HostFunctionContext context(boolean declared) {
        Control.FunctionDoc doc = new Control.FunctionDoc("fn_1", "app.hello", "app_1", null, null, false, null, Map.of(), Map.of(), Map.of(), List.of());
        Describe describe = new Describe(1, List.of(), List.of(), List.of(), List.of(), List.of(), List.of(), List.of(), declared, new Describe.Jvm("a.B"));
        return factory.build(doc, 4, describe);
    }

    @Test
    void anUndeclaredFunctionIsRefusedWithoutAPlatformCall() {
        HostFunctionContext ctx = context(false);

        assertThatThrownBy(ctx::platformToken).isInstanceOfSatisfying(PlatformTokenException.class, e -> {
            assertThat(e.code()).isEqualTo("NOT_DECLARED");
            assertThat(e.retryable()).isFalse();
        });
        assertThatThrownBy(() -> ctx.platformToken(true)).isInstanceOf(PlatformTokenException.class);
        assertThat(plane.tokenRequests).isEmpty();
    }

    @Test
    void theRequestNamesTheFunctionAndVersionAndTheTokenIsCached() {
        HostFunctionContext ctx = context(true);

        assertThat(ctx.platformToken()).isEqualTo("fake-token-1");
        assertThat(ctx.platformToken()).isEqualTo("fake-token-1");
        clock.advance(Duration.ofMinutes(10));
        assertThat(ctx.platformToken()).isEqualTo("fake-token-1");

        assertThat(plane.tokenRequests).containsExactly(new Control.TokenRequest("fn_1", 4));
    }

    @Test
    void theTokenIsSharedByEveryVersionOfTheFunction() {
        Control.FunctionDoc doc = new Control.FunctionDoc("fn_1", "app.hello", "app_1", null, null, false, null, Map.of(), Map.of(), Map.of(), List.of());
        Describe describe = new Describe(1, List.of(), List.of(), List.of(), List.of(), List.of(), List.of(), List.of(), true, new Describe.Jvm("a.B"));

        factory.build(doc, 1, describe).platformToken();
        factory.build(doc, 2, describe).platformToken();

        assertThat(plane.tokenRequests).hasSize(1);
    }

    @Test
    void aTokenIsRenewedWhenLessThanTheMarginRemains() {
        HostFunctionContext ctx = context(true);
        assertThat(ctx.platformToken()).isEqualTo("fake-token-1");

        // One hour: the margin is 10% (6 minutes). Still valid with 7 minutes left.
        clock.advance(Duration.ofMinutes(53));
        assertThat(ctx.platformToken()).isEqualTo("fake-token-1");

        // 5 minutes left: renewed.
        clock.advance(Duration.ofMinutes(2));
        assertThat(ctx.platformToken()).isEqualTo("fake-token-2");
        assertThat(plane.tokenRequests).hasSize(2);
    }

    @Test
    void aShortLivedTokenUsesTheSixtySecondFloor() {
        plane.tokenAnswer = r -> new Control.TokenResponse("fake-token-" + issued.incrementAndGet(), "Bearer", 300);
        HostFunctionContext ctx = context(true);
        assertThat(ctx.platformToken()).isEqualTo("fake-token-1");

        // 10% of 300 s is 30 s, below the floor: renewed once less than 60 s remains.
        clock.advance(Duration.ofSeconds(239));
        assertThat(ctx.platformToken()).isEqualTo("fake-token-1");
        clock.advance(Duration.ofSeconds(2));
        assertThat(ctx.platformToken()).isEqualTo("fake-token-2");
    }

    @Test
    void concurrentCallersShareOneFetch() throws Exception {
        CountDownLatch inFetch = new CountDownLatch(1);
        CountDownLatch release = new CountDownLatch(1);
        plane.tokenAnswer = r -> {
            inFetch.countDown();
            try {
                release.await();
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
            }
            return new Control.TokenResponse("fake-token-" + issued.incrementAndGet(), "Bearer", 3600);
        };
        HostFunctionContext ctx = context(true);
        int n = 32;
        try (ExecutorService pool = Executors.newVirtualThreadPerTaskExecutor()) {
            List<Future<String>> results = new ArrayList<>();
            for (int i = 0; i < n; i++) {
                results.add(pool.submit(() -> ctx.platformToken()));
            }
            inFetch.await();
            release.countDown();
            for (Future<String> f : results) {
                assertThat(f.get()).isEqualTo("fake-token-1");
            }
        }
        assertThat(plane.tokenRequests).hasSize(1);
    }

    @Test
    void refreshFetchesANewTokenAndReplacesTheEntry() {
        HostFunctionContext ctx = context(true);
        assertThat(ctx.platformToken()).isEqualTo("fake-token-1");

        assertThat(ctx.platformToken(true)).isEqualTo("fake-token-2");

        assertThat(ctx.platformToken()).isEqualTo("fake-token-2");
        assertThat(plane.tokenRequests).hasSize(2);
    }

    @Test
    void aFailedFetchIsNotCached() {
        plane.tokenAnswer = r -> {
            throw new PlatformTokenException(PlatformTokenException.UNAVAILABLE, "down");
        };
        HostFunctionContext ctx = context(true);
        assertThatThrownBy(ctx::platformToken).isInstanceOfSatisfying(PlatformTokenException.class, e -> assertThat(e.retryable()).isTrue());
        assertThatThrownBy(ctx::platformToken).isInstanceOf(PlatformTokenException.class);
        assertThat(plane.tokenRequests).hasSize(2);

        plane.tokenAnswer = r -> new Control.TokenResponse("fake-token-ok", "Bearer", 3600);
        assertThat(ctx.platformToken()).isEqualTo("fake-token-ok");
    }

    @Test
    void aFailedRefreshKeepsTheCachedTokenForTheNextPlainCall() {
        HostFunctionContext ctx = context(true);
        assertThat(ctx.platformToken()).isEqualTo("fake-token-1");
        plane.tokenAnswer = r -> {
            throw new PlatformTokenException("NOT_ALLOWED", "PLATFORM_TOKEN_NOT_DECLARED");
        };

        assertThatThrownBy(() -> ctx.platformToken(true)).isInstanceOfSatisfying(PlatformTokenException.class,
                e -> assertThat(e.code()).isEqualTo("NOT_ALLOWED"));

        assertThat(ctx.platformToken()).isEqualTo("fake-token-1");
    }

    @Test
    void aBlankTokenInTheAnswerIsUnavailable() {
        plane.tokenAnswer = r -> new Control.TokenResponse(" ", "Bearer", 3600);
        assertThatThrownBy(() -> context(true).platformToken()).isInstanceOfSatisfying(PlatformTokenException.class,
                e -> assertThat(e.code()).isEqualTo("UNAVAILABLE"));
    }

    @Test
    void anExpiredInvocationDeadlineIsRefusedWithoutAPlatformCall() throws Exception {
        HostFunctionContext ctx = context(true);
        Instant past = clock.instant().minusSeconds(1);

        assertThatThrownBy(() -> ScopedValue.where(InvocationDeadline.CURRENT, past).call(ctx::platformToken))
                .isInstanceOfSatisfying(PlatformTokenException.class, e -> {
                    assertThat(e.code()).isEqualTo("DEADLINE");
                    assertThat(e.retryable()).isFalse();
                });
        assertThat(plane.tokenRequests).isEmpty();
    }

    @Test
    void retainDropsTheEntriesOfFunctionsThatLeft() {
        context(true).platformToken();
        assertThat(factory.platformTokens().size()).isEqualTo(1);

        factory.retainFunctions(java.util.Set.of("fn_other"));
        assertThat(factory.platformTokens().size()).isZero();

        context(true).platformToken();
        assertThat(plane.tokenRequests).hasSize(2);
    }

    @Test
    void theTokenIsNeverLoggedOrPrinted() {
        PrintStream originalErr = System.err;
        PrintStream originalOut = System.out;
        ByteArrayOutputStream captured = new ByteArrayOutputStream();
        PrintStream sink = new PrintStream(captured, true, java.nio.charset.StandardCharsets.UTF_8);
        String secret = "fake-secret-token-never-log-me";
        String leaked;
        try {
            System.setErr(sink);
            System.setOut(sink);
            plane.tokenAnswer = r -> new Control.TokenResponse(secret, "Bearer", 3600);
            HostFunctionContext ctx = context(true);
            // A canary proves the capture sees the function logger's output.
            ctx.logger().log(System.Logger.Level.WARNING, "canary-line");
            ctx.platformToken();
            ctx.platformToken(true);
            plane.tokenAnswer = r -> {
                throw new PlatformTokenException("NOT_ALLOWED", "PLATFORM_TOKEN_NOT_DECLARED");
            };
            try {
                ctx.platformToken(true);
            } catch (PlatformTokenException e) {
                ctx.logger().log(System.Logger.Level.WARNING, "failed: " + e);
            }
            ctx.logger().log(System.Logger.Level.WARNING, "ctx: " + ctx.secrets() + " " + new Control.TokenResponse(secret, "Bearer", 1));
        } finally {
            System.setErr(originalErr);
            System.setOut(originalOut);
            leaked = captured.toString(java.nio.charset.StandardCharsets.UTF_8);
        }
        assertThat(leaked).contains("canary-line").contains("PLATFORM_TOKEN_NOT_DECLARED");
        assertThat(leaked).doesNotContain(secret);
    }
}
