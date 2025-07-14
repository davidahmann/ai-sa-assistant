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
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/your-org/ai-sa-assistant/internal/clarification"
	"github.com/your-org/ai-sa-assistant/internal/classifier"
	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/conversation"
	"github.com/your-org/ai-sa-assistant/internal/diagram"
	"github.com/your-org/ai-sa-assistant/internal/feedback"
	"github.com/your-org/ai-sa-assistant/internal/health"
	"github.com/your-org/ai-sa-assistant/internal/session"
	"github.com/your-org/ai-sa-assistant/internal/teams"
	"go.uber.org/zap"
)

const (
	// HealthCheckTimeout is the timeout for health check requests
	HealthCheckTimeout = 10 * time.Second
)

func main() {
	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()

	// Check if running in test mode
	testMode := os.Getenv("TEST_MODE") == "true" || os.Getenv("CI") == "true"

	var cfg *config.Config
	var err error

	if testMode {
		cfg, err = config.LoadWithOptions(config.LoadOptions{
			ConfigPath:       "./configs/config.yaml",
			EnableHotReload:  false,
			Environment:      "test",
			ValidateRequired: false,
			TestMode:         true,
		})
	} else {
		cfg, err = config.Load("./configs/config.yaml")
	}

	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	// Initialize feedback logger
	feedbackConfig := feedback.Config{
		StorageType: "file", // Default to file storage
		FilePath:    "./logs/feedback.jsonl",
		DBPath:      "./data/feedback.db",
	}

	// Override with config values if available
	if cfg.Feedback.StorageType != "" {
		feedbackConfig.StorageType = cfg.Feedback.StorageType
	}
	if cfg.Feedback.FilePath != "" {
		feedbackConfig.FilePath = cfg.Feedback.FilePath
	}
	if cfg.Feedback.DBPath != "" {
		feedbackConfig.DBPath = cfg.Feedback.DBPath
	}

	feedbackLogger, err := feedback.NewLogger(feedbackConfig, logger)
	if err != nil {
		logger.Fatal("Failed to initialize feedback logger", zap.Error(err))
	}
	defer func() { _ = feedbackLogger.Close() }()

	// Initialize diagram renderer
	diagramConfig := diagram.RendererConfig{
		MermaidInkURL:  cfg.Diagram.MermaidInkURL,
		Timeout:        time.Duration(cfg.Diagram.Timeout) * time.Second,
		CacheExpiry:    time.Duration(cfg.Diagram.CacheExpiry) * time.Hour,
		EnableCaching:  cfg.Diagram.EnableCaching,
		MaxDiagramSize: cfg.Diagram.MaxDiagramSize,
	}
	diagramRenderer := diagram.NewRenderer(diagramConfig, logger)

	// Initialize session manager
	sessionConfig := session.Config{
		StorageType:     session.StorageType(cfg.Session.StorageType),
		RedisURL:        cfg.Session.RedisURL,
		DefaultTTL:      time.Duration(cfg.Session.DefaultTTL) * time.Minute,
		MaxSessions:     cfg.Session.MaxSessions,
		CleanupInterval: time.Duration(cfg.Session.CleanupInterval) * time.Minute,
	}
	sessionManager, err := session.NewManager(sessionConfig, logger)
	if err != nil {
		logger.Fatal("Failed to initialize session manager", zap.Error(err))
	}
	defer func() { _ = sessionManager.Close() }()

	// Initialize conversation manager
	conversationManager := conversation.NewManager(sessionManager, logger)

	// Initialize query classifier
	queryClassifier := classifier.NewQueryClassifier()

	// Test diagram renderer connection
	const testTimeout = 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	if err := diagramRenderer.TestConnection(ctx); err != nil {
		logger.Warn("Failed to test diagram renderer connection", zap.Error(err))
	}

	router := gin.Default()

	// Initialize health check manager
	healthManager := health.NewManager("teamsbot", "1.0.0", logger)
	setupHealthChecks(healthManager, cfg, diagramRenderer)

	// Initialize message parser and webhook validator
	messageParser := teams.NewMessageParser(logger)
	webhookValidator := teams.NewWebhookValidator(cfg.Teams.WebhookSecret, logger)

	// Initialize orchestrator
	orchestrator := teams.NewOrchestrator(cfg, healthManager, diagramRenderer, sessionManager, logger)

	// Health check endpoint
	router.GET("/health", gin.WrapH(healthManager.HTTPHandler()))

	// Teams webhook endpoint
	router.POST("/teams-webhook", func(c *gin.Context) {
		handleTeamsWebhook(c, cfg, orchestrator, messageParser, webhookValidator, queryClassifier, logger)
	})

	// Feedback endpoint
	router.POST("/teams-feedback", func(c *gin.Context) {
		handleFeedback(c, cfg, feedbackLogger, logger)
	})

	// Regeneration endpoint
	router.POST("/teams-regenerate", func(c *gin.Context) {
		handleRegeneration(c, cfg, orchestrator, feedbackLogger, logger)
	})

	// Clarification endpoint
	router.POST("/teams-clarify", func(c *gin.Context) {
		handleClarification(c, cfg, orchestrator, feedbackLogger, logger)
	})

	// Conversation API endpoints (if enabled)
	if cfg.Session.EnableConversationAPI {
		conversationAPIHandler := conversation.NewAPIHandler(conversationManager, logger)
		conversationAPIHandler.RegisterRoutes(router)
		logger.Info("Conversation API endpoints enabled")
	}

	logger.Info("Starting teamsbot service",
		zap.String("retrieve_url", cfg.Services.RetrieveURL),
		zap.String("synthesize_url", cfg.Services.SynthesizeURL),
		zap.String("websearch_url", cfg.Services.WebSearchURL),
		zap.String("diagram_renderer_url", cfg.Diagram.MermaidInkURL))

	if err := router.Run(":8080"); err != nil {
		logger.Fatal("Failed to start server", zap.Error(err))
	}
}

