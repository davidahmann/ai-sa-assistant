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

// Package feedback provides functionality for logging and storing user feedback
// on assistant responses. It supports both file-based and SQLite storage.
package feedback

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

const (
	StorageTypeFile   = "file"
	StorageTypeSQLite = "sqlite"
)

// Feedback represents a user feedback record
type Feedback struct {
	ID        string    `json:"id"`
	Query     string    `json:"query"`
	Feedback  string    `json:"feedback"`
	Timestamp time.Time `json:"timestamp"`
	UserID    string    `json:"user_id,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
}

// Logger handles feedback logging to various storage backends
type Logger struct {
	config Config
	logger *zap.Logger
	db     *sql.DB
	mu     sync.RWMutex
}

// Config holds configuration for feedback logging
type Config struct {
	StorageType string `json:"storage_type"` // StorageTypeFile or StorageTypeSQLite
	FilePath    string `json:"file_path"`    // Path for file storage
	DBPath      string `json:"db_path"`      // Path for SQLite database
}

// NewLogger creates a new feedback logger
func NewLogger(config Config, logger *zap.Logger) (*Logger, error) {
	fl := &Logger{
		config: config,
		logger: logger,
	}

	switch config.StorageType {
	case "file":
		if err := fl.initFileStorage(); err != nil {
			return nil, fmt.Errorf("failed to initialize file storage: %w", err)
		}
	case StorageTypeSQLite:
		if err := fl.initSQLiteStorage(); err != nil {
			return nil, fmt.Errorf("failed to initialize SQLite storage: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", config.StorageType)
	}

	return fl, nil
}

// initFileStorage initializes file-based storage
func (fl *Logger) initFileStorage() error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(fl.config.FilePath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create feedback directory: %w", err)
	}

	// Create file if it doesn't exist
	if _, err := os.Stat(fl.config.FilePath); os.IsNotExist(err) {
		file, err := os.Create(fl.config.FilePath)
		if err != nil {
			return fmt.Errorf("failed to create feedback file: %w", err)
		}
		_ = file.Close()
	}

	return nil
}

// initSQLiteStorage initializes SQLite-based storage
func (fl *Logger) initSQLiteStorage() error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(fl.config.DBPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create feedback database directory: %w", err)
	}

	// Open database connection
	db, err := sql.Open("sqlite3", fl.config.DBPath)
	if err != nil {
		return fmt.Errorf("failed to open SQLite database: %w", err)
	}

	// Create feedback table if it doesn't exist
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS feedback (
			id TEXT PRIMARY KEY,
			query TEXT NOT NULL,
			feedback TEXT NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			user_id TEXT,
			session_id TEXT
		);
	`

	if _, err := db.Exec(createTableSQL); err != nil {
		_ = db.Close()
		return fmt.Errorf("failed to create feedback table: %w", err)
	}

	fl.db = db
	return nil
}

// LogFeedback records a feedback entry
func (fl *Logger) LogFeedback(query, feedback string) error {
	return fl.LogFeedbackWithContext(query, feedback, "", "")
}

// LogFeedbackWithContext records a feedback entry with additional context
func (fl *Logger) LogFeedbackWithContext(query, feedback, userID, sessionID string) error {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	feedbackRecord := Feedback{
		ID:        generateFeedbackID(),
		Query:     query,
		Feedback:  feedback,
		Timestamp: time.Now(),
		UserID:    userID,
		SessionID: sessionID,
	}

	switch fl.config.StorageType {
	case "file":
		return fl.logToFile(feedbackRecord)
	case StorageTypeSQLite:
		return fl.logToSQLite(feedbackRecord)
	default:
		return fmt.Errorf("unsupported storage type: %s", fl.config.StorageType)
	}
}

