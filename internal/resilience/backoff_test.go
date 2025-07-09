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
	fn := func(ctx context.Context) error {
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
	fn := func(ctx context.Context) error {
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
	fn := func(ctx context.Context) error {
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
	fn := func(ctx context.Context) error {
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
	fn := func(ctx context.Context) error {
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
	fn := func(ctx context.Context) error {
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
	fn := func(ctx context.Context) error {
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
	fn := func(ctx context.Context) error {
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
