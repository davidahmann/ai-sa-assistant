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
	"time"

	"go.uber.org/zap"
)

// TimeoutConfig holds configuration for timeout operations
type TimeoutConfig struct {
	DefaultTimeout time.Duration
	MaxTimeout     time.Duration
	Logger         *zap.Logger
}

const (
	// DefaultTimeoutSeconds is the default timeout in seconds
	DefaultTimeoutSeconds = 30
)

// DefaultTimeoutConfig returns the default timeout configuration
// with 30s max timeout as specified in requirements
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		DefaultTimeout: DefaultTimeoutSeconds * time.Second,
		MaxTimeout:     DefaultTimeoutSeconds * time.Second,
		Logger:         zap.NewNop(),
	}
}

// TimeoutFunc is a function that can be executed with a timeout
type TimeoutFunc func(ctx context.Context) error

// WithTimeout executes a function with a timeout
func WithTimeout(ctx context.Context, timeout time.Duration, logger *zap.Logger, fn TimeoutFunc) error {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Create a context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Channel to receive the result
	done := make(chan error, 1)

	// Execute the function in a goroutine
	go func() {
		done <- fn(timeoutCtx)
	}()

	// Wait for either completion or timeout
	select {
	case err := <-done:
		if err != nil {
			logger.Debug("Operation completed with error",
				zap.Error(err),
				zap.Duration("timeout", timeout))
		} else {
			logger.Debug("Operation completed successfully",
				zap.Duration("timeout", timeout))
		}
		return err
	case <-timeoutCtx.Done():
		logger.Warn("Operation timed out",
			zap.Duration("timeout", timeout),
			zap.Error(timeoutCtx.Err()))
		return NewTimeoutError("Operation timed out", timeoutCtx.Err())
	}
}

// WithDefaultTimeout executes a function with the default timeout (30s)
func WithDefaultTimeout(ctx context.Context, logger *zap.Logger, fn TimeoutFunc) error {
	return WithTimeout(ctx, DefaultTimeoutConfig().DefaultTimeout, logger, fn)
}

// WithCustomTimeout executes a function with a custom timeout, but caps it at maxTimeout
func WithCustomTimeout(ctx context.Context, timeout time.Duration, logger *zap.Logger, fn TimeoutFunc) error {
	config := DefaultTimeoutConfig()
	if timeout > config.MaxTimeout {
		timeout = config.MaxTimeout
		logger.Warn("Timeout capped at maximum",
			zap.Duration("requested_timeout", timeout),
			zap.Duration("max_timeout", config.MaxTimeout))
	}
	return WithTimeout(ctx, timeout, logger, fn)
}

// TimeoutManager manages timeout operations across the application
type TimeoutManager struct {
	config TimeoutConfig
	logger *zap.Logger
}

// NewTimeoutManager creates a new timeout manager with the given configuration
func NewTimeoutManager(config TimeoutConfig) *TimeoutManager {
	if config.Logger == nil {
		config.Logger = zap.NewNop()
	}
	return &TimeoutManager{
		config: config,
		logger: config.Logger,
	}
}

// Execute executes a function with the manager's default timeout
func (tm *TimeoutManager) Execute(ctx context.Context, fn TimeoutFunc) error {
	return WithTimeout(ctx, tm.config.DefaultTimeout, tm.logger, fn)
}

// ExecuteWithTimeout executes a function with a custom timeout
func (tm *TimeoutManager) ExecuteWithTimeout(ctx context.Context, timeout time.Duration, fn TimeoutFunc) error {
	return WithCustomTimeout(ctx, timeout, tm.logger, fn)
}

// GetDefaultTimeout returns the default timeout duration
func (tm *TimeoutManager) GetDefaultTimeout() time.Duration {
	return tm.config.DefaultTimeout
}

// GetMaxTimeout returns the maximum timeout duration
func (tm *TimeoutManager) GetMaxTimeout() time.Duration {
	return tm.config.MaxTimeout
}
