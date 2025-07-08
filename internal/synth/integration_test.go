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

package synth

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"
)

// Integration tests that require OpenAI API key
// Run with: go test -tags=integration

func TestPromptIntegrationWithLLM(t *testing.T) {
	// Skip if no API key is provided
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: OPENAI_API_KEY not set")
	}

	client := openai.NewClient(apiKey)

	tests := []struct {
		name             string
		query            string
		contextItems     []ContextItem
		webResults       []string
		queryType        QueryType
		expectedPatterns []string // Patterns we expect to find in LLM response
	}{
		{
			name:  "Technical AWS Architecture Query",
			query: "Design a highly available web application architecture on AWS with auto-scaling capabilities",
			contextItems: []ContextItem{
				{
					Content:  "AWS Auto Scaling helps maintain application availability and allows you to automatically add or remove EC2 instances according to defined conditions.",
					SourceID: "aws-autoscaling-doc",
					Priority: 1,
					Score:    0.9,
				},
				{
					Content:  "Application Load Balancer distributes incoming traffic across multiple EC2 instances in multiple Availability Zones.",
					SourceID: "aws-alb-doc",
					Priority: 1,
					Score:    0.85,
				},
			},
			webResults: []string{
				"AWS announced new auto-scaling features in 2025 including predictive scaling improvements",
			},
			queryType: TechnicalQuery,
			expectedPatterns: []string{
				"Auto Scaling",
				"Load Balancer",
				"[aws-autoscaling-doc]",
				"[aws-alb-doc]",
				"mermaid",
				"graph TD",
			},
		},
		{
			name:  "Business Cost Analysis Query",
			query: "What are the cost implications and ROI of migrating to cloud infrastructure?",
			contextItems: []ContextItem{
				{
					Content:  "Cloud migration typically reduces operational costs by 20-30% within the first year through improved resource utilization and reduced maintenance overhead.",
					SourceID: "cost-analysis-report",
					Priority: 1,
					Score:    0.9,
				},
				{
					Content:  "ROI calculations should include both direct cost savings (hardware, power, cooling) and indirect benefits (increased agility, faster deployment).",
					SourceID: "roi-methodology",
					Priority: 1,
					Score:    0.8,
				},
			},
			webResults: []string{
				"2025 cloud cost optimization strategies show average savings of 25% year-over-year",
			},
			queryType: BusinessQuery,
			expectedPatterns: []string{
				"cost",
				"ROI",
				"savings",
				"[cost-analysis-report]",
				"[roi-methodology]",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate prompt
			config := PromptConfig{
				MaxTokens:       6000,
				MaxContextItems: 10,
				MaxWebResults:   5,
				QueryType:       tt.queryType,
			}

			prompt := BuildPromptWithConfig(tt.query, tt.contextItems, tt.webResults, config)

			// Validate prompt before sending to LLM
			if err := ValidatePrompt(prompt); err != nil {
				t.Fatalf("Generated prompt failed validation: %v", err)
			}

			// Send to OpenAI
			resp, err := client.CreateChatCompletion(
				context.Background(),
				openai.ChatCompletionRequest{
					Model: openai.GPT4o,
					Messages: []openai.ChatCompletionMessage{
						{
							Role:    openai.ChatMessageRoleUser,
							Content: prompt,
						},
					},
					MaxTokens:   2000,
					Temperature: 0.7,
				},
			)

			if err != nil {
				t.Fatalf("OpenAI API call failed: %v", err)
			}

			if len(resp.Choices) == 0 {
				t.Fatal("No response choices returned from OpenAI")
			}

			response := resp.Choices[0].Message.Content

			// Log the response for manual inspection
			t.Logf("LLM Response for %s:\n%s", tt.name, response)

			// Check for expected patterns
			for _, pattern := range tt.expectedPatterns {
				if !strings.Contains(response, pattern) {
					t.Errorf("Expected response to contain '%s', but it didn't", pattern)
				}
			}

			// Parse and validate structured response
			parsedResponse := ParseResponse(response)

			// Validate that citations are present
			if len(parsedResponse.Sources) == 0 {
				t.Error("Expected response to contain source citations")
			}

			// Validate that main text is substantial
			if len(parsedResponse.MainText) < 200 {
				t.Error("Expected substantial main text content")
			}

			// For technical queries, expect diagrams or code
			if tt.queryType == TechnicalQuery {
				if parsedResponse.DiagramCode == "" && len(parsedResponse.CodeSnippets) == 0 {
					t.Error("Technical query should generate either diagram or code snippets")
				}
			}

			// Validate Mermaid diagram syntax if present
			if parsedResponse.DiagramCode != "" {
				if !strings.Contains(parsedResponse.DiagramCode, "graph TD") {
					t.Error("Mermaid diagram should use 'graph TD' syntax")
				}
			}

			// Validate code snippets
			for _, snippet := range parsedResponse.CodeSnippets {
				if snippet.Language == "" {
					t.Error("Code snippet should have language identifier")
				}
				if len(snippet.Code) < 10 {
					t.Error("Code snippet should have substantial content")
				}
			}

			// Add a small delay to respect rate limits
			time.Sleep(1 * time.Second)
		})
	}
}

