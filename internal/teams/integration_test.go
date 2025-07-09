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
// +build integration

package teams

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/diagram"
	"github.com/your-org/ai-sa-assistant/internal/health"
	"github.com/your-org/ai-sa-assistant/internal/synth"
	"go.uber.org/zap/zaptest"
)

// TestTeamsWorkflowIntegration tests the complete Teams interaction workflow
func TestTeamsWorkflowIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := zaptest.NewLogger(t)

	// Create mock backend services
	retrieveServer := createMockRetrieveServer()
	defer retrieveServer.Close()

	websearchServer := createMockWebSearchServer()
	defer websearchServer.Close()

	synthesizeServer := createMockSynthesizeServer()
	defer synthesizeServer.Close()

	// Create configuration
	cfg := &config.Config{
		Services: config.ServicesConfig{
			RetrieveURL:   retrieveServer.URL,
			WebSearchURL:  websearchServer.URL,
			SynthesizeURL: synthesizeServer.URL,
		},
		Teams: config.TeamsConfig{
			WebhookURL:    "https://hooks.teams.example.com/webhook",
			WebhookSecret: "", // Disabled for testing
		},
		WebSearch: config.WebSearchConfig{
			FreshnessKeywords: []string{"latest", "recent"},
		},
		Diagram: config.DiagramConfig{
			MermaidInkURL:  "https://mermaid.ink/img",
			Timeout:        30,
			EnableCaching:  false,
			MaxDiagramSize: 10240,
		},
	}

	// Initialize components
	messageParser := NewMessageParser(logger)
	webhookValidator := NewWebhookValidator(cfg.Teams.WebhookSecret, logger)
	healthManager := health.NewManager("test", "1.0.0", logger)

	diagramConfig := diagram.RendererConfig{
		MermaidInkURL:  cfg.Diagram.MermaidInkURL,
		Timeout:        30,
		EnableCaching:  false,
		MaxDiagramSize: 10240,
	}
	diagramRenderer := diagram.NewRenderer(diagramConfig, logger)

	orchestrator := NewOrchestrator(cfg, healthManager, diagramRenderer, logger)

	// Test scenarios
	testScenarios := []struct {
		name           string
		message        Message
		expectedStatus int
		shouldProcess  bool
	}{
		{
			name: "direct_message_query",
			message: Message{
				Type: "message",
				Text: "Generate a lift-and-shift plan for 120 VMs",
				From: &From{
					ID:   "user123",
					Name: "John Doe",
				},
				Conversation: &Conversation{
					ID:               "conv123",
					ConversationType: "personal",
					IsGroup:          false,
				},
			},
			expectedStatus: http.StatusOK,
			shouldProcess:  true,
		},
		{
			name: "bot_mention_in_channel",
			message: Message{
				Type: "message",
				Text: "@SA-Assistant Design hybrid architecture",
				From: &From{
					ID:   "user456",
					Name: "Jane Smith",
				},
				Conversation: &Conversation{
					ID:               "channel789",
					ConversationType: "channel",
					IsGroup:          true,
				},
			},
			expectedStatus: http.StatusOK,
			shouldProcess:  true,
		},
		{
			name: "freshness_query_with_websearch",
			message: Message{
				Type: "message",
				Text: "@SA-Assistant Latest AWS updates for containers",
				From: &From{
					ID:   "user789",
					Name: "Bob Wilson",
				},
				Conversation: &Conversation{
					ID:               "channel456",
					ConversationType: "channel",
					IsGroup:          true,
				},
			},
			expectedStatus: http.StatusOK,
			shouldProcess:  true,
		},
		{
			name: "channel_message_without_mention",
			message: Message{
				Type: "message",
				Text: "This is just a regular channel message",
				From: &From{
					ID:   "user101",
					Name: "Alice Brown",
				},
				Conversation: &Conversation{
					ID:               "channel101",
					ConversationType: "channel",
					IsGroup:          true,
				},
			},
			expectedStatus: http.StatusOK,
			shouldProcess:  false,
		},
		{
			name: "invalid_message_too_short",
			message: Message{
				Type: "message",
				Text: "hi",
				From: &From{
					ID:   "user202",
					Name: "Charlie Green",
				},
				Conversation: &Conversation{
					ID:               "conv202",
					ConversationType: "personal",
				},
			},
			expectedStatus: http.StatusBadRequest,
			shouldProcess:  false,
		},
	}

	for _, scenario := range testScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Create test HTTP request
			messageJSON, err := json.Marshal(scenario.message)
			if err != nil {
				t.Fatalf("Failed to marshal message: %v", err)
			}

			req := httptest.NewRequest("POST", "/teams-webhook", bytes.NewReader(messageJSON))
			req.Header.Set("Content-Type", "application/json")

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = req

			// Simulate the webhook handler logic
			body, err := io.ReadAll(c.Request.Body)
			if err != nil {
				t.Fatalf("Failed to read request body: %v", err)
			}

			// Restore body for JSON parsing
			c.Request.Body = io.NopCloser(bytes.NewReader(body))

			// Validate webhook security
			validationResult := webhookValidator.ValidateWebhook(c.Request, body)
			if !validationResult.Valid && scenario.expectedStatus != http.StatusUnauthorized {
				t.Logf("Webhook validation failed (expected for test): %s", validationResult.ErrorMessage)
			}

			// Parse Teams message
			var message Message
			if err := json.Unmarshal(body, &message); err != nil {
				if scenario.expectedStatus != http.StatusBadRequest {
					t.Fatalf("Failed to parse Teams message: %v", err)
				}
				return
			}

			// Parse and validate message content
			parsedQuery, err := messageParser.ParseMessage(&message)
			if err != nil {
				if scenario.expectedStatus == http.StatusBadRequest {
					// Expected error for invalid messages
					return
				}
				t.Fatalf("Failed to parse message content: %v", err)
			}

			// Check if message should be processed
			shouldProcess := messageParser.ShouldProcessMessage(parsedQuery)
			if shouldProcess != scenario.shouldProcess {
				t.Errorf("Expected shouldProcess=%v, got %v", scenario.shouldProcess, shouldProcess)
			}

			if shouldProcess {
				// Test orchestration
				ctx := context.Background()
				result := orchestrator.ProcessQuery(ctx, parsedQuery.Query)

				if result.Error != nil {
					t.Logf("Orchestration completed with error (may be expected): %v", result.Error)
				}

				if result.Response == nil && !result.FallbackUsed {
					t.Error("Expected response or fallback to be used")
				}

				if result.ExecutionTimeMs <= 0 {
					t.Error("Expected positive execution time")
				}

				// Validate response structure if available
				if result.Response != nil {
					if result.Response.MainText == "" {
						t.Error("Expected non-empty main text in response")
					}

					if len(result.Response.Sources) == 0 && !result.FallbackUsed {
						t.Error("Expected sources in non-fallback response")
					}
				}
			}
		})
	}
}

