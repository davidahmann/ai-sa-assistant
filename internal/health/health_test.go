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

package health

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestManager_Check(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager("test-service", "1.0.0", logger)

	// Add healthy checker
	manager.AddCheckerFunc("healthy", func(ctx context.Context) CheckResult {
		return CheckResult{
			Status:    StatusHealthy,
			Timestamp: time.Now(),
		}
	})

	// Add unhealthy checker
	manager.AddCheckerFunc("unhealthy", func(ctx context.Context) CheckResult {
		return CheckResult{
			Status:    StatusUnhealthy,
			Error:     "service is down",
			Timestamp: time.Now(),
		}
	})

	ctx := context.Background()
	result := manager.Check(ctx)

	// Overall status should be unhealthy due to one unhealthy dependency
	if result.Status != StatusUnhealthy {
		t.Errorf("Expected status to be unhealthy, got %s", result.Status)
	}

	if result.Service != "test-service" {
		t.Errorf("Expected service to be test-service, got %s", result.Service)
	}

	if result.Version != "1.0.0" {
		t.Errorf("Expected version to be 1.0.0, got %s", result.Version)
	}

	if len(result.Dependencies) != 2 {
		t.Errorf("Expected 2 dependencies, got %d", len(result.Dependencies))
	}

	healthyResult := result.Dependencies["healthy"]
	if healthyResult.Status != StatusHealthy {
		t.Errorf("Expected healthy dependency to be healthy, got %s", healthyResult.Status)
	}

	unhealthyResult := result.Dependencies["unhealthy"]
	if unhealthyResult.Status != StatusUnhealthy {
		t.Errorf("Expected unhealthy dependency to be unhealthy, got %s", unhealthyResult.Status)
	}

	if unhealthyResult.Error != "service is down" {
		t.Errorf("Expected error message, got %s", unhealthyResult.Error)
	}
}

func TestManager_Check_AllHealthy(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager("test-service", "1.0.0", logger)

	// Add multiple healthy checkers
	manager.AddCheckerFunc("service1", func(ctx context.Context) CheckResult {
		return CheckResult{
			Status:    StatusHealthy,
			Timestamp: time.Now(),
		}
	})

	manager.AddCheckerFunc("service2", func(ctx context.Context) CheckResult {
		return CheckResult{
			Status:    StatusHealthy,
			Timestamp: time.Now(),
		}
	})

	ctx := context.Background()
	result := manager.Check(ctx)

	// Overall status should be healthy
	if result.Status != StatusHealthy {
		t.Errorf("Expected status to be healthy, got %s", result.Status)
	}

	if len(result.Dependencies) != 2 {
		t.Errorf("Expected 2 dependencies, got %d", len(result.Dependencies))
	}
}

func TestManager_Check_DegradedStatus(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager("test-service", "1.0.0", logger)

	// Add healthy checker
	manager.AddCheckerFunc("healthy", func(ctx context.Context) CheckResult {
		return CheckResult{
			Status:    StatusHealthy,
			Timestamp: time.Now(),
		}
	})

	// Add degraded checker
	manager.AddCheckerFunc("degraded", func(ctx context.Context) CheckResult {
		return CheckResult{
			Status:    StatusDegraded,
			Error:     "service is slow",
			Timestamp: time.Now(),
		}
	})

	ctx := context.Background()
	result := manager.Check(ctx)

	// Overall status should be degraded
	if result.Status != StatusDegraded {
		t.Errorf("Expected status to be degraded, got %s", result.Status)
	}
}

func TestManager_Check_Timeout(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager("test-service", "1.0.0", logger)
	manager.SetTimeout(100 * time.Millisecond)

	// Add slow checker that takes longer than timeout
	manager.AddCheckerFunc("slow", func(ctx context.Context) CheckResult {
		select {
		case <-time.After(200 * time.Millisecond):
			return CheckResult{
				Status:    StatusHealthy,
				Timestamp: time.Now(),
			}
		case <-ctx.Done():
			return CheckResult{
				Status:    StatusUnhealthy,
				Error:     "timeout",
				Timestamp: time.Now(),
			}
		}
	})

	ctx := context.Background()
	result := manager.Check(ctx)

	// Should handle timeout gracefully
	if result.Status != StatusUnhealthy {
		t.Errorf("Expected status to be unhealthy due to timeout, got %s", result.Status)
	}
}

func TestHTTPHealthChecker(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, `{"status": "healthy"}`)
	}))
	defer server.Close()

	checker := HTTPHealthChecker(server.URL, nil)
	result := checker.Check(context.Background())

	if result.Status != StatusHealthy {
		t.Errorf("Expected status to be healthy, got %s", result.Status)
	}

	if result.Metadata["url"] != server.URL {
		t.Errorf("Expected URL metadata to be %s, got %v", server.URL, result.Metadata["url"])
	}

	if result.Metadata["status_code"] != http.StatusOK {
		t.Errorf("Expected status code to be %d, got %v", http.StatusOK, result.Metadata["status_code"])
	}
}

