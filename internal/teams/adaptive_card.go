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
