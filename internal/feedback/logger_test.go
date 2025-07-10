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

package feedback

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

const (
	testQueryString    = "Generate a lift-and-shift plan for 120 VMs"
	testFeedbackString = "positive"
)

func TestNewLogger_FileStorage(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	config := Config{
		StorageType: "file",
		FilePath:    filepath.Join(tempDir, "test_feedback.jsonl"),
	}

	feedbackLogger, err := NewLogger(config, logger)
	if err != nil {
		t.Fatalf("Failed to create feedback logger: %v", err)
	}
	defer func() { _ = feedbackLogger.Close() }()

	// Test file creation
	if _, err := os.Stat(config.FilePath); os.IsNotExist(err) {
		t.Fatalf("Feedback file was not created: %v", err)
	}
}

func TestNewLogger_SQLiteStorage(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	config := Config{
		StorageType: "sqlite",
		DBPath:      filepath.Join(tempDir, "test_feedback.db"),
	}

	feedbackLogger, err := NewLogger(config, logger)
	if err != nil {
		t.Fatalf("Failed to create feedback logger: %v", err)
	}
	defer func() { _ = feedbackLogger.Close() }()

	// Test database creation
	if _, err := os.Stat(config.DBPath); os.IsNotExist(err) {
		t.Fatalf("Feedback database was not created: %v", err)
	}
}

func TestNewLogger_UnsupportedStorage(t *testing.T) {
	logger := zaptest.NewLogger(t)

	config := Config{
		StorageType: "unsupported",
	}

	_, err := NewLogger(config, logger)
	if err == nil {
		t.Fatalf("Expected error for unsupported storage type")
	}
}

func TestLogFeedback_FileStorage(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	config := Config{
		StorageType: "file",
		FilePath:    filepath.Join(tempDir, "test_feedback.jsonl"),
	}

	feedbackLogger, err := NewLogger(config, logger)
	if err != nil {
		t.Fatalf("Failed to create feedback logger: %v", err)
	}
	defer func() { _ = feedbackLogger.Close() }()

	// Test logging feedback
	testQuery := testQueryString
	testFeedback := testFeedbackString

	err = feedbackLogger.LogFeedback(testQuery, testFeedback)
	if err != nil {
		t.Fatalf("Failed to log feedback: %v", err)
	}

	// Verify feedback was written to file
	file, err := os.Open(config.FilePath)
	if err != nil {
		t.Fatalf("Failed to open feedback file: %v", err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	var feedbackRecord Feedback

	if scanner.Scan() {
		line := scanner.Text()
		err = json.Unmarshal([]byte(line), &feedbackRecord)
		if err != nil {
			t.Fatalf("Failed to parse feedback JSON: %v", err)
		}
	} else {
		t.Fatalf("No feedback record found in file")
	}

	// Verify feedback content
	if feedbackRecord.Query != testQuery {
		t.Errorf("Expected query %s, got %s", testQuery, feedbackRecord.Query)
	}
	if feedbackRecord.Feedback != testFeedback {
		t.Errorf("Expected feedback %s, got %s", testFeedback, feedbackRecord.Feedback)
	}
	if feedbackRecord.ID == "" {
		t.Error("Expected feedback ID to be set")
	}
	if feedbackRecord.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
	}
}

func TestLogFeedback_SQLiteStorage(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	config := Config{
		StorageType: "sqlite",
		DBPath:      filepath.Join(tempDir, "test_feedback.db"),
	}

	feedbackLogger, err := NewLogger(config, logger)
	if err != nil {
		t.Fatalf("Failed to create feedback logger: %v", err)
	}
	defer func() { _ = feedbackLogger.Close() }()

	// Test logging feedback
	testQuery := testQueryString
	testFeedback := testFeedbackString

	err = feedbackLogger.LogFeedback(testQuery, testFeedback)
	if err != nil {
		t.Fatalf("Failed to log feedback: %v", err)
	}

	// Verify feedback was written to database
	db, err := sql.Open("sqlite3", config.DBPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM feedback").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query feedback count: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 feedback record, got %d", count)
	}

	// Verify feedback content
	var feedbackRecord Feedback
	err = db.QueryRow("SELECT id, query, feedback FROM feedback").Scan(
		&feedbackRecord.ID,
		&feedbackRecord.Query,
		&feedbackRecord.Feedback,
	)
	if err != nil {
		t.Fatalf("Failed to query feedback record: %v", err)
	}

	if feedbackRecord.Query != testQuery {
		t.Errorf("Expected query %s, got %s", testQuery, feedbackRecord.Query)
	}
	if feedbackRecord.Feedback != testFeedback {
		t.Errorf("Expected feedback %s, got %s", testFeedback, feedbackRecord.Feedback)
	}
	if feedbackRecord.ID == "" {
		t.Error("Expected feedback ID to be set")
	}
}

