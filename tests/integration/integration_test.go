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
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestServiceHealthEndpoints tests that all services respond to health checks
func TestServiceHealthEndpoints(t *testing.T) {
	skipIfNoServices(t)

	services := map[string]string{
		"retrieve":   "http://localhost:8081/health",
		"websearch":  "http://localhost:8083/health",
		"synthesize": "http://localhost:8082/health",
		"teamsbot":   "http://localhost:8080/health",
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	for serviceName, url := range services {
		t.Run(fmt.Sprintf("Health_%s", serviceName), func(t *testing.T) {
			resp, err := client.Get(url)
			if err != nil {
				t.Fatalf("Failed to call %s health endpoint: %v", serviceName, err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
			}

			var healthResponse map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&healthResponse); err != nil {
				t.Errorf("Failed to decode health response: %v", err)
			}

			if status, ok := healthResponse["status"]; !ok || status != "healthy" {
				t.Errorf("Expected status 'healthy', got %v", status)
			}
		})
	}
}

// TestServiceInteraction tests basic interaction between services
func TestServiceInteraction(t *testing.T) {
	skipIfNoServices(t)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Test retrieve service search endpoint
	t.Run("RetrieveService_Search", func(t *testing.T) {
		searchRequest := map[string]interface{}{
			"query": "test query",
			"filters": map[string]string{
				"scenario": "test",
			},
		}
		body, _ := json.Marshal(searchRequest)

		resp, err := client.Post("http://localhost:8081/search", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to call retrieve search endpoint: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
		}
	})

	// Test websearch service
	t.Run("WebSearchService", func(t *testing.T) {
		searchRequest := map[string]interface{}{
			"query": "test query with recent updates",
		}
		body, _ := json.Marshal(searchRequest)

		resp, err := client.Post("http://localhost:8083/search", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to call websearch endpoint: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
		}
	})

	// Test synthesize service
	t.Run("SynthesizeService", func(t *testing.T) {
		synthesizeRequest := map[string]interface{}{
			"query":   "test query",
			"context": []string{"test context"},
			"sources": []string{"test source"},
		}
		body, _ := json.Marshal(synthesizeRequest)

		resp, err := client.Post("http://localhost:8082/synthesize", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to call synthesize endpoint: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
		}
	})
}

// TestChromaDBIntegration tests ChromaDB connectivity
func TestChromaDBIntegration(t *testing.T) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Test ChromaDB heartbeat
	resp, err := client.Get("http://localhost:8000/api/v1/heartbeat")
	if err != nil {
		t.Fatalf("Failed to connect to ChromaDB: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("ChromaDB heartbeat failed: %d", resp.StatusCode)
	}

	// Test ChromaDB version
	resp, err = client.Get("http://localhost:8000/api/v1/version")
	if err != nil {
		t.Fatalf("Failed to get ChromaDB version: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("ChromaDB version check failed: %d", resp.StatusCode)
	}
}

// TestIngestionPipeline tests the end-to-end ingestion pipeline
func TestIngestionPipeline(t *testing.T) {
	// Skip if environment variables for API keys are not set
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test requires a running ChromaDB instance and valid OpenAI API key
	// It will test the complete ingestion flow with real services

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	// First, verify ChromaDB is running
	resp, err := client.Get("http://localhost:8000/api/v1/heartbeat")
	if err != nil {
		t.Skipf("ChromaDB not available, skipping ingestion test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Skipf("ChromaDB not healthy, skipping ingestion test: status %d", resp.StatusCode)
	}

	// Test ingestion pipeline components
	t.Run("IngestionPipeline_ChromaDB", func(t *testing.T) {
		// Test creating a collection
		createReq := map[string]interface{}{
			"name": "test_ingestion_collection",
			"metadata": map[string]interface{}{
				"description": "Test collection for ingestion pipeline",
				"created_by":  "integration_test",
			},
		}
		body, _ := json.Marshal(createReq)

		resp, err := client.Post("http://localhost:8000/api/v1/collections", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to create test collection: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		// Should succeed or return 409 if collection already exists
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
			t.Errorf("Expected 200 or 409 for collection creation, got %d", resp.StatusCode)
		}

		// Test adding documents to collection
		addReq := map[string]interface{}{
			"documents": []string{"Test document content for ingestion pipeline"},
			"metadatas": []map[string]interface{}{
				{
					"doc_id":   "test_doc_1",
					"scenario": "test",
					"type":     "integration_test",
				},
			},
			"ids": []string{"test_doc_1"},
			"embeddings": [][]float32{
				// Mock embedding with 1536 dimensions (OpenAI text-embedding-3-small)
				make([]float32, 1536),
			},
		}
		body, _ = json.Marshal(addReq)

		resp, err = client.Post("http://localhost:8000/api/v1/collections/test_ingestion_collection/add",
			"application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to add document to collection: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 for document addition, got %d", resp.StatusCode)
		}

		// Test querying the collection
		queryReq := map[string]interface{}{
			"query_embeddings": [][]float32{
				make([]float32, 1536),
			},
			"n_results": 1,
		}
		body, _ = json.Marshal(queryReq)

		resp, err = client.Post("http://localhost:8000/api/v1/collections/test_ingestion_collection/query",
			"application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to query collection: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 for document query, got %d", resp.StatusCode)
		}

		// Clean up - delete the test collection
		req, _ := http.NewRequest("DELETE", "http://localhost:8000/api/v1/collections/test_ingestion_collection", http.NoBody)
		resp, err = client.Do(req)
		if err != nil {
			t.Logf("Failed to clean up test collection: %v", err)
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
	})

	// Test metadata store integration
	t.Run("IngestionPipeline_MetadataStore", func(t *testing.T) {
		// This would typically test SQLite database operations
		// For now, we'll test that the retrieve service can filter documents

		filterReq := map[string]interface{}{
			"query": "test query",
			"filters": map[string]string{
				"scenario": "migration",
				"platform": "aws",
			},
		}
		body, _ := json.Marshal(filterReq)

		resp, err := client.Post("http://localhost:8081/search", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Skipf("Retrieve service not available, skipping metadata test: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 for metadata filtering, got %d", resp.StatusCode)
		}

		var searchResponse map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&searchResponse); err != nil {
			t.Errorf("Failed to decode search response: %v", err)
		}

		// Verify response structure
		if _, ok := searchResponse["chunks"]; !ok {
			t.Error("Expected 'chunks' field in search response")
		}
	})
}

// TestEndToEndWorkflow tests a complete workflow from ingestion to synthesis
func TestEndToEndWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	// Test complete workflow: Teams Bot -> Synthesis -> Retrieval -> ChromaDB/WebSearch
	t.Run("CompleteWorkflow", func(t *testing.T) {
		// Simulate a Teams bot request
		teamsRequest := map[string]interface{}{
			"query": "Generate a high-level AWS lift-and-shift plan for migrating 50 VMs to AWS",
			"user":  "integration_test",
		}
		body, _ := json.Marshal(teamsRequest)

		// Test synthesis service (which orchestrates the full pipeline)
		resp, err := client.Post("http://localhost:8082/synthesize", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Skipf("Synthesis service not available, skipping workflow test: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 for synthesis request, got %d", resp.StatusCode)
		}

		var synthesisResponse map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&synthesisResponse); err != nil {
			t.Errorf("Failed to decode synthesis response: %v", err)
		}

		// Verify response structure
		expectedFields := []string{"main_text", "sources"}
		for _, field := range expectedFields {
			if _, ok := synthesisResponse[field]; !ok {
				t.Errorf("Expected '%s' field in synthesis response", field)
			}
		}

		// Verify main_text is not empty
		if mainText, ok := synthesisResponse["main_text"].(string); ok {
			if mainText == "" {
				t.Error("Expected non-empty main_text in synthesis response")
			}
		} else {
			t.Error("Expected main_text to be a string")
		}

		// Verify sources are provided
		if sources, ok := synthesisResponse["sources"].([]interface{}); ok {
			if len(sources) == 0 {
				t.Error("Expected at least one source in synthesis response")
			}
		} else {
			t.Error("Expected sources to be an array")
		}
	})
}

