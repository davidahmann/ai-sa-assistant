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

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// MockTeamsServer represents a mock Teams webhook server for E2E testing
type MockTeamsServer struct {
	server       *httptest.Server
	mutex        sync.RWMutex
	responses    []AdaptiveCardResponse
	callCount    int
	lastResponse *AdaptiveCardResponse
}

// AdaptiveCardResponse represents a parsed Teams Adaptive Card response
type AdaptiveCardResponse struct {
	Type        string            `json:"type"`
	Attachments []CardAttachment  `json:"attachments"`
	Timestamp   time.Time         `json:"timestamp"`
	Headers     map[string]string `json:"headers"`
	StatusCode  int               `json:"status_code"`

	// Parsed content from the adaptive card
	ParsedContent *ParsedCardContent `json:"parsed_content,omitempty"`
}

// CardAttachment represents an attachment in the Teams payload
type CardAttachment struct {
	ContentType string      `json:"contentType"`
	Content     interface{} `json:"content"`
}

// ParsedCardContent represents the parsed content from an adaptive card
type ParsedCardContent struct {
	MainText     string            `json:"main_text"`
	Query        string            `json:"query"`
	DiagramURL   string            `json:"diagram_url"`
	CodeSnippets []CodeSnippet     `json:"code_snippets"`
	Sources      []string          `json:"sources"`
	Actions      []CardAction      `json:"actions"`
	HasDiagram   bool              `json:"has_diagram"`
	ResponseID   string            `json:"response_id"`
	Metadata     map[string]string `json:"metadata"`
}

// CodeSnippet represents a code snippet in the response
type CodeSnippet struct {
	Language string `json:"language"`
	Code     string `json:"code"`
}

// CardAction represents an action in the adaptive card
type CardAction struct {
	Type   string                 `json:"type"`
	Title  string                 `json:"title"`
	URL    string                 `json:"url,omitempty"`
	Method string                 `json:"method,omitempty"`
	Body   map[string]interface{} `json:"body,omitempty"`
}

// NewMockTeamsServer creates a new mock Teams webhook server
func NewMockTeamsServer() *MockTeamsServer {
	mock := &MockTeamsServer{
		responses: []AdaptiveCardResponse{},
		callCount: 0,
	}

	// Create HTTP handler
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", mock.handleWebhook)
	mux.HandleFunc("/teams-feedback", mock.handleFeedback)

	// Create test server
	mock.server = httptest.NewServer(mux)

	return mock
}

// GetURL returns the mock server URL
func (m *MockTeamsServer) GetURL() string {
	return m.server.URL
}

// GetWebhookURL returns the webhook endpoint URL
func (m *MockTeamsServer) GetWebhookURL() string {
	return m.server.URL + "/webhook"
}

// Close shuts down the mock server
func (m *MockTeamsServer) Close() {
	m.server.Close()
}

// GetResponses returns all received responses
func (m *MockTeamsServer) GetResponses() []AdaptiveCardResponse {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return append([]AdaptiveCardResponse(nil), m.responses...)
}

// GetLastResponse returns the most recent response
func (m *MockTeamsServer) GetLastResponse() *AdaptiveCardResponse {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.lastResponse
}

// GetCallCount returns the number of webhook calls received
func (m *MockTeamsServer) GetCallCount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.callCount
}

// WaitForResponse waits for a response to be received within the timeout
func (m *MockTeamsServer) WaitForResponse(timeout time.Duration) (*AdaptiveCardResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond) //nolint:mnd // Standard ticker interval
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for Teams response after %v", timeout)
		case <-ticker.C:
			if response := m.GetLastResponse(); response != nil {
				return response, nil
			}
		}
	}
}

// Reset clears all stored responses and resets call count
func (m *MockTeamsServer) Reset() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.responses = []AdaptiveCardResponse{}
	m.callCount = 0
	m.lastResponse = nil
}