// createMockRetrieveServer creates a mock retrieve service for testing
func createMockRetrieveServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthEndpoint {
			w.WriteHeader(http.StatusOK)
			return
		}

		response := RetrieveResponse{
			Chunks: []RetrieveChunk{
				{
					Text:     "Mock content for cloud migration planning and best practices",
					Score:    0.9,
					DocID:    "mock-doc-1",
					SourceID: "mock-source-1",
					Metadata: map[string]interface{}{
						"scenario": "migration",
						"cloud":    "aws",
					},
				},
				{
					Text:     "Additional context about hybrid architectures and connectivity",
					Score:    0.8,
					DocID:    "mock-doc-2",
					SourceID: "mock-source-2",
					Metadata: map[string]interface{}{
						"scenario": "hybrid",
						"cloud":    "azure",
					},
				},
			},
			Count: 2,
			Query: "test query",
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
}

// createMockWebSearchServer creates a mock web search service for testing
func createMockWebSearchServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthEndpoint {
			w.WriteHeader(http.StatusOK)
			return
		}

		response := struct {
			Results []string `json:"results"`
		}{
			Results: []string{
				"Title: Latest AWS Container Updates Q4 2024\n" +
					"Snippet: AWS announced new ECS and EKS features including improved security and performance\n" +
					"URL: https://aws.amazon.com/blogs/containers/latest-updates",
				"Title: Azure Container Instances New Features\n" +
					"Snippet: Microsoft released new container orchestration capabilities for hybrid workloads\n" +
					"URL: https://azure.microsoft.com/en-us/blog/container-updates",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
}

// createMockSynthesizeServer creates a mock synthesis service for testing
func createMockSynthesizeServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthEndpoint {
			w.WriteHeader(http.StatusOK)
			return
		}

		response := synth.SynthesisResponse{
			MainText: `# Cloud Migration Plan

Based on your requirements, here's a comprehensive lift-and-shift migration plan:

## Assessment
- 120 VMs identified for migration
- Mixed Windows and Linux workloads
- Dependencies mapped and analyzed

## Migration Strategy
1. **Phase 1**: Non-critical development systems
2. **Phase 2**: Staging and testing environments
3. **Phase 3**: Production workloads

## AWS Architecture
- VPC with public/private subnets
- EC2 instances with right-sizing recommendations
- Enhanced security groups and NACLs

The migration will use AWS MGN for block-level replication with minimal downtime.`,
			DiagramCode: `graph TD
    subgraph "On-Premises"
        VMs[120 VMs]
    end

    subgraph "AWS Cloud"
        VPC[VPC: 10.0.0.0/16]
        PubSub[Public Subnet]
        PrivSub[Private Subnet]
        MGN[AWS MGN]
        EC2[EC2 Instances]
    end

    VMs -->|Replication| MGN
    MGN --> EC2
    VPC --> PubSub
    VPC --> PrivSub
    PrivSub --> EC2`,
			CodeSnippets: []synth.CodeSnippet{
				{
					Language: "bash",
					Code: `# Install AWS MGN agent
wget https://aws-mgn-agent.s3.amazonaws.com/latest/linux/aws-replication-installer-init.py
sudo python3 aws-replication-installer-init.py --region us-west-2`,
				},
				{
					Language: "terraform",
					Code: `resource "aws_vpc" "migration_vpc" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name = "Migration-VPC"
  }
}`,
				},
			},
			Sources: []string{
				"AWS Migration Hub Documentation",
				"EC2 Instance Types Guide",
				"VPC Best Practices",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
}

// TestWebhookValidation tests webhook security validation
func TestWebhookValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := zaptest.NewLogger(t)

	testCases := []struct {
		name          string
		webhookSecret string
		headers       map[string]string
		body          string
		expectedValid bool
	}{
		{
			name:          "validation_disabled_empty_secret",
			webhookSecret: "",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			body:          `{"type":"message","text":"test"}`,
			expectedValid: true,
		},
		{
			name:          "invalid_content_type",
			webhookSecret: "",
			headers: map[string]string{
				"Content-Type": "text/plain",
			},
			body:          `{"type":"message","text":"test"}`,
			expectedValid: false,
		},
		{
			name:          "missing_content_type",
			webhookSecret: "",
			headers:       map[string]string{},
			body:          `{"type":"message","text":"test"}`,
			expectedValid: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validator := NewWebhookValidator(tc.webhookSecret, logger)

			// Create test request
			req := httptest.NewRequest("POST", "/webhook", bytes.NewReader([]byte(tc.body)))
			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}

			result := validator.ValidateWebhook(req, []byte(tc.body))

			if result.Valid != tc.expectedValid {
				t.Errorf("Expected validation result %v, got %v (error: %s)",
					tc.expectedValid, result.Valid, result.ErrorMessage)
			}
		})
	}
}

