package io.flowcatalyst.ingestor.functions.load;

import java.util.function.Supplier;

/// L5 fixture (docs/jvm-functions-in-ingestor.md §3): the **host's**
/// `java.util.function.Supplier` provider, registered under
/// `META-INF/services/java.util.function.Supplier` on `function-host`'s
/// own test class path. `ServiceLoaderTest` asserts a function's own
/// unqualified `ServiceLoader.load(Supplier.class)` never finds this one.
public final class HostSupplier implements Supplier<String> {

    @Override
    public String get() {
        return "host";
    }
}
