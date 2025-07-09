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

// Package main provides the Teams bot webhook handler for the AI SA Assistant.
// It receives Teams messages and orchestrates calls to backend services.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/diagram"
	"github.com/your-org/ai-sa-assistant/internal/synth"
	"github.com/your-org/ai-sa-assistant/internal/teams"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()

	cfg, err := config.Load("./configs/config.yaml")
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	// Initialize diagram renderer
	diagramConfig := diagram.RendererConfig{
		MermaidInkURL:  cfg.Diagram.MermaidInkURL,
		Timeout:        time.Duration(cfg.Diagram.Timeout) * time.Second,
		CacheExpiry:    time.Duration(cfg.Diagram.CacheExpiry) * time.Hour,
		EnableCaching:  cfg.Diagram.EnableCaching,
		MaxDiagramSize: cfg.Diagram.MaxDiagramSize,
	}
	diagramRenderer := diagram.NewRenderer(diagramConfig, logger)

	// Test diagram renderer connection
	const testTimeout = 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	if err := diagramRenderer.TestConnection(ctx); err != nil {
		logger.Warn("Failed to test diagram renderer connection", zap.Error(err))
	}

	router := gin.Default()

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "service": "teamsbot"})
	})

	// Teams webhook endpoint
	router.POST("/teams-webhook", func(c *gin.Context) {
		handleTeamsWebhook(c, cfg, diagramRenderer, logger)
	})

	// Feedback endpoint
	router.POST("/teams-feedback", func(c *gin.Context) {
		handleFeedback(c, cfg, logger)
	})

	logger.Info("Starting teamsbot service",
		zap.String("retrieve_url", cfg.Services.RetrieveURL),
		zap.String("synthesize_url", cfg.Services.SynthesizeURL),
		zap.String("websearch_url", cfg.Services.WebSearchURL),
		zap.String("diagram_renderer_url", cfg.Diagram.MermaidInkURL))

	if err := router.Run(":8080"); err != nil {
		logger.Fatal("Failed to start server", zap.Error(err))
	}
}

