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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDataManager manages test data setup and teardown
type TestDataManager struct {
	ChromaURL    string
	MetadataPath string
	Client       *http.Client
}

// NewTestDataManager creates a new test data manager
func NewTestDataManager() *TestDataManager {
	return &TestDataManager{
		ChromaURL:    "http://localhost:8000",
		MetadataPath: "test_metadata.db",
		Client: &http.Client{
			Timeout: 30 * time.Second, //nolint:mnd // Standard timeout value
		},
	}
}

// SetupTestData sets up test data for integration tests
func (tm *TestDataManager) SetupTestData(t *testing.T) {
	t.Helper()

	// Create test collection in ChromaDB
	if err := tm.createTestCollection(); err != nil {
		t.Logf("Warning: Could not create test collection: %v", err)
	}

	// Insert test documents
	if err := tm.insertTestDocuments(); err != nil {
		t.Logf("Warning: Could not insert test documents: %v", err)
	}

	// Create test metadata
	if err := tm.createTestMetadata(); err != nil {
		t.Logf("Warning: Could not create test metadata: %v", err)
	}
}

// TeardownTestData cleans up test data after integration tests
func (tm *TestDataManager) TeardownTestData(t *testing.T) {
	t.Helper()

	// Delete test collection
	if err := tm.deleteTestCollection(); err != nil {
		t.Logf("Warning: Could not delete test collection: %v", err)
	}

	// Clean up test metadata
	if err := tm.cleanupTestMetadata(); err != nil {
		t.Logf("Warning: Could not cleanup test metadata: %v", err)
	}
}

