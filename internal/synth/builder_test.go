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
	"strings"
	"testing"
)

func TestBuildPrompt(t *testing.T) {
	tests := []struct {
		name             string
		query            string
		contextItems     []ContextItem
		webResults       []string
		expectedContains []string
	}{
		{
			name:         "Basic prompt with query only",
			query:        "What is AWS EC2?",
			contextItems: []ContextItem{},
			webResults:   []string{},
			expectedContains: []string{
				"User Query: What is AWS EC2?",
				"Solutions Architect assistant",
				"[source_id]",
			},
		},
		{
			name:  "Prompt with context items",
			query: "How to deploy microservices?",
			contextItems: []ContextItem{
				{Content: "Microservices are distributed systems", SourceID: "doc-1"},
				{Content: "Use container orchestration", SourceID: "doc-2"},
			},
			webResults: []string{},
			expectedContains: []string{
				"User Query: How to deploy microservices?",
				"Internal Document Context",
				"Context 1 [doc-1]: Microservices are distributed systems",
				"Context 2 [doc-2]: Use container orchestration",
			},
		},
		{
			name:         "Prompt with web results",
			query:        "Latest AWS updates 2025",
			contextItems: []ContextItem{},
			webResults: []string{
				"AWS announced new features in 2025",
				"Lambda pricing updates",
			},
			expectedContains: []string{
				"User Query: Latest AWS updates 2025",
				"Live Web Search Results",
				"Web Result 1: AWS announced new features in 2025",
				"Web Result 2: Lambda pricing updates",
			},
		},
		{
			name:  "Complete prompt with all components",
			query: "Design a scalable architecture",
			contextItems: []ContextItem{
				{Content: "Use load balancers", SourceID: "arch-1", Score: 0.9, Priority: 1},
				{Content: "Consider auto-scaling", SourceID: "arch-2", Score: 0.8, Priority: 2},
			},
			webResults: []string{"Latest scaling patterns"},
			expectedContains: []string{
				"User Query: Design a scalable architecture",
				"Internal Document Context",
				"Context 1 [arch-2]: Consider auto-scaling", // Higher priority (2) comes first
				"Context 2 [arch-1]: Use load balancers",    // Lower priority (1) comes second
				"Live Web Search Results",
				"Web Result 1: Latest scaling patterns",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildPrompt(tt.query, tt.contextItems, tt.webResults)

			for _, expected := range tt.expectedContains {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected prompt to contain '%s', but it didn't. Got: %s", expected, result)
				}
			}
		})
	}
}

func TestBuildPromptWithConfig(t *testing.T) {
	tests := []struct {
		name             string
		query            string
		contextItems     []ContextItem
		webResults       []string
		config           PromptConfig
		expectedContains []string
	}{
		{
			name:  "Technical query with technical prompt",
			query: "How to configure Kubernetes clusters?",
			contextItems: []ContextItem{
				{Content: "K8s best practices", SourceID: "k8s-1"},
			},
			webResults: []string{},
			config: PromptConfig{
				MaxTokens:       2000,
				MaxContextItems: 5,
				MaxWebResults:   3,
				QueryType:       TechnicalQuery,
			},
			expectedContains: []string{
				"TECHNICAL FOCUS",
				"technical implementation details",
				"configuration examples",
			},
		},
		{
			name:  "Business query with business prompt",
			query: "What is the ROI of cloud migration?",
			contextItems: []ContextItem{
				{Content: "Cloud cost analysis", SourceID: "cost-1"},
			},
			webResults: []string{},
			config: PromptConfig{
				MaxTokens:       2000,
				MaxContextItems: 5,
				MaxWebResults:   3,
				QueryType:       BusinessQuery,
			},
			expectedContains: []string{
				"BUSINESS FOCUS",
				"business value",
				"cost considerations",
				"ROI analysis",
			},
		},
		{
			name:  "Limited context items",
			query: "General query",
			contextItems: []ContextItem{
				{Content: "Item 1", SourceID: "id-1", Priority: 1},
				{Content: "Item 2", SourceID: "id-2", Priority: 2},
				{Content: "Item 3", SourceID: "id-3", Priority: 3},
			},
			webResults: []string{},
			config: PromptConfig{
				MaxTokens:       4000,
				MaxContextItems: 2,
				MaxWebResults:   3,
				QueryType:       GeneralQuery,
			},
			expectedContains: []string{
				"Context 1 [id-3]: Item 3", // Higher priority first
				"Context 2 [id-2]: Item 2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildPromptWithConfig(tt.query, tt.contextItems, tt.webResults, tt.config)

			for _, expected := range tt.expectedContains {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected prompt to contain '%s', but it didn't. Got: %s", expected, result)
				}
			}
		})
	}
}

