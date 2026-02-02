package admin

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// MetricsCollector collects real-time platform metrics
type MetricsCollector struct {
	redis  *redis.Client
	logger *zap.Logger

	// In-memory counters with periodic flush
	counters   map[string]int64
	countersMu sync.Mutex

	// Gauge values
	gauges   map[string]float64
	gaugesMu sync.RWMutex

	// Flush interval
	flushInterval time.Duration
	done          chan struct{}
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(redis *redis.Client, logger *zap.Logger) *MetricsCollector {
	mc := &MetricsCollector{
		redis:         redis,
		logger:        logger,
		counters:      make(map[string]int64),
		gauges:        make(map[string]float64),
		flushInterval: 10 * time.Second,
		done:          make(chan struct{}),
	}

	// Start background flush
	go mc.flushLoop()

	return mc
}

// IncrementCounter increments a counter metric
func (mc *MetricsCollector) IncrementCounter(name string, value int64) {
	mc.countersMu.Lock()
	mc.counters[name] += value
	mc.countersMu.Unlock()
}

// SetGauge sets a gauge metric value
func (mc *MetricsCollector) SetGauge(name string, value float64) {
	mc.gaugesMu.Lock()
	mc.gauges[name] = value
	mc.gaugesMu.Unlock()
}

// GetGauge gets a gauge metric value
func (mc *MetricsCollector) GetGauge(name string) float64 {
	mc.gaugesMu.RLock()
	defer mc.gaugesMu.RUnlock()
	return mc.gauges[name]
}

// RecordLatency records a latency measurement
func (mc *MetricsCollector) RecordLatency(name string, duration time.Duration) {
	// Store in Redis sorted set for percentile calculations
	if mc.redis != nil {
		ctx := context.Background()
		key := "metrics:latency:" + name + ":" + time.Now().Format("2006010215")
		mc.redis.ZAdd(ctx, key, redis.Z{
			Score:  float64(duration.Milliseconds()),
			Member: time.Now().UnixNano(),
		})
		mc.redis.Expire(ctx, key, 25*time.Hour)
	}
}

// GetLatencyPercentiles returns latency percentiles for a metric
func (mc *MetricsCollector) GetLatencyPercentiles(ctx context.Context, name string, hour time.Time) (*LatencyStats, error) {
	if mc.redis == nil {
		return nil, nil
	}

	key := "metrics:latency:" + name + ":" + hour.Format("2006010215")

	// Get count
	count, err := mc.redis.ZCard(ctx, key).Result()
	if err != nil || count == 0 {
		return nil, err
	}

	stats := &LatencyStats{}

	// Get percentiles
	percentiles := map[string]float64{
		"p50": 0.5,
		"p90": 0.9,
		"p95": 0.95,
		"p99": 0.99,
	}

	for name, pct := range percentiles {
		idx := int64(float64(count) * pct)
		vals, err := mc.redis.ZRange(ctx, key, idx, idx).Result()
		if err == nil && len(vals) > 0 {
			switch name {
			case "p50":
				stats.P50 = parseLatency(vals[0])
			case "p90":
				stats.P90 = parseLatency(vals[0])
			case "p95":
				stats.P95 = parseLatency(vals[0])
			case "p99":
				stats.P99 = parseLatency(vals[0])
			}
		}
	}

	stats.Count = count

	return stats, nil
}

// LatencyStats contains latency statistics
type LatencyStats struct {
	Count int64   `json:"count"`
	P50   float64 `json:"p50_ms"`
	P90   float64 `json:"p90_ms"`
	P95   float64 `json:"p95_ms"`
	P99   float64 `json:"p99_ms"`
}

// RecordEvent records a timestamped event
func (mc *MetricsCollector) RecordEvent(ctx context.Context, eventType string, data map[string]interface{}) {
	if mc.redis == nil {
		return
	}

	event := map[string]interface{}{
		"type":      eventType,
		"data":      data,
		"timestamp": time.Now().Unix(),
	}

	eventJSON, _ := json.Marshal(event)

	// Add to event stream
	mc.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: "metrics:events",
		MaxLen: 10000,
		Values: map[string]interface{}{
			"event": string(eventJSON),
		},
	})
}

