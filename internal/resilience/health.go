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
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// HealthStatus represents the health status of a component
type HealthStatus string

const (
	// HealthStatusHealthy indicates the component is healthy
	HealthStatusHealthy HealthStatus = "healthy"
	// HealthStatusUnhealthy indicates the component is unhealthy
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	// HealthStatusDegraded indicates the component is degraded but functional
	HealthStatusDegraded HealthStatus = "degraded"
	// HealthStatusUnknown indicates the component status is unknown
	HealthStatusUnknown HealthStatus = "unknown"
)

// HealthCheckFunc is a function that performs a health check
type HealthCheckFunc func(ctx context.Context) HealthCheckResult

// HealthCheckResult represents the result of a health check
type HealthCheckResult struct {
	Status    HealthStatus  `json:"status"`
	Message   string        `json:"message,omitempty"`
	Details   interface{}   `json:"details,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
	Duration  time.Duration `json:"duration"`
}

// HealthCheck represents a single health check
type HealthCheck struct {
	Name     string
	Timeout  time.Duration
	Interval time.Duration
	Checker  HealthCheckFunc
}

// HealthReport represents the overall health report
type HealthReport struct {
	Status          HealthStatus                   `json:"status"`
	Timestamp       time.Time                      `json:"timestamp"`
	Version         string                         `json:"version,omitempty"`
	ServiceName     string                         `json:"service_name"`
	Uptime          time.Duration                  `json:"uptime"`
	Checks          map[string]HealthCheckResult   `json:"checks"`
	Dependencies    map[string]HealthCheckResult   `json:"dependencies"`
	Errors          []string                       `json:"errors,omitempty"`
	CircuitBreakers map[string]CircuitBreakerStats `json:"circuit_breakers,omitempty"`
}

// HealthMonitor monitors the health of various components
type HealthMonitor struct {
	serviceName       string
	version           string
	checks            map[string]HealthCheck
	dependencies      map[string]HealthCheck
	circuitBreakers   map[string]*CircuitBreaker
	results           map[string]HealthCheckResult
	dependencyResults map[string]HealthCheckResult
	startTime         time.Time
	mu                sync.RWMutex
	logger            *zap.Logger
}

// NewHealthMonitor creates a new health monitor
func NewHealthMonitor(serviceName, version string, logger *zap.Logger) *HealthMonitor {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &HealthMonitor{
		serviceName:       serviceName,
		version:           version,
		checks:            make(map[string]HealthCheck),
		dependencies:      make(map[string]HealthCheck),
		circuitBreakers:   make(map[string]*CircuitBreaker),
		results:           make(map[string]HealthCheckResult),
		dependencyResults: make(map[string]HealthCheckResult),
		startTime:         time.Now(),
		logger:            logger,
	}
}

// AddCheck adds a health check for a service component
func (hm *HealthMonitor) AddCheck(name string, timeout time.Duration, checker HealthCheckFunc) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.checks[name] = HealthCheck{
		Name:    name,
		Timeout: timeout,
		Checker: checker,
	}

	hm.logger.Info("Health check added",
		zap.String("name", name),
		zap.Duration("timeout", timeout))
}

// AddDependency adds a health check for an external dependency
func (hm *HealthMonitor) AddDependency(name string, timeout time.Duration, checker HealthCheckFunc) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.dependencies[name] = HealthCheck{
		Name:    name,
		Timeout: timeout,
		Checker: checker,
	}

	hm.logger.Info("Dependency health check added",
		zap.String("name", name),
		zap.Duration("timeout", timeout))
}

// AddCircuitBreaker adds a circuit breaker to monitor
func (hm *HealthMonitor) AddCircuitBreaker(name string, cb *CircuitBreaker) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.circuitBreakers[name] = cb
	hm.logger.Info("Circuit breaker added to health monitor", zap.String("name", name))
}

// runHealthCheck executes a single health check with timeout
func (hm *HealthMonitor) runHealthCheck(ctx context.Context, check HealthCheck) HealthCheckResult {
	start := time.Now()

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, check.Timeout)
	defer cancel()

	// Run the check
	result := check.Checker(timeoutCtx)
	result.Duration = time.Since(start)
	result.Timestamp = time.Now()

	// If the check didn't set a status, infer it from the result
	if result.Status == "" {
		if result.Message == "" {
			result.Status = HealthStatusHealthy
		} else {
			result.Status = HealthStatusUnhealthy
		}
	}

	return result
}

// GetHealthReport generates a comprehensive health report
func (hm *HealthMonitor) GetHealthReport(ctx context.Context) HealthReport {
	hm.mu.RLock()
	checks := make(map[string]HealthCheck)
	dependencies := make(map[string]HealthCheck)
	circuitBreakers := make(map[string]*CircuitBreaker)

	for k, v := range hm.checks {
		checks[k] = v
	}
	for k, v := range hm.dependencies {
		dependencies[k] = v
	}
	for k, v := range hm.circuitBreakers {
		circuitBreakers[k] = v
	}
	hm.mu.RUnlock()

	// Run all health checks
	checkResults := make(map[string]HealthCheckResult)
	dependencyResults := make(map[string]HealthCheckResult)
	var errors []string

	// Check service components
	for name, check := range checks {
		result := hm.runHealthCheck(ctx, check)
		checkResults[name] = result

		if result.Status == HealthStatusUnhealthy {
			errors = append(errors, fmt.Sprintf("%s: %s", name, result.Message))
		}
	}

	// Check dependencies
	for name, check := range dependencies {
		result := hm.runHealthCheck(ctx, check)
		dependencyResults[name] = result

		if result.Status == HealthStatusUnhealthy {
			errors = append(errors, fmt.Sprintf("dependency %s: %s", name, result.Message))
		}
	}

	// Get circuit breaker stats
	cbStats := make(map[string]CircuitBreakerStats)
	for name, cb := range circuitBreakers {
		cbStats[name] = cb.GetStats()
	}

	// Determine overall status
	overallStatus := hm.determineOverallStatus(checkResults, dependencyResults, cbStats)

	return HealthReport{
		Status:          overallStatus,
		Timestamp:       time.Now(),
		Version:         hm.version,
		ServiceName:     hm.serviceName,
		Uptime:          time.Since(hm.startTime),
		Checks:          checkResults,
		Dependencies:    dependencyResults,
		Errors:          errors,
		CircuitBreakers: cbStats,
	}
}

// determineOverallStatus determines the overall health status based on component results
func (hm *HealthMonitor) determineOverallStatus(
	checks map[string]HealthCheckResult,
	dependencies map[string]HealthCheckResult,
	circuitBreakers map[string]CircuitBreakerStats,
) HealthStatus {
	// If any critical component is unhealthy, overall status is unhealthy
	for _, result := range checks {
		if result.Status == HealthStatusUnhealthy {
			return HealthStatusUnhealthy
		}
	}

	// Check if any dependencies are unhealthy or circuit breakers are open
	degraded := false
	for _, result := range dependencies {
		if result.Status == HealthStatusUnhealthy {
			degraded = true
		}
	}

	for _, stats := range circuitBreakers {
		if stats.State == CircuitOpen {
			degraded = true
		}
	}

	if degraded {
		return HealthStatusDegraded
	}

	return HealthStatusHealthy
}

// CreateHealthCheckHandler creates an HTTP handler for health checks
func (hm *HealthMonitor) CreateHealthCheckHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Get health report
		report := hm.GetHealthReport(ctx)

		// Set appropriate status code
		statusCode := http.StatusOK
		switch report.Status {
		case HealthStatusUnhealthy:
			statusCode = http.StatusServiceUnavailable
		case HealthStatusDegraded:
			statusCode = http.StatusOK // Still serving requests
		}

		// Write response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)

		if err := json.NewEncoder(w).Encode(report); err != nil {
			hm.logger.Error("Failed to encode health report", zap.Error(err))
		}
	}
}

// CreateReadinessHandler creates an HTTP handler for readiness checks
func (hm *HealthMonitor) CreateReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Get health report
		report := hm.GetHealthReport(ctx)

		// Readiness check should fail if service is unhealthy
		if report.Status == HealthStatusUnhealthy {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, "Service not ready: %v", report.Errors)
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Ready")
	}
}

// CreateLivenessHandler creates an HTTP handler for liveness checks
func (hm *HealthMonitor) CreateLivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Liveness check should only fail if the service is completely broken
		// For now, we'll always return healthy if we can respond
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Alive")
	}
}

// Common health check functions

// HTTPHealthCheck creates a health check that makes an HTTP request
func HTTPHealthCheck(url string, timeout time.Duration) HealthCheckFunc {
	return func(ctx context.Context) HealthCheckResult {
		client := &http.Client{Timeout: timeout}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return HealthCheckResult{
				Status:  HealthStatusUnhealthy,
				Message: fmt.Sprintf("Failed to create request: %v", err),
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			return HealthCheckResult{
				Status:  HealthStatusUnhealthy,
				Message: fmt.Sprintf("Request failed: %v", err),
			}
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return HealthCheckResult{
				Status:  HealthStatusHealthy,
				Message: fmt.Sprintf("HTTP %d", resp.StatusCode),
			}
		}

		return HealthCheckResult{
			Status:  HealthStatusUnhealthy,
			Message: fmt.Sprintf("HTTP %d", resp.StatusCode),
		}
	}
}

// DatabaseHealthCheck creates a health check that pings a database
func DatabaseHealthCheck(pingFunc func(ctx context.Context) error) HealthCheckFunc {
	return func(ctx context.Context) HealthCheckResult {
		if err := pingFunc(ctx); err != nil {
			return HealthCheckResult{
				Status:  HealthStatusUnhealthy,
				Message: fmt.Sprintf("Database ping failed: %v", err),
			}
		}

		return HealthCheckResult{
			Status:  HealthStatusHealthy,
			Message: "Database connection healthy",
		}
	}
}