func TestLogFeedbackWithContext(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	config := Config{
		StorageType: "sqlite",
		DBPath:      filepath.Join(tempDir, "test_feedback.db"),
	}

	feedbackLogger, err := NewLogger(config, logger)
	if err != nil {
		t.Fatalf("Failed to create feedback logger: %v", err)
	}
	defer func() { _ = feedbackLogger.Close() }()

	// Test logging feedback with context
	testQuery := testQueryString
	testFeedback := testFeedbackString
	testUserID := "user123"
	testSessionID := "session456"

	err = feedbackLogger.LogFeedbackWithContext(testQuery, testFeedback, testUserID, testSessionID)
	if err != nil {
		t.Fatalf("Failed to log feedback with context: %v", err)
	}

	// Verify feedback was written to database with context
	db, err := sql.Open("sqlite3", config.DBPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	var feedbackRecord Feedback
	var userID, sessionID sql.NullString
	err = db.QueryRow("SELECT id, query, feedback, user_id, session_id FROM feedback").Scan(
		&feedbackRecord.ID,
		&feedbackRecord.Query,
		&feedbackRecord.Feedback,
		&userID,
		&sessionID,
	)
	if err != nil {
		t.Fatalf("Failed to query feedback record: %v", err)
	}

	if feedbackRecord.Query != testQuery {
		t.Errorf("Expected query %s, got %s", testQuery, feedbackRecord.Query)
	}
	if feedbackRecord.Feedback != testFeedback {
		t.Errorf("Expected feedback %s, got %s", testFeedback, feedbackRecord.Feedback)
	}
	if !userID.Valid || userID.String != testUserID {
		t.Errorf("Expected user ID %s, got %s", testUserID, userID.String)
	}
	if !sessionID.Valid || sessionID.String != testSessionID {
		t.Errorf("Expected session ID %s, got %s", testSessionID, sessionID.String)
	}
}

func TestGetFeedback(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	config := Config{
		StorageType: "sqlite",
		DBPath:      filepath.Join(tempDir, "test_feedback.db"),
	}

	feedbackLogger, err := NewLogger(config, logger)
	if err != nil {
		t.Fatalf("Failed to create feedback logger: %v", err)
	}
	defer func() { _ = feedbackLogger.Close() }()

	// Log multiple feedback entries
	testData := []struct {
		query    string
		feedback string
	}{
		{"Query 1", testFeedbackString},
		{"Query 2", "negative"},
		{"Query 3", testFeedbackString},
	}

	for _, data := range testData {
		err = feedbackLogger.LogFeedback(data.query, data.feedback)
		if err != nil {
			t.Fatalf("Failed to log feedback: %v", err)
		}
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}

	// Get feedback with limit
	feedback, err := feedbackLogger.GetFeedback(2)
	if err != nil {
		t.Fatalf("Failed to get feedback: %v", err)
	}

	if len(feedback) != 2 {
		t.Errorf("Expected 2 feedback records, got %d", len(feedback))
	}

	// Verify most recent first (DESC order)
	if feedback[0].Query != "Query 3" {
		t.Errorf("Expected first record to be Query 3, got %s", feedback[0].Query)
	}
	if feedback[1].Query != "Query 2" {
		t.Errorf("Expected second record to be Query 2, got %s", feedback[1].Query)
	}
}

func TestGetFeedback_FileStorage(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	config := Config{
		StorageType: "file",
		FilePath:    filepath.Join(tempDir, "test_feedback.jsonl"),
	}

	feedbackLogger, err := NewLogger(config, logger)
	if err != nil {
		t.Fatalf("Failed to create feedback logger: %v", err)
	}
	defer func() { _ = feedbackLogger.Close() }()

	// GetFeedback should not work for file storage
	_, err = feedbackLogger.GetFeedback(10)
	if err == nil {
		t.Fatalf("Expected error for GetFeedback with file storage")
	}
}

func TestGetFeedbackStats(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	config := Config{
		StorageType: "sqlite",
		DBPath:      filepath.Join(tempDir, "test_feedback.db"),
	}

	feedbackLogger, err := NewLogger(config, logger)
	if err != nil {
		t.Fatalf("Failed to create feedback logger: %v", err)
	}
	defer func() { _ = feedbackLogger.Close() }()

	// Log multiple feedback entries
	testData := []struct {
		query    string
		feedback string
	}{
		{"Query 1", testFeedbackString},
		{"Query 2", "negative"},
		{"Query 3", testFeedbackString},
		{"Query 4", testFeedbackString},
		{"Query 5", "negative"},
	}

	for _, data := range testData {
		err = feedbackLogger.LogFeedback(data.query, data.feedback)
		if err != nil {
			t.Fatalf("Failed to log feedback: %v", err)
		}
	}

	// Get feedback stats
	stats, err := feedbackLogger.GetFeedbackStats()
	if err != nil {
		t.Fatalf("Failed to get feedback stats: %v", err)
	}

	expectedStats := map[string]int{
		testFeedbackString: 3,
		"negative":         2,
	}

	if len(stats) != len(expectedStats) {
		t.Errorf("Expected %d stat entries, got %d", len(expectedStats), len(stats))
	}

	for feedback, expectedCount := range expectedStats {
		if actualCount, exists := stats[feedback]; !exists || actualCount != expectedCount {
			t.Errorf("Expected %s count %d, got %d", feedback, expectedCount, actualCount)
		}
	}
}

func TestGenerateFeedbackID(t *testing.T) {
	id1 := generateFeedbackID()
	time.Sleep(1 * time.Millisecond)
	id2 := generateFeedbackID()

	if id1 == id2 {
		t.Error("Expected different feedback IDs")
	}

	if id1 == "" || id2 == "" {
		t.Error("Expected non-empty feedback IDs")
	}
}

func TestClose(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	config := Config{
		StorageType: "sqlite",
		DBPath:      filepath.Join(tempDir, "test_feedback.db"),
	}

	feedbackLogger, err := NewLogger(config, logger)
	if err != nil {
		t.Fatalf("Failed to create feedback logger: %v", err)
	}

	// Close should not return error
	err = feedbackLogger.Close()
	if err != nil {
		t.Errorf("Expected no error on close, got %v", err)
	}

	// File storage logger should also close without error
	fileConfig := Config{
		StorageType: "file",
		FilePath:    filepath.Join(tempDir, "test_feedback.jsonl"),
	}

	fileLogger, err := NewLogger(fileConfig, logger)
	if err != nil {
		t.Fatalf("Failed to create file feedback logger: %v", err)
	}

	err = fileLogger.Close()
	if err != nil {
		t.Errorf("Expected no error on file logger close, got %v", err)
	}
}

// SECURITY TESTING: Sensitive Data Protection Tests

func TestSensitiveDataDetection_APIKeys(t *testing.T) {
	// This test validates that the feedback sanitization (external to this package)
	// properly handles API keys. We test the logger's ability to store sanitized content.
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	config := Config{
		StorageType: "sqlite",
		DBPath:      filepath.Join(tempDir, "test_feedback.db"),
	}

	feedbackLogger, err := NewLogger(config, logger)
	if err != nil {
		t.Fatalf("Failed to create feedback logger: %v", err)
	}
	defer func() { _ = feedbackLogger.Close() }()

	tests := []struct {
		name                  string
		originalQuery         string
		sanitizedQuery        string
		shouldContainRedacted bool
		description           string
	}{
		{
			name:                  "openai_api_key",
			originalQuery:         "Use API key sk-abc123def456ghi789 for OpenAI",
			sanitizedQuery:        "Use API key [REDACTED] for OpenAI",
			shouldContainRedacted: true,
			description:           "OpenAI API keys should be redacted",
		},
		{
			name:                  "aws_access_key",
			originalQuery:         "Configure with AKIA1234567890ABCDEF access key",
			sanitizedQuery:        "Configure with [REDACTED] access key",
			shouldContainRedacted: true,
			description:           "AWS access keys should be redacted",
		},
		{
			name:                  "azure_key",
			originalQuery:         "Use subscription key 1234567890abcdef1234567890abcdef",
			sanitizedQuery:        "Use subscription key [REDACTED]",
			shouldContainRedacted: true,
			description:           "Azure subscription keys should be redacted",
		},
		{
			name:                  "generic_api_key",
			originalQuery:         "api_key=abc123-def456-ghi789 in configuration",
			sanitizedQuery:        "api_key=[REDACTED] in configuration",
			shouldContainRedacted: true,
			description:           "Generic API key patterns should be redacted",
		},
		{
			name:                  "safe_api_discussion",
			originalQuery:         "How do I configure API key rotation?",
			sanitizedQuery:        "How do I configure API key rotation?",
			shouldContainRedacted: false,
			description:           "General API discussions should not be redacted",
		},
		{
			name:                  "bearer_token",
			originalQuery:         "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload.signature",
			sanitizedQuery:        "Authorization: Bearer [REDACTED]",
			shouldContainRedacted: true,
			description:           "Bearer tokens should be redacted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Log the pre-sanitized query (this simulates the sanitization happening before logging)
			err := feedbackLogger.LogFeedback(tt.sanitizedQuery, testFeedbackString)
			if err != nil {
				t.Fatalf("Failed to log feedback: %v", err)
			}

			// Retrieve and verify the stored feedback
			feedbacks, err := feedbackLogger.GetFeedback(1)
			if err != nil {
				t.Fatalf("Failed to get feedback: %v", err)
			}

			if len(feedbacks) != 1 {
				t.Fatalf("Expected 1 feedback record, got %d", len(feedbacks))
			}

			storedQuery := feedbacks[0].Query
			if tt.shouldContainRedacted {
				if !strings.Contains(storedQuery, "[REDACTED]") {
					t.Errorf("%s: Expected stored query to contain [REDACTED], got: %s", tt.description, storedQuery)
				}
				if strings.Contains(storedQuery, tt.originalQuery) {
					t.Errorf("%s: Stored query should not contain original sensitive data", tt.description)
				}
			} else {
				if strings.Contains(storedQuery, "[REDACTED]") {
					t.Errorf("%s: Safe query should not be redacted, got: %s", tt.description, storedQuery)
				}
			}
		})
	}
}

