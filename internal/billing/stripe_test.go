package billing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStripeClient(t *testing.T) {
	t.Run("sets default base URL", func(t *testing.T) {
		client := NewStripeClient(StripeConfig{
			SecretKey: "sk_test_123",
		})

		assert.Equal(t, "https://api.stripe.com", client.config.BaseURL)
	})

	t.Run("uses custom base URL", func(t *testing.T) {
		client := NewStripeClient(StripeConfig{
			SecretKey: "sk_test_123",
			BaseURL:   "https://custom.stripe.com",
		})

		assert.Equal(t, "https://custom.stripe.com", client.config.BaseURL)
	})

	t.Run("sets http client with timeout", func(t *testing.T) {
		client := NewStripeClient(StripeConfig{})
		assert.NotNil(t, client.httpClient)
		assert.Equal(t, 30*time.Second, client.httpClient.Timeout)
	})
}

func TestStripeClient_CreateCustomer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/customers", r.URL.Path)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		// Verify basic auth
		user, _, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "sk_test_123", user)

		// Parse form
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "test@example.com", r.Form.Get("email"))
		assert.Equal(t, "Test User", r.Form.Get("name"))
		assert.NotEmpty(t, r.Form.Get("metadata[tenant_id]"))

		// Return customer
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Customer{
			ID:    "cus_123",
			Email: "test@example.com",
			Name:  "Test User",
		})
	}))
	defer server.Close()

	client := NewStripeClient(StripeConfig{
		SecretKey: "sk_test_123",
		BaseURL:   server.URL,
	})

	customer, err := client.CreateCustomer(context.Background(), "test@example.com", "Test User", uuid.New())
	require.NoError(t, err)
	assert.Equal(t, "cus_123", customer.ID)
	assert.Equal(t, "test@example.com", customer.Email)
}

func TestStripeClient_GetCustomer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/v1/customers/cus_123", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Customer{
			ID:    "cus_123",
			Email: "test@example.com",
		})
	}))
	defer server.Close()

	client := NewStripeClient(StripeConfig{
		SecretKey: "sk_test_123",
		BaseURL:   server.URL,
	})

	customer, err := client.GetCustomer(context.Background(), "cus_123")
	require.NoError(t, err)
	assert.Equal(t, "cus_123", customer.ID)
}

func TestStripeClient_UpdateCustomer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/customers/cus_123", r.URL.Path)

		require.NoError(t, r.ParseForm())
		assert.Equal(t, "newemail@example.com", r.Form.Get("email"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Customer{
			ID:    "cus_123",
			Email: "newemail@example.com",
		})
	}))
	defer server.Close()

	client := NewStripeClient(StripeConfig{
		SecretKey: "sk_test_123",
		BaseURL:   server.URL,
	})

	customer, err := client.UpdateCustomer(context.Background(), "cus_123", url.Values{
		"email": {"newemail@example.com"},
	})
	require.NoError(t, err)
	assert.Equal(t, "newemail@example.com", customer.Email)
}

func TestStripeClient_CreateSubscription(t *testing.T) {
	t.Run("without options", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "/v1/subscriptions", r.URL.Path)

			require.NoError(t, r.ParseForm())
			assert.Equal(t, "cus_123", r.Form.Get("customer"))
			assert.Equal(t, "price_123", r.Form.Get("items[0][price]"))

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(StripeSubscription{
				ID:       "sub_123",
				Customer: "cus_123",
				Status:   "active",
			})
		}))
		defer server.Close()

		client := NewStripeClient(StripeConfig{BaseURL: server.URL})
		sub, err := client.CreateSubscription(context.Background(), "cus_123", "price_123", nil)
		require.NoError(t, err)
		assert.Equal(t, "sub_123", sub.ID)
		assert.Equal(t, "active", sub.Status)
	})

	t.Run("with options", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			assert.Equal(t, "14", r.Form.Get("trial_period_days"))
			assert.Equal(t, "pm_123", r.Form.Get("default_payment_method"))
			assert.Equal(t, "tenant-456", r.Form.Get("metadata[tenant_id]"))

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(StripeSubscription{
				ID:     "sub_123",
				Status: "trialing",
			})
		}))
		defer server.Close()

		client := NewStripeClient(StripeConfig{BaseURL: server.URL})
		sub, err := client.CreateSubscription(context.Background(), "cus_123", "price_123", &SubscriptionOptions{
			TrialDays:       14,
			PaymentMethodID: "pm_123",
			Metadata:        map[string]string{"tenant_id": "tenant-456"},
		})
		require.NoError(t, err)
		assert.Equal(t, "trialing", sub.Status)
	})
}

