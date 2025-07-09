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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestNewStore(t *testing.T) {
	logger := zap.NewNop()

	// Test with in-memory database
	store, err := NewStore(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Logf("Failed to close store: %v", closeErr)
		}
	}()

	// Test database schema was created
	var tableName string
	err = store.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='metadata'").Scan(&tableName)
	if err != nil {
		t.Fatalf("Failed to find metadata table: %v", err)
	}

	if tableName != "metadata" {
		t.Errorf("Expected table name 'metadata', got '%s'", tableName)
	}
}

func TestNewStoreWithFile(t *testing.T) {
	logger := zap.NewNop()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath, logger)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Logf("Failed to close store: %v", closeErr)
		}
	}()

	// Test that database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("Database file was not created: %s", dbPath)
	}
}

func TestAddMetadata(t *testing.T) {
	logger := zap.NewNop()
	store, err := NewStore(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Logf("Failed to close store: %v", closeErr)
		}
	}()

	entry := Entry{
		DocID:         "test-doc.md",
		Title:         "Test Document",
		Platform:      "aws",
		Scenario:      "migration",
		Type:          "playbook",
		SourceURL:     "internal",
		Path:          "docs/test-doc.md",
		Tags:          []string{"test", "example"},
		Difficulty:    "intermediate",
		EstimatedTime: "2 hours",
	}

	err = store.AddMetadata(entry)
	if err != nil {
		t.Fatalf("Failed to add metadata: %v", err)
	}

	// Verify the entry was added
	retrieved, err := store.GetMetadataByDocID("test-doc.md")
	if err != nil {
		t.Fatalf("Failed to retrieve metadata: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Retrieved metadata is nil")
	}

	if retrieved.DocID != entry.DocID {
		t.Errorf("Expected DocID '%s', got '%s'", entry.DocID, retrieved.DocID)
	}

	if retrieved.Title != entry.Title {
		t.Errorf("Expected Title '%s', got '%s'", entry.Title, retrieved.Title)
	}

	if len(retrieved.Tags) != len(entry.Tags) {
		t.Errorf("Expected %d tags, got %d", len(entry.Tags), len(retrieved.Tags))
	}
}

func TestLoadFromJSON(t *testing.T) {
	logger := zap.NewNop()
	store, err := NewStore(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Logf("Failed to close store: %v", closeErr)
		}
	}()

	// Create test metadata JSON
	metadataIndex := Index{
		SchemaVersion: "1.0",
		Description:   "Test metadata",
		LastUpdated:   "2024-01-01",
		Documents: []Entry{
			{
				DocID:         "doc1.md",
				Title:         "Document 1",
				Platform:      "aws",
				Scenario:      "migration",
				Type:          "playbook",
				SourceURL:     "internal",
				Path:          "docs/doc1.md",
				Tags:          []string{"aws", "migration"},
				Difficulty:    "intermediate",
				EstimatedTime: "2 hours",
			},
			{
				DocID:         "doc2.md",
				Title:         "Document 2",
				Platform:      "azure",
				Scenario:      "hybrid",
				Type:          "runbook",
				SourceURL:     "internal",
				Path:          "docs/doc2.md",
				Tags:          []string{"azure", "hybrid"},
				Difficulty:    "advanced",
				EstimatedTime: "4 hours",
			},
		},
	}

	// Create temporary JSON file
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "metadata.json")

	jsonData, err := json.Marshal(metadataIndex)
	if err != nil {
		t.Fatalf("Failed to marshal metadata: %v", err)
	}

	err = os.WriteFile(jsonPath, jsonData, 0600)
	if err != nil {
		t.Fatalf("Failed to write JSON file: %v", err)
	}

	// Load from JSON
	err = store.LoadFromJSON(jsonPath)
	if err != nil {
		t.Fatalf("Failed to load from JSON: %v", err)
	}

	// Verify entries were loaded
	entries, err := store.GetAllMetadata()
	if err != nil {
		t.Fatalf("Failed to get all metadata: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(entries))
	}

	// Verify specific entry
	doc1, err := store.GetMetadataByDocID("doc1.md")
	if err != nil {
		t.Fatalf("Failed to get doc1: %v", err)
	}

	if doc1 == nil {
		t.Fatal("doc1 is nil")
	}

	if doc1.Platform != "aws" {
		t.Errorf("Expected platform 'aws', got '%s'", doc1.Platform)
	}
}