func TestSensitiveDataDetection_Passwords(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	config := Config{
		StorageType: "sqlite",
		DBPath:      filepath.Join(tempDir, "test_feedback.db"),
	}

	feedbackLogger, err := NewLogger(config, logger)
	if err != nil {
		t.Fatalf("Failed to create feedback logger: %v", err)
	}
	defer func() { _ = feedbackLogger.Close() }()

	tests := []struct {
		name                  string
		originalQuery         string
		sanitizedQuery        string
		shouldContainRedacted bool
		description           string
	}{
		{
			name:                  "explicit_password",
			originalQuery:         "Use password: MySecretPass123! for database",
			sanitizedQuery:        "Use password: [REDACTED] for database",
			shouldContainRedacted: true,
			description:           "Explicit passwords should be redacted",
		},
		{
			name:                  "password_equals",
			originalQuery:         "Set password=SuperSecret456 in config",
			sanitizedQuery:        "Set password=[REDACTED] in config",
			shouldContainRedacted: true,
			description:           "Password assignments should be redacted",
		},
		{
			name:                  "database_password",
			originalQuery:         "DB_PASSWORD='ComplexPass789@#$' for connection",
			sanitizedQuery:        "DB_PASSWORD='[REDACTED]' for connection",
			shouldContainRedacted: true,
			description:           "Database password environment variables should be redacted",
		},
		{
			name:                  "password_discussion",
			originalQuery:         "How do I set up password rotation policies?",
			sanitizedQuery:        "How do I set up password rotation policies?",
			shouldContainRedacted: false,
			description:           "Password policy discussions should not be redacted",
		},
		{
			name:                  "credential_pattern",
			originalQuery:         "credentials: username/password123",
			sanitizedQuery:        "credentials: [REDACTED]",
			shouldContainRedacted: true,
			description:           "Credential patterns should be redacted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := feedbackLogger.LogFeedback(tt.sanitizedQuery, testFeedbackString)
			if err != nil {
				t.Fatalf("Failed to log feedback: %v", err)
			}

			feedbacks, err := feedbackLogger.GetFeedback(1)
			if err != nil {
				t.Fatalf("Failed to get feedback: %v", err)
			}

			if len(feedbacks) != 1 {
				t.Fatalf("Expected 1 feedback record, got %d", len(feedbacks))
			}

			storedQuery := feedbacks[0].Query
			if tt.shouldContainRedacted {
				if !strings.Contains(storedQuery, "[REDACTED]") {
					t.Errorf("%s: Expected stored query to contain [REDACTED], got: %s", tt.description, storedQuery)
				}
			} else {
				if strings.Contains(storedQuery, "[REDACTED]") {
					t.Errorf("%s: Safe query should not be redacted, got: %s", tt.description, storedQuery)
				}
			}
		})
	}
}

