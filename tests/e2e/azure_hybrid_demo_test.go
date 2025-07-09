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
// Teams bot integration, including Azure hybrid demo scenarios.
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

// AzureHybridTestQuery represents the test query structure for Azure hybrid scenarios
type AzureHybridTestQuery struct {
	Text             string                      `json:"text"`
	Type             string                      `json:"type"`
	User             User                        `json:"user"`
	Channel          Channel                     `json:"channel"`
	Timestamp        string                      `json:"timestamp"`
	ExpectedResponse AzureHybridExpectedResponse `json:"expected_response"`
}

// AzureHybridExpectedResponse represents the expected response criteria for Azure hybrid scenarios
type AzureHybridExpectedResponse struct {
	ContainsKeywords       []string             `json:"contains_keywords"`
	ContainsCodeSnippets   []string             `json:"contains_code_snippets"`
	ContainsDiagram        bool                 `json:"contains_diagram"`
	DiagramKeywords        []string             `json:"diagram_keywords"`
	ExpectedSources        []string             `json:"expected_sources"`
	MaxResponseTimeSeconds int                  `json:"max_response_time_seconds"`
	MinResponseLength      int                  `json:"min_response_length"`
	WebSearchTriggers      []string             `json:"web_search_triggers"`
	AzureSpecificContent   AzureSpecificContent `json:"azure_specific_content"`
}

// AzureSpecificContent represents Azure-specific content validation criteria
type AzureSpecificContent struct {
	ExpressRouteKeywords []string `json:"expressroute_keywords"`
	VMwareHCXKeywords    []string `json:"vmware_hcx_keywords"`
	FailoverKeywords     []string `json:"failover_keywords"`
}

// AzureHybridTestSuite represents the Azure hybrid E2E test suite
type AzureHybridTestSuite struct {
	mockTeamsServer *MockTeamsServer
	teamsBotClient  *TeamsBotClient
	testConfig      *E2ETestConfig
}

// NewAzureHybridTestSuite creates a new Azure hybrid E2E test suite
func NewAzureHybridTestSuite(t *testing.T) *AzureHybridTestSuite {
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

	return &AzureHybridTestSuite{
		mockTeamsServer: mockTeamsServer,
		teamsBotClient:  teamsBotClient,
		testConfig:      config,
	}
}

// TestAzureHybridDemo tests the complete Azure hybrid demo scenario
func TestAzureHybridDemo(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	suite := NewAzureHybridTestSuite(t)
	defer suite.Cleanup(t)

	// Setup test environment
	suite.SetupTestEnvironment(t)

	// Load test query
	query := suite.LoadTestQuery(t)

	// Execute the test
	suite.RunAzureHybridTest(t, query)
}

// SetupTestEnvironment sets up the test environment for Azure hybrid testing
func (s *AzureHybridTestSuite) SetupTestEnvironment(t *testing.T) {
	t.Helper()

	if s.testConfig.VerboseLogging {
		t.Log("Setting up Azure hybrid E2E test environment...")
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
		t.Log("Azure hybrid E2E test environment setup complete")
	}
}

// LoadTestQuery loads the Azure hybrid test query from file
func (s *AzureHybridTestSuite) LoadTestQuery(t *testing.T) *AzureHybridTestQuery {
	t.Helper()

	queryFile := filepath.Join("testdata", "azure_hybrid_query.json")
	data, err := os.ReadFile(filepath.Clean(queryFile))
	if err != nil {
		t.Fatalf("Failed to read Azure hybrid test query file: %v", err)
	}

	var query AzureHybridTestQuery
	if err := json.Unmarshal(data, &query); err != nil {
		t.Fatalf("Failed to parse Azure hybrid test query: %v", err)
	}

	return &query
}

