package io.flowcatalyst.function;

import java.util.NoSuchElementException;
import java.util.Optional;

/// The function's declared (`describe.config`), non-secret config keys, resolved by the
/// host from the platform's function settings.
public interface Config {

    /// The value for `key`, or empty if it was not declared.
    Optional<String> get(String key);

    /// The value for `key`.
    ///
    /// @throws NoSuchElementException if `key` was not declared
    default String require(String key) {
        return get(key).orElseThrow(() -> new NoSuchElementException("config key not declared: " + key));
    }
}
