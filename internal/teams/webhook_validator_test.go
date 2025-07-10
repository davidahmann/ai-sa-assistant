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

package teams

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

const (
	testWebhookSecret = "test-secret-key-for-validation"
	validTestPayload  = `{"type":"message","text":"test query","from":{"id":"user123"}}`
)

func TestNewWebhookValidator(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name           string
		webhookSecret  string
		expectedValid  bool
		expectedLogger bool
	}{
		{
			name:           "with_secret",
			webhookSecret:  testWebhookSecret,
			expectedValid:  true,
			expectedLogger: true,
		},
		{
			name:           "without_secret",
			webhookSecret:  "",
			expectedValid:  false,
			expectedLogger: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewWebhookValidator(tt.webhookSecret, logger)

			if validator == nil {
				t.Fatal("Expected validator to be created, got nil")
			}

			if validator.enabled != tt.expectedValid {
				t.Errorf("Expected enabled=%v, got %v", tt.expectedValid, validator.enabled)
			}

			if (validator.logger != nil) != tt.expectedLogger {
				t.Errorf("Expected logger set=%v, got %v", tt.expectedLogger, validator.logger != nil)
			}

			if validator.webhookSecret != tt.webhookSecret {
				t.Errorf("Expected webhookSecret=%s, got %s", tt.webhookSecret, validator.webhookSecret)
			}
		})
	}
}

func TestValidateWebhook_DisabledValidation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	validator := NewWebhookValidator("", logger) // Empty secret disables validation

	req := createTestHTTPRequest(t, http.MethodPost, validTestPayload, nil)
	result := validator.ValidateWebhook(req, []byte(validTestPayload))

	if !result.Valid {
		t.Errorf("Expected validation to pass when disabled, got error: %s", result.ErrorMessage)
	}

	if result.SecurityLevel != "disabled" {
		t.Errorf("Expected security level 'disabled', got %s", result.SecurityLevel)
	}
}

func TestValidateWebhook_ContentTypeValidation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	validator := NewWebhookValidator(testWebhookSecret, logger)

	tests := []struct {
		name        string
		contentType string
		expectValid bool
	}{
		{
			name:        "valid_json",
			contentType: "application/json",
			expectValid: true,
		},
		{
			name:        "valid_json_with_charset",
			contentType: "application/json; charset=utf-8",
			expectValid: true,
		},
		{
			name:        "invalid_content_type",
			contentType: "text/plain",
			expectValid: false,
		},
		{
			name:        "missing_content_type",
			contentType: "",
			expectValid: false,
		},
		{
			name:        "xml_content_type",
			contentType: "application/xml",
			expectValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := map[string]string{}
			if tt.contentType != "" {
				headers["Content-Type"] = tt.contentType
			}

			req := createTestHTTPRequest(t, http.MethodPost, validTestPayload, headers)
			result := validator.ValidateWebhook(req, []byte(validTestPayload))

			if result.Valid != tt.expectValid {
				t.Errorf("Expected valid=%v, got %v with error: %s", tt.expectValid, result.Valid, result.ErrorMessage)
			}

			if !tt.expectValid && !strings.Contains(result.ErrorMessage, "content type") {
				t.Errorf("Expected error about content type, got: %s", result.ErrorMessage)
			}
		})
	}
}