func TestHTTPHealthChecker_Unhealthy(t *testing.T) {
	// Create test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintln(w, `{"error": "service unavailable"}`)
	}))
	defer server.Close()

	checker := HTTPHealthChecker(server.URL, nil)
	result := checker.Check(context.Background())

	if result.Status != StatusUnhealthy {
		t.Errorf("Expected status to be unhealthy, got %s", result.Status)
	}

	if result.Metadata["status_code"] != http.StatusInternalServerError {
		t.Errorf("Expected status code to be %d, got %v", http.StatusInternalServerError, result.Metadata["status_code"])
	}
}

func TestDatabaseHealthChecker(t *testing.T) {
	// Test successful ping
	checker := DatabaseHealthChecker("test-db", func(ctx context.Context) error {
		return nil
	})

	result := checker.Check(context.Background())

	if result.Status != StatusHealthy {
		t.Errorf("Expected status to be healthy, got %s", result.Status)
	}

	if result.Metadata["database"] != "test-db" {
		t.Errorf("Expected database metadata to be test-db, got %v", result.Metadata["database"])
	}
}

func TestDatabaseHealthChecker_Unhealthy(t *testing.T) {
	// Test failed ping
	checker := DatabaseHealthChecker("test-db", func(ctx context.Context) error {
		return errors.New("connection failed")
	})

	result := checker.Check(context.Background())

	if result.Status != StatusUnhealthy {
		t.Errorf("Expected status to be unhealthy, got %s", result.Status)
	}

	if result.Error == "" {
		t.Error("Expected error message to be set")
	}
}

func TestExternalServiceHealthChecker(t *testing.T) {
	// Test successful check
	checker := ExternalServiceHealthChecker("test-service", func(ctx context.Context) error {
		return nil
	})

	result := checker.Check(context.Background())

	if result.Status != StatusHealthy {
		t.Errorf("Expected status to be healthy, got %s", result.Status)
	}

	if result.Metadata["service"] != "test-service" {
		t.Errorf("Expected service metadata to be test-service, got %v", result.Metadata["service"])
	}
}

func TestExternalServiceHealthChecker_Degraded(t *testing.T) {
	// Test temporary error (should be degraded)
	checker := ExternalServiceHealthChecker("test-service", func(ctx context.Context) error {
		return errors.New("timeout occurred")
	})

	result := checker.Check(context.Background())

	if result.Status != StatusDegraded {
		t.Errorf("Expected status to be degraded, got %s", result.Status)
	}

	if result.Error == "" {
		t.Error("Expected error message to be set")
	}
}

func TestExternalServiceHealthChecker_Unhealthy(t *testing.T) {
	// Test non-temporary error (should be unhealthy)
	checker := ExternalServiceHealthChecker("test-service", func(ctx context.Context) error {
		return errors.New("service permanently unavailable")
	})

	result := checker.Check(context.Background())

	if result.Status != StatusUnhealthy {
		t.Errorf("Expected status to be unhealthy, got %s", result.Status)
	}
}

func TestIsTemporaryError(t *testing.T) {
	temporaryErrors := []error{
		errors.New("timeout occurred"),
		errors.New("connection refused"),
		errors.New("temporary failure in name resolution"),
		errors.New("network is unreachable"),
		errors.New("context deadline exceeded"),
	}

	for _, err := range temporaryErrors {
		if !isTemporaryError(err) {
			t.Errorf("Expected %v to be temporary error", err)
		}
	}

	nonTemporaryErrors := []error{
		errors.New("service unavailable"),
		errors.New("authentication failed"),
		errors.New("permission denied"),
	}

	for _, err := range nonTemporaryErrors {
		if isTemporaryError(err) {
			t.Errorf("Expected %v to not be temporary error", err)
		}
	}

	// Test nil error
	if isTemporaryError(nil) {
		t.Error("Expected nil error to not be temporary")
	}
}

func TestManager_HTTPHandler(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager("test-service", "1.0.0", logger)

	// Add healthy checker
	manager.AddCheckerFunc("healthy", func(ctx context.Context) CheckResult {
		return CheckResult{
			Status:    StatusHealthy,
			Timestamp: time.Now(),
		}
	})

	handler := manager.HTTPHandler()

	// Test GET request
	req, err := http.NewRequest("GET", "/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, status)
	}

	if contentType := rr.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Expected content type application/json, got %s", contentType)
	}
}

func TestManager_HTTPHandler_MethodNotAllowed(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager("test-service", "1.0.0", logger)

	handler := manager.HTTPHandler()

	// Test POST request (should be method not allowed)
	req, err := http.NewRequest("POST", "/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusMethodNotAllowed {
		t.Errorf("Expected status code %d, got %d", http.StatusMethodNotAllowed, status)
	}
}

func TestManager_HTTPHandler_ServiceUnavailable(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager("test-service", "1.0.0", logger)

	// Add unhealthy checker
	manager.AddCheckerFunc("unhealthy", func(ctx context.Context) CheckResult {
		return CheckResult{
			Status:    StatusUnhealthy,
			Error:     "service is down",
			Timestamp: time.Now(),
		}
	})

	handler := manager.HTTPHandler()

	req, err := http.NewRequest("GET", "/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %d, got %d", http.StatusServiceUnavailable, status)
	}
}
