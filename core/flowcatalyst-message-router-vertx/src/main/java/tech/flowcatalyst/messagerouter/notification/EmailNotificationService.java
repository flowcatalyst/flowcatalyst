package tech.flowcatalyst.messagerouter.notification;

import io.quarkus.mailer.Mail;
import io.quarkus.mailer.Mailer;
import jakarta.enterprise.inject.Vetoed;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.model.Warning;

import java.time.format.DateTimeFormatter;

/**
 * Email notification service using Quarkus Mailer.
 * Sends formatted HTML emails for warnings and critical errors.
 */
@Vetoed
public class EmailNotificationService implements NotificationService {

    private static final Logger LOG = Logger.getLogger(EmailNotificationService.class);
    private static final DateTimeFormatter DATE_FORMATTER = DateTimeFormatter.ISO_LOCAL_DATE_TIME;

    private final Mailer mailer;
    private final String fromAddress;
    private final String toAddress;
    private final boolean enabled;

    public EmailNotificationService(Mailer mailer, String fromAddress, String toAddress, boolean enabled) {
        this.mailer = mailer;
        this.fromAddress = fromAddress;
        this.toAddress = toAddress;
        this.enabled = enabled;
        LOG.infof("EmailNotificationService initialized (enabled=%s, from=%s, to=%s)",
            enabled, fromAddress, toAddress);
    }

    @Override
    public void notifyWarning(Warning warning) {
        if (!enabled) {
            return;
        }

        try {
            String subject = String.format("[FlowCatalyst] %s - %s",
                warning.severity(), warning.category());

            String htmlBody = buildHtmlEmail(warning);

            mailer.send(
                Mail.withHtml(toAddress, subject, htmlBody)
                    .setFrom(fromAddress)
            );

            LOG.infof("Email notification sent: %s - %s", warning.severity(), warning.category());

        } catch (Exception e) {
            LOG.errorf(e, "Failed to send email notification for warning: %s", warning.category());
        }
    }

    @Override
    public void notifyCriticalError(String message, String source) {
        if (!enabled) {
            return;
        }

        try {
            String subject = "[FlowCatalyst] CRITICAL ERROR";

            String htmlBody = String.format("""
                <html>
                <body style="font-family: Arial, sans-serif;">
                    <div style="background-color: #dc3545; color: white; padding: 20px; border-radius: 5px;">
                        <h2 style="margin: 0;">CRITICAL ERROR</h2>
                    </div>
                    <div style="padding: 20px; background-color: #f8f9fa; margin-top: 10px; border-radius: 5px;">
                        <p><strong>Source:</strong> %s</p>
                        <p><strong>Message:</strong></p>
                        <pre style="background-color: white; padding: 15px; border-left: 4px solid #dc3545;">%s</pre>
                    </div>
                    <div style="margin-top: 20px; padding: 10px; background-color: #fff3cd; border-left: 4px solid #ffc107;">
                        <p style="margin: 0;"><strong>Action Required:</strong> Immediate investigation needed</p>
                    </div>
                </body>
                </html>
                """, source, escapeHtml(message));

            mailer.send(
                Mail.withHtml(toAddress, subject, htmlBody)
                    .setFrom(fromAddress)
            );

            LOG.infof("Critical error email sent to %s", toAddress);

        } catch (Exception e) {
            LOG.errorf(e, "Failed to send critical error email");
        }
    }

    @Override
    public void notifySystemEvent(String eventType, String message) {
        if (!enabled) {
            return;
        }

        try {
            String subject = String.format("[FlowCatalyst] System Event - %s", eventType);

            String htmlBody = String.format("""
                <html>
                <body style="font-family: Arial, sans-serif;">
                    <div style="background-color: #17a2b8; color: white; padding: 20px; border-radius: 5px;">
                        <h2 style="margin: 0;">System Event: %s</h2>
                    </div>
                    <div style="padding: 20px; background-color: #f8f9fa; margin-top: 10px; border-radius: 5px;">
                        <pre style="background-color: white; padding: 15px;">%s</pre>
                    </div>
                </body>
                </html>
                """, eventType, escapeHtml(message));

            mailer.send(
                Mail.withHtml(toAddress, subject, htmlBody)
                    .setFrom(fromAddress)
            );

            LOG.debugf("System event email sent: %s", eventType);

        } catch (Exception e) {
            LOG.errorf(e, "Failed to send system event email");
        }
    }

    @Override
    public boolean isEnabled() {
        return enabled;
    }

    /**
     * Build HTML email body for a warning
     */
    private String buildHtmlEmail(Warning warning) {
        String severityColor = getSeverityColor(warning.severity());
        String timestamp = warning.timestamp().atZone(java.time.ZoneId.systemDefault())
            .format(DATE_FORMATTER);

        return String.format("""
            <html>
            <head>
                <style>
                    body { font-family: Arial, sans-serif; margin: 0; padding: 0; }
                    .header { background-color: %s; color: white; padding: 20px; border-radius: 5px; }
                    .content { padding: 20px; background-color: #f8f9fa; margin-top: 10px; border-radius: 5px; }
                    .metadata { display: flex; flex-wrap: wrap; gap: 20px; margin-bottom: 15px; }
                    .metadata-item { flex: 1; min-width: 200px; }
                    .metadata-label { font-weight: bold; color: #6c757d; }
                    .message { background-color: white; padding: 15px; border-left: 4px solid %s; white-space: pre-wrap; }
                    .footer { margin-top: 20px; padding: 10px; font-size: 12px; color: #6c757d; }
                </style>
            </head>
            <body>
                <div class="header">
                    <h2 style="margin: 0;">%s - %s</h2>
                </div>
                <div class="content">
                    <div class="metadata">
                        <div class="metadata-item">
                            <div class="metadata-label">Category</div>
                            <div>%s</div>
                        </div>
                        <div class="metadata-item">
                            <div class="metadata-label">Source</div>
                            <div>%s</div>
                        </div>
                        <div class="metadata-item">
                            <div class="metadata-label">Timestamp</div>
                            <div>%s</div>
                        </div>
                    </div>
                    <div class="metadata-label">Message</div>
                    <div class="message">%s</div>
                </div>
                <div class="footer">
                    FlowCatalyst Message Router - Automated Notification
                </div>
            </body>
            </html>
            """,
            severityColor,
            severityColor,
            warning.severity(),
            warning.category(),
            warning.category(),
            warning.source(),
            timestamp,
            escapeHtml(warning.message())
        );
    }

    /**
     * Get color for severity level
     */
    private String getSeverityColor(String severity) {
        return switch (severity.toUpperCase()) {
            case "CRITICAL" -> "#dc3545"; // Red
            case "ERROR" -> "#fd7e14";    // Orange
            case "WARNING" -> "#ffc107";  // Yellow
            case "INFO" -> "#17a2b8";     // Cyan
            default -> "#6c757d";         // Gray
        };
    }

    /**
     * Basic HTML escaping
     */
    private String escapeHtml(String text) {
        if (text == null) {
            return "";
        }
        return text
            .replace("&", "&amp;")
            .replace("<", "&lt;")
            .replace(">", "&gt;")
            .replace("\"", "&quot;")
            .replace("'", "&#39;");
    }
}
