package reporting

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// NotificationService handles sending notifications
type NotificationService struct {
	httpClient *http.Client
	logger     *zap.Logger
	baseURL    string // Base URL for report links
}

// NewNotificationService creates a new notification service
func NewNotificationService(baseURL string, logger *zap.Logger) *NotificationService {
	return &NotificationService{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     logger,
		baseURL:    baseURL,
	}
}

// SlackMessage represents a Slack webhook message
type SlackMessage struct {
	Text        string            `json:"text,omitempty"`
	Attachments []SlackAttachment `json:"attachments,omitempty"`
	Blocks      []SlackBlock      `json:"blocks,omitempty"`
}

// SlackAttachment is a Slack message attachment
type SlackAttachment struct {
	Color      string            `json:"color,omitempty"`
	Title      string            `json:"title,omitempty"`
	TitleLink  string            `json:"title_link,omitempty"`
	Text       string            `json:"text,omitempty"`
	Fields     []SlackField      `json:"fields,omitempty"`
	Footer     string            `json:"footer,omitempty"`
	FooterIcon string            `json:"footer_icon,omitempty"`
	Ts         json.Number       `json:"ts,omitempty"`
}

// SlackField is a field in a Slack attachment
type SlackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// SlackBlock is a Slack block element
type SlackBlock struct {
	Type string      `json:"type"`
	Text *SlackText  `json:"text,omitempty"`
}

// SlackText is text content in a block
type SlackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// NotifySlack sends a notification to Slack
func (n *NotificationService) NotifySlack(ctx context.Context, report *TestRunReport, config *SlackConfig) error {
	// Check if we should send based on config
	if !config.OnSuccess && report.Executive.Status == "passed" {
		return nil
	}
	if !config.OnFailure && report.Executive.Status != "passed" {
		return nil
	}

	// Build Slack message
	var color string
	var emoji string
	if report.Executive.DeploymentSafe {
		color = "#36a64f" // green
		emoji = "✅"
	} else {
		color = "#dc3545" // red
		emoji = "❌"
	}

	// Build fields
	fields := []SlackField{
		{Title: "Tests", Value: fmt.Sprintf("%d passed, %d failed", report.Executive.Passed, report.Executive.Failed), Short: true},
		{Title: "Health", Value: fmt.Sprintf("%.0f%%", report.Executive.HealthScore), Short: true},
		{Title: "Duration", Value: report.Executive.Duration, Short: true},
		{Title: "Risk", Value: report.Executive.RiskLevel, Short: true},
	}

	// Add healing info if applicable
	if report.Healing != nil && report.Healing.Healed > 0 {
		fields = append(fields, SlackField{
			Title: "Auto-Healed",
			Value: fmt.Sprintf("🔧 %d tests", report.Healing.Healed),
			Short: true,
		})
	}

	// Add deployment status
	deployStatus := "✓ Safe to Deploy"
	if !report.Executive.DeploymentSafe {
		deployStatus = "✗ Deployment Blocked"
	}
	fields = append(fields, SlackField{
		Title: "Deployment",
		Value: deployStatus,
		Short: true,
	})

	reportURL := fmt.Sprintf("%s/runs/%s/report", n.baseURL, report.RunID)

	msg := SlackMessage{
		Attachments: []SlackAttachment{
			{
				Color:     color,
				Title:     fmt.Sprintf("%s Test Run: %s", emoji, report.Executive.Status),
				TitleLink: reportURL,
				Text:      report.Executive.OneLiner,
				Fields:    fields,
				Footer:    "TestForge",
				Ts:        json.Number(fmt.Sprintf("%d", time.Now().Unix())),
			},
		},
	}

	// Add AI recommendations if available
	if report.AIInsights != nil && len(report.AIInsights.Recommendations) > 0 {
		recText := "*AI Recommendations:*\n"
		for i, rec := range report.AIInsights.Recommendations {
			if i >= 3 {
				break
			}
			recText += fmt.Sprintf("• %s\n", rec.Title)
		}
		msg.Attachments = append(msg.Attachments, SlackAttachment{
			Color: "#6366f1",
			Text:  recText,
		})
	}

	return n.sendSlackWebhook(ctx, config.WebhookURL, &msg)
}

// sendSlackWebhook sends a message to a Slack webhook
func (n *NotificationService) sendSlackWebhook(ctx context.Context, webhookURL string, msg *SlackMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Slack message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Slack webhook returned status %d", resp.StatusCode)
	}

	n.logger.Info("Slack notification sent",
		zap.String("status", "success"))

	return nil
}

// WebhookPayload is the payload sent to generic webhooks
type WebhookPayload struct {
	Event          string  `json:"event"`
	RunID          string  `json:"run_id"`
	ProjectID      string  `json:"project_id"`
	Status         string  `json:"status"`
	HealthScore    float64 `json:"health_score"`
	Passed         int     `json:"passed"`
	Failed         int     `json:"failed"`
	Healed         int     `json:"healed"`
	DeploymentSafe bool    `json:"deployment_safe"`
	ReportURL      string  `json:"report_url"`
	Timestamp      string  `json:"timestamp"`
}

// SendWebhook sends a notification to a generic webhook
func (n *NotificationService) SendWebhook(ctx context.Context, report *TestRunReport, config *WebhookConfig) error {
	healed := 0
	if report.Healing != nil {
		healed = report.Healing.Healed
	}

	payload := WebhookPayload{
		Event:          "test_run_completed",
		RunID:          report.RunID,
		ProjectID:      report.ProjectID,
		Status:         report.Executive.Status,
		HealthScore:    report.Executive.HealthScore,
		Passed:         report.Executive.Passed,
		Failed:         report.Executive.Failed,
		Healed:         healed,
		DeploymentSafe: report.Executive.DeploymentSafe,
		ReportURL:      fmt.Sprintf("%s/runs/%s/report", n.baseURL, report.RunID),
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", config.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "TestForge/1.0")

	// Add custom headers
	for k, v := range config.Headers {
		req.Header.Set(k, v)
	}

	// Add signature if secret is configured
	if config.Secret != "" {
		sig := hmac.New(sha256.New, []byte(config.Secret))
		sig.Write(body)
		req.Header.Set("X-TestForge-Signature", hex.EncodeToString(sig.Sum(nil)))
	}

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	n.logger.Info("Webhook notification sent",
		zap.String("url", config.URL),
		zap.Int("status", resp.StatusCode))

	return nil
}

// SendEmail sends an email notification (placeholder)
func (n *NotificationService) SendEmail(ctx context.Context, report *TestRunReport, config *EmailConfig) error {
	// TODO: Implement email sending via SMTP or service like SendGrid
	n.logger.Info("Email notification skipped (not implemented)",
		zap.Strings("recipients", config.Recipients))
	return nil
}

// NotifyAll sends notifications to all configured channels
func (n *NotificationService) NotifyAll(ctx context.Context, report *TestRunReport, config *NotificationConfig) error {
	var lastErr error

	if config.Slack != nil {
		if err := n.NotifySlack(ctx, report, config.Slack); err != nil {
			n.logger.Error("Failed to send Slack notification", zap.Error(err))
			lastErr = err
		}
	}

	if config.Webhook != nil {
		if err := n.SendWebhook(ctx, report, config.Webhook); err != nil {
			n.logger.Error("Failed to send webhook notification", zap.Error(err))
			lastErr = err
		}
	}

	if config.Email != nil {
		if err := n.SendEmail(ctx, report, config.Email); err != nil {
			n.logger.Error("Failed to send email notification", zap.Error(err))
			lastErr = err
		}
	}

	return lastErr
}