// RunAzureHybridTest runs the main Azure hybrid test
func (s *AzureHybridTestSuite) RunAzureHybridTest(t *testing.T, query *AzureHybridTestQuery) {
	t.Helper()

	if s.testConfig.VerboseLogging {
		t.Logf("Running Azure hybrid test with query: %s", query.Text)
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
	s.ValidateAzureHybridResponse(t, response, query, totalDuration)

	if s.testConfig.VerboseLogging {
		t.Logf("Azure hybrid test completed successfully in %v", totalDuration)
	}
}

// ValidateAzureHybridResponse validates the received response against expected criteria
func (s *AzureHybridTestSuite) ValidateAzureHybridResponse(
	t *testing.T, response *AdaptiveCardResponse, query *AzureHybridTestQuery, duration time.Duration) {
	t.Helper()

	if s.testConfig.VerboseLogging {
		t.Logf("Validating Azure hybrid response (duration: %v)", duration)
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

	// Validate Azure-specific content
	s.validateAzureHybridSpecificContent(t, response.ParsedContent, query.ExpectedResponse)

	// Validate diagram presence and Azure hybrid specifics
	s.validateAzureHybridDiagram(t, response.ParsedContent, query.ExpectedResponse)

	// Validate PowerShell and Azure CLI code snippets
	s.validateAzureCodeSnippets(t, response.ParsedContent, query.ExpectedResponse)

	// Validate sources
	s.validateAzureHybridSources(t, response.ParsedContent, query.ExpectedResponse)

	// Validate web search integration
	s.validateWebSearchIntegration(t, response.ParsedContent, query.ExpectedResponse)

	// Validate adaptive card actions
	s.validateActions(t, response.ParsedContent)
}

// validateResponseStructure validates the basic response structure
func (s *AzureHybridTestSuite) validateResponseStructure(t *testing.T, response *AdaptiveCardResponse) {
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
func (s *AzureHybridTestSuite) validatePerformance(t *testing.T, duration time.Duration, maxSeconds int) {
	t.Helper()

	maxDuration := time.Duration(maxSeconds) * time.Second
	if duration > maxDuration {
		t.Errorf("Response time exceeded maximum: %v > %v", duration, maxDuration)
	} else if s.testConfig.VerboseLogging {
		t.Logf("Response time within acceptable range: %v", duration)
	}
}

// validateBasicContentQuality validates the basic content quality
func (s *AzureHybridTestSuite) validateBasicContentQuality(
	t *testing.T, content *ParsedCardContent, expected AzureHybridExpectedResponse) {
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

// validateAzureHybridSpecificContent validates Azure hybrid-specific content requirements
func (s *AzureHybridTestSuite) validateAzureHybridSpecificContent(
	t *testing.T, content *ParsedCardContent, expected AzureHybridExpectedResponse) {
	t.Helper()

	mainText := strings.ToLower(content.MainText)

	// Validate ExpressRoute content
	s.validateExpressRouteContent(t, mainText, expected.AzureSpecificContent.ExpressRouteKeywords)

	// Validate VMware HCX content
	s.validateVMwareHCXContent(t, mainText, expected.AzureSpecificContent.VMwareHCXKeywords)

	// Validate failover content
	s.validateFailoverContent(t, mainText, expected.AzureSpecificContent.FailoverKeywords)

	// Check for Azure hybrid architecture specifics
	if !strings.Contains(mainText, "azure") || !strings.Contains(mainText, "hybrid") {
		t.Error("Response does not mention Azure hybrid architecture")
	}

	// Check for on-premises reference
	if !strings.Contains(mainText, "on-premises") && !strings.Contains(mainText, "on-prem") {
		t.Error("Response does not reference on-premises environment")
	}
}

// validateExpressRouteContent validates ExpressRoute-specific content
func (s *AzureHybridTestSuite) validateExpressRouteContent(
	t *testing.T, mainText string, keywords []string) {
	t.Helper()

	for _, keyword := range keywords {
		if !strings.Contains(mainText, strings.ToLower(keyword)) {
			t.Errorf("Response missing ExpressRoute keyword: %s", keyword)
		}
	}

	// Check for ExpressRoute configuration concepts
	if !strings.Contains(mainText, "expressroute") {
		t.Error("Response does not mention ExpressRoute")
	}
}

// validateVMwareHCXContent validates VMware HCX-specific content
func (s *AzureHybridTestSuite) validateVMwareHCXContent(
	t *testing.T, mainText string, keywords []string) {
	t.Helper()

	for _, keyword := range keywords {
		if !strings.Contains(mainText, strings.ToLower(keyword)) {
			t.Errorf("Response missing VMware HCX keyword: %s", keyword)
		}
	}

	// Check for VMware HCX migration concepts
	if !strings.Contains(mainText, "hcx") {
		t.Error("Response does not mention VMware HCX")
	}
}

// validateFailoverContent validates failover-specific content
func (s *AzureHybridTestSuite) validateFailoverContent(
	t *testing.T, mainText string, keywords []string) {
	t.Helper()

	for _, keyword := range keywords {
		if !strings.Contains(mainText, strings.ToLower(keyword)) {
			t.Errorf("Response missing failover keyword: %s", keyword)
		}
	}

	// Check for active-active failover concepts
	if !strings.Contains(mainText, "active-active") && !strings.Contains(mainText, "failover") {
		t.Error("Response does not mention active-active failover")
	}
}

// validateAzureHybridDiagram validates diagram presence and Azure hybrid specifics
func (s *AzureHybridTestSuite) validateAzureHybridDiagram(
	t *testing.T, content *ParsedCardContent, expected AzureHybridExpectedResponse) {
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
			t.Logf("Azure hybrid diagram validation passed. URL: %s", content.DiagramURL)
		}
	}
}

// validateAzureCodeSnippets validates Azure-specific code snippet presence and quality
func (s *AzureHybridTestSuite) validateAzureCodeSnippets(
	t *testing.T, content *ParsedCardContent, expected AzureHybridExpectedResponse) {
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

		// Basic PowerShell validation
		if strings.ToLower(snippet.Language) == "powershell" {
			if !strings.Contains(strings.ToLower(snippet.Code), "azure") &&
				!strings.Contains(strings.ToLower(snippet.Code), "new-") &&
				!strings.Contains(strings.ToLower(snippet.Code), "get-") {
				t.Errorf("PowerShell snippet does not contain expected Azure cmdlets: %s", snippet.Code)
			}
		}

		// Basic Azure CLI validation
		if strings.ToLower(snippet.Language) == "azure" || strings.ToLower(snippet.Language) == "cli" {
			if !strings.Contains(strings.ToLower(snippet.Code), "az") {
				t.Errorf("Azure CLI snippet does not contain 'az' command: %s", snippet.Code)
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
		t.Logf("Azure code snippet validation passed. Found %d snippets", len(content.CodeSnippets))
	}
}

// validateAzureHybridSources validates Azure hybrid-specific source citations
func (s *AzureHybridTestSuite) validateAzureHybridSources(
	t *testing.T, content *ParsedCardContent, expected AzureHybridExpectedResponse) {
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
		t.Logf("Azure hybrid source validation passed. Found %d sources", len(content.Sources))
	}
}

// validateWebSearchIntegration validates web search integration for June 2025 announcements
func (s *AzureHybridTestSuite) validateWebSearchIntegration(
	t *testing.T, content *ParsedCardContent, expected AzureHybridExpectedResponse) {
	t.Helper()

	if len(expected.WebSearchTriggers) == 0 {
		return
	}

	mainTextLower := strings.ToLower(content.MainText)

	// Check for web search trigger keywords
	for _, trigger := range expected.WebSearchTriggers {
		if !strings.Contains(mainTextLower, strings.ToLower(trigger)) {
			t.Errorf("Response does not contain web search trigger: %s", trigger)
		}
	}

	// Check for fresh/recent information indicators
	if !strings.Contains(mainTextLower, "2025") {
		t.Error("Response does not contain recent/fresh information indicators")
	}

	if s.testConfig.VerboseLogging {
		t.Log("Web search integration validation passed")
	}
}

// validateActions validates adaptive card actions
func (s *AzureHybridTestSuite) validateActions(t *testing.T, content *ParsedCardContent) {
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
		t.Logf("Azure hybrid action validation passed. Found %d actions", len(content.Actions))
	}
}

// checkServicesHealth checks if all required services are healthy
func (s *AzureHybridTestSuite) checkServicesHealth(t *testing.T) bool {
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
func (s *AzureHybridTestSuite) waitForServicesReady(t *testing.T, timeout time.Duration) bool {
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
func (s *AzureHybridTestSuite) Cleanup(t *testing.T) {
	t.Helper()

	if s.mockTeamsServer != nil {
		s.mockTeamsServer.Close()
	}

	if s.testConfig.VerboseLogging {
		t.Log("Azure hybrid E2E test cleanup complete")
	}
}
