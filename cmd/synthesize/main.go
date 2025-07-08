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
	internalopenai "github.com/your-org/ai-sa-assistant/internal/openai"
	"github.com/your-org/ai-sa-assistant/internal/synth"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// SynthesisRequest represents the incoming synthesis request
type SynthesisRequest struct {
	Query      string      `json:"query" binding:"required"`
	Chunks     []ChunkItem `json:"chunks"`
	WebResults []WebResult `json:"web_results"`
}

// ChunkItem represents a document chunk with metadata
type ChunkItem struct {
	Text  string `json:"text" binding:"required"`
	DocID string `json:"doc_id" binding:"required"`
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

	if len(req.Query) > 10000 {
		return fmt.Errorf("query is too long (max 10000 characters)")
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
	// Load configuration first to get logging settings
	cfg, err := config.Load("")
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger based on configuration
	logger, err := initializeLogger(cfg)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	// Initialize OpenAI client
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

	// Set Gin mode based on log level
	if cfg.Logging.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.Default()

	// Health check endpoint with enhanced information
	router.GET("/health", func(c *gin.Context) {
		// Test OpenAI API connectivity
		openaiStatus := "healthy"
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Test with a simple embedding call
		_, err := openaiClient.EmbedTexts(ctx, []string{"health check"})
		if err != nil {
			openaiStatus = "unhealthy"
			logger.Warn("OpenAI API health check failed", zap.Error(err))
		}

		c.JSON(http.StatusOK, gin.H{
			"status":        "healthy",
			"service":       "synthesize",
			"version":       "1.0.0",
			"environment":   os.Getenv("ENVIRONMENT"),
			"openai_status": openaiStatus,
			"config": gin.H{
				"model":       cfg.Synthesis.Model,
				"max_tokens":  cfg.Synthesis.MaxTokens,
				"temperature": cfg.Synthesis.Temperature,
			},
		})
	})

	// Synthesis endpoint
	router.POST("/synthesize", func(c *gin.Context) {
		startTime := time.Now()

		logger.Info("Synthesis request received",
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.GetHeader("User-Agent")),
		)

		// Parse request
		var req SynthesisRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			logger.Error("Failed to parse synthesis request", zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"details": err.Error(),
			})
			return
		}

		// Validate request
		if err := validateSynthesisRequest(req); err != nil {
			logger.Error("Invalid synthesis request", zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request",
				"details": err.Error(),
			})
			return
		}

		// Convert request to internal format
		contextItems := make([]synth.ContextItem, len(req.Chunks))
		for i, chunk := range req.Chunks {
			contextItems[i] = synth.ContextItem{
				Content:  chunk.Text,
				SourceID: chunk.DocID,
				Score:    1.0, // Default score
				Priority: 1,   // Default priority
			}
		}

		// Convert web results to strings
		webResultStrings := make([]string, len(req.WebResults))
		for i, webResult := range req.WebResults {
			if webResult.Title != "" && webResult.Snippet != "" {
				webResultStrings[i] = fmt.Sprintf("Title: %s\nSnippet: %s\nURL: %s", webResult.Title, webResult.Snippet, webResult.URL)
			} else if webResult.Title != "" {
				webResultStrings[i] = fmt.Sprintf("Title: %s\nURL: %s", webResult.Title, webResult.URL)
			} else {
				webResultStrings[i] = fmt.Sprintf("Snippet: %s\nURL: %s", webResult.Snippet, webResult.URL)
			}
		}

		// Build comprehensive prompt
		prompt := synth.BuildPrompt(req.Query, contextItems, webResultStrings)

		// Call OpenAI Chat Completion API
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		response, err := openaiClient.CreateChatCompletion(ctx, internalopenai.ChatCompletionRequest{
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

		if err != nil {
			logger.Error("OpenAI API call failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to generate response",
				"details": err.Error(),
			})
			return
		}

		// Parse response into structured format
		synthesisResponse := synth.ParseResponse(response.Content)

		// Log synthesis completion
		processingTime := time.Since(startTime)
		logger.Info("Synthesis completed",
			zap.String("query", req.Query),
			zap.Int("context_items", len(contextItems)),
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

		// Return structured response
		c.JSON(http.StatusOK, gin.H{
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
		})
	})

	// Start server
	port := ":8082" // Default port for synthesize service
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
