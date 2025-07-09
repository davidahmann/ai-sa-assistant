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

// Package teams provides webhook validation functionality for Microsoft Teams
// integration security and authentication.
package teams

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	// TeamsSignatureHeader is the header containing the webhook signature
	TeamsSignatureHeader = "X-Hub-Signature-256"
	// TeamsTimestampHeader is the header containing the request timestamp
	TeamsTimestampHeader = "X-Timestamp"
	// MaxWebhookAge is the maximum age allowed for webhook requests (5 minutes)
	MaxWebhookAge = 5 * time.Minute
	// ExpectedContentType is the expected content type for Teams webhooks
	ExpectedContentType = "application/json"
)

// WebhookValidator handles security validation for Teams webhook requests
type WebhookValidator struct {
	webhookSecret string
	logger        *zap.Logger
	enabled       bool
}

// ValidationResult represents the result of webhook validation
type ValidationResult struct {
	Valid         bool
	ErrorMessage  string
	SecurityLevel string
	Timestamp     time.Time
}

// NewWebhookValidator creates a new webhook validator
func NewWebhookValidator(webhookSecret string, logger *zap.Logger) *WebhookValidator {
	enabled := webhookSecret != ""

	if !enabled {
		logger.Warn("Webhook validation disabled - no secret provided. " +
			"This should only be used in development environments.")
	}

	return &WebhookValidator{
		webhookSecret: webhookSecret,
		logger:        logger,
		enabled:       enabled,
	}
}

// ValidateWebhook validates an incoming Teams webhook request
func (wv *WebhookValidator) ValidateWebhook(req *http.Request, body []byte) *ValidationResult {
	result := &ValidationResult{
		Valid:         false,
		SecurityLevel: "none",
		Timestamp:     time.Now(),
	}

	// If validation is disabled, allow all requests (development only)
	if !wv.enabled {
		wv.logger.Debug("Webhook validation disabled - allowing request")
		result.Valid = true
		result.SecurityLevel = "disabled"
		return result
	}

	// Validate content type
	if err := wv.validateContentType(req); err != nil {
		result.ErrorMessage = fmt.Sprintf("invalid content type: %v", err)
		wv.logger.Warn("Webhook validation failed", zap.String("reason", result.ErrorMessage))
		return result
	}

	// Validate request timestamp (if provided)
	if err := wv.validateTimestamp(req); err != nil {
		result.ErrorMessage = fmt.Sprintf("invalid timestamp: %v", err)
		wv.logger.Warn("Webhook validation failed", zap.String("reason", result.ErrorMessage))
		return result
	}

	// Validate webhook signature
	if err := wv.validateSignature(req, body); err != nil {
		result.ErrorMessage = fmt.Sprintf("signature validation failed: %v", err)
		wv.logger.Error("Webhook signature validation failed", zap.Error(err))
		return result
	}

	// Additional security checks
	if err := wv.validateSecurityHeaders(req); err != nil {
		result.ErrorMessage = fmt.Sprintf("security header validation failed: %v", err)
		wv.logger.Warn("Webhook security validation failed", zap.String("reason", result.ErrorMessage))
		return result
	}

	result.Valid = true
	result.SecurityLevel = "validated"
	wv.logger.Debug("Webhook validation successful")

	return result
}

// validateContentType validates the request content type
func (wv *WebhookValidator) validateContentType(req *http.Request) error {
	contentType := req.Header.Get("Content-Type")
	if contentType == "" {
		return fmt.Errorf("missing Content-Type header")
	}

	// Check for expected content type (allow charset specification)
	if !strings.HasPrefix(contentType, ExpectedContentType) {
		return fmt.Errorf("expected %s, got %s", ExpectedContentType, contentType)
	}

	return nil
}

// validateTimestamp validates the request timestamp to prevent replay attacks
func (wv *WebhookValidator) validateTimestamp(req *http.Request) error {
	timestampStr := req.Header.Get(TeamsTimestampHeader)
	if timestampStr == "" {
		// Timestamp validation is optional - some Teams configurations may not include it
		wv.logger.Debug("No timestamp header provided - skipping timestamp validation")
		return nil
	}

	timestamp, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		return fmt.Errorf("invalid timestamp format: %w", err)
	}

	// Check if the request is too old
	age := time.Since(timestamp)
	if age > MaxWebhookAge {
		return fmt.Errorf("request too old: %v (max age: %v)", age, MaxWebhookAge)
	}

	// Check if the request is from the future (with some tolerance)
	if timestamp.After(time.Now().Add(1 * time.Minute)) {
		return fmt.Errorf("request timestamp is in the future")
	}

	return nil
}

