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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/your-org/ai-sa-assistant/internal/chroma"
	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/metadata"
	"go.uber.org/zap"
)

// RetrievalAPITestSuite manages the test environment for retrieval API integration tests
type RetrievalAPITestSuite struct {
	TestDataManager     *TestDataManager
	HealthChecker       *ServiceHealthChecker
	ResultValidator     *TestResultValidator
	FSHelper            *FileSystemHelper
	MetadataStore       *metadata.Store
	ChromaClient        *chroma.Client
	Config              *config.Config
	Logger              *zap.Logger
	RetrievalServiceURL string
	TestTimeout         time.Duration
}

// SearchRequest represents the JSON payload for search requests
type SearchRequest struct {
	Query   string                 `json:"query"`
	Filters map[string]interface{} `json:"filters,omitempty"`
}

// SearchChunk represents a single search result chunk
type SearchChunk struct {
	Text     string                 `json:"text"`
	Score    float64                `json:"score"`
	DocID    string                 `json:"doc_id"`
	SourceID string                 `json:"source_id"`
	Metadata map[string]interface{} `json:"metadata"`
}

// SearchResponse represents the JSON response for search requests
type SearchResponse struct {
	Chunks            []SearchChunk `json:"chunks"`
	Count             int           `json:"count"`
	Query             string        `json:"query"`
	FallbackTriggered bool          `json:"fallback_triggered"`
	FallbackReason    string        `json:"fallback_reason,omitempty"`
}

// TestMain sets up the test environment and runs the tests
func TestMain(m *testing.M) {
	// Check if we should skip integration tests (check command line args)
	for _, arg := range os.Args {
		if arg == "-test.short" {
			fmt.Println("Skipping integration tests in short mode")
			os.Exit(0)
		}
	}

	// Check if required services are available
	if !servicesAvailable() {
		fmt.Println("Required services not available - skipping integration tests")
		fmt.Println("To run integration tests:")
		fmt.Println("  1. Start services: docker-compose up -d")
		fmt.Println("  2. Or use test environment: docker-compose -f docker-compose.test.yml up -d")
		fmt.Println("  3. Run tests: go test -tags=integration ./tests/integration/...")
		os.Exit(0)
	}

	// Start ChromaDB container if not already running
	if err := ensureChromaDBRunning(); err != nil {
		fmt.Printf("Failed to start ChromaDB: %v\n", err)
		os.Exit(1)
	}

	// Run the tests
	exitCode := m.Run()

	// Clean up test environment
	cleanupTestEnvironment()

	os.Exit(exitCode)
}

// TestRetrievalAPIIntegration is the main integration test for the retrieval API
func TestRetrievalAPIIntegration(t *testing.T) {
	suite := setupTestSuite(t)
	defer suite.teardown(t)

	// Wait for services to be healthy
	if !suite.HealthChecker.WaitForServices(t, 60*time.Second) {
		t.Fatal("Services failed to become healthy within timeout")
	}

	// Set up test data
	suite.TestDataManager.SetupTestData(t)
	defer suite.TestDataManager.TeardownTestData(t)

	// Run all test scenarios
	t.Run("UnfilteredSearch", suite.testUnfilteredSearch)
	t.Run("FilteredSearch", suite.testFilteredSearch)
	t.Run("FallbackLogic", suite.testFallbackLogic)
	t.Run("ResponseFormat", suite.testResponseFormat)
	t.Run("ErrorHandling", suite.testErrorHandling)
	t.Run("Performance", suite.testPerformance)
}

// testUnfilteredSearch tests the unfiltered search functionality
func (suite *RetrievalAPITestSuite) testUnfilteredSearch(t *testing.T) {
	searchReq := SearchRequest{
		Query: "AWS migration best practices",
	}

	response, err := suite.performSearch(searchReq)
	if err != nil {
		t.Fatalf("Unfiltered search failed: %v", err)
	}

	// Validate response
	if response.Count == 0 {
		t.Error("Expected at least one result from unfiltered search")
	}

	if response.Query != searchReq.Query {
		t.Errorf("Expected query %s, got %s", searchReq.Query, response.Query)
	}

	if response.FallbackTriggered {
		t.Error("Fallback should not be triggered for unfiltered search")
	}

	// Validate chunks
	for i, chunk := range response.Chunks {
		if chunk.Text == "" {
			t.Errorf("Chunk %d has empty text", i)
		}
		if chunk.Score <= 0 || chunk.Score > 1 {
			t.Errorf("Chunk %d has invalid score: %f", i, chunk.Score)
		}
		if chunk.DocID == "" {
			t.Errorf("Chunk %d has empty doc_id", i)
		}
		if chunk.SourceID == "" {
			t.Errorf("Chunk %d has empty source_id", i)
		}
	}

	t.Logf("Unfiltered search returned %d chunks", response.Count)
}