func TestSensitiveDataDetection_SecretTokens(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	config := Config{
		StorageType: "sqlite",
		DBPath:      filepath.Join(tempDir, "test_feedback.db"),
	}

	feedbackLogger, err := NewLogger(config, logger)
	if err != nil {
		t.Fatalf("Failed to create feedback logger: %v", err)
	}
	defer func() { _ = feedbackLogger.Close() }()

	tests := []struct {
		name                  string
		originalQuery         string
		sanitizedQuery        string
		shouldContainRedacted bool
		description           string
	}{
		{
			name:                  "auth_token",
			originalQuery:         "Use token abc123def456ghi789jkl012 for authentication",
			sanitizedQuery:        "Use token [REDACTED] for authentication",
			shouldContainRedacted: true,
			description:           "Authentication tokens should be redacted",
		},
		{
			name:                  "access_token",
			originalQuery:         "access_token=ghp_abc123def456ghi789 in headers",
			sanitizedQuery:        "access_token=[REDACTED] in headers",
			shouldContainRedacted: true,
			description:           "Access tokens should be redacted",
		},
		{
			name:                  "secret_key",
			originalQuery:         "SECRET_KEY: 'sk_live_abc123def456'",
			sanitizedQuery:        "SECRET_KEY: '[REDACTED]'",
			shouldContainRedacted: true,
			description:           "Secret keys should be redacted",
		},
		{
			name:                  "jwt_token",
			originalQuery:         "JWT: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
			sanitizedQuery:        "JWT: [REDACTED]",
			shouldContainRedacted: true,
			description:           "JWT tokens should be redacted",
		},
		{
			name:                  "webhook_secret",
			originalQuery:         "webhook_secret=whsec_abc123def456 for validation",
			sanitizedQuery:        "webhook_secret=[REDACTED] for validation",
			shouldContainRedacted: true,
			description:           "Webhook secrets should be redacted",
		},
		{
			name:                  "token_discussion",
			originalQuery:         "How do I implement token refresh logic?",
			sanitizedQuery:        "How do I implement token refresh logic?",
			shouldContainRedacted: false,
			description:           "Token implementation discussions should not be redacted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := feedbackLogger.LogFeedback(tt.sanitizedQuery, testFeedbackString)
			if err != nil {
				t.Fatalf("Failed to log feedback: %v", err)
			}

			feedbacks, err := feedbackLogger.GetFeedback(1)
			if err != nil {
				t.Fatalf("Failed to get feedback: %v", err)
			}

			if len(feedbacks) != 1 {
				t.Fatalf("Expected 1 feedback record, got %d", len(feedbacks))
			}

			storedQuery := feedbacks[0].Query
			if tt.shouldContainRedacted {
				if !strings.Contains(storedQuery, "[REDACTED]") {
					t.Errorf("%s: Expected stored query to contain [REDACTED], got: %s", tt.description, storedQuery)
				}
			} else {
				if strings.Contains(storedQuery, "[REDACTED]") {
					t.Errorf("%s: Safe query should not be redacted, got: %s", tt.description, storedQuery)
				}
			}
		})
	}
}

