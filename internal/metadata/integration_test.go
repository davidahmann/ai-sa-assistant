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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

// setupRealMetadataStore creates a store and loads real metadata from file
func setupRealMetadataStore(t *testing.T) (*Store, []Entry) {
	logger := zap.NewNop()
	store, err := NewStore(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Find the actual metadata.json file
	metadataPath := filepath.Join("..", "..", "docs", "metadata.json")
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		t.Skip("metadata.json not found, skipping integration test")
	}

	// Load from real metadata.json
	err = store.LoadFromJSON(metadataPath)
	if err != nil {
		t.Fatalf("Failed to load from real metadata.json: %v", err)
	}

	// Test that data was loaded
	entries, err := store.GetAllMetadata()
	if err != nil {
		t.Fatalf("Failed to get all metadata: %v", err)
	}

	if len(entries) == 0 {
		t.Error("No metadata entries loaded from real file")
	}

	t.Logf("Loaded %d metadata entries from real file", len(entries))
	return store, entries
}

// runFilterTestAndLog runs a filter test and logs the results
func runFilterTestAndLog(t *testing.T, store *Store, testName string, filters FilterOptions) []string {
	docIDs, err := store.FilterDocuments(filters)
	if err != nil {
		t.Fatalf("Failed to filter %s documents: %v", testName, err)
	}
	t.Logf("Found %d %s documents", len(docIDs), testName)
	return docIDs
}

func TestIntegrationWithRealMetadata(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	store, entries := setupRealMetadataStore(t)
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Logf("Failed to close store: %v", closeErr)
		}
	}()

	// Test basic platform filtering
	runFilterTestAndLog(t, store, "AWS", FilterOptions{
		Platform:   "aws",
		AndFilters: true,
	})

	runFilterTestAndLog(t, store, "Azure", FilterOptions{
		Platform:   "azure",
		AndFilters: true,
	})

	// Test scenario filtering
	runFilterTestAndLog(t, store, "migration", FilterOptions{
		Scenario:   "migration",
		AndFilters: true,
	})

	runFilterTestAndLog(t, store, "disaster recovery", FilterOptions{
		Scenario:   "disaster-recovery",
		AndFilters: true,
	})

	// Test complex filtering (AWS AND migration)
	runFilterTestAndLog(t, store, "AWS migration", FilterOptions{
		Platform:   "aws",
		Scenario:   "migration",
		AndFilters: true,
	})

	// Test tag filtering
	runFilterTestAndLog(t, store, "documents with 'migration' tag", FilterOptions{
		Tags:       []string{"migration"},
		AndFilters: true,
	})

	// Test statistics
	stats, err := store.GetStats()
	if err != nil {
		t.Fatalf("Failed to get statistics: %v", err)
	}

	t.Logf("Statistics: %+v", stats)

	// Verify statistics make sense
	if totalDocs, ok := stats["total_documents"].(int); !ok || totalDocs != len(entries) {
		t.Errorf("Total documents in stats (%v) doesn't match entries count (%d)",
			stats["total_documents"], len(entries))
	}

	// Test Demo Scenarios
	runFilterTestAndLog(t, store, "Demo 1 (AWS Lift-and-Shift)", FilterOptions{
		Platform:   "aws",
		Scenario:   "migration",
		AndFilters: true,
	})

	runFilterTestAndLog(t, store, "Demo 2 (Azure Hybrid Architecture)", FilterOptions{
		Platform:   "azure",
		Scenario:   "hybrid",
		AndFilters: true,
	})

	runFilterTestAndLog(t, store, "Demo 3 (Azure Disaster Recovery)", FilterOptions{
		Platform:   "azure",
		Scenario:   "disaster-recovery",
		AndFilters: true,
	})

	runFilterTestAndLog(t, store, "Demo 4 (Security Compliance)", FilterOptions{
		Scenario:   "security-compliance",
		AndFilters: true,
	})

	// Test filtering by document type
	runFilterTestAndLog(t, store, "playbook", FilterOptions{
		Type:       "playbook",
		AndFilters: true,
	})

	runFilterTestAndLog(t, store, "runbook", FilterOptions{
		Type:       "runbook",
		AndFilters: true,
	})

	runFilterTestAndLog(t, store, "SOW", FilterOptions{
		Type:       "sow",
		AndFilters: true,
	})

	// Test filtering by difficulty
	runFilterTestAndLog(t, store, "beginner", FilterOptions{
		Difficulty: "beginner",
		AndFilters: true,
	})

	runFilterTestAndLog(t, store, "intermediate", FilterOptions{
		Difficulty: "intermediate",
		AndFilters: true,
	})

	runFilterTestAndLog(t, store, "advanced", FilterOptions{
		Difficulty: "advanced",
		AndFilters: true,
	})

	// Test specific document retrieval
	for _, docID := range []string{"aws-lift-shift-guide.md", "azure-disaster-recovery.md", "security-compliance.md"} {
		doc, err := store.GetMetadataByDocID(docID)
		if err != nil {
			t.Fatalf("Failed to get document %s: %v", docID, err)
		}

		if doc == nil {
			t.Logf("Document %s not found (this may be expected)", docID)
		} else {
			t.Logf("Retrieved document %s: %s (%s, %s)", docID, doc.Title, doc.Platform, doc.Scenario)
		}
	}
}

