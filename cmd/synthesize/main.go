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

// Package main provides the synthesis service API for the AI SA Assistant.
// It combines retrieval results with web search to generate comprehensive responses.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/health"
	internalopenai "github.com/your-org/ai-sa-assistant/internal/openai"
	"github.com/your-org/ai-sa-assistant/internal/synth"
)

const (
	// MaxQueryLength defines the maximum length for query text
	MaxQueryLength = 10000
	// HealthCheckTimeout defines the timeout for health checks
	HealthCheckTimeout = 5 * time.Second
	// SynthesisRequestTimeout defines the timeout for synthesis requests
	SynthesisRequestTimeout = 30 * time.Second
)

// SynthesisRequest represents the incoming synthesis request
type SynthesisRequest struct {
	Query      string      `json:"query" binding:"required"`
	Chunks     []ChunkItem `json:"chunks"`
	WebResults []WebResult `json:"web_results"`
}

// ChunkItem represents a document chunk with metadata
type ChunkItem struct {
	Text     string `json:"text" binding:"required"`
	DocID    string `json:"doc_id" binding:"required"`
	SourceID string `json:"source_id"`
}

// WebResult represents a web search result
type WebResult struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	URL     string `json:"url"`
}

// validateSynthesisRequest validates the synthesis request
func validateSynthesisRequest(req SynthesisRequest) error {
	if strings.TrimSpace(req.Query) == "" {
		return fmt.Errorf("query cannot be empty")
	}

	if len(req.Query) > MaxQueryLength {
		return fmt.Errorf("query is too long (max %d characters)", MaxQueryLength)
	}

	if len(req.Chunks) == 0 && len(req.WebResults) == 0 {
		return fmt.Errorf("at least one chunk or web result must be provided")
	}

	// Validate chunks
	for i, chunk := range req.Chunks {
		if strings.TrimSpace(chunk.Text) == "" {
			return fmt.Errorf("chunk %d text cannot be empty", i)
		}
		if strings.TrimSpace(chunk.DocID) == "" {
			return fmt.Errorf("chunk %d doc_id cannot be empty", i)
		}
		// SourceID is optional, so no validation required
	}

	// Validate web results
	for i, webResult := range req.WebResults {
		if strings.TrimSpace(webResult.Title) == "" && strings.TrimSpace(webResult.Snippet) == "" {
			return fmt.Errorf("web result %d must have either title or snippet", i)
		}
	}

	return nil
}

func main() {
	// Setup configuration and logger
	cfg, logger := setupConfiguration()
	defer func() { _ = logger.Sync() }()

	// Initialize services
	openaiClient := setupServices(cfg, logger)

	// Setup router and handlers
	router := setupRouter(cfg, logger, openaiClient)

	// Start server
	startServer(router, cfg, logger)
}

// setupConfiguration loads configuration and initializes logger
func setupConfiguration() (*config.Config, *zap.Logger) {
	// Check if running in test mode
	testMode := os.Getenv("TEST_MODE") == "true" || os.Getenv("CI") == "true"

	var cfg *config.Config
	var err error

	if testMode {
		cfg, err = config.LoadWithOptions(config.LoadOptions{
			ConfigPath:       "",
			EnableHotReload:  false,
			Environment:      "test",
			ValidateRequired: false,
			TestMode:         true,
		})
	} else {
		cfg, err = config.Load("")
	}
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	logger, err := initializeLogger(cfg)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	return cfg, logger
}

// setupServices initializes all required services
func setupServices(cfg *config.Config, logger *zap.Logger) *internalopenai.Client {
	openaiClient, err := internalopenai.NewClient(cfg.OpenAI.APIKey, logger)
	if err != nil {
		logger.Fatal("Failed to initialize OpenAI client", zap.Error(err))
	}

	// Log configuration with masked sensitive values
	maskedConfig := cfg.MaskSensitiveValues()
	logger.Info("Configuration loaded successfully",
		zap.String("service", "synthesize"),
		zap.String("environment", os.Getenv("ENVIRONMENT")),
		zap.String("synthesis_model", maskedConfig.Synthesis.Model),
		zap.Int("max_tokens", maskedConfig.Synthesis.MaxTokens),
		zap.Float64("temperature", maskedConfig.Synthesis.Temperature),
		zap.String("openai_endpoint", maskedConfig.OpenAI.Endpoint),
		zap.String("openai_api_key", maskedConfig.OpenAI.APIKey),
	)

	return openaiClient
}

