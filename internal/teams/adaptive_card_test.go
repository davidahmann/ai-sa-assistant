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

package teams

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/your-org/ai-sa-assistant/internal/synth"
)

const (
	textBlockType = "TextBlock"
)

func TestGenerateCard(t *testing.T) {
	tests := []struct {
		name        string
		response    synth.SynthesisResponse
		query       string
		diagramURL  string
		expectError bool
	}{
		{
			name: "basic response with main text only",
			response: synth.SynthesisResponse{
				MainText: "This is a basic response with just text content.",
			},
			query:       "What is cloud computing?",
			diagramURL:  "",
			expectError: false,
		},
		{
			name: "complete response with all elements",
			response: synth.SynthesisResponse{
				MainText:    "This is a comprehensive response about AWS architecture.",
				DiagramCode: "graph TD\n    A[User] --> B[Load Balancer]\n    B --> C[Web Server]",
				CodeSnippets: []synth.CodeSnippet{
					{
						Language: "terraform",
						Code:     "resource \"aws_vpc\" \"main\" {\n  cidr_block = \"10.0.0.0/16\"\n}",
					},
					{
						Language: "bash",
						Code:     "aws ec2 describe-instances --region us-west-2",
					},
				},
				Sources: []string{
					"aws-migration-guide.md",
					"https://docs.aws.amazon.com/vpc/",
				},
			},
			query:       "Design AWS migration architecture",
			diagramURL:  "https://mermaid.ink/img/abcd1234",
			expectError: false,
		},
		{
			name: "empty main text",
			response: synth.SynthesisResponse{
				MainText: "",
			},
			query:       "Empty response test",
			diagramURL:  "",
			expectError: false,
		},
		{
			name: "very long text content",
			response: synth.SynthesisResponse{
				MainText: strings.Repeat("This is a very long response that tests the handling of "+
					"large text content in Adaptive Cards. ", 100),
			},
			query:       "Long text test",
			diagramURL:  "",
			expectError: false,
		},
		{
			name: "special characters in text",
			response: synth.SynthesisResponse{
				MainText: "Response with special chars: @#$%^&*()_+{}|:<>?[]\\;',./",
			},
			query:       "Special chars test: @#$%^&*()",
			diagramURL:  "",
			expectError: false,
		},
		{
			name: "unicode characters",
			response: synth.SynthesisResponse{
				MainText: "Response with unicode: 你好世界 🌍 emojis and accents: café naïve résumé",
			},
			query:       "Unicode test: 测试 🧪",
			diagramURL:  "",
			expectError: false,
		},
		{
			name: "code snippets with various languages",
			response: synth.SynthesisResponse{
				MainText: "Code examples in multiple languages.",
				CodeSnippets: []synth.CodeSnippet{
					{
						Language: "python",
						Code:     "def hello():\n    print('Hello, World!')",
					},
					{
						Language: "json",
						Code:     "{\n  \"name\": \"test\",\n  \"value\": 123\n}",
					},
					{
						Language: "yaml",
						Code:     "apiVersion: v1\nkind: Pod\nmetadata:\n  name: test-pod",
					},
				},
			},
			query:       "Show code examples",
			diagramURL:  "",
			expectError: false,
		},
		{
			name: "multiple sources",
			response: synth.SynthesisResponse{
				MainText: "Information gathered from multiple sources.",
				Sources: []string{
					"internal-doc-1.md",
					"internal-doc-2.md",
					"https://example.com/docs",
					"https://aws.amazon.com/documentation",
					"azure-guide.pdf",
				},
			},
			query:       "Multi-source test",
			diagramURL:  "",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cardJSON, err := GenerateCard(tt.response, tt.query, tt.diagramURL)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if err != nil {
				return // Expected error case
			}

			// Validate that the result is valid JSON
			var card map[string]interface{}
			if err := json.Unmarshal([]byte(cardJSON), &card); err != nil {
				t.Errorf("Generated card is not valid JSON: %v", err)
				return
			}

			// Validate basic Adaptive Card structure
			if card["type"] != "AdaptiveCard" {
				t.Errorf("Expected type 'AdaptiveCard', got %v", card["type"])
			}

			if card["$schema"] != "http://adaptivecards.io/schemas/adaptive-card.json" {
				t.Errorf("Expected correct schema, got %v", card["$schema"])
			}

			if card["version"] != "1.5" {
				t.Errorf("Expected version '1.5', got %v", card["version"])
			}

			// Validate body exists
			body, ok := card["body"].([]interface{})
			if !ok {
				t.Errorf("Expected body to be an array")
				return
			}

			// Check that we have at least the header and query elements
			if len(body) < 2 {
				t.Errorf("Expected at least 2 body elements (header and query), got %d", len(body))
			}

			// Validate header element
			header, ok := body[0].(map[string]interface{})
			if !ok {
				t.Errorf("Expected first element to be a map")
				return
			}

			if header["type"] != textBlockType {
				t.Errorf("Expected first element type to be '%s', got %v", textBlockType, header["type"])
			}

			if header["text"] != "🤖 Cloud SA Assistant" {
				t.Errorf("Expected header text to be '🤖 Cloud SA Assistant', got %v", header["text"])
			}

			// Validate query element
			queryElement, ok := body[1].(map[string]interface{})
			if !ok {
				t.Errorf("Expected second element to be a map")
				return
			}

			if queryElement["type"] != textBlockType {
				t.Errorf("Expected second element type to be '%s', got %v", textBlockType, queryElement["type"])
			}

			expectedQueryText := "**Query:** " + tt.query
			if queryElement["text"] != expectedQueryText {
				t.Errorf("Expected query text to be '%s', got %v", expectedQueryText, queryElement["text"])
			}

			// Validate main text is present if not empty
			if tt.response.MainText != "" {
				found := false
				for _, element := range body {
					if elem, ok := element.(map[string]interface{}); ok {
						if elem["type"] == textBlockType && elem["text"] == tt.response.MainText {
							found = true
							break
						}
					}
				}
				if !found {
					t.Errorf("Main text not found in card body")
				}
			}

			// Validate diagram image is present if diagramURL is provided
			if tt.diagramURL != "" {
				found := false
				for _, element := range body {
					if elem, ok := element.(map[string]interface{}); ok {
						if elem["type"] == "Image" && elem["url"] == tt.diagramURL {
							found = true
							break
						}
					}
				}
				if !found {
					t.Errorf("Diagram image not found in card body")
				}
			}

			// Validate code snippets are present if provided
			if len(tt.response.CodeSnippets) > 0 {
				codeBlockCount := 0
				for _, element := range body {
					if elem, ok := element.(map[string]interface{}); ok {
						if elem["type"] == textBlockType {
							if text, ok := elem["text"].(string); ok && strings.Contains(text, "```") {
								codeBlockCount++
							}
						}
					}
				}
				if codeBlockCount == 0 {
					t.Errorf("Code snippets not found in card body")
				}
			}

			// Validate sources are present if provided
			if len(tt.response.Sources) > 0 {
				found := false
				for _, element := range body {
					if elem, ok := element.(map[string]interface{}); ok {
						if elem["type"] == textBlockType {
							if text, ok := elem["text"].(string); ok {
								if strings.Contains(text, "**Sources:**") {
									found = true
									break
								}
							}
						}
					}
				}
				if !found {
					t.Errorf("Sources section not found in card body")
				}
			}

			// Validate actions are present
			actions, ok := card["actions"].([]interface{})
			if !ok {
				t.Errorf("Expected actions to be an array")
				return
			}

			if len(actions) != 2 {
				t.Errorf("Expected 2 actions (positive and negative feedback), got %d", len(actions))
			}

			// Validate feedback actions
			expectedActions := []struct {
				title    string
				feedback string
			}{
				{"👍 Helpful", "positive"},
				{"👎 Not Helpful", "negative"},
			}

			for i, action := range actions {
				actionMap, ok := action.(map[string]interface{})
				if !ok {
					t.Errorf("Expected action %d to be a map", i)
					continue
				}

				if actionMap["type"] != "Action.Http" {
					t.Errorf("Expected action %d type to be 'Action.Http', got %v", i, actionMap["type"])
				}

				if actionMap["method"] != "POST" {
					t.Errorf("Expected action %d method to be 'POST', got %v", i, actionMap["method"])
				}

				if actionMap["url"] != "/teams-feedback" {
					t.Errorf("Expected action %d url to be '/teams-feedback', got %v", i, actionMap["url"])
				}

				// Validate action title
				if i < len(expectedActions) {
					if actionMap["title"] != expectedActions[i].title {
						t.Errorf("Expected action %d title to be '%s', got %v", i, expectedActions[i].title, actionMap["title"])
					}

					// Validate action body
					body, ok := actionMap["body"].(map[string]interface{})
					if !ok {
						t.Errorf("Expected action %d body to be a map", i)
						continue
					}

					if body["query"] != tt.query {
						t.Errorf("Expected action %d body query to be '%s', got %v", i, tt.query, body["query"])
					}

					if body["feedback"] != expectedActions[i].feedback {
						t.Errorf("Expected action %d body feedback to be '%s', got %v", i, expectedActions[i].feedback, body["feedback"])
					}

					// Validate response_id is present
					if _, exists := body["response_id"]; !exists {
						t.Errorf("Expected action %d body to contain response_id", i)
					}

					// Validate timestamp is present
					if _, exists := body["timestamp"]; !exists {
						t.Errorf("Expected action %d body to contain timestamp", i)
					}
				}
			}
		})
	}
}

