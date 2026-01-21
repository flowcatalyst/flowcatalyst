package tech.flowcatalyst.app;

import io.quarkus.runtime.ShutdownEvent;
import io.quarkus.runtime.StartupEvent;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.event.Observes;

import java.util.logging.Logger;

/**
 * FlowCatalyst App startup handler.
 *
 * <p>This deployment includes:
 * <ul>
 *   <li>flowcatalyst-platform - Auth, OAuth/OIDC, principals, applications</li>
 *   <li>flowcatalyst-stream-processor - Change stream processing (config: stream-processor.enabled)</li>
 *   <li>flowcatalyst-dispatch-scheduler - Job dispatch/scheduling (config: dispatch-scheduler.enabled)</li>
 * </ul>
 *
 * <p>Each module has its own auto-start handler that respects its enabled config.</p>
 */
@ApplicationScoped
public class AppStartup {

    private static final Logger LOG = Logger.getLogger(AppStartup.class.getName());

    void onStart(@Observes StartupEvent event) {
        LOG.info("FlowCatalyst App started");
    }

    void onShutdown(@Observes ShutdownEvent event) {
        LOG.info("FlowCatalyst App shutdown");
    }
}