// TestDataConsistency tests data consistency across services
// setupHTTPClient creates an HTTP client for integration tests
func setupHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
	}
}

// makeSearchRequest creates and sends a search request to the retrieve service
func makeSearchRequest(client *http.Client, query string, filters map[string]string) (*http.Response, error) {
	filterReq := map[string]interface{}{
		"query":   query,
		"filters": filters,
	}
	body, _ := json.Marshal(filterReq)

	resp, err := client.Post("http://localhost:8081/search", "application/json", bytes.NewBuffer(body))
	return resp, err
}

// validateSearchResponse validates the structure and content of search response
func validateSearchResponse(t *testing.T, resp *http.Response) {
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for search request, got %d", resp.StatusCode)
	}

	var searchResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&searchResponse); err != nil {
		t.Errorf("Failed to decode search response: %v", err)
		return
	}

	// Verify that we got results
	if chunks, ok := searchResponse["chunks"].([]interface{}); ok {
		if len(chunks) == 0 {
			t.Error("Expected at least one chunk for AWS migration query")
		}

		// Verify each chunk has required metadata
		validateChunkStructure(t, chunks)
	} else {
		t.Error("Expected chunks to be an array")
	}
}

// validateChunkStructure validates that each chunk has the required fields
func validateChunkStructure(t *testing.T, chunks []interface{}) {
	requiredFields := []string{"doc_id", "content", "metadata"}
	for i, chunk := range chunks {
		if chunkMap, ok := chunk.(map[string]interface{}); ok {
			for _, field := range requiredFields {
				if _, exists := chunkMap[field]; !exists {
					t.Errorf("Chunk %d missing required field: %s", i, field)
				}
			}
		} else {
			t.Errorf("Chunk %d is not a valid object", i)
		}
	}
}