// GetRecentEvents returns recent events
func (mc *MetricsCollector) GetRecentEvents(ctx context.Context, count int64) ([]map[string]interface{}, error) {
	if mc.redis == nil {
		return nil, nil
	}

	results, err := mc.redis.XRevRange(ctx, "metrics:events", "+", "-").Result()
	if err != nil {
		return nil, err
	}

	events := make([]map[string]interface{}, 0, len(results))
	for _, r := range results {
		if eventJSON, ok := r.Values["event"].(string); ok {
			var event map[string]interface{}
			if json.Unmarshal([]byte(eventJSON), &event) == nil {
				events = append(events, event)
			}
		}
		if int64(len(events)) >= count {
			break
		}
	}

	return events, nil
}

// GetRealTimeMetrics returns current real-time metrics
func (mc *MetricsCollector) GetRealTimeMetrics() *RealTimeMetrics {
	mc.gaugesMu.RLock()
	defer mc.gaugesMu.RUnlock()

	return &RealTimeMetrics{
		ActiveConnections:  int64(mc.gauges["active_connections"]),
		RequestsPerSecond:  mc.gauges["requests_per_second"],
		RunningTests:       int64(mc.gauges["running_tests"]),
		QueueDepth:         int64(mc.gauges["queue_depth"]),
		WorkerUtilization:  mc.gauges["worker_utilization"],
		MemoryUsageMB:      mc.gauges["memory_usage_mb"],
		CPUUsagePercent:    mc.gauges["cpu_usage_percent"],
		Timestamp:          time.Now(),
	}
}

// RealTimeMetrics contains real-time metrics
type RealTimeMetrics struct {
	ActiveConnections int64     `json:"active_connections"`
	RequestsPerSecond float64   `json:"requests_per_second"`
	RunningTests      int64     `json:"running_tests"`
	QueueDepth        int64     `json:"queue_depth"`
	WorkerUtilization float64   `json:"worker_utilization"`
	MemoryUsageMB     float64   `json:"memory_usage_mb"`
	CPUUsagePercent   float64   `json:"cpu_usage_percent"`
	Timestamp         time.Time `json:"timestamp"`
}

// Close stops the metrics collector
func (mc *MetricsCollector) Close() {
	close(mc.done)
}

// Private methods

func (mc *MetricsCollector) flushLoop() {
	ticker := time.NewTicker(mc.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mc.flush()
		case <-mc.done:
			mc.flush()
			return
		}
	}
}

func (mc *MetricsCollector) flush() {
	mc.countersMu.Lock()
	counters := mc.counters
	mc.counters = make(map[string]int64)
	mc.countersMu.Unlock()

	if mc.redis == nil || len(counters) == 0 {
		return
	}

	ctx := context.Background()
	now := time.Now()
	hourKey := now.Format("2006010215")

	for name, value := range counters {
		key := "metrics:counter:" + name + ":" + hourKey
		mc.redis.IncrBy(ctx, key, value)
		mc.redis.Expire(ctx, key, 25*time.Hour)
	}
}

func parseLatency(s string) float64 {
	var f float64
	json.Unmarshal([]byte(s), &f)
	return f
}

// Predefined metric names
const (
	MetricAPIRequests      = "api_requests"
	MetricAPIErrors        = "api_errors"
	MetricTestsStarted     = "tests_started"
	MetricTestsCompleted   = "tests_completed"
	MetricTestsFailed      = "tests_failed"
	MetricAITokensUsed     = "ai_tokens_used"
	MetricCacheHits        = "cache_hits"
	MetricCacheMisses      = "cache_misses"
	MetricSandboxStarted   = "sandbox_started"
	MetricSandboxCompleted = "sandbox_completed"

	GaugeActiveConnections = "active_connections"
	GaugeRunningTests      = "running_tests"
	GaugeQueueDepth        = "queue_depth"
	GaugeWorkerUtilization = "worker_utilization"
	GaugeMemoryUsage       = "memory_usage_mb"
	GaugeCPUUsage          = "cpu_usage_percent"
)
