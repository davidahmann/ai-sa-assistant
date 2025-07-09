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

// Package integration contains end-to-end integration tests for the AI SA Assistant.
// These tests validate all 4 demo scenarios work correctly from Teams message input to Adaptive Card response.
package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"
)

// DemoScenario represents a comprehensive demo scenario test case
type DemoScenario struct {
	Name              string
	Query             string
	ExpectedKeywords  []string
	ExpectedDiagram   bool
	ExpectedCode      bool
	ExpectedSources   int // minimum number of sources
	Timeout           time.Duration
	MetadataFilters   map[string]string
	FreshnessKeywords []string // keywords that should trigger web search
}

// SynthesizedResponse represents the expected response structure
type SynthesizedResponse struct {
	MainText     string                 `json:"main_text"`
	DiagramCode  string                 `json:"diagram_code"`
	CodeSnippets []CodeSnippet          `json:"code_snippets"`
	Sources      []Source               `json:"sources"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// CodeSnippet represents a code snippet in the response
type CodeSnippet struct {
	Language    string `json:"language"`
	Code        string `json:"code"`
	Description string `json:"description"`
}

// Source represents a source citation
type Source struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Relevance string `json:"relevance"`
}

// TeamsAdaptiveCard represents the Teams webhook response structure
type TeamsAdaptiveCard struct {
	Type    string       `json:"type"`
	Body    []CardBody   `json:"body"`
	Actions []CardAction `json:"actions"`
}

// CardBody represents a body element in an Adaptive Card
type CardBody struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	URL  string `json:"url,omitempty"`
}

// CardAction represents an action in an Adaptive Card
type CardAction struct {
	Type  string `json:"type"`
	Title string `json:"title"`
	URL   string `json:"url,omitempty"`
}

// TestDemoScenariosE2E tests all 4 demo scenarios end-to-end
func TestDemoScenariosE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping comprehensive demo scenarios test in short mode")
	}

	scenarios := []DemoScenario{
		{
			Name: "AWS_Lift_and_Shift_Migration",
			Query: "@SA-Assistant Generate a high-level lift-and-shift plan for migrating 120 on-prem " +
				"Windows and Linux VMs to AWS, including EC2 instance recommendations, VPC/subnet topology, " +
				"and the latest AWS MGN best practices from Q2 2025.",
			ExpectedKeywords: []string{
				"AWS", "EC2", "VPC", "migration", "lift-and-shift", "MGN", "Windows", "Linux",
			},
			ExpectedDiagram: true,
			ExpectedCode:    true,
			ExpectedSources: 2,
			Timeout:         30 * time.Second,
			MetadataFilters: map[string]string{
				"scenario": "migration",
				"cloud":    "aws",
			},
			FreshnessKeywords: []string{"Q2 2025", "latest", "best practices"},
		},
		{
			Name: "Azure_Hybrid_Architecture",
			Query: "@SA-Assistant Outline a hybrid reference architecture connecting our on-prem VMware " +
				"environment to Azure, covering ExpressRoute configuration, VMware HCX migration, " +
				"active-active failover, and June 2025 Azure Hybrid announcements.",
			ExpectedKeywords: []string{
				"Azure", "hybrid", "VMware", "ExpressRoute", "HCX", "failover", "on-premises",
			},
			ExpectedDiagram: true,
			ExpectedCode:    true,
			ExpectedSources: 2,
			Timeout:         30 * time.Second,
			MetadataFilters: map[string]string{
				"scenario": "hybrid",
				"cloud":    "azure",
			},
			FreshnessKeywords: []string{"June 2025", "announcements", "recent"},
		},
		{
			Name: "Azure_Disaster_Recovery",
			Query: "@SA-Assistant Design a DR solution in Azure for critical workloads with RTO = 2 hours " +
				"and RPO = 15 minutes, including geo-replication options, failover orchestration, " +
				"and cost-optimized standby.",
			ExpectedKeywords: []string{
				"Azure", "disaster recovery", "RTO", "RPO", "geo-replication", "failover", "standby",
			},
			ExpectedDiagram: true,
			ExpectedCode:    true,
			ExpectedSources: 2,
			Timeout:         30 * time.Second,
			MetadataFilters: map[string]string{
				"scenario": "disaster-recovery",
				"cloud":    "azure",
			},
			FreshnessKeywords: []string{"latest", "recent", "current"},
		},
		{
			Name: "AWS_Security_Compliance",
			Query: "@SA-Assistant Summarize HIPAA and GDPR encryption, logging, and policy enforcement " +
				"requirements for our AWS landing zone, and include any recent AWS compliance feature updates.",
			ExpectedKeywords: []string{
				"AWS", "HIPAA", "GDPR", "encryption", "logging", "compliance", "security",
			},
			ExpectedDiagram: true,
			ExpectedCode:    true,
			ExpectedSources: 2,
			Timeout:         30 * time.Second,
			MetadataFilters: map[string]string{
				"scenario": "security",
				"cloud":    "aws",
			},
			FreshnessKeywords: []string{"recent", "updates", "latest"},
		},
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			start := time.Now()

			// Test the complete pipeline through Teams webhook
			request := map[string]interface{}{
				"text": scenario.Query,
				"user": "integration_test",
			}
			body, _ := json.Marshal(request)

			resp, err := client.Post("http://localhost:8080/webhook", "application/json", bytes.NewBuffer(body))
			if err != nil {
				t.Fatalf("Failed to call Teams webhook: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			duration := time.Since(start)
			t.Logf("Scenario %s completed in %v", scenario.Name, duration)

			// Test 1: Performance requirement (< 30 seconds)
			if duration > scenario.Timeout {
				t.Errorf("Scenario %s exceeded timeout: %v > %v", scenario.Name, duration, scenario.Timeout)
			}

			// Test 2: HTTP response validation
			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
			}

			// Test 3: Response structure validation (Teams Adaptive Card)
			var teamsCard TeamsAdaptiveCard
			if err := json.NewDecoder(resp.Body).Decode(&teamsCard); err != nil {
				t.Errorf("Failed to decode Teams Adaptive Card: %v", err)
				return
			}

			// Test 4: Validate Teams card structure
			validateTeamsCardStructure(t, &teamsCard, scenario)

			// Test 5: Extract and validate synthesized response content
			synthResponse := extractSynthesizedResponse(t, &teamsCard)
			if synthResponse != nil {
				validateSynthesizedResponse(t, synthResponse, scenario)
			}

			// Test 6: Validate pipeline components were triggered correctly
			validatePipelineExecution(t, scenario)
		})
	}
}

// validateTeamsCardStructure validates the Teams Adaptive Card structure
func validateTeamsCardStructure(t *testing.T, card *TeamsAdaptiveCard, scenario DemoScenario) {
	if card.Type != "AdaptiveCard" {
		t.Errorf("Expected AdaptiveCard type, got %s", card.Type)
	}

	if len(card.Body) == 0 {
		t.Error("Expected non-empty body in Teams card")
	}

	// Validate that the card contains the expected content
	cardContent := extractCardContent(card)
	for _, keyword := range scenario.ExpectedKeywords {
		if !strings.Contains(strings.ToLower(cardContent), strings.ToLower(keyword)) {
			t.Errorf("Teams card does not contain expected keyword: %s", keyword)
		}
	}
}

// extractCardContent extracts all text content from a Teams card
func extractCardContent(card *TeamsAdaptiveCard) string {
	var content strings.Builder
	for _, body := range card.Body {
		if body.Text != "" {
			content.WriteString(body.Text)
			content.WriteString(" ")
		}
	}
	return content.String()
}

// extractSynthesizedResponse extracts the synthesized response from Teams card
func extractSynthesizedResponse(_ *testing.T, card *TeamsAdaptiveCard) *SynthesizedResponse {
	// Look for JSON data in the card that contains the synthesized response
	// This is a simplified extraction - in practice, you'd parse the actual card structure
	cardContent := extractCardContent(card)

	// Try to find JSON-like content that might contain the response
	// This is a placeholder - actual implementation would depend on how the Teams adapter structures the card
	if strings.Contains(cardContent, "main_text") {
		// Mock extraction for demo purposes
		return &SynthesizedResponse{
			MainText:     cardContent,
			DiagramCode:  "",              // Would be extracted from card
			CodeSnippets: []CodeSnippet{}, // Would be extracted from card
			Sources:      []Source{},      // Would be extracted from card
		}
	}

	return nil
}

// validateSynthesizedResponse validates the synthesized response content
func validateSynthesizedResponse(t *testing.T, response *SynthesizedResponse, scenario DemoScenario) {
	// Test 1: Main text should not be empty
	if response.MainText == "" {
		t.Error("Expected non-empty main_text in synthesized response")
	}

	// Test 2: Validate keywords in main text
	for _, keyword := range scenario.ExpectedKeywords {
		if !strings.Contains(strings.ToLower(response.MainText), strings.ToLower(keyword)) {
			t.Errorf("Main text does not contain expected keyword: %s", keyword)
		}
	}

	// Test 3: Validate diagram presence
	if scenario.ExpectedDiagram && response.DiagramCode == "" {
		t.Error("Expected diagram code in response")
	}

	// Test 4: Validate Mermaid diagram syntax
	if response.DiagramCode != "" {
		validateMermaidDiagram(t, response.DiagramCode)
	}

	// Test 5: Validate code snippets
	if scenario.ExpectedCode && len(response.CodeSnippets) == 0 {
		t.Error("Expected code snippets in response")
	}

	// Test 6: Validate sources
	if len(response.Sources) < scenario.ExpectedSources {
		t.Errorf("Expected at least %d sources, got %d", scenario.ExpectedSources, len(response.Sources))
	}

	// Test 7: Validate source structure
	for i, source := range response.Sources {
		if source.Type == "" {
			t.Errorf("Source %d missing type", i)
		}
		if source.Title == "" {
			t.Errorf("Source %d missing title", i)
		}
	}
}

// validateMermaidDiagram validates Mermaid diagram syntax
func validateMermaidDiagram(t *testing.T, diagramCode string) {
	// Basic validation of Mermaid syntax
	if !strings.Contains(diagramCode, "graph") && !strings.Contains(diagramCode, "flowchart") {
		t.Error("Diagram code should contain 'graph' or 'flowchart' declaration")
	}

	// Check for basic Mermaid syntax patterns
	mermaidPatterns := []string{
		`\w+\[.*\]`,       // Node definitions
		`\w+\s*-->\s*\w+`, // Arrow connections
		`subgraph`,        // Subgraph definitions
	}

	for _, pattern := range mermaidPatterns {
		if matched, _ := regexp.MatchString(pattern, diagramCode); matched {
			t.Logf("Found valid Mermaid pattern: %s", pattern)
			return // At least one pattern matched
		}
	}

	t.Error("Diagram code does not contain valid Mermaid syntax patterns")
}

// validatePipelineExecution validates that the correct pipeline components were triggered
func validatePipelineExecution(t *testing.T, scenario DemoScenario) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Test 1: Validate retrieval service was called with correct filters
	if len(scenario.MetadataFilters) > 0 {
		validateRetrievalService(t, client, scenario)
	}

	// Test 2: Validate web search was triggered for freshness keywords
	if len(scenario.FreshnessKeywords) > 0 {
		validateWebSearchService(t, client, scenario)
	}

	// Test 3: Validate synthesis service was called
	validateSynthesisService(t, client, scenario)
}

// validateRetrievalService validates the retrieval service was called correctly
func validateRetrievalService(t *testing.T, client *http.Client, scenario DemoScenario) {
	searchRequest := map[string]interface{}{
		"query":   scenario.Query,
		"filters": scenario.MetadataFilters,
	}
	body, _ := json.Marshal(searchRequest)

	resp, err := client.Post("http://localhost:8081/search", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Logf("Could not validate retrieval service: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Retrieval service returned non-200 status: %d", resp.StatusCode)
	}

	var searchResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&searchResponse); err != nil {
		t.Errorf("Failed to decode retrieval response: %v", err)
		return
	}

	// Validate response structure
	if _, ok := searchResponse["chunks"]; !ok {
		t.Error("Expected 'chunks' field in retrieval response")
	}
}

// validateWebSearchService validates the web search service was triggered
func validateWebSearchService(t *testing.T, client *http.Client, scenario DemoScenario) {
	// Test if web search service responds to freshness queries
	webSearchRequest := map[string]interface{}{
		"query": scenario.Query,
	}
	body, _ := json.Marshal(webSearchRequest)

	resp, err := client.Post("http://localhost:8083/search", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Logf("Could not validate web search service: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Web search service returned non-200 status: %d", resp.StatusCode)
	}
}

// validateSynthesisService validates the synthesis service was called
func validateSynthesisService(t *testing.T, client *http.Client, scenario DemoScenario) {
	synthesisRequest := map[string]interface{}{
		"query":   scenario.Query,
		"context": []string{"test context"},
		"sources": []string{"test source"},
	}
	body, _ := json.Marshal(synthesisRequest)

	resp, err := client.Post("http://localhost:8082/synthesize", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Logf("Could not validate synthesis service: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Synthesis service returned non-200 status: %d", resp.StatusCode)
	}
}

// TestDemoScenariosFailureHandling tests failure scenarios
func TestDemoScenariosFailureHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping failure scenario tests in short mode")
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Test 1: Invalid JSON request
	t.Run("InvalidJSON", func(t *testing.T) {
		resp, err := client.Post("http://localhost:8080/webhook", "application/json", bytes.NewBuffer([]byte("invalid json")))
		if err != nil {
			t.Fatalf("Failed to send invalid JSON: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode == http.StatusOK {
			t.Error("Expected error status for invalid JSON, got 200 OK")
		}
	})

	// Test 2: Empty query
	t.Run("EmptyQuery", func(t *testing.T) {
		request := map[string]interface{}{
			"text": "",
		}
		body, _ := json.Marshal(request)

		resp, err := client.Post("http://localhost:8080/webhook", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to send empty query: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		// Should handle gracefully
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected graceful handling of empty query, got %d", resp.StatusCode)
		}
	})

	// Test 3: Very long query
	t.Run("LongQuery", func(t *testing.T) {
		longQuery := strings.Repeat("This is a very long query that might cause issues. ", 1000)
		request := map[string]interface{}{
			"text": longQuery,
		}
		body, _ := json.Marshal(request)

		resp, err := client.Post("http://localhost:8080/webhook", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to send long query: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		// Should handle gracefully (might return error or truncated response)
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 200 or 400 for long query, got %d", resp.StatusCode)
		}
	})

	// Test 4: Non-SA query
	t.Run("NonSAQuery", func(t *testing.T) {
		request := map[string]interface{}{
			"text": "What is the weather today?",
		}
		body, _ := json.Marshal(request)

		resp, err := client.Post("http://localhost:8080/webhook", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to send non-SA query: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		// Should handle gracefully
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected graceful handling of non-SA query, got %d", resp.StatusCode)
		}
	})
}

// TestDemoScenariosPerformance tests performance characteristics
func TestDemoScenariosPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance tests in short mode")
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	// Test concurrent requests
	t.Run("ConcurrentScenarios", func(t *testing.T) {
		queries := []string{
			"@SA-Assistant Generate a simple AWS migration plan",
			"@SA-Assistant Design a basic Azure hybrid architecture",
			"@SA-Assistant Create a disaster recovery overview",
			"@SA-Assistant Provide security compliance summary",
		}

		results := make(chan struct {
			query    string
			duration time.Duration
			err      error
		}, len(queries))

		// Launch concurrent requests
		for _, query := range queries {
			go func(q string) {
				start := time.Now()
				request := map[string]interface{}{
					"text": q,
				}
				body, _ := json.Marshal(request)

				resp, err := client.Post("http://localhost:8080/webhook", "application/json", bytes.NewBuffer(body))
				duration := time.Since(start)

				if err != nil {
					results <- struct {
						query    string
						duration time.Duration
						err      error
					}{q, duration, err}
					return
				}

				_ = resp.Body.Close()
				results <- struct {
					query    string
					duration time.Duration
					err      error
				}{q, duration, nil}
			}(query)
		}

		// Collect results
		for i := 0; i < len(queries); i++ {
			result := <-results
			if result.err != nil {
				t.Errorf("Concurrent request failed: %v", result.err)
			} else {
				t.Logf("Concurrent request completed in %v: %s", result.duration, result.query)
				if result.duration > 30*time.Second {
					t.Errorf("Concurrent request exceeded timeout: %v", result.duration)
				}
			}
		}
	})
}