// TeamsMessage represents a Teams webhook message
type TeamsMessage struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// handleTeamsWebhook handles incoming Teams webhook messages
func handleTeamsWebhook(c *gin.Context, cfg *config.Config, diagramRenderer *diagram.Renderer, logger *zap.Logger) {
	var message TeamsMessage
	if err := c.ShouldBindJSON(&message); err != nil {
		logger.Error("Failed to parse Teams message", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid message format"})
		return
	}

	// Extract user query from message text
	query := strings.TrimSpace(message.Text)
	if query == "" {
		logger.Warn("Empty query received from Teams")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Empty query"})
		return
	}

	// Remove bot mention if present
	query = strings.TrimPrefix(query, "@SA-Assistant")
	query = strings.TrimSpace(query)

	logger.Info("Processing Teams query", zap.String("query", query))

	// Process the query through the full pipeline
	const queryTimeout = 60 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	response, err := processQuery(ctx, query, cfg, diagramRenderer, logger)
	if err != nil {
		logger.Error("Failed to process query", zap.Error(err))
		sendErrorResponse(c, cfg, query, err, logger)
		return
	}

	// Send response back to Teams
	if err := sendTeamsResponse(c, cfg, query, response, logger); err != nil {
		logger.Error("Failed to send Teams response", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send response"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Query processed successfully"})
}

// processQuery processes a user query through the full RAG pipeline
func processQuery(
	ctx context.Context,
	query string,
	cfg *config.Config,
	diagramRenderer *diagram.Renderer,
	logger *zap.Logger,
) (*synth.SynthesisResponse, error) {
	// Step 1: Call retrieve service
	retrieveResponse, err := callRetrieveService(ctx, query, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("retrieve service failed: %w", err)
	}

	// Step 2: Conditionally call websearch service
	var webResults []string
	if needsFreshness(query, cfg.WebSearch.FreshnessKeywords) {
		webResults, err = callWebSearchService(ctx, query, cfg, logger)
		if err != nil {
			logger.Warn("Web search failed, continuing without web results", zap.Error(err))
		}
	}

	// Step 3: Convert retrieve chunks to synthesis format and call synthesize service
	contextItems := convertRetrieveChunksToContextItems(retrieveResponse.Chunks)
	synthesizeRequest := createSynthesizeRequest(query, contextItems, webResults)

	synthesizeResponse, err := callSynthesizeService(ctx, synthesizeRequest, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("synthesize service failed: %w", err)
	}

	// Step 4: Render diagram if present
	if synthesizeResponse.DiagramCode != "" {
		diagramURL, fallbackText, err := diagramRenderer.RenderDiagramWithFallback(ctx, synthesizeResponse.DiagramCode)
		if err != nil {
			logger.Warn("Failed to render diagram", zap.Error(err))
		}

		// If we have a fallback text, append it to the main text
		if fallbackText != "" {
			synthesizeResponse.MainText += "\n\n" + fallbackText
			synthesizeResponse.DiagramCode = "" // Clear diagram code since we're using fallback
		}

		// Store diagram URL for use in Adaptive Card
		synthesizeResponse.DiagramURL = diagramURL
	}

	return synthesizeResponse, nil
}

// RetrieveResponse represents the response from the retrieve service
type RetrieveResponse struct {
	Chunks            []RetrieveChunk `json:"chunks"`
	Count             int             `json:"count"`
	Query             string          `json:"query"`
	FallbackTriggered bool            `json:"fallback_triggered"`
	FallbackReason    string          `json:"fallback_reason,omitempty"`
}

// RetrieveChunk represents a chunk from the retrieve service
type RetrieveChunk struct {
	Text     string                 `json:"text"`
	Score    float64                `json:"score"`
	DocID    string                 `json:"doc_id"`
	SourceID string                 `json:"source_id"`
	Metadata map[string]interface{} `json:"metadata"`
}

// callRetrieveService calls the retrieve service to get context
func callRetrieveService(
	ctx context.Context,
	query string,
	cfg *config.Config,
	_ *zap.Logger,
) (*RetrieveResponse, error) {
	reqBody := map[string]string{"query": query}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cfg.Services.RetrieveURL+"/search", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("retrieve service returned status %d: %s", resp.StatusCode, string(body))
	}

	var retrieveResponse RetrieveResponse
	if err := json.NewDecoder(resp.Body).Decode(&retrieveResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &retrieveResponse, nil
}

// callWebSearchService calls the web search service
func callWebSearchService(ctx context.Context, query string, cfg *config.Config, _ *zap.Logger) ([]string, error) {
	reqBody := map[string]string{"query": query}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cfg.Services.WebSearchURL+"/search", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("web search service returned status %d: %s", resp.StatusCode, string(body))
	}

	var webResponse struct {
		Results []string `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&webResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return webResponse.Results, nil
}

// callSynthesizeService calls the synthesize service
func callSynthesizeService(
	ctx context.Context,
	request SynthesizeRequest,
	cfg *config.Config,
	_ *zap.Logger,
) (*synth.SynthesisResponse, error) {
	jsonBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		cfg.Services.SynthesizeURL+"/synthesize",
		bytes.NewBuffer(jsonBody),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("synthesize service returned status %d: %s", resp.StatusCode, string(body))
	}

	var synthesizeResponse synth.SynthesisResponse
	if err := json.NewDecoder(resp.Body).Decode(&synthesizeResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &synthesizeResponse, nil
}

// convertRetrieveChunksToContextItems converts retrieve chunks to synthesis context items
func convertRetrieveChunksToContextItems(chunks []RetrieveChunk) []synth.ContextItem {
	contextItems := make([]synth.ContextItem, len(chunks))
	for i, chunk := range chunks {
		// Use SourceID if available, otherwise fall back to DocID
		sourceID := chunk.SourceID
		if sourceID == "" {
			sourceID = chunk.DocID
		}

		contextItems[i] = synth.ContextItem{
			Content:  chunk.Text,
			SourceID: sourceID,
			Score:    chunk.Score,
			Priority: 1,
		}
	}
	return contextItems
}

// SynthesizeChunkItem represents a chunk item for synthesis request
type SynthesizeChunkItem struct {
	Text     string `json:"text"`
	DocID    string `json:"doc_id"`
	SourceID string `json:"source_id"`
}

// SynthesizeWebResult represents a web result for synthesis request
type SynthesizeWebResult struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	URL     string `json:"url"`
}

// SynthesizeRequest represents a request to the synthesis service
type SynthesizeRequest struct {
	Query      string                `json:"query"`
	Chunks     []SynthesizeChunkItem `json:"chunks"`
	WebResults []SynthesizeWebResult `json:"web_results"`
}

// createSynthesizeRequest creates a synthesis request from context and web results
func createSynthesizeRequest(query string, contextItems []synth.ContextItem, webResults []string) SynthesizeRequest {
	// Convert context items to chunk items
	chunks := make([]SynthesizeChunkItem, len(contextItems))
	for i, item := range contextItems {
		chunks[i] = SynthesizeChunkItem{
			Text:     item.Content,
			DocID:    item.SourceID, // Use SourceID as DocID for backwards compatibility
			SourceID: item.SourceID,
		}
	}

	// Convert web results to structured format
	webResultItems := make([]SynthesizeWebResult, len(webResults))
	for i, result := range webResults {
		// Parse web result format: "Title: <title>\nSnippet: <snippet>\nURL: <url>"
		lines := strings.Split(result, "\n")
		var title, snippet, url string
		for _, line := range lines {
			switch {
			case strings.HasPrefix(line, "Title: "):
				title = strings.TrimPrefix(line, "Title: ")
			case strings.HasPrefix(line, "Snippet: "):
				snippet = strings.TrimPrefix(line, "Snippet: ")
			case strings.HasPrefix(line, "URL: "):
				url = strings.TrimPrefix(line, "URL: ")
			}
		}
		webResultItems[i] = SynthesizeWebResult{
			Title:   title,
			Snippet: snippet,
			URL:     url,
		}
	}

	return SynthesizeRequest{
		Query:      query,
		Chunks:     chunks,
		WebResults: webResultItems,
	}
}

// needsFreshness checks if the query needs fresh web search results
func needsFreshness(query string, keywords []string) bool {
	queryLower := strings.ToLower(query)
	for _, keyword := range keywords {
		if strings.Contains(queryLower, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

// sendTeamsResponse sends the response back to Teams
func sendTeamsResponse(
	_ *gin.Context,
	cfg *config.Config,
	query string,
	response *synth.SynthesisResponse,
	logger *zap.Logger,
) error {
	// Generate Adaptive Card
	cardJSON, err := teams.GenerateCard(*response, query, response.DiagramURL)
	if err != nil {
		return fmt.Errorf("failed to generate adaptive card: %w", err)
	}

	// Create Teams webhook payload
	payload, err := teams.CreateTeamsPayload(cardJSON)
	if err != nil {
		return fmt.Errorf("failed to create teams payload: %w", err)
	}

	// Send to Teams webhook
	req, err := http.NewRequest("POST", cfg.Teams.WebhookURL, strings.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("teams webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	logger.Info("Successfully sent Teams response", zap.String("query", query))
	return nil
}

// sendErrorResponse sends an error response to Teams
func sendErrorResponse(
	_ *gin.Context,
	cfg *config.Config,
	_ string,
	err error,
	logger *zap.Logger,
) {
	errorMessage := fmt.Sprintf(
		"I encountered an error processing your request: %s",
		err.Error(),
	)

	cardJSON, cardErr := teams.GenerateSimpleCard("Error", errorMessage)
	if cardErr != nil {
		logger.Error("Failed to generate error card", zap.Error(cardErr))
		return
	}

	payload, cardErr := teams.CreateTeamsPayload(cardJSON)
	if cardErr != nil {
		logger.Error("Failed to create error payload", zap.Error(cardErr))
		return
	}

	req, cardErr := http.NewRequest("POST", cfg.Teams.WebhookURL, strings.NewReader(payload))
	if cardErr != nil {
		logger.Error("Failed to create error webhook request", zap.Error(cardErr))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, cardErr := client.Do(req)
	if cardErr != nil {
		logger.Error("Failed to send error webhook", zap.Error(cardErr))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Error("Error webhook failed", zap.Int("status", resp.StatusCode), zap.String("body", string(body)))
	}
}

// FeedbackRequest represents a feedback request
type FeedbackRequest struct {
	Query    string `json:"query"`
	Feedback string `json:"feedback"`
}

// handleFeedback handles feedback submissions
func handleFeedback(c *gin.Context, _ *config.Config, logger *zap.Logger) {
	var feedback FeedbackRequest
	if err := c.ShouldBindJSON(&feedback); err != nil {
		logger.Error("Failed to parse feedback", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid feedback format"})
		return
	}

	// Log the feedback
	logger.Info("Received feedback",
		zap.String("query", feedback.Query),
		zap.String("feedback", feedback.Feedback))

	// TODO: Store feedback in database or file based on configuration
	// For now, just log it

	c.JSON(http.StatusOK, gin.H{"message": "Feedback received"})
}
