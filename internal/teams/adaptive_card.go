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

// Package teams provides functionality for creating and managing Microsoft Teams
// Adaptive Cards. It handles the generation of rich, interactive cards that display
// synthesis results, architecture diagrams, and user feedback elements.
package teams

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/your-org/ai-sa-assistant/internal/synth"
)

// AdaptiveCard represents the structure of a Teams Adaptive Card
type AdaptiveCard struct {
	Type    string        `json:"type"`
	Schema  string        `json:"$schema"`
	Version string        `json:"version"`
	Body    []CardElement `json:"body"`
	Actions []CardAction  `json:"actions,omitempty"`
}

// CardElement represents an element in the card body
type CardElement struct {
	Type      string        `json:"type"`
	Text      string        `json:"text,omitempty"`
	Size      string        `json:"size,omitempty"`
	Weight    string        `json:"weight,omitempty"`
	Color     string        `json:"color,omitempty"`
	Wrap      bool          `json:"wrap,omitempty"`
	FontType  string        `json:"fontType,omitempty"`
	URL       string        `json:"url,omitempty"`
	AltText   string        `json:"altText,omitempty"`
	Spacing   string        `json:"spacing,omitempty"`
	Separator bool          `json:"separator,omitempty"`
	Items     []CardElement `json:"items,omitempty"`
}

// CardAction represents an action in the card
type CardAction struct {
	Type   string                 `json:"type"`
	Title  string                 `json:"title"`
	URL    string                 `json:"url,omitempty"`
	Method string                 `json:"method,omitempty"`
	Body   map[string]interface{} `json:"body,omitempty"`
}

// GenerateCard creates a Teams Adaptive Card from synthesis response
func GenerateCard(response synth.SynthesisResponse, query string, diagramURL string) (string, error) {
	// Generate unique response ID for correlation
	responseID := generateResponseID()
	card := AdaptiveCard{
		Type:    "AdaptiveCard",
		Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
		Version: "1.5",
		Body:    []CardElement{},
		Actions: []CardAction{},
	}

	// Header
	card.Body = append(card.Body, CardElement{
		Type:   "TextBlock",
		Text:   "🤖 Cloud SA Assistant",
		Size:   "Medium",
		Weight: "Bolder",
		Color:  "Accent",
	})

	// Query
	card.Body = append(card.Body, CardElement{
		Type:      "TextBlock",
		Text:      fmt.Sprintf("**Query:** %s", query),
		Wrap:      true,
		Spacing:   "Medium",
		Separator: true,
	})

	// Main response
	if response.MainText != "" {
		card.Body = append(card.Body, CardElement{
			Type:    "TextBlock",
			Text:    response.MainText,
			Wrap:    true,
			Spacing: "Medium",
		})
	}

	// Architecture diagram
	if diagramURL != "" {
		card.Body = append(card.Body, CardElement{
			Type:      "TextBlock",
			Text:      "**Architecture Diagram:**",
			Weight:    "Bolder",
			Spacing:   "Medium",
			Separator: true,
		})

		card.Body = append(card.Body, CardElement{
			Type:    "Image",
			URL:     diagramURL,
			AltText: "Architecture Diagram",
			Spacing: "Small",
		})
	}

	// Code snippets
	if len(response.CodeSnippets) > 0 {
		card.Body = append(card.Body, CardElement{
			Type:      "TextBlock",
			Text:      "**Code Snippets:**",
			Weight:    "Bolder",
			Spacing:   "Medium",
			Separator: true,
		})

		for _, snippet := range response.CodeSnippets {
			// Language header
			card.Body = append(card.Body, CardElement{
				Type:    "TextBlock",
				Text:    fmt.Sprintf("*%s:*", strings.ToUpper(snippet.Language[:1])+snippet.Language[1:]),
				Weight:  "Bolder",
				Spacing: "Small",
			})

			// Code block
			card.Body = append(card.Body, CardElement{
				Type:     "TextBlock",
				Text:     fmt.Sprintf("```\n%s\n```", snippet.Code),
				FontType: "Monospace",
				Wrap:     true,
				Spacing:  "Small",
			})
		}
	}

	// Sources
	if len(response.Sources) > 0 {
		card.Body = append(card.Body, CardElement{
			Type:      "TextBlock",
			Text:      "**Sources:**",
			Weight:    "Bolder",
			Spacing:   "Medium",
			Separator: true,
		})

		sourceText := "• " + strings.Join(response.Sources, "\n• ")
		card.Body = append(card.Body, CardElement{
			Type:    "TextBlock",
			Text:    sourceText,
			Wrap:    true,
			Spacing: "Small",
		})
	}

	// Feedback and regeneration actions
	card.Actions = append(card.Actions,
		CardAction{
			Type:   "Action.Http",
			Title:  "👍 Helpful",
			Method: "POST",
			URL:    "/teams-feedback",
			Body: map[string]interface{}{
				"query":       query,
				"response_id": responseID,
				"feedback":    "positive",
				"timestamp":   time.Now().Format(time.RFC3339),
			},
		},
		CardAction{
			Type:   "Action.Http",
			Title:  "👎 Not Helpful",
			Method: "POST",
			URL:    "/teams-feedback",
			Body: map[string]interface{}{
				"query":       query,
				"response_id": responseID,
				"feedback":    "negative",
				"timestamp":   time.Now().Format(time.RFC3339),
			},
		},
		CardAction{
			Type:   "Action.Http",
			Title:  "🔄 Regenerate",
			Method: "POST",
			URL:    "/teams-regenerate",
			Body: map[string]interface{}{
				"query":             query,
				"response_id":       responseID,
				"previous_response": response.MainText,
				"action":            "show_options",
				"timestamp":         time.Now().Format(time.RFC3339),
			},
		},
	)

	// Marshal to JSON
	cardJSON, err := json.MarshalIndent(card, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal adaptive card: %w", err)
	}

	return string(cardJSON), nil
}

