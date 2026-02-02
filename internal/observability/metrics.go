package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metrics
type Metrics struct {
	// HTTP metrics
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPRequestsActive  prometheus.Gauge

	// Business metrics
	TestRunsTotal       *prometheus.CounterVec
	TestsExecutedTotal  *prometheus.CounterVec
	SelfHealingAttempts *prometheus.CounterVec
	DiscoveryPages      *prometheus.HistogramVec

	// Claude API metrics
	ClaudeRequestsTotal   *prometheus.CounterVec
	ClaudeRequestDuration *prometheus.HistogramVec
	ClaudeTokensUsed      *prometheus.CounterVec
	ClaudeCostTotal       prometheus.Counter
	ClaudeCacheHits       prometheus.Counter
	ClaudeCacheMisses     prometheus.Counter

	// Visual AI metrics
	VisualAIRequestsTotal   *prometheus.CounterVec
	VisualAIRequestDuration *prometheus.HistogramVec

	// Temporal workflow metrics
	WorkflowsStarted   *prometheus.CounterVec
	WorkflowsCompleted *prometheus.CounterVec
	WorkflowDuration   *prometheus.HistogramVec
	ActivitiesExecuted *prometheus.CounterVec

	// System metrics
	DBConnectionsActive prometheus.Gauge
	DBConnectionsIdle   prometheus.Gauge
	CacheSize           prometheus.Gauge
	GoroutinesActive    prometheus.Gauge
}

// NewMetrics creates a new metrics instance with all Prometheus metrics registered
func NewMetrics(namespace string) *Metrics {
	if namespace == "" {
		namespace = "testforge"
	}

	m := &Metrics{
		// HTTP metrics
		HTTPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"method", "path"},
		),
		HTTPRequestsActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "http_requests_active",
				Help:      "Number of active HTTP requests",
			},
		),

		// Business metrics
		TestRunsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "test_runs_total",
				Help:      "Total number of test runs",
			},
			[]string{"tenant_id", "status"},
		),
		TestsExecutedTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "tests_executed_total",
				Help:      "Total number of tests executed",
			},
			[]string{"tenant_id", "status", "test_type"},
		),
		SelfHealingAttempts: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "self_healing_attempts_total",
				Help:      "Total number of self-healing attempts",
			},
			[]string{"strategy", "status"},
		),
		DiscoveryPages: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "discovery_pages_crawled",
				Help:      "Number of pages crawled per discovery run",
				Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500},
			},
			[]string{"tenant_id"},
		),

		// Claude API metrics
		ClaudeRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "claude_requests_total",
				Help:      "Total number of Claude API requests",
			},
			[]string{"model", "purpose", "status"},
		),
		ClaudeRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "claude_request_duration_seconds",
				Help:      "Claude API request duration in seconds",
				Buckets:   []float64{1, 2, 5, 10, 20, 30, 60, 120},
			},
			[]string{"model", "purpose"},
		),
		ClaudeTokensUsed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "claude_tokens_used_total",
				Help:      "Total number of tokens used",
			},
			[]string{"model", "type"}, // type: input, output
		),
		ClaudeCostTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "claude_cost_usd_total",
				Help:      "Total estimated cost in USD",
			},
		),
		ClaudeCacheHits: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "claude_cache_hits_total",
				Help:      "Total number of cache hits",
			},
		),
		ClaudeCacheMisses: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "claude_cache_misses_total",
				Help:      "Total number of cache misses",
			},
		),

		// Visual AI metrics
		VisualAIRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "visual_ai_requests_total",
				Help:      "Total number of Visual AI requests",
			},
			[]string{"model", "operation", "status"},
		),
		VisualAIRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "visual_ai_request_duration_seconds",
				Help:      "Visual AI request duration in seconds",
				Buckets:   []float64{.1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"model", "operation"},
		),

		// Temporal workflow metrics
		WorkflowsStarted: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "workflows_started_total",
				Help:      "Total number of workflows started",
			},
			[]string{"workflow_type"},
		),
		WorkflowsCompleted: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "workflows_completed_total",
				Help:      "Total number of workflows completed",
			},
			[]string{"workflow_type", "status"},
		),
		WorkflowDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "workflow_duration_seconds",
				Help:      "Workflow execution duration in seconds",
				Buckets:   []float64{10, 30, 60, 120, 300, 600, 1200, 1800},
			},
			[]string{"workflow_type"},
		),
		ActivitiesExecuted: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "activities_executed_total",
				Help:      "Total number of activities executed",
			},
			[]string{"activity_type", "status"},
		),

		// System metrics
		DBConnectionsActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "db_connections_active",
				Help:      "Number of active database connections",
			},
		),
		DBConnectionsIdle: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "db_connections_idle",
				Help:      "Number of idle database connections",
			},
		),
		CacheSize: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "cache_size",
				Help:      "Current cache size (number of entries)",
			},
		),
		GoroutinesActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "goroutines_active",
				Help:      "Number of active goroutines",
			},
		),
	}

	return m
}