// testFilteredSearch tests the filtered search with valid metadata filters
func (suite *RetrievalAPITestSuite) testFilteredSearch(t *testing.T) {
	// Test with scenario filter
	searchReq := SearchRequest{
		Query: "migration planning",
		Filters: map[string]interface{}{
			"scenario": "migration",
			"cloud":    "aws",
		},
	}

	response, err := suite.performSearch(searchReq)
	if err != nil {
		t.Fatalf("Filtered search failed: %v", err)
	}

	// Validate that results are filtered correctly
	if response.Count == 0 {
		t.Error("Expected at least one result from filtered search")
	}

	// Validate that all chunks match the filter criteria
	for i, chunk := range response.Chunks {
		scenario, ok := chunk.Metadata["scenario"]
		if !ok {
			t.Errorf("Chunk %d missing scenario metadata", i)
			continue
		}
		if scenario != "migration" {
			t.Errorf("Chunk %d has unexpected scenario: %v", i, scenario)
		}

		cloud, ok := chunk.Metadata["cloud"]
		if !ok {
			t.Errorf("Chunk %d missing cloud metadata", i)
			continue
		}
		if cloud != "aws" {
			t.Errorf("Chunk %d has unexpected cloud: %v", i, cloud)
		}
	}

	t.Logf("Filtered search returned %d chunks matching filters", response.Count)
}

// testFallbackLogic tests the fallback logic with filters that yield insufficient results
func (suite *RetrievalAPITestSuite) testFallbackLogic(t *testing.T) {
	// Use filters that should yield insufficient results to trigger fallback
	searchReq := SearchRequest{
		Query: "cloud architecture",
		Filters: map[string]interface{}{
			"scenario": "nonexistent-scenario",
			"cloud":    "nonexistent-cloud",
		},
	}

	response, err := suite.performSearch(searchReq)
	if err != nil {
		t.Fatalf("Fallback logic test failed: %v", err)
	}

	// Validate that fallback was triggered
	if !response.FallbackTriggered {
		t.Error("Expected fallback to be triggered for insufficient results")
	}

	if response.FallbackReason == "" {
		t.Error("Expected fallback reason to be provided")
	}

	// Validate that fallback search returned results
	if response.Count == 0 {
		t.Error("Expected fallback search to return results")
	}

	t.Logf("Fallback logic test succeeded: %s", response.FallbackReason)
}

// testResponseFormat validates the response format, chunk content, and similarity scores
func (suite *RetrievalAPITestSuite) testResponseFormat(t *testing.T) {
	searchReq := SearchRequest{
		Query: "security compliance",
	}

	response, err := suite.performSearch(searchReq)
	if err != nil {
		t.Fatalf("Response format test failed: %v", err)
	}

	// Validate response structure
	requiredFields := []string{"chunks", "count", "query", "fallback_triggered"}
	responseMap := map[string]interface{}{
		"chunks":             response.Chunks,
		"count":              response.Count,
		"query":              response.Query,
		"fallback_triggered": response.FallbackTriggered,
	}

	suite.ResultValidator.ValidateJSONStructure(t, responseMap, requiredFields)

	// Validate chunk structure
	for i, chunk := range response.Chunks {
		chunkMap := map[string]interface{}{
			"text":      chunk.Text,
			"score":     chunk.Score,
			"doc_id":    chunk.DocID,
			"source_id": chunk.SourceID,
			"metadata":  chunk.Metadata,
		}

		chunkRequiredFields := []string{"text", "score", "doc_id", "source_id", "metadata"}
		suite.ResultValidator.ValidateJSONStructure(t, chunkMap, chunkRequiredFields)

		// Validate score is a valid similarity score
		if chunk.Score < 0 || chunk.Score > 1 {
			t.Errorf("Chunk %d has invalid similarity score: %f", i, chunk.Score)
		}

		// Validate metadata is not empty
		if len(chunk.Metadata) == 0 {
			t.Errorf("Chunk %d has empty metadata", i)
		}
	}

	t.Logf("Response format validation passed for %d chunks", response.Count)
}

