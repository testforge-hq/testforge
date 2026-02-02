package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// WebhookEvent represents a Stripe webhook event
type WebhookEvent struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Created int64           `json:"created"`
	Data    WebhookEventData `json:"data"`
}

// WebhookEventData holds the event data
type WebhookEventData struct {
	Object json.RawMessage `json:"object"`
}

// WebhookHandler handles Stripe webhooks
type WebhookHandler struct {
	webhookSecret string
	subService    *SubscriptionService
	logger        *zap.Logger
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(webhookSecret string, subService *SubscriptionService, logger *zap.Logger) *WebhookHandler {
	return &WebhookHandler{
		webhookSecret: webhookSecret,
		subService:    subService,
		logger:        logger,
	}
}

// HandleWebhook processes incoming Stripe webhooks
func (h *WebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("failed to read webhook body", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Verify signature
	signature := r.Header.Get("Stripe-Signature")
	if err := h.verifySignature(body, signature); err != nil {
		h.logger.Error("invalid webhook signature", zap.Error(err))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Parse event
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		h.logger.Error("failed to parse webhook event", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	h.logger.Info("received Stripe webhook",
		zap.String("event_id", event.ID),
		zap.String("event_type", event.Type),
	)

	// Handle event
	ctx := r.Context()
	if err := h.processEvent(ctx, &event); err != nil {
		h.logger.Error("failed to process webhook event",
			zap.String("event_type", event.Type),
			zap.Error(err),
		)
		// Return 200 anyway to prevent Stripe from retrying
		// We'll handle failures via dead letter queue or manual retry
	}

	w.WriteHeader(http.StatusOK)
}

// verifySignature verifies the Stripe webhook signature
func (h *WebhookHandler) verifySignature(payload []byte, header string) error {
	if h.webhookSecret == "" {
		return nil // Skip verification in development
	}

	// Parse signature header
	// Format: t=timestamp,v1=signature
	parts := strings.Split(header, ",")
	var timestamp string
	var signature string

	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v1":
			signature = kv[1]
		}
	}

	if timestamp == "" || signature == "" {
		return errors.New("invalid signature header format")
	}

	// Verify timestamp is within tolerance (5 minutes)
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return errors.New("invalid timestamp")
	}
	if time.Now().Unix()-ts > 300 {
		return errors.New("timestamp too old")
	}

	// Compute expected signature
	signedPayload := timestamp + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(h.webhookSecret))
	mac.Write([]byte(signedPayload))
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return errors.New("signature mismatch")
	}

	return nil
}

// processEvent processes a webhook event
func (h *WebhookHandler) processEvent(ctx context.Context, event *WebhookEvent) error {
	switch event.Type {
	// Subscription events
	case "customer.subscription.created":
		return h.handleSubscriptionCreated(ctx, event)
	case "customer.subscription.updated":
		return h.handleSubscriptionUpdated(ctx, event)
	case "customer.subscription.deleted":
		return h.handleSubscriptionDeleted(ctx, event)
	case "customer.subscription.trial_will_end":
		return h.handleTrialWillEnd(ctx, event)

	// Invoice events
	case "invoice.payment_succeeded":
		return h.handleInvoicePaid(ctx, event)
	case "invoice.payment_failed":
		return h.handleInvoiceFailed(ctx, event)
	case "invoice.finalized":
		return h.handleInvoiceFinalized(ctx, event)

	// Customer events
	case "customer.updated":
		return h.handleCustomerUpdated(ctx, event)

	// Payment method events
	case "payment_method.attached":
		return h.handlePaymentMethodAttached(ctx, event)
	case "payment_method.detached":
		return h.handlePaymentMethodDetached(ctx, event)

	default:
		h.logger.Debug("unhandled webhook event", zap.String("type", event.Type))
		return nil
	}
}

func (h *WebhookHandler) handleSubscriptionCreated(ctx context.Context, event *WebhookEvent) error {
	var sub Subscription
	if err := json.Unmarshal(event.Data.Object, &sub); err != nil {
		return err
	}

	// Sync from Stripe
	if sub.StripeSubscriptionID != nil {
		return h.subService.SyncFromStripe(ctx, *sub.StripeSubscriptionID)
	}
	return nil
}