// setupRouter creates and configures the Gin router with all endpoints
func setupRouter(cfg *config.Config, logger *zap.Logger, openaiClient *internalopenai.Client) *gin.Engine {
	// Set Gin mode based on log level
	if cfg.Logging.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.Default()

	// Initialize health check manager
	healthManager := health.NewManager("synthesize", "1.0.0", logger)
	setupHealthChecks(healthManager, cfg, openaiClient)

	router.GET("/health", gin.WrapH(healthManager.HTTPHandler()))
	router.POST("/synthesize", createSynthesisHandler(cfg, logger, openaiClient))

	return router
}

// setupHealthChecks configures health checks for the synthesize service
func setupHealthChecks(manager *health.Manager, cfg *config.Config, openaiClient *internalopenai.Client) {
	// OpenAI health check
	manager.AddCheckerFunc("openai", func(ctx context.Context) health.CheckResult {
		if _, err := openaiClient.EmbedTexts(ctx, []string{"health check"}); err != nil {
			return health.CheckResult{
				Status:    health.StatusUnhealthy,
				Error:     fmt.Sprintf("OpenAI API health check failed: %v", err),
				Timestamp: time.Now(),
			}
		}

		return health.CheckResult{
			Status:    health.StatusHealthy,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"model":       cfg.Synthesis.Model,
				"max_tokens":  cfg.Synthesis.MaxTokens,
				"temperature": cfg.Synthesis.Temperature,
			},
		}
	})

	// Synthesis configuration health check
	manager.AddCheckerFunc("synthesis_config", func(_ context.Context) health.CheckResult {
		// Validate synthesis configuration
		if cfg.Synthesis.Model == "" {
			return health.CheckResult{
				Status:    health.StatusUnhealthy,
				Error:     "Synthesis model not configured",
				Timestamp: time.Now(),
			}
		}

		if cfg.Synthesis.MaxTokens <= 0 {
			return health.CheckResult{
				Status:    health.StatusDegraded,
				Error:     "Invalid max tokens configuration",
				Timestamp: time.Now(),
			}
		}

		return health.CheckResult{
			Status:    health.StatusHealthy,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"model":       cfg.Synthesis.Model,
				"max_tokens":  cfg.Synthesis.MaxTokens,
				"temperature": cfg.Synthesis.Temperature,
			},
		}
	})

	// Set timeout for health checks
	manager.SetTimeout(HealthCheckTimeout)
}

// createSynthesisHandler creates the synthesis endpoint handler
func createSynthesisHandler(
	cfg *config.Config,
	logger *zap.Logger,
	openaiClient *internalopenai.Client,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		logger.Info("Synthesis request received",
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.GetHeader("User-Agent")),
		)

		// Parse and validate request
		req, valid := parseSynthesisRequest(c, logger)
		if !valid {
			return
		}

		// Process the synthesis request
		response, err := processSynthesisRequest(req, cfg, logger, openaiClient)
		if err != nil {
			logger.Error("Synthesis processing failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to process synthesis request",
				"details": err.Error(),
			})
			return
		}

		// Log completion and return response
		processingTime := time.Since(startTime)
		logSynthesisCompletion(req, response, processingTime, logger)
		c.JSON(http.StatusOK, buildSynthesisResponse(response, cfg, processingTime))
	}
}

// parseSynthesisRequest parses and validates the synthesis request
func parseSynthesisRequest(c *gin.Context, logger *zap.Logger) (SynthesisRequest, bool) {
	var req SynthesisRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error("Failed to parse synthesis request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request format",
			"details": err.Error(),
		})
		return req, false
	}

	if err := validateSynthesisRequest(req); err != nil {
		logger.Error("Invalid synthesis request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request",
			"details": err.Error(),
		})
		return req, false
	}

	return req, true
}