// validateSignature validates the webhook signature using HMAC-SHA256
func (wv *WebhookValidator) validateSignature(req *http.Request, body []byte) error {
	signature := req.Header.Get(TeamsSignatureHeader)
	if signature == "" {
		// For Teams, signature validation might be optional depending on configuration
		wv.logger.Debug("No signature header provided - signature validation skipped")
		return nil
	}

	// Remove the "sha256=" prefix if present
	signature = strings.TrimPrefix(signature, "sha256=")

	// Compute expected signature
	expectedSignature, err := wv.computeSignature(body)
	if err != nil {
		return fmt.Errorf("failed to compute expected signature: %w", err)
	}

	// Compare signatures using constant-time comparison
	if !wv.compareSignatures(signature, expectedSignature) {
		return fmt.Errorf("signature mismatch")
	}

	return nil
}

// computeSignature computes HMAC-SHA256 signature for the given payload
func (wv *WebhookValidator) computeSignature(payload []byte) (string, error) {
	if wv.webhookSecret == "" {
		return "", fmt.Errorf("webhook secret not configured")
	}

	mac := hmac.New(sha256.New, []byte(wv.webhookSecret))
	if _, err := mac.Write(payload); err != nil {
		return "", fmt.Errorf("failed to write payload to HMAC: %w", err)
	}

	signature := hex.EncodeToString(mac.Sum(nil))
	return signature, nil
}

// compareSignatures performs constant-time comparison of two signatures
func (wv *WebhookValidator) compareSignatures(provided, expected string) bool {
	// Decode both signatures from hex
	providedBytes, err1 := hex.DecodeString(provided)
	expectedBytes, err2 := hex.DecodeString(expected)

	if err1 != nil || err2 != nil {
		wv.logger.Debug("Failed to decode signatures for comparison",
			zap.Error(err1), zap.Error(err2))
		return false
	}

	return hmac.Equal(providedBytes, expectedBytes)
}

// validateSecurityHeaders performs additional security validations
func (wv *WebhookValidator) validateSecurityHeaders(req *http.Request) error {
	// Check User-Agent header (Teams should provide a specific User-Agent)
	userAgent := req.Header.Get("User-Agent")
	if userAgent != "" && !wv.isValidTeamsUserAgent(userAgent) {
		wv.logger.Debug("Suspicious User-Agent detected", zap.String("user_agent", userAgent))
		// This is a warning, not a hard failure as User-Agent can be customized
	}

	// Validate request method
	if req.Method != http.MethodPost {
		return fmt.Errorf("invalid HTTP method: expected POST, got %s", req.Method)
	}

	// Check for suspicious headers that might indicate bot/automated requests
	suspiciousHeaders := []string{
		"X-Forwarded-For",
		"X-Real-IP",
		"X-Cluster-Client-IP",
	}

	for _, header := range suspiciousHeaders {
		if value := req.Header.Get(header); value != "" {
			wv.logger.Debug("Request contains forwarded IP header",
				zap.String("header", header),
				zap.String("value", value))
		}
	}

	return nil
}

// isValidTeamsUserAgent checks if the User-Agent appears to be from Microsoft Teams
func (wv *WebhookValidator) isValidTeamsUserAgent(userAgent string) bool {
	// Common Teams User-Agent patterns
	teamsPatterns := []string{
		"Microsoft-Teams",
		"Microsoft Teams",
		"SkypeBot",
		"Microsoft-BotFramework",
	}

	userAgentLower := strings.ToLower(userAgent)
	for _, pattern := range teamsPatterns {
		if strings.Contains(userAgentLower, strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}

// GetValidationMiddleware returns a Gin middleware for webhook validation
func (wv *WebhookValidator) GetValidationMiddleware() func(*http.Request, []byte) error {
	return func(req *http.Request, body []byte) error {
		result := wv.ValidateWebhook(req, body)
		if !result.Valid {
			return fmt.Errorf("webhook validation failed: %s", result.ErrorMessage)
		}
		return nil
	}
}

// LogValidationAttempt logs a validation attempt for monitoring and debugging
func (wv *WebhookValidator) LogValidationAttempt(req *http.Request, result *ValidationResult) {
	fields := []zap.Field{
		zap.Bool("valid", result.Valid),
		zap.String("security_level", result.SecurityLevel),
		zap.String("method", req.Method),
		zap.String("content_type", req.Header.Get("Content-Type")),
		zap.String("user_agent", req.Header.Get("User-Agent")),
		zap.Time("timestamp", result.Timestamp),
	}

	if result.ErrorMessage != "" {
		fields = append(fields, zap.String("error", result.ErrorMessage))
	}

	if result.Valid {
		wv.logger.Info("Webhook validation successful", fields...)
	} else {
		wv.logger.Warn("Webhook validation failed", fields...)
	}
}

// IsValidationEnabled returns whether webhook validation is enabled
func (wv *WebhookValidator) IsValidationEnabled() bool {
	return wv.enabled
}

// ValidateBase64Secret validates and decodes a base64-encoded webhook secret
func ValidateBase64Secret(encodedSecret string) (string, error) {
	if encodedSecret == "" {
		return "", nil // Empty secret is allowed for development
	}

	// Try to decode as base64
	decoded, err := base64.StdEncoding.DecodeString(encodedSecret)
	if err != nil {
		// If it's not valid base64, treat it as a plain string
		return encodedSecret, nil
	}

	return string(decoded), nil
}