func (h *WebhookHandler) handleSubscriptionUpdated(ctx context.Context, event *WebhookEvent) error {
	var stripeSub struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(event.Data.Object, &stripeSub); err != nil {
		return err
	}

	return h.subService.SyncFromStripe(ctx, stripeSub.ID)
}

func (h *WebhookHandler) handleSubscriptionDeleted(ctx context.Context, event *WebhookEvent) error {
	var stripeSub struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(event.Data.Object, &stripeSub); err != nil {
		return err
	}

	return h.subService.SyncFromStripe(ctx, stripeSub.ID)
}

func (h *WebhookHandler) handleTrialWillEnd(ctx context.Context, event *WebhookEvent) error {
	var stripeSub struct {
		ID       string `json:"id"`
		Customer string `json:"customer"`
		TrialEnd int64  `json:"trial_end"`
	}
	if err := json.Unmarshal(event.Data.Object, &stripeSub); err != nil {
		return err
	}

	// TODO: Send trial ending notification email
	h.logger.Info("trial ending soon",
		zap.String("subscription_id", stripeSub.ID),
		zap.String("customer_id", stripeSub.Customer),
		zap.Time("trial_end", time.Unix(stripeSub.TrialEnd, 0)),
	)

	return nil
}

func (h *WebhookHandler) handleInvoicePaid(ctx context.Context, event *WebhookEvent) error {
	var invoice struct {
		ID           string `json:"id"`
		Customer     string `json:"customer"`
		Subscription string `json:"subscription"`
		AmountPaid   int64  `json:"amount_paid"`
		Currency     string `json:"currency"`
	}
	if err := json.Unmarshal(event.Data.Object, &invoice); err != nil {
		return err
	}

	h.logger.Info("invoice paid",
		zap.String("invoice_id", invoice.ID),
		zap.String("customer_id", invoice.Customer),
		zap.Int64("amount", invoice.AmountPaid),
	)

	// Update subscription status
	if invoice.Subscription != "" {
		return h.subService.SyncFromStripe(ctx, invoice.Subscription)
	}

	return nil
}

func (h *WebhookHandler) handleInvoiceFailed(ctx context.Context, event *WebhookEvent) error {
	var invoice struct {
		ID           string `json:"id"`
		Customer     string `json:"customer"`
		Subscription string `json:"subscription"`
		AmountDue    int64  `json:"amount_due"`
		AttemptCount int    `json:"attempt_count"`
	}
	if err := json.Unmarshal(event.Data.Object, &invoice); err != nil {
		return err
	}

	h.logger.Warn("invoice payment failed",
		zap.String("invoice_id", invoice.ID),
		zap.String("customer_id", invoice.Customer),
		zap.Int64("amount_due", invoice.AmountDue),
		zap.Int("attempt_count", invoice.AttemptCount),
	)

	// TODO: Send payment failed notification email
	// TODO: Update subscription status to past_due if needed

	if invoice.Subscription != "" {
		return h.subService.SyncFromStripe(ctx, invoice.Subscription)
	}

	return nil
}

func (h *WebhookHandler) handleInvoiceFinalized(ctx context.Context, event *WebhookEvent) error {
	// Store invoice in database for records
	h.logger.Debug("invoice finalized")
	return nil
}

func (h *WebhookHandler) handleCustomerUpdated(ctx context.Context, event *WebhookEvent) error {
	// Update customer info if needed
	h.logger.Debug("customer updated")
	return nil
}

func (h *WebhookHandler) handlePaymentMethodAttached(ctx context.Context, event *WebhookEvent) error {
	var pm struct {
		ID       string `json:"id"`
		Customer string `json:"customer"`
		Type     string `json:"type"`
	}
	if err := json.Unmarshal(event.Data.Object, &pm); err != nil {
		return err
	}

	h.logger.Info("payment method attached",
		zap.String("payment_method_id", pm.ID),
		zap.String("customer_id", pm.Customer),
		zap.String("type", pm.Type),
	)

	// TODO: Store payment method in database
	return nil
}

func (h *WebhookHandler) handlePaymentMethodDetached(ctx context.Context, event *WebhookEvent) error {
	var pm struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(event.Data.Object, &pm); err != nil {
		return err
	}

	h.logger.Info("payment method detached",
		zap.String("payment_method_id", pm.ID),
	)

	// TODO: Remove payment method from database
	return nil
}

// ServeHTTP implements http.Handler
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	h.HandleWebhook(w, r)
}