// GenerateSimpleCard creates a simple text-only card for errors or simple responses
func GenerateSimpleCard(title, message string) (string, error) {
	card := AdaptiveCard{
		Type:    "AdaptiveCard",
		Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
		Version: "1.5",
		Body: []CardElement{
			{
				Type:   "TextBlock",
				Text:   title,
				Size:   "Medium",
				Weight: "Bolder",
				Color:  "Warning",
			},
			{
				Type:    "TextBlock",
				Text:    message,
				Wrap:    true,
				Spacing: "Medium",
			},
		},
	}

	cardJSON, err := json.MarshalIndent(card, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal simple card: %w", err)
	}

	return string(cardJSON), nil
}

// WebhookPayload represents the payload sent to Teams
type WebhookPayload struct {
	Type        string           `json:"type"`
	Attachments []CardAttachment `json:"attachments"`
}

// CardAttachment represents an attachment in the Teams payload
type CardAttachment struct {
	ContentType string      `json:"contentType"`
	Content     interface{} `json:"content"`
}

// CreateTeamsPayload wraps an Adaptive Card in the Teams webhook payload format
func CreateTeamsPayload(cardJSON string) (string, error) {
	var card map[string]interface{}
	if err := json.Unmarshal([]byte(cardJSON), &card); err != nil {
		return "", fmt.Errorf("failed to unmarshal card JSON: %w", err)
	}

	payload := WebhookPayload{
		Type: "message",
		Attachments: []CardAttachment{
			{
				ContentType: "application/vnd.microsoft.card.adaptive",
				Content:     card,
			},
		},
	}

	payloadJSON, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal Teams payload: %w", err)
	}

	return string(payloadJSON), nil
}

