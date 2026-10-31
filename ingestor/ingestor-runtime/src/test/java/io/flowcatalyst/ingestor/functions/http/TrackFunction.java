package io.flowcatalyst.ingestor.functions.http;

import io.flowcatalyst.ingestor.functions.Fx;
import java.util.List;

/// A function that sleeps for the concurrency tests and counts how many calls are inside it at once, in objects it
/// parks in the JDK's system properties (the one place a function and its host share without sharing a class).
final class TrackFunction {

    static final String ENTRYPOINT = "fixture.app.Track";

    private TrackFunction() {
    }

    static final String SOURCE = """
            package fixture.app;
            import io.flowcatalyst.function.*;
            import java.util.concurrent.atomic.AtomicInteger;
            public final class Track implements Function {
                private static AtomicInteger counter(String name) {
                    return (AtomicInteger) System.getProperties().computeIfAbsent(name, k -> new AtomicInteger());
                }
                public Result handle(Request in, FunctionContext ctx) throws Exception {
                    long ms = Long.parseLong(in.pathParams().get("ms"));
                    switch (in.route()) {
                        case "GET /track/{ms}": {
                            AtomicInteger inflight = counter("fx.inflight");
                            int now = inflight.incrementAndGet();
                            counter("fx.max").accumulateAndGet(now, Math::max);
                            try {
                                Thread.sleep(ms);
                            } finally {
                                inflight.decrementAndGet();
                            }
                            return Result.json(200, "{\\"tag\\":\\"%s\\"}");
                        }
                        case "GET /stubborn/{ms}": {
                            // ignores interrupts: keeps running past its deadline
                            long end = System.nanoTime() + ms * 1_000_000L;
                            while (System.nanoTime() < end) {
                                try { Thread.sleep(5); } catch (InterruptedException ignored) { }
                            }
                            System.getProperties().put("fx.stubborn.returned", Boolean.TRUE);
                            return Result.ack();
                        }
                        default:
                            return Result.status(418);
                    }
                }
            }
            """;

    static Fx.Jar jar(String tag) {
        return Fx.jar(ENTRYPOINT, SOURCE.formatted(tag));
    }

    static String describe() {
        return Fx.describe(ENTRYPOINT, List.of(Fx.endpoint("GET", "/track/{ms}", "none"), Fx.endpoint("GET", "/stubborn/{ms}", "none")));
    }
}