func TestValidateWebhook_TimestampValidation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	validator := NewWebhookValidator(testWebhookSecret, logger)

	now := time.Now()

	tests := []struct {
		name        string
		timestamp   string
		expectValid bool
		description string
	}{
		{
			name:        "valid_recent_timestamp",
			timestamp:   now.Add(-1 * time.Minute).Format(time.RFC3339),
			expectValid: true,
			description: "Recent timestamp should be valid",
		},
		{
			name:        "no_timestamp_header",
			timestamp:   "",
			expectValid: true,
			description: "Missing timestamp should be allowed (optional)",
		},
		{
			name:        "too_old_timestamp",
			timestamp:   now.Add(-10 * time.Minute).Format(time.RFC3339),
			expectValid: false,
			description: "Timestamp older than max age should be rejected",
		},
		{
			name:        "future_timestamp",
			timestamp:   now.Add(2 * time.Minute).Format(time.RFC3339),
			expectValid: false,
			description: "Future timestamp should be rejected",
		},
		{
			name:        "invalid_timestamp_format",
			timestamp:   "not-a-timestamp",
			expectValid: false,
			description: "Invalid timestamp format should be rejected",
		},
		{
			name:        "valid_edge_case_timestamp",
			timestamp:   now.Add(-4 * time.Minute).Format(time.RFC3339),
			expectValid: true,
			description: "Timestamp just within max age should be valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := map[string]string{
				"Content-Type": "application/json",
			}
			if tt.timestamp != "" {
				headers[TeamsTimestampHeader] = tt.timestamp
			}

			req := createTestHTTPRequest(t, http.MethodPost, validTestPayload, headers)
			result := validator.ValidateWebhook(req, []byte(validTestPayload))

			if result.Valid != tt.expectValid {
				t.Errorf("%s: Expected valid=%v, got %v with error: %s", tt.description, tt.expectValid, result.Valid, result.ErrorMessage)
			}

			if !tt.expectValid && !strings.Contains(result.ErrorMessage, "timestamp") {
				t.Errorf("Expected error about timestamp, got: %s", result.ErrorMessage)
			}
		})
	}
}

func TestValidateWebhook_SignatureValidation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	validator := NewWebhookValidator(testWebhookSecret, logger)

	// Generate valid signature for test payload
	validSignature := generateValidSignature(t, testWebhookSecret, []byte(validTestPayload))

	tests := []struct {
		name        string
		signature   string
		payload     string
		expectValid bool
		description string
	}{
		{
			name:        "valid_signature",
			signature:   validSignature,
			payload:     validTestPayload,
			expectValid: true,
			description: "Valid HMAC signature should pass",
		},
		{
			name:        "valid_signature_with_prefix",
			signature:   "sha256=" + validSignature,
			payload:     validTestPayload,
			expectValid: true,
			description: "Valid signature with sha256= prefix should pass",
		},
		{
			name:        "no_signature_header",
			signature:   "",
			payload:     validTestPayload,
			expectValid: true,
			description: "Missing signature should be allowed (optional for Teams)",
		},
		{
			name:        "invalid_signature",
			signature:   "invalid-signature",
			payload:     validTestPayload,
			expectValid: false,
			description: "Invalid signature should be rejected",
		},
		{
			name:        "signature_for_different_payload",
			signature:   generateValidSignature(t, testWebhookSecret, []byte("different payload")),
			payload:     validTestPayload,
			expectValid: false,
			description: "Signature for different payload should be rejected",
		},
		{
			name:        "signature_with_different_secret",
			signature:   generateValidSignature(t, "different-secret", []byte(validTestPayload)),
			payload:     validTestPayload,
			expectValid: false,
			description: "Signature with different secret should be rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := map[string]string{
				"Content-Type": "application/json",
			}
			if tt.signature != "" {
				headers[TeamsSignatureHeader] = tt.signature
			}

			req := createTestHTTPRequest(t, http.MethodPost, tt.payload, headers)
			result := validator.ValidateWebhook(req, []byte(tt.payload))

			if result.Valid != tt.expectValid {
				t.Errorf("%s: Expected valid=%v, got %v with error: %s", tt.description, tt.expectValid, result.Valid, result.ErrorMessage)
			}

			if !tt.expectValid && tt.signature != "" && !strings.Contains(result.ErrorMessage, "signature") {
				t.Errorf("Expected error about signature, got: %s", result.ErrorMessage)
			}
		})
	}
}