// setupTestStoreForFiltering creates a test store with predefined test data
func setupTestStoreForFiltering(t *testing.T) *Store {
	logger := zap.NewNop()
	store, err := NewStore(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	testEntries := []Entry{
		{
			DocID:         "aws-migration.md",
			Title:         "AWS Migration Guide",
			Platform:      "aws",
			Scenario:      "migration",
			Type:          "playbook",
			SourceURL:     "internal",
			Path:          "docs/aws-migration.md",
			Tags:          []string{"aws", "migration", "ec2"},
			Difficulty:    "intermediate",
			EstimatedTime: "4 hours",
		},
		{
			DocID:         "azure-hybrid.md",
			Title:         "Azure Hybrid Guide",
			Platform:      "azure",
			Scenario:      "hybrid",
			Type:          "playbook",
			SourceURL:     "internal",
			Path:          "docs/azure-hybrid.md",
			Tags:          []string{"azure", "hybrid", "expressroute"},
			Difficulty:    "advanced",
			EstimatedTime: "6 hours",
		},
		{
			DocID:         "aws-security.md",
			Title:         "AWS Security Guide",
			Platform:      "aws",
			Scenario:      "security-compliance",
			Type:          "runbook",
			SourceURL:     "internal",
			Path:          "docs/aws-security.md",
			Tags:          []string{"aws", "security", "compliance"},
			Difficulty:    "advanced",
			EstimatedTime: "3 hours",
		},
	}

	for _, entry := range testEntries {
		err = store.AddMetadata(entry)
		if err != nil {
			t.Fatalf("Failed to add metadata for %s: %v", entry.DocID, err)
		}
	}

	return store
}

// validateDocumentIDsExact checks if the returned document IDs match expected ones exactly
func validateDocumentIDsExact(t *testing.T, docIDs []string, expectedIDs []string) {
	if len(docIDs) != len(expectedIDs) {
		t.Errorf("Expected %d documents, got %d", len(expectedIDs), len(docIDs))
		return
	}

	expectedMap := make(map[string]bool)
	for _, id := range expectedIDs {
		expectedMap[id] = true
	}

	for _, docID := range docIDs {
		if !expectedMap[docID] {
			t.Errorf("Unexpected document ID: %s", docID)
		}
	}
}

// validateSingleDocumentID checks if exactly one document with the expected ID is returned
func validateSingleDocumentID(t *testing.T, docIDs []string, expectedID string) {
	if len(docIDs) != 1 {
		t.Errorf("Expected 1 document, got %d", len(docIDs))
		return
	}

	if docIDs[0] != expectedID {
		t.Errorf("Expected '%s', got '%s'", expectedID, docIDs[0])
	}
}

// validateDocumentCount checks if the number of returned documents matches expected count
func validateDocumentCount(t *testing.T, docIDs []string, expectedCount int) {
	if len(docIDs) != expectedCount {
		t.Errorf("Expected %d documents, got %d", expectedCount, len(docIDs))
	}
}

// runFilterTest executes a filter test and validates the results
func runFilterTest(t *testing.T, store *Store, filters FilterOptions, validator func([]string)) {
	docIDs, err := store.FilterDocuments(filters)
	if err != nil {
		t.Fatalf("Failed to filter documents: %v", err)
	}
	validator(docIDs)
}

func TestFilterDocuments(t *testing.T) {
	store := setupTestStoreForFiltering(t)
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Logf("Failed to close store: %v", closeErr)
		}
	}()

	// Test filtering by platform
	t.Run("FilterByPlatform", func(t *testing.T) {
		filters := FilterOptions{
			Platform:   "aws",
			AndFilters: true,
		}

		runFilterTest(t, store, filters, func(docIDs []string) {
			validateDocumentIDsExact(t, docIDs, []string{"aws-migration.md", "aws-security.md"})
		})
	})

	// Test filtering by scenario
	t.Run("FilterByScenario", func(t *testing.T) {
		filters := FilterOptions{
			Scenario:   "migration",
			AndFilters: true,
		}

		runFilterTest(t, store, filters, func(docIDs []string) {
			validateSingleDocumentID(t, docIDs, "aws-migration.md")
		})
	})

	// Test filtering by type
	t.Run("FilterByType", func(t *testing.T) {
		filters := FilterOptions{
			Type:       "playbook",
			AndFilters: true,
		}

		runFilterTest(t, store, filters, func(docIDs []string) {
			validateDocumentCount(t, docIDs, 2)
		})
	})

	// Test filtering by tags
	t.Run("FilterByTags", func(t *testing.T) {
		filters := FilterOptions{
			Tags:       []string{"migration"},
			AndFilters: true,
		}

		runFilterTest(t, store, filters, func(docIDs []string) {
			validateSingleDocumentID(t, docIDs, "aws-migration.md")
		})
	})

	// Test complex filtering (AND combination)
	t.Run("FilterComplexAND", func(t *testing.T) {
		filters := FilterOptions{
			Platform:   "aws",
			Scenario:   "migration",
			AndFilters: true,
		}

		runFilterTest(t, store, filters, func(docIDs []string) {
			validateSingleDocumentID(t, docIDs, "aws-migration.md")
		})
	})

	// Test IN filters
	t.Run("FilterWithIN", func(t *testing.T) {
		filters := FilterOptions{
			PlatformIn: []string{"aws", "azure"},
			AndFilters: true,
		}

		runFilterTest(t, store, filters, func(docIDs []string) {
			validateDocumentCount(t, docIDs, 3)
		})
	})

	// Test filtering by difficulty
	t.Run("FilterByDifficulty", func(t *testing.T) {
		filters := FilterOptions{
			Difficulty: "advanced",
			AndFilters: true,
		}

		runFilterTest(t, store, filters, func(docIDs []string) {
			validateDocumentCount(t, docIDs, 2)
		})
	})
}

