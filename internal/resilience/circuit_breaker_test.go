// Copyright 2024 AI SA Assistant Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package resilience

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewCircuitBreaker(t *testing.T) {
	config := DefaultCircuitBreakerConfig("test")
	logger := zap.NewNop()

	cb := NewCircuitBreaker(config, logger)

	if cb == nil {
		t.Fatal("Expected circuit breaker to be created")
	}

	if cb.GetState() != CircuitClosed {
		t.Errorf("Expected initial state to be closed, got %v", cb.GetState())
	}

	stats := cb.GetStats()
	if stats.Name != "test" {
		t.Errorf("Expected name 'test', got %s", stats.Name)
	}
}

func TestCircuitBreakerStateTransitions(t *testing.T) {
	config := DefaultCircuitBreakerConfig("test")
	config.MaxFailures = 2
	config.ResetTimeout = 10 * time.Millisecond
	config.HalfOpenMaxRequests = 1 // Allow circuit to close after 1 successful request
	logger := zap.NewNop()

	cb := NewCircuitBreaker(config, logger)

	// Initial state should be closed
	if cb.GetState() != CircuitClosed {
		t.Errorf("Expected initial state to be closed, got %v", cb.GetState())
	}

	// Successful execution should keep it closed
	err := cb.Execute(context.Background(), func(_ context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if cb.GetState() != CircuitClosed {
		t.Errorf("Expected state to remain closed after success, got %v", cb.GetState())
	}

	// First failure should keep it closed
	err = cb.Execute(context.Background(), func(_ context.Context) error {
		return errors.New("failure 1")
	})
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if cb.GetState() != CircuitClosed {
		t.Errorf("Expected state to remain closed after first failure, got %v", cb.GetState())
	}

	// Second failure should open the circuit
	err = cb.Execute(context.Background(), func(_ context.Context) error {
		return errors.New("failure 2")
	})
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if cb.GetState() != CircuitOpen {
		t.Errorf("Expected state to be open after max failures, got %v", cb.GetState())
	}

	// Subsequent calls should fail fast
	err = cb.Execute(context.Background(), func(_ context.Context) error {
		return nil
	})
	if err != ErrCircuitBreakerOpen {
		t.Errorf("Expected ErrCircuitBreakerOpen, got %v", err)
	}

	// Wait for reset timeout
	time.Sleep(20 * time.Millisecond)

	// Should transition to half-open and allow one request
	err = cb.Execute(context.Background(), func(_ context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected no error in half-open state, got %v", err)
	}
	if cb.GetState() != CircuitClosed {
		t.Errorf("Expected state to be closed after successful recovery, got %v", cb.GetState())
	}
}

func TestCircuitBreakerHalfOpenState(t *testing.T) {
	config := DefaultCircuitBreakerConfig("test")
	config.MaxFailures = 1
	config.ResetTimeout = 10 * time.Millisecond
	config.HalfOpenMaxRequests = 2
	logger := zap.NewNop()

	cb := NewCircuitBreaker(config, logger)

	// Trigger failure to open circuit
	err := cb.Execute(context.Background(), func(_ context.Context) error {
		return errors.New("failure")
	})
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if cb.GetState() != CircuitOpen {
		t.Errorf("Expected state to be open, got %v", cb.GetState())
	}

	// Wait for reset timeout
	time.Sleep(20 * time.Millisecond)

	// First request should transition to half-open
	err = cb.Execute(context.Background(), func(_ context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected no error in half-open state, got %v", err)
	}

	// Second successful request should close the circuit
	err = cb.Execute(context.Background(), func(_ context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if cb.GetState() != CircuitClosed {
		t.Errorf("Expected state to be closed after recovery, got %v", cb.GetState())
	}
}

func TestCircuitBreakerHalfOpenFailure(t *testing.T) {
	config := DefaultCircuitBreakerConfig("test")
	config.MaxFailures = 1
	config.ResetTimeout = 10 * time.Millisecond
	logger := zap.NewNop()

	cb := NewCircuitBreaker(config, logger)

	// Trigger failure to open circuit
	err := cb.Execute(context.Background(), func(_ context.Context) error {
		return errors.New("failure")
	})
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if cb.GetState() != CircuitOpen {
		t.Errorf("Expected state to be open, got %v", cb.GetState())
	}

	// Wait for reset timeout
	time.Sleep(20 * time.Millisecond)

	// Failure in half-open should immediately open the circuit
	err = cb.Execute(context.Background(), func(_ context.Context) error {
		return errors.New("failure in half-open")
	})
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if cb.GetState() != CircuitOpen {
		t.Errorf("Expected state to be open after half-open failure, got %v", cb.GetState())
	}
}

func TestCircuitBreakerStats(t *testing.T) {
	config := DefaultCircuitBreakerConfig("test")
	config.MaxFailures = 2
	logger := zap.NewNop()

	cb := NewCircuitBreaker(config, logger)

	// Execute some successful requests
	for i := 0; i < 3; i++ {
		err := cb.Execute(context.Background(), func(_ context.Context) error {
			return nil
		})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	}

	// Execute some failed requests
	for i := 0; i < 2; i++ {
		err := cb.Execute(context.Background(), func(_ context.Context) error {
			return errors.New("failure")
		})
		if err == nil {
			t.Error("Expected error, got nil")
		}
	}

	stats := cb.GetStats()

	if stats.Name != "test" {
		t.Errorf("Expected name 'test', got %s", stats.Name)
	}

	if stats.State != CircuitOpen {
		t.Errorf("Expected state to be open, got %v", stats.State)
	}

	if stats.Failures != 2 {
		t.Errorf("Expected 2 failures, got %d", stats.Failures)
	}

	if stats.FailedReqs != 2 {
		t.Errorf("Expected 2 failed requests, got %d", stats.FailedReqs)
	}

	if stats.SuccessfulReqs != 3 {
		t.Errorf("Expected 3 successful requests, got %d", stats.SuccessfulReqs)
	}
}

func TestCircuitBreakerReset(t *testing.T) {
	config := DefaultCircuitBreakerConfig("test")
	config.MaxFailures = 1
	logger := zap.NewNop()

	cb := NewCircuitBreaker(config, logger)

	// Trigger failure to open circuit
	err := cb.Execute(context.Background(), func(_ context.Context) error {
		return errors.New("failure")
	})
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if cb.GetState() != CircuitOpen {
		t.Errorf("Expected state to be open, got %v", cb.GetState())
	}

	// Reset should close the circuit
	cb.Reset()
	if cb.GetState() != CircuitClosed {
		t.Errorf("Expected state to be closed after reset, got %v", cb.GetState())
	}

	// Should be able to execute successfully
	err = cb.Execute(context.Background(), func(_ context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected no error after reset, got %v", err)
	}
}

func TestCircuitBreakerIsFailureFunc(t *testing.T) {
	config := DefaultCircuitBreakerConfig("test")
	config.MaxFailures = 1
	config.IsFailureFunc = func(err error) bool {
		return err != nil && err.Error() == "real failure"
	}
	logger := zap.NewNop()

	cb := NewCircuitBreaker(config, logger)

	// Non-failure error should not count as failure
	err := cb.Execute(context.Background(), func(_ context.Context) error {
		return errors.New("not a real failure")
	})
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if cb.GetState() != CircuitClosed {
		t.Errorf("Expected state to remain closed, got %v", cb.GetState())
	}

	// Real failure should open the circuit
	err = cb.Execute(context.Background(), func(_ context.Context) error {
		return errors.New("real failure")
	})
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if cb.GetState() != CircuitOpen {
		t.Errorf("Expected state to be open after real failure, got %v", cb.GetState())
	}
}

func TestCircuitBreakerStateChange(t *testing.T) {
	var stateChanges []string

	config := DefaultCircuitBreakerConfig("test")
	config.MaxFailures = 1
	config.OnStateChange = func(from, to CircuitState) {
		stateChanges = append(stateChanges, from.String()+" -> "+to.String())
	}
	logger := zap.NewNop()

	cb := NewCircuitBreaker(config, logger)

	// Trigger failure to open circuit
	err := cb.Execute(context.Background(), func(_ context.Context) error {
		return errors.New("failure")
	})
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// Wait for callback to be called
	time.Sleep(10 * time.Millisecond)

	if len(stateChanges) != 1 {
		t.Errorf("Expected 1 state change, got %d", len(stateChanges))
	}

	if stateChanges[0] != "closed -> open" {
		t.Errorf("Expected 'closed -> open', got %s", stateChanges[0])
	}
}
