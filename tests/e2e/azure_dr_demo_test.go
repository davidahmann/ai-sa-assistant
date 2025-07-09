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
// Teams bot integration, including Azure disaster recovery demo scenarios.
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

// AzureDRTestQuery represents the test query structure for Azure DR scenarios
type AzureDRTestQuery struct {
	Text             string                  `json:"text"`
	Type             string                  `json:"type"`
	User             User                    `json:"user"`
	Channel          Channel                 `json:"channel"`
	Timestamp        string                  `json:"timestamp"`
	ExpectedResponse AzureDRExpectedResponse `json:"expected_response"`
}

// AzureDRExpectedResponse represents the expected response criteria for Azure DR scenarios
type AzureDRExpectedResponse struct {
	ContainsKeywords       []string               `json:"contains_keywords"`
	ContainsCodeSnippets   []string               `json:"contains_code_snippets"`
	ContainsDiagram        bool                   `json:"contains_diagram"`
	DiagramKeywords        []string               `json:"diagram_keywords"`
	ExpectedSources        []string               `json:"expected_sources"`
	MaxResponseTimeSeconds int                    `json:"max_response_time_seconds"`
	MinResponseLength      int                    `json:"min_response_length"`
	FallbackSearchTriggers []string               `json:"fallback_search_triggers"`
	AzureDRSpecificContent AzureDRSpecificContent `json:"azure_dr_specific_content"`
}

// AzureDRSpecificContent represents Azure DR-specific content validation criteria
type AzureDRSpecificContent struct {
	RTORPOKeywords           []string `json:"rto_rpo_keywords"`
	SiteRecoveryKeywords     []string `json:"site_recovery_keywords"`
	FailoverKeywords         []string `json:"failover_keywords"`
	CostOptimizationKeywords []string `json:"cost_optimization_keywords"`
	GeoReplicationKeywords   []string `json:"geo_replication_keywords"`
}

// AzureDRTestSuite represents the Azure DR E2E test suite
type AzureDRTestSuite struct {
	mockTeamsServer *MockTeamsServer
	teamsBotClient  *TeamsBotClient
	testConfig      *E2ETestConfig
}

// NewAzureDRTestSuite creates a new Azure DR E2E test suite
func NewAzureDRTestSuite(t *testing.T) *AzureDRTestSuite {
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

	return &AzureDRTestSuite{
		mockTeamsServer: mockTeamsServer,
		teamsBotClient:  teamsBotClient,
		testConfig:      config,
	}
}

// TestAzureDRDemo tests the complete Azure DR demo scenario
func TestAzureDRDemo(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	suite := NewAzureDRTestSuite(t)
	defer suite.Cleanup(t)

	// Setup test environment
	suite.SetupTestEnvironment(t)

	// Load test query
	query := suite.LoadTestQuery(t)

	// Execute the test
	suite.RunAzureDRTest(t, query)
}

// SetupTestEnvironment sets up the test environment for Azure DR testing
func (s *AzureDRTestSuite) SetupTestEnvironment(t *testing.T) {
	t.Helper()

	if s.testConfig.VerboseLogging {
		t.Log("Setting up Azure DR E2E test environment...")
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
		t.Log("Azure DR E2E test environment setup complete")
	}
}

// LoadTestQuery loads the Azure DR test query from file
func (s *AzureDRTestSuite) LoadTestQuery(t *testing.T) *AzureDRTestQuery {
	t.Helper()

	queryFile := filepath.Join("testdata", "azure_dr_query.json")
	data, err := os.ReadFile(filepath.Clean(queryFile))
	if err != nil {
		t.Fatalf("Failed to read Azure DR test query file: %v", err)
	}

	var query AzureDRTestQuery
	if err := json.Unmarshal(data, &query); err != nil {
		t.Fatalf("Failed to parse Azure DR test query: %v", err)
	}

	return &query
}

