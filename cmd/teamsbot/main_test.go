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

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/feedback"
	"go.uber.org/zap/zaptest"
)

func TestSanitizeFeedbackQuery(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal query",
			input:    "Generate AWS architecture plan",
			expected: "Generate AWS architecture plan",
		},
		{
			name:     "password in query",
			input:    "Connect to database with password mypassword123",
			expected: "Connect to database with [REDACTED]",
		},
		{
			name:     "api key in query",
			input:    "Use API key sk-abcdef123456 for authentication",
			expected: "Use API key sk-abcdef123456 for authentication",
		},
		{
			name:     "secret token", // pragma: allowlist secret
			input:    "Configure secret token abc123def456", // pragma: allowlist secret
			expected: "Configure [REDACTED] abc123def456",
		},
		{
			name:     "base64 encoded data",
			input:    "Use encoded data: dGVzdCBkYXRhIGZvciB0ZXN0aW5nIGFuZCBkZW1vbnN0cmF0aW9u",
			expected: "Use encoded data: [REDACTED]",
		},
		{
			name:     "hex string",
			input:    "Secret key: 1234567890abcdef1234567890abcdef12345678",
			expected: "[REDACTED] [REDACTED]",
		},
		{
			name:     "multiple sensitive items",
			input:    "password: secret123 and api_key: abc123def456",
			expected: "[REDACTED] and [REDACTED]",
		},
		{
			name: "very long query",
			input: "This is a very long query that exceeds the maximum length limit. " +
				"It should be truncated to prevent abuse and keep the log files manageable. " +
				"This text continues for a very long time to test the truncation functionality. " +
				"The query should be cut off at the maximum allowed length with ellipsis added. " +
				"This ensures that the logging system doesn't become overwhelmed with very long queries. " +
				"Additional text that should be truncated because it exceeds the maximum length limit.",
			expected: "This is a very long query that exceeds the maximum length limit. " +
				"It should be truncated to prevent abuse and keep the log files manageable. " +
				"This text continues for a very long time to test the truncation functionality. " +
				"The query should be cut off at the maximum allowed length with ellipsis added. " +
				"This ensures that the logging system doesn't become overwhelmed with very long queries. " +
				"Additional text that should be truncated because it exceeds the maximum length limit.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeFeedbackQuery(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeFeedbackQuery() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExtractUserIDFromRequest(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected string
	}{
		{
			name: "teams user id header",
			headers: map[string]string{
				"X-Teams-User-ID": "user123",
			},
			expected: "user123",
		},
		{
			name: "user id header",
			headers: map[string]string{
				"X-User-ID": "user456",
			},
			expected: "user456",
		},
		{
			name: "both headers present - teams takes precedence",
			headers: map[string]string{
				"X-Teams-User-ID": "teams-user",
				"X-User-ID":       "generic-user",
			},
			expected: "teams-user",
		},
		{
			name:     "no headers - fallback to IP",
			headers:  map[string]string{},
			expected: "ip:192.0.2.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test request
			req := httptest.NewRequest("POST", "/teams-feedback", nil)
			req.RemoteAddr = "192.0.2.1:12345"

			// Add headers
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			// Create gin context
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			result := extractUserIDFromRequest(c)
			if result != tt.expected {
				t.Errorf("extractUserIDFromRequest() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestHandleFeedback(t *testing.T) {
	// Create test logger
	logger := zaptest.NewLogger(t)

	// Create test feedback logger
	feedbackConfig := feedback.Config{
		StorageType: "file",
		FilePath:    "/tmp/test_feedback.log",
	}
	feedbackLogger, err := feedback.NewLogger(feedbackConfig, logger)
	if err != nil {
		t.Fatalf("Failed to create feedback logger: %v", err)
	}
	defer func() { _ = feedbackLogger.Close() }()

	// Create test config
	cfg := &config.Config{}

	tests := []struct {
		name           string
		requestBody    FeedbackRequest
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "valid positive feedback",
			requestBody: FeedbackRequest{
				Query:      "Generate AWS architecture plan",
				ResponseID: "resp_123456",
				Feedback:   "positive",
				Timestamp:  "2024-01-01T12:00:00Z",
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"message":"Feedback received"}`,
		},
		{
			name: "valid negative feedback",
			requestBody: FeedbackRequest{
				Query:      "Design Azure hybrid architecture",
				ResponseID: "resp_789012",
				Feedback:   "negative",
				Timestamp:  "2024-01-01T12:00:00Z",
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"message":"Feedback received"}`,
		},
		{
			name: "invalid feedback type",
			requestBody: FeedbackRequest{
				Query:      "Generate plan",
				ResponseID: "resp_123456",
				Feedback:   "invalid",
				Timestamp:  "2024-01-01T12:00:00Z",
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"Invalid feedback type"}`,
		},
		{
			name: "feedback with sensitive data",
			requestBody: FeedbackRequest{
				Query:      "Connect with password secret123",
				ResponseID: "resp_123456",
				Feedback:   "positive",
				Timestamp:  "2024-01-01T12:00:00Z",
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"message":"Feedback received"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request body
			jsonBody, err := json.Marshal(tt.requestBody)
			if err != nil {
				t.Fatalf("Failed to marshal request body: %v", err)
			}

			// Create test request
			req := httptest.NewRequest("POST", "/teams-feedback", bytes.NewBuffer(jsonBody))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = "192.0.2.1:12345"

			// Create gin context
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			// Call handler
			handleFeedback(c, cfg, feedbackLogger, logger)

			// Check response
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if w.Body.String() != tt.expectedBody {
				t.Errorf("Expected body %s, got %s", tt.expectedBody, w.Body.String())
			}
		})
	}
}

func TestHandleFeedback_InvalidJSON(t *testing.T) {
	// Create test logger
	logger := zaptest.NewLogger(t)

	// Create test feedback logger
	feedbackConfig := feedback.Config{
		StorageType: "file",
		FilePath:    "/tmp/test_feedback.log",
	}
	feedbackLogger, err := feedback.NewLogger(feedbackConfig, logger)
	if err != nil {
		t.Fatalf("Failed to create feedback logger: %v", err)
	}
	defer func() { _ = feedbackLogger.Close() }()

	// Create test config
	cfg := &config.Config{}

	// Create request with invalid JSON
	req := httptest.NewRequest("POST", "/teams-feedback", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")

	// Create gin context
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Call handler
	handleFeedback(c, cfg, feedbackLogger, logger)

	// Check response
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	expectedBody := `{"error":"Invalid feedback format"}`
	if w.Body.String() != expectedBody {
		t.Errorf("Expected body %s, got %s", expectedBody, w.Body.String())
	}
}
