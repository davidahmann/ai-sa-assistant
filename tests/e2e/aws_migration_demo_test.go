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

//go:build e2e

// Package e2e provides end-to-end test functionality for the AI SA Assistant
// Teams bot integration, including AWS migration demo scenarios.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// AWSMigrationTestQuery represents the test query structure
type AWSMigrationTestQuery struct {
	Text             string           `json:"text"`
	Type             string           `json:"type"`
	User             User             `json:"user"`
	Channel          Channel          `json:"channel"`
	Timestamp        string           `json:"timestamp"`
	ExpectedResponse ExpectedResponse `json:"expected_response"`
}

// User represents a test user
type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Channel represents a test channel
type Channel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ExpectedResponse represents the expected response criteria
type ExpectedResponse struct {
	ContainsKeywords       []string `json:"contains_keywords"`
	ContainsCodeSnippets   []string `json:"contains_code_snippets"`
	ContainsDiagram        bool     `json:"contains_diagram"`
	DiagramKeywords        []string `json:"diagram_keywords"`
	ExpectedSources        []string `json:"expected_sources"`
	MaxResponseTimeSeconds int      `json:"max_response_time_seconds"`
	MinResponseLength      int      `json:"min_response_length"`
}

// E2ETestSuite represents the E2E test suite
type E2ETestSuite struct {
	mockTeamsServer *MockTeamsServer
	teamsBotClient  *TeamsBotClient
	testConfig      *E2ETestConfig
}

// E2ETestConfig represents the E2E test configuration
type E2ETestConfig struct {
	TeamsBotURL       string
	DockerComposeFile string
	Timeout           time.Duration
	SkipDocker        bool
	VerboseLogging    bool
}

// NewE2ETestSuite creates a new E2E test suite
func NewE2ETestSuite(t *testing.T) *E2ETestSuite {
	t.Helper()

	config := &E2ETestConfig{
		TeamsBotURL:       getEnvOrDefault("E2E_TEAMSBOT_URL", "http://localhost:8080"),
		DockerComposeFile: getEnvOrDefault("E2E_DOCKER_COMPOSE", "docker-compose.yml"),
		Timeout:           parseDurationOrDefault(getEnvOrDefault("E2E_TIMEOUT", "60s"), 60*time.Second),
		SkipDocker:        getEnvOrDefault("E2E_SKIP_DOCKER", "false") == "true",
		VerboseLogging:    getEnvOrDefault("E2E_VERBOSE", "false") == "true",
	}

	mockTeamsServer := NewMockTeamsServer()
	teamsBotClient := NewTeamsBotClient(config.TeamsBotURL)

	return &E2ETestSuite{
		mockTeamsServer: mockTeamsServer,
		teamsBotClient:  teamsBotClient,
		testConfig:      config,
	}
}

// TestAWSMigrationDemo tests the complete AWS migration demo scenario
func TestAWSMigrationDemo(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	suite := NewE2ETestSuite(t)
	defer suite.Cleanup(t)

	// Setup test environment
	suite.SetupTestEnvironment(t)

	// Load test query
	query := suite.LoadTestQuery(t)

	// Execute the test
	suite.RunAWSMigrationTest(t, query)
}

// SetupTestEnvironment sets up the test environment
func (s *E2ETestSuite) SetupTestEnvironment(t *testing.T) {
	t.Helper()

	if s.testConfig.VerboseLogging {
		t.Log("Setting up E2E test environment...")
	}

	// Check if services are running
	if !s.checkServicesHealth(t) {
		if s.testConfig.SkipDocker {
			t.Fatal("Services are not running and Docker setup is skipped")
		}
		t.Log("Services not running, they should be started via docker-compose up")
		t.Skip("Services not available - run 'docker-compose up' to start services")
	}

	// Wait for services to be fully ready
	if !s.waitForServicesReady(t, s.testConfig.Timeout) {
		t.Fatal("Services did not become ready within timeout")
	}

	if s.testConfig.VerboseLogging {
		t.Log("E2E test environment setup complete")
	}
}

