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
	"sync"
	"time"

	"go.uber.org/zap"
)

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	// CircuitClosed means the circuit breaker is closed (normal operation)
	CircuitClosed CircuitState = iota
	// CircuitOpen means the circuit breaker is open (failing fast)
	CircuitOpen
	// CircuitHalfOpen means the circuit breaker is half-open (testing recovery)
	CircuitHalfOpen
)

// String returns the string representation of the circuit state
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig holds configuration for circuit breaker behavior
type CircuitBreakerConfig struct {
	Name                string
	MaxFailures         int
	ResetTimeout        time.Duration
	HalfOpenMaxRequests int
	IsFailureFunc       func(error) bool
	OnStateChange       func(CircuitState, CircuitState)
}

// DefaultCircuitBreakerConfig returns default configuration for circuit breaker
func DefaultCircuitBreakerConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:                name,
		MaxFailures:         5,
		ResetTimeout:        60 * time.Second,
		HalfOpenMaxRequests: 3,
		IsFailureFunc:       DefaultIsFailureFunc,
		OnStateChange:       nil,
	}
}

// DefaultIsFailureFunc determines if an error should be considered a failure
func DefaultIsFailureFunc(err error) bool {
	return err != nil
}

// CircuitBreakerStats holds statistics about circuit breaker performance
type CircuitBreakerStats struct {
	Name            string        `json:"name"`
	State           CircuitState  `json:"state"`
	Failures        int           `json:"failures"`
	Requests        int           `json:"requests"`
	SuccessfulReqs  int           `json:"successful_requests"`
	FailedReqs      int           `json:"failed_requests"`
	LastFailureTime time.Time     `json:"last_failure_time"`
	LastSuccessTime time.Time     `json:"last_success_time"`
	StateChanged    time.Time     `json:"state_changed"`
	Uptime          time.Duration `json:"uptime"`
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config          CircuitBreakerConfig
	state           CircuitState
	failures        int
	requests        int
	successfulReqs  int
	failedReqs      int
	lastFailureTime time.Time
	lastSuccessTime time.Time
	stateChanged    time.Time
	createdAt       time.Time
	mu              sync.RWMutex
	logger          *zap.Logger
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration
func NewCircuitBreaker(config CircuitBreakerConfig, logger *zap.Logger) *CircuitBreaker {
	if logger == nil {
		logger = zap.NewNop()
	}

	now := time.Now()
	cb := &CircuitBreaker{
		config:       config,
		state:        CircuitClosed,
		stateChanged: now,
		createdAt:    now,
		logger:       logger,
	}

	logger.Info("Circuit breaker created",
		zap.String("name", config.Name),
		zap.Int("max_failures", config.MaxFailures),
		zap.Duration("reset_timeout", config.ResetTimeout))

	return cb
}

// ErrCircuitBreakerOpen is returned when the circuit breaker is open
var ErrCircuitBreakerOpen = errors.New("circuit breaker is open")

// Execute runs the given function through the circuit breaker
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(context.Context) error) error {
	if cb == nil {
		return ErrCircuitBreakerOpen
	}

	// Check if we can execute the request
	if !cb.canExecute() {
		return ErrCircuitBreakerOpen
	}

	// Execute the function
	err := fn(ctx)

	// Record the result
	cb.recordResult(err)

	return err
}

// canExecute determines if a request can be executed based on circuit breaker state
func (cb *CircuitBreaker) canExecute() bool {
	if cb == nil {
		return false
	}
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// Check if we should transition to half-open
		if time.Since(cb.stateChanged) > cb.config.ResetTimeout {
			cb.mu.RUnlock()
			cb.mu.Lock()
			// Double-check after acquiring write lock
			if cb.state == CircuitOpen && time.Since(cb.stateChanged) > cb.config.ResetTimeout {
				cb.setState(CircuitHalfOpen)
			}
			cb.mu.Unlock()
			cb.mu.RLock()
		}
		return cb.state == CircuitHalfOpen
	case CircuitHalfOpen:
		return cb.requests < cb.config.HalfOpenMaxRequests
	default:
		return false
	}
}

// recordResult records the result of a function execution
func (cb *CircuitBreaker) recordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.requests++

	if cb.config.IsFailureFunc(err) {
		cb.failures++
		cb.failedReqs++
		cb.lastFailureTime = time.Now()

		cb.logger.Debug("Circuit breaker recorded failure",
			zap.String("name", cb.config.Name),
			zap.Error(err),
			zap.Int("failures", cb.failures),
			zap.String("state", cb.state.String()))

		// Check if we should open the circuit
		if cb.state == CircuitClosed && cb.failures >= cb.config.MaxFailures {
			cb.setState(CircuitOpen)
		} else if cb.state == CircuitHalfOpen {
			// Any failure in half-open state should open the circuit
			cb.setState(CircuitOpen)
		}
	} else {
		cb.successfulReqs++
		cb.lastSuccessTime = time.Now()

		cb.logger.Debug("Circuit breaker recorded success",
			zap.String("name", cb.config.Name),
			zap.Int("successful_requests", cb.successfulReqs),
			zap.String("state", cb.state.String()))

		// Reset failures on success
		if cb.state == CircuitClosed {
			cb.failures = 0
		} else if cb.state == CircuitHalfOpen {
			// Close the circuit if we've had enough successful requests
			if cb.successfulReqs >= cb.config.HalfOpenMaxRequests {
				cb.setState(CircuitClosed)
				cb.failures = 0
			}
		}
	}
}

// setState changes the circuit breaker state and triggers callbacks
func (cb *CircuitBreaker) setState(newState CircuitState) {
	oldState := cb.state
	cb.state = newState
	cb.stateChanged = time.Now()
	cb.requests = 0
	// Only reset successfulReqs when transitioning to half-open, not when closing
	if newState == CircuitHalfOpen {
		cb.successfulReqs = 0
	}

	cb.logger.Info("Circuit breaker state changed",
		zap.String("name", cb.config.Name),
		zap.String("from", oldState.String()),
		zap.String("to", newState.String()),
		zap.Int("failures", cb.failures))

	// Trigger state change callback if configured
	if cb.config.OnStateChange != nil {
		go cb.config.OnStateChange(oldState, newState)
	}
}

// GetStats returns current statistics about the circuit breaker
func (cb *CircuitBreaker) GetStats() CircuitBreakerStats {
	if cb == nil {
		return CircuitBreakerStats{
			Name:  "unknown",
			State: CircuitClosed,
		}
	}
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		Name:            cb.config.Name,
		State:           cb.state,
		Failures:        cb.failures,
		Requests:        cb.requests,
		SuccessfulReqs:  cb.successfulReqs,
		FailedReqs:      cb.failedReqs,
		LastFailureTime: cb.lastFailureTime,
		LastSuccessTime: cb.lastSuccessTime,
		StateChanged:    cb.stateChanged,
		Uptime:          time.Since(cb.createdAt),
	}
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitState {
	if cb == nil {
		return CircuitClosed
	}
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	if cb == nil {
		return
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.logger.Info("Circuit breaker manually reset", zap.String("name", cb.config.Name))
	cb.setState(CircuitClosed)
	cb.failures = 0
}
