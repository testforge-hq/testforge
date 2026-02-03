// Package billing provides Stripe integration for subscription management
package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrCustomerNotFound is returned when a Stripe customer doesn't exist
	ErrCustomerNotFound = errors.New("stripe customer not found")

	// ErrSubscriptionNotFound is returned when a subscription doesn't exist
	ErrSubscriptionNotFound = errors.New("subscription not found")

	// ErrPaymentFailed is returned when a payment fails
	ErrPaymentFailed = errors.New("payment failed")

	// ErrInvalidWebhook is returned when webhook verification fails
	ErrInvalidWebhook = errors.New("invalid webhook signature")
)

// StripeConfig holds Stripe configuration
type StripeConfig struct {
	SecretKey      string
	WebhookSecret  string
	BaseURL        string // For testing, defaults to https://api.stripe.com
	PublishableKey string // For frontend use
}

// StripeClient provides access to Stripe API
type StripeClient struct {
	config     StripeConfig
	httpClient *http.Client
}

// NewStripeClient creates a new Stripe client
func NewStripeClient(config StripeConfig) *StripeClient {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.stripe.com"
	}

	return &StripeClient{
		config:     config,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Customer represents a Stripe customer
type Customer struct {
	ID           string            `json:"id"`
	Email        string            `json:"email"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Metadata     map[string]string `json:"metadata"`
	Created      int64             `json:"created"`
	Currency     string            `json:"currency"`
	Delinquent   bool              `json:"delinquent"`
	DefaultSource string           `json:"default_source"`
}

// StripeSubscription represents a Stripe subscription API response
type StripeSubscription struct {
	ID                   string            `json:"id"`
	Customer             string            `json:"customer"`
	Status               string            `json:"status"`
	CurrentPeriodStart   int64             `json:"current_period_start"`
	CurrentPeriodEnd     int64             `json:"current_period_end"`
	CancelAtPeriodEnd    bool              `json:"cancel_at_period_end"`
	CancelAt             *int64            `json:"cancel_at"`
	CanceledAt           *int64            `json:"canceled_at"`
	TrialStart           *int64            `json:"trial_start"`
	TrialEnd             *int64            `json:"trial_end"`
	Items                *SubscriptionItems `json:"items"`
	Metadata             map[string]string `json:"metadata"`
	DefaultPaymentMethod string            `json:"default_payment_method"`
	LatestInvoice        string            `json:"latest_invoice"`
}

// SubscriptionItems represents subscription items
type SubscriptionItems struct {
	Data []SubscriptionItem `json:"data"`
}

// SubscriptionItem represents a single subscription item
type SubscriptionItem struct {
	ID       string `json:"id"`
	Price    Price  `json:"price"`
	Quantity int64  `json:"quantity"`
}

// Price represents a Stripe price
type Price struct {
	ID                string            `json:"id"`
	Product           string            `json:"product"`
	Currency          string            `json:"currency"`
	UnitAmount        int64             `json:"unit_amount"`
	Recurring         *PriceRecurring   `json:"recurring"`
	Type              string            `json:"type"`
	Metadata          map[string]string `json:"metadata"`
}

// PriceRecurring represents recurring price details
type PriceRecurring struct {
	Interval      string `json:"interval"`
	IntervalCount int    `json:"interval_count"`
	UsageType     string `json:"usage_type"`
}

// Invoice represents a Stripe invoice
type Invoice struct {
	ID                 string `json:"id"`
	Customer           string `json:"customer"`
	Subscription       string `json:"subscription"`
	Status             string `json:"status"`
	Currency           string `json:"currency"`
	AmountDue          int64  `json:"amount_due"`
	AmountPaid         int64  `json:"amount_paid"`
	AmountRemaining    int64  `json:"amount_remaining"`
	Total              int64  `json:"total"`
	Subtotal           int64  `json:"subtotal"`
	Tax                int64  `json:"tax"`
	InvoicePDF         string `json:"invoice_pdf"`
	HostedInvoiceURL   string `json:"hosted_invoice_url"`
	Number             string `json:"number"`
	PeriodStart        int64  `json:"period_start"`
	PeriodEnd          int64  `json:"period_end"`
	Created            int64  `json:"created"`
	DueDate            *int64 `json:"due_date"`
	Paid               bool   `json:"paid"`
}

// PaymentMethod represents a Stripe payment method
type PaymentMethod struct {
	ID             string                 `json:"id"`
	Type           string                 `json:"type"`
	Customer       string                 `json:"customer"`
	Card           *PaymentMethodCard     `json:"card"`
	BillingDetails *PaymentBillingDetails `json:"billing_details"`
	Created        int64                  `json:"created"`
}

// PaymentMethodCard represents card details
type PaymentMethodCard struct {
	Brand    string `json:"brand"`
	Last4    string `json:"last4"`
	ExpMonth int    `json:"exp_month"`
	ExpYear  int    `json:"exp_year"`
}

// PaymentBillingDetails represents billing details
type PaymentBillingDetails struct {
	Email   string         `json:"email"`
	Name    string         `json:"name"`
	Address *BillingAddress `json:"address"`
}

// BillingAddress represents a billing address
type BillingAddress struct {
	Line1      string `json:"line1"`
	Line2      string `json:"line2"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
}

// CreateCustomer creates a new Stripe customer
func (c *StripeClient) CreateCustomer(ctx context.Context, email, name string, tenantID uuid.UUID) (*Customer, error) {
	data := url.Values{
		"email":               {email},
		"name":                {name},
		"metadata[tenant_id]": {tenantID.String()},
	}

	var customer Customer
	if err := c.post(ctx, "/v1/customers", data, &customer); err != nil {
		return nil, err
	}

	return &customer, nil
}

// GetCustomer retrieves a Stripe customer
func (c *StripeClient) GetCustomer(ctx context.Context, customerID string) (*Customer, error) {
	var customer Customer
	if err := c.get(ctx, "/v1/customers/"+customerID, &customer); err != nil {
		return nil, err
	}
	return &customer, nil
}

// UpdateCustomer updates a Stripe customer
func (c *StripeClient) UpdateCustomer(ctx context.Context, customerID string, data url.Values) (*Customer, error) {
	var customer Customer
	if err := c.post(ctx, "/v1/customers/"+customerID, data, &customer); err != nil {
		return nil, err
	}
	return &customer, nil
}

// CreateSubscription creates a new subscription
func (c *StripeClient) CreateSubscription(ctx context.Context, customerID, priceID string, opts *SubscriptionOptions) (*StripeSubscription, error) {
	data := url.Values{
		"customer":        {customerID},
		"items[0][price]": {priceID},
	}

	if opts != nil {
		if opts.TrialDays > 0 {
			data.Set("trial_period_days", fmt.Sprintf("%d", opts.TrialDays))
		}
		if opts.PaymentMethodID != "" {
			data.Set("default_payment_method", opts.PaymentMethodID)
		}
		for k, v := range opts.Metadata {
			data.Set("metadata["+k+"]", v)
		}
	}

	var subscription StripeSubscription
	if err := c.post(ctx, "/v1/subscriptions", data, &subscription); err != nil {
		return nil, err
	}

	return &subscription, nil
}

// SubscriptionOptions holds options for creating a subscription
type SubscriptionOptions struct {
	TrialDays       int
	PaymentMethodID string
	Metadata        map[string]string
}

// GetSubscription retrieves a subscription
func (c *StripeClient) GetSubscription(ctx context.Context, subscriptionID string) (*StripeSubscription, error) {
	var subscription StripeSubscription
	if err := c.get(ctx, "/v1/subscriptions/"+subscriptionID, &subscription); err != nil {
		return nil, err
	}
	return &subscription, nil
}

// UpdateSubscription updates a subscription
func (c *StripeClient) UpdateSubscription(ctx context.Context, subscriptionID string, data url.Values) (*StripeSubscription, error) {
	var subscription StripeSubscription
	if err := c.post(ctx, "/v1/subscriptions/"+subscriptionID, data, &subscription); err != nil {
		return nil, err
	}
	return &subscription, nil
}

// CancelSubscription cancels a subscription
func (c *StripeClient) CancelSubscription(ctx context.Context, subscriptionID string, cancelAtPeriodEnd bool) (*StripeSubscription, error) {
	if cancelAtPeriodEnd {
		return c.UpdateSubscription(ctx, subscriptionID, url.Values{
			"cancel_at_period_end": {"true"},
		})
	}

	var subscription StripeSubscription
	if err := c.delete(ctx, "/v1/subscriptions/"+subscriptionID, &subscription); err != nil {
		return nil, err
	}
	return &subscription, nil
}

// CreateUsageRecord reports usage for metered billing
func (c *StripeClient) CreateUsageRecord(ctx context.Context, subscriptionItemID string, quantity int64, timestamp int64, action string) error {
	data := url.Values{
		"quantity":  {fmt.Sprintf("%d", quantity)},
		"timestamp": {fmt.Sprintf("%d", timestamp)},
		"action":    {action}, // "increment" or "set"
	}

	var result map[string]interface{}
	return c.post(ctx, "/v1/subscription_items/"+subscriptionItemID+"/usage_records", data, &result)
}

// GetUpcomingInvoice retrieves the upcoming invoice for a customer
func (c *StripeClient) GetUpcomingInvoice(ctx context.Context, customerID string) (*Invoice, error) {
	var invoice Invoice
	if err := c.get(ctx, "/v1/invoices/upcoming?customer="+customerID, &invoice); err != nil {
		return nil, err
	}
	return &invoice, nil
}

// ListInvoices lists invoices for a customer
func (c *StripeClient) ListInvoices(ctx context.Context, customerID string, limit int) ([]Invoice, error) {
	var result struct {
		Data []Invoice `json:"data"`
	}
	path := fmt.Sprintf("/v1/invoices?customer=%s&limit=%d", customerID, limit)
	if err := c.get(ctx, path, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// CreateBillingPortalSession creates a Stripe billing portal session
func (c *StripeClient) CreateBillingPortalSession(ctx context.Context, customerID, returnURL string) (string, error) {
	data := url.Values{
		"customer":   {customerID},
		"return_url": {returnURL},
	}

	var result struct {
		URL string `json:"url"`
	}
	if err := c.post(ctx, "/v1/billing_portal/sessions", data, &result); err != nil {
		return "", err
	}
	return result.URL, nil
}

// CreateCheckoutSession creates a Stripe checkout session
func (c *StripeClient) CreateCheckoutSession(ctx context.Context, opts CheckoutOptions) (string, error) {
	data := url.Values{
		"mode":               {opts.Mode},
		"success_url":        {opts.SuccessURL},
		"cancel_url":         {opts.CancelURL},
		"line_items[0][price]": {opts.PriceID},
		"line_items[0][quantity]": {"1"},
	}

	if opts.CustomerID != "" {
		data.Set("customer", opts.CustomerID)
	}
	if opts.CustomerEmail != "" {
		data.Set("customer_email", opts.CustomerEmail)
	}
	for k, v := range opts.Metadata {
		data.Set("metadata["+k+"]", v)
	}

	var result struct {
		URL string `json:"url"`
	}
	if err := c.post(ctx, "/v1/checkout/sessions", data, &result); err != nil {
		return "", err
	}
	return result.URL, nil
}

// CheckoutOptions holds options for creating a checkout session
type CheckoutOptions struct {
	Mode          string // "subscription" or "payment"
	PriceID       string
	SuccessURL    string
	CancelURL     string
	CustomerID    string
	CustomerEmail string
	Metadata      map[string]string
}

// HTTP helpers
func (c *StripeClient) get(ctx context.Context, path string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.config.BaseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, result)
}

func (c *StripeClient) post(ctx context.Context, path string, data url.Values, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+path, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req, result)
}

func (c *StripeClient) delete(ctx context.Context, path string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", c.config.BaseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, result)
}

func (c *StripeClient) do(req *http.Request, result interface{}) error {
	req.SetBasicAuth(c.config.SecretKey, "")
	req.Header.Set("Stripe-Version", "2023-10-16")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		var stripeErr struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		json.Unmarshal(body, &stripeErr)
		return fmt.Errorf("stripe error: %s (%s)", stripeErr.Error.Message, stripeErr.Error.Type)
	}

	return json.Unmarshal(body, result)
}