// LoadTestQuery loads the test query from file
func (s *E2ETestSuite) LoadTestQuery(t *testing.T) *AWSMigrationTestQuery {
	t.Helper()

	queryFile := filepath.Join("testdata", "aws_migration_query.json")
	data, err := os.ReadFile(filepath.Clean(queryFile))
	if err != nil {
		t.Fatalf("Failed to read test query file: %v", err)
	}

	var query AWSMigrationTestQuery
	if err := json.Unmarshal(data, &query); err != nil {
		t.Fatalf("Failed to parse test query: %v", err)
	}

	return &query
}

// RunAWSMigrationTest runs the main AWS migration test
func (s *E2ETestSuite) RunAWSMigrationTest(t *testing.T, query *AWSMigrationTestQuery) {
	t.Helper()

	if s.testConfig.VerboseLogging {
		t.Logf("Running AWS migration test with query: %s", query.Text)
	}

	// Reset mock server
	s.mockTeamsServer.Reset()

	// Send message to Teams bot
	start := time.Now()
	_, err := s.teamsBotClient.SendMessage(t, query.Text, s.mockTeamsServer.GetWebhookURL())
	if err != nil {
		t.Fatalf("Failed to send message to Teams bot: %v", err)
	}

	// Wait for response
	response, err := s.mockTeamsServer.WaitForResponse(
		time.Duration(query.ExpectedResponse.MaxResponseTimeSeconds) * time.Second)
	if err != nil {
		t.Fatalf("Failed to receive response from Teams bot: %v", err)
	}

	totalDuration := time.Since(start)

	// Validate response
	s.ValidateResponse(t, response, query, totalDuration)

	if s.testConfig.VerboseLogging {
		t.Logf("AWS migration test completed successfully in %v", totalDuration)
	}
}

// ValidateResponse validates the received response against expected criteria
func (s *E2ETestSuite) ValidateResponse(
	t *testing.T, response *AdaptiveCardResponse, query *AWSMigrationTestQuery, duration time.Duration) {
	t.Helper()

	if s.testConfig.VerboseLogging {
		t.Logf("Validating response (duration: %v)", duration)
	}

	// Validate response structure
	s.validateResponseStructure(t, response)

	// Validate performance requirement
	s.validatePerformance(t, duration, query.ExpectedResponse.MaxResponseTimeSeconds)

	if response.ParsedContent == nil {
		t.Fatal("Response does not contain parsed content")
	}

	// Validate content quality
	s.validateContentQuality(t, response.ParsedContent, query.ExpectedResponse)

	// Validate diagram presence and quality
	s.validateDiagram(t, response.ParsedContent, query.ExpectedResponse)

	// Validate code snippets
	s.validateCodeSnippets(t, response.ParsedContent, query.ExpectedResponse)

	// Validate sources
	s.validateSources(t, response.ParsedContent, query.ExpectedResponse)

	// Validate adaptive card actions
	s.validateActions(t, response.ParsedContent)
}

// validateResponseStructure validates the basic response structure
func (s *E2ETestSuite) validateResponseStructure(t *testing.T, response *AdaptiveCardResponse) {
	t.Helper()

	if response.Type != "message" {
		t.Errorf("Expected response type 'message', got '%s'", response.Type)
	}

	if len(response.Attachments) == 0 {
		t.Fatal("Response contains no attachments")
	}

	attachment := response.Attachments[0]
	if attachment.ContentType != "application/vnd.microsoft.card.adaptive" {
		t.Errorf("Expected content type 'application/vnd.microsoft.card.adaptive', got '%s'", attachment.ContentType)
	}

	if attachment.Content == nil {
		t.Fatal("Attachment content is nil")
	}
}

// validatePerformance validates the response time requirement
func (s *E2ETestSuite) validatePerformance(t *testing.T, duration time.Duration, maxSeconds int) {
	t.Helper()

	maxDuration := time.Duration(maxSeconds) * time.Second
	if duration > maxDuration {
		t.Errorf("Response time exceeded maximum: %v > %v", duration, maxDuration)
	} else if s.testConfig.VerboseLogging {
		t.Logf("Response time within acceptable range: %v", duration)
	}
}