func TestStripeClient_GetSubscription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/v1/subscriptions/sub_123", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(StripeSubscription{
			ID:                 "sub_123",
			Status:             "active",
			CurrentPeriodStart: 1700000000,
			CurrentPeriodEnd:   1702592000,
		})
	}))
	defer server.Close()

	client := NewStripeClient(StripeConfig{BaseURL: server.URL})
	sub, err := client.GetSubscription(context.Background(), "sub_123")
	require.NoError(t, err)
	assert.Equal(t, "sub_123", sub.ID)
	assert.Equal(t, int64(1700000000), sub.CurrentPeriodStart)
}

func TestStripeClient_CancelSubscription(t *testing.T) {
	t.Run("cancel at period end", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "/v1/subscriptions/sub_123", r.URL.Path)

			require.NoError(t, r.ParseForm())
			assert.Equal(t, "true", r.Form.Get("cancel_at_period_end"))

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(StripeSubscription{
				ID:                "sub_123",
				Status:            "active",
				CancelAtPeriodEnd: true,
			})
		}))
		defer server.Close()

		client := NewStripeClient(StripeConfig{BaseURL: server.URL})
		sub, err := client.CancelSubscription(context.Background(), "sub_123", true)
		require.NoError(t, err)
		assert.True(t, sub.CancelAtPeriodEnd)
	})

	t.Run("cancel immediately", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "DELETE", r.Method)
			assert.Equal(t, "/v1/subscriptions/sub_123", r.URL.Path)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(StripeSubscription{
				ID:     "sub_123",
				Status: "canceled",
			})
		}))
		defer server.Close()

		client := NewStripeClient(StripeConfig{BaseURL: server.URL})
		sub, err := client.CancelSubscription(context.Background(), "sub_123", false)
		require.NoError(t, err)
		assert.Equal(t, "canceled", sub.Status)
	})
}

func TestStripeClient_CreateUsageRecord(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/subscription_items/si_123/usage_records", r.URL.Path)

		require.NoError(t, r.ParseForm())
		assert.Equal(t, "100", r.Form.Get("quantity"))
		assert.Equal(t, "increment", r.Form.Get("action"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":       "mbur_123",
			"quantity": 100,
		})
	}))
	defer server.Close()

	client := NewStripeClient(StripeConfig{BaseURL: server.URL})
	err := client.CreateUsageRecord(context.Background(), "si_123", 100, time.Now().Unix(), "increment")
	require.NoError(t, err)
}

func TestStripeClient_GetUpcomingInvoice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Contains(t, r.URL.Path, "/v1/invoices/upcoming")
		assert.Equal(t, "cus_123", r.URL.Query().Get("customer"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Invoice{
			ID:        "in_upcoming",
			Customer:  "cus_123",
			AmountDue: 9900,
			Currency:  "usd",
		})
	}))
	defer server.Close()

	client := NewStripeClient(StripeConfig{BaseURL: server.URL})
	invoice, err := client.GetUpcomingInvoice(context.Background(), "cus_123")
	require.NoError(t, err)
	assert.Equal(t, int64(9900), invoice.AmountDue)
}

func TestStripeClient_ListInvoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "cus_123", r.URL.Query().Get("customer"))
		assert.Equal(t, "10", r.URL.Query().Get("limit"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []Invoice{
				{ID: "in_1", AmountPaid: 9900},
				{ID: "in_2", AmountPaid: 9900},
			},
		})
	}))
	defer server.Close()

	client := NewStripeClient(StripeConfig{BaseURL: server.URL})
	invoices, err := client.ListInvoices(context.Background(), "cus_123", 10)
	require.NoError(t, err)
	assert.Len(t, invoices, 2)
}

func TestStripeClient_CreateBillingPortalSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/billing_portal/sessions", r.URL.Path)

		require.NoError(t, r.ParseForm())
		assert.Equal(t, "cus_123", r.Form.Get("customer"))
		assert.Equal(t, "https://example.com/billing", r.Form.Get("return_url"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"url": "https://billing.stripe.com/session/xyz",
		})
	}))
	defer server.Close()

	client := NewStripeClient(StripeConfig{BaseURL: server.URL})
	url, err := client.CreateBillingPortalSession(context.Background(), "cus_123", "https://example.com/billing")
	require.NoError(t, err)
	assert.Equal(t, "https://billing.stripe.com/session/xyz", url)
}

