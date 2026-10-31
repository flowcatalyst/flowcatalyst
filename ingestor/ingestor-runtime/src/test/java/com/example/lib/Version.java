package com.example.lib;

/// L2 fixture (docs/jvm-functions-in-ingestor.md §3): the **host's** copy
/// of `com.example.lib.Version`, v1 — on `function-host`'s own test class
/// path, so it is what `FunctionParentLoader`'s mutant (L1, the host
/// application loader as parent) would resolve instead of the fixture
/// jar's v2 copy. Deliberately lacks `v2Only()`.
public final class Version {

    public String v1Only() {
        return "v1";
    }
}
