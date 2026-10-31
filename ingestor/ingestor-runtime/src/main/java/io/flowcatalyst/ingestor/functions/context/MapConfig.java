package io.flowcatalyst.ingestor.functions.context;

import io.flowcatalyst.function.Config;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.Set;

/// [Config] over the desired-state entry's `config` map, restricted to the keys this version's describe
/// declares (`describe.config`): an undeclared key is absent, as the Go runner answers it.
/// `require` is [Config]'s own (`NoSuchElementException` naming the key).
public final class MapConfig implements Config {

    private final Map<String, String> values;

    public MapConfig(Map<String, String> settings, Iterable<String> declared) {
        Objects.requireNonNull(settings, "settings");
        Map<String, String> kept = new LinkedHashMap<>();
        for (String key : declared) {
            String value = settings.get(key);
            if (value != null) {
                kept.put(key, value);
            }
        }
        this.values = Map.copyOf(kept);
    }

    @Override
    public Optional<String> get(String key) {
        return Optional.ofNullable(values.get(key));
    }

    /// The keys that have a value, for logs (never the values).
    public Set<String> keys() {
        return values.keySet();
    }

    @Override
    public String toString() {
        return "Config[keys=" + values.keySet() + "]";
    }
}
