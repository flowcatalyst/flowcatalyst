package tech.flowcatalyst.messagerouter.standby;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.messagerouter.warning.WarningService;
import tech.flowcatalyst.standby.StandbyWarningService;

/**
 * Adapter that bridges the shared standby module's StandbyWarningService
 * to the message-router's WarningService.
 */
@ApplicationScoped
public class StandbyWarningAdapter implements StandbyWarningService {

    @Inject
    WarningService warningService;

    @Override
    public void addWarning(String id, String severity, String title, String message) {
        // The message-router WarningService uses (category, severity, message, source)
        // Map title to category, and combine title+message for the message field
        warningService.addWarning(title, severity, message, "StandbyService");
    }

    @Override
    public void acknowledgeWarning(String id) {
        warningService.acknowledgeWarning(id);
    }
}
