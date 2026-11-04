package io.flowcatalyst.ingestor.functions.context;

import io.flowcatalyst.function.PlatformTokenException;
import java.time.Clock;
import java.time.Duration;
import java.util.Objects;

/// One loaded version's `platformToken`: refuses use the version's describe did not declare (`NOT_DECLARED`, before
/// any platform call), refuses once the invocation's deadline has passed (`DEADLINE`), and otherwise asks the
/// host's [PlatformTokenCache] for the function.
final class PlatformTokenSource {

    private final PlatformTokenCache cache;
    private final String functionId;
    private final int version;
    private final boolean declared;
    private final Clock clock;

    PlatformTokenSource(PlatformTokenCache cache, String functionId, int version, boolean declared, Clock clock) {
        this.cache = Objects.requireNonNull(cache, "cache");
        this.functionId = Objects.requireNonNull(functionId, "functionId");
        this.version = version;
        this.declared = declared;
        this.clock = Objects.requireNonNull(clock, "clock");
    }

    String token(boolean refresh) {
        if (!declared) {
            throw new PlatformTokenException("NOT_DECLARED", "this version's describe does not declare platformToken");
        }
        if (InvocationDeadline.remaining(clock).equals(Duration.ZERO)) {
            throw new PlatformTokenException("DEADLINE", "the invocation's deadline has passed");
        }
        return cache.token(functionId, version, refresh);
    }
}
