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
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestDefaultBackoffConfig(t *testing.T) {
	config := DefaultBackoffConfig()

	if config.BaseDelay != 1*time.Second {
		t.Errorf("Expected BaseDelay to be 1 second, got %v", config.BaseDelay)
	}

	if config.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries to be 3, got %d", config.MaxRetries)
	}

	if config.Multiplier != 2.0 {
		t.Errorf("Expected Multiplier to be 2.0, got %f", config.Multiplier)
	}

	if config.MaxDelay != 30*time.Second {
		t.Errorf("Expected MaxDelay to be 30 seconds, got %v", config.MaxDelay)
	}
}

func TestWithExponentialBackoff_Success(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultBackoffConfig()

	attempts := 0
	fn := func(_ context.Context) error {
		attempts++
		return nil
	}

	err := WithExponentialBackoff(context.Background(), logger, config, fn)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if attempts != 1 {
		t.Errorf("Expected 1 attempt, got %d", attempts)
	}
}

func TestWithExponentialBackoff_SuccessAfterRetry(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultBackoffConfig()
	config.BaseDelay = 10 * time.Millisecond // Speed up test

	attempts := 0
	fn := func(_ context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	}

	start := time.Now()
	err := WithExponentialBackoff(context.Background(), logger, config, fn)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}

	// Should have some delay from retries
	if duration < 10*time.Millisecond {
		t.Errorf("Expected some delay from retries, got %v", duration)
	}
}

func TestWithExponentialBackoff_ExhaustRetries(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultBackoffConfig()
	config.BaseDelay = 1 * time.Millisecond // Speed up test
	config.MaxRetries = 2

	attempts := 0
	testError := errors.New("persistent error")
	fn := func(_ context.Context) error {
		attempts++
		return testError
	}

	err := WithExponentialBackoff(context.Background(), logger, config, fn)

	if err == nil {
		t.Error("Expected error after exhausting retries")
	}

	if attempts != 3 { // MaxRetries + 1
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}

	if !errors.Is(err, testError) {
		t.Errorf("Expected wrapped error to contain original error")
	}
}

func TestWithExponentialBackoff_NonRetryableError(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultBackoffConfig()
	config.RetryOnFunc = func(err error) bool {
		return err.Error() != "non-retryable"
	}

	attempts := 0
	fn := func(_ context.Context) error {
		attempts++
		return errors.New("non-retryable")
	}

	err := WithExponentialBackoff(context.Background(), logger, config, fn)

	if err == nil {
		t.Error("Expected error for non-retryable error")
	}

	if attempts != 1 {
		t.Errorf("Expected 1 attempt for non-retryable error, got %d", attempts)
	}
}

func TestWithExponentialBackoff_ContextCancellation(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultBackoffConfig()
	config.BaseDelay = 100 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())

	attempts := 0
	fn := func(_ context.Context) error {
		attempts++
		if attempts == 1 {
			return errors.New("first error")
		}
		return nil
	}

	// Cancel context after a short delay
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := WithExponentialBackoff(ctx, logger, config, fn)

	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}

	if attempts != 1 {
		t.Errorf("Expected 1 attempt before cancellation, got %d", attempts)
	}
}

func TestSimpleRetry(t *testing.T) {
	logger := zap.NewNop()

	attempts := 0
	fn := func(_ context.Context) error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary error")
		}
		return nil
	}

	err := SimpleRetry(context.Background(), logger, fn)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}

func TestRetryWithMaxAttempts(t *testing.T) {
	logger := zap.NewNop()
	maxRetries := 1

	attempts := 0
	fn := func(_ context.Context) error {
		attempts++
		return errors.New("persistent error")
	}

	err := RetryWithMaxAttempts(context.Background(), logger, maxRetries, fn)

	if err == nil {
		t.Error("Expected error after exhausting retries")
	}

	if attempts != 2 { // maxRetries + 1
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}

func TestRetryWithBaseDelay(t *testing.T) {
	logger := zap.NewNop()
	baseDelay := 5 * time.Millisecond

	attempts := 0
	fn := func(_ context.Context) error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary error")
		}
		return nil
	}

	start := time.Now()
	err := RetryWithBaseDelay(context.Background(), logger, baseDelay, fn)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}

	// Should have at least the base delay minus jitter tolerance (10% of base delay)
	jitterTolerance := time.Duration(float64(baseDelay) * 0.1)
	minExpectedDelay := baseDelay - jitterTolerance
	if duration < minExpectedDelay {
		t.Errorf("Expected at least %v delay (with jitter tolerance), got %v", minExpectedDelay, duration)
	}
}