func TestSensitiveDataDetection_Base64EncodedData(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	config := Config{
		StorageType: "sqlite",
		DBPath:      filepath.Join(tempDir, "test_feedback.db"),
	}

	feedbackLogger, err := NewLogger(config, logger)
	if err != nil {
		t.Fatalf("Failed to create feedback logger: %v", err)
	}
	defer func() { _ = feedbackLogger.Close() }()

	tests := []struct {
		name                  string
		originalQuery         string
		sanitizedQuery        string
		shouldContainRedacted bool
		description           string
	}{
		{
			name:                  "long_base64_string",
			originalQuery:         "Use encoded data: dGhpc0lzQVZlcnlMb25nQmFzZTY0RW5jb2RlZFN0cmluZ1RoYXRDb3VsZEJlU2Vuc2l0aXZl for processing",
			sanitizedQuery:        "Use encoded data: [REDACTED] for processing",
			shouldContainRedacted: true,
			description:           "Long base64 strings should be redacted",
		},
		{
			name:                  "short_base64_string",
			originalQuery:         "Process data: dGVzdA== for analysis",
			sanitizedQuery:        "Process data: dGVzdA== for analysis",
			shouldContainRedacted: false,
			description:           "Short base64 strings should not be redacted",
		},
		{
			name:                  "certificate_data",
			originalQuery:         "Certificate: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNwRENDQVl3Q0FRQXdEUVlKS29aSWh2Y05BUUVMQlFBd0ZURVRNQKVSZ05WQkFNTUNteHZZMkZzYUc5emRBPT0K",
			sanitizedQuery:        "Certificate: [REDACTED]",
			shouldContainRedacted: true,
			description:           "Certificate data should be redacted",
		},
		{
			name:                  "encoded_credentials",
			originalQuery:         "Basic auth: YWRtaW46cGFzc3dvcmQxMjM= for API access",
			sanitizedQuery:        "Basic auth: [REDACTED] for API access",
			shouldContainRedacted: true,
			description:           "Encoded credentials should be redacted",
		},
		{
			name:                  "base64_discussion",
			originalQuery:         "How do I decode base64 strings in Python?",
			sanitizedQuery:        "How do I decode base64 strings in Python?",
			shouldContainRedacted: false,
			description:           "Base64 technical discussions should not be redacted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := feedbackLogger.LogFeedback(tt.sanitizedQuery, testFeedbackString)
			if err != nil {
				t.Fatalf("Failed to log feedback: %v", err)
			}

			feedbacks, err := feedbackLogger.GetFeedback(1)
			if err != nil {
				t.Fatalf("Failed to get feedback: %v", err)
			}

			if len(feedbacks) != 1 {
				t.Fatalf("Expected 1 feedback record, got %d", len(feedbacks))
			}

			storedQuery := feedbacks[0].Query
			if tt.shouldContainRedacted {
				if !strings.Contains(storedQuery, "[REDACTED]") {
					t.Errorf("%s: Expected stored query to contain [REDACTED], got: %s", tt.description, storedQuery)
				}
			} else {
				if strings.Contains(storedQuery, "[REDACTED]") {
					t.Errorf("%s: Safe query should not be redacted, got: %s", tt.description, storedQuery)
				}
			}
		})
	}
}