func TestPromptTokenLimiting(t *testing.T) {
	// Test with very large context to ensure token limiting works
	largeContext := []ContextItem{}
	for i := 0; i < 20; i++ {
		largeContext = append(largeContext, ContextItem{
			Content:  strings.Repeat(fmt.Sprintf("Large content block %d. ", i), 200),
			SourceID: fmt.Sprintf("large-doc-%d", i),
			Priority: i % 3,
			Score:    float64(i) * 0.05,
		})
	}

	largeWebResults := []string{}
	for i := 0; i < 10; i++ {
		largeWebResults = append(largeWebResults, strings.Repeat(fmt.Sprintf("Large web result %d. ", i), 100))
	}

	config := PromptConfig{
		MaxTokens:       1000, // Very restrictive
		MaxContextItems: 3,
		MaxWebResults:   2,
		QueryType:       TechnicalQuery,
	}

	prompt := BuildPromptWithConfig("Test query", largeContext, largeWebResults, config)

	// Validate that token limiting worked
	estimatedTokens := EstimateTokens(prompt)
	if estimatedTokens > config.MaxTokens {
		t.Errorf("Expected prompt to be under %d tokens, but got %d", config.MaxTokens, estimatedTokens)
	}

	// Validate that context was limited
	contextCount := strings.Count(prompt, "Context ")
	if contextCount > config.MaxContextItems {
		t.Errorf("Expected max %d context items, but got %d", config.MaxContextItems, contextCount)
	}

	// Validate that web results were limited
	webResultCount := strings.Count(prompt, "Web Result ")
	if webResultCount > config.MaxWebResults {
		t.Errorf("Expected max %d web results, but got %d", config.MaxWebResults, webResultCount)
	}
}

func TestPromptOptimizationForDifferentQueryTypes(t *testing.T) {
	baseContext := []ContextItem{
		{Content: "Generic cloud content", SourceID: "generic-doc", Priority: 1, Score: 0.8},
	}
	baseWebResults := []string{"Generic web result"}

	tests := []struct {
		name             string
		query            string
		queryType        QueryType
		expectedInPrompt string
	}{
		{
			name:             "Technical query optimization",
			query:            "How to configure Kubernetes networking?",
			queryType:        TechnicalQuery,
			expectedInPrompt: "TECHNICAL FOCUS",
		},
		{
			name:             "Business query optimization",
			query:            "What is the ROI of cloud migration?",
			queryType:        BusinessQuery,
			expectedInPrompt: "BUSINESS FOCUS",
		},
		{
			name:             "General query uses standard prompt",
			query:            "Tell me about cloud computing",
			queryType:        GeneralQuery,
			expectedInPrompt: "Solutions Architect assistant",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := PromptConfig{
				MaxTokens:       6000,
				MaxContextItems: 10,
				MaxWebResults:   5,
				QueryType:       tt.queryType,
			}

			prompt := BuildPromptWithConfig(tt.query, baseContext, baseWebResults, config)

			if !strings.Contains(prompt, tt.expectedInPrompt) {
				t.Errorf("Expected prompt to contain '%s' for query type %v", tt.expectedInPrompt, tt.queryType)
			}

			// Validate prompt structure
			if err := ValidatePrompt(prompt); err != nil {
				t.Errorf("Generated prompt failed validation: %v", err)
			}
		})
	}
}

func TestContextPrioritizationInPrompt(t *testing.T) {
	// Create context items with different priorities and scores
	contextItems := []ContextItem{
		{Content: "Low priority content", SourceID: "low-pri", Priority: 1, Score: 0.5},
		{Content: "High priority content", SourceID: "high-pri", Priority: 3, Score: 0.9},
		{Content: "Medium priority content", SourceID: "med-pri", Priority: 2, Score: 0.7},
		{Content: "Another low priority", SourceID: "low-pri-2", Priority: 1, Score: 0.6},
	}

	config := PromptConfig{
		MaxTokens:       6000,
		MaxContextItems: 2, // Limit to force prioritization
		MaxWebResults:   5,
		QueryType:       GeneralQuery,
	}

	prompt := BuildPromptWithConfig("Test query", contextItems, []string{}, config)

	// Check that high priority items appear first
	if !strings.Contains(prompt, "Context 1 [high-pri]") {
		t.Error("Expected highest priority item to appear first")
	}

	if !strings.Contains(prompt, "Context 2 [med-pri]") {
		t.Error("Expected second highest priority item to appear second")
	}

	// Check that lower priority items are excluded
	if strings.Contains(prompt, "[low-pri]") {
		t.Error("Expected low priority items to be excluded due to limit")
	}
}