func TestDefaultRetryOnFunc(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "context canceled",
			err:      context.Canceled,
			expected: false,
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: false,
		},
		{
			name:     "regular error",
			err:      errors.New("some error"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DefaultRetryOnFunc(tt.err)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestRetryOnSpecificErrors tests retry behavior with specific error types
func TestRetryOnSpecificErrors(t *testing.T) {
	tests := []struct {
		name           string
		errorMsg       string
		shouldRetry    bool
		expectedTiming time.Duration
	}{
		{
			name:           "rate limit error",
			errorMsg:       "rate limit exceeded",
			shouldRetry:    true,
			expectedTiming: 10 * time.Millisecond, // Base delay for test
		},
		{
			name:           "timeout error",
			errorMsg:       "request timeout",
			shouldRetry:    true,
			expectedTiming: 10 * time.Millisecond,
		},
		{
			name:           "server error",
			errorMsg:       "internal server error",
			shouldRetry:    true,
			expectedTiming: 10 * time.Millisecond,
		},
		{
			name:           "connection error",
			errorMsg:       "connection refused",
			shouldRetry:    true,
			expectedTiming: 10 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zap.NewNop()
			config := DefaultBackoffConfig()
			config.BaseDelay = tt.expectedTiming
			config.MaxRetries = 1

			attempts := 0
			fn := func(_ context.Context) error {
				attempts++
				if attempts == 1 {
					return errors.New(tt.errorMsg)
				}
				return nil
			}

			start := time.Now()
			err := WithExponentialBackoff(context.Background(), logger, config, fn)
			duration := time.Since(start)

			if err != nil {
				t.Errorf("Expected no error after retry, got %v", err)
			}

			if attempts != 2 {
				t.Errorf("Expected 2 attempts, got %d", attempts)
			}

			// Should have at least the base delay minus jitter tolerance (10% of base delay)
			jitterTolerance := time.Duration(float64(tt.expectedTiming) * 0.1)
			minExpectedDelay := tt.expectedTiming - jitterTolerance
			if duration < minExpectedDelay {
				t.Errorf("Expected at least %v delay (with jitter tolerance), got %v", minExpectedDelay, duration)
			}
		})
	}
}

// TestNonRetryableErrors tests that certain errors are not retried
func TestNonRetryableErrors(t *testing.T) {
	tests := []struct {
		name       string
		errorMsg   string
		statusCode int
	}{
		{
			name:       "unauthorized (401)",
			errorMsg:   "unauthorized access",
			statusCode: 401,
		},
		{
			name:       "forbidden (403)",
			errorMsg:   "forbidden operation",
			statusCode: 403,
		},
		{
			name:       "not found (404)",
			errorMsg:   "resource not found",
			statusCode: 404,
		},
		{
			name:       "bad request (400)",
			errorMsg:   "bad request format",
			statusCode: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zap.NewNop()
			config := DefaultBackoffConfig()
			config.BaseDelay = 1 * time.Millisecond
			config.MaxRetries = 3
			config.RetryOnFunc = func(err error) bool {
				// Don't retry on 4xx errors (client errors)
				if err != nil {
					errStr := err.Error()
					if strings.Contains(errStr, "unauthorized") ||
						strings.Contains(errStr, "forbidden") ||
						strings.Contains(errStr, "not found") ||
						strings.Contains(errStr, "bad request") {
						return false
					}
				}
				return DefaultRetryOnFunc(err)
			}

			attempts := 0
			fn := func(_ context.Context) error {
				attempts++
				return errors.New(tt.errorMsg)
			}

			err := WithExponentialBackoff(context.Background(), logger, config, fn)

			if err == nil {
				t.Error("Expected error for non-retryable error")
			}

			if attempts != 1 {
				t.Errorf("Expected 1 attempt for non-retryable error, got %d", attempts)
			}

			if !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("Expected error to contain %s, got %v", tt.errorMsg, err)
			}
		})
	}
}

// TestExponentialBackoffTiming tests the 1s, 2s, 4s progression
func TestExponentialBackoffTiming(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultBackoffConfig()
	config.BaseDelay = 50 * time.Millisecond // Speed up test
	config.MaxRetries = 3
	config.Jitter = false // Disable jitter for predictable timing

	var delays []time.Duration
	start := time.Now()
	lastTime := start

	attempts := 0
	fn := func(_ context.Context) error {
		attempts++
		if attempts > 1 {
			now := time.Now()
			delay := now.Sub(lastTime)
			delays = append(delays, delay)
			lastTime = now
		}
		if attempts <= 3 {
			return errors.New("retry error")
		}
		return nil
	}

	err := WithExponentialBackoff(context.Background(), logger, config, fn)

	if err != nil {
		t.Errorf("Expected no error after successful retry, got %v", err)
	}

	if attempts != 4 {
		t.Errorf("Expected 4 attempts, got %d", attempts)
	}

	if len(delays) != 3 {
		t.Errorf("Expected 3 delays, got %d", len(delays))
	}

	// Check exponential progression: 50ms, 100ms, 200ms
	expectedDelays := []time.Duration{
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
	}

	for i, expectedDelay := range expectedDelays {
		if i < len(delays) {
			// Allow 20% tolerance for timing variations
			tolerance := expectedDelay / 5
			if delays[i] < expectedDelay-tolerance || delays[i] > expectedDelay+tolerance {
				t.Errorf("Delay %d: expected ~%v, got %v", i+1, expectedDelay, delays[i])
			}
		}
	}
}