// processSynthesisRequest handles the core synthesis logic
func processSynthesisRequest(
	req SynthesisRequest,
	cfg *config.Config,
	logger *zap.Logger,
	openaiClient *internalopenai.Client,
) (*internalopenai.ChatCompletionResponse, error) {
	// Convert request to internal format
	contextItems := convertChunksToContextItems(req.Chunks)
	webResultStrings, webSourceURLs := convertWebResults(req.WebResults)

	// Validate source metadata
	if err := validateSynthesisSourceMetadata(contextItems, webSourceURLs, logger); err != nil {
		logger.Warn("Source metadata validation failed, continuing with available data",
			zap.Error(err),
			zap.Int("context_items", len(contextItems)),
			zap.Int("web_sources", len(webSourceURLs)))
		// Continue processing despite validation warnings
	}

	// Build comprehensive prompt
	prompt := synth.BuildPrompt(req.Query, contextItems, webResultStrings)

	// Call OpenAI Chat Completion API
	ctx, cancel := context.WithTimeout(context.Background(), SynthesisRequestTimeout)
	defer cancel()

	return openaiClient.CreateChatCompletion(ctx, internalopenai.ChatCompletionRequest{
		Model:       cfg.Synthesis.Model,
		MaxTokens:   cfg.Synthesis.MaxTokens,
		Temperature: float32(cfg.Synthesis.Temperature),
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	})
}

// convertChunksToContextItems converts request chunks to internal context items
func convertChunksToContextItems(chunks []ChunkItem) []synth.ContextItem {
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
			Score:    1.0,
			Priority: 1,
		}
	}
	return contextItems
}

// convertWebResults converts web results to strings and extracts URLs
func convertWebResults(webResults []WebResult) ([]string, []string) {
	webResultStrings := make([]string, len(webResults))
	webSourceURLs := make([]string, 0, len(webResults))

	for i, webResult := range webResults {
		switch {
		case webResult.Title != "" && webResult.Snippet != "":
			webResultStrings[i] = fmt.Sprintf("Title: %s\nSnippet: %s\nURL: %s",
				webResult.Title, webResult.Snippet, webResult.URL)
		case webResult.Title != "":
			webResultStrings[i] = fmt.Sprintf("Title: %s\nURL: %s", webResult.Title, webResult.URL)
		default:
			webResultStrings[i] = fmt.Sprintf("Snippet: %s\nURL: %s", webResult.Snippet, webResult.URL)
		}

		if webResult.URL != "" {
			webSourceURLs = append(webSourceURLs, webResult.URL)
		}
	}

	return webResultStrings, webSourceURLs
}

// logSynthesisCompletion logs synthesis completion details
func logSynthesisCompletion(
	req SynthesisRequest,
	response *internalopenai.ChatCompletionResponse,
	processingTime time.Duration,
	logger *zap.Logger,
) {
	allAvailableSources := make([]string, 0, len(req.Chunks)+len(req.WebResults))
	for _, chunk := range req.Chunks {
		// Prefer SourceID if available, otherwise use DocID
		if chunk.SourceID != "" {
			allAvailableSources = append(allAvailableSources, chunk.SourceID)
		} else if chunk.DocID != "" {
			allAvailableSources = append(allAvailableSources, chunk.DocID)
		}
	}
	for _, webResult := range req.WebResults {
		if webResult.URL != "" {
			allAvailableSources = append(allAvailableSources, webResult.URL)
		}
	}

	synthesisResponse := synth.ParseResponseWithSources(response.Content, allAvailableSources)

	logger.Info("Synthesis completed",
		zap.String("query", req.Query),
		zap.Int("context_items", len(req.Chunks)),
		zap.Int("web_results", len(req.WebResults)),
		zap.Int("total_tokens", response.Usage.TotalTokens),
		zap.Int("prompt_tokens", response.Usage.PromptTokens),
		zap.Int("completion_tokens", response.Usage.CompletionTokens),
		zap.Duration("processing_time", processingTime),
		zap.Int("response_length", len(synthesisResponse.MainText)),
		zap.Int("sources_count", len(synthesisResponse.Sources)),
		zap.Int("code_snippets_count", len(synthesisResponse.CodeSnippets)),
		zap.Bool("has_diagram", synthesisResponse.DiagramCode != ""),
	)
}