func TestDetectQueryType(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected QueryType
	}{
		{
			name:     "Technical query with architecture keyword",
			query:    "Design a microservices architecture using AWS",
			expected: TechnicalQuery,
		},
		{
			name:     "Technical query with deployment keyword",
			query:    "How to deploy Kubernetes clusters?",
			expected: TechnicalQuery,
		},
		{
			name:     "Business query with cost keyword",
			query:    "What is the cost of cloud migration?",
			expected: BusinessQuery,
		},
		{
			name:     "Business query with ROI keyword",
			query:    "Calculate ROI for our cloud strategy",
			expected: BusinessQuery,
		},
		{
			name:     "Mixed query favoring technical",
			query:    "Deploy cost-effective AWS architecture",
			expected: TechnicalQuery,
		},
		{
			name:     "General query with no specific keywords",
			query:    "Help me understand cloud computing",
			expected: GeneralQuery,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectQueryType(tt.query)
			if result != tt.expected {
				t.Errorf("Expected query type %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestPrioritizeContext(t *testing.T) {
	tests := []struct {
		name         string
		contextItems []ContextItem
		maxItems     int
		expected     []string // Expected source IDs in order
	}{
		{
			name: "Prioritize by priority then score",
			contextItems: []ContextItem{
				{SourceID: "low-pri", Priority: 1, Score: 0.9},
				{SourceID: "high-pri", Priority: 3, Score: 0.7},
				{SourceID: "med-pri", Priority: 2, Score: 0.8},
			},
			maxItems: 2,
			expected: []string{"high-pri", "med-pri"},
		},
		{
			name: "Same priority, prioritize by score",
			contextItems: []ContextItem{
				{SourceID: "low-score", Priority: 1, Score: 0.5},
				{SourceID: "high-score", Priority: 1, Score: 0.9},
				{SourceID: "med-score", Priority: 1, Score: 0.7},
			},
			maxItems: 2,
			expected: []string{"high-score", "med-score"},
		},
		{
			name: "No limiting when under max",
			contextItems: []ContextItem{
				{SourceID: "item1", Priority: 1, Score: 0.5},
				{SourceID: "item2", Priority: 2, Score: 0.6},
			},
			maxItems: 5,
			expected: []string{"item2", "item1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PrioritizeContext(tt.contextItems, tt.maxItems)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d items, got %d", len(tt.expected), len(result))
				return
			}

			for i, expected := range tt.expected {
				if result[i].SourceID != expected {
					t.Errorf("Expected item %d to be %s, got %s", i, expected, result[i].SourceID)
				}
			}
		})
	}
}

func TestLimitWebResults(t *testing.T) {
	tests := []struct {
		name       string
		webResults []string
		maxResults int
		expected   int
	}{
		{
			name:       "Limit results when over max",
			webResults: []string{"result1", "result2", "result3", "result4"},
			maxResults: 2,
			expected:   2,
		},
		{
			name:       "No limiting when under max",
			webResults: []string{"result1", "result2"},
			maxResults: 5,
			expected:   2,
		},
		{
			name:       "Empty results",
			webResults: []string{},
			maxResults: 3,
			expected:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := LimitWebResults(tt.webResults, tt.maxResults)
			if len(result) != tt.expected {
				t.Errorf("Expected %d results, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "Empty text",
			text:     "",
			expected: 0,
		},
		{
			name:     "Short text",
			text:     "Hello world",
			expected: 2, // 11 chars / 4 = 2.75 -> 2
		},
		{
			name:     "Longer text",
			text:     "This is a longer text that should be estimated for tokens",
			expected: 14, // 57 chars / 4 = 14.25 -> 14
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EstimateTokens(tt.text)
			if result != tt.expected {
				t.Errorf("Expected %d tokens, got %d", tt.expected, result)
			}
		})
	}
}

func TestTruncateToTokenLimit(t *testing.T) {
	tests := []struct {
		name            string
		text            string
		maxTokens       int
		expectTruncated bool
	}{
		{
			name:            "No truncation needed",
			text:            "Short text",
			maxTokens:       100,
			expectTruncated: false,
		},
		{
			name:            "Truncation needed",
			text:            strings.Repeat("a", 1000),
			maxTokens:       10,
			expectTruncated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateToTokenLimit(tt.text, tt.maxTokens)

			if tt.expectTruncated {
				if !strings.Contains(result, "Context truncated") {
					t.Errorf("Expected truncated text to contain truncation notice")
				}
			} else {
				if result != tt.text {
					t.Errorf("Expected text to remain unchanged")
				}
			}
		})
	}
}

func TestValidatePrompt(t *testing.T) {
	tests := []struct {
		name        string
		prompt      string
		expectError bool
	}{
		{
			name:        "Empty prompt",
			prompt:      "",
			expectError: true,
		},
		{
			name:        "Too short prompt",
			prompt:      "Short",
			expectError: true,
		},
		{
			name:        "Missing user query",
			prompt:      strings.Repeat("a", 200),
			expectError: true,
		},
		{
			name:        "Missing SA persona",
			prompt:      "User Query: test " + strings.Repeat("a", 200),
			expectError: true,
		},
		{
			name:        "Missing citation instructions",
			prompt:      "User Query: test Solutions Architect " + strings.Repeat("a", 200),
			expectError: true,
		},
		{
			name:        "Missing Mermaid instructions",
			prompt:      "User Query: test Solutions Architect [source_id] " + strings.Repeat("a", 200),
			expectError: true,
		},
		{
			name:        "Missing graph TD instructions",
			prompt:      "User Query: test Solutions Architect [source_id] MERMAID.JS DIAGRAM GENERATION INSTRUCTIONS " + strings.Repeat("a", 200),
			expectError: true,
		},
		{
			name:        "Missing mermaid code block instructions",
			prompt:      "User Query: test Solutions Architect [source_id] MERMAID.JS DIAGRAM GENERATION INSTRUCTIONS graph TD " + strings.Repeat("a", 200),
			expectError: true,
		},
		{
			name:        "Valid prompt with all requirements",
			prompt:      "User Query: test Solutions Architect [source_id] MERMAID.JS DIAGRAM GENERATION INSTRUCTIONS graph TD ```mermaid " + strings.Repeat("a", 200),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePrompt(tt.prompt)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestDefaultPromptConfig(t *testing.T) {
	config := DefaultPromptConfig()

	if config.MaxTokens != 4000 {
		t.Errorf("Expected MaxTokens to be 4000, got %d", config.MaxTokens)
	}

	if config.MaxContextItems != 10 {
		t.Errorf("Expected MaxContextItems to be 10, got %d", config.MaxContextItems)
	}

	if config.MaxWebResults != 5 {
		t.Errorf("Expected MaxWebResults to be 5, got %d", config.MaxWebResults)
	}

	if config.QueryType != GeneralQuery {
		t.Errorf("Expected QueryType to be GeneralQuery, got %v", config.QueryType)
	}
}

func TestDetectArchitectureQuery(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected bool
	}{
		{
			name:     "AWS architecture query",
			query:    "Design a scalable AWS architecture for microservices",
			expected: true,
		},
		{
			name:     "Azure migration query",
			query:    "Plan a lift-and-shift migration to Azure",
			expected: true,
		},
		{
			name:     "Disaster recovery query",
			query:    "Create a disaster recovery plan for our infrastructure",
			expected: true,
		},
		{
			name:     "Network topology query",
			query:    "Design VPC network topology with subnets",
			expected: true,
		},
		{
			name:     "Kubernetes deployment query",
			query:    "How to deploy applications on Kubernetes cluster",
			expected: true,
		},
		{
			name:     "CI/CD pipeline query",
			query:    "Set up CI/CD pipeline for automated deployments",
			expected: true,
		},
		{
			name:     "Database integration query",
			query:    "Integrate database with our API services",
			expected: true,
		},
		{
			name:     "Pure business query",
			query:    "What is the ROI of our cloud investment?",
			expected: true, // Contains "cloud" keyword
		},
		{
			name:     "General question",
			query:    "What is cloud computing?",
			expected: true, // Contains "cloud" keyword
		},
		{
			name:     "Pricing question",
			query:    "How much does S3 storage cost?",
			expected: true, // Contains "storage" keyword
		},
		{
			name:     "Pure business query without architecture",
			query:    "What is the ROI of our marketing investment?",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectArchitectureQuery(tt.query)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for query: %s", tt.expected, result, tt.query)
			}
		})
	}
}

func TestIsBusinessOnlyQuery(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected bool
	}{
		{
			name:     "Pure cost question",
			query:    "What is the cost of migrating to cloud?",
			expected: false, // Contains "migrating" (architecture keyword)
		},
		{
			name:     "ROI question without architecture",
			query:    "Calculate ROI for our IT investment",
			expected: true,
		},
		{
			name:     "Business case question",
			query:    "Create a business case for digital transformation",
			expected: true,
		},
		{
			name:     "Timeline question",
			query:    "What is the timeline for project completion?",
			expected: true,
		},
		{
			name:     "Compliance question without architecture",
			query:    "What are the compliance requirements for our industry?",
			expected: false, // Contains "compliance" keyword which is in architecture list
		},
		{
			name:     "Mixed business and technical",
			query:    "What is the cost of deploying AWS infrastructure?",
			expected: false, // Contains "deploying" and "AWS" (architecture keywords)
		},
		{
			name:     "Technical architecture question",
			query:    "Design a scalable microservices architecture",
			expected: false,
		},
		{
			name:     "General question",
			query:    "What is cloud computing?",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsBusinessOnlyQuery(tt.query)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for query: %s", tt.expected, result, tt.query)
			}
		})
	}
}

