package io.flowcatalyst.sdk.outbox;

import java.nio.file.Path;

/** Lets other test packages find the framework checkout the way the outbox conformance tests do. */
public final class PushOutboxConformanceTestAccess {
    private PushOutboxConformanceTestAccess() {}

    public static Path frameworkRoot() {
        return PushOutboxConformanceTest.frameworkRoot();
    }
}
