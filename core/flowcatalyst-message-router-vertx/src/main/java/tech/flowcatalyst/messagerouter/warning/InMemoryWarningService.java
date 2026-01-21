package tech.flowcatalyst.messagerouter.warning;

import jakarta.enterprise.context.ApplicationScoped;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.model.Warning;

import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ConcurrentMap;
import java.util.stream.Collectors;

@ApplicationScoped
public class InMemoryWarningService implements WarningService {

    private static final Logger LOG = Logger.getLogger(InMemoryWarningService.class);
    private static final int MAX_WARNINGS = 1000;

    private final ConcurrentMap<String, Warning> warnings = new ConcurrentHashMap<>();

    @Override
    public void addWarning(String category, String severity, String message, String source) {
        // Limit warning storage
        if (warnings.size() >= MAX_WARNINGS) {
            // Remove oldest warning
            warnings.entrySet().stream()
                .min((e1, e2) -> e1.getValue().timestamp().compareTo(e2.getValue().timestamp()))
                .ifPresent(entry -> warnings.remove(entry.getKey()));
        }

        String warningId = UUID.randomUUID().toString();
        Warning warning = new Warning(
            warningId,
            category,
            severity,
            message,
            Instant.now(),
            source,
            false
        );

        warnings.put(warningId, warning);
        LOG.infof("Warning added: [%s] %s - %s - %s", severity, category, source, message);
    }

    @Override
    public List<Warning> getAllWarnings() {
        return warnings.values().stream()
            .sorted((w1, w2) -> w2.timestamp().compareTo(w1.timestamp())) // Newest first
            .collect(Collectors.toList());
    }

    @Override
    public List<Warning> getWarningsBySeverity(String severity) {
        return warnings.values().stream()
            .filter(w -> w.severity().equalsIgnoreCase(severity))
            .sorted((w1, w2) -> w2.timestamp().compareTo(w1.timestamp()))
            .collect(Collectors.toList());
    }

    @Override
    public List<Warning> getUnacknowledgedWarnings() {
        return warnings.values().stream()
            .filter(w -> !w.acknowledged())
            .sorted((w1, w2) -> w2.timestamp().compareTo(w1.timestamp()))
            .collect(Collectors.toList());
    }

    @Override
    public boolean acknowledgeWarning(String warningId) {
        Warning existing = warnings.get(warningId);
        if (existing != null) {
            Warning acknowledged = new Warning(
                existing.id(),
                existing.category(),
                existing.severity(),
                existing.message(),
                existing.timestamp(),
                existing.source(),
                true
            );
            warnings.put(warningId, acknowledged);
            LOG.infof("Warning acknowledged: %s", warningId);
            return true;
        }
        return false;
    }

    @Override
    public void clearAllWarnings() {
        int count = warnings.size();
        warnings.clear();
        LOG.infof("Cleared all warnings: %d warnings removed", count);
    }

    @Override
    public void clearOldWarnings(int hoursOld) {
        Instant threshold = Instant.now().minus(hoursOld, ChronoUnit.HOURS);
        List<String> toRemove = warnings.values().stream()
            .filter(w -> w.timestamp().isBefore(threshold))
            .map(Warning::id)
            .collect(Collectors.toList());

        toRemove.forEach(warnings::remove);
        LOG.infof("Cleared %d warnings older than %d hours", toRemove.size(), hoursOld);
    }
}