func TestGenerateSimpleCard(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		message     string
		expectError bool
	}{
		{
			name:        "basic error card",
			title:       "Error",
			message:     "Something went wrong",
			expectError: false,
		},
		{
			name:        "empty title and message",
			title:       "",
			message:     "",
			expectError: false,
		},
		{
			name:        "long message",
			title:       "Warning",
			message:     strings.Repeat("This is a very long warning message. ", 50),
			expectError: false,
		},
		{
			name:        "special characters",
			title:       "Error: Special @#$%",
			message:     "Message with special chars: @#$%^&*()_+{}|:<>?[]\\;',./",
			expectError: false,
		},
		{
			name:        "unicode characters",
			title:       "错误",
			message:     "Unicode message: 你好世界 🌍 café naïve résumé",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cardJSON, err := GenerateSimpleCard(tt.title, tt.message)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if err != nil {
				return // Expected error case
			}

			// Validate that the result is valid JSON
			var card map[string]interface{}
			if err := json.Unmarshal([]byte(cardJSON), &card); err != nil {
				t.Errorf("Generated card is not valid JSON: %v", err)
				return
			}

			// Validate basic Adaptive Card structure
			if card["type"] != "AdaptiveCard" {
				t.Errorf("Expected type 'AdaptiveCard', got %v", card["type"])
			}

			if card["$schema"] != "http://adaptivecards.io/schemas/adaptive-card.json" {
				t.Errorf("Expected correct schema, got %v", card["$schema"])
			}

			if card["version"] != "1.5" {
				t.Errorf("Expected version '1.5', got %v", card["version"])
			}

			// Validate body exists and has correct structure
			body, ok := card["body"].([]interface{})
			if !ok {
				t.Errorf("Expected body to be an array")
				return
			}

			if len(body) != 2 {
				t.Errorf("Expected exactly 2 body elements (title and message), got %d", len(body))
			}

			// Validate title element
			titleElement, ok := body[0].(map[string]interface{})
			if !ok {
				t.Errorf("Expected first element to be a map")
				return
			}

			if titleElement["type"] != textBlockType {
				t.Errorf("Expected first element type to be '%s', got %v", textBlockType, titleElement["type"])
			}

			// Handle empty string case where JSON unmarshalling might return nil
			titleText := titleElement["text"]
			if (tt.title != "" || titleText != nil) && titleText != tt.title {
				t.Errorf("Expected title text to be '%s', got %v", tt.title, titleText)
			}

			if titleElement["color"] != "Warning" {
				t.Errorf("Expected title color to be 'Warning', got %v", titleElement["color"])
			}

			// Validate message element
			messageElement, ok := body[1].(map[string]interface{})
			if !ok {
				t.Errorf("Expected second element to be a map")
				return
			}

			if messageElement["type"] != textBlockType {
				t.Errorf("Expected second element type to be '%s', got %v", textBlockType, messageElement["type"])
			}

			// Handle empty string case where JSON unmarshalling might return nil
			messageText := messageElement["text"]
			if (tt.message != "" || messageText != nil) && messageText != tt.message {
				t.Errorf("Expected message text to be '%s', got %v", tt.message, messageText)
			}

			if messageElement["wrap"] != true {
				t.Errorf("Expected message wrap to be true, got %v", messageElement["wrap"])
			}

			// Validate no actions are present in simple card
			if actions, exists := card["actions"]; exists {
				if actionArray, ok := actions.([]interface{}); ok && len(actionArray) > 0 {
					t.Errorf("Expected simple card to have no actions, got %d actions", len(actionArray))
				}
			}
		})
	}
}

