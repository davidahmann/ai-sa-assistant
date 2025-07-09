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

// Package health provides health check functionality for microservices
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	// StatusHealthy represents healthy status
	StatusHealthy = "healthy"
	// StatusUnhealthy represents unhealthy status
	StatusUnhealthy = "unhealthy"
	// StatusDegraded represents degraded status
	StatusDegraded = "degraded"
	// DefaultTimeout is the default timeout for health checks
	DefaultTimeout = 5 * time.Second
)

// CheckResult represents the result of a health check
type CheckResult struct {
	Status    string                 `json:"status"`
	Latency   time.Duration          `json:"latency"`
	Error     string                 `json:"error,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// HealthResponse represents the complete health check response
type HealthResponse struct {
	Status       string                 `json:"status"`
	Service      string                 `json:"service"`
	Version      string                 `json:"version"`
	Environment  string                 `json:"environment"`
	Uptime       time.Duration          `json:"uptime"`
	Dependencies map[string]CheckResult `json:"dependencies"`
	Metadata     map[string]interface{} `json:"metadata"`
	Timestamp    time.Time              `json:"timestamp"`
}

// Checker interface for health checks
type Checker interface {
	Check(ctx context.Context) CheckResult
}

// CheckerFunc is a function adapter for the Checker interface
type CheckerFunc func(ctx context.Context) CheckResult

// Check implements the Checker interface
func (f CheckerFunc) Check(ctx context.Context) CheckResult {
	return f(ctx)
}

// Manager manages health checks for a service
type Manager struct {
	serviceName string
	version     string
	startTime   time.Time
	checkers    map[string]Checker
	timeout     time.Duration
	logger      *zap.Logger
}

// NewManager creates a new health check manager
func NewManager(serviceName, version string, logger *zap.Logger) *Manager {
	return &Manager{
		serviceName: serviceName,
		version:     version,
		startTime:   time.Now(),
		checkers:    make(map[string]Checker),
		timeout:     DefaultTimeout,
		logger:      logger,
	}
}

// SetTimeout sets the timeout for health checks
func (m *Manager) SetTimeout(timeout time.Duration) {
	m.timeout = timeout
}

// AddChecker adds a health checker
func (m *Manager) AddChecker(name string, checker Checker) {
	m.checkers[name] = checker
}

// AddCheckerFunc adds a health checker function
func (m *Manager) AddCheckerFunc(name string, checkFunc func(ctx context.Context) CheckResult) {
	m.checkers[name] = CheckerFunc(checkFunc)
}

// Check performs all health checks and returns the result
func (m *Manager) Check(ctx context.Context) HealthResponse {
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	dependencies := make(map[string]CheckResult)
	overallStatus := StatusHealthy

	// Run all checks
	for name, checker := range m.checkers {
		start := time.Now()
		result := checker.Check(ctx)
		result.Latency = time.Since(start)
		result.Timestamp = time.Now()

		dependencies[name] = result

		// Update overall status based on dependency status
		if result.Status == StatusUnhealthy {
			overallStatus = StatusUnhealthy
		} else if result.Status == StatusDegraded && overallStatus != StatusUnhealthy {
			overallStatus = StatusDegraded
		}
	}

	return HealthResponse{
		Status:       overallStatus,
		Service:      m.serviceName,
		Version:      m.version,
		Environment:  getEnvironment(),
		Uptime:       time.Since(m.startTime),
		Dependencies: dependencies,
		Metadata:     m.getSystemMetadata(),
		Timestamp:    time.Now(),
	}
}

// HTTPHandler returns a HTTP handler for health checks
func (m *Manager) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ctx := r.Context()
		result := m.Check(ctx)

		// Set HTTP status code based on health status
		statusCode := http.StatusOK
		if result.Status == StatusUnhealthy {
			statusCode = http.StatusServiceUnavailable
		} else if result.Status == StatusDegraded {
			statusCode = http.StatusOK // Keep 200 for degraded
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)

		// Write response
		if err := writeJSON(w, result); err != nil {
			m.logger.Error("Failed to write health check response", zap.Error(err))
		}
	}
}

// getSystemMetadata returns system metadata
func (m *Manager) getSystemMetadata() map[string]interface{} {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return map[string]interface{}{
		"go_version":   runtime.Version(),
		"goroutines":   runtime.NumGoroutine(),
		"memory_alloc": memStats.Alloc,
		"memory_sys":   memStats.Sys,
		"gc_runs":      memStats.NumGC,
		"hostname":     getHostname(),
		"process_id":   os.Getpid(),
	}
}

// getEnvironment returns the environment name
func getEnvironment() string {
	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = "unknown"
	}
	return env
}

// getHostname returns the hostname
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// writeJSON writes JSON response
func writeJSON(w http.ResponseWriter, v interface{}) error {
	return json.NewEncoder(w).Encode(v)
}

// HTTPHealthChecker creates a health checker for HTTP endpoints
func HTTPHealthChecker(url string, client *http.Client) Checker {
	if client == nil {
		client = &http.Client{Timeout: DefaultTimeout}
	}

	return CheckerFunc(func(ctx context.Context) CheckResult {
		start := time.Now()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return CheckResult{
				Status:    StatusUnhealthy,
				Error:     fmt.Sprintf("failed to create request: %v", err),
				Latency:   time.Since(start),
				Timestamp: time.Now(),
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			return CheckResult{
				Status:    StatusUnhealthy,
				Error:     fmt.Sprintf("request failed: %v", err),
				Latency:   time.Since(start),
				Timestamp: time.Now(),
			}
		}
		defer func() { _ = resp.Body.Close() }()

		status := StatusHealthy
		if resp.StatusCode >= 400 {
			status = StatusUnhealthy
		}

		return CheckResult{
			Status:    status,
			Latency:   time.Since(start),
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"url":         url,
				"status_code": resp.StatusCode,
			},
		}
	})
}

// DatabaseHealthChecker creates a health checker for database connections
func DatabaseHealthChecker(name string, pingFunc func(ctx context.Context) error) Checker {
	return CheckerFunc(func(ctx context.Context) CheckResult {
		start := time.Now()

		if err := pingFunc(ctx); err != nil {
			return CheckResult{
				Status:    StatusUnhealthy,
				Error:     fmt.Sprintf("database ping failed: %v", err),
				Latency:   time.Since(start),
				Timestamp: time.Now(),
			}
		}

		return CheckResult{
			Status:    StatusHealthy,
			Latency:   time.Since(start),
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"database": name,
			},
		}
	})
}

// ExternalServiceHealthChecker creates a health checker for external services
func ExternalServiceHealthChecker(name string, checkFunc func(ctx context.Context) error) Checker {
	return CheckerFunc(func(ctx context.Context) CheckResult {
		start := time.Now()

		if err := checkFunc(ctx); err != nil {
			// Check if it's a timeout or temporary error
			status := StatusUnhealthy
			if isTemporaryError(err) {
				status = StatusDegraded
			}

			return CheckResult{
				Status:    status,
				Error:     fmt.Sprintf("external service check failed: %v", err),
				Latency:   time.Since(start),
				Timestamp: time.Now(),
			}
		}

		return CheckResult{
			Status:    StatusHealthy,
			Latency:   time.Since(start),
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"service": name,
			},
		}
	})
}

// isTemporaryError checks if an error is temporary
func isTemporaryError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	temporaryPatterns := []string{
		"timeout",
		"connection refused",
		"temporary failure",
		"network is unreachable",
		"context deadline exceeded",
	}

	for _, pattern := range temporaryPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}