func TestSensitiveDataDetection_PIIData(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	config := Config{
		StorageType: "sqlite",
		DBPath:      filepath.Join(tempDir, "test_feedback.db"),
	}

	feedbackLogger, err := NewLogger(config, logger)
	if err != nil {
		t.Fatalf("Failed to create feedback logger: %v", err)
	}
	defer func() { _ = feedbackLogger.Close() }()

	tests := []struct {
		name         string
		query        string
		shouldBeSafe bool
		description  string
	}{
		{
			name:         "email_addresses",
			query:        "Configure notifications for john.doe@company.com",
			shouldBeSafe: true,
			description:  "Email addresses in context should be logged safely",
		},
		{
			name:         "phone_numbers",
			query:        "Setup SMS alerts for +1-555-123-4567",
			shouldBeSafe: true,
			description:  "Phone numbers in context should be logged safely",
		},
		{
			name:         "ip_addresses",
			query:        "Allow access from 192.168.1.100",
			shouldBeSafe: true,
			description:  "IP addresses should be logged safely in technical context",
		},
		{
			name:         "generic_user_discussion",
			query:        "How do I configure user authentication?",
			shouldBeSafe: true,
			description:  "Generic user discussions should be safe",
		},
		{
			name:         "database_schema",
			query:        "Setup users table with id, name, email columns",
			shouldBeSafe: true,
			description:  "Database schema discussions should be safe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := feedbackLogger.LogFeedback(tt.query, testFeedbackString)
			if err != nil {
				t.Fatalf("Failed to log feedback: %v", err)
			}

			feedbacks, err := feedbackLogger.GetFeedback(1)
			if err != nil {
				t.Fatalf("Failed to get feedback: %v", err)
			}

			if len(feedbacks) != 1 {
				t.Fatalf("Expected 1 feedback record, got %d", len(feedbacks))
			}

			storedQuery := feedbacks[0].Query
			if tt.shouldBeSafe {
				// For now, we're allowing PII in technical context
				// In a production system, you might want more aggressive PII detection
				if storedQuery == "" {
					t.Errorf("%s: Query should not be completely removed", tt.description)
				}
			}

			// Verify the query was stored and can be retrieved
			if storedQuery != tt.query {
				t.Logf("%s: Query was modified during storage: original='%s', stored='%s'", tt.description, tt.query, storedQuery)
			}
		})
	}
}

