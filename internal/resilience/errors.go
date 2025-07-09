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
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// ErrorResponse represents the standard error response format across all APIs
type ErrorResponse struct {
	Error     string    `json:"error"`
	Code      string    `json:"code,omitempty"`
	RequestID string    `json:"request_id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// ErrorCode represents standard error codes used across the system
type ErrorCode string

const (
	// Client errors (4xx)
	ErrorCodeBadRequest      ErrorCode = "BAD_REQUEST"
	ErrorCodeUnauthorized    ErrorCode = "UNAUTHORIZED"
	ErrorCodeNotFound        ErrorCode = "NOT_FOUND"
	ErrorCodeTooManyRequests ErrorCode = "TOO_MANY_REQUESTS"

	// Server errors (5xx)
	ErrorCodeInternalError      ErrorCode = "INTERNAL_ERROR"
	ErrorCodeServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	ErrorCodeTimeout            ErrorCode = "TIMEOUT"
	ErrorCodeDependencyFailure  ErrorCode = "DEPENDENCY_FAILURE"
)

// ServiceError represents an error with additional context for proper handling
type ServiceError struct {
	Message    string
	Code       ErrorCode
	StatusCode int
	Internal   error
	Context    map[string]interface{}
}

// Error implements the error interface
func (e *ServiceError) Error() string {
	return e.Message
}

// Unwrap returns the underlying error
func (e *ServiceError) Unwrap() error {
	return e.Internal
}

// ToErrorResponse converts a ServiceError to an ErrorResponse
func (e *ServiceError) ToErrorResponse(requestID string) ErrorResponse {
	return ErrorResponse{
		Error:     e.Message,
		Code:      string(e.Code),
		RequestID: requestID,
		Timestamp: time.Now(),
	}
}

// NewServiceError creates a new ServiceError with the given parameters
func NewServiceError(message string, code ErrorCode, statusCode int, internal error) *ServiceError {
	return &ServiceError{
		Message:    message,
		Code:       code,
		StatusCode: statusCode,
		Internal:   internal,
		Context:    make(map[string]interface{}),
	}
}

// NewBadRequestError creates a new bad request error
func NewBadRequestError(message string, internal error) *ServiceError {
	return NewServiceError(message, ErrorCodeBadRequest, http.StatusBadRequest, internal)
}

// NewNotFoundError creates a new not found error
func NewNotFoundError(message string, internal error) *ServiceError {
	return NewServiceError(message, ErrorCodeNotFound, http.StatusNotFound, internal)
}

// NewInternalError creates a new internal server error
func NewInternalError(message string, internal error) *ServiceError {
	return NewServiceError(message, ErrorCodeInternalError, http.StatusInternalServerError, internal)
}

// NewServiceUnavailableError creates a new service unavailable error
func NewServiceUnavailableError(message string, internal error) *ServiceError {
	return NewServiceError(message, ErrorCodeServiceUnavailable, http.StatusServiceUnavailable, internal)
}

// NewTimeoutError creates a new timeout error
func NewTimeoutError(message string, internal error) *ServiceError {
	return NewServiceError(message, ErrorCodeTimeout, http.StatusRequestTimeout, internal)
}

// NewDependencyFailureError creates a new dependency failure error
func NewDependencyFailureError(message string, internal error) *ServiceError {
	return NewServiceError(message, ErrorCodeDependencyFailure, http.StatusBadGateway, internal)
}

// NewUnauthorizedError creates a new unauthorized error
func NewUnauthorizedError(message string, internal error) *ServiceError {
	return NewServiceError(message, ErrorCodeUnauthorized, http.StatusUnauthorized, internal)
}

// NewTooManyRequestsError creates a new too many requests error
func NewTooManyRequestsError(message string, internal error) *ServiceError {
	return NewServiceError(message, ErrorCodeTooManyRequests, http.StatusTooManyRequests, internal)
}

// ErrorHandler provides utilities for handling and formatting errors
type ErrorHandler struct {
	logger *zap.Logger
}

// NewErrorHandler creates a new error handler with the given logger
func NewErrorHandler(logger *zap.Logger) *ErrorHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ErrorHandler{logger: logger}
}

// WrapError wraps an error with user-friendly message and proper error code
func (eh *ErrorHandler) WrapError(err error, operation string) *ServiceError {
	if err == nil {
		return nil
	}

	if eh == nil {
		return NewInternalError(fmt.Sprintf("An error occurred while %s", operation), err)
	}

	// Check if it's already a ServiceError
	var serviceErr *ServiceError
	if AsServiceError(err, &serviceErr) {
		return serviceErr
	}

	// Determine appropriate error type and user-friendly message
	userMessage := eh.getUserFriendlyMessage(err, operation)

	// Determine error code and status code based on error type
	code, statusCode := eh.categorizeError(err)

	// Log the internal error for debugging
	eh.logger.Error("Error occurred during operation",
		zap.String("operation", operation),
		zap.Error(err),
		zap.String("user_message", userMessage),
		zap.String("error_code", string(code)))

	return NewServiceError(userMessage, code, statusCode, err)
}

// AsServiceError checks if an error is a ServiceError
func AsServiceError(err error, target **ServiceError) bool {
	if err == nil {
		return false
	}

	if serviceErr, ok := err.(*ServiceError); ok {
		*target = serviceErr
		return true
	}

	return false
}

// getUserFriendlyMessage converts technical errors to user-friendly messages
func (eh *ErrorHandler) getUserFriendlyMessage(err error, operation string) string {
	if err == nil {
		return ""
	}
	errStr := strings.ToLower(err.Error())

	switch {
	case strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded"):
		return "The operation is taking longer than expected. Please try again."
	case strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "connection reset"):
		return "Unable to connect to the service. Please try again later."
	case strings.Contains(errStr, "circuit breaker") || strings.Contains(errStr, "circuit breaker is open"):
		return "The service is temporarily unavailable. Please try again in a few minutes."
	case strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "too many requests"):
		return "Too many requests. Please wait a moment and try again."
	case strings.Contains(errStr, "unauthorized") || strings.Contains(errStr, "authentication"):
		return "Authentication failed. Please check your credentials."
	case strings.Contains(errStr, "not found") || strings.Contains(errStr, "does not exist"):
		return "The requested resource was not found."
	case strings.Contains(errStr, "bad request") || strings.Contains(errStr, "invalid"):
		return "The request is invalid. Please check your input and try again."
	case strings.Contains(errStr, "service unavailable") || strings.Contains(errStr, "unavailable"):
		return "The service is temporarily unavailable. Please try again later."
	default:
		return fmt.Sprintf("An error occurred while %s. Please try again.", operation)
	}
}

// categorizeError determines the appropriate error code and HTTP status code
func (eh *ErrorHandler) categorizeError(err error) (ErrorCode, int) {
	errStr := strings.ToLower(err.Error())

	switch {
	case strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded"):
		return ErrorCodeTimeout, http.StatusRequestTimeout
	case strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "connection reset"):
		return ErrorCodeDependencyFailure, http.StatusBadGateway
	case strings.Contains(errStr, "circuit breaker"):
		return ErrorCodeServiceUnavailable, http.StatusServiceUnavailable
	case strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "too many requests"):
		return ErrorCodeTooManyRequests, http.StatusTooManyRequests
	case strings.Contains(errStr, "unauthorized") || strings.Contains(errStr, "authentication"):
		return ErrorCodeUnauthorized, http.StatusUnauthorized
	case strings.Contains(errStr, "not found"):
		return ErrorCodeNotFound, http.StatusNotFound
	case strings.Contains(errStr, "bad request") || strings.Contains(errStr, "invalid"):
		return ErrorCodeBadRequest, http.StatusBadRequest
	case strings.Contains(errStr, "service unavailable"):
		return ErrorCodeServiceUnavailable, http.StatusServiceUnavailable
	default:
		return ErrorCodeInternalError, http.StatusInternalServerError
	}
}

// WriteErrorResponse writes an error response to an HTTP response writer
func (eh *ErrorHandler) WriteErrorResponse(w http.ResponseWriter, err error, requestID string) {
	var serviceErr *ServiceError
	if !AsServiceError(err, &serviceErr) {
		// Wrap non-ServiceError errors
		if eh == nil {
			serviceErr = NewInternalError("An error occurred while processing request", err)
		} else {
			serviceErr = eh.WrapError(err, "processing request")
		}
	}

	// Set appropriate headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(serviceErr.StatusCode)

	// Create and write error response
	response := serviceErr.ToErrorResponse(requestID)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		eh.logger.Error("Failed to encode error response", zap.Error(err))
	}
}

// LogError logs an error with appropriate context
func (eh *ErrorHandler) LogError(err error, operation string, fields ...zap.Field) {
	if err == nil {
		return
	}

	if eh == nil || eh.logger == nil {
		return
	}

	logFields := []zap.Field{
		zap.String("operation", operation),
		zap.Error(err),
	}
	logFields = append(logFields, fields...)

	var serviceErr *ServiceError
	if AsServiceError(err, &serviceErr) {
		logFields = append(logFields,
			zap.String("error_code", string(serviceErr.Code)),
			zap.Int("status_code", serviceErr.StatusCode))
	}

	eh.logger.Error("Operation failed", logFields...)
}