func TestCreateTeamsPayload(t *testing.T) {
	tests := []struct {
		name        string
		cardJSON    string
		expectError bool
	}{
		{
			name: "valid card JSON",
			cardJSON: `{
				"type": "AdaptiveCard",
				"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
				"version": "1.5",
				"body": [
					{
						"type": "TextBlock",
						"text": "Test Card"
					}
				]
			}`,
			expectError: false,
		},
		{
			name:        "invalid JSON",
			cardJSON:    `{"type": "AdaptiveCard", "invalid": json}`,
			expectError: true,
		},
		{
			name:        "empty JSON",
			cardJSON:    "",
			expectError: true,
		},
		{
			name:        "malformed JSON",
			cardJSON:    `{"type": "AdaptiveCard"`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payloadJSON, err := CreateTeamsPayload(tt.cardJSON)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if err != nil {
				return // Expected error case
			}

			// Validate that the result is valid JSON
			var payload map[string]interface{}
			if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
				t.Errorf("Generated payload is not valid JSON: %v", err)
				return
			}

			// Validate Teams webhook payload structure
			if payload["type"] != "message" {
				t.Errorf("Expected type 'message', got %v", payload["type"])
			}

			attachments, ok := payload["attachments"].([]interface{})
			if !ok {
				t.Errorf("Expected attachments to be an array")
				return
			}

			if len(attachments) != 1 {
				t.Errorf("Expected exactly 1 attachment, got %d", len(attachments))
			}

			attachment, ok := attachments[0].(map[string]interface{})
			if !ok {
				t.Errorf("Expected attachment to be a map")
				return
			}

			if attachment["contentType"] != "application/vnd.microsoft.card.adaptive" {
				t.Errorf("Expected contentType 'application/vnd.microsoft.card.adaptive', got %v", attachment["contentType"])
			}

			// Validate that content is present
			if _, exists := attachment["content"]; !exists {
				t.Errorf("Expected content to be present in attachment")
			}
		})
	}
}

