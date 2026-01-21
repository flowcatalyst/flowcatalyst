package notification

import (
	"fmt"
	"html"
	"log/slog"
	"net/smtp"
	"strings"
	"time"
)

// EmailConfig holds email notification configuration
type EmailConfig struct {
	SMTPHost    string
	SMTPPort    int
	Username    string
	Password    string
	FromAddress string
	ToAddress   string
	Enabled     bool
}

// EmailService sends formatted HTML emails for warnings and critical errors
type EmailService struct {
	config *EmailConfig
	auth   smtp.Auth
}

// NewEmailService creates a new email notification service
func NewEmailService(config *EmailConfig) *EmailService {
	svc := &EmailService{
		config: config,
	}

	if config.Username != "" && config.Password != "" {
		svc.auth = smtp.PlainAuth("", config.Username, config.Password, config.SMTPHost)
	}

	slog.Info("EmailNotificationService initialized",
		"enabled", config.Enabled,
		"from", config.FromAddress,
		"to", config.ToAddress)

	return svc
}

// NotifyWarning sends an email notification for a warning
func (s *EmailService) NotifyWarning(warning *Warning) {
	if !s.config.Enabled {
		return
	}

	subject := fmt.Sprintf("[FlowCatalyst] %s - %s", warning.Severity, warning.Category)
	htmlBody := s.buildHtmlEmail(warning)

	if err := s.sendMail(subject, htmlBody); err != nil {
		slog.Error("Failed to send email notification for warning",
			"error", err,
			"category", warning.Category)
		return
	}

	slog.Info("Email notification sent",
		"severity", warning.Severity,
		"category", warning.Category)
}

// NotifyCriticalError sends an email for a critical error
func (s *EmailService) NotifyCriticalError(message, source string) {
	if !s.config.Enabled {
		return
	}

	subject := "[FlowCatalyst] CRITICAL ERROR"
	htmlBody := fmt.Sprintf(`
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
`, html.EscapeString(source), html.EscapeString(message))

	if err := s.sendMail(subject, htmlBody); err != nil {
		slog.Error("Failed to send critical error email", "error", err)
		return
	}

	slog.Info("Critical error email sent", "to", s.config.ToAddress)
}

// NotifySystemEvent sends an email for a system event
func (s *EmailService) NotifySystemEvent(eventType, message string) {
	if !s.config.Enabled {
		return
	}

	subject := fmt.Sprintf("[FlowCatalyst] System Event - %s", eventType)
	htmlBody := fmt.Sprintf(`
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
`, html.EscapeString(eventType), html.EscapeString(message))

	if err := s.sendMail(subject, htmlBody); err != nil {
		slog.Error("Failed to send system event email", "error", err)
		return
	}

	slog.Debug("System event email sent", "eventType", eventType)
}

// IsEnabled returns whether email notifications are enabled
func (s *EmailService) IsEnabled() bool {
	return s.config.Enabled
}

// sendMail sends an HTML email
func (s *EmailService) sendMail(subject, htmlBody string) error {
	headers := make(map[string]string)
	headers["From"] = s.config.FromAddress
	headers["To"] = s.config.ToAddress
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"

	var msg strings.Builder
	for k, v := range headers {
		msg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)

	addr := fmt.Sprintf("%s:%d", s.config.SMTPHost, s.config.SMTPPort)
	return smtp.SendMail(addr, s.auth, s.config.FromAddress, []string{s.config.ToAddress}, []byte(msg.String()))
}

// buildHtmlEmail builds HTML email body for a warning
func (s *EmailService) buildHtmlEmail(warning *Warning) string {
	severityColor := getSeverityColor(warning.Severity)
	timestamp := warning.Timestamp.Format(time.RFC3339)

	return fmt.Sprintf(`
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
`,
		severityColor,
		severityColor,
		warning.Severity,
		html.EscapeString(warning.Category),
		html.EscapeString(warning.Category),
		html.EscapeString(warning.Source),
		timestamp,
		html.EscapeString(warning.Message))
}

// getSeverityColor returns color for severity level
func getSeverityColor(severity string) string {
	switch strings.ToUpper(severity) {
	case "CRITICAL":
		return "#dc3545" // Red
	case "ERROR":
		return "#fd7e14" // Orange
	case "WARNING":
		return "#ffc107" // Yellow
	case "INFO":
		return "#17a2b8" // Cyan
	default:
		return "#6c757d" // Gray
	}
}