// createTestCollection creates a test collection in ChromaDB
func (tm *TestDataManager) createTestCollection() error {
	collectionReq := map[string]interface{}{
		"name": "test_demo_collection",
		"metadata": map[string]interface{}{
			"description": "Test collection for demo scenarios",
			"created_by":  "integration_test",
			"created_at":  time.Now().Format(time.RFC3339),
		},
	}
	body, _ := json.Marshal(collectionReq)

	resp, err := tm.Client.Post(tm.ChromaURL+"/api/v1/collections", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	// 200 OK or 409 Conflict (already exists) are both acceptable
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// insertTestDocuments inserts test documents into ChromaDB
func (tm *TestDataManager) insertTestDocuments() error {
	testDocuments := []TestDocument{
		{
			ID: "aws_migration_doc_1",
			Content: "AWS Migration best practices include using AWS MGN for lift-and-shift migrations. " +
				"EC2 instance types should be selected based on workload requirements. " +
				"VPC design should follow AWS Well-Architected principles.",
			Metadata: map[string]interface{}{
				"scenario": "migration",
				"cloud":    "aws",
				"type":     "playbook",
				"title":    "AWS Migration Playbook",
			},
		},
		{
			ID: "azure_hybrid_doc_1",
			Content: "Azure hybrid architecture requires ExpressRoute for secure connectivity. " +
				"VMware HCX enables seamless migration between on-premises VMware environments and Azure. " +
				"Active-active failover ensures high availability.",
			Metadata: map[string]interface{}{
				"scenario": "hybrid",
				"cloud":    "azure",
				"type":     "playbook",
				"title":    "Azure Hybrid Architecture Guide",
			},
		},
		{
			ID: "azure_dr_doc_1",
			Content: "Azure disaster recovery solutions support RTO and RPO requirements through " +
				"Azure Site Recovery. Geo-replication provides data protection across regions. " +
				"Cost optimization can be achieved through cold standby configurations.",
			Metadata: map[string]interface{}{
				"scenario": "disaster-recovery",
				"cloud":    "azure",
				"type":     "playbook",
				"title":    "Azure Disaster Recovery Guide",
			},
		},
		{
			ID: "aws_security_doc_1",
			Content: "AWS security compliance includes HIPAA and GDPR requirements. " +
				"Encryption at rest and in transit is mandatory. CloudTrail logging ensures audit compliance. " +
				"AWS Config monitors policy enforcement.",
			Metadata: map[string]interface{}{
				"scenario": "security",
				"cloud":    "aws",
				"type":     "compliance",
				"title":    "AWS Security Compliance Guide",
				"tags":     []string{"HIPAA", "GDPR", "compliance"},
			},
		},
	}

	for _, doc := range testDocuments {
		if err := tm.insertDocument(doc); err != nil {
			return fmt.Errorf("failed to insert document %s: %v", doc.ID, err)
		}
	}

	return nil
}

// insertDocument inserts a single document into ChromaDB
func (tm *TestDataManager) insertDocument(doc TestDocument) error {
	// Generate a simple mock embedding (in real usage, this would come from OpenAI)
	const embeddingDim = 1536 // OpenAI text-embedding-3-small dimension
	embedding := make([]float32, embeddingDim)
	for i := range embedding {
		embedding[i] = float32(i) / embeddingDim // Simple mock embedding
	}

	addReq := map[string]interface{}{
		"documents":  []string{doc.Content},
		"metadatas":  []map[string]interface{}{doc.Metadata},
		"ids":        []string{doc.ID},
		"embeddings": [][]float32{embedding},
	}
	body, _ := json.Marshal(addReq)

	resp, err := tm.Client.Post(
		tm.ChromaURL+"/api/v1/collections/test_demo_collection/add",
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// createTestMetadata creates test metadata for the SQLite database
func (tm *TestDataManager) createTestMetadata() error {
	// In a real implementation, this would interact with the SQLite database
	// For now, we'll just create a simple metadata file
	metadata := map[string]interface{}{
		"documents": []map[string]interface{}{
			{
				"doc_id":   "aws_migration_doc_1",
				"scenario": "migration",
				"cloud":    "aws",
				"type":     "playbook",
			},
			{
				"doc_id":   "azure_hybrid_doc_1",
				"scenario": "hybrid",
				"cloud":    "azure",
				"type":     "playbook",
			},
			{
				"doc_id":   "azure_dr_doc_1",
				"scenario": "disaster-recovery",
				"cloud":    "azure",
				"type":     "playbook",
			},
			{
				"doc_id":   "aws_security_doc_1",
				"scenario": "security",
				"cloud":    "aws",
				"type":     "compliance",
			},
		},
	}

	file, err := os.Create(tm.MetadataPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	return json.NewEncoder(file).Encode(metadata)
}

// deleteTestCollection deletes the test collection from ChromaDB
func (tm *TestDataManager) deleteTestCollection() error {
	req, err := http.NewRequest("DELETE", tm.ChromaURL+"/api/v1/collections/test_demo_collection", http.NoBody)
	if err != nil {
		return err
	}

	resp, err := tm.Client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	// 200 OK or 404 Not Found are both acceptable
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// cleanupTestMetadata removes test metadata files
func (tm *TestDataManager) cleanupTestMetadata() error {
	if _, err := os.Stat(tm.MetadataPath); os.IsNotExist(err) {
		return nil // File doesn't exist, nothing to clean up
	}
	return os.Remove(tm.MetadataPath)
}

// TestDocument represents a test document structure
type TestDocument struct {
	ID       string                 `json:"id"`
	Content  string                 `json:"content"`
	Metadata map[string]interface{} `json:"metadata"`
}

// MockTeamsWebhook represents a mock Teams webhook for testing
type MockTeamsWebhook struct {
	Responses []TeamsWebhookResponse
	CallCount int
}

// TeamsWebhookResponse represents a response from the Teams webhook
type TeamsWebhookResponse struct {
	StatusCode int
	Body       interface{}
	Headers    map[string]string
}

// NewMockTeamsWebhook creates a new mock Teams webhook
func NewMockTeamsWebhook() *MockTeamsWebhook {
	return &MockTeamsWebhook{
		Responses: []TeamsWebhookResponse{},
		CallCount: 0,
	}
}

// AddResponse adds a mock response to the webhook
func (m *MockTeamsWebhook) AddResponse(statusCode int, body interface{}) {
	m.Responses = append(m.Responses, TeamsWebhookResponse{
		StatusCode: statusCode,
		Body:       body,
		Headers:    map[string]string{"Content-Type": "application/json"},
	})
}

// GetNextResponse returns the next mock response
func (m *MockTeamsWebhook) GetNextResponse() TeamsWebhookResponse {
	if m.CallCount >= len(m.Responses) {
		// Return a default response if no more responses are defined
		return TeamsWebhookResponse{
			StatusCode: http.StatusOK,
			Body:       map[string]interface{}{"message": "Mock response"},
			Headers:    map[string]string{"Content-Type": "application/json"},
		}
	}

	response := m.Responses[m.CallCount]
	m.CallCount++
	return response
}

// Reset resets the mock webhook state
func (m *MockTeamsWebhook) Reset() {
	m.CallCount = 0
	m.Responses = []TeamsWebhookResponse{}
}

// ServiceHealthChecker provides utilities for checking service health
type ServiceHealthChecker struct {
	Client *http.Client
}

// NewServiceHealthChecker creates a new service health checker
func NewServiceHealthChecker() *ServiceHealthChecker {
	return &ServiceHealthChecker{
		Client: &http.Client{
			Timeout: 10 * time.Second, //nolint:mnd // Standard timeout value
		},
	}
}

// CheckAllServices checks the health of all services
func (s *ServiceHealthChecker) CheckAllServices(t *testing.T) bool {
	t.Helper()

	services := map[string]string{
		"retrieve":   "http://localhost:8081/health",
		"websearch":  "http://localhost:8083/health",
		"synthesize": "http://localhost:8082/health",
		"teamsbot":   "http://localhost:8080/health",
	}

	allHealthy := true
	for serviceName, url := range services {
		if !s.CheckService(t, serviceName, url) {
			allHealthy = false
		}
	}

	return allHealthy
}

// CheckService checks the health of a single service
func (s *ServiceHealthChecker) CheckService(t *testing.T, serviceName, url string) bool {
	t.Helper()

	resp, err := s.Client.Get(url)
	if err != nil {
		t.Logf("Service %s health check failed: %v", serviceName, err)
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Logf("Service %s unhealthy: status %d", serviceName, resp.StatusCode)
		return false
	}

	var healthResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&healthResponse); err != nil {
		t.Logf("Service %s health response decode failed: %v", serviceName, err)
		return false
	}

	if status, ok := healthResponse["status"]; !ok || status != "healthy" {
		t.Logf("Service %s reports unhealthy status: %v", serviceName, status)
		return false
	}

	t.Logf("Service %s is healthy", serviceName)
	return true
}

// WaitForServices waits for all services to become healthy
func (s *ServiceHealthChecker) WaitForServices(t *testing.T, timeout time.Duration) bool {
	t.Helper()

	start := time.Now()
	for time.Since(start) < timeout {
		if s.CheckAllServices(t) {
			t.Logf("All services are healthy after %v", time.Since(start))
			return true
		}
		time.Sleep(2 * time.Second) //nolint:mnd // Standard retry interval
	}

	t.Logf("Services did not become healthy within %v", timeout)
	return false
}

// TestResultValidator provides utilities for validating test results
type TestResultValidator struct{}

// NewTestResultValidator creates a new test result validator
func NewTestResultValidator() *TestResultValidator {
	return &TestResultValidator{}
}

// ValidateResponseTime validates that a response time is within acceptable limits
func (v *TestResultValidator) ValidateResponseTime(
	t *testing.T, duration time.Duration, maxDuration time.Duration, scenarioName string,
) {
	t.Helper()

	if duration > maxDuration {
		t.Errorf("Scenario %s exceeded maximum response time: %v > %v", scenarioName, duration, maxDuration)
	} else {
		t.Logf("Scenario %s completed within acceptable time: %v", scenarioName, duration)
	}
}

// ValidateJSONStructure validates that a JSON response has the expected structure
func (v *TestResultValidator) ValidateJSONStructure(t *testing.T, data interface{}, requiredFields []string) {
	t.Helper()

	dataMap, ok := data.(map[string]interface{})
	if !ok {
		t.Error("Expected JSON object, got different type")
		return
	}

	for _, field := range requiredFields {
		if _, exists := dataMap[field]; !exists {
			t.Errorf("Required field missing: %s", field)
		}
	}
}

// ValidateStringContains validates that a string contains expected substrings
func (v *TestResultValidator) ValidateStringContains(t *testing.T, text string, expectedSubstrings []string) {
	t.Helper()

	for _, expected := range expectedSubstrings {
		if !containsIgnoreCase(text, expected) {
			t.Errorf("Text does not contain expected substring: %s", expected)
		}
	}
}

// containsIgnoreCase checks if a string contains a substring (case-insensitive)
func containsIgnoreCase(text, substring string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(substring))
}

// FileSystemHelper provides utilities for file system operations during tests
type FileSystemHelper struct {
	TempDir string
}

// NewFileSystemHelper creates a new file system helper
func NewFileSystemHelper(t *testing.T) *FileSystemHelper {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "ai-sa-assistant-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	return &FileSystemHelper{
		TempDir: tempDir,
	}
}

// CreateTempFile creates a temporary file with content
func (f *FileSystemHelper) CreateTempFile(t *testing.T, filename, content string) string {
	t.Helper()

	filePath := filepath.Clean(filepath.Join(f.TempDir, filename))
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = file.Close() }()

	if _, err := file.WriteString(content); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	return filePath
}

// Cleanup removes the temporary directory and all files
func (f *FileSystemHelper) Cleanup(t *testing.T) {
	t.Helper()

	if err := os.RemoveAll(f.TempDir); err != nil {
		t.Logf("Warning: Failed to cleanup temp directory: %v", err)
	}
}

// LoadTestConfig loads test configuration from environment variables
func LoadTestConfig() TestConfig {
	return TestConfig{
		ServicesBaseURL: getEnvOrDefault("TEST_SERVICES_BASE_URL", "http://localhost"),
		ChromaDBURL:     getEnvOrDefault("TEST_CHROMADB_URL", "http://localhost:8000"),
		Timeout: parseDurationOrDefault(
			getEnvOrDefault("TEST_TIMEOUT", "60s"), 60*time.Second, //nolint:mnd // Default timeout
		),
		SkipSlowTests:  getEnvOrDefault("TEST_SKIP_SLOW", "false") == "true",
		VerboseLogging: getEnvOrDefault("TEST_VERBOSE", "false") == "true",
	}
}

// TestConfig represents test configuration
type TestConfig struct {
	ServicesBaseURL string
	ChromaDBURL     string
	Timeout         time.Duration
	SkipSlowTests   bool
	VerboseLogging  bool
}

// getEnvOrDefault gets an environment variable or returns a default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// parseDurationOrDefault parses a duration string or returns a default value
func parseDurationOrDefault(value string, defaultValue time.Duration) time.Duration {
	if duration, err := time.ParseDuration(value); err == nil {
		return duration
	}
	return defaultValue
}
