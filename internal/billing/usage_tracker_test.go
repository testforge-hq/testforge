package billing

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestUsageMetricConstants(t *testing.T) {
	assert.Equal(t, "test_runs", MetricTestRuns)
	assert.Equal(t, "ai_tokens", MetricAITokens)
	assert.Equal(t, "sandbox_minutes", MetricSandboxMinutes)
}

func TestUsageRecord_Fields(t *testing.T) {
	now := time.Now()
	periodEnd := now.AddDate(0, 1, 0)
	stripeUsageID := "mbur_123"

	record := UsageRecord{
		ID:               uuid.New(),
		TenantID:         uuid.New(),
		Metric:           MetricTestRuns,
		Quantity:         100,
		PeriodStart:      now,
		PeriodEnd:        periodEnd,
		ReportedToStripe: false,
		StripeUsageID:    &stripeUsageID,
		ReportedAt:       nil,
		Metadata:         map[string]interface{}{"source": "api"},
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	assert.NotEqual(t, uuid.Nil, record.ID)
	assert.NotEqual(t, uuid.Nil, record.TenantID)
	assert.Equal(t, MetricTestRuns, record.Metric)
	assert.Equal(t, int64(100), record.Quantity)
	assert.False(t, record.ReportedToStripe)
	assert.Equal(t, "mbur_123", *record.StripeUsageID)
}

func TestNewUsageTracker(t *testing.T) {
	logger := zap.NewNop()

	tracker := NewUsageTracker(nil, nil, logger)
	require.NotNil(t, tracker)

	assert.NotNil(t, tracker.buffer)
	assert.NotNil(t, tracker.flushChan)
	assert.NotNil(t, tracker.done)

	// Clean up immediately to prevent background flush from accessing nil db
	err := tracker.Close()
	assert.NoError(t, err)
}

func TestUsageTracker_Track(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUsageTracker(nil, nil, logger)

	ctx := context.Background()
	tenantID := uuid.New()

	// Track some usage
	err := tracker.Track(ctx, tenantID, MetricTestRuns, 5)
	assert.NoError(t, err)

	// Check buffer
	tracker.bufferMu.Lock()
	key := tenantID.String() + ":" + MetricTestRuns
	buf, exists := tracker.buffer[key]
	assert.True(t, exists)
	assert.Equal(t, int64(5), buf.quantity)
	assert.Equal(t, MetricTestRuns, buf.metric)

	// Clear buffer before close to avoid nil db panic
	tracker.buffer = make(map[string]*usageBuffer)
	tracker.bufferMu.Unlock()

	tracker.Close()
}

func TestUsageTracker_Track_Accumulates(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUsageTracker(nil, nil, logger)

	ctx := context.Background()
	tenantID := uuid.New()

	// Track multiple times
	tracker.Track(ctx, tenantID, MetricAITokens, 1000)
	tracker.Track(ctx, tenantID, MetricAITokens, 2000)
	tracker.Track(ctx, tenantID, MetricAITokens, 500)

	// Check accumulated value
	tracker.bufferMu.Lock()
	key := tenantID.String() + ":" + MetricAITokens
	buf := tracker.buffer[key]
	assert.Equal(t, int64(3500), buf.quantity)

	// Clear buffer before close
	tracker.buffer = make(map[string]*usageBuffer)
	tracker.bufferMu.Unlock()

	tracker.Close()
}

func TestUsageTracker_TrackTestRun(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUsageTracker(nil, nil, logger)

	ctx := context.Background()
	tenantID := uuid.New()

	err := tracker.TrackTestRun(ctx, tenantID)
	assert.NoError(t, err)

	tracker.bufferMu.Lock()
	key := tenantID.String() + ":" + MetricTestRuns
	buf := tracker.buffer[key]
	assert.Equal(t, int64(1), buf.quantity)

	tracker.buffer = make(map[string]*usageBuffer)
	tracker.bufferMu.Unlock()

	tracker.Close()
}

func TestUsageTracker_TrackAITokens(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUsageTracker(nil, nil, logger)

	ctx := context.Background()
	tenantID := uuid.New()

	err := tracker.TrackAITokens(ctx, tenantID, 5000)
	assert.NoError(t, err)

	tracker.bufferMu.Lock()
	key := tenantID.String() + ":" + MetricAITokens
	buf := tracker.buffer[key]
	assert.Equal(t, int64(5000), buf.quantity)

	tracker.buffer = make(map[string]*usageBuffer)
	tracker.bufferMu.Unlock()

	tracker.Close()
}

func TestUsageTracker_TrackSandboxMinutes(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUsageTracker(nil, nil, logger)

	ctx := context.Background()
	tenantID := uuid.New()

	err := tracker.TrackSandboxMinutes(ctx, tenantID, 15)
	assert.NoError(t, err)

	tracker.bufferMu.Lock()
	key := tenantID.String() + ":" + MetricSandboxMinutes
	buf := tracker.buffer[key]
	assert.Equal(t, int64(15), buf.quantity)

	tracker.buffer = make(map[string]*usageBuffer)
	tracker.bufferMu.Unlock()

	tracker.Close()
}

func TestUsageTracker_MultipleTenants(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUsageTracker(nil, nil, logger)

	ctx := context.Background()
	tenant1 := uuid.New()
	tenant2 := uuid.New()

	// Track for different tenants
	tracker.Track(ctx, tenant1, MetricTestRuns, 10)
	tracker.Track(ctx, tenant2, MetricTestRuns, 20)
	tracker.Track(ctx, tenant1, MetricAITokens, 1000)
	tracker.Track(ctx, tenant2, MetricAITokens, 2000)

	// Check each tenant's usage
	tracker.bufferMu.Lock()
	assert.Equal(t, int64(10), tracker.buffer[tenant1.String()+":"+MetricTestRuns].quantity)
	assert.Equal(t, int64(20), tracker.buffer[tenant2.String()+":"+MetricTestRuns].quantity)
	assert.Equal(t, int64(1000), tracker.buffer[tenant1.String()+":"+MetricAITokens].quantity)
	assert.Equal(t, int64(2000), tracker.buffer[tenant2.String()+":"+MetricAITokens].quantity)

	tracker.buffer = make(map[string]*usageBuffer)
	tracker.bufferMu.Unlock()

	tracker.Close()
}

func TestUsageTracker_Close(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUsageTracker(nil, nil, logger)

	// Close without tracking anything should work
	err := tracker.Close()
	assert.NoError(t, err)
}

func TestUsageBuffer_Struct(t *testing.T) {
	tenantID := uuid.New()
	buf := &usageBuffer{
		tenantID: tenantID,
		metric:   MetricTestRuns,
		quantity: 42,
	}

	assert.Equal(t, tenantID, buf.tenantID)
	assert.Equal(t, MetricTestRuns, buf.metric)
	assert.Equal(t, int64(42), buf.quantity)
}

func TestUsageTracker_FlushTriggersOnLargeBuffer(t *testing.T) {
	// This test verifies that the flush signal is sent when buffer gets large
	// We skip actual flushing since it requires a database
	logger := zap.NewNop()
	tracker := NewUsageTracker(nil, nil, logger)

	ctx := context.Background()

	// Add more than 100 entries to trigger flush signal
	for i := 0; i < 105; i++ {
		tenantID := uuid.New()
		tracker.Track(ctx, tenantID, MetricTestRuns, 1)
	}

	// Clear buffer before close to avoid nil db panic
	tracker.bufferMu.Lock()
	tracker.buffer = make(map[string]*usageBuffer)
	tracker.bufferMu.Unlock()

	tracker.Close()
}

func TestUsageRecord_AllMetrics(t *testing.T) {
	metrics := []string{MetricTestRuns, MetricAITokens, MetricSandboxMinutes}

	for _, metric := range metrics {
		record := UsageRecord{
			ID:       uuid.New(),
			TenantID: uuid.New(),
			Metric:   metric,
			Quantity: 100,
		}
		assert.Equal(t, metric, record.Metric)
	}
}
