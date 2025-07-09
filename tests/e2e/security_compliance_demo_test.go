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
// Teams bot integration, including security compliance demo scenarios.
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

// SecurityComplianceTestQuery represents the test query structure for security compliance scenarios
type SecurityComplianceTestQuery struct {
	Text             string                             `json:"text"`
	Type             string                             `json:"type"`
	User             User                               `json:"user"`
	Channel          Channel                            `json:"channel"`
	Timestamp        string                             `json:"timestamp"`
	ExpectedResponse SecurityComplianceExpectedResponse `json:"expected_response"`
}

// SecurityComplianceExpectedResponse represents the expected response criteria for security compliance scenarios
type SecurityComplianceExpectedResponse struct {
	ContainsKeywords                  []string                          `json:"contains_keywords"`
	ContainsCodeSnippets              []string                          `json:"contains_code_snippets"`
	ContainsDiagram                   bool                              `json:"contains_diagram"`
	DiagramKeywords                   []string                          `json:"diagram_keywords"`
	ExpectedSources                   []string                          `json:"expected_sources"`
	MaxResponseTimeSeconds            int                               `json:"max_response_time_seconds"`
	MinResponseLength                 int                               `json:"min_response_length"`
	WebSearchTriggers                 []string                          `json:"web_search_triggers"`
	SecurityComplianceSpecificContent SecurityComplianceSpecificContent `json:"security_compliance_specific_content"`
}

// SecurityComplianceSpecificContent represents security compliance-specific content validation criteria
type SecurityComplianceSpecificContent struct {
	HIPAAKeywords                    []string `json:"hipaa_keywords"`
	GDPRKeywords                     []string `json:"gdpr_keywords"`
	AWSSecurityKeywords              []string `json:"aws_security_keywords"`
	ComplianceImplementationKeywords []string `json:"compliance_implementation_keywords"`
	ExecutiveChecklistKeywords       []string `json:"executive_checklist_keywords"`
}

// SecurityComplianceTestSuite represents the security compliance E2E test suite
type SecurityComplianceTestSuite struct {
	mockTeamsServer *MockTeamsServer
	teamsBotClient  *TeamsBotClient
	testConfig      *E2ETestConfig
}

// NewSecurityComplianceTestSuite creates a new security compliance E2E test suite
func NewSecurityComplianceTestSuite(t *testing.T) *SecurityComplianceTestSuite {
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

	return &SecurityComplianceTestSuite{
		mockTeamsServer: mockTeamsServer,
		teamsBotClient:  teamsBotClient,
		testConfig:      config,
	}
}

// TestSecurityComplianceDemo tests the complete security compliance demo scenario
func TestSecurityComplianceDemo(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	suite := NewSecurityComplianceTestSuite(t)
	defer suite.Cleanup(t)

	// Setup test environment
	suite.SetupTestEnvironment(t)

	// Load test query
	query := suite.LoadTestQuery(t)

	// Execute the test
	suite.RunSecurityComplianceTest(t, query)
}

// SetupTestEnvironment sets up the test environment for security compliance testing
func (s *SecurityComplianceTestSuite) SetupTestEnvironment(t *testing.T) {
	t.Helper()

	if s.testConfig.VerboseLogging {
		t.Log("Setting up security compliance E2E test environment...")
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
		t.Log("Security compliance E2E test environment setup complete")
	}
}

// LoadTestQuery loads the security compliance test query from file
func (s *SecurityComplianceTestSuite) LoadTestQuery(t *testing.T) *SecurityComplianceTestQuery {
	t.Helper()

	queryFile := filepath.Join("testdata", "security_compliance_query.json")
	data, err := os.ReadFile(filepath.Clean(queryFile))
	if err != nil {
		t.Fatalf("Failed to read security compliance test query file: %v", err)
	}

	var query SecurityComplianceTestQuery
	if err := json.Unmarshal(data, &query); err != nil {
		t.Fatalf("Failed to parse security compliance test query: %v", err)
	}

	return &query
}