// GenerateRegenerationOptionsCard creates a card with parameter selection options
func GenerateRegenerationOptionsCard(query, responseID, previousResponse string) (string, error) {
	card := AdaptiveCard{
		Type:    "AdaptiveCard",
		Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
		Version: "1.5",
		Body:    []CardElement{},
		Actions: []CardAction{},
	}

	// Header
	card.Body = append(card.Body, CardElement{
		Type:   "TextBlock",
		Text:   "🔄 Regenerate Response",
		Size:   "Medium",
		Weight: "Bolder",
		Color:  "Accent",
	})

	// Description
	card.Body = append(card.Body, CardElement{
		Type:    "TextBlock",
		Text:    "Choose how you'd like the response to be regenerated:",
		Wrap:    true,
		Spacing: "Medium",
	})

	// Preset options
	presetOptions := []struct {
		preset      string
		title       string
		description string
		emoji       string
	}{
		{"creative", "More Creative", "Higher creativity with varied approaches", "🎨"},
		{"balanced", "Balanced", "Good balance of creativity and focus", "⚖️"},
		{"focused", "More Focused", "Precise and deterministic responses", "🎯"},
		{"detailed", "More Detailed", "Comprehensive and thorough responses", "📚"},
		{"concise", "More Concise", "Brief and to-the-point responses", "⚡"},
	}

	// Add preset action buttons
	for _, option := range presetOptions {
		card.Actions = append(card.Actions, CardAction{
			Type:   "Action.Http",
			Title:  fmt.Sprintf("%s %s", option.emoji, option.title),
			Method: "POST",
			URL:    "/teams-regenerate",
			Body: map[string]interface{}{
				"query":             query,
				"response_id":       responseID,
				"previous_response": previousResponse,
				"action":            "regenerate",
				"preset":            option.preset,
				"timestamp":         time.Now().Format(time.RFC3339),
			},
		})
	}

	// Cancel action
	card.Actions = append(card.Actions, CardAction{
		Type:   "Action.Http",
		Title:  "❌ Cancel",
		Method: "POST",
		URL:    "/teams-feedback",
		Body: map[string]interface{}{
			"query":       query,
			"response_id": responseID,
			"action":      "cancel_regeneration",
			"timestamp":   time.Now().Format(time.RFC3339),
		},
	})

	// Marshal to JSON
	cardJSON, err := json.MarshalIndent(card, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal regeneration options card: %w", err)
	}

	return string(cardJSON), nil
}

// GenerateComparisonCard creates a card showing both original and regenerated responses
func GenerateComparisonCard(query, originalResponse, regeneratedResponse string, preset string) (string, error) {
	responseID := generateResponseID()
	card := AdaptiveCard{
		Type:    "AdaptiveCard",
		Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
		Version: "1.5",
		Body:    []CardElement{},
		Actions: []CardAction{},
	}

	// Header
	card.Body = append(card.Body, CardElement{
		Type:   "TextBlock",
		Text:   "🔄 Regenerated Response",
		Size:   "Medium",
		Weight: "Bolder",
		Color:  "Accent",
	})

	// Query
	card.Body = append(card.Body, CardElement{
		Type:      "TextBlock",
		Text:      fmt.Sprintf("**Query:** %s", query),
		Wrap:      true,
		Spacing:   "Medium",
		Separator: true,
	})

	// Preset used
	card.Body = append(card.Body, CardElement{
		Type:    "TextBlock",
		Text:    fmt.Sprintf("**Generated with:** %s preset", strings.Title(preset)),
		Weight:  "Bolder",
		Spacing: "Small",
		Color:   "Good",
	})

	// New response
	card.Body = append(card.Body, CardElement{
		Type:    "TextBlock",
		Text:    regeneratedResponse,
		Wrap:    true,
		Spacing: "Medium",
	})

	// Original response (collapsible)
	if originalResponse != "" {
		card.Body = append(card.Body, CardElement{
			Type:      "TextBlock",
			Text:      "**Previous Response:**",
			Weight:    "Bolder",
			Spacing:   "Large",
			Separator: true,
		})

		card.Body = append(card.Body, CardElement{
			Type:    "TextBlock",
			Text:    originalResponse,
			Wrap:    true,
			Spacing: "Small",
			Color:   "Attention",
		})
	}

	// Actions
	card.Actions = append(card.Actions,
		CardAction{
			Type:   "Action.Http",
			Title:  "👍 Better",
			Method: "POST",
			URL:    "/teams-feedback",
			Body: map[string]interface{}{
				"query":       query,
				"response_id": responseID,
				"feedback":    "regeneration_better",
				"preset":      preset,
				"timestamp":   time.Now().Format(time.RFC3339),
			},
		},
		CardAction{
			Type:   "Action.Http",
			Title:  "👎 Worse",
			Method: "POST",
			URL:    "/teams-feedback",
			Body: map[string]interface{}{
				"query":       query,
				"response_id": responseID,
				"feedback":    "regeneration_worse",
				"preset":      preset,
				"timestamp":   time.Now().Format(time.RFC3339),
			},
		},
		CardAction{
			Type:   "Action.Http",
			Title:  "🔄 Try Another",
			Method: "POST",
			URL:    "/teams-regenerate",
			Body: map[string]interface{}{
				"query":             query,
				"response_id":       responseID,
				"previous_response": regeneratedResponse,
				"action":            "show_options",
				"timestamp":         time.Now().Format(time.RFC3339),
			},
		},
	)

	// Marshal to JSON
	cardJSON, err := json.MarshalIndent(card, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal comparison card: %w", err)
	}

	return string(cardJSON), nil
}