// validateContentQuality validates the content quality against expected criteria
func (s *E2ETestSuite) validateContentQuality(t *testing.T, content *ParsedCardContent, expected ExpectedResponse) {
	t.Helper()

	// Validate main text length
	if len(content.MainText) < expected.MinResponseLength {
		t.Errorf("Response text too short: %d < %d", len(content.MainText), expected.MinResponseLength)
	}

	// Validate keyword presence
	mainTextLower := strings.ToLower(content.MainText)
	for _, keyword := range expected.ContainsKeywords {
		if !strings.Contains(mainTextLower, strings.ToLower(keyword)) {
			t.Errorf("Response does not contain expected keyword: %s", keyword)
		}
	}

	// Validate AWS-specific content
	s.validateAWSSpecificContent(t, content, expected)
}

// validateAWSSpecificContent validates AWS-specific content requirements
func (s *E2ETestSuite) validateAWSSpecificContent(t *testing.T, content *ParsedCardContent, _ ExpectedResponse) {
	t.Helper()

	mainText := strings.ToLower(content.MainText)

	// Check for AWS migration specifics
	awsKeywords := []string{"aws", "migration", "ec2", "vpc", "subnet"}
	for _, keyword := range awsKeywords {
		if !strings.Contains(mainText, keyword) {
			t.Errorf("Response missing AWS migration keyword: %s", keyword)
		}
	}

	// Check for lift-and-shift specifics
	if !strings.Contains(mainText, "lift") || !strings.Contains(mainText, "shift") {
		t.Error("Response does not mention lift-and-shift migration")
	}

	// Check for VM count reference
	if !strings.Contains(mainText, "120") {
		t.Error("Response does not reference the 120 VM count from query")
	}
}

// validateDiagram validates diagram presence and quality
func (s *E2ETestSuite) validateDiagram(t *testing.T, content *ParsedCardContent, expected ExpectedResponse) {
	t.Helper()

	if expected.ContainsDiagram {
		if !content.HasDiagram {
			t.Error("Response should contain a diagram but doesn't")
		}

		if content.DiagramURL == "" {
			t.Error("Response should contain a diagram URL but doesn't")
		}

		// Validate diagram keywords in the main text (since we can't directly inspect the diagram)
		mainTextLower := strings.ToLower(content.MainText)
		for _, keyword := range expected.DiagramKeywords {
			if !strings.Contains(mainTextLower, strings.ToLower(keyword)) {
				t.Errorf("Response does not contain expected diagram-related keyword: %s", keyword)
			}
		}

		if s.testConfig.VerboseLogging {
			t.Logf("Diagram validation passed. URL: %s", content.DiagramURL)
		}
	}
}

// validateCodeSnippets validates code snippet presence and quality
func (s *E2ETestSuite) validateCodeSnippets(t *testing.T, content *ParsedCardContent, expected ExpectedResponse) {
	t.Helper()

	if len(expected.ContainsCodeSnippets) == 0 {
		return
	}

	if len(content.CodeSnippets) == 0 {
		t.Error("Response should contain code snippets but doesn't")
		return
	}

	// Check for expected code snippet types
	foundSnippetTypes := make(map[string]bool)
	for _, snippet := range content.CodeSnippets {
		foundSnippetTypes[strings.ToLower(snippet.Language)] = true

		// Validate code snippet is not empty
		if strings.TrimSpace(snippet.Code) == "" {
			t.Errorf("Code snippet for %s is empty", snippet.Language)
		}

		// Basic AWS CLI validation
		if strings.ToLower(snippet.Language) == "aws" || strings.ToLower(snippet.Language) == "cli" {
			if !strings.Contains(strings.ToLower(snippet.Code), "aws") {
				t.Errorf("AWS CLI snippet does not contain 'aws' command: %s", snippet.Code)
			}
		}
	}

	// Check that we have the expected snippet types
	for _, expectedType := range expected.ContainsCodeSnippets {
		if !foundSnippetTypes[strings.ToLower(expectedType)] {
			t.Errorf("Response missing expected code snippet type: %s", expectedType)
		}
	}

	if s.testConfig.VerboseLogging {
		t.Logf("Code snippet validation passed. Found %d snippets", len(content.CodeSnippets))
	}
}