func TestGetMetadataByDocID(t *testing.T) {
	logger := zap.NewNop()
	store, err := NewStore(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Logf("Failed to close store: %v", closeErr)
		}
	}()

	entry := Entry{
		DocID:         "test-doc.md",
		Title:         "Test Document",
		Platform:      "aws",
		Scenario:      "migration",
		Type:          "playbook",
		SourceURL:     "internal",
		Path:          "docs/test-doc.md",
		Tags:          []string{"test", "example"},
		Difficulty:    "intermediate",
		EstimatedTime: "2 hours",
	}

	err = store.AddMetadata(entry)
	if err != nil {
		t.Fatalf("Failed to add metadata: %v", err)
	}

	// Test existing document
	retrieved, err := store.GetMetadataByDocID("test-doc.md")
	if err != nil {
		t.Fatalf("Failed to retrieve metadata: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Retrieved metadata is nil")
	}

	if retrieved.DocID != entry.DocID {
		t.Errorf("Expected DocID '%s', got '%s'", entry.DocID, retrieved.DocID)
	}

	// Test non-existing document
	notFound, err := store.GetMetadataByDocID("non-existent.md")
	if err != nil {
		t.Fatalf("Failed to query non-existent document: %v", err)
	}

	if notFound != nil {
		t.Error("Expected nil for non-existent document")
	}
}