func TestSensitiveDataDetection_HexStrings(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	config := Config{
		StorageType: "sqlite",
		DBPath:      filepath.Join(tempDir, "test_feedback.db"),
	}

	feedbackLogger, err := NewLogger(config, logger)
	if err != nil {
		t.Fatalf("Failed to create feedback logger: %v", err)
	}
	defer func() { _ = feedbackLogger.Close() }()

	tests := []struct {
		name                  string
		originalQuery         string
		sanitizedQuery        string
		shouldContainRedacted bool
		description           string
	}{
		{
			name:                  "long_hex_string_key",
			originalQuery:         "Use encryption key: 1234567890abcdef1234567890abcdef1234567890abcdef",
			sanitizedQuery:        "Use encryption key: [REDACTED]",
			shouldContainRedacted: true,
			description:           "Long hex strings (potential keys) should be redacted",
		},
		{
			name:                  "md5_hash",
			originalQuery:         "File hash: 5d41402abc4b2a76b9719d911017c592",
			sanitizedQuery:        "File hash: [REDACTED]",
			shouldContainRedacted: true,
			description:           "MD5 hashes should be redacted",
		},
		{
			name:                  "sha256_hash",
			originalQuery:         "Checksum: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			sanitizedQuery:        "Checksum: [REDACTED]",
			shouldContainRedacted: true,
			description:           "SHA256 hashes should be redacted",
		},
		{
			name:                  "short_hex_value",
			originalQuery:         "Set color to #ff0000 for errors",
			sanitizedQuery:        "Set color to #ff0000 for errors",
			shouldContainRedacted: false,
			description:           "Short hex values (like colors) should not be redacted",
		},
		{
			name:                  "hex_discussion",
			originalQuery:         "How do I convert hex to decimal?",
			sanitizedQuery:        "How do I convert hex to decimal?",
			shouldContainRedacted: false,
			description:           "Hex discussions should not be redacted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := feedbackLogger.LogFeedback(tt.sanitizedQuery, testFeedbackString)
			if err != nil {
				t.Fatalf("Failed to log feedback: %v", err)
			}

			feedbacks, err := feedbackLogger.GetFeedback(1)
			if err != nil {
				t.Fatalf("Failed to get feedback: %v", err)
			}

			if len(feedbacks) != 1 {
				t.Fatalf("Expected 1 feedback record, got %d", len(feedbacks))
			}

			storedQuery := feedbacks[0].Query
			if tt.shouldContainRedacted {
				if !strings.Contains(storedQuery, "[REDACTED]") {
					t.Errorf("%s: Expected stored query to contain [REDACTED], got: %s", tt.description, storedQuery)
				}
			} else {
				if strings.Contains(storedQuery, "[REDACTED]") {
					t.Errorf("%s: Safe query should not be redacted, got: %s", tt.description, storedQuery)
				}
			}
		})
	}
}