func TestValidateWebhook_SecurityHeaderValidation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	validator := NewWebhookValidator(testWebhookSecret, logger)

	tests := []struct {
		name        string
		method      string
		userAgent   string
		expectValid bool
		description string
	}{
		{
			name:        "valid_post_method",
			method:      http.MethodPost,
			userAgent:   "Microsoft-Teams/1.0",
			expectValid: true,
			description: "POST method should be valid",
		},
		{
			name:        "invalid_get_method",
			method:      http.MethodGet,
			userAgent:   "Microsoft-Teams/1.0",
			expectValid: false,
			description: "GET method should be rejected",
		},
		{
			name:        "invalid_put_method",
			method:      http.MethodPut,
			userAgent:   "Microsoft-Teams/1.0",
			expectValid: false,
			description: "PUT method should be rejected",
		},
		{
			name:        "valid_teams_user_agent",
			method:      http.MethodPost,
			userAgent:   "Microsoft-Teams/1.0",
			expectValid: true,
			description: "Microsoft Teams User-Agent should be valid",
		},
		{
			name:        "valid_skype_user_agent",
			method:      http.MethodPost,
			userAgent:   "SkypeBot/1.0",
			expectValid: true,
			description: "SkypeBot User-Agent should be valid",
		},
		{
			name:        "suspicious_user_agent",
			method:      http.MethodPost,
			userAgent:   "curl/7.68.0",
			expectValid: true,
			description: "Suspicious User-Agent should be logged but not rejected",
		},
		{
			name:        "empty_user_agent",
			method:      http.MethodPost,
			userAgent:   "",
			expectValid: true,
			description: "Empty User-Agent should be allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := map[string]string{
				"Content-Type": "application/json",
			}
			if tt.userAgent != "" {
				headers["User-Agent"] = tt.userAgent
			}

			req := createTestHTTPRequest(t, tt.method, validTestPayload, headers)
			result := validator.ValidateWebhook(req, []byte(validTestPayload))

			if result.Valid != tt.expectValid {
				t.Errorf("%s: Expected valid=%v, got %v with error: %s", tt.description, tt.expectValid, result.Valid, result.ErrorMessage)
			}

			if !tt.expectValid && !strings.Contains(result.ErrorMessage, "method") {
				t.Errorf("Expected error about HTTP method, got: %s", result.ErrorMessage)
			}
		})
	}
}

func TestValidateWebhook_MalformedPayloadAttacks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	validator := NewWebhookValidator(testWebhookSecret, logger)

	tests := []struct {
		name        string
		payload     string
		expectValid bool
		description string
	}{
		{
			name:        "valid_json_payload",
			payload:     validTestPayload,
			expectValid: true,
			description: "Valid JSON should pass",
		},
		{
			name:        "empty_payload",
			payload:     "",
			expectValid: true,
			description: "Empty payload should pass webhook validation (will fail JSON parsing later)",
		},
		{
			name:        "invalid_json_payload",
			payload:     `{"invalid": json}`,
			expectValid: true,
			description: "Invalid JSON should pass webhook validation (will fail JSON parsing later)",
		},
		{
			name:        "extremely_large_payload",
			payload:     strings.Repeat("a", 1024*1024), // 1MB payload
			expectValid: true,
			description: "Large payload should pass webhook validation (size limits handled elsewhere)",
		},
		{
			name:        "null_bytes_in_payload",
			payload:     "{\x00\"type\":\x00\"message\"}",
			expectValid: true,
			description: "Null bytes should pass webhook validation",
		},
		{
			name:        "unicode_payload",
			payload:     `{"type":"message","text":"🚀 test ñiño"}`,
			expectValid: true,
			description: "Unicode characters should be valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := map[string]string{
				"Content-Type": "application/json",
			}

			req := createTestHTTPRequest(t, http.MethodPost, tt.payload, headers)
			result := validator.ValidateWebhook(req, []byte(tt.payload))

			if result.Valid != tt.expectValid {
				t.Errorf("%s: Expected valid=%v, got %v with error: %s", tt.description, tt.expectValid, result.Valid, result.ErrorMessage)
			}
		})
	}
}

