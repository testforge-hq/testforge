// Package resilience provides circuit breaker and other resilience patterns
// for handling external service failures gracefully.
package resilience

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// CircuitBreakerState represents the state of a circuit breaker
type CircuitBreakerState int32

const (
	// StateClosed - circuit is closed, requests flow normally
	StateClosed CircuitBreakerState = iota
	// StateOpen - circuit is open, requests are rejected immediately
	StateOpen
	// StateHalfOpen - circuit is testing if the service has recovered
	StateHalfOpen
)

func (s CircuitBreakerState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

var (
	// ErrCircuitOpen is returned when the circuit breaker is open
	ErrCircuitOpen = errors.New("circuit breaker is open")

	// ErrTooManyRequests is returned when too many requests are in flight in half-open state
	ErrTooManyRequests = errors.New("too many requests in half-open state")
)

// CircuitBreakerConfig holds configuration for the circuit breaker
type CircuitBreakerConfig struct {
	// Name identifies this circuit breaker (for logging/metrics)
	Name string

	// MaxRequests is the max number of requests allowed in half-open state
	MaxRequests uint32

	// Interval is the cyclic period of the closed state for the CircuitBreaker to
	// clear the internal counts. If 0, counts are never cleared in closed state.
	Interval time.Duration

	// Timeout is how long to wait before transitioning from open to half-open
	Timeout time.Duration

	// ReadyToTrip is called with a copy of Counts whenever a request fails
	// If it returns true, the circuit breaker trips to open state
	ReadyToTrip func(counts Counts) bool

	// OnStateChange is called whenever the state changes
	OnStateChange func(name string, from, to CircuitBreakerState)

	// IsSuccessful determines if the response should be considered a success
	// By default, any non-error response is considered successful
	IsSuccessful func(err error) bool
}

// DefaultCircuitBreakerConfig returns sensible defaults for external APIs
func DefaultCircuitBreakerConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:        name,
		MaxRequests: 3,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts Counts) bool {
			// Trip if failure rate exceeds 60% with at least 5 requests
			if counts.Requests < 5 {
				return false
			}
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return failureRatio >= 0.6
		},
		IsSuccessful: func(err error) bool {
			return err == nil
		},
	}
}

// Counts holds the numbers of requests and their successes/failures
type Counts struct {
	Requests             uint32
	TotalSuccesses       uint32
	TotalFailures        uint32
	ConsecutiveSuccesses uint32
	ConsecutiveFailures  uint32
}

func (c *Counts) onRequest() {
	atomic.AddUint32(&c.Requests, 1)
}

func (c *Counts) onSuccess() {
	atomic.AddUint32(&c.TotalSuccesses, 1)
	atomic.AddUint32(&c.ConsecutiveSuccesses, 1)
	atomic.StoreUint32(&c.ConsecutiveFailures, 0)
}

func (c *Counts) onFailure() {
	atomic.AddUint32(&c.TotalFailures, 1)
	atomic.AddUint32(&c.ConsecutiveFailures, 1)
	atomic.StoreUint32(&c.ConsecutiveSuccesses, 0)
}

func (c *Counts) clear() {
	atomic.StoreUint32(&c.Requests, 0)
	atomic.StoreUint32(&c.TotalSuccesses, 0)
	atomic.StoreUint32(&c.TotalFailures, 0)
	atomic.StoreUint32(&c.ConsecutiveSuccesses, 0)
	atomic.StoreUint32(&c.ConsecutiveFailures, 0)
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	name          string
	maxRequests   uint32
	interval      time.Duration
	timeout       time.Duration
	readyToTrip   func(counts Counts) bool
	onStateChange func(name string, from, to CircuitBreakerState)
	isSuccessful  func(err error) bool

	mu          sync.Mutex
	state       CircuitBreakerState
	generation  uint64
	counts      Counts
	expiry      time.Time
	halfOpenReq uint32
}

// NewCircuitBreaker creates a new circuit breaker with the given config
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	cb := &CircuitBreaker{
		name:          config.Name,
		maxRequests:   config.MaxRequests,
		interval:      config.Interval,
		timeout:       config.Timeout,
		readyToTrip:   config.ReadyToTrip,
		onStateChange: config.OnStateChange,
		isSuccessful:  config.IsSuccessful,
	}

	if cb.maxRequests == 0 {
		cb.maxRequests = 1
	}
	if cb.timeout == 0 {
		cb.timeout = 30 * time.Second
	}
	if cb.readyToTrip == nil {
		cb.readyToTrip = func(counts Counts) bool {
			return counts.ConsecutiveFailures > 5
		}
	}
	if cb.isSuccessful == nil {
		cb.isSuccessful = func(err error) bool {
			return err == nil
		}
	}

	cb.toNewGeneration(time.Now())

	return cb
}

// State returns the current state of the circuit breaker
func (cb *CircuitBreaker) State() CircuitBreakerState {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	state, _ := cb.currentState(now)
	return state
}