func TestIntegrationPersistence(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := zap.NewNop()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "integration_test.db")

	// Create store and add data
	store, err := NewStore(dbPath, logger)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	entry := Entry{
		DocID:         "test-persistence.md",
		Title:         "Test Persistence",
		Platform:      "aws",
		Scenario:      "migration",
		Type:          "playbook",
		Tags:          []string{"test", "persistence"},
		Difficulty:    "intermediate",
		EstimatedTime: "2 hours",
	}

	err = store.AddMetadata(entry)
	if err != nil {
		t.Fatalf("Failed to add metadata: %v", err)
	}

	// Close the store
	err = store.Close()
	if err != nil {
		t.Fatalf("Failed to close store: %v", err)
	}

	// Reopen store and verify data persisted
	store2, err := NewStore(dbPath, logger)
	if err != nil {
		t.Fatalf("Failed to reopen store: %v", err)
	}
	defer func() {
		if closeErr := store2.Close(); closeErr != nil {
			t.Logf("Failed to close store2: %v", closeErr)
		}
	}()

	retrieved, err := store2.GetMetadataByDocID("test-persistence.md")
	if err != nil {
		t.Fatalf("Failed to retrieve persisted data: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Persisted data not found")
	}

	if retrieved.DocID != entry.DocID {
		t.Errorf("Expected DocID '%s', got '%s'", entry.DocID, retrieved.DocID)
	}

	if retrieved.Title != entry.Title {
		t.Errorf("Expected Title '%s', got '%s'", entry.Title, retrieved.Title)
	}

	t.Log("Data persistence test passed")
}

func TestIntegrationMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := zap.NewNop()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "migration_test.db")

	// Create store with initial data
	store, err := NewStore(dbPath, logger)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	entry := Entry{
		DocID:         "migration-test.md",
		Title:         "Migration Test",
		Platform:      "aws",
		Scenario:      "migration",
		Type:          "playbook",
		Tags:          []string{"migration", "test"},
		Difficulty:    "intermediate",
		EstimatedTime: "2 hours",
	}

	err = store.AddMetadata(entry)
	if err != nil {
		t.Fatalf("Failed to add metadata: %v", err)
	}

	// Run migration
	err = store.Migrate()
	if err != nil {
		t.Fatalf("Failed to migrate: %v", err)
	}

	// Verify migration was successful
	var version int
	err = store.db.QueryRow("PRAGMA user_version").Scan(&version)
	if err != nil {
		t.Fatalf("Failed to get schema version: %v", err)
	}

	if version != 1 {
		t.Errorf("Expected schema version 1, got %d", version)
	}

	// Verify data is still accessible after migration
	retrieved, err := store.GetMetadataByDocID("migration-test.md")
	if err != nil {
		t.Fatalf("Failed to retrieve data after migration: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Data not found after migration")
	}

	if closeErr := store.Close(); closeErr != nil {
		t.Logf("Failed to close store: %v", closeErr)
	}
	t.Log("Migration test passed")
}

func TestIntegrationBulkOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

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

	// Create a large dataset
	const numEntries = 1000
	entries := make([]Entry, numEntries)

	for i := 0; i < numEntries; i++ {
		entries[i] = Entry{
			DocID:         fmt.Sprintf("bulk-test-%d.md", i),
			Title:         fmt.Sprintf("Bulk Test Document %d", i),
			Platform:      []string{"aws", "azure", "multi-cloud"}[i%3],
			Scenario:      []string{"migration", "hybrid", "disaster-recovery", "security-compliance"}[i%4],
			Type:          []string{"playbook", "runbook", "sow"}[i%3],
			Tags:          []string{"bulk", "test", fmt.Sprintf("tag-%d", i%10)},
			Difficulty:    []string{"beginner", "intermediate", "advanced"}[i%3],
			EstimatedTime: fmt.Sprintf("%d hours", (i%10)+1),
		}
	}

	// Add all entries
	for _, entry := range entries {
		err = store.AddMetadata(entry)
		if err != nil {
			t.Fatalf("Failed to add bulk entry %s: %v", entry.DocID, err)
		}
	}

	// Verify all entries were added
	allEntries, err := store.GetAllMetadata()
	if err != nil {
		t.Fatalf("Failed to get all entries: %v", err)
	}

	if len(allEntries) != numEntries {
		t.Errorf("Expected %d entries, got %d", numEntries, len(allEntries))
	}

	// Test bulk filtering
	awsEntries, err := store.FilterDocuments(FilterOptions{
		Platform:   "aws",
		AndFilters: true,
	})
	if err != nil {
		t.Fatalf("Failed to filter AWS entries: %v", err)
	}

	expectedAWS := numEntries / 3
	if len(awsEntries) != expectedAWS && len(awsEntries) != expectedAWS+1 {
		t.Errorf("Expected approximately %d AWS entries, got %d", expectedAWS, len(awsEntries))
	}

	// Test statistics with bulk data
	stats, err := store.GetStats()
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	if totalDocs, ok := stats["total_documents"].(int); !ok || totalDocs != numEntries {
		t.Errorf("Expected %d total documents in stats, got %v", numEntries, stats["total_documents"])
	}

	t.Logf("Bulk operations test passed with %d entries", numEntries)
}

func TestIntegrationErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := zap.NewNop()

	// Test with invalid database path
	invalidPath := "/invalid/path/to/database.db"
	_, err := NewStore(invalidPath, logger)
	if err == nil {
		t.Error("Expected error for invalid database path")
	}

	// Test with read-only directory (if possible)
	tmpDir := t.TempDir()
	readOnlyPath := filepath.Join(tmpDir, "readonly")

	err = os.MkdirAll(readOnlyPath, 0750)
	if err != nil {
		t.Fatalf("Failed to create readonly directory: %v", err)
	}

	err = os.Chmod(readOnlyPath, 0400)
	if err != nil {
		t.Logf("Failed to make directory readonly: %v", err)
	} else {
		defer func() {
			_ = os.Chmod(readOnlyPath, 0700) // #nosec G302 - restoring directory permissions for cleanup
		}()

		dbPath := filepath.Join(readOnlyPath, "readonly.db")
		_, err = NewStore(dbPath, logger)
		if err == nil {
			t.Error("Expected error for read-only directory")
		}
	}
}

func TestIntegrationConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

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

	// Ensure schema is initialized by adding a test entry first
	testEntry := Entry{
		DocID:         "init-test.md",
		Title:         "Init Test",
		Platform:      "aws",
		Scenario:      "migration",
		Type:          "playbook",
		Tags:          []string{"init"},
		Difficulty:    "intermediate",
		EstimatedTime: "1 hour",
	}

	err = store.AddMetadata(testEntry)
	if err != nil {
		t.Fatalf("Failed to initialize store: %v", err)
	}

	// Test concurrent operations
	const numGoroutines = 10
	const entriesPerGoroutine = 100

	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer func() { done <- true }()

			for j := 0; j < entriesPerGoroutine; j++ {
				entry := Entry{
					DocID:         fmt.Sprintf("concurrent-%d-%d.md", goroutineID, j),
					Title:         fmt.Sprintf("Concurrent Document %d-%d", goroutineID, j),
					Platform:      "aws",
					Scenario:      "migration",
					Type:          "playbook",
					Tags:          []string{"concurrent", "test"},
					Difficulty:    "intermediate",
					EstimatedTime: "1 hour",
				}

				err := store.AddMetadata(entry)
				if err != nil {
					t.Errorf("Failed to add concurrent entry: %v", err)
					return
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify all entries were added
	allEntries, err := store.GetAllMetadata()
	if err != nil {
		t.Fatalf("Failed to get all entries: %v", err)
	}

	expectedTotal := numGoroutines*entriesPerGoroutine + 1 // +1 for initialization entry
	if len(allEntries) != expectedTotal {
		t.Errorf("Expected %d entries, got %d", expectedTotal, len(allEntries))
	}

	t.Logf("Concurrency test passed with %d entries", len(allEntries))
}
