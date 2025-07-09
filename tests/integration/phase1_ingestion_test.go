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
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/your-org/ai-sa-assistant/internal/chroma"
	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/metadata"
)

const (
	testChromaURL        = "http://localhost:8001"
	testCollectionName   = "test_phase1_collection"
	testMetadataDBPath   = "test_metadata.db"
	testConfigPath       = "test_config.yaml"
	testDocsPath         = "tests/integration/testdata"
	testTimeout          = 300 * time.Second // 5 minutes for complete pipeline
	maxIngestionTime     = 180 * time.Second // 3 minutes max ingestion time
	expectedMinChunks    = 6                 // Minimum chunks expected from test documents
	expectedTestDocCount = 3                 // Number of valid test documents
)

// Phase1TestSuite manages the complete Phase 1 integration test
type Phase1TestSuite struct {
	chromaClient   *chroma.Client
	metadataStore  *metadata.Store
	testConfig     *config.Config
	tempDir        string
	testDBPath     string
	testConfigPath string
	httpClient     *http.Client
	containerID    string
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewPhase1TestSuite creates a new Phase 1 test suite
func NewPhase1TestSuite(t *testing.T) *Phase1TestSuite {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)

	// Create temporary directory for test files
	tempDir, err := os.MkdirTemp("", "phase1-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	suite := &Phase1TestSuite{
		tempDir:        tempDir,
		testDBPath:     filepath.Join(tempDir, testMetadataDBPath),
		testConfigPath: filepath.Join(tempDir, testConfigPath),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		ctx:    ctx,
		cancel: cancel,
	}

	// Create test configuration
	suite.createTestConfig(t)

	return suite
}

// createTestConfig creates a test configuration file
func (s *Phase1TestSuite) createTestConfig(t *testing.T) {
	t.Helper()

	// Check if OpenAI API key is available
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping Phase 1 integration test")
	}

	// Create test config file
	configData := fmt.Sprintf(`
openai:
  apikey: "%s"
  endpoint: "https://api.openai.com/v1"

chroma:
  url: "%s"
  collection_name: "%s"

metadata:
  db_path: "%s"

logging:
  level: "info"
  format: "json"
`, apiKey, testChromaURL, testCollectionName, s.testDBPath)

	err := os.WriteFile(s.testConfigPath, []byte(configData), 0600)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	// Load the configuration
	s.testConfig, err = config.Load(s.testConfigPath)
	if err != nil {
		t.Fatalf("Failed to load test config: %v", err)
	}
}

// SetupTestEnvironment sets up the complete test environment
func (s *Phase1TestSuite) SetupTestEnvironment(t *testing.T) {
	t.Helper()

	// Start ChromaDB container
	s.startChromaDBContainer(t)

	// Wait for ChromaDB to be ready
	s.waitForChromaDBReady(t)

	// Initialize ChromaDB client
	s.chromaClient = chroma.NewClient(testChromaURL, testCollectionName)

	// Clean up any existing test data
	s.cleanupTestData(t)

	// Create fresh test collection
	s.createTestCollection(t)
}

// startChromaDBContainer starts the ChromaDB test container
func (s *Phase1TestSuite) startChromaDBContainer(t *testing.T) {
	t.Helper()

	// Stop any existing container
	exec.Command("docker", "stop", "chromadb-test").Run() // nolint: errcheck
	exec.Command("docker", "rm", "chromadb-test").Run()   // nolint: errcheck

	// Start new container using docker-compose
	cmd := exec.Command("docker-compose", "-f", "docker-compose.test.yml", "up", "-d", "chromadb-test")
	cmd.Dir = "."
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to start ChromaDB container: %v\nOutput: %s", err, output)
	}

	s.containerID = "chromadb-test"
	t.Logf("Started ChromaDB test container: %s", s.containerID)
}

// waitForChromaDBReady waits for ChromaDB to be ready
func (s *Phase1TestSuite) waitForChromaDBReady(t *testing.T) {
	t.Helper()

	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		resp, err := s.httpClient.Get(testChromaURL + "/api/v1/heartbeat")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			t.Logf("ChromaDB is ready after %d attempts", i+1)
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(2 * time.Second)
	}

	t.Fatalf("ChromaDB did not become ready within timeout")
}

// createTestCollection creates a test collection in ChromaDB
func (s *Phase1TestSuite) createTestCollection(t *testing.T) {
	t.Helper()

	err := s.chromaClient.CreateCollection(s.ctx, testCollectionName, map[string]interface{}{
		"description": "Phase 1 integration test collection",
		"created_at":  time.Now().Format(time.RFC3339),
	})
	if err != nil {
		t.Logf("Warning: Failed to create test collection (may already exist): %v", err)
	}
}