// RunAzureDRTest runs the main Azure DR test
func (s *AzureDRTestSuite) RunAzureDRTest(t *testing.T, query *AzureDRTestQuery) {
	t.Helper()

	if s.testConfig.VerboseLogging {
		t.Logf("Running Azure DR test with query: %s", query.Text)
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
	s.ValidateAzureDRResponse(t, response, query, totalDuration)

	if s.testConfig.VerboseLogging {
		t.Logf("Azure DR test completed successfully in %v", totalDuration)
	}
}

// ValidateAzureDRResponse validates the received response against expected criteria
func (s *AzureDRTestSuite) ValidateAzureDRResponse(
	t *testing.T, response *AdaptiveCardResponse, query *AzureDRTestQuery, duration time.Duration) {
	t.Helper()

	if s.testConfig.VerboseLogging {
		t.Logf("Validating Azure DR response (duration: %v)", duration)
	}

	// Validate response structure
	s.validateResponseStructure(t, response)

	// Validate performance requirement
	s.validatePerformance(t, duration, query.ExpectedResponse.MaxResponseTimeSeconds)

	if response.ParsedContent == nil {
		t.Fatal("Response does not contain parsed content")
	}

	// Validate basic content quality
	s.validateBasicContentQuality(t, response.ParsedContent, query.ExpectedResponse)

	// Validate Azure DR-specific content
	s.validateAzureDRSpecificContent(t, response.ParsedContent, query.ExpectedResponse)

	// Validate RTO/RPO requirements
	s.validateRTORPOContent(t, response.ParsedContent, query.ExpectedResponse)

	// Validate Azure Site Recovery content
	s.validateSiteRecoveryContent(t, response.ParsedContent, query.ExpectedResponse)

	// Validate cost optimization content
	s.validateCostOptimizationContent(t, response.ParsedContent, query.ExpectedResponse)

	// Validate diagram presence and DR specifics
	s.validateAzureDRDiagram(t, response.ParsedContent, query.ExpectedResponse)

	// Validate PowerShell and Azure CLI code snippets
	s.validateAzureDRCodeSnippets(t, response.ParsedContent, query.ExpectedResponse)

	// Validate sources
	s.validateAzureDRSources(t, response.ParsedContent, query.ExpectedResponse)

	// Validate fallback search integration
	s.validateFallbackSearchIntegration(t, response.ParsedContent, query.ExpectedResponse)

	// Validate adaptive card actions
	s.validateActions(t, response.ParsedContent)
}

// validateResponseStructure validates the basic response structure
func (s *AzureDRTestSuite) validateResponseStructure(t *testing.T, response *AdaptiveCardResponse) {
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
func (s *AzureDRTestSuite) validatePerformance(t *testing.T, duration time.Duration, maxSeconds int) {
	t.Helper()

	maxDuration := time.Duration(maxSeconds) * time.Second
	if duration > maxDuration {
		t.Errorf("Response time exceeded maximum: %v > %v", duration, maxDuration)
	} else if s.testConfig.VerboseLogging {
		t.Logf("Response time within acceptable range: %v", duration)
	}
}

// validateBasicContentQuality validates the basic content quality
func (s *AzureDRTestSuite) validateBasicContentQuality(
	t *testing.T, content *ParsedCardContent, expected AzureDRExpectedResponse) {
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
}

// validateAzureDRSpecificContent validates Azure DR-specific content requirements
func (s *AzureDRTestSuite) validateAzureDRSpecificContent(
	t *testing.T, content *ParsedCardContent, expected AzureDRExpectedResponse) {
	t.Helper()

	mainText := strings.ToLower(content.MainText)

	// Check for Azure DR architecture specifics
	if !strings.Contains(mainText, "azure") || !strings.Contains(mainText, "disaster recovery") {
		t.Error("Response does not mention Azure disaster recovery")
	}

	// Check for critical workloads reference
	if !strings.Contains(mainText, "critical workloads") && !strings.Contains(mainText, "critical") {
		t.Error("Response does not reference critical workloads")
	}
}

// validateRTORPOContent validates RTO/RPO-specific content
func (s *AzureDRTestSuite) validateRTORPOContent(
	t *testing.T, content *ParsedCardContent, expected AzureDRExpectedResponse) {
	t.Helper()

	mainText := strings.ToLower(content.MainText)

	for _, keyword := range expected.AzureDRSpecificContent.RTORPOKeywords {
		if !strings.Contains(mainText, strings.ToLower(keyword)) {
			t.Errorf("Response missing RTO/RPO keyword: %s", keyword)
		}
	}

	// Check for specific RTO/RPO values
	if !strings.Contains(mainText, "2 hours") && !strings.Contains(mainText, "2 hour") {
		t.Error("Response does not mention 2 hours RTO requirement")
	}

	if !strings.Contains(mainText, "15 minutes") && !strings.Contains(mainText, "15 minute") {
		t.Error("Response does not mention 15 minutes RPO requirement")
	}

	if s.testConfig.VerboseLogging {
		t.Log("RTO/RPO validation passed")
	}
}

// validateSiteRecoveryContent validates Azure Site Recovery-specific content
func (s *AzureDRTestSuite) validateSiteRecoveryContent(
	t *testing.T, content *ParsedCardContent, expected AzureDRExpectedResponse) {
	t.Helper()

	mainText := strings.ToLower(content.MainText)

	for _, keyword := range expected.AzureDRSpecificContent.SiteRecoveryKeywords {
		if !strings.Contains(mainText, strings.ToLower(keyword)) {
			t.Errorf("Response missing Site Recovery keyword: %s", keyword)
		}
	}

	// Check for Site Recovery configuration concepts
	if !strings.Contains(mainText, "site recovery") {
		t.Error("Response does not mention Azure Site Recovery")
	}

	if s.testConfig.VerboseLogging {
		t.Log("Site Recovery validation passed")
	}
}

// validateCostOptimizationContent validates cost optimization-specific content
func (s *AzureDRTestSuite) validateCostOptimizationContent(
	t *testing.T, content *ParsedCardContent, expected AzureDRExpectedResponse) {
	t.Helper()

	mainText := strings.ToLower(content.MainText)

	for _, keyword := range expected.AzureDRSpecificContent.CostOptimizationKeywords {
		if !strings.Contains(mainText, strings.ToLower(keyword)) {
			t.Errorf("Response missing cost optimization keyword: %s", keyword)
		}
	}

	// Check for cost optimization concepts
	if !strings.Contains(mainText, "cost") {
		t.Error("Response does not mention cost optimization")
	}

	if s.testConfig.VerboseLogging {
		t.Log("Cost optimization validation passed")
	}
}

// validateAzureDRDiagram validates diagram presence and Azure DR specifics
func (s *AzureDRTestSuite) validateAzureDRDiagram(
	t *testing.T, content *ParsedCardContent, expected AzureDRExpectedResponse) {
	t.Helper()

	if expected.ContainsDiagram {
		if !content.HasDiagram {
			t.Error("Response should contain a diagram but doesn't")
		}

		if content.DiagramURL == "" {
			t.Error("Response should contain a diagram URL but doesn't")
		}

		// Validate diagram keywords in the main text
		mainTextLower := strings.ToLower(content.MainText)
		for _, keyword := range expected.DiagramKeywords {
			if !strings.Contains(mainTextLower, strings.ToLower(keyword)) {
				t.Errorf("Response does not contain expected diagram-related keyword: %s", keyword)
			}
		}

		if s.testConfig.VerboseLogging {
			t.Logf("Azure DR diagram validation passed. URL: %s", content.DiagramURL)
		}
	}
}

// validateAzureDRCodeSnippets validates Azure DR-specific code snippet presence and quality
func (s *AzureDRTestSuite) validateAzureDRCodeSnippets(
	t *testing.T, content *ParsedCardContent, expected AzureDRExpectedResponse) {
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

		// Basic PowerShell validation for DR scenarios
		if strings.ToLower(snippet.Language) == "powershell" {
			if !strings.Contains(strings.ToLower(snippet.Code), "azure") &&
				!strings.Contains(strings.ToLower(snippet.Code), "recovery") &&
				!strings.Contains(strings.ToLower(snippet.Code), "failover") {
				t.Errorf("PowerShell snippet does not contain expected Azure DR cmdlets: %s", snippet.Code)
			}
		}

		// Basic Azure CLI validation for DR scenarios
		if strings.ToLower(snippet.Language) == "azure" || strings.ToLower(snippet.Language) == "cli" {
			if !strings.Contains(strings.ToLower(snippet.Code), "az") ||
				!strings.Contains(strings.ToLower(snippet.Code), "recovery") {
				t.Errorf("Azure CLI snippet does not contain expected DR commands: %s", snippet.Code)
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
		t.Logf("Azure DR code snippet validation passed. Found %d snippets", len(content.CodeSnippets))
	}
}

// validateAzureDRSources validates Azure DR-specific source citations
func (s *AzureDRTestSuite) validateAzureDRSources(
	t *testing.T, content *ParsedCardContent, expected AzureDRExpectedResponse) {
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
		t.Logf("Azure DR source validation passed. Found %d sources", len(content.Sources))
	}
}

// validateFallbackSearchIntegration validates fallback search integration
func (s *AzureDRTestSuite) validateFallbackSearchIntegration(
	t *testing.T, content *ParsedCardContent, expected AzureDRExpectedResponse) {
	t.Helper()

	if len(expected.FallbackSearchTriggers) == 0 {
		return
	}

	// This is a behavioral test - we're checking that the system would handle
	// fallback search scenarios, but we can't directly test it without
	// manipulating the retrieval system. We'll check for comprehensive content
	// that suggests the system retrieved sufficient information.

	mainText := strings.ToLower(content.MainText)

	// Check for comprehensive DR content that suggests good retrieval
	comprehensiveKeywords := []string{
		"disaster recovery", "rto", "rpo", "site recovery",
		"failover", "geo-replication", "azure",
	}

	for _, keyword := range comprehensiveKeywords {
		if !strings.Contains(mainText, keyword) {
			t.Errorf("Response lacks comprehensive content, missing: %s", keyword)
		}
	}

	// Check for good source coverage
	if len(content.Sources) < 2 {
		t.Error("Response should have multiple sources indicating good retrieval coverage")
	}

	if s.testConfig.VerboseLogging {
		t.Log("Fallback search integration validation passed")
	}
}

// validateActions validates adaptive card actions
func (s *AzureDRTestSuite) validateActions(t *testing.T, content *ParsedCardContent) {
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
		t.Logf("Azure DR action validation passed. Found %d actions", len(content.Actions))
	}
}

// checkServicesHealth checks if all required services are healthy
func (s *AzureDRTestSuite) checkServicesHealth(t *testing.T) bool {
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
func (s *AzureDRTestSuite) waitForServicesReady(t *testing.T, timeout time.Duration) bool {
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
func (s *AzureDRTestSuite) Cleanup(t *testing.T) {
	t.Helper()

	if s.mockTeamsServer != nil {
		s.mockTeamsServer.Close()
	}

	if s.testConfig.VerboseLogging {
		t.Log("Azure DR E2E test cleanup complete")
	}
}
