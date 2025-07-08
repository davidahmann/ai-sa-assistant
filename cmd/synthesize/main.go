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
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/your-org/ai-sa-assistant/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

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
		c.JSON(http.StatusOK, gin.H{
			"status":      "healthy",
			"service":     "synthesize",
			"version":     "1.0.0",
			"environment": os.Getenv("ENVIRONMENT"),
			"config": gin.H{
				"model":       cfg.Synthesis.Model,
				"max_tokens":  cfg.Synthesis.MaxTokens,
				"temperature": cfg.Synthesis.Temperature,
			},
		})
	})

	// Synthesis endpoint
	router.POST("/synthesize", func(c *gin.Context) {
		// TODO: Implement synthesis logic
		// 1. Parse request (query, context chunks, web results)
		// 2. Build comprehensive prompt
		// 3. Call OpenAI Chat Completion API
		// 4. Parse response (main_text, diagram_code, code_snippets)
		// 5. Extract sources
		// 6. Return structured response

		logger.Info("Synthesis request received",
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.GetHeader("User-Agent")),
		)

		c.JSON(http.StatusOK, gin.H{
			"message":     "Synthesis endpoint not yet implemented",
			"model":       cfg.Synthesis.Model,
			"max_tokens":  cfg.Synthesis.MaxTokens,
			"temperature": cfg.Synthesis.Temperature,
			"config": gin.H{
				"model":       cfg.Synthesis.Model,
				"max_tokens":  cfg.Synthesis.MaxTokens,
				"temperature": cfg.Synthesis.Temperature,
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