func TestGetAllMetadata(t *testing.T) {
	logger := zap.NewNop()
	store, err := NewStore(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Logf("Failed to close store: %v", closeErr)
		}
	}()

	// Add test data
	testEntries := []Entry{
		{
			DocID:         "doc1.md",
			Title:         "Document 1",
			Platform:      "aws",
			Scenario:      "migration",
			Type:          "playbook",
			Tags:          []string{"aws", "migration"},
			Difficulty:    "intermediate",
			EstimatedTime: "2 hours",
		},
		{
			DocID:         "doc2.md",
			Title:         "Document 2",
			Platform:      "azure",
			Scenario:      "hybrid",
			Type:          "runbook",
			Tags:          []string{"azure", "hybrid"},
			Difficulty:    "advanced",
			EstimatedTime: "4 hours",
		},
	}

	for _, entry := range testEntries {
		err = store.AddMetadata(entry)
		if err != nil {
			t.Fatalf("Failed to add metadata: %v", err)
		}
	}

	// Get all metadata
	entries, err := store.GetAllMetadata()
	if err != nil {
		t.Fatalf("Failed to get all metadata: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(entries))
	}

	// Verify entries are correct
	found := make(map[string]bool)
	for _, entry := range entries {
		found[entry.DocID] = true
	}

	if !found["doc1.md"] {
		t.Error("doc1.md not found in results")
	}

	if !found["doc2.md"] {
		t.Error("doc2.md not found in results")
	}
}

func TestGetStats(t *testing.T) {
	logger := zap.NewNop()
	store, err := NewStore(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Logf("Failed to close store: %v", closeErr)
		}
	}()

	// Add test data
	testEntries := []Entry{
		{
			DocID:         "aws-doc1.md",
			Title:         "AWS Document 1",
			Platform:      "aws",
			Scenario:      "migration",
			Type:          "playbook",
			Tags:          []string{"aws", "migration"},
			Difficulty:    "intermediate",
			EstimatedTime: "2 hours",
		},
		{
			DocID:         "aws-doc2.md",
			Title:         "AWS Document 2",
			Platform:      "aws",
			Scenario:      "security-compliance",
			Type:          "runbook",
			Tags:          []string{"aws", "security"},
			Difficulty:    "advanced",
			EstimatedTime: "3 hours",
		},
		{
			DocID:         "azure-doc1.md",
			Title:         "Azure Document 1",
			Platform:      "azure",
			Scenario:      "hybrid",
			Type:          "playbook",
			Tags:          []string{"azure", "hybrid"},
			Difficulty:    "advanced",
			EstimatedTime: "4 hours",
		},
	}

	for _, entry := range testEntries {
		err = store.AddMetadata(entry)
		if err != nil {
			t.Fatalf("Failed to add metadata: %v", err)
		}
	}

	// Get statistics
	stats, err := store.GetStats()
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	// Verify total documents
	if totalDocs, ok := stats["total_documents"].(int); !ok || totalDocs != 3 {
		t.Errorf("Expected 3 total documents, got %v", stats["total_documents"])
	}

	// Verify platform stats
	if platformStats, ok := stats["by_platform"].(map[string]int); ok {
		if platformStats["aws"] != 2 {
			t.Errorf("Expected 2 AWS documents, got %d", platformStats["aws"])
		}
		if platformStats["azure"] != 1 {
			t.Errorf("Expected 1 Azure document, got %d", platformStats["azure"])
		}
	} else {
		t.Error("by_platform stats not found or invalid type")
	}

	// Verify scenario stats
	if scenarioStats, ok := stats["by_scenario"].(map[string]int); ok {
		if scenarioStats["migration"] != 1 {
			t.Errorf("Expected 1 migration document, got %d", scenarioStats["migration"])
		}
		if scenarioStats["security-compliance"] != 1 {
			t.Errorf("Expected 1 security-compliance document, got %d", scenarioStats["security-compliance"])
		}
		if scenarioStats["hybrid"] != 1 {
			t.Errorf("Expected 1 hybrid document, got %d", scenarioStats["hybrid"])
		}
	} else {
		t.Error("by_scenario stats not found or invalid type")
	}

	// Verify type stats
	if typeStats, ok := stats["by_type"].(map[string]int); ok {
		if typeStats["playbook"] != 2 {
			t.Errorf("Expected 2 playbook documents, got %d", typeStats["playbook"])
		}
		if typeStats["runbook"] != 1 {
			t.Errorf("Expected 1 runbook document, got %d", typeStats["runbook"])
		}
	} else {
		t.Error("by_type stats not found or invalid type")
	}
}

