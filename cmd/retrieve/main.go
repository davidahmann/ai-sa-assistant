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
		zap.String("service", "retrieve"),
		zap.String("environment", os.Getenv("ENVIRONMENT")),
		zap.String("chroma_url", maskedConfig.Chroma.URL),
		zap.String("collection_name", maskedConfig.Chroma.CollectionName),
		zap.String("metadata_db_path", maskedConfig.Metadata.DBPath),
		zap.Int("max_chunks", maskedConfig.Retrieval.MaxChunks),
		zap.Float64("confidence_threshold", maskedConfig.Retrieval.ConfidenceThreshold),
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
			"service":     "retrieve",
			"version":     "1.0.0",
			"environment": os.Getenv("ENVIRONMENT"),
			"config": gin.H{
				"chroma_url":           cfg.Chroma.URL,
				"collection_name":      cfg.Chroma.CollectionName,
				"max_chunks":           cfg.Retrieval.MaxChunks,
				"fallback_threshold":   cfg.Retrieval.FallbackThreshold,
				"confidence_threshold": cfg.Retrieval.ConfidenceThreshold,
			},
		})
	})

	// Main search endpoint
	router.POST("/search", func(c *gin.Context) {
		// TODO: Implement search logic
		// 1. Parse request (query, filters)
		// 2. Query metadata store if filters present
		// 3. Embed query using OpenAI
		// 4. Search ChromaDB
		// 5. Apply fallback logic if needed
		// 6. Return results

		logger.Info("Search request received",
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.GetHeader("User-Agent")),
		)

		c.JSON(http.StatusOK, gin.H{
			"message": "Search endpoint not yet implemented",
			"query":   c.Request.Header.Get("query"),
			"config": gin.H{
				"max_chunks":           cfg.Retrieval.MaxChunks,
				"fallback_threshold":   cfg.Retrieval.FallbackThreshold,
				"confidence_threshold": cfg.Retrieval.ConfidenceThreshold,
			},
		})
	})

	// Start server
	port := ":8081" // Default port for retrieve service
	logger.Info("Starting retrieve service",
		zap.String("port", port),
		zap.String("chroma_url", cfg.Chroma.URL),
		zap.String("collection_name", cfg.Chroma.CollectionName),
		zap.String("metadata_db_path", cfg.Metadata.DBPath),
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
		zapConfig.OutputPaths = []string{"retrieve.log"}
		zapConfig.ErrorOutputPaths = []string{"retrieve.log"}
	} else {
		zapConfig.OutputPaths = []string{"stdout"}
		zapConfig.ErrorOutputPaths = []string{"stderr"}
	}

	return zapConfig.Build()
}
