package resilience

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCircuitBreaker_StartsInClosedState(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig("test"))

	if cb.State() != StateClosed {
		t.Errorf("initial state = %v, want Closed", cb.State())
	}
}

func TestCircuitBreaker_TripsToOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		Name:        "test",
		MaxRequests: 1,
		Timeout:     100 * time.Millisecond,
		Interval:    1 * time.Second,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
	}
	cb := NewCircuitBreaker(config)

	// Simulate failures
	alwaysFail := func() (interface{}, error) {
		return nil, errors.New("failure")
	}

	for i := 0; i < 5; i++ {
		cb.Execute(alwaysFail)
	}

	if cb.State() != StateOpen {
		t.Errorf("state after failures = %v, want Open", cb.State())
	}
}

func TestCircuitBreaker_RejectsWhenOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		Name:        "test",
		MaxRequests: 1,
		Timeout:     10 * time.Second, // Long timeout
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 1
		},
	}
	cb := NewCircuitBreaker(config)

	// Trip the circuit
	cb.Execute(func() (interface{}, error) {
		return nil, errors.New("failure")
	})

	// Try another request
	_, err := cb.Execute(func() (interface{}, error) {
		return "success", nil
	})

	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("error = %v, want ErrCircuitOpen", err)
	}
}

func TestCircuitBreaker_TransitionsToHalfOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		Name:        "test",
		MaxRequests: 1,
		Timeout:     50 * time.Millisecond,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 1
		},
	}
	cb := NewCircuitBreaker(config)

	// Trip the circuit
	cb.Execute(func() (interface{}, error) {
		return nil, errors.New("failure")
	})

	if cb.State() != StateOpen {
		t.Fatalf("state = %v, want Open", cb.State())
	}

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	// Should be half-open now
	if cb.State() != StateHalfOpen {
		t.Errorf("state after timeout = %v, want HalfOpen", cb.State())
	}
}

func TestCircuitBreaker_ClosesAfterSuccessInHalfOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		Name:        "test",
		MaxRequests: 1,
		Timeout:     50 * time.Millisecond,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 1
		},
	}
	cb := NewCircuitBreaker(config)

	// Trip the circuit
	cb.Execute(func() (interface{}, error) {
		return nil, errors.New("failure")
	})

	// Wait for half-open
	time.Sleep(100 * time.Millisecond)

	// Successful request in half-open should close
	result, err := cb.Execute(func() (interface{}, error) {
		return "success", nil
	})

	if err != nil {
		t.Fatalf("successful request error = %v", err)
	}
	if result != "success" {
		t.Errorf("result = %v, want success", result)
	}
	if cb.State() != StateClosed {
		t.Errorf("state after success = %v, want Closed", cb.State())
	}
}

func TestCircuitBreaker_ReOpensAfterFailureInHalfOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		Name:        "test",
		MaxRequests: 1,
		Timeout:     50 * time.Millisecond,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 1
		},
	}
	cb := NewCircuitBreaker(config)

	// Trip the circuit
	cb.Execute(func() (interface{}, error) {
		return nil, errors.New("failure")
	})

	// Wait for half-open
	time.Sleep(100 * time.Millisecond)

	// Failing request in half-open should re-open
	cb.Execute(func() (interface{}, error) {
		return nil, errors.New("another failure")
	})

	if cb.State() != StateOpen {
		t.Errorf("state after failure in half-open = %v, want Open", cb.State())
	}
}

func TestCircuitBreaker_ExecuteWithContext(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig("test"))

	// Test with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := cb.ExecuteWithContext(ctx, func(ctx context.Context) (interface{}, error) {
		return "should not reach", nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

func TestCircuitBreaker_ConcurrentRequests(t *testing.T) {
	config := DefaultCircuitBreakerConfig("test")
	config.ReadyToTrip = func(counts Counts) bool {
		// Higher threshold for concurrent test
		return counts.TotalFailures >= 50
	}
	cb := NewCircuitBreaker(config)

	var wg sync.WaitGroup
	successes := 0
	var mu sync.Mutex

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cb.Execute(func() (interface{}, error) {
				return "ok", nil
			})
			if err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if successes != 100 {
		t.Errorf("successes = %d, want 100", successes)
	}
	if cb.State() != StateClosed {
		t.Errorf("state = %v, want Closed", cb.State())
	}
}

func TestCircuitBreaker_Counts(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig("test"))

	// Make some successful requests
	for i := 0; i < 5; i++ {
		cb.Execute(func() (interface{}, error) {
			return "ok", nil
		})
	}

	counts := cb.Counts()
	if counts.TotalSuccesses != 5 {
		t.Errorf("TotalSuccesses = %d, want 5", counts.TotalSuccesses)
	}
	if counts.Requests != 5 {
		t.Errorf("Requests = %d, want 5", counts.Requests)
	}

	// Make some failing requests
	for i := 0; i < 3; i++ {
		cb.Execute(func() (interface{}, error) {
			return nil, errors.New("fail")
		})
	}

	counts = cb.Counts()
	if counts.TotalFailures != 3 {
		t.Errorf("TotalFailures = %d, want 3", counts.TotalFailures)
	}
	if counts.ConsecutiveFailures != 3 {
		t.Errorf("ConsecutiveFailures = %d, want 3", counts.ConsecutiveFailures)
	}
}

func TestCircuitBreaker_OnStateChange(t *testing.T) {
	var changes []struct {
		from, to CircuitBreakerState
	}

	config := CircuitBreakerConfig{
		Name:        "test",
		MaxRequests: 1,
		Timeout:     50 * time.Millisecond,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 1
		},
		OnStateChange: func(name string, from, to CircuitBreakerState) {
			changes = append(changes, struct{ from, to CircuitBreakerState }{from, to})
		},
	}
	cb := NewCircuitBreaker(config)

	// Trip to open
	cb.Execute(func() (interface{}, error) {
		return nil, errors.New("failure")
	})

	// Wait for half-open
	time.Sleep(100 * time.Millisecond)
	cb.State() // Trigger state check

	// Success to close
	cb.Execute(func() (interface{}, error) {
		return "ok", nil
	})

	if len(changes) < 2 {
		t.Fatalf("expected at least 2 state changes, got %d", len(changes))
	}

	// First change should be Closed -> Open
	if changes[0].from != StateClosed || changes[0].to != StateOpen {
		t.Errorf("first change = %v->%v, want Closed->Open", changes[0].from, changes[0].to)
	}
}

func TestCircuitBreakerState_String(t *testing.T) {
	tests := []struct {
		state CircuitBreakerState
		want  string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{CircuitBreakerState(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %s, want %s", tt.state, got, tt.want)
		}
	}
}

func TestCircuitBreakerManager(t *testing.T) {
	manager := NewCircuitBreakerManager()

	// Get creates if not exists
	cb1 := manager.GetOrCreate("service-a")
	cb2 := manager.GetOrCreate("service-a")

	if cb1 != cb2 {
		t.Error("should return same circuit breaker for same name")
	}

	// Different names get different breakers
	cb3 := manager.GetOrCreate("service-b")
	if cb1 == cb3 {
		t.Error("should return different circuit breakers for different names")
	}

	// Check all states
	states := manager.AllStates()
	if len(states) != 2 {
		t.Errorf("AllStates() len = %d, want 2", len(states))
	}
	if states["service-a"] != StateClosed {
		t.Error("service-a should be closed")
	}
	if states["service-b"] != StateClosed {
		t.Error("service-b should be closed")
	}
}
