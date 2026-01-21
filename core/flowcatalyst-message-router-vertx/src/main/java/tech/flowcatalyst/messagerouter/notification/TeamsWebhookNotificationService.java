package tech.flowcatalyst.messagerouter.notification;

import jakarta.enterprise.inject.Vetoed;
import jakarta.ws.rs.client.Client;
import jakarta.ws.rs.client.Entity;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.model.Warning;

import java.time.format.DateTimeFormatter;

/**
 * Microsoft Teams webhook notification service.
 * Sends Adaptive Cards to Teams channels via webhook.
 */
@Vetoed
public class TeamsWebhookNotificationService implements NotificationService {

    private static final Logger LOG = Logger.getLogger(TeamsWebhookNotificationService.class);
    private static final DateTimeFormatter DATE_FORMATTER = DateTimeFormatter.ISO_LOCAL_DATE_TIME;

    private final Client httpClient;
    private final String webhookUrl;
    private final boolean enabled;

    public TeamsWebhookNotificationService(Client httpClient, String webhookUrl, boolean enabled) {
        this.httpClient = httpClient;
        this.webhookUrl = webhookUrl;
        this.enabled = enabled;
        LOG.infof("TeamsWebhookNotificationService initialized (enabled=%s)", enabled);
    }

    @Override
    public void notifyWarning(Warning warning) {
        if (!enabled) {
            return;
        }

        try {
            String adaptiveCard = buildAdaptiveCard(warning);
            sendToTeams(adaptiveCard);

            LOG.infof("Teams notification sent: %s - %s", warning.severity(), warning.category());

        } catch (Exception e) {
            LOG.errorf(e, "Failed to send Teams notification for warning: %s", warning.category());
        }
    }

    @Override
    public void notifyCriticalError(String message, String source) {
        if (!enabled) {
            return;
        }

        try {
            String adaptiveCard = buildCriticalErrorCard(message, source);
            sendToTeams(adaptiveCard);

            LOG.infof("Teams critical error notification sent");

        } catch (Exception e) {
            LOG.errorf(e, "Failed to send Teams critical error notification");
        }
    }

    @Override
    public void notifySystemEvent(String eventType, String message) {
        if (!enabled) {
            return;
        }

        try {
            String adaptiveCard = buildSystemEventCard(eventType, message);
            sendToTeams(adaptiveCard);

            LOG.debugf("Teams system event notification sent: %s", eventType);

        } catch (Exception e) {
            LOG.errorf(e, "Failed to send Teams system event notification");
        }
    }

    @Override
    public boolean isEnabled() {
        return enabled;
    }

    /**
     * Send Adaptive Card JSON to Teams webhook
     */
    private void sendToTeams(String adaptiveCardJson) {
        try (Response response = httpClient.target(webhookUrl)
            .request(MediaType.APPLICATION_JSON)
            .post(Entity.json(adaptiveCardJson))) {

            if (response.getStatus() >= 400) {
                LOG.errorf("Teams webhook returned error: %d - %s",
                    response.getStatus(), response.readEntity(String.class));
            }
        } catch (Exception e) {
            LOG.errorf(e, "Error sending to Teams webhook");
            throw e;
        }
    }