// handleWebhook handles incoming webhook requests
func (m *MockTeamsServer) handleWebhook(w http.ResponseWriter, r *http.Request) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Parse the webhook payload
	var payload AdaptiveCardResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "Failed to parse JSON payload", http.StatusBadRequest)
		return
	}

	// Set response metadata
	payload.Timestamp = time.Now()
	payload.StatusCode = http.StatusOK
	payload.Headers = make(map[string]string)
	for key, values := range r.Header {
		if len(values) > 0 {
			payload.Headers[key] = values[0]
		}
	}

	// Parse the adaptive card content
	if len(payload.Attachments) > 0 {
		parsedContent, err := m.parseAdaptiveCardContent(payload.Attachments[0].Content)
		if err == nil {
			payload.ParsedContent = parsedContent
		}
	}

	// Store the response
	m.responses = append(m.responses, payload)
	m.lastResponse = &payload
	m.callCount++

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := map[string]interface{}{
		"message":    "Webhook received successfully",
		"call_count": m.callCount,
		"timestamp":  time.Now().Format(time.RFC3339),
	}
	_ = json.NewEncoder(w).Encode(response)
}

// handleFeedback handles feedback requests
func (m *MockTeamsServer) handleFeedback(w http.ResponseWriter, _ *http.Request) {
	// Simple feedback handler for testing
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "Feedback received",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// parseAdaptiveCardContent parses the content of an adaptive card
func (m *MockTeamsServer) parseAdaptiveCardContent(content interface{}) (*ParsedCardContent, error) {
	contentMap, ok := content.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid content format")
	}

	parsed := &ParsedCardContent{
		CodeSnippets: []CodeSnippet{},
		Sources:      []string{},
		Actions:      []CardAction{},
		Metadata:     make(map[string]string),
	}

	// Parse body elements
	if body, ok := contentMap["body"].([]interface{}); ok {
		parsed.MainText = m.extractMainText(body)
		parsed.Query = m.extractQuery(body)
		parsed.DiagramURL = m.extractDiagramURL(body)
		parsed.CodeSnippets = m.extractCodeSnippets(body)
		parsed.Sources = m.extractSources(body)
		parsed.HasDiagram = parsed.DiagramURL != ""
	}

	// Parse actions
	if actions, ok := contentMap["actions"].([]interface{}); ok {
		parsed.Actions = m.extractActions(actions)
		parsed.ResponseID = m.extractResponseID(actions)
	}

	return parsed, nil
}

// extractMainText extracts the main text content from card body
func (m *MockTeamsServer) extractMainText(body []interface{}) string {
	var mainText string
	for _, element := range body {
		if elem, ok := element.(map[string]interface{}); ok {
			if elemType, ok := elem["type"].(string); ok && elemType == "TextBlock" {
				if text, ok := elem["text"].(string); ok {
					// Skip header and query, capture main content
					if !m.isHeaderOrQuery(text) {
						if mainText == "" {
							mainText = text
						} else {
							mainText += "\n" + text
						}
					}
				}
			}
		}
	}
	return mainText
}

// extractQuery extracts the query from card body
func (m *MockTeamsServer) extractQuery(body []interface{}) string {
	for _, element := range body {
		if elem, ok := element.(map[string]interface{}); ok {
			if text, ok := elem["text"].(string); ok {
				if len(text) > 8 && text[:8] == "**Query:" {
					return text[9:] // Remove "**Query: " prefix
				}
			}
		}
	}
	return ""
}

// extractDiagramURL extracts the diagram URL from card body
func (m *MockTeamsServer) extractDiagramURL(body []interface{}) string {
	for _, element := range body {
		if elem, ok := element.(map[string]interface{}); ok {
			if elemType, ok := elem["type"].(string); ok && elemType == "Image" {
				if url, ok := elem["url"].(string); ok {
					return url
				}
			}
		}
	}
	return ""
}

// extractCodeSnippets extracts code snippets from card body
func (m *MockTeamsServer) extractCodeSnippets(body []interface{}) []CodeSnippet {
	var snippets []CodeSnippet
	var currentLanguage string

	for _, element := range body {
		if elem, ok := element.(map[string]interface{}); ok {
			if text, ok := elem["text"].(string); ok {
				// Check if this is a language header
				if m.isCodeLanguageHeader(text) {
					currentLanguage = m.extractLanguageFromHeader(text)
				} else if m.isCodeBlock(text) && currentLanguage != "" {
					code := m.extractCodeFromBlock(text)
					snippets = append(snippets, CodeSnippet{
						Language: currentLanguage,
						Code:     code,
					})
					currentLanguage = ""
				}
			}
		}
	}

	return snippets
}