// handleTeamsWebhook handles incoming Teams webhook messages with enhanced parsing and validation
func handleTeamsWebhook(
	c *gin.Context,
	cfg *config.Config,
	orchestrator *teams.Orchestrator,
	messageParser *teams.MessageParser,
	webhookValidator *teams.WebhookValidator,
	queryClassifier *classifier.QueryClassifier,
	logger *zap.Logger,
) {
	// Read request body for validation
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.Error("Failed to read request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Restore body for JSON parsing
	c.Request.Body = io.NopCloser(strings.NewReader(string(body)))

	// Validate webhook security
	validationResult := webhookValidator.ValidateWebhook(c.Request, body)
	webhookValidator.LogValidationAttempt(c.Request, validationResult)

	if !validationResult.Valid {
		logger.Warn("Webhook validation failed", zap.String("error", validationResult.ErrorMessage))
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "Webhook validation failed",
			"details": validationResult.ErrorMessage,
		})
		return
	}

	// Parse Teams message
	var message teams.Message
	if err := c.ShouldBindJSON(&message); err != nil {
		logger.Error("Failed to parse Teams message", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid message format"})
		return
	}

	// Parse and validate message content
	parsedQuery, err := messageParser.ParseMessage(&message)
	if err != nil {
		logger.Error("Failed to parse Teams message content", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid message content",
			"details": err.Error(),
		})
		return
	}

	// Check if message should be processed
	if !messageParser.ShouldProcessMessage(parsedQuery) {
		logger.Debug("Message does not require processing",
			zap.Bool("is_direct_message", parsedQuery.IsDirectMessage),
			zap.Bool("bot_mentioned", parsedQuery.IsBotMentioned))
		c.JSON(http.StatusOK, gin.H{"message": "Message acknowledged but not processed"})
		return
	}

	// Classify query for cloud-related topics
	classificationResult := queryClassifier.ClassifyQuery(parsedQuery.Query)
	if !classificationResult.IsCloudRelated {
		logger.Info("Query rejected - not cloud-related",
			zap.String("query", parsedQuery.Query),
			zap.String("user_id", parsedQuery.UserID),
			zap.String("category", classificationResult.Category),
			zap.Float64("confidence", classificationResult.Confidence),
			zap.String("rejection_reason", classificationResult.RejectionReason),
		)

		// Send rejection message back to Teams
		rejectionMessage := queryClassifier.GetRejectionMessage(classificationResult)
		cardJSON, cardErr := teams.GenerateSimpleCard("Topic Not Supported", rejectionMessage)
		if cardErr != nil {
			logger.Error("Failed to generate rejection card", zap.Error(cardErr))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process rejection"})
			return
		}

		payload, cardErr := teams.CreateTeamsPayload(cardJSON)
		if cardErr != nil {
			logger.Error("Failed to create rejection payload", zap.Error(cardErr))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process rejection"})
			return
		}

		req, cardErr := http.NewRequest("POST", cfg.Teams.WebhookURL, strings.NewReader(payload))
		if cardErr != nil {
			logger.Error("Failed to create rejection webhook request", zap.Error(cardErr))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process rejection"})
			return
		}
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, cardErr := client.Do(req)
		if cardErr != nil {
			logger.Error("Failed to send rejection webhook", zap.Error(cardErr))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send rejection"})
			return
		}
		defer func() { _ = resp.Body.Close() }()

		c.JSON(http.StatusOK, gin.H{
			"message": "Query rejected - not cloud-related",
			"classification": gin.H{
				"is_cloud_related": false,
				"category":         classificationResult.Category,
				"confidence":       classificationResult.Confidence,
				"rejection_reason": classificationResult.RejectionReason,
			},
		})
		return
	}

	logger.Info("Query accepted - cloud-related",
		zap.String("query", parsedQuery.Query),
		zap.String("user_id", parsedQuery.UserID),
		zap.String("category", classificationResult.Category),
		zap.Float64("confidence", classificationResult.Confidence),
	)

	// Check if query needs clarification before processing
	analyzer := clarification.NewAnalyzer()
	analysis, err := analyzer.AnalyzeQuery(context.Background(), parsedQuery.Query, []session.Message{})
	if err != nil {
		logger.Error("Failed to analyze query for clarification", zap.Error(err))
		// Continue with normal processing if analysis fails
	} else if analysis.RequiresClarification {
		logger.Info("Query requires clarification",
			zap.String("query", parsedQuery.Query),
			zap.Bool("is_ambiguous", analysis.IsAmbiguous),
			zap.Bool("is_incomplete", analysis.IsIncomplete),
			zap.Float64("ambiguity_score", analysis.AmbiguityScore),
			zap.Float64("completeness_score", analysis.CompletenessScore))

		// Generate clarification request
		clarificationReq, err := analyzer.GenerateClarificationRequest(context.Background(), analysis)
		if err != nil {
			logger.Error("Failed to generate clarification request", zap.Error(err))
			// Continue with normal processing if clarification generation fails
		} else {
			// Convert to simple slices for card generation
			questions := make([]string, len(clarificationReq.Questions))
			for i, q := range clarificationReq.Questions {
				questions[i] = q.Question
			}

			suggestions := clarificationReq.Suggestions

			quickOptions := make([]string, len(clarificationReq.QuickOptions))
			for i, opt := range clarificationReq.QuickOptions {
				quickOptions[i] = opt.Text
			}

			// Generate and send clarification card
			clarificationCard, err := teams.GenerateClarificationCard(
				parsedQuery.Query,
				questions,
				suggestions,
				quickOptions,
			)
			if err != nil {
				logger.Error("Failed to generate clarification card", zap.Error(err))
				// Continue with normal processing if card generation fails
			} else {
				// Send clarification card to Teams
				payload, err := teams.CreateTeamsPayload(clarificationCard)
				if err != nil {
					logger.Error("Failed to create clarification payload", zap.Error(err))
				} else {
					req, err := http.NewRequest("POST", cfg.Teams.WebhookURL, strings.NewReader(payload))
					if err != nil {
						logger.Error("Failed to create clarification webhook request", zap.Error(err))
					} else {
						req.Header.Set("Content-Type", "application/json")

						client := &http.Client{}
						resp, err := client.Do(req)
						if err != nil {
							logger.Error("Failed to send clarification webhook", zap.Error(err))
						} else {
							defer func() { _ = resp.Body.Close() }()
							logger.Info("Clarification sent to Teams", zap.String("query", parsedQuery.Query))

							c.JSON(http.StatusOK, gin.H{
								"message": "Clarification requested",
								"clarification": gin.H{
									"is_ambiguous":        analysis.IsAmbiguous,
									"is_incomplete":       analysis.IsIncomplete,
									"ambiguity_score":     analysis.AmbiguityScore,
									"completeness_score":  analysis.CompletenessScore,
									"questions_count":     len(questions),
									"suggestions_count":   len(suggestions),
									"quick_options_count": len(quickOptions),
								},
							})
							return
						}
					}
				}
			}
		}
	}

	// Extract user ID for session management
	userID := session.ExtractUserIDFromContext(parsedQuery.UserID, c.GetHeader("X-User-ID"), c.ClientIP())

	logger.Info("Processing Teams query",
		zap.String("query", parsedQuery.Query),
		zap.String("user_id", parsedQuery.UserID),
		zap.String("session_user_id", userID),
		zap.String("conversation_id", parsedQuery.ConversationID),
		zap.Bool("is_direct_message", parsedQuery.IsDirectMessage),
		zap.Bool("bot_mentioned", parsedQuery.IsBotMentioned))

	// Process the query asynchronously to meet Teams webhook timeout requirements
	const webhookTimeout = 18 * time.Second // Leave 2 seconds buffer for response processing
	ctx, cancel := context.WithTimeout(context.Background(), webhookTimeout)
	defer cancel()

	// Channel to receive orchestration result
	resultChan := make(chan *teams.OrchestrationResult, 1)

	// Start async processing
	go func() {
		result := orchestrator.ProcessQuery(ctx, parsedQuery.Query, userID)
		resultChan <- result
	}()

	// Wait for result or timeout
	select {
	case result := <-resultChan:
		if result.Error != nil {
			logger.Error("Failed to process query", zap.Error(result.Error))
			sendErrorResponseWithResult(c, cfg, parsedQuery.Query, result, logger)
			return
		}

		// Send response back to Teams
		if err := sendTeamsResponseWithResult(c, cfg, parsedQuery.Query, result, logger); err != nil {
			logger.Error("Failed to send Teams response", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send response"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":           "Query processed successfully",
			"execution_time_ms": result.ExecutionTimeMs,
			"services_used":     result.ServicesUsed,
			"fallback_used":     result.FallbackUsed,
			"security_level":    validationResult.SecurityLevel,
			"user_id":           parsedQuery.UserID,
		})

	case <-ctx.Done():
		logger.Error("Query processing timed out",
			zap.String("query", parsedQuery.Query),
			zap.String("user_id", parsedQuery.UserID),
			zap.String("session_user_id", userID))

		// Send timeout response to Teams
		timeoutResult := &teams.OrchestrationResult{
			Error:           fmt.Errorf("query processing timed out"),
			FallbackUsed:    true,
			ExecutionTimeMs: webhookTimeout.Milliseconds(),
		}

		sendErrorResponseWithResult(c, cfg, parsedQuery.Query, timeoutResult, logger)
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "Query processing timed out"})
	}
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

