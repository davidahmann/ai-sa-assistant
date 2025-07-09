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

// Package resilience provides utilities for implementing resilient patterns
// such as exponential backoff and circuit breakers in the AI SA Assistant.
package resilience

import (
	"context"
	"fmt"
	"math"
	"time"

	"go.uber.org/zap"
)

// BackoffConfig holds configuration for exponential backoff retry logic
type BackoffConfig struct {
	BaseDelay   time.Duration
	MaxRetries  int
	MaxDelay    time.Duration
	Multiplier  float64
	Jitter      bool
	RetryOnFunc func(error) bool
}

const (
	// DefaultMaxRetries is the default maximum number of retry attempts
	DefaultMaxRetries = 3
	// DefaultMaxDelaySeconds is the default maximum delay in seconds
	DefaultMaxDelaySeconds = 30
	// DefaultMultiplier is the default exponential backoff multiplier
	DefaultMultiplier = 2.0
	// JitterModulus is used for random jitter calculation
	JitterModulus = 1000
)

// DefaultBackoffConfig returns the default configuration for exponential backoff
// as specified in the requirements: base delay 1s, max retries 3, doubles per retry
func DefaultBackoffConfig() BackoffConfig {
	return BackoffConfig{
		BaseDelay:   1 * time.Second,
		MaxRetries:  DefaultMaxRetries,
		MaxDelay:    DefaultMaxDelaySeconds * time.Second,
		Multiplier:  DefaultMultiplier,
		Jitter:      true,
		RetryOnFunc: DefaultRetryOnFunc,
	}
}

// DefaultRetryOnFunc determines if an error should trigger a retry
func DefaultRetryOnFunc(err error) bool {
	if err == nil {
		return false
	}

	// Don't retry on context cancellation or deadline exceeded
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}

	// Retry on most other errors
	return true
}

// RetryFunc is a function that can be retried with exponential backoff
type RetryFunc func(ctx context.Context) error

// WithExponentialBackoff executes a function with exponential backoff retry logic
func WithExponentialBackoff(ctx context.Context, logger *zap.Logger, config BackoffConfig, fn RetryFunc) error {
	if logger == nil {
		logger = zap.NewNop()
	}

	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Execute the function
		err := fn(ctx)
		if err == nil {
			if attempt > 0 {
				logger.Info("Operation succeeded after retry",
					zap.Int("attempt", attempt+1),
					zap.Int("total_attempts", config.MaxRetries+1))
			}
			return nil
		}

		lastErr = err

		// Check if we should retry this error
		if !config.RetryOnFunc(err) {
			logger.Debug("Error is not retryable, stopping attempts",
				zap.Error(err),
				zap.Int("attempt", attempt+1))
			return err
		}

		// Don't sleep on the last attempt
		if attempt == config.MaxRetries {
			break
		}

		// Calculate delay with exponential backoff
		delay := time.Duration(float64(config.BaseDelay) * math.Pow(config.Multiplier, float64(attempt)))
		if delay > config.MaxDelay {
			delay = config.MaxDelay
		}

		// Add jitter to prevent thundering herd
		if config.Jitter {
			jitter := time.Duration(float64(delay) * 0.1 * (2*float64(time.Now().UnixNano()%JitterModulus)/JitterModulus - 1))
			delay += jitter
		}

		logger.Debug("Retrying after delay",
			zap.Error(err),
			zap.Int("attempt", attempt+1),
			zap.Duration("delay", delay),
			zap.Int("max_retries", config.MaxRetries))

		// Wait before retry, but respect context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	logger.Error("All retry attempts exhausted",
		zap.Error(lastErr),
		zap.Int("total_attempts", config.MaxRetries+1))

	return fmt.Errorf("operation failed after %d attempts: %w", config.MaxRetries+1, lastErr)
}

// SimpleRetry is a convenience function for simple retry logic with default configuration
func SimpleRetry(ctx context.Context, logger *zap.Logger, fn RetryFunc) error {
	return WithExponentialBackoff(ctx, logger, DefaultBackoffConfig(), fn)
}

// RetryWithMaxAttempts is a convenience function for retry logic with custom max attempts
func RetryWithMaxAttempts(ctx context.Context, logger *zap.Logger, maxRetries int, fn RetryFunc) error {
	config := DefaultBackoffConfig()
	config.MaxRetries = maxRetries
	return WithExponentialBackoff(ctx, logger, config, fn)
}

// RetryWithBaseDelay is a convenience function for retry logic with custom base delay
func RetryWithBaseDelay(ctx context.Context, logger *zap.Logger, baseDelay time.Duration, fn RetryFunc) error {
	config := DefaultBackoffConfig()
	config.BaseDelay = baseDelay
	return WithExponentialBackoff(ctx, logger, config, fn)
}
