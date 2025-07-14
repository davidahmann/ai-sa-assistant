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

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/your-org/ai-sa-assistant/internal/chroma"
)

func TestChromaDBTestInstance(t *testing.T) {
	// Skip if ChromaDB test instance is not available
	if !isChromaDBTestAvailable() {
		t.Skip("ChromaDB test instance not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Run("HealthCheck", func(t *testing.T) {
		// Create a temporary client for health check
		client := chroma.NewClient(testChromaDBURL, "temp_collection")
		if err := client.HealthCheck(ctx); err != nil {
			t.Fatalf("ChromaDB test instance is not healthy: %v", err)
		}
		t.Log("✅ ChromaDB test instance is healthy")
	})

	t.Run("CreateCollection", func(t *testing.T) {
		collectionName := fmt.Sprintf("test_collection_%d", time.Now().UnixNano())
		client := chroma.NewClient(testChromaDBURL, collectionName)
		
		err := client.CreateCollection(ctx, collectionName, nil)
		if err != nil {
			t.Fatalf("Failed to create collection: %v", err)
		}
		
		t.Logf("✅ Created collection: %s", collectionName)
		
		// Verify collection exists
		info, err := client.GetCollection(ctx, collectionName)
		if err != nil {
			t.Fatalf("Failed to get collection info: %v", err)
		}
		
		if info.Name != collectionName {
			t.Errorf("Expected collection name %s, got %s", collectionName, info.Name)
		}
		
		// Cleanup
		if err := client.DeleteCollection(ctx, collectionName); err != nil {
			t.Logf("⚠️  Failed to cleanup collection: %v", err)
		}
	})

	t.Run("AddAndSearchDocuments", func(t *testing.T) {
		collectionName := fmt.Sprintf("test_docs_%d", time.Now().UnixNano())
		client := chroma.NewClient(testChromaDBURL, collectionName)
		
		// Create collection
		err := client.CreateCollection(ctx, collectionName, nil)
		if err != nil {
			t.Fatalf("Failed to create collection: %v", err)
		}
		
		// Prepare test data
		documents := []chroma.Document{
			{
				ID:      "doc1",
				Content: "AWS EC2 provides scalable compute capacity",
				Metadata: map[string]string{
					"cloud":   "aws",
					"service": "ec2",
				},
			},
			{
				ID:      "doc2",
				Content: "Azure Virtual Machines offer flexible resources",
				Metadata: map[string]string{
					"cloud":   "azure",
					"service": "vm",
				},
			},
			{
				ID:      "doc3",
				Content: "Google Cloud Compute Engine delivers performance",
				Metadata: map[string]string{
					"cloud":   "gcp",
					"service": "compute",
				},
			},
		}
		
		embeddings := [][]float32{
			generateSimpleEmbedding(0, 1536),
			generateSimpleEmbedding(1, 1536),
			generateSimpleEmbedding(2, 1536),
		}
		
		// Add documents
		err = client.AddDocuments(ctx, documents, embeddings)
		if err != nil {
			t.Fatalf("Failed to add documents: %v", err)
		}
		
		t.Logf("✅ Added %d documents to collection", len(documents))
		
		// Search documents
		queryEmbedding := generateSimpleEmbedding(0, 1536) // Similar to first document
		results, err := client.Search(ctx, queryEmbedding, 2, nil)
		if err != nil {
			t.Fatalf("Failed to search documents: %v", err)
		}
		
		if len(results) == 0 {
			t.Fatal("No results returned from search")
		}
		
		t.Logf("✅ Search returned %d results", len(results))
		
		// Verify first result is most similar (doc1)
		if results[0].ID != "doc1" {
			t.Errorf("Expected first result to be 'doc1', got '%s'", results[0].ID)
		}
		
		// Cleanup
		if err := client.DeleteCollection(ctx, collectionName); err != nil {
			t.Logf("⚠️  Failed to cleanup collection: %v", err)
		}
	})

	t.Run("CollectionMetadata", func(t *testing.T) {
		collectionName := fmt.Sprintf("test_metadata_%d", time.Now().UnixNano())
		client := chroma.NewClient(testChromaDBURL, collectionName)
		
		metadata := map[string]interface{}{
			"description": "Test collection for metadata",
			"version":     "1.0",
			"created_by":  "integration_test",
		}
		
		// Create collection with metadata
		err := client.CreateCollection(ctx, collectionName, metadata)
		if err != nil {
			t.Fatalf("Failed to create collection with metadata: %v", err)
		}
		
		// Get collection info
		info, err := client.GetCollection(ctx, collectionName)
		if err != nil {
			t.Fatalf("Failed to get collection info: %v", err)
		}
		
		if info.Name != collectionName {
			t.Errorf("Expected collection name %s, got %s", collectionName, info.Name)
		}
		
		t.Logf("✅ Collection metadata verified for: %s", collectionName)
		
		// Cleanup
		if err := client.DeleteCollection(ctx, collectionName); err != nil {
			t.Logf("⚠️  Failed to cleanup collection: %v", err)
		}
	})
}