func TestMermaidInstructionsInPrompt(t *testing.T) {
	tests := []struct {
		name                 string
		query                string
		queryType            QueryType
		expectedInstructions []string
	}{
		{
			name:      "Technical architecture query",
			query:     "Design AWS microservices architecture",
			queryType: TechnicalQuery,
			expectedInstructions: []string{
				"MERMAID.JS DIAGRAM GENERATION INSTRUCTIONS",
				"graph TD",
				"```mermaid",
				"AWS Architecture Diagrams",
				"Azure Architecture Diagrams",
				"Hybrid Cloud Architecture Diagrams",
				"Diagram Quality Requirements",
				"Node Formatting Guidelines",
				"Fallback Instructions",
			},
		},
		{
			name:      "Business query",
			query:     "What is the cost of cloud migration?",
			queryType: BusinessQuery,
			expectedInstructions: []string{
				"MERMAID.JS DIAGRAM GENERATION INSTRUCTIONS",
				"graph TD",
				"```mermaid",
				"BUSINESS FOCUS",
			},
		},
		{
			name:      "General query",
			query:     "Explain cloud computing basics",
			queryType: GeneralQuery,
			expectedInstructions: []string{
				"MERMAID.JS DIAGRAM GENERATION INSTRUCTIONS",
				"graph TD",
				"```mermaid",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := PromptConfig{
				MaxTokens:       4000,
				MaxContextItems: 10,
				MaxWebResults:   5,
				QueryType:       tt.queryType,
			}

			result := BuildPromptWithConfig(tt.query, []ContextItem{}, []string{}, config)

			for _, instruction := range tt.expectedInstructions {
				if !strings.Contains(result, instruction) {
					t.Errorf("Expected prompt to contain '%s', but it didn't", instruction)
				}
			}
		})
	}
}