// logToFile writes feedback to a JSON file
func (fl *Logger) logToFile(feedback Feedback) error {
	file, err := os.OpenFile(fl.config.FilePath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open feedback file: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Write JSON line
	jsonData, err := json.Marshal(feedback)
	if err != nil {
		return fmt.Errorf("failed to marshal feedback: %w", err)
	}

	if _, err := file.WriteString(string(jsonData) + "\n"); err != nil {
		return fmt.Errorf("failed to write feedback to file: %w", err)
	}

	fl.logger.Info("Feedback logged to file",
		zap.String("id", feedback.ID),
		zap.String("query", feedback.Query),
		zap.String("feedback", feedback.Feedback))

	return nil
}

// logToSQLite writes feedback to SQLite database
func (fl *Logger) logToSQLite(feedback Feedback) error {
	if fl.db == nil {
		return fmt.Errorf("SQLite database not initialized")
	}

	insertSQL := `
		INSERT INTO feedback (id, query, feedback, timestamp, user_id, session_id)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := fl.db.Exec(insertSQL,
		feedback.ID,
		feedback.Query,
		feedback.Feedback,
		feedback.Timestamp,
		feedback.UserID,
		feedback.SessionID,
	)

	if err != nil {
		return fmt.Errorf("failed to insert feedback into SQLite: %w", err)
	}

	fl.logger.Info("Feedback logged to SQLite",
		zap.String("id", feedback.ID),
		zap.String("query", feedback.Query),
		zap.String("feedback", feedback.Feedback))

	return nil
}

// GetFeedback retrieves feedback entries (for SQLite only)
func (fl *Logger) GetFeedback(limit int) ([]Feedback, error) {
	if fl.config.StorageType != StorageTypeSQLite {
		return nil, fmt.Errorf("GetFeedback only supported for SQLite storage")
	}

	if fl.db == nil {
		return nil, fmt.Errorf("SQLite database not initialized")
	}

	fl.mu.RLock()
	defer fl.mu.RUnlock()

	query := `
		SELECT id, query, feedback, timestamp, user_id, session_id
		FROM feedback
		ORDER BY timestamp DESC
		LIMIT ?
	`

	rows, err := fl.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query feedback: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var feedbacks []Feedback
	for rows.Next() {
		var feedback Feedback
		var userID, sessionID sql.NullString

		err := rows.Scan(
			&feedback.ID,
			&feedback.Query,
			&feedback.Feedback,
			&feedback.Timestamp,
			&userID,
			&sessionID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan feedback row: %w", err)
		}

		if userID.Valid {
			feedback.UserID = userID.String
		}
		if sessionID.Valid {
			feedback.SessionID = sessionID.String
		}

		feedbacks = append(feedbacks, feedback)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate feedback rows: %w", err)
	}

	return feedbacks, nil
}

// GetFeedbackStats returns feedback statistics (for SQLite only)
func (fl *Logger) GetFeedbackStats() (map[string]int, error) {
	if fl.config.StorageType != StorageTypeSQLite {
		return nil, fmt.Errorf("GetFeedbackStats only supported for SQLite storage")
	}

	if fl.db == nil {
		return nil, fmt.Errorf("SQLite database not initialized")
	}

	fl.mu.RLock()
	defer fl.mu.RUnlock()

	query := `
		SELECT feedback, COUNT(*) as count
		FROM feedback
		GROUP BY feedback
	`

	rows, err := fl.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query feedback stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	stats := make(map[string]int)
	for rows.Next() {
		var feedback string
		var count int

		err := rows.Scan(&feedback, &count)
		if err != nil {
			return nil, fmt.Errorf("failed to scan feedback stats row: %w", err)
		}

		stats[feedback] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate feedback stats rows: %w", err)
	}

	return stats, nil
}

// Close closes the feedback logger and any open resources
func (fl *Logger) Close() error {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	if fl.db != nil {
		return fl.db.Close()
	}

	return nil
}

// generateFeedbackID generates a unique ID for feedback entries
func generateFeedbackID() string {
	return fmt.Sprintf("feedback_%d", time.Now().UnixNano())
}