// Handler returns the Prometheus HTTP handler
func (m *Metrics) Handler() http.Handler {
	return promhttp.Handler()
}

// RecordHTTPRequest records HTTP request metrics
func (m *Metrics) RecordHTTPRequest(method, path string, status int, duration time.Duration) {
	m.HTTPRequestsTotal.WithLabelValues(method, path, strconv.Itoa(status)).Inc()
	m.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
}

// RecordClaudeRequest records Claude API metrics
func (m *Metrics) RecordClaudeRequest(model, purpose, status string, duration time.Duration, inputTokens, outputTokens int, cost float64) {
	m.ClaudeRequestsTotal.WithLabelValues(model, purpose, status).Inc()
	m.ClaudeRequestDuration.WithLabelValues(model, purpose).Observe(duration.Seconds())
	m.ClaudeTokensUsed.WithLabelValues(model, "input").Add(float64(inputTokens))
	m.ClaudeTokensUsed.WithLabelValues(model, "output").Add(float64(outputTokens))
	m.ClaudeCostTotal.Add(cost)
}

// RecordTestRun records test run metrics
func (m *Metrics) RecordTestRun(tenantID, status string) {
	m.TestRunsTotal.WithLabelValues(tenantID, status).Inc()
}

// RecordTestExecution records test execution metrics
func (m *Metrics) RecordTestExecution(tenantID, status, testType string, count int) {
	m.TestsExecutedTotal.WithLabelValues(tenantID, status, testType).Add(float64(count))
}

// RecordSelfHealing records self-healing metrics
func (m *Metrics) RecordSelfHealing(strategy, status string) {
	m.SelfHealingAttempts.WithLabelValues(strategy, status).Inc()
}

// RecordDiscovery records discovery metrics
func (m *Metrics) RecordDiscovery(tenantID string, pagesCrawled int) {
	m.DiscoveryPages.WithLabelValues(tenantID).Observe(float64(pagesCrawled))
}

// RecordVisualAI records Visual AI metrics
func (m *Metrics) RecordVisualAI(model, operation, status string, duration time.Duration) {
	m.VisualAIRequestsTotal.WithLabelValues(model, operation, status).Inc()
	m.VisualAIRequestDuration.WithLabelValues(model, operation).Observe(duration.Seconds())
}

// RecordWorkflowStart records workflow start
func (m *Metrics) RecordWorkflowStart(workflowType string) {
	m.WorkflowsStarted.WithLabelValues(workflowType).Inc()
}

// RecordWorkflowComplete records workflow completion
func (m *Metrics) RecordWorkflowComplete(workflowType, status string, duration time.Duration) {
	m.WorkflowsCompleted.WithLabelValues(workflowType, status).Inc()
	m.WorkflowDuration.WithLabelValues(workflowType).Observe(duration.Seconds())
}

// RecordActivityExecution records activity execution
func (m *Metrics) RecordActivityExecution(activityType, status string) {
	m.ActivitiesExecuted.WithLabelValues(activityType, status).Inc()
}

// HTTPMiddleware returns middleware for recording HTTP metrics
func (m *Metrics) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.HTTPRequestsActive.Inc()
		defer m.HTTPRequestsActive.Dec()

		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		m.RecordHTTPRequest(r.Method, r.URL.Path, wrapped.statusCode, duration)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Global metrics instance
var globalMetrics *Metrics

// InitMetrics initializes the global metrics instance
func InitMetrics(namespace string) *Metrics {
	globalMetrics = NewMetrics(namespace)
	return globalMetrics
}

// GetMetrics returns the global metrics instance
func GetMetrics() *Metrics {
	if globalMetrics == nil {
		globalMetrics = NewMetrics("testforge")
	}
	return globalMetrics
}