// generateResponseID generates a unique response ID for feedback correlation
func generateResponseID() string {
	return fmt.Sprintf("resp_%d", time.Now().UnixNano())
}

// GenerateClarificationCard creates a card for clarification requests
func GenerateClarificationCard(query string, questions []string, suggestions []string, quickOptions []string) (string, error) {
	card := AdaptiveCard{
		Type:    "AdaptiveCard",
		Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
		Version: "1.5",
		Body:    []CardElement{},
		Actions: []CardAction{},
	}

	// Header
	card.Body = append(card.Body, CardElement{
		Type:   "TextBlock",
		Text:   "❓ Clarification Needed",
		Size:   "Medium",
		Weight: "Bolder",
		Color:  "Warning",
	})

	// Original query
	card.Body = append(card.Body, CardElement{
		Type:      "TextBlock",
		Text:      fmt.Sprintf("**Your query:** %s", query),
		Wrap:      true,
		Spacing:   "Medium",
		Separator: true,
	})

	// Explanation
	card.Body = append(card.Body, CardElement{
		Type:    "TextBlock",
		Text:    "To provide you with the most accurate and helpful response, I need a bit more information:",
		Wrap:    true,
		Spacing: "Medium",
	})

	// Questions section
	if len(questions) > 0 {
		card.Body = append(card.Body, CardElement{
			Type:    "TextBlock",
			Text:    "**Questions to help me assist you better:**",
			Weight:  "Bolder",
			Spacing: "Medium",
		})

		for i, question := range questions {
			card.Body = append(card.Body, CardElement{
				Type:    "TextBlock",
				Text:    fmt.Sprintf("• %s", question),
				Wrap:    true,
				Spacing: "Small",
			})
			if i >= 4 { // Limit to 5 questions max
				break
			}
		}
	}

	// Suggestions section
	if len(suggestions) > 0 {
		card.Body = append(card.Body, CardElement{
			Type:    "TextBlock",
			Text:    "**Suggestions for a better query:**",
			Weight:  "Bolder",
			Spacing: "Medium",
		})

		for i, suggestion := range suggestions {
			card.Body = append(card.Body, CardElement{
				Type:    "TextBlock",
				Text:    fmt.Sprintf("• %s", suggestion),
				Wrap:    true,
				Spacing: "Small",
			})
			if i >= 2 { // Limit to 3 suggestions max
				break
			}
		}
	}

	// Quick options as action buttons
	if len(quickOptions) > 0 {
		card.Body = append(card.Body, CardElement{
			Type:    "TextBlock",
			Text:    "**Quick selection options:**",
			Weight:  "Bolder",
			Spacing: "Medium",
		})

		// Add quick option buttons
		for i, option := range quickOptions {
			card.Actions = append(card.Actions, CardAction{
				Type:   "Action.Http",
				Title:  option,
				Method: "POST",
				URL:    "/teams-clarify",
				Body: map[string]interface{}{
					"original_query": query,
					"clarification":  option,
					"action":         "quick_select",
					"timestamp":      time.Now().Format(time.RFC3339),
				},
			})
			if i >= 3 { // Limit to 4 quick options
				break
			}
		}
	}

	// General actions
	card.Actions = append(card.Actions,
		CardAction{
			Type:   "Action.Http",
			Title:  "📝 Provide More Details",
			Method: "POST",
			URL:    "/teams-clarify",
			Body: map[string]interface{}{
				"original_query": query,
				"action":         "provide_details",
				"timestamp":      time.Now().Format(time.RFC3339),
			},
		},
		CardAction{
			Type:   "Action.Http",
			Title:  "🎯 Use Template",
			Method: "POST",
			URL:    "/teams-clarify",
			Body: map[string]interface{}{
				"original_query": query,
				"action":         "use_template",
				"timestamp":      time.Now().Format(time.RFC3339),
			},
		},
	)

	// Marshal to JSON
	cardJSON, err := json.MarshalIndent(card, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal clarification card: %w", err)
	}

	return string(cardJSON), nil
}