func TestValidateWebhook_ComprehensiveScenarios(t *testing.T) {
	logger := zaptest.NewLogger(t)
	validator := NewWebhookValidator(testWebhookSecret, logger)

	// Generate valid signature for comprehensive test
	validSignature := generateValidSignature(t, testWebhookSecret, []byte(validTestPayload))

	tests := []struct {
		name        string
		headers     map[string]string
		method      string
		payload     string
		expectValid bool
		description string
	}{
		{
			name: "fully_valid_request",
			headers: map[string]string{
				"Content-Type":       "application/json",
				TeamsSignatureHeader: validSignature,
				TeamsTimestampHeader: time.Now().Add(-1 * time.Minute).Format(time.RFC3339),
				"User-Agent":         "Microsoft-Teams/1.0",
			},
			method:      http.MethodPost,
			payload:     validTestPayload,
			expectValid: true,
			description: "Fully compliant request should pass all validations",
		},
		{
			name: "minimal_valid_request",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			method:      http.MethodPost,
			payload:     validTestPayload,
			expectValid: true,
			description: "Minimal request (no signature/timestamp) should pass",
		},
		{
			name: "request_with_forwarded_headers",
			headers: map[string]string{
				"Content-Type":        "application/json",
				"X-Forwarded-For":     "192.168.1.1",
				"X-Real-IP":           "10.0.0.1",
				"X-Cluster-Client-IP": "172.16.0.1",
			},
			method:      http.MethodPost,
			payload:     validTestPayload,
			expectValid: true,
			description: "Request with forwarded IP headers should pass but be logged",
		},
		{
			name: "attack_with_wrong_method",
			headers: map[string]string{
				"Content-Type":       "application/json",
				TeamsSignatureHeader: validSignature,
			},
			method:      http.MethodGet,
			payload:     validTestPayload,
			expectValid: false,
			description: "Attack using wrong HTTP method should fail",
		},
		{
			name: "replay_attack_old_timestamp",
			headers: map[string]string{
				"Content-Type":       "application/json",
				TeamsSignatureHeader: validSignature,
				TeamsTimestampHeader: time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
				"User-Agent":         "Microsoft-Teams/1.0",
			},
			method:      http.MethodPost,
			payload:     validTestPayload,
			expectValid: false,
			description: "Replay attack with old timestamp should fail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := createTestHTTPRequest(t, tt.method, tt.payload, tt.headers)
			result := validator.ValidateWebhook(req, []byte(tt.payload))

			if result.Valid != tt.expectValid {
				t.Errorf("%s: Expected valid=%v, got %v with error: %s", tt.description, tt.expectValid, result.Valid, result.ErrorMessage)
			}

			// Check security level is set appropriately
			if result.Valid && result.SecurityLevel == "" {
				t.Errorf("Valid result should have security level set")
			}
		})
	}
}

func TestComputeSignature(t *testing.T) {
	logger := zaptest.NewLogger(t)
	validator := NewWebhookValidator(testWebhookSecret, logger)

	tests := []struct {
		name        string
		payload     []byte
		expectError bool
		description string
	}{
		{
			name:        "valid_payload",
			payload:     []byte(validTestPayload),
			expectError: false,
			description: "Valid payload should generate signature",
		},
		{
			name:        "empty_payload",
			payload:     []byte(""),
			expectError: false,
			description: "Empty payload should generate signature",
		},
		{
			name:        "large_payload",
			payload:     []byte(strings.Repeat("test", 1000)),
			expectError: false,
			description: "Large payload should generate signature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signature, err := validator.computeSignature(tt.payload)

			if (err != nil) != tt.expectError {
				t.Errorf("%s: Expected error=%v, got error=%v", tt.description, tt.expectError, err != nil)
			}

			if !tt.expectError {
				if signature == "" {
					t.Errorf("Expected non-empty signature")
				}

				// Verify signature is valid hex
				if _, err := hex.DecodeString(signature); err != nil {
					t.Errorf("Expected valid hex signature, got error: %v", err)
				}

				// Verify signature is deterministic
				signature2, err := validator.computeSignature(tt.payload)
				if err != nil {
					t.Errorf("Second signature generation failed: %v", err)
				}
				if signature != signature2 {
					t.Errorf("Signatures should be deterministic")
				}
			}
		})
	}
}