// sendErrorResponseWithResult sends an error response to Teams with orchestration result context
func sendErrorResponseWithResult(
	_ *gin.Context,
	cfg *config.Config,
	query string,
	result *teams.OrchestrationResult,
	logger *zap.Logger,
) {
	var errorMessage string
	if result.FallbackUsed {
		errorMessage = fmt.Sprintf(
			"I encountered some issues while processing your query: %s. "+
				"I've provided a fallback response based on available information.",
			query,
		)
	} else {
		errorMessage = fmt.Sprintf(
			"I encountered an error processing your request: %s. Services tested: %s. Execution time: %dms",
			result.Error.Error(),
			strings.Join(result.ServicesTested, ", "),
			result.ExecutionTimeMs,
		)
	}

	cardJSON, cardErr := teams.GenerateSimpleCard("Service Error", errorMessage)
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

// sendTeamsResponseWithResult sends the response back to Teams with orchestration result
func sendTeamsResponseWithResult(
	_ *gin.Context,
	cfg *config.Config,
	query string,
	result *teams.OrchestrationResult,
	logger *zap.Logger,
) error {
	// Generate Adaptive Card
	cardJSON, err := teams.GenerateCard(*result.Response, query, result.Response.DiagramURL)
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

	logger.Info("Successfully sent Teams response",
		zap.String("query", query),
		zap.Int64("execution_time_ms", result.ExecutionTimeMs),
		zap.Strings("services_used", result.ServicesUsed),
		zap.Bool("fallback_used", result.FallbackUsed))

	return nil
}

// FeedbackRequest represents a feedback request
type FeedbackRequest struct {
	Query      string `json:"query"`
	ResponseID string `json:"response_id,omitempty"`
	Feedback   string `json:"feedback"`
	Timestamp  string `json:"timestamp,omitempty"`
}

// RegenerationRequest represents a request to regenerate a response
type RegenerationRequest struct {
	Query            string `json:"query"`
	ResponseID       string `json:"response_id,omitempty"`
	PreviousResponse string `json:"previous_response,omitempty"`
	Action           string `json:"action"` // "show_options" or "regenerate"
	Preset           string `json:"preset,omitempty"`
	Timestamp        string `json:"timestamp,omitempty"`
}

// ClarificationRequest represents a request for clarification handling
type ClarificationRequest struct {
	OriginalQuery string `json:"original_query"`
	Action        string `json:"action"` // "quick_select", "provide_details", "use_template",
	// "apply_template", "show_templates", "back_to_clarification"
	Clarification string `json:"clarification,omitempty"`
	Template      string `json:"template,omitempty"`
	Timestamp     string `json:"timestamp,omitempty"`
}

// handleFeedback handles feedback submissions
func handleFeedback(c *gin.Context, _ *config.Config, feedbackLogger *feedback.Logger, logger *zap.Logger) {
	var feedbackRequest FeedbackRequest
	if err := c.ShouldBindJSON(&feedbackRequest); err != nil {
		logger.Error("Failed to parse feedback", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid feedback format"})
		return
	}

	// Privacy controls: sanitize sensitive information from query
	sanitizedQuery := sanitizeFeedbackQuery(feedbackRequest.Query)

	// Validate feedback type
	if feedbackRequest.Feedback != "positive" && feedbackRequest.Feedback != "negative" {
		logger.Error("Invalid feedback type", zap.String("feedback", feedbackRequest.Feedback))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid feedback type"})
		return
	}

	// Store feedback using the feedback logger with enhanced context
	userID := extractUserIDFromRequest(c)
	sessionID := feedbackRequest.ResponseID // Use response_id as session correlation

	if err := feedbackLogger.LogFeedbackWithContext(
		sanitizedQuery,
		feedbackRequest.Feedback,
		userID,
		sessionID,
	); err != nil {
		logger.Error("Failed to log feedback", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store feedback"})
		return
	}

	logger.Info("Received and stored feedback",
		zap.String("query", sanitizedQuery),
		zap.String("feedback", feedbackRequest.Feedback),
		zap.String("response_id", feedbackRequest.ResponseID),
		zap.String("user_id", userID))

	c.JSON(http.StatusOK, gin.H{"message": "Feedback received"})
}

// handleRegeneration handles regeneration requests
func handleRegeneration(
	c *gin.Context,
	cfg *config.Config,
	orchestrator *teams.Orchestrator,
	feedbackLogger *feedback.Logger,
	logger *zap.Logger,
) {
	var regenRequest RegenerationRequest
	if err := c.ShouldBindJSON(&regenRequest); err != nil {
		logger.Error("Failed to parse regeneration request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid regeneration request format"})
		return
	}

	logger.Info("Received regeneration request",
		zap.String("query", regenRequest.Query),
		zap.String("action", regenRequest.Action),
		zap.String("preset", regenRequest.Preset),
		zap.String("response_id", regenRequest.ResponseID))

	switch regenRequest.Action {
	case "show_options":
		// Generate and send regeneration options card
		optionsCard, err := teams.GenerateRegenerationOptionsCard(
			regenRequest.Query,
			regenRequest.ResponseID,
			regenRequest.PreviousResponse,
		)
		if err != nil {
			logger.Error("Failed to generate regeneration options card", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate options"})
			return
		}

		// Send the options card to Teams
		if err := sendCardToTeams(cfg, optionsCard, logger); err != nil {
			logger.Error("Failed to send options card to Teams", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send options"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Regeneration options sent"})

	case "regenerate":
		// Validate preset
		if regenRequest.Preset == "" {
			logger.Error("No preset specified for regeneration")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Preset required for regeneration"})
			return
		}

		// Call orchestrator to perform regeneration
		result, err := orchestrator.ProcessRegenerationQuery(
			context.Background(),
			regenRequest.Query,
			regenRequest.Preset,
			regenRequest.PreviousResponse,
		)
		if err != nil {
			logger.Error("Failed to process regeneration", zap.Error(err))
			sendErrorCardToTeams(cfg, regenRequest.Query, fmt.Sprintf("Regeneration failed: %v", err), logger)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Regeneration failed"})
			return
		}

		// Generate comparison card with original and regenerated responses
		comparisonCard, err := teams.GenerateComparisonCard(
			regenRequest.Query,
			regenRequest.PreviousResponse,
			result.Response.MainText,
			regenRequest.Preset,
		)
		if err != nil {
			logger.Error("Failed to generate comparison card", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate comparison"})
			return
		}

		// Send the comparison card to Teams
		if err := sendCardToTeams(cfg, comparisonCard, logger); err != nil {
			logger.Error("Failed to send comparison card to Teams", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send comparison"})
			return
		}

		// Log the regeneration for feedback analysis
		userID := extractUserIDFromRequest(c)
		if err := feedbackLogger.LogFeedbackWithContext(
			regenRequest.Query,
			fmt.Sprintf("regeneration_requested_%s", regenRequest.Preset),
			userID,
			regenRequest.ResponseID,
		); err != nil {
			logger.Warn("Failed to log regeneration feedback", zap.Error(err))
		}

		c.JSON(http.StatusOK, gin.H{"message": "Regeneration completed"})

	default:
		logger.Error("Invalid regeneration action", zap.String("action", regenRequest.Action))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid action"})
	}
}

// sendCardToTeams sends an Adaptive Card to Teams via webhook
func sendCardToTeams(cfg *config.Config, cardJSON string, logger *zap.Logger) error {
	payload, err := teams.CreateTeamsPayload(cardJSON)
	if err != nil {
		return fmt.Errorf("failed to create Teams payload: %w", err)
	}

	resp, err := http.Post(cfg.Teams.WebhookURL, "application/json", strings.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to send to Teams: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.Error("failed to close response body", zap.Error(closeErr))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Teams webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	logger.Debug("Successfully sent card to Teams", zap.Int("status_code", resp.StatusCode))
	return nil
}

// sendErrorCardToTeams sends a simple error card to Teams
func sendErrorCardToTeams(cfg *config.Config, _, errorMessage string, logger *zap.Logger) {
	errorCard, err := teams.GenerateSimpleCard("❌ Error", errorMessage)
	if err != nil {
		logger.Error("Failed to generate error card", zap.Error(err))
		return
	}

	if err := sendCardToTeams(cfg, errorCard, logger); err != nil {
		logger.Error("Failed to send error card to Teams", zap.Error(err))
	}
}

// sanitizeFeedbackQuery removes sensitive information from queries before logging
func sanitizeFeedbackQuery(query string) string {
	// Remove potential sensitive patterns
	sensitivePatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)password[:\s=]+[^\s]+`),
		regexp.MustCompile(`(?i)api[_-]?key[:\s=]+[^\s]+`),
		regexp.MustCompile(`(?i)secret[:\s=]+[^\s]+`),
		regexp.MustCompile(`(?i)token[:\s=]+[^\s]+`),
		regexp.MustCompile(`(?i)credential[s]?[:\s=]+[^\s]+`),
		regexp.MustCompile(`[A-Za-z0-9+/]{20,}={0,2}`), // Base64 encoded strings
		regexp.MustCompile(`[0-9a-fA-F]{32,}`),         // Hex strings (potential keys)
	}

	sanitized := query
	for _, pattern := range sensitivePatterns {
		sanitized = pattern.ReplaceAllString(sanitized, "[REDACTED]")
	}

	// Limit length to prevent abuse
	const maxQueryLength = 500
	if len(sanitized) > maxQueryLength {
		sanitized = sanitized[:maxQueryLength] + "..."
	}

	return sanitized
}

// extractUserIDFromRequest extracts user ID from request headers or context
func extractUserIDFromRequest(c *gin.Context) string {
	// Try to extract from Teams context if available
	if userID := c.GetHeader("X-Teams-User-ID"); userID != "" {
		return userID
	}

	// Try to extract from authentication headers
	if userID := c.GetHeader("X-User-ID"); userID != "" {
		return userID
	}

	// Try to extract from client IP as fallback
	if clientIP := c.ClientIP(); clientIP != "" {
		return fmt.Sprintf("ip:%s", clientIP)
	}

	return "anonymous"
}

// handleClarification handles clarification requests
func handleClarification(c *gin.Context, cfg *config.Config, orchestrator *teams.Orchestrator, _ *feedback.Logger, logger *zap.Logger) {
	var clarifyRequest ClarificationRequest
	if err := c.ShouldBindJSON(&clarifyRequest); err != nil {
		logger.Error("Failed to parse clarification request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid clarification request format"})
		return
	}

	logger.Info("Received clarification request",
		zap.String("original_query", clarifyRequest.OriginalQuery),
		zap.String("action", clarifyRequest.Action),
		zap.String("clarification", clarifyRequest.Clarification),
		zap.String("template", clarifyRequest.Template))

	switch clarifyRequest.Action {
	case "quick_select":
		// User selected a quick option - enhance query and proceed
		enhancedQuery := clarifyRequest.OriginalQuery + " " + clarifyRequest.Clarification
		logger.Info("Processing enhanced query from quick selection",
			zap.String("original", clarifyRequest.OriginalQuery),
			zap.String("enhanced", enhancedQuery))

		// Process the enhanced query through normal flow
		userID := extractUserIDFromRequest(c)
		result := orchestrator.ProcessQuery(context.Background(), enhancedQuery, userID)
		if result.Error != nil {
			logger.Error("Failed to process enhanced query", zap.Error(result.Error))
			sendErrorCard(cfg, enhancedQuery, fmt.Sprintf("Processing failed: %v", result.Error), logger)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Processing failed"})
			return
		}

		// Generate and send response card
		if err := sendResponseCard(cfg, result, enhancedQuery, logger); err != nil {
			logger.Error("Failed to send response card", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send response"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Enhanced query processed"})

	case "provide_details":
		// Show guidance for providing more details
		detailsMessage := "Please add more specific information to your question:\n\n" +
			"• Include specific technologies, versions, or tools\n" +
			"• Mention your environment (production, development, etc.)\n" +
			"• Specify requirements, constraints, or goals\n" +
			"• Add context about current setup or challenges\n\n" +
			"Then send your updated question as a new message."

		detailsCard, err := teams.GenerateSimpleCard(
			"📝 Provide More Details",
			detailsMessage,
		)
		if err != nil {
			logger.Error("Failed to generate details card", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate guidance"})
			return
		}

		if err := sendCardToTeams(cfg, detailsCard, logger); err != nil {
			logger.Error("Failed to send details card", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send guidance"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Details guidance sent"})

	case "use_template":
		// Show template selection card
		templateCard, err := teams.GenerateTemplateSelectionCard(clarifyRequest.OriginalQuery)
		if err != nil {
			logger.Error("Failed to generate template selection card", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate templates"})
			return
		}

		if err := sendCardToTeams(cfg, templateCard, logger); err != nil {
			logger.Error("Failed to send template card", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send templates"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Template selection sent"})

	case "apply_template":
		// Show specific template guidance
		if clarifyRequest.Template == "" {
			logger.Error("Template type not specified")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Template type required"})
			return
		}

		guidedCard, err := teams.GenerateGuidedTemplateCard(clarifyRequest.OriginalQuery, clarifyRequest.Template)
		if err != nil {
			logger.Error("Failed to generate guided template card", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate template"})
			return
		}

		if err := sendCardToTeams(cfg, guidedCard, logger); err != nil {
			logger.Error("Failed to send guided template card", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send template"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Guided template sent"})

	case "show_templates":
		// Navigate back to template selection
		templateCard, err := teams.GenerateTemplateSelectionCard(clarifyRequest.OriginalQuery)
		if err != nil {
			logger.Error("Failed to generate template selection card", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate templates"})
			return
		}

		if err := sendCardToTeams(cfg, templateCard, logger); err != nil {
			logger.Error("Failed to send template card", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send templates"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Template selection sent"})

	case "back_to_clarification":
		// Generate a new clarification analysis
		analyzer := clarification.NewAnalyzer()
		analysis, err := analyzer.AnalyzeQuery(context.Background(), clarifyRequest.OriginalQuery, []session.Message{})
		if err != nil {
			logger.Error("Failed to analyze query for clarification", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Analysis failed"})
			return
		}

		clarificationReq, err := analyzer.GenerateClarificationRequest(context.Background(), analysis)
		if err != nil {
			logger.Error("Failed to generate clarification request", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Clarification generation failed"})
			return
		}

		// Convert to simple slices for card generation
		questions := make([]string, len(clarificationReq.Questions))
		for i, q := range clarificationReq.Questions {
			questions[i] = q.Question
		}

		suggestions := clarificationReq.Suggestions

		quickOptions := make([]string, len(clarificationReq.QuickOptions))
		for i, opt := range clarificationReq.QuickOptions {
			quickOptions[i] = opt.Text
		}

		clarificationCard, err := teams.GenerateClarificationCard(
			clarifyRequest.OriginalQuery,
			questions,
			suggestions,
			quickOptions,
		)
		if err != nil {
			logger.Error("Failed to generate clarification card", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate clarification"})
			return
		}

		if err := sendCardToTeams(cfg, clarificationCard, logger); err != nil {
			logger.Error("Failed to send clarification card", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send clarification"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Clarification sent"})

	default:
		logger.Error("Invalid clarification action", zap.String("action", clarifyRequest.Action))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid action"})
	}
}

// Helper functions for sending cards
func sendResponseCard(cfg *config.Config, result *teams.OrchestrationResult, query string, logger *zap.Logger) error {
	// Generate diagram URL if diagram exists
	var diagramURL string
	if result.Response.DiagramCode != "" {
		diagramURL = generateDiagramURL(result.Response.DiagramCode)
	}

	// Generate the main response card
	cardJSON, err := teams.GenerateCard(*result.Response, query, diagramURL)
	if err != nil {
		return fmt.Errorf("failed to generate response card: %w", err)
	}

	return sendCardToTeams(cfg, cardJSON, logger)
}

func sendErrorCard(cfg *config.Config, _, errorMessage string, logger *zap.Logger) {
	errorCard, err := teams.GenerateSimpleCard("❌ Error", errorMessage)
	if err != nil {
		logger.Error("Failed to generate error card", zap.Error(err))
		return
	}

	if err := sendCardToTeams(cfg, errorCard, logger); err != nil {
		logger.Error("Failed to send error card to Teams", zap.Error(err))
	}
}

func generateDiagramURL(_ string) string {
	// Simple placeholder - in real implementation this would call diagram service
	return ""
}

// setupHealthChecks configures health checks for the teamsbot service
func setupHealthChecks(manager *health.Manager, cfg *config.Config, diagramRenderer *diagram.Renderer) {
	// Retrieve service health check
	manager.AddChecker("retrieve", health.HTTPHealthChecker(cfg.Services.RetrieveURL+"/health", &http.Client{
		Timeout: HealthCheckTimeout,
	}))

	// Synthesize service health check
	manager.AddChecker("synthesize", health.HTTPHealthChecker(cfg.Services.SynthesizeURL+"/health", &http.Client{
		Timeout: HealthCheckTimeout,
	}))

	// WebSearch service health check
	manager.AddChecker("websearch", health.HTTPHealthChecker(cfg.Services.WebSearchURL+"/health", &http.Client{
		Timeout: HealthCheckTimeout,
	}))

	// Diagram renderer health check
	manager.AddCheckerFunc("diagram_renderer", func(ctx context.Context) health.CheckResult {
		if err := diagramRenderer.TestConnection(ctx); err != nil {
			return health.CheckResult{
				Status:    health.StatusDegraded,
				Error:     fmt.Sprintf("Diagram renderer connection failed: %v", err),
				Timestamp: time.Now(),
			}
		}

		return health.CheckResult{
			Status:    health.StatusHealthy,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"mermaid_ink_url": cfg.Diagram.MermaidInkURL,
				"caching_enabled": cfg.Diagram.EnableCaching,
			},
		}
	})

	// Teams webhook configuration health check
	manager.AddCheckerFunc("teams_webhook", func(_ context.Context) health.CheckResult {
		if cfg.Teams.WebhookURL == "" {
			return health.CheckResult{
				Status:    health.StatusUnhealthy,
				Error:     "Teams webhook URL not configured",
				Timestamp: time.Now(),
			}
		}

		return health.CheckResult{
			Status:    health.StatusHealthy,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"webhook_configured": true,
			},
		}
	})

	// Service endpoints configuration health check
	manager.AddCheckerFunc("service_endpoints", func(_ context.Context) health.CheckResult {
		var errors []string

		if cfg.Services.RetrieveURL == "" {
			errors = append(errors, "retrieve service URL not configured")
		}
		if cfg.Services.SynthesizeURL == "" {
			errors = append(errors, "synthesize service URL not configured")
		}
		if cfg.Services.WebSearchURL == "" {
			errors = append(errors, "websearch service URL not configured")
		}

		if len(errors) > 0 {
			return health.CheckResult{
				Status:    health.StatusUnhealthy,
				Error:     fmt.Sprintf("Service configuration errors: %s", strings.Join(errors, ", ")),
				Timestamp: time.Now(),
			}
		}

		return health.CheckResult{
			Status:    health.StatusHealthy,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"retrieve_url":   cfg.Services.RetrieveURL,
				"synthesize_url": cfg.Services.SynthesizeURL,
				"websearch_url":  cfg.Services.WebSearchURL,
			},
		}
	})

	// Set timeout for health checks
	const healthCheckTimeoutSeconds = 15
	manager.SetTimeout(healthCheckTimeoutSeconds * time.Second)
}