// buildSynthesisResponse builds the final synthesis response
func buildSynthesisResponse(
	response *internalopenai.ChatCompletionResponse,
	cfg *config.Config,
	processingTime time.Duration,
) gin.H {
	allAvailableSources := []string{} // Simplified for response building
	synthesisResponse := synth.ParseResponseWithSources(response.Content, allAvailableSources)

	return gin.H{
		"main_text":     synthesisResponse.MainText,
		"diagram_code":  synthesisResponse.DiagramCode,
		"code_snippets": synthesisResponse.CodeSnippets,
		"sources":       synthesisResponse.Sources,
		"metadata": gin.H{
			"processing_time":   processingTime.Milliseconds(),
			"total_tokens":      response.Usage.TotalTokens,
			"prompt_tokens":     response.Usage.PromptTokens,
			"completion_tokens": response.Usage.CompletionTokens,
			"model":             cfg.Synthesis.Model,
		},
	}
}

// startServer starts the HTTP server
func startServer(router *gin.Engine, cfg *config.Config, logger *zap.Logger) {
	port := ":8082"
	logger.Info("Starting synthesize service",
		zap.String("port", port),
		zap.String("model", cfg.Synthesis.Model),
		zap.Int("max_tokens", cfg.Synthesis.MaxTokens),
		zap.Float64("temperature", cfg.Synthesis.Temperature),
	)

	if err := router.Run(port); err != nil {
		logger.Fatal("Failed to start server", zap.Error(err))
	}
}

// validateSynthesisSourceMetadata validates source metadata for synthesis
func validateSynthesisSourceMetadata(
	contextItems []synth.ContextItem,
	webSourceURLs []string,
	logger *zap.Logger,
) error {
	var errors []string

	// Validate context items have source IDs
	emptySourceCount := 0
	for i, item := range contextItems {
		if strings.TrimSpace(item.SourceID) == "" {
			emptySourceCount++
			logger.Debug("Context item missing source ID", zap.Int("item_index", i))
		}
		if strings.TrimSpace(item.Content) == "" {
			errors = append(errors, fmt.Sprintf("context item %d has empty content", i))
		}
	}

	if emptySourceCount > 0 {
		logger.Warn("Some context items missing source IDs",
			zap.Int("empty_source_count", emptySourceCount),
			zap.Int("total_items", len(contextItems)))
	}

	// Validate web source URLs
	invalidURLCount := 0
	for i, url := range webSourceURLs {
		if !isValidWebSourceURL(url) {
			invalidURLCount++
			logger.Debug("Invalid web source URL", zap.Int("url_index", i), zap.String("url", url))
		}
	}

	if invalidURLCount > 0 {
		logger.Warn("Some web source URLs are invalid",
			zap.Int("invalid_url_count", invalidURLCount),
			zap.Int("total_urls", len(webSourceURLs)))
	}

	// Check for minimum sources
	totalSources := len(contextItems) + len(webSourceURLs)
	if totalSources == 0 {
		errors = append(errors, "no sources provided for synthesis")
	}

	if len(errors) > 0 {
		return fmt.Errorf("source validation errors: %s", strings.Join(errors, ", "))
	}

	return nil
}

// isValidWebSourceURL validates a web source URL
func isValidWebSourceURL(url string) bool {
	if strings.TrimSpace(url) == "" {
		return false
	}

	url = strings.TrimSpace(url)
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

// initializeLogger creates a logger based on configuration settings
func initializeLogger(cfg *config.Config) (*zap.Logger, error) {
	var zapConfig zap.Config

	if cfg.Logging.Format == "json" {
		zapConfig = zap.NewProductionConfig()
	} else {
		zapConfig = zap.NewDevelopmentConfig()
	}

	// Set log level
	switch cfg.Logging.Level {
	case "debug":
		zapConfig.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	case "info":
		zapConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	case "warn":
		zapConfig.Level = zap.NewAtomicLevelAt(zapcore.WarnLevel)
	case "error":
		zapConfig.Level = zap.NewAtomicLevelAt(zapcore.ErrorLevel)
	default:
		zapConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}

	// Set output destination
	if cfg.Logging.Output == "file" {
		zapConfig.OutputPaths = []string{"synthesize.log"}
		zapConfig.ErrorOutputPaths = []string{"synthesize.log"}
	} else {
		zapConfig.OutputPaths = []string{"stdout"}
		zapConfig.ErrorOutputPaths = []string{"stderr"}
	}

	return zapConfig.Build()
}