// RunSecurityComplianceTest runs the main security compliance test
func (s *SecurityComplianceTestSuite) RunSecurityComplianceTest(t *testing.T, query *SecurityComplianceTestQuery) {
	t.Helper()

	if s.testConfig.VerboseLogging {
		t.Logf("Running security compliance test with query: %s", query.Text)
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
	s.ValidateSecurityComplianceResponse(t, response, query, totalDuration)

	if s.testConfig.VerboseLogging {
		t.Logf("Security compliance test completed successfully in %v", totalDuration)
	}
}

// ValidateSecurityComplianceResponse validates the received response against expected criteria
func (s *SecurityComplianceTestSuite) ValidateSecurityComplianceResponse(
	t *testing.T, response *AdaptiveCardResponse, query *SecurityComplianceTestQuery, duration time.Duration) {
	t.Helper()

	if s.testConfig.VerboseLogging {
		t.Logf("Validating security compliance response (duration: %v)", duration)
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

	// Validate security compliance-specific content
	s.validateSecurityComplianceSpecificContent(t, response.ParsedContent, query.ExpectedResponse)

	// Validate HIPAA compliance content
	s.validateHIPAAContent(t, response.ParsedContent, query.ExpectedResponse)

	// Validate GDPR compliance content
	s.validateGDPRContent(t, response.ParsedContent, query.ExpectedResponse)

	// Validate AWS security features content
	s.validateAWSSecurityContent(t, response.ParsedContent, query.ExpectedResponse)

	// Validate executive checklist format
	s.validateExecutiveChecklistContent(t, response.ParsedContent, query.ExpectedResponse)

	// Validate diagram presence and compliance specifics
	s.validateSecurityComplianceDiagram(t, response.ParsedContent, query.ExpectedResponse)

	// Validate AWS CLI and CloudFormation code snippets
	s.validateSecurityComplianceCodeSnippets(t, response.ParsedContent, query.ExpectedResponse)

	// Validate sources
	s.validateSecurityComplianceSources(t, response.ParsedContent, query.ExpectedResponse)

	// Validate web search integration
	s.validateWebSearchIntegration(t, response.ParsedContent, query.ExpectedResponse)

	// Validate adaptive card actions
	s.validateActions(t, response.ParsedContent)
}

// validateResponseStructure validates the basic response structure
func (s *SecurityComplianceTestSuite) validateResponseStructure(t *testing.T, response *AdaptiveCardResponse) {
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
func (s *SecurityComplianceTestSuite) validatePerformance(t *testing.T, duration time.Duration, maxSeconds int) {
	t.Helper()

	maxDuration := time.Duration(maxSeconds) * time.Second
	if duration > maxDuration {
		t.Errorf("Response time exceeded maximum: %v > %v", duration, maxDuration)
	} else if s.testConfig.VerboseLogging {
		t.Logf("Response time within acceptable range: %v", duration)
	}
}

// validateBasicContentQuality validates the basic content quality
func (s *SecurityComplianceTestSuite) validateBasicContentQuality(
	t *testing.T, content *ParsedCardContent, expected SecurityComplianceExpectedResponse) {
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

// validateSecurityComplianceSpecificContent validates security compliance-specific content requirements
func (s *SecurityComplianceTestSuite) validateSecurityComplianceSpecificContent(
	t *testing.T, content *ParsedCardContent, expected SecurityComplianceExpectedResponse) {
	t.Helper()

	mainText := strings.ToLower(content.MainText)

	// Check for AWS landing zone specifics
	if !strings.Contains(mainText, "aws") || !strings.Contains(mainText, "landing zone") {
		t.Error("Response does not mention AWS landing zone")
	}

	// Check for compliance assessment specifics
	if !strings.Contains(mainText, "compliance") {
		t.Error("Response does not reference compliance requirements")
	}

	// Check for encryption, logging, and policy enforcement
	requiredSections := []string{"encryption", "logging", "policy"}
	for _, section := range requiredSections {
		if !strings.Contains(mainText, section) {
			t.Errorf("Response does not mention required section: %s", section)
		}
	}
}

// validateHIPAAContent validates HIPAA-specific content
func (s *SecurityComplianceTestSuite) validateHIPAAContent(
	t *testing.T, content *ParsedCardContent, expected SecurityComplianceExpectedResponse) {
	t.Helper()

	mainText := strings.ToLower(content.MainText)

	for _, keyword := range expected.SecurityComplianceSpecificContent.HIPAAKeywords {
		if !strings.Contains(mainText, strings.ToLower(keyword)) {
			t.Errorf("Response missing HIPAA keyword: %s", keyword)
		}
	}

	// Check for HIPAA compliance specifics
	if !strings.Contains(mainText, "hipaa") {
		t.Error("Response does not mention HIPAA compliance")
	}

	// Check for PHI protection concepts
	if !strings.Contains(mainText, "protected health information") && !strings.Contains(mainText, "phi") {
		t.Error("Response does not mention PHI protection")
	}

	if s.testConfig.VerboseLogging {
		t.Log("HIPAA compliance validation passed")
	}
}

// validateGDPRContent validates GDPR-specific content
func (s *SecurityComplianceTestSuite) validateGDPRContent(
	t *testing.T, content *ParsedCardContent, expected SecurityComplianceExpectedResponse) {
	t.Helper()

	mainText := strings.ToLower(content.MainText)

	for _, keyword := range expected.SecurityComplianceSpecificContent.GDPRKeywords {
		if !strings.Contains(mainText, strings.ToLower(keyword)) {
			t.Errorf("Response missing GDPR keyword: %s", keyword)
		}
	}

	// Check for GDPR compliance specifics
	if !strings.Contains(mainText, "gdpr") {
		t.Error("Response does not mention GDPR compliance")
	}

	// Check for personal data protection concepts
	if !strings.Contains(mainText, "personal data") && !strings.Contains(mainText, "data protection") {
		t.Error("Response does not mention personal data protection")
	}

	if s.testConfig.VerboseLogging {
		t.Log("GDPR compliance validation passed")
	}
}

// validateAWSSecurityContent validates AWS security features content
func (s *SecurityComplianceTestSuite) validateAWSSecurityContent(
	t *testing.T, content *ParsedCardContent, expected SecurityComplianceExpectedResponse) {
	t.Helper()

	mainText := strings.ToLower(content.MainText)

	for _, keyword := range expected.SecurityComplianceSpecificContent.AWSSecurityKeywords {
		if !strings.Contains(mainText, strings.ToLower(keyword)) {
			t.Errorf("Response missing AWS security keyword: %s", keyword)
		}
	}

	// Check for key AWS security services
	awsSecurityServices := []string{"cloudtrail", "config", "guardduty", "kms", "iam"}
	foundServices := 0
	for _, service := range awsSecurityServices {
		if strings.Contains(mainText, service) {
			foundServices++
		}
	}

	if foundServices < 3 {
		t.Errorf("Response mentions too few AWS security services: %d < 3", foundServices)
	}

	if s.testConfig.VerboseLogging {
		t.Log("AWS security features validation passed")
	}
}

// validateExecutiveChecklistContent validates executive-friendly checklist format
func (s *SecurityComplianceTestSuite) validateExecutiveChecklistContent(
	t *testing.T, content *ParsedCardContent, expected SecurityComplianceExpectedResponse) {
	t.Helper()

	mainText := strings.ToLower(content.MainText)

	for _, keyword := range expected.SecurityComplianceSpecificContent.ExecutiveChecklistKeywords {
		if !strings.Contains(mainText, strings.ToLower(keyword)) {
			t.Errorf("Response missing executive checklist keyword: %s", keyword)
		}
	}

	// Check for executive-friendly language
	if !strings.Contains(mainText, "action") && !strings.Contains(mainText, "requirements") {
		t.Error("Response does not contain executive-friendly action items")
	}

	// Check for implementation guidance
	if !strings.Contains(mainText, "implementation") && !strings.Contains(mainText, "checklist") {
		t.Error("Response does not contain implementation guidance")
	}

	if s.testConfig.VerboseLogging {
		t.Log("Executive checklist validation passed")
	}
}

// validateSecurityComplianceDiagram validates diagram presence and security compliance specifics
func (s *SecurityComplianceTestSuite) validateSecurityComplianceDiagram(
	t *testing.T, content *ParsedCardContent, expected SecurityComplianceExpectedResponse) {
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
			t.Logf("Security compliance diagram validation passed. URL: %s", content.DiagramURL)
		}
	}
}

// validateSecurityComplianceCodeSnippets validates security compliance-specific code snippet presence and quality
func (s *SecurityComplianceTestSuite) validateSecurityComplianceCodeSnippets(
	t *testing.T, content *ParsedCardContent, expected SecurityComplianceExpectedResponse) {
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

		// Basic AWS CLI validation for compliance scenarios
		if strings.ToLower(snippet.Language) == "aws" || strings.ToLower(snippet.Language) == "cli" {
			if !strings.Contains(strings.ToLower(snippet.Code), "aws") {
				t.Errorf("AWS CLI snippet does not contain 'aws' command: %s", snippet.Code)
			}
			// Check for compliance-related AWS commands
			complianceServices := []string{"cloudtrail", "config", "guardduty", "kms", "iam"}
			hasComplianceService := false
			for _, service := range complianceServices {
				if strings.Contains(strings.ToLower(snippet.Code), service) {
					hasComplianceService = true
					break
				}
			}
			if !hasComplianceService {
				t.Errorf("AWS CLI snippet does not contain compliance-related services: %s", snippet.Code)
			}
		}

		// Basic CloudFormation validation for compliance scenarios
		if strings.ToLower(snippet.Language) == "cloudformation" || strings.ToLower(snippet.Language) == "yaml" {
			if !strings.Contains(strings.ToLower(snippet.Code), "type") &&
				!strings.Contains(strings.ToLower(snippet.Code), "properties") {
				t.Errorf("CloudFormation snippet does not contain expected structure: %s", snippet.Code)
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
		t.Logf("Security compliance code snippet validation passed. Found %d snippets", len(content.CodeSnippets))
	}
}

// validateSecurityComplianceSources validates security compliance-specific source citations
func (s *SecurityComplianceTestSuite) validateSecurityComplianceSources(
	t *testing.T, content *ParsedCardContent, expected SecurityComplianceExpectedResponse) {
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
		t.Logf("Security compliance source validation passed. Found %d sources", len(content.Sources))
	}
}

// validateWebSearchIntegration validates web search integration for recent AWS compliance updates
func (s *SecurityComplianceTestSuite) validateWebSearchIntegration(
	t *testing.T, content *ParsedCardContent, expected SecurityComplianceExpectedResponse) {
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
	if !strings.Contains(mainTextLower, "recent") && !strings.Contains(mainTextLower, "latest") &&
		!strings.Contains(mainTextLower, "new") && !strings.Contains(mainTextLower, "updates") {
		t.Error("Response does not contain recent/fresh information indicators")
	}

	if s.testConfig.VerboseLogging {
		t.Log("Web search integration validation passed")
	}
}

// validateActions validates adaptive card actions
func (s *SecurityComplianceTestSuite) validateActions(t *testing.T, content *ParsedCardContent) {
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
		t.Logf("Security compliance action validation passed. Found %d actions", len(content.Actions))
	}
}

// checkServicesHealth checks if all required services are healthy
func (s *SecurityComplianceTestSuite) checkServicesHealth(t *testing.T) bool {
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
func (s *SecurityComplianceTestSuite) waitForServicesReady(t *testing.T, timeout time.Duration) bool {
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
func (s *SecurityComplianceTestSuite) Cleanup(t *testing.T) {
	t.Helper()

	if s.mockTeamsServer != nil {
		s.mockTeamsServer.Close()
	}

	if s.testConfig.VerboseLogging {
		t.Log("Security compliance E2E test cleanup complete")
	}
}
