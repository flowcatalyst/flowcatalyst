package io.flowcatalyst.ingestor.functions.context;

import io.flowcatalyst.function.Secrets;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;

/// [Secrets] over the desired-state entry's `secrets` map, restricted to the keys this version's describe
/// declares (`describe.secrets`). `toString` prints keys only — never a value, so an accidental
/// `LOG.info("{}", ctx.secrets())` cannot leak one.
public final class MapSecrets implements Secrets {

    private final Map<String, String> values;

    public MapSecrets(Map<String, String> settings, Iterable<String> declared) {
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

    @Override
    public String toString() {
        return "Secrets[keys=" + values.keySet() + "]";
    }
}
