package billing

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewWebhookHandler(t *testing.T) {
	logger := zap.NewNop()
	handler := NewWebhookHandler("whsec_test", nil, logger)

	assert.NotNil(t, handler)
	assert.Equal(t, "whsec_test", handler.webhookSecret)
	assert.Equal(t, logger, handler.logger)
}

func TestWebhookEvent_Struct(t *testing.T) {
	event := WebhookEvent{
		ID:      "evt_123",
		Type:    "customer.subscription.created",
		Created: 1700000000,
		Data: WebhookEventData{
			Object: json.RawMessage(`{"id":"sub_123","status":"active"}`),
		},
	}

	assert.Equal(t, "evt_123", event.ID)
	assert.Equal(t, "customer.subscription.created", event.Type)
	assert.Equal(t, int64(1700000000), event.Created)
	assert.NotNil(t, event.Data.Object)
}

func TestWebhookHandler_VerifySignature(t *testing.T) {
	logger := zap.NewNop()
	secret := "whsec_test_secret"
	handler := NewWebhookHandler(secret, nil, logger)

	t.Run("valid signature", func(t *testing.T) {
		payload := []byte(`{"id":"evt_123"}`)
		timestamp := fmt.Sprintf("%d", time.Now().Unix())

		// Compute signature
		signedPayload := timestamp + "." + string(payload)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(signedPayload))
		signature := hex.EncodeToString(mac.Sum(nil))

		header := fmt.Sprintf("t=%s,v1=%s", timestamp, signature)

		err := handler.verifySignature(payload, header)
		assert.NoError(t, err)
	})

	t.Run("invalid signature", func(t *testing.T) {
		payload := []byte(`{"id":"evt_123"}`)
		timestamp := fmt.Sprintf("%d", time.Now().Unix())
		header := fmt.Sprintf("t=%s,v1=invalid_signature", timestamp)

		err := handler.verifySignature(payload, header)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signature mismatch")
	})

	t.Run("missing timestamp", func(t *testing.T) {
		payload := []byte(`{"id":"evt_123"}`)
		header := "v1=some_signature"

		err := handler.verifySignature(payload, header)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid signature header format")
	})

	t.Run("missing signature", func(t *testing.T) {
		payload := []byte(`{"id":"evt_123"}`)
		timestamp := fmt.Sprintf("%d", time.Now().Unix())
		header := fmt.Sprintf("t=%s", timestamp)

		err := handler.verifySignature(payload, header)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid signature header format")
	})

	t.Run("timestamp too old", func(t *testing.T) {
		payload := []byte(`{"id":"evt_123"}`)
		oldTimestamp := fmt.Sprintf("%d", time.Now().Unix()-400) // More than 5 minutes ago

		signedPayload := oldTimestamp + "." + string(payload)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(signedPayload))
		signature := hex.EncodeToString(mac.Sum(nil))

		header := fmt.Sprintf("t=%s,v1=%s", oldTimestamp, signature)

		err := handler.verifySignature(payload, header)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "timestamp too old")
	})

	t.Run("invalid timestamp format", func(t *testing.T) {
		payload := []byte(`{"id":"evt_123"}`)
		header := "t=not_a_number,v1=some_signature"

		err := handler.verifySignature(payload, header)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid timestamp")
	})

	t.Run("empty secret skips verification", func(t *testing.T) {
		handlerNoSecret := NewWebhookHandler("", nil, logger)
		payload := []byte(`{"id":"evt_123"}`)

		err := handlerNoSecret.verifySignature(payload, "any_header")
		assert.NoError(t, err)
	})
}

