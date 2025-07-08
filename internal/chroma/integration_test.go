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

//go:build integration
// +build integration

package chroma

import (
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"
)

const (
	testChromaURL     = "http://localhost:8000"
	testCollection    = "integration-test-collection"
	cleanupCollection = "cleanup-test-collection"
)

// skipIfChromaUnavailable checks if ChromaDB is available and skips the test if not
func skipIfChromaUnavailable(t *testing.T) *Client {
	client := NewClientWithOptions(testChromaURL, testCollection, zap.NewNop(), 1, 100*time.Millisecond)

	if err := client.HealthCheck(); err != nil {
		t.Skipf("ChromaDB not available at %s: %v", testChromaURL, err)
	}

	return client
}

// cleanupCollection removes test collections
func cleanupTestCollection(t *testing.T, client *Client, collectionName string) {
	if err := client.DeleteCollection(collectionName); err != nil {
		t.Logf("Warning: failed to cleanup collection %s: %v", collectionName, err)
	}
}

func TestIntegration_HealthCheck(t *testing.T) {
	client := skipIfChromaUnavailable(t)

	err := client.HealthCheck()
	if err != nil {
		t.Errorf("Health check failed: %v", err)
	}
}

func TestIntegration_CollectionManagement(t *testing.T) {
	client := skipIfChromaUnavailable(t)

	collectionName := fmt.Sprintf("%s-%d", cleanupCollection, time.Now().Unix())
	defer cleanupTestCollection(t, client, collectionName)

	// Test create collection
	metadata := map[string]interface{}{
		"description": "Integration test collection",
		"created_at":  time.Now().Format(time.RFC3339),
	}

	err := client.CreateCollection(collectionName, metadata)
	if err != nil {
		t.Fatalf("Failed to create collection: %v", err)
	}

	// Test get collection
	collection, err := client.GetCollection(collectionName)
	if err != nil {
		t.Fatalf("Failed to get collection: %v", err)
	}

	if collection.Name != collectionName {
		t.Errorf("Expected collection name %s, got %s", collectionName, collection.Name)
	}

	if collection.Metadata["description"] != "Integration test collection" {
		t.Errorf("Expected description metadata, got %v", collection.Metadata)
	}

	// Test list collections
	collections, err := client.ListCollections()
	if err != nil {
		t.Fatalf("Failed to list collections: %v", err)
	}

	found := false
	for _, coll := range collections {
		if coll.Name == collectionName {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Created collection %s not found in list", collectionName)
	}

	// Test delete collection
	err = client.DeleteCollection(collectionName)
	if err != nil {
		t.Fatalf("Failed to delete collection: %v", err)
	}

	// Verify collection is deleted
	_, err = client.GetCollection(collectionName)
	if err == nil {
		t.Error("Expected error when getting deleted collection, got nil")
	}
}

func TestIntegration_DocumentOperations(t *testing.T) {
	client := skipIfChromaUnavailable(t)

	collectionName := fmt.Sprintf("%s-%d", testCollection, time.Now().Unix())

	// Create test collection
	err := client.CreateCollection(collectionName, map[string]interface{}{
		"description": "Document operations test",
	})
	if err != nil {
		t.Fatalf("Failed to create collection: %v", err)
	}
	defer cleanupTestCollection(t, client, collectionName)

	// Create client for the new collection
	docClient := NewClientWithOptions(testChromaURL, collectionName, zap.NewNop(), 3, 100*time.Millisecond)

	// Test add documents
	documents := []Document{
		{
			ID:      "doc1",
			Content: "This is a test document about artificial intelligence",
			Metadata: map[string]string{
				"category": "ai",
				"doc_id":   "doc1",
			},
		},
		{
			ID:      "doc2",
			Content: "This is a test document about machine learning algorithms",
			Metadata: map[string]string{
				"category": "ml",
				"doc_id":   "doc2",
			},
		},
		{
			ID:      "doc3",
			Content: "This is a test document about cloud computing services",
			Metadata: map[string]string{
				"category": "cloud",
				"doc_id":   "doc3",
			},
		},
	}

	embeddings := [][]float32{
		{0.1, 0.2, 0.3, 0.4, 0.5},
		{0.2, 0.3, 0.4, 0.5, 0.6},
		{0.3, 0.4, 0.5, 0.6, 0.7},
	}

	err = docClient.AddDocuments(documents, embeddings)
	if err != nil {
		t.Fatalf("Failed to add documents: %v", err)
	}

	// Give ChromaDB a moment to index the documents
	time.Sleep(500 * time.Millisecond)

	// Test search without filters
	queryEmbedding := []float32{0.15, 0.25, 0.35, 0.45, 0.55}
	results, err := docClient.Search(queryEmbedding, 3, nil)
	if err != nil {
		t.Fatalf("Failed to search documents: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected at least one search result, got none")
	}

	// Verify results have required fields
	for i, result := range results {
		if result.ID == "" {
			t.Errorf("Result %d missing ID", i)
		}
		if result.Content == "" {
			t.Errorf("Result %d missing content", i)
		}
		if result.Distance < 0 {
			t.Errorf("Result %d has negative distance: %f", i, result.Distance)
		}
		if result.Metadata == nil {
			t.Errorf("Result %d missing metadata", i)
		}
	}

	// Test search with document ID filter
	docIDFilter := []string{"doc1", "doc3"}
	filteredResults, err := docClient.Search(queryEmbedding, 2, docIDFilter)
	if err != nil {
		t.Fatalf("Failed to search with doc ID filter: %v", err)
	}

	if len(filteredResults) > 2 {
		t.Errorf("Expected at most 2 results with filter, got %d", len(filteredResults))
	}

	// Verify filtered results only contain requested doc IDs
	for _, result := range filteredResults {
		if result.ID != "doc1" && result.ID != "doc3" {
			t.Errorf("Filtered result contains unexpected doc ID: %s", result.ID)
		}
	}

	// Test search with small result limit
	limitedResults, err := docClient.Search(queryEmbedding, 1, nil)
	if err != nil {
		t.Fatalf("Failed to search with result limit: %v", err)
	}

	if len(limitedResults) != 1 {
		t.Errorf("Expected exactly 1 result with limit, got %d", len(limitedResults))
	}
}

func TestIntegration_ErrorHandling(t *testing.T) {
	client := skipIfChromaUnavailable(t)

	// Test getting non-existent collection
	_, err := client.GetCollection("non-existent-collection")
	if err == nil {
		t.Error("Expected error when getting non-existent collection, got nil")
	}

	// Test deleting non-existent collection
	err = client.DeleteCollection("non-existent-collection")
	if err == nil {
		t.Error("Expected error when deleting non-existent collection, got nil")
	}

	// Test searching in non-existent collection
	nonExistentClient := NewClientWithOptions(testChromaURL, "non-existent-collection", zap.NewNop(), 1, 100*time.Millisecond)

	queryEmbedding := []float32{0.1, 0.2, 0.3}
	_, err = nonExistentClient.Search(queryEmbedding, 1, nil)
	if err == nil {
		t.Error("Expected error when searching non-existent collection, got nil")
	}

	// Test adding documents to non-existent collection
	documents := []Document{{ID: "test", Content: "test"}}
	embeddings := [][]float32{{0.1, 0.2, 0.3}}

	err = nonExistentClient.AddDocuments(documents, embeddings)
	if err == nil {
		t.Error("Expected error when adding documents to non-existent collection, got nil")
	}
}

func TestIntegration_RetryMechanism(t *testing.T) {
	// Test with invalid URL to trigger retries
	invalidClient := NewClientWithOptions("http://invalid-host:9999", "test", zap.NewNop(), 2, 50*time.Millisecond)

	start := time.Now()
	err := invalidClient.HealthCheck()
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected health check to fail with invalid host, got nil error")
	}

	// Should have taken time for retries (at least 50ms base + 100ms for first retry)
	if duration < 100*time.Millisecond {
		t.Errorf("Expected retry mechanism to take at least 100ms, took %v", duration)
	}
}

