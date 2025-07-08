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

package metadata

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

// Store handles queries to the SQLite metadata database
type Store struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewStore creates a new metadata store
func NewStore(dbPath string, logger *zap.Logger) (*Store, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	logger.Info("Initializing metadata store", zap.String("db_path", dbPath))

	// Ensure the directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test database connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	store := &Store{
		db:     db,
		logger: logger,
	}

	// Initialize database schema
	if err := store.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	s.logger.Info("Closing metadata store")
	return s.db.Close()
}

// initSchema creates the metadata table if it doesn't exist
func (s *Store) initSchema() error {
	s.logger.Info("Initializing database schema")

	query := `
		CREATE TABLE IF NOT EXISTS metadata (
			doc_id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			platform TEXT NOT NULL,
			scenario TEXT NOT NULL,
			type TEXT NOT NULL,
			source_url TEXT,
			path TEXT,
			tags TEXT, -- JSON array stored as text
			difficulty TEXT,
			estimated_time TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_platform ON metadata(platform);
		CREATE INDEX IF NOT EXISTS idx_scenario ON metadata(scenario);
		CREATE INDEX IF NOT EXISTS idx_type ON metadata(type);
		CREATE INDEX IF NOT EXISTS idx_difficulty ON metadata(difficulty);
	`

	_, err := s.db.Exec(query)
	if err != nil {
		s.logger.Error("Failed to initialize schema", zap.Error(err))
		return fmt.Errorf("failed to create schema: %w", err)
	}

	s.logger.Info("Database schema initialized successfully")
	return nil
}

// MetadataEntry represents a metadata entry matching the new metadata.json structure
type MetadataEntry struct {
	DocID         string   `json:"doc_id"`
	Title         string   `json:"title"`
	Platform      string   `json:"platform"`
	Scenario      string   `json:"scenario"`
	Type          string   `json:"type"`
	SourceURL     string   `json:"source_url"`
	Path          string   `json:"path"`
	Tags          []string `json:"tags"`
	Difficulty    string   `json:"difficulty"`
	EstimatedTime string   `json:"estimated_time"`
}

// MetadataIndex represents the root structure of metadata.json
type MetadataIndex struct {
	SchemaVersion  string          `json:"schema_version"`
	Description    string          `json:"description"`
	LastUpdated    string          `json:"last_updated"`
	Documents      []MetadataEntry `json:"documents"`
	MetadataSchema interface{}     `json:"metadata_schema"`
}

