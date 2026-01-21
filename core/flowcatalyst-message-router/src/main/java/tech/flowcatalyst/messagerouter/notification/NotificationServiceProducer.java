package tech.flowcatalyst.messagerouter.notification;

import io.quarkus.mailer.Mailer;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.inject.Produces;
import jakarta.inject.Inject;
import jakarta.ws.rs.client.Client;
import jakarta.ws.rs.client.ClientBuilder;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;

import java.util.ArrayList;
import java.util.List;
import java.util.Optional;

/**
 * Produces the appropriate NotificationService implementation based on configuration.
 * Supports multiple notification channels (email, Teams) with batching.
 */
@ApplicationScoped
public class NotificationServiceProducer {

    private static final Logger LOG = Logger.getLogger(NotificationServiceProducer.class);

    @Inject
    Mailer mailer;

    @Inject
    NoOpNotificationService noOpService;

    @ConfigProperty(name = "notification.enabled", defaultValue = "true")
    boolean notificationEnabled;

    @ConfigProperty(name = "notification.min.severity", defaultValue = "WARNING")
    String minSeverity;

    // Email configuration
    @ConfigProperty(name = "notification.email.enabled", defaultValue = "false")
    boolean emailEnabled;

    @ConfigProperty(name = "notification.email.from")
    Optional<String> emailFrom;

    @ConfigProperty(name = "notification.email.to")
    Optional<String> emailTo;

    // Teams configuration
    @ConfigProperty(name = "notification.teams.enabled", defaultValue = "false")
    boolean teamsEnabled;

    @ConfigProperty(name = "notification.teams.webhook.url")
    Optional<String> teamsWebhookUrl;

    @Produces
    @ApplicationScoped
    public BatchingNotificationService produceBatchingService() {
        List<NotificationService> delegates = new ArrayList<>();

        if (!notificationEnabled) {
            LOG.info("Notifications disabled - using NoOpNotificationService only");
            delegates.add(noOpService);
            return new BatchingNotificationService(delegates, minSeverity);
        }

        // Add email notification if enabled
        if (emailEnabled) {
            if (emailFrom.isPresent() && emailTo.isPresent()) {
                EmailNotificationService emailService = new EmailNotificationService(
                    mailer,
                    emailFrom.get(),
                    emailTo.get(),
                    true
                );
                delegates.add(emailService);
                LOG.infof("Email notifications enabled: from=%s, to=%s", emailFrom.get(), emailTo.get());
            } else {
                LOG.warn("Email notifications enabled but from/to addresses not configured - skipping email");
            }
        }

        // Add Teams notification if enabled
        if (teamsEnabled) {
            if (teamsWebhookUrl.isPresent() && !teamsWebhookUrl.get().isBlank()) {
                Client httpClient = ClientBuilder.newClient();
                TeamsWebhookNotificationService teamsService = new TeamsWebhookNotificationService(
                    httpClient,
                    teamsWebhookUrl.get(),
                    true
                );
                delegates.add(teamsService);
                LOG.info("Teams webhook notifications enabled");
            } else {
                LOG.warn("Teams notifications enabled but webhook URL not configured - skipping Teams");
            }
        }

        // If no delegates were configured, use NoOp
        if (delegates.isEmpty()) {
            LOG.info("No notification channels configured - using NoOpNotificationService");
            delegates.add(noOpService);
        }

        // Wrap delegates in batching service
        LOG.infof("Creating BatchingNotificationService with %d delegates (min severity: %s)",
            delegates.size(), minSeverity);

        return new BatchingNotificationService(delegates, minSeverity);
    }
}