// extractSources extracts source citations from card body
func (m *MockTeamsServer) extractSources(body []interface{}) []string {
	var sources []string
	inSourcesSection := false

	for _, element := range body {
		if elem, ok := element.(map[string]interface{}); ok {
			if text, ok := elem["text"].(string); ok {
				if text == "**Sources:**" {
					inSourcesSection = true
				} else if inSourcesSection && text != "" {
					// Parse bullet-pointed sources
					if len(text) > 2 && text[:2] == "• " {
						sourceLines := m.parseBulletPoints(text)
						sources = append(sources, sourceLines...)
					}
				}
			}
		}
	}

	return sources
}

// extractActions extracts actions from card actions
func (m *MockTeamsServer) extractActions(actions []interface{}) []CardAction {
	var cardActions []CardAction

	for _, action := range actions {
		if actionMap, ok := action.(map[string]interface{}); ok {
			cardAction := CardAction{}
			if actionType, ok := actionMap["type"].(string); ok {
				cardAction.Type = actionType
			}
			if title, ok := actionMap["title"].(string); ok {
				cardAction.Title = title
			}
			if url, ok := actionMap["url"].(string); ok {
				cardAction.URL = url
			}
			if method, ok := actionMap["method"].(string); ok {
				cardAction.Method = method
			}
			if body, ok := actionMap["body"].(map[string]interface{}); ok {
				cardAction.Body = body
			}
			cardActions = append(cardActions, cardAction)
		}
	}

	return cardActions
}

// extractResponseID extracts response ID from actions
func (m *MockTeamsServer) extractResponseID(actions []interface{}) string {
	for _, action := range actions {
		if actionMap, ok := action.(map[string]interface{}); ok {
			if body, ok := actionMap["body"].(map[string]interface{}); ok {
				if responseID, ok := body["response_id"].(string); ok {
					return responseID
				}
			}
		}
	}
	return ""
}

// Helper methods for parsing

func (m *MockTeamsServer) isHeaderOrQuery(text string) bool {
	return text == "🤖 Cloud SA Assistant" ||
		(len(text) > 8 && text[:8] == "**Query:")
}

func (m *MockTeamsServer) isCodeLanguageHeader(text string) bool {
	return len(text) > 3 && text[0] == '*' && text[len(text)-1] == '*' && text[len(text)-2] == ':'
}

func (m *MockTeamsServer) extractLanguageFromHeader(text string) string {
	// Extract language from "*Language:*" format
	if len(text) > 3 { //nolint:mnd // Minimum text length for parsing
		return text[1 : len(text)-2] // Remove "*" and ":*"
	}
	return ""
}

func (m *MockTeamsServer) isCodeBlock(text string) bool {
	return len(text) > 6 && text[:3] == "```" && text[len(text)-3:] == "```"
}

func (m *MockTeamsServer) extractCodeFromBlock(text string) string {
	// Extract code from "```\ncode\n```" format
	if len(text) > 8 { //nolint:mnd // Query prefix length
		return text[4 : len(text)-4] // Remove "```\n" and "\n```"
	}
	return ""
}

func (m *MockTeamsServer) parseBulletPoints(text string) []string {
	// Split by bullet points and clean up
	lines := []string{}
	for _, line := range []string{text} {
		if len(line) > 2 && line[:2] == "• " {
			lines = append(lines, line[2:])
		}
	}
	return lines
}

// TeamsBotClient is a client for sending messages to the Teams bot
type TeamsBotClient struct {
	BaseURL string
	Client  *http.Client
}

// NewTeamsBotClient creates a new Teams bot client
func NewTeamsBotClient(baseURL string) *TeamsBotClient {
	return &TeamsBotClient{
		BaseURL: baseURL,
		Client: &http.Client{
			Timeout: 60 * time.Second, //nolint:mnd // Standard timeout value
		},
	}
}

// SendMessage sends a message to the Teams bot and returns the response time
func (c *TeamsBotClient) SendMessage(t *testing.T, message string, webhookURL string) (time.Duration, error) {
	t.Helper()

	// Create the request payload
	payload := map[string]interface{}{
		"text":        message,
		"webhook_url": webhookURL,
		"user_id":     "e2e-test-user",
		"channel_id":  "e2e-test-channel",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send the request
	start := time.Now()
	resp, err := c.Client.Post(c.BaseURL+"/teams-message", "application/json", bytes.NewBuffer(body))
	duration := time.Since(start)

	if err != nil {
		return duration, fmt.Errorf("failed to send message: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return duration, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	return duration, nil
}