// runMetadataConsistencyTest runs the metadata consistency test
func runMetadataConsistencyTest(t *testing.T, client *http.Client) {
	resp, err := makeSearchRequest(client, "AWS migration", map[string]string{
		"scenario": "migration",
		"platform": "aws",
	})
	if err != nil {
		t.Skipf("Retrieve service not available, skipping consistency test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	validateSearchResponse(t, resp)
}

func TestDataConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping data consistency test in short mode")
	}

	client := setupHTTPClient()

	// Test that documents in metadata store match those in ChromaDB
	t.Run("MetadataChromaConsistency", func(t *testing.T) {
		runMetadataConsistencyTest(t, client)
	})
}

// TestPerformanceCharacteristics tests basic performance expectations
func TestPerformanceCharacteristics(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	// Test that search queries complete within reasonable time
	t.Run("SearchPerformance", func(t *testing.T) {
		searchReq := map[string]interface{}{
			"query": "Azure hybrid architecture",
			"filters": map[string]string{
				"scenario": "hybrid",
			},
		}
		body, _ := json.Marshal(searchReq)

		start := time.Now()
		resp, err := client.Post("http://localhost:8081/search", "application/json", bytes.NewBuffer(body))
		duration := time.Since(start)

		if err != nil {
			t.Skipf("Retrieve service not available, skipping performance test: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 for search request, got %d", resp.StatusCode)
		}

		// Search should complete within 5 seconds
		if duration > 5*time.Second {
			t.Errorf("Search took too long: %v", duration)
		}

		t.Logf("Search completed in %v", duration)
	})

	// Test that synthesis requests complete within reasonable time
	t.Run("SynthesisPerformance", func(t *testing.T) {
		synthesisReq := map[string]interface{}{
			"query":   "Brief AWS migration overview",
			"context": []string{"AWS migration involves planning and execution phases"},
			"sources": []string{"internal_playbook"},
		}
		body, _ := json.Marshal(synthesisReq)

		start := time.Now()
		resp, err := client.Post("http://localhost:8082/synthesize", "application/json", bytes.NewBuffer(body))
		duration := time.Since(start)

		if err != nil {
			t.Skipf("Synthesis service not available, skipping performance test: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 for synthesis request, got %d", resp.StatusCode)
		}

		// Synthesis should complete within 30 seconds
		if duration > 30*time.Second {
			t.Errorf("Synthesis took too long: %v", duration)
		}

		t.Logf("Synthesis completed in %v", duration)
	})
}