// GenerateTemplateSelectionCard creates a card for guided template selection
func GenerateTemplateSelectionCard(query string) (string, error) {
	card := AdaptiveCard{
		Type:    "AdaptiveCard",
		Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
		Version: "1.5",
		Body:    []CardElement{},
		Actions: []CardAction{},
	}

	// Header
	card.Body = append(card.Body, CardElement{
		Type:   "TextBlock",
		Text:   "🎯 Guided Question Templates",
		Size:   "Medium",
		Weight: "Bolder",
		Color:  "Good",
	})

	// Description
	card.Body = append(card.Body, CardElement{
		Type:    "TextBlock",
		Text:    "Choose a template to help structure your question for better results:",
		Wrap:    true,
		Spacing: "Medium",
	})

	// Migration template
	card.Body = append(card.Body, CardElement{
		Type:      "TextBlock",
		Text:      "**Migration Planning Template**",
		Weight:    "Bolder",
		Spacing:   "Medium",
		Separator: true,
	})
	card.Body = append(card.Body, CardElement{
		Type:    "TextBlock",
		Text:    "For planning cloud migrations and workload transfers",
		Spacing: "Small",
		Color:   "Default",
	})

	card.Actions = append(card.Actions, CardAction{
		Type:   "Action.Http",
		Title:  "🚀 Migration Template",
		Method: "POST",
		URL:    "/teams-clarify",
		Body: map[string]interface{}{
			"original_query": query,
			"template":       "migration",
			"action":         "apply_template",
			"timestamp":      time.Now().Format(time.RFC3339),
		},
	})

	// Security template
	card.Body = append(card.Body, CardElement{
		Type:    "TextBlock",
		Text:    "**Security & Compliance Template**",
		Weight:  "Bolder",
		Spacing: "Medium",
	})
	card.Body = append(card.Body, CardElement{
		Type:    "TextBlock",
		Text:    "For security assessments and compliance planning",
		Spacing: "Small",
		Color:   "Default",
	})

	card.Actions = append(card.Actions, CardAction{
		Type:   "Action.Http",
		Title:  "🔒 Security Template",
		Method: "POST",
		URL:    "/teams-clarify",
		Body: map[string]interface{}{
			"original_query": query,
			"template":       "security",
			"action":         "apply_template",
			"timestamp":      time.Now().Format(time.RFC3339),
		},
	})

	// Architecture template
	card.Body = append(card.Body, CardElement{
		Type:    "TextBlock",
		Text:    "**Architecture Design Template**",
		Weight:  "Bolder",
		Spacing: "Medium",
	})
	card.Body = append(card.Body, CardElement{
		Type:    "TextBlock",
		Text:    "For solution architecture and technical design",
		Spacing: "Small",
		Color:   "Default",
	})

	card.Actions = append(card.Actions, CardAction{
		Type:   "Action.Http",
		Title:  "🏗️ Architecture Template",
		Method: "POST",
		URL:    "/teams-clarify",
		Body: map[string]interface{}{
			"original_query": query,
			"template":       "architecture",
			"action":         "apply_template",
			"timestamp":      time.Now().Format(time.RFC3339),
		},
	})

	// Cost optimization template
	card.Actions = append(card.Actions, CardAction{
		Type:   "Action.Http",
		Title:  "💰 Cost Optimization Template",
		Method: "POST",
		URL:    "/teams-clarify",
		Body: map[string]interface{}{
			"original_query": query,
			"template":       "cost",
			"action":         "apply_template",
			"timestamp":      time.Now().Format(time.RFC3339),
		},
	})

	// Back action
	card.Actions = append(card.Actions, CardAction{
		Type:   "Action.Http",
		Title:  "← Back to Clarification",
		Method: "POST",
		URL:    "/teams-clarify",
		Body: map[string]interface{}{
			"original_query": query,
			"action":         "back_to_clarification",
			"timestamp":      time.Now().Format(time.RFC3339),
		},
	})

	// Marshal to JSON
	cardJSON, err := json.MarshalIndent(card, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal template selection card: %w", err)
	}

	return string(cardJSON), nil
}

