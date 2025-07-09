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
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestServiceError(t *testing.T) {
	internal := errors.New("internal error")
	serviceErr := NewServiceError("user message", ErrorCodeInternalError, http.StatusInternalServerError, internal)

	if serviceErr.Error() != "user message" {
		t.Errorf("Expected 'user message', got %s", serviceErr.Error())
	}

	if serviceErr.Unwrap() != internal {
		t.Errorf("Expected unwrapped error to be internal error")
	}

	if serviceErr.Code != ErrorCodeInternalError {
		t.Errorf("Expected ErrorCodeInternalError, got %s", serviceErr.Code)
	}

	if serviceErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", serviceErr.StatusCode)
	}
}

func TestServiceErrorConvenience(t *testing.T) {
	internal := errors.New("internal")

	tests := []struct {
		name         string
		err          *ServiceError
		expectCode   ErrorCode
		expectStatus int
	}{
		{
			name:         "bad request",
			err:          NewBadRequestError("bad request", internal),
			expectCode:   ErrorCodeBadRequest,
			expectStatus: http.StatusBadRequest,
		},
		{
			name:         "not found",
			err:          NewNotFoundError("not found", internal),
			expectCode:   ErrorCodeNotFound,
			expectStatus: http.StatusNotFound,
		},
		{
			name:         "internal error",
			err:          NewInternalError("internal error", internal),
			expectCode:   ErrorCodeInternalError,
			expectStatus: http.StatusInternalServerError,
		},
		{
			name:         "service unavailable",
			err:          NewServiceUnavailableError("service unavailable", internal),
			expectCode:   ErrorCodeServiceUnavailable,
			expectStatus: http.StatusServiceUnavailable,
		},
		{
			name:         "timeout",
			err:          NewTimeoutError("timeout", internal),
			expectCode:   ErrorCodeTimeout,
			expectStatus: http.StatusRequestTimeout,
		},
		{
			name:         "dependency failure",
			err:          NewDependencyFailureError("dependency failure", internal),
			expectCode:   ErrorCodeDependencyFailure,
			expectStatus: http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Code != tt.expectCode {
				t.Errorf("Expected code %s, got %s", tt.expectCode, tt.err.Code)
			}
			if tt.err.StatusCode != tt.expectStatus {
				t.Errorf("Expected status %d, got %d", tt.expectStatus, tt.err.StatusCode)
			}
			if tt.err.Unwrap() != internal {
				t.Errorf("Expected unwrapped error to be internal error")
			}
		})
	}
}

func TestServiceErrorToErrorResponse(t *testing.T) {
	internal := errors.New("internal")
	serviceErr := NewServiceError("user message", ErrorCodeInternalError, http.StatusInternalServerError, internal)

	response := serviceErr.ToErrorResponse("request-123")

	if response.Error != "user message" {
		t.Errorf("Expected 'user message', got %s", response.Error)
	}

	if response.Code != string(ErrorCodeInternalError) {
		t.Errorf("Expected '%s', got %s", ErrorCodeInternalError, response.Code)
	}

	if response.RequestID != "request-123" {
		t.Errorf("Expected 'request-123', got %s", response.RequestID)
	}

	if response.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
	}
}

func TestErrorHandler_WrapError(t *testing.T) {
	logger := zap.NewNop()
	handler := NewErrorHandler(logger)

	tests := []struct {
		name            string
		inputError      error
		operation       string
		expectedCode    ErrorCode
		expectedStatus  int
		expectedMessage string
	}{
		{
			name:            "timeout error",
			inputError:      errors.New("timeout exceeded"),
			operation:       "processing request",
			expectedCode:    ErrorCodeTimeout,
			expectedStatus:  http.StatusRequestTimeout,
			expectedMessage: "The operation is taking longer than expected. Please try again.",
		},
		{
			name:            "connection error",
			inputError:      errors.New("connection refused"),
			operation:       "connecting to database",
			expectedCode:    ErrorCodeDependencyFailure,
			expectedStatus:  http.StatusBadGateway,
			expectedMessage: "Unable to connect to the service. Please try again later.",
		},
		{
			name:            "circuit breaker error",
			inputError:      errors.New("circuit breaker is open"),
			operation:       "calling service",
			expectedCode:    ErrorCodeServiceUnavailable,
			expectedStatus:  http.StatusServiceUnavailable,
			expectedMessage: "The service is temporarily unavailable. Please try again in a few minutes.",
		},
		{
			name:            "rate limit error",
			inputError:      errors.New("rate limit exceeded"),
			operation:       "making API call",
			expectedCode:    ErrorCodeTooManyRequests,
			expectedStatus:  http.StatusTooManyRequests,
			expectedMessage: "Too many requests. Please wait a moment and try again.",
		},
		{
			name:            "not found error",
			inputError:      errors.New("resource not found"),
			operation:       "finding resource",
			expectedCode:    ErrorCodeNotFound,
			expectedStatus:  http.StatusNotFound,
			expectedMessage: "The requested resource was not found.",
		},
		{
			name:            "generic error",
			inputError:      errors.New("some random error"),
			operation:       "processing data",
			expectedCode:    ErrorCodeInternalError,
			expectedStatus:  http.StatusInternalServerError,
			expectedMessage: "An error occurred while processing data. Please try again.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serviceErr := handler.WrapError(tt.inputError, tt.operation)

			if serviceErr == nil {
				t.Fatal("Expected ServiceError, got nil")
			}

			if serviceErr.Code != tt.expectedCode {
				t.Errorf("Expected code %s, got %s", tt.expectedCode, serviceErr.Code)
			}

			if serviceErr.StatusCode != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, serviceErr.StatusCode)
			}

			if serviceErr.Message != tt.expectedMessage {
				t.Errorf("Expected message %s, got %s", tt.expectedMessage, serviceErr.Message)
			}

			if serviceErr.Unwrap() != tt.inputError {
				t.Errorf("Expected unwrapped error to be original error")
			}
		})
	}
}