func TestCompareSignatures(t *testing.T) {
	logger := zaptest.NewLogger(t)
	validator := NewWebhookValidator(testWebhookSecret, logger)

	validSig := generateValidSignature(t, testWebhookSecret, []byte(validTestPayload))

	tests := []struct {
		name        string
		provided    string
		expected    string
		shouldMatch bool
		description string
	}{
		{
			name:        "identical_signatures",
			provided:    validSig,
			expected:    validSig,
			shouldMatch: true,
			description: "Identical signatures should match",
		},
		{
			name:        "different_signatures",
			provided:    "different",
			expected:    validSig,
			shouldMatch: false,
			description: "Different signatures should not match",
		},
		{
			name:        "invalid_hex_provided",
			provided:    "not-hex",
			expected:    validSig,
			shouldMatch: false,
			description: "Invalid hex in provided signature should not match",
		},
		{
			name:        "invalid_hex_expected",
			provided:    validSig,
			expected:    "not-hex",
			shouldMatch: false,
			description: "Invalid hex in expected signature should not match",
		},
		{
			name:        "empty_signatures",
			provided:    "",
			expected:    "",
			shouldMatch: true,
			description: "Empty signatures should match",
		},
		{
			name:        "case_sensitivity",
			provided:    strings.ToUpper(validSig),
			expected:    strings.ToLower(validSig),
			shouldMatch: true,
			description: "Case should not matter for hex signatures",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.compareSignatures(tt.provided, tt.expected)
			if result != tt.shouldMatch {
				t.Errorf("%s: Expected match=%v, got %v", tt.description, tt.shouldMatch, result)
			}
		})
	}
}

func TestValidateBase64Secret(t *testing.T) {
	tests := []struct {
		name          string
		encodedSecret string
		expectedError bool
		description   string
	}{
		{
			name:          "empty_secret",
			encodedSecret: "",
			expectedError: false,
			description:   "Empty secret should be allowed",
		},
		{
			name:          "valid_base64",
			encodedSecret: "dGVzdC1zZWNyZXQ=", // "test-secret" in base64
			expectedError: false,
			description:   "Valid base64 should be decoded",
		},
		{
			name:          "plain_text_secret",
			encodedSecret: "plain-text-secret",
			expectedError: false,
			description:   "Plain text should be used as-is",
		},
		{
			name:          "invalid_base64_chars",
			encodedSecret: "invalid@base64!",
			expectedError: false,
			description:   "Invalid base64 should be treated as plain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateBase64Secret(tt.encodedSecret)

			if (err != nil) != tt.expectedError {
				t.Errorf("%s: Expected error=%v, got error=%v", tt.description, tt.expectedError, err != nil)
			}

			if !tt.expectedError && tt.encodedSecret != "" && result == "" {
				t.Errorf("Expected non-empty result for non-empty input")
			}
		})
	}
}

func TestWebhookValidator_Methods(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Test enabled validator
	enabledValidator := NewWebhookValidator(testWebhookSecret, logger)
	if !enabledValidator.IsValidationEnabled() {
		t.Error("Expected validation to be enabled with secret")
	}

	// Test disabled validator
	disabledValidator := NewWebhookValidator("", logger)
	if disabledValidator.IsValidationEnabled() {
		t.Error("Expected validation to be disabled without secret")
	}

	// Test validation middleware
	middleware := enabledValidator.GetValidationMiddleware()
	if middleware == nil {
		t.Error("Expected middleware function to be returned")
	}

	// Test middleware with valid request
	req := createTestHTTPRequest(t, http.MethodPost, validTestPayload, map[string]string{
		"Content-Type": "application/json",
	})
	err := middleware(req, []byte(validTestPayload))
	if err != nil {
		t.Errorf("Expected middleware to pass for valid request, got error: %v", err)
	}
}

// Helper functions

func createTestHTTPRequest(t *testing.T, method, payload string, headers map[string]string) *http.Request {
	t.Helper()

	req, err := http.NewRequest(method, "/test", bytes.NewReader([]byte(payload)))
	if err != nil {
		t.Fatalf("Failed to create test request: %v", err)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	return req
}

func generateValidSignature(t *testing.T, secret string, payload []byte) string {
	t.Helper()

	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write(payload); err != nil {
		t.Fatalf("Failed to generate test signature: %v", err)
	}
	return hex.EncodeToString(mac.Sum(nil))
}