// GenerateGuidedTemplateCard creates a guided template form
func GenerateGuidedTemplateCard(query, templateType string) (string, error) {
	card := AdaptiveCard{
		Type:    "AdaptiveCard",
		Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
		Version: "1.5",
		Body:    []CardElement{},
		Actions: []CardAction{},
	}

	// Header based on template type
	var headerText, templateDescription, exampleText string
	switch templateType {
	case "migration":
		headerText = "🚀 Migration Planning Template"
		templateDescription = "Fill in the details below to get a comprehensive migration plan:"
		exampleText = "Example: Help me migrate 50 Windows VMs from VMware to AWS with 6-month timeline for production workloads"
	case "security":
		headerText = "🔒 Security & Compliance Template"
		templateDescription = "Provide security requirements for a tailored assessment:"
		exampleText = "Example: Create a HIPAA-compliant security plan for healthcare data in Azure production environment"
	case "architecture":
		headerText = "🏗️ Architecture Design Template"
		templateDescription = "Specify your architectural requirements:"
		exampleText = "Example: Design a scalable web application architecture for 100,000 users on AWS with high availability"
	case "cost":
		headerText = "💰 Cost Optimization Template"
		templateDescription = "Help us understand your cost optimization goals:"
		exampleText = "Example: Optimize AWS costs for development environments with $10,000 monthly budget"
	default:
		headerText = "🎯 Guided Template"
		templateDescription = "Fill in the template below:"
		exampleText = "Provide specific details for better assistance"
	}

	card.Body = append(card.Body, CardElement{
		Type:   "TextBlock",
		Text:   headerText,
		Size:   "Medium",
		Weight: "Bolder",
		Color:  "Good",
	})

	card.Body = append(card.Body, CardElement{
		Type:    "TextBlock",
		Text:    templateDescription,
		Wrap:    true,
		Spacing: "Medium",
	})

	// Example
	card.Body = append(card.Body, CardElement{
		Type:    "TextBlock",
		Text:    fmt.Sprintf("**Example:** %s", exampleText),
		Wrap:    true,
		Spacing: "Medium",
		Color:   "Attention",
	})

	// Template-specific guidance
	switch templateType {
	case "migration":
		card.Body = append(card.Body, CardElement{
			Type:    "TextBlock",
			Text:    "**Key Information Needed:**\n• Source environment (VMware, Hyper-V, physical)\n• Target cloud provider (AWS, Azure, GCP)\n• Workload types and count\n• Timeline and constraints\n• Compliance requirements",
			Wrap:    true,
			Spacing: "Medium",
		})
	case "security":
		card.Body = append(card.Body, CardElement{
			Type:    "TextBlock",
			Text:    "**Key Information Needed:**\n• Compliance standards (HIPAA, GDPR, SOX, PCI)\n• Data types and sensitivity\n• Environment (production, development)\n• Current security concerns\n• Budget and timeline",
			Wrap:    true,
			Spacing: "Medium",
		})
	case "architecture":
		card.Body = append(card.Body, CardElement{
			Type:    "TextBlock",
			Text:    "**Key Information Needed:**\n• Application type and requirements\n• Expected load and scale\n• Performance requirements\n• High availability needs\n• Integration requirements",
			Wrap:    true,
			Spacing: "Medium",
		})
	case "cost":
		card.Body = append(card.Body, CardElement{
			Type:    "TextBlock",
			Text:    "**Key Information Needed:**\n• Current cloud spending\n• Workload types to optimize\n• Target savings goals\n• Usage patterns\n• Acceptable trade-offs",
			Wrap:    true,
			Spacing: "Medium",
		})
	}

	// Instructions
	card.Body = append(card.Body, CardElement{
		Type:      "TextBlock",
		Text:      "**Instructions:** Please rewrite your question using the guidance above, then send it as a new message to get a comprehensive response.",
		Wrap:      true,
		Spacing:   "Medium",
		Separator: true,
		Color:     "Good",
	})

	// Back action
	card.Actions = append(card.Actions, CardAction{
		Type:   "Action.Http",
		Title:  "← Back to Templates",
		Method: "POST",
		URL:    "/teams-clarify",
		Body: map[string]interface{}{
			"original_query": query,
			"action":         "show_templates",
			"timestamp":      time.Now().Format(time.RFC3339),
		},
	})

	// Marshal to JSON
	cardJSON, err := json.MarshalIndent(card, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal guided template card: %w", err)
	}

	return string(cardJSON), nil
}