// validateSources validates source citations
func (s *E2ETestSuite) validateSources(t *testing.T, content *ParsedCardContent, expected ExpectedResponse) {
	t.Helper()

	if len(expected.ExpectedSources) == 0 {
		return
	}

	if len(content.Sources) == 0 {
		t.Error("Response should contain source citations but doesn't")
		return
	}

	// Check for expected sources
	foundSources := make(map[string]bool)
	for _, source := range content.Sources {
		foundSources[strings.ToLower(source)] = true
	}

	for _, expectedSource := range expected.ExpectedSources {
		found := false
		for source := range foundSources {
			if strings.Contains(source, strings.ToLower(expectedSource)) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Response missing expected source: %s", expectedSource)
		}
	}

	if s.testConfig.VerboseLogging {
		t.Logf("Source validation passed. Found %d sources", len(content.Sources))
	}
}

// validateActions validates adaptive card actions
func (s *E2ETestSuite) validateActions(t *testing.T, content *ParsedCardContent) {
	t.Helper()

	if len(content.Actions) == 0 {
		t.Error("Response should contain feedback actions but doesn't")
		return
	}

	// Check for expected feedback actions
	hasPositiveFeedback := false
	hasNegativeFeedback := false

	for _, action := range content.Actions {
		if action.Type == "Action.Http" {
			if strings.Contains(strings.ToLower(action.Title), "helpful") {
				hasPositiveFeedback = true
			}
			if strings.Contains(strings.ToLower(action.Title), "not helpful") {
				hasNegativeFeedback = true
			}
		}
	}

	if !hasPositiveFeedback {
		t.Error("Response missing positive feedback action")
	}

	if !hasNegativeFeedback {
		t.Error("Response missing negative feedback action")
	}

	// Validate response ID is present
	if content.ResponseID == "" {
		t.Error("Response missing response ID for feedback correlation")
	}

	if s.testConfig.VerboseLogging {
		t.Logf("Action validation passed. Found %d actions", len(content.Actions))
	}
}

// checkServicesHealth checks if all required services are healthy
func (s *E2ETestSuite) checkServicesHealth(t *testing.T) bool {
	t.Helper()

	services := map[string]string{
		"teamsbot":   fmt.Sprintf("%s/health", s.testConfig.TeamsBotURL),
		"retrieve":   "http://localhost:8081/health",
		"synthesize": "http://localhost:8082/health",
		"websearch":  "http://localhost:8083/health",
	}

	client := &http.Client{Timeout: 5 * time.Second}
	allHealthy := true

	for serviceName, url := range services {
		resp, err := client.Get(url)
		if err != nil {
			if s.testConfig.VerboseLogging {
				t.Logf("Service %s health check failed: %v", serviceName, err)
			}
			allHealthy = false
			continue
		}
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			if s.testConfig.VerboseLogging {
				t.Logf("Service %s unhealthy: status %d", serviceName, resp.StatusCode)
			}
			allHealthy = false
		}
	}

	return allHealthy
}

// waitForServicesReady waits for all services to become ready
func (s *E2ETestSuite) waitForServicesReady(t *testing.T, timeout time.Duration) bool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if s.checkServicesHealth(t) {
				return true
			}
		}
	}
}

// Cleanup cleans up test resources
func (s *E2ETestSuite) Cleanup(t *testing.T) {
	t.Helper()

	if s.mockTeamsServer != nil {
		s.mockTeamsServer.Close()
	}

	if s.testConfig.VerboseLogging {
		t.Log("E2E test cleanup complete")
	}
}

// Utility functions

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseDurationOrDefault(value string, defaultValue time.Duration) time.Duration {
	if duration, err := time.ParseDuration(value); err == nil {
		return duration
	}
	return defaultValue
}