// testErrorHandling tests error handling for invalid queries and service failures
func (suite *RetrievalAPITestSuite) testErrorHandling(t *testing.T) {
	// Test empty query
	t.Run("EmptyQuery", func(t *testing.T) {
		searchReq := SearchRequest{
			Query: "",
		}

		_, err := suite.performSearch(searchReq)
		if err == nil {
			t.Error("Expected error for empty query")
		}
	})

	// Test invalid JSON
	t.Run("InvalidJSON", func(t *testing.T) {
		invalidJSON := `{"query": "test", "filters": {invalid json}}`

		resp, err := http.Post(
			suite.RetrievalServiceURL+"/search",
			"application/json",
			bytes.NewBuffer([]byte(invalidJSON)),
		)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})

	// Test service unavailable scenario
	t.Run("ServiceUnavailable", func(t *testing.T) {
		// This test would require stopping ChromaDB, which is complex
		// For now, we'll test graceful degradation when the service is available
		// but returns an error condition
		t.Skip("Service unavailable test requires complex setup")
	})
}

// testPerformance tests performance characteristics and response times
func (suite *RetrievalAPITestSuite) testPerformance(t *testing.T) {
	searchReq := SearchRequest{
		Query: "performance test query",
	}

	// Test single request performance
	start := time.Now()
	response, err := suite.performSearch(searchReq)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Performance test failed: %v", err)
	}

	// Validate response time is within acceptable limits (5 seconds as per requirements)
	maxDuration := 5 * time.Second
	suite.ResultValidator.ValidateResponseTime(t, duration, maxDuration, "SingleRequest")

	// Test concurrent requests
	t.Run("ConcurrentRequests", func(t *testing.T) {
		concurrency := 5
		done := make(chan bool, concurrency)
		errors := make(chan error, concurrency)

		start := time.Now()
		for i := 0; i < concurrency; i++ {
			go func(requestID int) {
				req := SearchRequest{
					Query: fmt.Sprintf("concurrent test query %d", requestID),
				}
				_, err := suite.performSearch(req)
				if err != nil {
					errors <- err
				}
				done <- true
			}(i)
		}

		// Wait for all requests to complete
		for i := 0; i < concurrency; i++ {
			select {
			case <-done:
				// Request completed successfully
			case err := <-errors:
				t.Errorf("Concurrent request failed: %v", err)
			case <-time.After(10 * time.Second):
				t.Error("Concurrent request timed out")
			}
		}

		concurrentDuration := time.Since(start)
		suite.ResultValidator.ValidateResponseTime(t, concurrentDuration, 10*time.Second, "ConcurrentRequests")
	})

	t.Logf("Performance test completed: single request %v, %d results", duration, response.Count)
}

// performSearch performs a search request and returns the response
func (suite *RetrievalAPITestSuite) performSearch(req SearchRequest) (*SearchResponse, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), suite.TestTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, "POST", suite.RetrievalServiceURL+"/search", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: suite.TestTimeout,
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &searchResp, nil
}

// setupTestSuite initializes the test suite with all required components
func setupTestSuite(t *testing.T) *RetrievalAPITestSuite {
	t.Helper()

	// Load test configuration
	testConfig := LoadTestConfig()

	// Initialize logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// Load application configuration
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Override ChromaDB URL for testing
	cfg.Chroma.URL = "http://localhost:8001" // Use test port

	// Initialize test components
	suite := &RetrievalAPITestSuite{
		TestDataManager:     NewTestDataManager(),
		HealthChecker:       NewServiceHealthChecker(),
		ResultValidator:     NewTestResultValidator(),
		FSHelper:            NewFileSystemHelper(t),
		Config:              cfg,
		Logger:              logger,
		RetrievalServiceURL: testConfig.ServicesBaseURL + ":8081",
		TestTimeout:         testConfig.Timeout,
	}

	// Override ChromaDB URL in test data manager
	suite.TestDataManager.ChromaURL = "http://localhost:8001"

	return suite
}

// teardown cleans up the test suite
func (suite *RetrievalAPITestSuite) teardown(t *testing.T) {
	t.Helper()

	if suite.FSHelper != nil {
		suite.FSHelper.Cleanup(t)
	}

	if suite.MetadataStore != nil {
		if err := suite.MetadataStore.Close(); err != nil {
			t.Logf("Warning: Failed to close metadata store: %v", err)
		}
	}

	if suite.Logger != nil {
		_ = suite.Logger.Sync()
	}
}

// ensureChromaDBRunning ensures ChromaDB test container is running
func ensureChromaDBRunning() error {
	// This would typically use docker-compose or docker commands
	// For now, we assume it's managed externally
	client := &http.Client{Timeout: 5 * time.Second}

	// Try to ping ChromaDB test instance
	resp, err := client.Get("http://localhost:8001/api/v1/heartbeat")
	if err != nil {
		return fmt.Errorf("ChromaDB test instance not running: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ChromaDB test instance unhealthy: status %d", resp.StatusCode)
	}

	return nil
}

// cleanupTestEnvironment cleans up the test environment
func cleanupTestEnvironment() {
	// This would typically stop test containers
	// For now, we leave cleanup to external process
}