func TestWebhookHandler_HandleWebhook(t *testing.T) {
	logger := zap.NewNop()

	t.Run("invalid method returns 405", func(t *testing.T) {
		handler := NewWebhookHandler("", nil, logger)

		req := httptest.NewRequest("GET", "/webhook", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("invalid signature returns 401", func(t *testing.T) {
		handler := NewWebhookHandler("whsec_test", nil, logger)

		body := bytes.NewBufferString(`{"id":"evt_123","type":"test"}`)
		req := httptest.NewRequest("POST", "/webhook", body)
		req.Header.Set("Stripe-Signature", "invalid")
		rec := httptest.NewRecorder()

		handler.HandleWebhook(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		handler := NewWebhookHandler("", nil, logger) // Skip signature verification

		body := bytes.NewBufferString(`not json`)
		req := httptest.NewRequest("POST", "/webhook", body)
		rec := httptest.NewRecorder()

		handler.HandleWebhook(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("valid event returns 200", func(t *testing.T) {
		handler := NewWebhookHandler("", nil, logger) // Skip signature verification

		event := WebhookEvent{
			ID:   "evt_123",
			Type: "customer.updated", // Unhandled event type
			Data: WebhookEventData{
				Object: json.RawMessage(`{"id":"cus_123"}`),
			},
		}
		body, _ := json.Marshal(event)

		req := httptest.NewRequest("POST", "/webhook", bytes.NewBuffer(body))
		rec := httptest.NewRecorder()

		handler.HandleWebhook(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestWebhookHandler_ProcessEvents(t *testing.T) {
	logger := zap.NewNop()

	// Test events that don't require SubscriptionService
	// Events like customer.updated, invoice.finalized, payment_method.* just log
	eventTypes := []string{
		"invoice.finalized",
		"customer.updated",
		"payment_method.attached",
		"payment_method.detached",
		"unknown.event.type",
	}

	for _, eventType := range eventTypes {
		t.Run(eventType, func(t *testing.T) {
			handler := NewWebhookHandler("", nil, logger)

			var eventData json.RawMessage
			switch {
			case eventType == "payment_method.attached" ||
				eventType == "payment_method.detached":
				eventData = json.RawMessage(`{"id":"pm_123","customer":"cus_123","type":"card"}`)
			default:
				eventData = json.RawMessage(`{"id":"obj_123"}`)
			}

			event := WebhookEvent{
				ID:   "evt_123",
				Type: eventType,
				Data: WebhookEventData{Object: eventData},
			}
			body, _ := json.Marshal(event)

			req := httptest.NewRequest("POST", "/webhook", bytes.NewBuffer(body))
			rec := httptest.NewRecorder()

			handler.HandleWebhook(rec, req)
			// Should return 200 for all events
			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

func TestWebhookHandler_TrialWillEnd(t *testing.T) {
	// Trial will end just logs, doesn't need SubscriptionService
	logger := zap.NewNop()
	handler := NewWebhookHandler("", nil, logger)

	event := WebhookEvent{
		ID:   "evt_123",
		Type: "customer.subscription.trial_will_end",
		Data: WebhookEventData{
			Object: json.RawMessage(`{"id":"sub_123","customer":"cus_123","trial_end":1700000000}`),
		},
	}
	body, _ := json.Marshal(event)

	req := httptest.NewRequest("POST", "/webhook", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	handler.HandleWebhook(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestWebhookHandler_ServeHTTP(t *testing.T) {
	logger := zap.NewNop()
	handler := NewWebhookHandler("", nil, logger)

	t.Run("POST method calls HandleWebhook", func(t *testing.T) {
		event := WebhookEvent{
			ID:   "evt_123",
			Type: "customer.updated",
			Data: WebhookEventData{Object: json.RawMessage(`{}`)},
		}
		body, _ := json.Marshal(event)

		req := httptest.NewRequest("POST", "/webhook", bytes.NewBuffer(body))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("GET returns 405", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/webhook", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("PUT returns 405", func(t *testing.T) {
		req := httptest.NewRequest("PUT", "/webhook", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("DELETE returns 405", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/webhook", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})
}

func TestWebhookHandler_SignatureVerificationWithRealData(t *testing.T) {
	secret := "whsec_test_secret_12345"
	logger := zap.NewNop()
	handler := NewWebhookHandler(secret, nil, logger)

	// Simulate a real Stripe webhook payload
	payload := []byte(`{
		"id": "evt_1234567890",
		"type": "customer.subscription.updated",
		"created": 1700000000,
		"data": {
			"object": {
				"id": "sub_123",
				"customer": "cus_456",
				"status": "active"
			}
		}
	}`)

	timestamp := fmt.Sprintf("%d", time.Now().Unix())

	// Compute valid signature
	signedPayload := timestamp + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedPayload))
	signature := hex.EncodeToString(mac.Sum(nil))

	header := fmt.Sprintf("t=%s,v1=%s", timestamp, signature)

	err := handler.verifySignature(payload, header)
	require.NoError(t, err)
}

func TestWebhookEventData_JSON(t *testing.T) {
	data := WebhookEventData{
		Object: json.RawMessage(`{"id":"sub_123","status":"active","customer":"cus_456"}`),
	}

	// Verify we can parse the object
	var sub struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Customer string `json:"customer"`
	}
	err := json.Unmarshal(data.Object, &sub)
	require.NoError(t, err)

	assert.Equal(t, "sub_123", sub.ID)
	assert.Equal(t, "active", sub.Status)
	assert.Equal(t, "cus_456", sub.Customer)
}

func TestWebhookHandler_EmptyBody(t *testing.T) {
	logger := zap.NewNop()
	handler := NewWebhookHandler("", nil, logger)

	req := httptest.NewRequest("POST", "/webhook", bytes.NewBuffer([]byte{}))
	rec := httptest.NewRecorder()

	handler.HandleWebhook(rec, req)
	// Empty body should fail JSON parsing
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestWebhookSignatureHeaderParsing(t *testing.T) {
	logger := zap.NewNop()
	secret := "whsec_test"
	handler := NewWebhookHandler(secret, nil, logger)

	t.Run("handles multiple v1 signatures", func(t *testing.T) {
		payload := []byte(`{"id":"evt_123"}`)
		timestamp := fmt.Sprintf("%d", time.Now().Unix())

		// Compute signature
		signedPayload := timestamp + "." + string(payload)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(signedPayload))
		signature := hex.EncodeToString(mac.Sum(nil))

		// Header with multiple signatures (Stripe sometimes sends this)
		header := fmt.Sprintf("t=%s,v1=%s,v0=old_signature", timestamp, signature)

		err := handler.verifySignature(payload, header)
		assert.NoError(t, err)
	})

	t.Run("handles malformed header parts", func(t *testing.T) {
		payload := []byte(`{"id":"evt_123"}`)
		// Missing equals sign in one part
		header := "t=123,v1signature,v0=test"

		err := handler.verifySignature(payload, header)
		// Should fail due to missing v1 signature
		assert.Error(t, err)
	})
}
