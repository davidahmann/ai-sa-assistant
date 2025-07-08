package teams

import (
	"encoding/json"
	"fmt"
	"strings"

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

	// Feedback actions
	card.Actions = append(card.Actions,
		CardAction{
			Type:   "Action.Http",
			Title:  "👍 Helpful",
			Method: "POST",
			URL:    "/teams-feedback",
			Body: map[string]interface{}{
				"query":    query,
				"feedback": "positive",
			},
		},
		CardAction{
			Type:   "Action.Http",
			Title:  "👎 Not Helpful",
			Method: "POST",
			URL:    "/teams-feedback",
			Body: map[string]interface{}{
				"query":    query,
				"feedback": "negative",
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

// TeamsWebhookPayload represents the payload sent to Teams
type TeamsWebhookPayload struct {
	Type        string                `json:"type"`
	Attachments []TeamsCardAttachment `json:"attachments"`
}

// TeamsCardAttachment represents an attachment in the Teams payload
type TeamsCardAttachment struct {
	ContentType string      `json:"contentType"`
	Content     interface{} `json:"content"`
}

// CreateTeamsPayload wraps an Adaptive Card in the Teams webhook payload format
func CreateTeamsPayload(cardJSON string) (string, error) {
	var card map[string]interface{}
	if err := json.Unmarshal([]byte(cardJSON), &card); err != nil {
		return "", fmt.Errorf("failed to unmarshal card JSON: %w", err)
	}

	payload := TeamsWebhookPayload{
		Type: "message",
		Attachments: []TeamsCardAttachment{
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
