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
	"os"
	"path/filepath"
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