func TestAdaptiveCardStructure(t *testing.T) {
	// Test that the AdaptiveCard struct can be marshaled/unmarshaled correctly
	card := AdaptiveCard{
		Type:    "AdaptiveCard",
		Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
		Version: "1.5",
		Body: []CardElement{
			{
				Type: "TextBlock",
				Text: "Test Text",
				Size: "Medium",
				Wrap: true,
			},
			{
				Type:    "Image",
				URL:     "https://example.com/image.png",
				AltText: "Test Image",
			},
		},
		Actions: []CardAction{
			{
				Type:   "Action.Http",
				Title:  "Test Action",
				Method: "POST",
				URL:    "/test",
				Body: map[string]interface{}{
					"key": "value",
				},
			},
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("Failed to marshal AdaptiveCard: %v", err)
	}

	// Unmarshal back to struct
	var unmarshaledCard AdaptiveCard
	if err := json.Unmarshal(jsonData, &unmarshaledCard); err != nil {
		t.Fatalf("Failed to unmarshal AdaptiveCard: %v", err)
	}

	// Validate structure
	if unmarshaledCard.Type != card.Type {
		t.Errorf("Expected Type %s, got %s", card.Type, unmarshaledCard.Type)
	}

	if unmarshaledCard.Schema != card.Schema {
		t.Errorf("Expected Schema %s, got %s", card.Schema, unmarshaledCard.Schema)
	}

	if unmarshaledCard.Version != card.Version {
		t.Errorf("Expected Version %s, got %s", card.Version, unmarshaledCard.Version)
	}

	if len(unmarshaledCard.Body) != len(card.Body) {
		t.Errorf("Expected %d body elements, got %d", len(card.Body), len(unmarshaledCard.Body))
	}

	if len(unmarshaledCard.Actions) != len(card.Actions) {
		t.Errorf("Expected %d actions, got %d", len(card.Actions), len(unmarshaledCard.Actions))
	}
}

func TestWebhookPayloadStructure(t *testing.T) {
	// Test that the WebhookPayload struct can be marshaled/unmarshaled correctly
	payload := WebhookPayload{
		Type: "message",
		Attachments: []CardAttachment{
			{
				ContentType: "application/vnd.microsoft.card.adaptive",
				Content: map[string]interface{}{
					"type": "AdaptiveCard",
					"body": []interface{}{
						map[string]interface{}{
							"type": "TextBlock",
							"text": "Test",
						},
					},
				},
			},
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal WebhookPayload: %v", err)
	}

	// Unmarshal back to struct
	var unmarshaledPayload WebhookPayload
	if err := json.Unmarshal(jsonData, &unmarshaledPayload); err != nil {
		t.Fatalf("Failed to unmarshal WebhookPayload: %v", err)
	}

	// Validate structure
	if unmarshaledPayload.Type != payload.Type {
		t.Errorf("Expected Type %s, got %s", payload.Type, unmarshaledPayload.Type)
	}

	if len(unmarshaledPayload.Attachments) != len(payload.Attachments) {
		t.Errorf("Expected %d attachments, got %d", len(payload.Attachments), len(unmarshaledPayload.Attachments))
	}
}

// Benchmark tests for performance validation
func BenchmarkGenerateCard(b *testing.B) {
	response := synth.SynthesisResponse{
		MainText:    "This is a test response for benchmarking performance.",
		DiagramCode: "graph TD\n    A[User] --> B[System]",
		CodeSnippets: []synth.CodeSnippet{
			{
				Language: "terraform",
				Code:     "resource \"aws_vpc\" \"main\" {\n  cidr_block = \"10.0.0.0/16\"\n}",
			},
		},
		Sources: []string{
			"test-doc.md",
			"https://example.com/docs",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GenerateCard(response, "benchmark test", "https://example.com/diagram.png")
		if err != nil {
			b.Fatalf("GenerateCard failed: %v", err)
		}
	}
}

func BenchmarkGenerateSimpleCard(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GenerateSimpleCard("Error", "This is a test error message")
		if err != nil {
			b.Fatalf("GenerateSimpleCard failed: %v", err)
		}
	}
}

func BenchmarkCreateTeamsPayload(b *testing.B) {
	cardJSON := `{
		"type": "AdaptiveCard",
		"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
		"version": "1.5",
		"body": [{"type": "TextBlock", "text": "Test"}]
	}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := CreateTeamsPayload(cardJSON)
		if err != nil {
			b.Fatalf("CreateTeamsPayload failed: %v", err)
		}
	}
}