func TestIntegration_ConcurrentOperations(t *testing.T) {
	client := skipIfChromaUnavailable(t)

	collectionName := fmt.Sprintf("%s-concurrent-%d", testCollection, time.Now().Unix())

	// Create test collection
	err := client.CreateCollection(collectionName, map[string]interface{}{
		"description": "Concurrent operations test",
	})
	if err != nil {
		t.Fatalf("Failed to create collection: %v", err)
	}
	defer cleanupTestCollection(t, client, collectionName)

	docClient := NewClientWithOptions(testChromaURL, collectionName, zap.NewNop(), 3, 100*time.Millisecond)

	// Add initial documents
	documents := []Document{
		{ID: "doc1", Content: "Document 1", Metadata: map[string]string{"doc_id": "doc1"}},
		{ID: "doc2", Content: "Document 2", Metadata: map[string]string{"doc_id": "doc2"}},
	}
	embeddings := [][]float32{{0.1, 0.2}, {0.3, 0.4}}

	err = docClient.AddDocuments(documents, embeddings)
	if err != nil {
		t.Fatalf("Failed to add initial documents: %v", err)
	}

	time.Sleep(200 * time.Millisecond) // Let documents index

	// Perform concurrent searches
	done := make(chan error, 5)
	queryEmbedding := []float32{0.2, 0.3}

	for i := 0; i < 5; i++ {
		go func() {
			_, err := docClient.Search(queryEmbedding, 2, nil)
			done <- err
		}()
	}

	// Wait for all searches to complete
	for i := 0; i < 5; i++ {
		if err := <-done; err != nil {
			t.Errorf("Concurrent search %d failed: %v", i, err)
		}
	}
}