// Counts returns current counts (for monitoring)
func (cb *CircuitBreaker) Counts() Counts {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.counts
}

// Execute runs the given request if the circuit breaker allows it
func (cb *CircuitBreaker) Execute(req func() (interface{}, error)) (interface{}, error) {
	return cb.ExecuteWithContext(context.Background(), func(ctx context.Context) (interface{}, error) {
		return req()
	})
}

// ExecuteWithContext runs the given request with context if the circuit breaker allows it
func (cb *CircuitBreaker) ExecuteWithContext(ctx context.Context, req func(context.Context) (interface{}, error)) (interface{}, error) {
	generation, err := cb.beforeRequest()
	if err != nil {
		return nil, err
	}

	// Check context before executing
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	result, err := req(ctx)

	cb.afterRequest(generation, cb.isSuccessful(err))

	return result, err
}

func (cb *CircuitBreaker) beforeRequest() (uint64, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	state, generation := cb.currentState(now)

	switch state {
	case StateOpen:
		return generation, ErrCircuitOpen
	case StateHalfOpen:
		if cb.halfOpenReq >= cb.maxRequests {
			return generation, ErrTooManyRequests
		}
		cb.halfOpenReq++
	}

	cb.counts.onRequest()
	return generation, nil
}

func (cb *CircuitBreaker) afterRequest(before uint64, success bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	state, generation := cb.currentState(now)

	// If generation changed, counts have been cleared - nothing to do
	if generation != before {
		return
	}

	if success {
		cb.onSuccess(state, now)
	} else {
		cb.onFailure(state, now)
	}
}

func (cb *CircuitBreaker) onSuccess(state CircuitBreakerState, now time.Time) {
	switch state {
	case StateClosed:
		cb.counts.onSuccess()
	case StateHalfOpen:
		cb.counts.onSuccess()
		// If we've had enough successful requests in half-open, close the circuit
		if cb.counts.ConsecutiveSuccesses >= cb.maxRequests {
			cb.setState(StateClosed, now)
		}
	}
}

func (cb *CircuitBreaker) onFailure(state CircuitBreakerState, now time.Time) {
	switch state {
	case StateClosed:
		cb.counts.onFailure()
		if cb.readyToTrip(cb.counts) {
			cb.setState(StateOpen, now)
		}
	case StateHalfOpen:
		// Any failure in half-open state trips back to open
		cb.setState(StateOpen, now)
	}
}

func (cb *CircuitBreaker) currentState(now time.Time) (CircuitBreakerState, uint64) {
	switch cb.state {
	case StateClosed:
		if !cb.expiry.IsZero() && cb.expiry.Before(now) {
			cb.toNewGeneration(now)
		}
	case StateOpen:
		if cb.expiry.Before(now) {
			cb.setState(StateHalfOpen, now)
		}
	}
	return cb.state, cb.generation
}

func (cb *CircuitBreaker) setState(state CircuitBreakerState, now time.Time) {
	if cb.state == state {
		return
	}

	prev := cb.state
	cb.state = state

	cb.toNewGeneration(now)

	if cb.onStateChange != nil {
		cb.onStateChange(cb.name, prev, state)
	}
}

func (cb *CircuitBreaker) toNewGeneration(now time.Time) {
	cb.generation++
	cb.counts.clear()
	cb.halfOpenReq = 0

	var zero time.Time
	switch cb.state {
	case StateClosed:
		if cb.interval > 0 {
			cb.expiry = now.Add(cb.interval)
		} else {
			cb.expiry = zero
		}
	case StateOpen:
		cb.expiry = now.Add(cb.timeout)
	case StateHalfOpen:
		cb.expiry = zero
	}
}

// CircuitBreakerManager manages multiple circuit breakers
type CircuitBreakerManager struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
}

// NewCircuitBreakerManager creates a new manager
func NewCircuitBreakerManager() *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
	}
}

// Get returns the circuit breaker for the given name, creating it if needed
func (m *CircuitBreakerManager) Get(name string, config CircuitBreakerConfig) *CircuitBreaker {
	m.mu.RLock()
	cb, ok := m.breakers[name]
	m.mu.RUnlock()

	if ok {
		return cb
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if cb, ok := m.breakers[name]; ok {
		return cb
	}

	config.Name = name
	cb = NewCircuitBreaker(config)
	m.breakers[name] = cb
	return cb
}

// GetOrCreate returns existing breaker or creates with default config
func (m *CircuitBreakerManager) GetOrCreate(name string) *CircuitBreaker {
	return m.Get(name, DefaultCircuitBreakerConfig(name))
}

// AllStates returns the state of all circuit breakers
func (m *CircuitBreakerManager) AllStates() map[string]CircuitBreakerState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	states := make(map[string]CircuitBreakerState, len(m.breakers))
	for name, cb := range m.breakers {
		states[name] = cb.State()
	}
	return states
}