func TestMigrate(t *testing.T) {
	logger := zap.NewNop()
	store, err := NewStore(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Logf("Failed to close store: %v", closeErr)
		}
	}()

	// Run migration
	err = store.Migrate()
	if err != nil {
		t.Fatalf("Failed to migrate: %v", err)
	}

	// Verify schema version was updated
	var version int
	err = store.db.QueryRow("PRAGMA user_version").Scan(&version)
	if err != nil {
		t.Fatalf("Failed to get schema version: %v", err)
	}

	if version != 1 {
		t.Errorf("Expected schema version 1, got %d", version)
	}

	// Verify indexes were created
	var indexCount int
	err = store.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name LIKE 'idx_%'").
		Scan(&indexCount)
	if err != nil {
		t.Fatalf("Failed to count indexes: %v", err)
	}

	if indexCount < 4 {
		t.Errorf("Expected at least 4 indexes, got %d", indexCount)
	}
}

func TestEmptyFilters(t *testing.T) {
	logger := zap.NewNop()
	store, err := NewStore(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Logf("Failed to close store: %v", closeErr)
		}
	}()

	// Add test data
	entry := Entry{
		DocID:         "test-doc.md",
		Title:         "Test Document",
		Platform:      "aws",
		Scenario:      "migration",
		Type:          "playbook",
		Tags:          []string{"test"},
		Difficulty:    "intermediate",
		EstimatedTime: "2 hours",
	}

	err = store.AddMetadata(entry)
	if err != nil {
		t.Fatalf("Failed to add metadata: %v", err)
	}

	// Test empty filters (should return all documents)
	filters := FilterOptions{
		AndFilters: true,
	}

	docIDs, err := store.FilterDocuments(filters)
	if err != nil {
		t.Fatalf("Failed to filter documents: %v", err)
	}

	if len(docIDs) != 1 {
		t.Errorf("Expected 1 document, got %d", len(docIDs))
	}

	if docIDs[0] != "test-doc.md" {
		t.Errorf("Expected 'test-doc.md', got '%s'", docIDs[0])
	}
}

func TestInvalidJSONPath(t *testing.T) {
	logger := zap.NewNop()
	store, err := NewStore(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Logf("Failed to close store: %v", closeErr)
		}
	}()

	// Test with non-existent file
	err = store.LoadFromJSON("/non/existent/file.json")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestInvalidJSON(t *testing.T) {
	logger := zap.NewNop()
	store, err := NewStore(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Logf("Failed to close store: %v", closeErr)
		}
	}()

	// Create temporary file with invalid JSON
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "invalid.json")

	err = os.WriteFile(jsonPath, []byte("invalid json content"), 0600)
	if err != nil {
		t.Fatalf("Failed to write invalid JSON file: %v", err)
	}

	// Test loading invalid JSON
	err = store.LoadFromJSON(jsonPath)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestCloseStore(t *testing.T) {
	logger := zap.NewNop()
	store, err := NewStore(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Test closing store
	err = store.Close()
	if err != nil {
		t.Fatalf("Failed to close store: %v", err)
	}

	// Test that operations fail after close
	entry := Entry{
		DocID:    "test-doc.md",
		Title:    "Test Document",
		Platform: "aws",
		Scenario: "migration",
		Type:     "playbook",
		Tags:     []string{"test"},
	}

	err = store.AddMetadata(entry)
	if err == nil {
		t.Error("Expected error when adding metadata to closed store")
	}
}