// AddMetadata adds a metadata entry to the database
func (s *Store) AddMetadata(entry MetadataEntry) error {
	s.logger.Debug("Adding metadata entry", zap.String("doc_id", entry.DocID))

	tagsJSON, err := json.Marshal(entry.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	query := `
		INSERT OR REPLACE INTO metadata (
			doc_id, title, platform, scenario, type, source_url, path, tags, difficulty, estimated_time, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`

	_, err = s.db.Exec(query, entry.DocID, entry.Title, entry.Platform, entry.Scenario, entry.Type,
		entry.SourceURL, entry.Path, string(tagsJSON), entry.Difficulty, entry.EstimatedTime)
	if err != nil {
		s.logger.Error("Failed to insert metadata", zap.Error(err), zap.String("doc_id", entry.DocID))
		return fmt.Errorf("failed to insert metadata: %w", err)
	}

	s.logger.Debug("Metadata entry added successfully", zap.String("doc_id", entry.DocID))
	return nil
}

// LoadFromJSON loads metadata from a JSON file and populates the database
func (s *Store) LoadFromJSON(jsonPath string) error {
	s.logger.Info("Loading metadata from JSON file", zap.String("json_path", jsonPath))

	// Read JSON file
	data, err := ioutil.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("failed to read JSON file: %w", err)
	}

	// Parse JSON
	var metadataIndex MetadataIndex
	if err := json.Unmarshal(data, &metadataIndex); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	s.logger.Info("Parsed metadata index",
		zap.String("schema_version", metadataIndex.SchemaVersion),
		zap.String("description", metadataIndex.Description),
		zap.String("last_updated", metadataIndex.LastUpdated),
		zap.Int("document_count", len(metadataIndex.Documents)))

	// Begin transaction for bulk insert
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare statement for bulk insert
	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO metadata (
			doc_id, title, platform, scenario, type, source_url, path, tags, difficulty, estimated_time, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	// Insert all documents
	for _, entry := range metadataIndex.Documents {
		tagsJSON, err := json.Marshal(entry.Tags)
		if err != nil {
			return fmt.Errorf("failed to marshal tags for %s: %w", entry.DocID, err)
		}

		_, err = stmt.Exec(entry.DocID, entry.Title, entry.Platform, entry.Scenario, entry.Type,
			entry.SourceURL, entry.Path, string(tagsJSON), entry.Difficulty, entry.EstimatedTime)
		if err != nil {
			s.logger.Error("Failed to insert metadata entry", zap.Error(err), zap.String("doc_id", entry.DocID))
			return fmt.Errorf("failed to insert metadata for %s: %w", entry.DocID, err)
		}

		s.logger.Debug("Inserted metadata entry", zap.String("doc_id", entry.DocID))
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	s.logger.Info("Successfully loaded metadata from JSON",
		zap.String("json_path", jsonPath),
		zap.Int("documents_loaded", len(metadataIndex.Documents)))

	return nil
}

// FilterOptions represents filter criteria for metadata queries
type FilterOptions struct {
	Platform   string
	Scenario   string
	Type       string
	Tags       []string
	Difficulty string
	// Support for complex queries
	PlatformIn []string
	ScenarioIn []string
	TypeIn     []string
	AndFilters bool // true for AND, false for OR
}

// FilterDocuments returns document IDs matching the given filters
func (s *Store) FilterDocuments(filters FilterOptions) ([]string, error) {
	s.logger.Debug("Filtering documents", zap.Any("filters", filters))

	var conditions []string
	var args []interface{}

	// Single value filters
	if filters.Platform != "" {
		conditions = append(conditions, "platform = ?")
		args = append(args, filters.Platform)
	}

	if filters.Scenario != "" {
		conditions = append(conditions, "scenario = ?")
		args = append(args, filters.Scenario)
	}

	if filters.Type != "" {
		conditions = append(conditions, "type = ?")
		args = append(args, filters.Type)
	}

	if filters.Difficulty != "" {
		conditions = append(conditions, "difficulty = ?")
		args = append(args, filters.Difficulty)
	}

	// Multi-value filters (IN clauses)
	if len(filters.PlatformIn) > 0 {
		placeholders := make([]string, len(filters.PlatformIn))
		for i, platform := range filters.PlatformIn {
			placeholders[i] = "?"
			args = append(args, platform)
		}
		conditions = append(conditions, "platform IN ("+strings.Join(placeholders, ",")+")")
	}

	if len(filters.ScenarioIn) > 0 {
		placeholders := make([]string, len(filters.ScenarioIn))
		for i, scenario := range filters.ScenarioIn {
			placeholders[i] = "?"
			args = append(args, scenario)
		}
		conditions = append(conditions, "scenario IN ("+strings.Join(placeholders, ",")+")")
	}

	if len(filters.TypeIn) > 0 {
		placeholders := make([]string, len(filters.TypeIn))
		for i, docType := range filters.TypeIn {
			placeholders[i] = "?"
			args = append(args, docType)
		}
		conditions = append(conditions, "type IN ("+strings.Join(placeholders, ",")+")")
	}

	// Tag filters (JSON array search)
	if len(filters.Tags) > 0 {
		tagConditions := make([]string, len(filters.Tags))
		for i, tag := range filters.Tags {
			tagConditions[i] = "json_extract(tags, '$') LIKE ?"
			args = append(args, "%\""+tag+"\"%")
		}

		if filters.AndFilters {
			conditions = append(conditions, "("+strings.Join(tagConditions, " AND ")+")")
		} else {
			conditions = append(conditions, "("+strings.Join(tagConditions, " OR ")+")")
		}
	}

	// Build query
	query := "SELECT doc_id FROM metadata"
	if len(conditions) > 0 {
		operator := " AND "
		if !filters.AndFilters {
			operator = " OR "
		}
		query += " WHERE " + strings.Join(conditions, operator)
	}

	s.logger.Debug("Executing query", zap.String("query", query), zap.Any("args", args))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		s.logger.Error("Failed to query metadata", zap.Error(err))
		return nil, fmt.Errorf("failed to query metadata: %w", err)
	}
	defer rows.Close()

	var docIDs []string
	for rows.Next() {
		var docID string
		if err := rows.Scan(&docID); err != nil {
			s.logger.Error("Failed to scan doc_id", zap.Error(err))
			return nil, fmt.Errorf("failed to scan doc_id: %w", err)
		}
		docIDs = append(docIDs, docID)
	}

	if err := rows.Err(); err != nil {
		s.logger.Error("Error iterating rows", zap.Error(err))
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	s.logger.Debug("Filter results", zap.Strings("doc_ids", docIDs))
	return docIDs, nil
}

// GetAllMetadata returns all metadata entries
func (s *Store) GetAllMetadata() ([]MetadataEntry, error) {
	s.logger.Debug("Getting all metadata entries")

	query := "SELECT doc_id, title, platform, scenario, type, source_url, path, tags, difficulty, estimated_time FROM metadata"

	rows, err := s.db.Query(query)
	if err != nil {
		s.logger.Error("Failed to query all metadata", zap.Error(err))
		return nil, fmt.Errorf("failed to query all metadata: %w", err)
	}
	defer rows.Close()

	var entries []MetadataEntry
	for rows.Next() {
		var entry MetadataEntry
		var tagsJSON string
		err := rows.Scan(&entry.DocID, &entry.Title, &entry.Platform, &entry.Scenario, &entry.Type,
			&entry.SourceURL, &entry.Path, &tagsJSON, &entry.Difficulty, &entry.EstimatedTime)
		if err != nil {
			s.logger.Error("Failed to scan metadata entry", zap.Error(err))
			return nil, fmt.Errorf("failed to scan metadata entry: %w", err)
		}

		// Parse tags JSON
		if err := json.Unmarshal([]byte(tagsJSON), &entry.Tags); err != nil {
			s.logger.Warn("Failed to unmarshal tags", zap.Error(err), zap.String("doc_id", entry.DocID))
			entry.Tags = []string{} // Default to empty array on error
		}

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		s.logger.Error("Error iterating metadata rows", zap.Error(err))
		return nil, fmt.Errorf("error iterating metadata rows: %w", err)
	}

	s.logger.Debug("Retrieved all metadata entries", zap.Int("count", len(entries)))
	return entries, nil
}

// GetMetadataByDocID returns metadata for a specific document ID
func (s *Store) GetMetadataByDocID(docID string) (*MetadataEntry, error) {
	s.logger.Debug("Getting metadata by doc_id", zap.String("doc_id", docID))

	query := "SELECT doc_id, title, platform, scenario, type, source_url, path, tags, difficulty, estimated_time FROM metadata WHERE doc_id = ?"

	row := s.db.QueryRow(query, docID)

	var entry MetadataEntry
	var tagsJSON string
	err := row.Scan(&entry.DocID, &entry.Title, &entry.Platform, &entry.Scenario, &entry.Type,
		&entry.SourceURL, &entry.Path, &tagsJSON, &entry.Difficulty, &entry.EstimatedTime)
	if err != nil {
		if err == sql.ErrNoRows {
			s.logger.Debug("Document not found", zap.String("doc_id", docID))
			return nil, nil // Document not found
		}
		s.logger.Error("Failed to scan metadata", zap.Error(err), zap.String("doc_id", docID))
		return nil, fmt.Errorf("failed to scan metadata: %w", err)
	}

	// Parse tags JSON
	if err := json.Unmarshal([]byte(tagsJSON), &entry.Tags); err != nil {
		s.logger.Warn("Failed to unmarshal tags", zap.Error(err), zap.String("doc_id", docID))
		entry.Tags = []string{} // Default to empty array on error
	}

	s.logger.Debug("Retrieved metadata by doc_id", zap.String("doc_id", docID))
	return &entry, nil
}

// GetStats returns statistics about the metadata store
func (s *Store) GetStats() (map[string]interface{}, error) {
	s.logger.Debug("Getting metadata store statistics")

	stats := make(map[string]interface{})

	// Total documents
	var totalDocs int
	err := s.db.QueryRow("SELECT COUNT(*) FROM metadata").Scan(&totalDocs)
	if err != nil {
		return nil, fmt.Errorf("failed to get total documents: %w", err)
	}
	stats["total_documents"] = totalDocs

	// Documents by platform
	platformStats := make(map[string]int)
	rows, err := s.db.Query("SELECT platform, COUNT(*) FROM metadata GROUP BY platform")
	if err != nil {
		return nil, fmt.Errorf("failed to get platform stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var platform string
		var count int
		if err := rows.Scan(&platform, &count); err != nil {
			return nil, fmt.Errorf("failed to scan platform stats: %w", err)
		}
		platformStats[platform] = count
	}
	stats["by_platform"] = platformStats

	// Documents by scenario
	scenarioStats := make(map[string]int)
	rows, err = s.db.Query("SELECT scenario, COUNT(*) FROM metadata GROUP BY scenario")
	if err != nil {
		return nil, fmt.Errorf("failed to get scenario stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var scenario string
		var count int
		if err := rows.Scan(&scenario, &count); err != nil {
			return nil, fmt.Errorf("failed to scan scenario stats: %w", err)
		}
		scenarioStats[scenario] = count
	}
	stats["by_scenario"] = scenarioStats

	// Documents by type
	typeStats := make(map[string]int)
	rows, err = s.db.Query("SELECT type, COUNT(*) FROM metadata GROUP BY type")
	if err != nil {
		return nil, fmt.Errorf("failed to get type stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var docType string
		var count int
		if err := rows.Scan(&docType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan type stats: %w", err)
		}
		typeStats[docType] = count
	}
	stats["by_type"] = typeStats

	// Last updated
	var lastUpdated string
	err = s.db.QueryRow("SELECT MAX(updated_at) FROM metadata").Scan(&lastUpdated)
	if err != nil {
		return nil, fmt.Errorf("failed to get last updated: %w", err)
	}
	stats["last_updated"] = lastUpdated

	s.logger.Debug("Retrieved metadata store statistics", zap.Any("stats", stats))
	return stats, nil
}

// Migrate performs database migrations
func (s *Store) Migrate() error {
	s.logger.Info("Starting database migration")

	// Get current schema version
	var currentVersion int
	err := s.db.QueryRow("PRAGMA user_version").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("failed to get current schema version: %w", err)
	}

	s.logger.Info("Current schema version", zap.Int("version", currentVersion))

	// Define migrations
	migrations := []func(*sql.DB) error{
		// Migration 1: Add indexes if they don't exist
		func(db *sql.DB) error {
			_, err := db.Exec(`
				CREATE INDEX IF NOT EXISTS idx_platform ON metadata(platform);
				CREATE INDEX IF NOT EXISTS idx_scenario ON metadata(scenario);
				CREATE INDEX IF NOT EXISTS idx_type ON metadata(type);
				CREATE INDEX IF NOT EXISTS idx_difficulty ON metadata(difficulty);
			`)
			return err
		},
	}

	// Apply migrations
	for i := currentVersion; i < len(migrations); i++ {
		s.logger.Info("Applying migration", zap.Int("version", i+1))
		if err := migrations[i](s.db); err != nil {
			return fmt.Errorf("migration %d failed: %w", i+1, err)
		}
	}

	// Update schema version
	if currentVersion < len(migrations) {
		_, err = s.db.Exec(fmt.Sprintf("PRAGMA user_version = %d", len(migrations)))
		if err != nil {
			return fmt.Errorf("failed to update schema version: %w", err)
		}
		s.logger.Info("Schema migration completed", zap.Int("new_version", len(migrations)))
	} else {
		s.logger.Info("Schema is up to date")
	}

	return nil
}
