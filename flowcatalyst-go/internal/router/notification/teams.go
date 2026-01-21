package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// TeamsConfig holds Teams webhook configuration
type TeamsConfig struct {
	WebhookURL string
	Enabled    bool
}

// TeamsService sends Adaptive Cards to Teams channels via webhook
type TeamsService struct {
	config     *TeamsConfig
	httpClient *http.Client
}

// NewTeamsService creates a new Teams webhook notification service
func NewTeamsService(config *TeamsConfig) *TeamsService {
	slog.Info("TeamsWebhookNotificationService initialized",
		"enabled", config.Enabled)

	return &TeamsService{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NotifyWarning sends a Teams notification for a warning
func (s *TeamsService) NotifyWarning(warning *Warning) {
	if !s.config.Enabled {
		return
	}

	adaptiveCard := s.buildAdaptiveCard(warning)
	if err := s.sendToTeams(adaptiveCard); err != nil {
		slog.Error("Failed to send Teams notification for warning",
			"error", err,
			"category", warning.Category)
		return
	}

	slog.Info("Teams notification sent",
		"severity", warning.Severity,
		"category", warning.Category)
}

// NotifyCriticalError sends a Teams notification for a critical error
func (s *TeamsService) NotifyCriticalError(message, source string) {
	if !s.config.Enabled {
		return
	}

	adaptiveCard := s.buildCriticalErrorCard(message, source)
	if err := s.sendToTeams(adaptiveCard); err != nil {
		slog.Error("Failed to send Teams critical error notification", "error", err)
		return
	}

	slog.Info("Teams critical error notification sent")
}

// NotifySystemEvent sends a Teams notification for a system event
func (s *TeamsService) NotifySystemEvent(eventType, message string) {
	if !s.config.Enabled {
		return
	}

	adaptiveCard := s.buildSystemEventCard(eventType, message)
	if err := s.sendToTeams(adaptiveCard); err != nil {
		slog.Error("Failed to send Teams system event notification", "error", err)
		return
	}

	slog.Debug("Teams system event notification sent", "eventType", eventType)
}

// IsEnabled returns whether Teams notifications are enabled
func (s *TeamsService) IsEnabled() bool {
	return s.config.Enabled
}

// sendToTeams sends Adaptive Card JSON to Teams webhook
func (s *TeamsService) sendToTeams(adaptiveCardJSON string) error {
	req, err := http.NewRequest(http.MethodPost, s.config.WebhookURL, bytes.NewBufferString(adaptiveCardJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("teams webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// buildAdaptiveCard builds Adaptive Card for a warning
func (s *TeamsService) buildAdaptiveCard(warning *Warning) string {
	color := getTeamsSeverityColor(warning.Severity)
	timestamp := warning.Timestamp.Format(time.RFC3339)

	card := map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content": map[string]interface{}{
					"type":    "AdaptiveCard",
					"version": "1.4",
					"body": []map[string]interface{}{
						{
							"type":  "Container",
							"style": "emphasis",
							"items": []map[string]interface{}{
								{
									"type": "ColumnSet",
									"columns": []map[string]interface{}{
										{
											"type":  "Column",
											"width": "auto",
											"items": []map[string]interface{}{
												{"type": "TextBlock", "text": "⚠️", "size": "Large"},
											},
										},
										{
											"type":  "Column",
											"width": "stretch",
											"items": []map[string]interface{}{
												{"type": "TextBlock", "text": "FlowCatalyst Alert", "weight": "Bolder", "size": "Large"},
												{"type": "TextBlock", "text": fmt.Sprintf("%s - %s", warning.Severity, warning.Category), "color": color, "weight": "Bolder", "size": "Medium", "spacing": "None"},
											},
										},
									},
								},
							},
						},
						{
							"type": "FactSet",
							"facts": []map[string]interface{}{
								{"title": "Category:", "value": warning.Category},
								{"title": "Source:", "value": warning.Source},
								{"title": "Time:", "value": timestamp},
							},
						},
						{"type": "TextBlock", "text": "Message", "weight": "Bolder", "separator": true},
						{"type": "TextBlock", "text": warning.Message, "wrap": true, "spacing": "Small"},
					},
				},
			},
		},
	}

	jsonBytes, _ := json.Marshal(card)
	return string(jsonBytes)
}

// buildCriticalErrorCard builds Adaptive Card for critical error
func (s *TeamsService) buildCriticalErrorCard(message, source string) string {
	card := map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content": map[string]interface{}{
					"type":    "AdaptiveCard",
					"version": "1.4",
					"body": []map[string]interface{}{
						{
							"type":  "Container",
							"style": "attention",
							"items": []map[string]interface{}{
								{"type": "TextBlock", "text": "🚨 CRITICAL ERROR", "weight": "Bolder", "size": "ExtraLarge", "color": "Attention"},
							},
						},
						{
							"type": "FactSet",
							"facts": []map[string]interface{}{
								{"title": "Source:", "value": source},
							},
						},
						{"type": "TextBlock", "text": message, "wrap": true, "spacing": "Medium"},
						{"type": "TextBlock", "text": "⚡ Immediate action required", "weight": "Bolder", "color": "Attention", "separator": true},
					},
				},
			},
		},
	}

	jsonBytes, _ := json.Marshal(card)
	return string(jsonBytes)
}

// buildSystemEventCard builds Adaptive Card for system event
func (s *TeamsService) buildSystemEventCard(eventType, message string) string {
	card := map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content": map[string]interface{}{
					"type":    "AdaptiveCard",
					"version": "1.4",
					"body": []map[string]interface{}{
						{
							"type":  "Container",
							"style": "accent",
							"items": []map[string]interface{}{
								{"type": "TextBlock", "text": fmt.Sprintf("ℹ️ System Event: %s", eventType), "weight": "Bolder", "size": "Large"},
							},
						},
						{"type": "TextBlock", "text": message, "wrap": true, "spacing": "Medium"},
					},
				},
			},
		},
	}

	jsonBytes, _ := json.Marshal(card)
	return string(jsonBytes)
}

// getTeamsSeverityColor returns Teams color for severity level
func getTeamsSeverityColor(severity string) string {
	switch strings.ToUpper(severity) {
	case "CRITICAL", "ERROR":
		return "Attention"
	case "WARNING":
		return "Warning"
	case "INFO":
		return "Accent"
	default:
		return "Default"
	}
}