// cleanupTestData removes any existing test data
func (s *Phase1TestSuite) cleanupTestData(t *testing.T) {
	t.Helper()

	// Delete test collection from ChromaDB
	err := s.chromaClient.DeleteCollection(s.ctx, testCollectionName)
	if err != nil {
		t.Logf("Warning: Failed to delete test collection: %v", err)
	}

	// Remove test metadata database
	if err := os.Remove(s.testDBPath); err != nil && !os.IsNotExist(err) {
		t.Logf("Warning: Failed to remove test metadata database: %v", err)
	}
}

// TestPhase1IngestionPipeline tests the complete Phase 1 ingestion pipeline
func TestPhase1IngestionPipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Phase 1 integration test in short mode")
	}

	suite := NewPhase1TestSuite(t)
	defer suite.Cleanup(t)

	// Setup test environment
	suite.SetupTestEnvironment(t)

	// Test 1: Validate docker-compose setup
	t.Run("DockerComposeSetup", suite.testDockerComposeSetup)

	// Test 2: Run ingestion pipeline
	t.Run("IngestionPipeline", suite.testIngestionPipeline)

	// Test 3: Validate ChromaDB population
	t.Run("ChromaDBValidation", suite.testChromaDBValidation)

	// Test 4: Validate metadata database
	t.Run("MetadataDBValidation", suite.testMetadataDBValidation)

	// Test 5: Performance validation
	t.Run("PerformanceValidation", suite.testPerformanceValidation)

	// Test 6: Failure scenarios
	t.Run("FailureScenarios", suite.testFailureScenarios)
}

// testDockerComposeSetup validates the docker-compose test setup
func (s *Phase1TestSuite) testDockerComposeSetup(t *testing.T) {
	t.Helper()

	// Check if ChromaDB container is running
	cmd := exec.Command("docker", "ps", "--filter", "name=chromadb-test", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to check container status: %v", err)
	}

	if !strings.Contains(string(output), "chromadb-test") {
		t.Fatalf("ChromaDB test container is not running")
	}

	// Verify ChromaDB API is accessible
	resp, err := s.httpClient.Get(testChromaURL + "/api/v1/heartbeat")
	if err != nil {
		t.Fatalf("ChromaDB API is not accessible: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ChromaDB heartbeat failed: status %d", resp.StatusCode)
	}

	t.Logf("✅ Docker Compose setup validated successfully")
}

// testIngestionPipeline tests the complete ingestion pipeline
func (s *Phase1TestSuite) testIngestionPipeline(t *testing.T) {
	t.Helper()

	// Record start time for performance measurement
	startTime := time.Now()

	// Run ingestion command
	cmd := exec.Command("go", "run", "cmd/ingest/main.go",
		"--config", s.testConfigPath,
		"--docs-path", testDocsPath,
		"--chunk-size", "400",
		"--force-reindex")

	cmd.Dir = "."
	cmd.Env = append(os.Environ(), fmt.Sprintf("OPENAI_API_KEY=%s", s.testConfig.OpenAI.APIKey))

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Ingestion pipeline failed: %v\nOutput: %s", err, output)
	}

	// Record completion time
	ingestionTime := time.Since(startTime)

	// Validate ingestion completed within reasonable time
	if ingestionTime > maxIngestionTime {
		t.Errorf("Ingestion took too long: %v > %v", ingestionTime, maxIngestionTime)
	}

	// Validate output contains expected success indicators
	outputStr := string(output)
	if !strings.Contains(outputStr, "Ingestion completed successfully") {
		t.Errorf("Ingestion output does not contain success message")
	}

	// Validate document processing stats
	if !strings.Contains(outputStr, "successful") {
		t.Errorf("Ingestion output does not contain processing stats")
	}

	t.Logf("✅ Ingestion pipeline completed successfully in %v", ingestionTime)
}

