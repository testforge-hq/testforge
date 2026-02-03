package billing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// UsageMetric constants
const (
	MetricTestRuns       = "test_runs"
	MetricAITokens       = "ai_tokens"
	MetricSandboxMinutes = "sandbox_minutes"
)

// UsageRecord represents a usage record
type UsageRecord struct {
	ID                 uuid.UUID         `db:"id" json:"id"`
	TenantID           uuid.UUID         `db:"tenant_id" json:"tenant_id"`
	Metric             string            `db:"metric" json:"metric"`
	Quantity           int64             `db:"quantity" json:"quantity"`
	PeriodStart        time.Time         `db:"period_start" json:"period_start"`
	PeriodEnd          time.Time         `db:"period_end" json:"period_end"`
	ReportedToStripe   bool              `db:"reported_to_stripe" json:"reported_to_stripe"`
	StripeUsageID      *string           `db:"stripe_usage_record_id" json:"stripe_usage_record_id,omitempty"`
	ReportedAt         *time.Time        `db:"reported_at" json:"reported_at,omitempty"`
	Metadata           map[string]interface{} `db:"metadata" json:"metadata,omitempty"`
	CreatedAt          time.Time         `db:"created_at" json:"created_at"`
	UpdatedAt          time.Time         `db:"updated_at" json:"updated_at"`
}

// UsageTracker tracks usage for metered billing
type UsageTracker struct {
	db     *sqlx.DB
	stripe *StripeClient
	logger *zap.Logger

	// Buffer for batching usage updates
	buffer    map[string]*usageBuffer
	bufferMu  sync.Mutex
	flushChan chan struct{}
	done      chan struct{}
	wg        sync.WaitGroup
}

type usageBuffer struct {
	tenantID uuid.UUID
	metric   string
	quantity int64
}

// NewUsageTracker creates a new usage tracker
func NewUsageTracker(db *sqlx.DB, stripe *StripeClient, logger *zap.Logger) *UsageTracker {
	t := &UsageTracker{
		db:        db,
		stripe:    stripe,
		logger:    logger,
		buffer:    make(map[string]*usageBuffer),
		flushChan: make(chan struct{}, 1),
		done:      make(chan struct{}),
	}

	// Start background flusher
	t.wg.Add(1)
	go t.backgroundFlusher()

	return t
}

// Track records usage for a metric
func (t *UsageTracker) Track(ctx context.Context, tenantID uuid.UUID, metric string, quantity int64) error {
	t.bufferMu.Lock()
	key := tenantID.String() + ":" + metric

	if buf, ok := t.buffer[key]; ok {
		buf.quantity += quantity
	} else {
		t.buffer[key] = &usageBuffer{
			tenantID: tenantID,
			metric:   metric,
			quantity: quantity,
		}
	}
	t.bufferMu.Unlock()

	// Signal flush if buffer is getting large
	if len(t.buffer) > 100 {
		select {
		case t.flushChan <- struct{}{}:
		default:
		}
	}

	return nil
}

// TrackTestRun tracks a test run
func (t *UsageTracker) TrackTestRun(ctx context.Context, tenantID uuid.UUID) error {
	return t.Track(ctx, tenantID, MetricTestRuns, 1)
}

// TrackAITokens tracks AI token usage
func (t *UsageTracker) TrackAITokens(ctx context.Context, tenantID uuid.UUID, tokens int64) error {
	return t.Track(ctx, tenantID, MetricAITokens, tokens)
}

// TrackSandboxMinutes tracks sandbox execution time
func (t *UsageTracker) TrackSandboxMinutes(ctx context.Context, tenantID uuid.UUID, minutes int64) error {
	return t.Track(ctx, tenantID, MetricSandboxMinutes, minutes)
}

// GetCurrentUsage retrieves current period usage for a tenant
func (t *UsageTracker) GetCurrentUsage(ctx context.Context, tenantID uuid.UUID) (map[string]int64, error) {
	// First flush any buffered data
	t.flush()

	query := `
		SELECT metric, COALESCE(SUM(quantity), 0) as total
		FROM usage_records
		WHERE tenant_id = $1
		  AND period_start <= NOW()
		  AND period_end > NOW()
		GROUP BY metric`

	rows, err := t.db.QueryxContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	usage := map[string]int64{
		MetricTestRuns:       0,
		MetricAITokens:       0,
		MetricSandboxMinutes: 0,
	}

	for rows.Next() {
		var metric string
		var total int64
		if err := rows.Scan(&metric, &total); err != nil {
			return nil, err
		}
		usage[metric] = total
	}

	return usage, nil
}

// backgroundFlusher periodically flushes the buffer
func (t *UsageTracker) backgroundFlusher() {
	defer t.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.flush()
		case <-t.flushChan:
			t.flush()
		case <-t.done:
			t.flush()
			return
		}
	}
}