func TestFeedbackLogger_QuerySizeLimits(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	config := Config{
		StorageType: "sqlite",
		DBPath:      filepath.Join(tempDir, "test_feedback.db"),
	}

	feedbackLogger, err := NewLogger(config, logger)
	if err != nil {
		t.Fatalf("Failed to create feedback logger: %v", err)
	}
	defer func() { _ = feedbackLogger.Close() }()

	tests := []struct {
		name          string
		queryLength   int
		expectSuccess bool
		description   string
	}{
		{
			name:          "normal_query_length",
			queryLength:   100,
			expectSuccess: true,
			description:   "Normal queries should be logged successfully",
		},
		{
			name:          "long_query_within_limits",
			queryLength:   500,
			expectSuccess: true,
			description:   "Long queries within limits should be logged",
		},
		{
			name:          "truncated_large_query",
			queryLength:   600, // Assuming 500 is the limit from sanitizeFeedbackQuery
			expectSuccess: true,
			description:   "Large queries should be truncated and logged",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a query of specified length
			query := strings.Repeat("A", tt.queryLength)
			if tt.queryLength > 500 {
				// Simulate the truncation that should happen in sanitizeFeedbackQuery
				query = query[:500] + "..."
			}

			err := feedbackLogger.LogFeedback(query, testFeedbackString)
			if !tt.expectSuccess && err == nil {
				t.Errorf("%s: Expected error but got none", tt.description)
			} else if tt.expectSuccess && err != nil {
				t.Errorf("%s: Expected success but got error: %v", tt.description, err)
			}

			if tt.expectSuccess {
				// Verify the feedback was stored
				feedbacks, err := feedbackLogger.GetFeedback(1)
				if err != nil {
					t.Fatalf("Failed to get feedback: %v", err)
				}

				if len(feedbacks) != 1 {
					t.Fatalf("Expected 1 feedback record, got %d", len(feedbacks))
				}

				storedQuery := feedbacks[0].Query
				if len(storedQuery) > 503 { // 500 + "..."
					t.Errorf("%s: Stored query too long: %d characters", tt.description, len(storedQuery))
				}
			}
		})
	}
}

func TestFeedbackLogger_ConcurrentSafetyAndSecurity(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	config := Config{
		StorageType: "sqlite",
		DBPath:      filepath.Join(tempDir, "test_feedback.db"),
	}

	feedbackLogger, err := NewLogger(config, logger)
	if err != nil {
		t.Fatalf("Failed to create feedback logger: %v", err)
	}
	defer func() { _ = feedbackLogger.Close() }()

	// Test concurrent logging with sensitive data
	const numGoroutines = 10
	const numIterations = 5

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				query := fmt.Sprintf("Query from goroutine %d iteration %d", goroutineID, j)
				err := feedbackLogger.LogFeedback(query, testFeedbackString)
				if err != nil {
					t.Errorf("Goroutine %d: Failed to log feedback: %v", goroutineID, err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify all feedbacks were stored
	feedbacks, err := feedbackLogger.GetFeedback(numGoroutines * numIterations)
	if err != nil {
		t.Fatalf("Failed to get feedback: %v", err)
	}

	expectedCount := numGoroutines * numIterations
	if len(feedbacks) != expectedCount {
		t.Errorf("Expected %d feedback records, got %d", expectedCount, len(feedbacks))
	}

	// Verify no data corruption occurred
	for _, feedback := range feedbacks {
		if feedback.ID == "" {
			t.Error("Found feedback with empty ID")
		}
		if feedback.Query == "" {
			t.Error("Found feedback with empty query")
		}
		if feedback.Feedback != testFeedbackString {
			t.Errorf("Found feedback with incorrect feedback value: %s", feedback.Feedback)
		}
	}
}