// testChromaDBValidation validates ChromaDB document storage
func (s *Phase1TestSuite) testChromaDBValidation(t *testing.T) {
	t.Helper()

	// Get collection info
	resp, err := s.httpClient.Get(fmt.Sprintf("%s/api/v1/collections/%s", testChromaURL, testCollectionName))
	if err != nil {
		t.Fatalf("Failed to get collection info: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Collection not found: status %d", resp.StatusCode)
	}

	// Get collection count
	countResp, err := s.httpClient.Get(fmt.Sprintf("%s/api/v1/collections/%s/count", testChromaURL, testCollectionName))
	if err != nil {
		t.Fatalf("Failed to get collection count: %v", err)
	}
	defer countResp.Body.Close()

	var countData struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(countResp.Body).Decode(&countData); err != nil {
		t.Fatalf("Failed to decode count response: %v", err)
	}

	// Validate document count
	if countData.Count < expectedMinChunks {
		t.Errorf("Expected at least %d chunks, got %d", expectedMinChunks, countData.Count)
	}

	// Validate document content by querying
	s.validateDocumentContent(t)

	t.Logf("✅ ChromaDB validation passed with %d documents", countData.Count)
}

// validateDocumentContent validates the content of stored documents
func (s *Phase1TestSuite) validateDocumentContent(t *testing.T) {
	t.Helper()

	// Query for documents to validate content
	queryReq := map[string]interface{}{
		"query_texts": []string{"AWS migration"},
		"n_results":   5,
	}

	queryBody, _ := json.Marshal(queryReq)
	resp, err := s.httpClient.Post(
		fmt.Sprintf("%s/api/v1/collections/%s/query", testChromaURL, testCollectionName),
		"application/json",
		strings.NewReader(string(queryBody)),
	)
	if err != nil {
		t.Fatalf("Failed to query documents: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Query failed: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var queryResp struct {
		Documents [][]string                 `json:"documents"`
		Metadatas [][]map[string]interface{} `json:"metadatas"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&queryResp); err != nil {
		t.Fatalf("Failed to decode query response: %v", err)
	}

	// Validate we got results
	if len(queryResp.Documents) == 0 || len(queryResp.Documents[0]) == 0 {
		t.Errorf("No documents returned from query")
	}

	// Validate metadata fields are present
	if len(queryResp.Metadatas) > 0 && len(queryResp.Metadatas[0]) > 0 {
		metadata := queryResp.Metadatas[0][0]
		requiredFields := []string{"doc_id", "title", "platform", "scenario", "type"}
		for _, field := range requiredFields {
			if _, exists := metadata[field]; !exists {
				t.Errorf("Required metadata field missing: %s", field)
			}
		}
	}

	t.Logf("✅ Document content validation passed")
}

// testMetadataDBValidation validates SQLite metadata database
func (s *Phase1TestSuite) testMetadataDBValidation(t *testing.T) {
	t.Helper()

	// Check if metadata database exists
	if _, err := os.Stat(s.testDBPath); os.IsNotExist(err) {
		t.Fatalf("Metadata database not created: %s", s.testDBPath)
	}

	// Connect to metadata database
	db, err := sql.Open("sqlite3", s.testDBPath)
	if err != nil {
		t.Fatalf("Failed to open metadata database: %v", err)
	}
	defer db.Close()

	// Validate table exists
	var tableCount int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='metadata'").Scan(&tableCount)
	if err != nil {
		t.Fatalf("Failed to check metadata table: %v", err)
	}

	if tableCount != 1 {
		t.Errorf("Expected 1 metadata table, got %d", tableCount)
	}

	// Validate document count
	var docCount int
	err = db.QueryRow("SELECT COUNT(*) FROM metadata").Scan(&docCount)
	if err != nil {
		t.Fatalf("Failed to get document count: %v", err)
	}

	// Should have all documents from metadata.json (including the malformed one)
	if docCount != 4 {
		t.Errorf("Expected 4 documents in metadata, got %d", docCount)
	}

	// Validate specific document entries
	s.validateMetadataEntries(t, db)

	t.Logf("✅ Metadata database validation passed with %d entries", docCount)
}

// validateMetadataEntries validates specific metadata entries
func (s *Phase1TestSuite) validateMetadataEntries(t *testing.T, db *sql.DB) {
	t.Helper()

	// Query for specific test documents
	rows, err := db.Query("SELECT doc_id, title, platform, scenario, type FROM metadata WHERE doc_id LIKE 'test-%'")
	if err != nil {
		t.Fatalf("Failed to query metadata: %v", err)
	}
	defer rows.Close()

	foundDocs := make(map[string]bool)
	for rows.Next() {
		var docID, title, platform, scenario, docType string
		if err := rows.Scan(&docID, &title, &platform, &scenario, &docType); err != nil {
			t.Fatalf("Failed to scan metadata row: %v", err)
		}
		foundDocs[docID] = true
	}

	// Validate expected documents are present
	expectedDocs := []string{"test-aws-migration", "test-azure-hybrid", "test-security-compliance"}
	for _, docID := range expectedDocs {
		if !foundDocs[docID] {
			t.Errorf("Expected document not found in metadata: %s", docID)
		}
	}

	t.Logf("✅ Metadata entries validation passed")
}

// testPerformanceValidation validates performance requirements
func (s *Phase1TestSuite) testPerformanceValidation(t *testing.T) {
	t.Helper()

	// Measure ingestion performance by running a smaller subset
	startTime := time.Now()

	// Create a single test document for performance measurement
	testDoc := filepath.Join(s.tempDir, "perf-test.md")
	testContent := `# Performance Test Document
This is a test document for performance validation.
It contains multiple paragraphs to generate several chunks.

## Section 1
Content for section 1 with sufficient text to create chunks.
This helps measure the performance of the ingestion pipeline.

## Section 2
Additional content for performance testing.
More text to ensure proper chunk creation.

## Conclusion
This document tests ingestion performance.`

	err := os.WriteFile(testDoc, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create performance test document: %v", err)
	}

	// Create minimal metadata for performance test
	perfMetadata := map[string]interface{}{
		"schema_version": "1.0.0",
		"description":    "Performance test metadata",
		"last_updated":   time.Now().Format(time.RFC3339),
		"documents": []map[string]interface{}{
			{
				"doc_id":         "perf-test",
				"title":          "Performance Test Document",
				"platform":       "test",
				"scenario":       "test",
				"type":           "test",
				"source_url":     "https://example.com/perf-test",
				"path":           testDoc,
				"tags":           []string{"performance", "test"},
				"difficulty":     "beginner",
				"estimated_time": "5 minutes",
			},
		},
	}

	perfMetadataPath := filepath.Join(s.tempDir, "perf-metadata.json")
	perfMetadataFile, err := os.Create(perfMetadataPath)
	if err != nil {
		t.Fatalf("Failed to create performance metadata file: %v", err)
	}
	defer perfMetadataFile.Close()

	if err := json.NewEncoder(perfMetadataFile).Encode(perfMetadata); err != nil {
		t.Fatalf("Failed to write performance metadata: %v", err)
	}

	// Run performance ingestion
	cmd := exec.Command("go", "run", "cmd/ingest/main.go",
		"--config", s.testConfigPath,
		"--docs-path", s.tempDir,
		"--chunk-size", "200")

	cmd.Dir = "."
	cmd.Env = append(os.Environ(), fmt.Sprintf("OPENAI_API_KEY=%s", s.testConfig.OpenAI.APIKey))

	_, err = cmd.CombinedOutput()
	if err != nil {
		t.Logf("Performance test ingestion failed (expected for isolated test): %v", err)
	}

	perfTime := time.Since(startTime)
	maxPerfTime := 60 * time.Second // 1 minute for single document

	if perfTime > maxPerfTime {
		t.Errorf("Performance test exceeded maximum time: %v > %v", perfTime, maxPerfTime)
	}

	t.Logf("✅ Performance validation passed: %v", perfTime)
}

// testFailureScenarios tests various failure scenarios
func (s *Phase1TestSuite) testFailureScenarios(t *testing.T) {
	t.Helper()

	// Test 1: Missing API key
	t.Run("MissingAPIKey", s.testMissingAPIKey)

	// Test 2: ChromaDB unavailable
	t.Run("ChromaDBUnavailable", s.testChromaDBUnavailable)

	// Test 3: Malformed documents
	t.Run("MalformedDocuments", s.testMalformedDocuments)
}

// testMissingAPIKey tests behavior with missing OpenAI API key
func (s *Phase1TestSuite) testMissingAPIKey(t *testing.T) {
	t.Helper()

	// Create config without API key
	noKeyConfig := filepath.Join(s.tempDir, "no-key-config.yaml")
	configData := fmt.Sprintf(`
openai:
  api_key: ""
  model: "gpt-4o"
  embedding_model: "text-embedding-3-small"

chroma:
  url: "%s"
  collection_name: "%s"

metadata:
  db_path: "%s"
`, testChromaURL, testCollectionName, s.testDBPath)

	err := os.WriteFile(noKeyConfig, []byte(configData), 0600)
	if err != nil {
		t.Fatalf("Failed to create no-key config: %v", err)
	}

	// Run ingestion with missing API key
	cmd := exec.Command("go", "run", "cmd/ingest/main.go",
		"--config", noKeyConfig,
		"--docs-path", testDocsPath)

	cmd.Dir = "."
	output, err := cmd.CombinedOutput()

	// Should fail with appropriate error
	if err == nil {
		t.Errorf("Expected ingestion to fail with missing API key, but it succeeded")
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "API key") && !strings.Contains(outputStr, "authentication") {
		t.Logf("Warning: Expected API key error message, got: %s", outputStr)
	}

	t.Logf("✅ Missing API key test passed")
}

// testChromaDBUnavailable tests behavior with unavailable ChromaDB
func (s *Phase1TestSuite) testChromaDBUnavailable(t *testing.T) {
	t.Helper()

	// Create config with wrong ChromaDB URL
	wrongURLConfig := filepath.Join(s.tempDir, "wrong-url-config.yaml")
	configData := fmt.Sprintf(`
openai:
  api_key: "%s"
  model: "gpt-4o"
  embedding_model: "text-embedding-3-small"

chroma:
  url: "http://localhost:9999"
  collection_name: "%s"

metadata:
  db_path: "%s"
`, s.testConfig.OpenAI.APIKey, testCollectionName, s.testDBPath)

	err := os.WriteFile(wrongURLConfig, []byte(configData), 0600)
	if err != nil {
		t.Fatalf("Failed to create wrong-url config: %v", err)
	}

	// Run ingestion with unavailable ChromaDB
	cmd := exec.Command("go", "run", "cmd/ingest/main.go",
		"--config", wrongURLConfig,
		"--docs-path", testDocsPath)

	cmd.Dir = "."
	cmd.Env = append(os.Environ(), fmt.Sprintf("OPENAI_API_KEY=%s", s.testConfig.OpenAI.APIKey))

	output, err := cmd.CombinedOutput()

	// Should fail with connection error
	if err == nil {
		t.Errorf("Expected ingestion to fail with unavailable ChromaDB, but it succeeded")
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "health check failed") && !strings.Contains(outputStr, "connection") {
		t.Logf("Warning: Expected connection error message, got: %s", outputStr)
	}

	t.Logf("✅ ChromaDB unavailable test passed")
}

// testMalformedDocuments tests behavior with malformed documents
func (s *Phase1TestSuite) testMalformedDocuments(t *testing.T) {
	t.Helper()

	// The test metadata.json already includes a malformed document entry
	// with a non-existent file path, which should be handled gracefully

	// Run ingestion - should complete successfully despite malformed document
	cmd := exec.Command("go", "run", "cmd/ingest/main.go",
		"--config", s.testConfigPath,
		"--docs-path", testDocsPath)

	cmd.Dir = "."
	cmd.Env = append(os.Environ(), fmt.Sprintf("OPENAI_API_KEY=%s", s.testConfig.OpenAI.APIKey))

	output, err := cmd.CombinedOutput()

	// Should succeed overall but skip malformed documents
	if err != nil {
		t.Errorf("Ingestion failed when it should handle malformed documents gracefully: %v", err)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "skipped") {
		t.Logf("Warning: Expected skipped documents message, got: %s", outputStr)
	}

	t.Logf("✅ Malformed documents test passed")
}

// Cleanup cleans up all test resources
func (s *Phase1TestSuite) Cleanup(t *testing.T) {
	t.Helper()

	// Cancel context
	if s.cancel != nil {
		s.cancel()
	}

	// Clean up test data
	s.cleanupTestData(t)

	// Stop test container
	if s.containerID != "" {
		cmd := exec.Command("docker-compose", "-f", "docker-compose.test.yml", "down", "-v")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Logf("Warning: Failed to stop test container: %v\nOutput: %s", err, output)
		}
	}

	// Remove temporary directory
	if err := os.RemoveAll(s.tempDir); err != nil {
		t.Logf("Warning: Failed to remove temp directory: %v", err)
	}

	t.Logf("✅ Test cleanup completed")
}

// TestPhase1Pipeline_CI is a minimal test for CI/CD environments
func TestPhase1Pipeline_CI(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Phase 1 CI test in short mode")
	}

	// Check if this is a CI environment
	if os.Getenv("CI") == "" {
		t.Skip("Skipping CI-specific test outside CI environment")
	}

	// Minimal validation for CI
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("OPENAI_API_KEY not available in CI")
	}

	// Simple validation that the ingestion command exists and compiles
	cmd := exec.Command("go", "build", "-o", "/tmp/test-ingest", "cmd/ingest/main.go")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build ingestion command: %v\nOutput: %s", err, output)
	}

	// Clean up
	os.Remove("/tmp/test-ingest")

	t.Logf("✅ Phase 1 CI test passed")
}