func TestErrorHandler_WrapError_Nil(t *testing.T) {
	logger := zap.NewNop()
	handler := NewErrorHandler(logger)

	result := handler.WrapError(nil, "test operation")
	if result != nil {
		t.Errorf("Expected nil for nil input, got %v", result)
	}
}

func TestErrorHandler_WrapError_AlreadyServiceError(t *testing.T) {
	logger := zap.NewNop()
	handler := NewErrorHandler(logger)

	original := NewBadRequestError("bad request", errors.New("internal"))
	result := handler.WrapError(original, "test operation")

	if result != original {
		t.Errorf("Expected same ServiceError to be returned")
	}
}

func TestAsServiceError(t *testing.T) {
	internal := errors.New("internal")
	serviceErr := NewServiceError("message", ErrorCodeInternalError, http.StatusInternalServerError, internal)

	var target *ServiceError

	// Test with ServiceError
	if !AsServiceError(serviceErr, &target) {
		t.Error("Expected AsServiceError to return true for ServiceError")
	}
	if target != serviceErr {
		t.Error("Expected target to be set to the ServiceError")
	}

	// Test with regular error
	target = nil
	if AsServiceError(errors.New("regular error"), &target) {
		t.Error("Expected AsServiceError to return false for regular error")
	}
	if target != nil {
		t.Error("Expected target to remain nil for regular error")
	}

	// Test with nil error
	target = nil
	if AsServiceError(nil, &target) {
		t.Error("Expected AsServiceError to return false for nil error")
	}
	if target != nil {
		t.Error("Expected target to remain nil for nil error")
	}
}

func TestErrorHandler_WriteErrorResponse(t *testing.T) {
	logger := zap.NewNop()
	handler := NewErrorHandler(logger)

	tests := []struct {
		name           string
		inputError     error
		expectedStatus int
		expectedInBody []string
	}{
		{
			name:           "service error",
			inputError:     NewBadRequestError("bad request", errors.New("internal")),
			expectedStatus: http.StatusBadRequest,
			expectedInBody: []string{"bad request", "BAD_REQUEST", "request-123"},
		},
		{
			name:           "regular error",
			inputError:     errors.New("timeout exceeded"),
			expectedStatus: http.StatusRequestTimeout,
			expectedInBody: []string{"The operation is taking longer than expected", "TIMEOUT", "request-123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			handler.WriteErrorResponse(w, tt.inputError, "request-123")

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if w.Header().Get("Content-Type") != "application/json" {
				t.Errorf("Expected Content-Type application/json, got %s", w.Header().Get("Content-Type"))
			}

			body := w.Body.String()
			for _, expected := range tt.expectedInBody {
				if !strings.Contains(body, expected) {
					t.Errorf("Expected body to contain %s, got %s", expected, body)
				}
			}
		})
	}
}

func TestErrorHandler_LogError(_ *testing.T) {
	// This test mainly verifies that LogError doesn't panic
	// In a real scenario, you'd want to use a test logger to verify log content
	logger := zap.NewNop()
	handler := NewErrorHandler(logger)

	// Test with ServiceError
	serviceErr := NewBadRequestError("bad request", errors.New("internal"))
	handler.LogError(serviceErr, "test operation")

	// Test with regular error
	handler.LogError(errors.New("regular error"), "test operation")

	// Test with nil error
	handler.LogError(nil, "test operation")
}

func TestErrorHandler_GetUserFriendlyMessage(t *testing.T) {
	logger := zap.NewNop()
	handler := NewErrorHandler(logger)

	tests := []struct {
		name       string
		inputError error
		operation  string
		expected   string
	}{
		{
			name:       "timeout error",
			inputError: errors.New("timeout exceeded"),
			operation:  "processing",
			expected:   "The operation is taking longer than expected. Please try again.",
		},
		{
			name:       "connection error",
			inputError: errors.New("connection refused"),
			operation:  "connecting",
			expected:   "Unable to connect to the service. Please try again later.",
		},
		{
			name:       "generic error",
			inputError: errors.New("some error"),
			operation:  "processing data",
			expected:   "An error occurred while processing data. Please try again.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.getUserFriendlyMessage(tt.inputError, tt.operation)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestErrorHandler_CategorizeError(t *testing.T) {
	logger := zap.NewNop()
	handler := NewErrorHandler(logger)

	tests := []struct {
		name           string
		inputError     error
		expectedCode   ErrorCode
		expectedStatus int
	}{
		{
			name:           "timeout",
			inputError:     errors.New("timeout exceeded"),
			expectedCode:   ErrorCodeTimeout,
			expectedStatus: http.StatusRequestTimeout,
		},
		{
			name:           "connection",
			inputError:     errors.New("connection refused"),
			expectedCode:   ErrorCodeDependencyFailure,
			expectedStatus: http.StatusBadGateway,
		},
		{
			name:           "circuit breaker",
			inputError:     errors.New("circuit breaker is open"),
			expectedCode:   ErrorCodeServiceUnavailable,
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name:           "generic",
			inputError:     errors.New("some error"),
			expectedCode:   ErrorCodeInternalError,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, status := handler.categorizeError(tt.inputError)
			if code != tt.expectedCode {
				t.Errorf("Expected code %s, got %s", tt.expectedCode, code)
			}
			if status != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, status)
			}
		})
	}
}