func TestMermaidDiagramExamplesInPrompt(t *testing.T) {
	query := "Design a scalable AWS architecture"
	result := BuildPrompt(query, []ContextItem{}, []string{})

	expectedExamples := []string{
		"Example AWS Pattern:",
		"subgraph \"AWS Cloud\"",
		"ALB[Application Load Balancer]",
		"Example Azure Pattern:",
		"subgraph \"Azure Subscription\"",
		"AG[Application Gateway]",
		"Example Hybrid Pattern:",
		"subgraph \"On-Premises\"",
		"VPN Connection",
	}

	for _, example := range expectedExamples {
		if !strings.Contains(result, example) {
			t.Errorf("Expected prompt to contain example '%s', but it didn't", example)
		}
	}
}

func TestPromptValidationWithMermaidInstructions(t *testing.T) {
	// Test that a real generated prompt passes validation
	query := "Design a disaster recovery architecture for AWS"
	contextItems := []ContextItem{
		{Content: "DR best practices", SourceID: "dr-guide"},
	}
	webResults := []string{"Latest AWS DR features"}

	prompt := BuildPrompt(query, contextItems, webResults)

	err := ValidatePrompt(prompt)
	if err != nil {
		t.Errorf("Generated prompt failed validation: %v", err)
	}

	// Verify specific Mermaid instructions are present
	mermaidRequirements := []string{
		"MERMAID.JS DIAGRAM GENERATION INSTRUCTIONS",
		"graph TD",
		"```mermaid",
		"When to Generate Diagrams",
		"Cloud Architecture Diagram Conventions",
		"Diagram Quality Requirements",
		"Fallback Instructions",
	}

	for _, requirement := range mermaidRequirements {
		if !strings.Contains(prompt, requirement) {
			t.Errorf("Prompt missing required Mermaid instruction: %s", requirement)
		}
	}
}