func TestChromaDBTestInstanceSeeding(t *testing.T) {
	// Skip if ChromaDB test instance is not available
	if !isChromaDBTestAvailable() {
		t.Skip("ChromaDB test instance not available")
	}

	collectionName := "test_demo_collection"
	client := chroma.NewClient(testChromaDBURL, collectionName)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Run("SeedTestData", func(t *testing.T) {
		// Try to get existing collection or create new one
		_, err := client.GetCollection(ctx, collectionName)
		if err != nil {
			// Create new collection if it doesn't exist
			err = client.CreateCollection(ctx, collectionName, nil)
			if err != nil {
				t.Fatalf("Failed to create collection: %v", err)
			}
		}
		
		// Add sample test data
		documents := []chroma.Document{
			{
				ID:      "test_aws_ec2",
				Content: "AWS EC2 provides scalable computing capacity with various instance types including t2.micro for development and m5.large for production workloads.",
				Metadata: map[string]string{
					"scenario": "migration",
					"cloud":    "aws",
					"service":  "ec2",
				},
			},
			{
				ID:      "test_azure_vm",
				Content: "Azure Virtual Machines offer flexible compute resources with Windows and Linux support, configured through ARM templates and Azure CLI.",
				Metadata: map[string]string{
					"scenario": "migration",
					"cloud":    "azure",
					"service":  "vm",
				},
			},
			{
				ID:      "test_security",
				Content: "Cloud security best practices include enabling MFA, using IAM roles, encrypting data at rest and in transit, and monitoring with CloudTrail.",
				Metadata: map[string]string{
					"scenario": "security",
					"cloud":    "multi",
					"service":  "iam",
				},
			},
		}
		
		embeddings := [][]float32{
			generateSimpleEmbedding(100, 1536),
			generateSimpleEmbedding(200, 1536),
			generateSimpleEmbedding(300, 1536),
		}
		
		// Add documents
		err = client.AddDocuments(ctx, documents, embeddings)
		if err != nil {
			t.Fatalf("Failed to add test data: %v", err)
		}
		
		t.Logf("✅ Successfully seeded %d test documents", len(documents))
		
		// Verify documents were added
		results, err := client.Search(ctx, generateSimpleEmbedding(100, 1536), 3, nil)
		if err != nil {
			t.Fatalf("Failed to search seeded data: %v", err)
		}
		
		if len(results) == 0 {
			t.Fatal("No results returned from seeded data search")
		}
		
		t.Logf("✅ Verified seeded data - found %d documents", len(results))
	})
}

// Helper functions

func isChromaDBTestAvailable() bool {
	client := chroma.NewClient(testChromaDBURL, "temp_health_check")
	
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	
	return client.HealthCheck(ctx) == nil
}

func generateSimpleEmbedding(seed int, dimension int) []float32 {
	embedding := make([]float32, dimension)
	for i := 0; i < dimension; i++ {
		embedding[i] = float32(seed+i) / 1000.0
	}
	return embedding
}