func TestStripeClient_CreateCheckoutSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/checkout/sessions", r.URL.Path)

		require.NoError(t, r.ParseForm())
		assert.Equal(t, "subscription", r.Form.Get("mode"))
		assert.Equal(t, "price_123", r.Form.Get("line_items[0][price]"))
		assert.Equal(t, "https://example.com/success", r.Form.Get("success_url"))
		assert.Equal(t, "https://example.com/cancel", r.Form.Get("cancel_url"))
		assert.Equal(t, "cus_123", r.Form.Get("customer"))
		assert.Equal(t, "tenant-456", r.Form.Get("metadata[tenant_id]"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"url": "https://checkout.stripe.com/pay/xyz",
		})
	}))
	defer server.Close()

	client := NewStripeClient(StripeConfig{BaseURL: server.URL})
	url, err := client.CreateCheckoutSession(context.Background(), CheckoutOptions{
		Mode:       "subscription",
		PriceID:    "price_123",
		SuccessURL: "https://example.com/success",
		CancelURL:  "https://example.com/cancel",
		CustomerID: "cus_123",
		Metadata:   map[string]string{"tenant_id": "tenant-456"},
	})
	require.NoError(t, err)
	assert.Equal(t, "https://checkout.stripe.com/pay/xyz", url)
}

func TestStripeClient_ErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"message": "Invalid customer",
				"type":    "invalid_request_error",
				"code":    "resource_missing",
			},
		})
	}))
	defer server.Close()

	client := NewStripeClient(StripeConfig{BaseURL: server.URL})
	_, err := client.GetCustomer(context.Background(), "invalid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid customer")
	assert.Contains(t, err.Error(), "invalid_request_error")
}

func TestStripeTypes(t *testing.T) {
	t.Run("Customer struct", func(t *testing.T) {
		customer := Customer{
			ID:          "cus_123",
			Email:       "test@example.com",
			Name:        "Test User",
			Description: "Test customer",
			Metadata:    map[string]string{"tenant_id": "123"},
			Created:     1700000000,
			Currency:    "usd",
			Delinquent:  false,
		}
		assert.Equal(t, "cus_123", customer.ID)
		assert.Equal(t, "test@example.com", customer.Email)
	})

	t.Run("StripeSubscription struct", func(t *testing.T) {
		cancelAt := int64(1702592000)
		sub := StripeSubscription{
			ID:                   "sub_123",
			Customer:             "cus_123",
			Status:               "active",
			CurrentPeriodStart:   1700000000,
			CurrentPeriodEnd:     1702592000,
			CancelAtPeriodEnd:    false,
			CancelAt:             &cancelAt,
			DefaultPaymentMethod: "pm_123",
		}
		assert.Equal(t, "sub_123", sub.ID)
		assert.Equal(t, "active", sub.Status)
		assert.NotNil(t, sub.CancelAt)
	})

	t.Run("Invoice struct", func(t *testing.T) {
		invoice := Invoice{
			ID:               "in_123",
			Customer:         "cus_123",
			Status:           "paid",
			Currency:         "usd",
			AmountDue:        9900,
			AmountPaid:       9900,
			Total:            9900,
			HostedInvoiceURL: "https://invoice.stripe.com/i/123",
			Paid:             true,
		}
		assert.Equal(t, "in_123", invoice.ID)
		assert.True(t, invoice.Paid)
	})

	t.Run("PaymentMethod struct", func(t *testing.T) {
		pm := PaymentMethod{
			ID:       "pm_123",
			Type:     "card",
			Customer: "cus_123",
			Card: &PaymentMethodCard{
				Brand:    "visa",
				Last4:    "4242",
				ExpMonth: 12,
				ExpYear:  2025,
			},
		}
		assert.Equal(t, "pm_123", pm.ID)
		assert.Equal(t, "visa", pm.Card.Brand)
		assert.Equal(t, "4242", pm.Card.Last4)
	})

	t.Run("CheckoutOptions struct", func(t *testing.T) {
		opts := CheckoutOptions{
			Mode:          "subscription",
			PriceID:       "price_123",
			SuccessURL:    "https://example.com/success",
			CancelURL:     "https://example.com/cancel",
			CustomerID:    "cus_123",
			CustomerEmail: "test@example.com",
			Metadata:      map[string]string{"key": "value"},
		}
		assert.Equal(t, "subscription", opts.Mode)
		assert.Equal(t, "price_123", opts.PriceID)
	})

	t.Run("SubscriptionOptions struct", func(t *testing.T) {
		opts := SubscriptionOptions{
			TrialDays:       14,
			PaymentMethodID: "pm_123",
			Metadata:        map[string]string{"plan": "pro"},
		}
		assert.Equal(t, 14, opts.TrialDays)
		assert.Equal(t, "pm_123", opts.PaymentMethodID)
	})
}

func TestStripeErrors(t *testing.T) {
	assert.Equal(t, "stripe customer not found", ErrCustomerNotFound.Error())
	assert.Equal(t, "subscription not found", ErrSubscriptionNotFound.Error())
	assert.Equal(t, "payment failed", ErrPaymentFailed.Error())
	assert.Equal(t, "invalid webhook signature", ErrInvalidWebhook.Error())
}