    /**
     * Build Adaptive Card for a warning
     */
    private String buildAdaptiveCard(Warning warning) {
        String color = getSeverityColor(warning.severity());
        String timestamp = warning.timestamp().atZone(java.time.ZoneId.systemDefault())
            .format(DATE_FORMATTER);

        // Escape JSON strings
        String escapedMessage = escapeJson(warning.message());
        String escapedCategory = escapeJson(warning.category());
        String escapedSource = escapeJson(warning.source());

        return String.format("""
            {
                "attachments": [
                    {
                        "contentType": "application/vnd.microsoft.card.adaptive",
                        "content": {
                            "type": "AdaptiveCard",
                            "version": "1.4",
                            "body": [
                                {
                                    "type": "Container",
                                    "style": "emphasis",
                                    "items": [
                                        {
                                            "type": "ColumnSet",
                                            "columns": [
                                                {
                                                    "type": "Column",
                                                    "width": "auto",
                                                    "items": [
                                                        {
                                                            "type": "TextBlock",
                                                            "text": "⚠️",
                                                            "size": "Large"
                                                        }
                                                    ]
                                                },
                                                {
                                                    "type": "Column",
                                                    "width": "stretch",
                                                    "items": [
                                                        {
                                                            "type": "TextBlock",
                                                            "text": "FlowCatalyst Alert",
                                                            "weight": "Bolder",
                                                            "size": "Large"
                                                        },
                                                        {
                                                            "type": "TextBlock",
                                                            "text": "%s - %s",
                                                            "color": "%s",
                                                            "weight": "Bolder",
                                                            "size": "Medium",
                                                            "spacing": "None"
                                                        }
                                                    ]
                                                }
                                            ]
                                        }
                                    ]
                                },
                                {
                                    "type": "FactSet",
                                    "facts": [
                                        {
                                            "title": "Category:",
                                            "value": "%s"
                                        },
                                        {
                                            "title": "Source:",
                                            "value": "%s"
                                        },
                                        {
                                            "title": "Time:",
                                            "value": "%s"
                                        }
                                    ]
                                },
                                {
                                    "type": "TextBlock",
                                    "text": "Message",
                                    "weight": "Bolder",
                                    "separator": true
                                },
                                {
                                    "type": "TextBlock",
                                    "text": "%s",
                                    "wrap": true,
                                    "spacing": "Small"
                                }
                            ]
                        }
                    }
                ]
            }
            """,
            warning.severity(), escapedCategory,
            color,
            escapedCategory,
            escapedSource,
            timestamp,
            escapedMessage
        );
    }

    /**
     * Build Adaptive Card for critical error
     */
    private String buildCriticalErrorCard(String message, String source) {
        String escapedMessage = escapeJson(message);
        String escapedSource = escapeJson(source);

        return String.format("""
            {
                "attachments": [
                    {
                        "contentType": "application/vnd.microsoft.card.adaptive",
                        "content": {
                            "type": "AdaptiveCard",
                            "version": "1.4",
                            "body": [
                                {
                                    "type": "Container",
                                    "style": "attention",
                                    "items": [
                                        {
                                            "type": "TextBlock",
                                            "text": "🚨 CRITICAL ERROR",
                                            "weight": "Bolder",
                                            "size": "ExtraLarge",
                                            "color": "Attention"
                                        }
                                    ]
                                },
                                {
                                    "type": "FactSet",
                                    "facts": [
                                        {
                                            "title": "Source:",
                                            "value": "%s"
                                        }
                                    ]
                                },
                                {
                                    "type": "TextBlock",
                                    "text": "%s",
                                    "wrap": true,
                                    "spacing": "Medium"
                                },
                                {
                                    "type": "TextBlock",
                                    "text": "⚡ Immediate action required",
                                    "weight": "Bolder",
                                    "color": "Attention",
                                    "separator": true
                                }
                            ]
                        }
                    }
                ]
            }
            """,
            escapedSource,
            escapedMessage
        );
    }

    /**
     * Build Adaptive Card for system event
     */
    private String buildSystemEventCard(String eventType, String message) {
        String escapedEventType = escapeJson(eventType);
        String escapedMessage = escapeJson(message);

        return String.format("""
            {
                "attachments": [
                    {
                        "contentType": "application/vnd.microsoft.card.adaptive",
                        "content": {
                            "type": "AdaptiveCard",
                            "version": "1.4",
                            "body": [
                                {
                                    "type": "Container",
                                    "style": "accent",
                                    "items": [
                                        {
                                            "type": "TextBlock",
                                            "text": "ℹ️ System Event: %s",
                                            "weight": "Bolder",
                                            "size": "Large"
                                        }
                                    ]
                                },
                                {
                                    "type": "TextBlock",
                                    "text": "%s",
                                    "wrap": true,
                                    "spacing": "Medium"
                                }
                            ]
                        }
                    }
                ]
            }
            """,
            escapedEventType,
            escapedMessage
        );
    }

    /**
     * Get Teams color for severity level
     */
    private String getSeverityColor(String severity) {
        return switch (severity.toUpperCase()) {
            case "CRITICAL", "ERROR" -> "Attention";
            case "WARNING" -> "Warning";
            case "INFO" -> "Accent";
            default -> "Default";
        };
    }

    /**
     * Escape JSON string
     */
    private String escapeJson(String text) {
        if (text == null) {
            return "";
        }
        return text
            .replace("\\", "\\\\")
            .replace("\"", "\\\"")
            .replace("\n", "\\n")
            .replace("\r", "\\r")
            .replace("\t", "\\t");
    }
}