// TestMessageParsingIntegration tests end-to-end message parsing scenarios
func TestMessageParsingIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := zaptest.NewLogger(t)
	parser := NewMessageParser(logger)

	realWorldMessages := []struct {
		name     string
		message  Message
		expected struct {
			shouldProcess   bool
			isBotMentioned  bool
			isDirectMessage bool
			queryContains   string
		}
	}{
		{
			name: "realistic_migration_query",
			message: Message{
				Type: "message",
				Text: "@SA-Assistant We need to migrate our 150 VM infrastructure from on-premises to AWS. " +
					"Can you help design a lift-and-shift strategy with minimal downtime? " +
					"Our environment includes Windows Server 2019, RHEL 8, and some legacy applications.",
				From: &From{
					ID:   "29:1a2b3c4d-5e6f-7890-1234-567890abcdef",
					Name: "Sarah Johnson",
				},
				Conversation: &Conversation{
					ID:               "19:meeting_abc123@thread.v2",
					ConversationType: "channel",
					IsGroup:          true,
					TenantID:         "tenant-uuid-here",
				},
				Timestamp: "2023-12-01T10:30:00.000Z",
			},
			expected: struct {
				shouldProcess   bool
				isBotMentioned  bool
				isDirectMessage bool
				queryContains   string
			}{
				shouldProcess:   true,
				isBotMentioned:  true,
				isDirectMessage: false,
				queryContains:   "migrate",
			},
		},
		{
			name: "direct_message_dr_planning",
			message: Message{
				Type: "message",
				Text: "I need help designing a disaster recovery solution for our critical workloads in Azure. " +
					"Requirements: RTO 4 hours, RPO 1 hour.",
				From: &From{
					ID:   "29:9z8y7x6w-5v4u-3t2s-1r0q-ponmlkjihgfe",
					Name: "Michael Chen",
				},
				Conversation: &Conversation{
					ID:               "19:direct_chat_xyz789@unq.gbl.spaces",
					ConversationType: "personal",
					IsGroup:          false,
					TenantID:         "tenant-uuid-here",
				},
				Timestamp: "2023-12-01T14:20:00.000Z",
			},
			expected: struct {
				shouldProcess   bool
				isBotMentioned  bool
				isDirectMessage bool
				queryContains   string
			}{
				shouldProcess:   true,
				isBotMentioned:  false,
				isDirectMessage: true,
				queryContains:   "disaster recovery",
			},
		},
	}

	for _, scenario := range realWorldMessages {
		t.Run(scenario.name, func(t *testing.T) {
			parsedQuery, err := parser.ParseMessage(&scenario.message)
			if err != nil {
				t.Fatalf("Failed to parse realistic message: %v", err)
			}

			// Validate parsing results
			if parsedQuery.IsBotMentioned != scenario.expected.isBotMentioned {
				t.Errorf("Expected IsBotMentioned=%v, got %v",
					scenario.expected.isBotMentioned, parsedQuery.IsBotMentioned)
			}

			if parsedQuery.IsDirectMessage != scenario.expected.isDirectMessage {
				t.Errorf("Expected IsDirectMessage=%v, got %v",
					scenario.expected.isDirectMessage, parsedQuery.IsDirectMessage)
			}

			shouldProcess := parser.ShouldProcessMessage(parsedQuery)
			if shouldProcess != scenario.expected.shouldProcess {
				t.Errorf("Expected ShouldProcessMessage=%v, got %v",
					scenario.expected.shouldProcess, shouldProcess)
			}

			// Validate query content
			if !contains([]string{parsedQuery.Query}, scenario.expected.queryContains) {
				t.Errorf("Expected query to contain '%s', got: %s",
					scenario.expected.queryContains, parsedQuery.Query)
			}

			// Validate user and conversation data extraction
			if parsedQuery.UserID == "" {
				t.Error("Expected non-empty UserID")
			}

			if parsedQuery.ConversationID == "" {
				t.Error("Expected non-empty ConversationID")
			}

			if parsedQuery.Timestamp == "" {
				t.Error("Expected non-empty Timestamp")
			}
		})
	}
}

// contains checks if any string in the slice contains the substring
func contains(slice []string, substring string) bool {
	for _, s := range slice {
		if strings.Contains(strings.ToLower(s), strings.ToLower(substring)) {
			return true
		}
	}
	return false
}