// flush writes buffered usage to the database
func (t *UsageTracker) flush() {
	t.bufferMu.Lock()
	if len(t.buffer) == 0 {
		t.bufferMu.Unlock()
		return
	}

	// Swap buffer
	oldBuffer := t.buffer
	t.buffer = make(map[string]*usageBuffer)
	t.bufferMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, buf := range oldBuffer {
		if err := t.recordUsage(ctx, buf.tenantID, buf.metric, buf.quantity); err != nil {
			t.logger.Error("failed to record usage",
				zap.String("tenant_id", buf.tenantID.String()),
				zap.String("metric", buf.metric),
				zap.Error(err),
			)
		}
	}
}

// recordUsage records usage in the database
func (t *UsageTracker) recordUsage(ctx context.Context, tenantID uuid.UUID, metric string, quantity int64) error {
	// Get current billing period
	var periodStart, periodEnd time.Time

	err := t.db.QueryRowContext(ctx, `
		SELECT current_period_start, current_period_end
		FROM subscriptions
		WHERE tenant_id = $1`, tenantID).Scan(&periodStart, &periodEnd)

	if err != nil {
		// No subscription, use monthly period
		now := time.Now()
		periodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		periodEnd = periodStart.AddDate(0, 1, 0)
	}

	// Insert usage record
	_, err = t.db.ExecContext(ctx, `
		INSERT INTO usage_records (tenant_id, metric, quantity, period_start, period_end)
		VALUES ($1, $2, $3, $4, $5)`,
		tenantID, metric, quantity, periodStart, periodEnd)

	return err
}

// ReportToStripe reports unreported usage to Stripe
func (t *UsageTracker) ReportToStripe(ctx context.Context) error {
	// Get unreported usage grouped by tenant and metric
	query := `
		SELECT ur.tenant_id, ur.metric, SUM(ur.quantity) as total,
		       s.stripe_subscription_id
		FROM usage_records ur
		JOIN subscriptions s ON s.tenant_id = ur.tenant_id
		WHERE ur.reported_to_stripe = false
		  AND s.stripe_subscription_id IS NOT NULL
		GROUP BY ur.tenant_id, ur.metric, s.stripe_subscription_id`

	rows, err := t.db.QueryxContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var tenantID uuid.UUID
		var metric string
		var total int64
		var stripeSubID string

		if err := rows.Scan(&tenantID, &metric, &total, &stripeSubID); err != nil {
			t.logger.Error("failed to scan usage row", zap.Error(err))
			continue
		}

		// Get subscription item ID for the metric
		subItemID, err := t.getSubscriptionItemID(ctx, stripeSubID, metric)
		if err != nil {
			t.logger.Error("failed to get subscription item",
				zap.String("subscription_id", stripeSubID),
				zap.String("metric", metric),
				zap.Error(err),
			)
			continue
		}

		// Report to Stripe
		if err := t.stripe.CreateUsageRecord(ctx, subItemID, total, time.Now().Unix(), "increment"); err != nil {
			t.logger.Error("failed to report usage to Stripe",
				zap.String("tenant_id", tenantID.String()),
				zap.String("metric", metric),
				zap.Error(err),
			)
			continue
		}

		// Mark as reported
		_, err = t.db.ExecContext(ctx, `
			UPDATE usage_records
			SET reported_to_stripe = true, reported_at = NOW()
			WHERE tenant_id = $1 AND metric = $2 AND reported_to_stripe = false`,
			tenantID, metric)

		if err != nil {
			t.logger.Error("failed to mark usage as reported", zap.Error(err))
		}
	}

	return nil
}

// getSubscriptionItemID retrieves the subscription item ID for a metric
func (t *UsageTracker) getSubscriptionItemID(ctx context.Context, subscriptionID, metric string) (string, error) {
	sub, err := t.stripe.GetSubscription(ctx, subscriptionID)
	if err != nil {
		return "", err
	}

	// Map metric to price lookup key
	priceLookup := map[string]string{
		MetricTestRuns:       "test_runs",
		MetricAITokens:       "ai_tokens",
		MetricSandboxMinutes: "sandbox_minutes",
	}

	lookup, ok := priceLookup[metric]
	if !ok {
		return "", fmt.Errorf("unknown metric: %s", metric)
	}

	for _, item := range sub.Items.Data {
		if lookupKey, ok := item.Price.Metadata["lookup_key"]; ok && lookupKey == lookup {
			return item.ID, nil
		}
	}

	return "", fmt.Errorf("no subscription item found for metric: %s", metric)
}

// Close shuts down the usage tracker
func (t *UsageTracker) Close() error {
	close(t.done)
	t.wg.Wait()
	return nil
}
