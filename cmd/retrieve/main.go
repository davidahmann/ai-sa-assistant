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
	"time"

	"github.com/gin-gonic/gin"
	"github.com/your-org/ai-sa-assistant/internal/chroma"
	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/metadata"
	"github.com/your-org/ai-sa-assistant/internal/openai"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// SearchRequest represents the JSON payload for search requests
type SearchRequest struct {
	Query   string                 `json:"query" binding:"required"`
	Filters map[string]interface{} `json:"filters,omitempty"`
}

// SearchChunk represents a single search result chunk
type SearchChunk struct {
	Text     string                 `json:"text"`
	Score    float64                `json:"score"`
	DocID    string                 `json:"doc_id"`
	Metadata map[string]interface{} `json:"metadata"`
}

// SearchResponse represents the JSON response for search requests
type SearchResponse struct {
	Chunks            []SearchChunk `json:"chunks"`
	Count             int           `json:"count"`
	Query             string        `json:"query"`
	FallbackTriggered bool          `json:"fallback_triggered"`
	FallbackReason    string        `json:"fallback_reason,omitempty"`
}

// ServiceDependencies holds initialized service dependencies
type ServiceDependencies struct {
	MetadataStore *metadata.Store
	ChromaClient  *chroma.Client
	OpenAIClient  *openai.Client
	Logger        *zap.Logger
	Config        *config.Config
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

	// Initialize service dependencies
	deps, err := initializeDependencies(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to initialize dependencies", zap.Error(err))
	}
	defer deps.MetadataStore.Close()

	// Set Gin mode based on log level
	if cfg.Logging.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.Default()

	// Health check endpoint with dependency health checks
	router.GET("/health", createHealthHandler(deps))

	// Main search endpoint
	router.POST("/search", createSearchHandler(deps))

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

// initializeDependencies initializes all service dependencies
func initializeDependencies(cfg *config.Config, logger *zap.Logger) (*ServiceDependencies, error) {
	logger.Info("Initializing service dependencies")

	// Initialize metadata store
	metadataStore, err := metadata.NewStore(cfg.Metadata.DBPath, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize metadata store: %w", err)
	}

	// Initialize ChromaDB client
	chromaClient := chroma.NewClientWithOptions(
		cfg.Chroma.URL,
		cfg.Chroma.CollectionName,
		logger,
		3,
		time.Second,
	)

	// Initialize OpenAI client
	openaiClient, err := openai.NewClient(cfg.OpenAI.APIKey, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize OpenAI client: %w", err)
	}

	logger.Info("Service dependencies initialized successfully")

	return &ServiceDependencies{
		MetadataStore: metadataStore,
		ChromaClient:  chromaClient,
		OpenAIClient:  openaiClient,
		Logger:        logger,
		Config:        cfg,
	}, nil
}

// createHealthHandler creates the health check endpoint handler
func createHealthHandler(deps *ServiceDependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		status := "healthy"
		statusCode := http.StatusOK
		dependencies := make(map[string]interface{})

		// Check ChromaDB health
		if err := deps.ChromaClient.HealthCheck(); err != nil {
			deps.Logger.Error("ChromaDB health check failed", zap.Error(err))
			dependencies["chroma"] = map[string]interface{}{
				"status": "unhealthy",
				"error":  err.Error(),
			}
			status = "unhealthy"
			statusCode = http.StatusServiceUnavailable
		} else {
			dependencies["chroma"] = map[string]interface{}{
				"status": "healthy",
				"url":    deps.Config.Chroma.URL,
			}
		}

		// Check OpenAI connectivity
		if _, err := deps.OpenAIClient.EmbedQuery(ctx, "health check"); err != nil {
			deps.Logger.Error("OpenAI health check failed", zap.Error(err))
			dependencies["openai"] = map[string]interface{}{
				"status": "unhealthy",
				"error":  err.Error(),
			}
			status = "unhealthy"
			statusCode = http.StatusServiceUnavailable
		} else {
			dependencies["openai"] = map[string]interface{}{
				"status": "healthy",
			}
		}

		// Check metadata store
		if _, err := deps.MetadataStore.GetStats(); err != nil {
			deps.Logger.Error("Metadata store health check failed", zap.Error(err))
			dependencies["metadata"] = map[string]interface{}{
				"status": "unhealthy",
				"error":  err.Error(),
			}
			status = "unhealthy"
			statusCode = http.StatusServiceUnavailable
		} else {
			dependencies["metadata"] = map[string]interface{}{
				"status":  "healthy",
				"db_path": deps.Config.Metadata.DBPath,
			}
		}

		c.JSON(statusCode, gin.H{
			"status":       status,
			"service":      "retrieve",
			"version":      "1.0.0",
			"environment":  os.Getenv("ENVIRONMENT"),
			"dependencies": dependencies,
			"config": gin.H{
				"chroma_url":               deps.Config.Chroma.URL,
				"collection_name":          deps.Config.Chroma.CollectionName,
				"max_chunks":               deps.Config.Retrieval.MaxChunks,
				"fallback_threshold":       deps.Config.Retrieval.FallbackThreshold,
				"confidence_threshold":     deps.Config.Retrieval.ConfidenceThreshold,
				"fallback_score_threshold": deps.Config.Retrieval.FallbackScoreThreshold,
			},
		})
	}
}

// createSearchHandler creates the main search endpoint handler
func createSearchHandler(deps *ServiceDependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		deps.Logger.Info("Search request received",
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.GetHeader("User-Agent")),
		)

		// Parse and validate request
		var searchReq SearchRequest
		if err := c.ShouldBindJSON(&searchReq); err != nil {
			deps.Logger.Error("Invalid search request", zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid request format: " + err.Error(),
			})
			return
		}

		if searchReq.Query == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Query parameter is required",
			})
			return
		}

		deps.Logger.Info("Processing search request",
			zap.String("query", searchReq.Query),
			zap.Any("filters", searchReq.Filters),
		)

		// Step 1: Apply metadata filters if present
		var filteredDocIDs []string
		if searchReq.Filters != nil && len(searchReq.Filters) > 0 {
			filterOpts := metadata.FilterOptions{
				AndFilters: true,
			}

			if platform, ok := searchReq.Filters["platform"].(string); ok {
				filterOpts.Platform = platform
			}
			if scenario, ok := searchReq.Filters["scenario"].(string); ok {
				filterOpts.Scenario = scenario
			}
			if docType, ok := searchReq.Filters["type"].(string); ok {
				filterOpts.Type = docType
			}

			var err error
			filteredDocIDs, err = deps.MetadataStore.FilterDocuments(filterOpts)
			if err != nil {
				deps.Logger.Error("Failed to filter documents", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "Failed to filter documents",
				})
				return
			}

			deps.Logger.Info("Applied metadata filters",
				zap.Any("filters", filterOpts),
				zap.Int("filtered_doc_count", len(filteredDocIDs)),
			)
		}

		// Step 2: Generate query embedding
		queryEmbedding, err := deps.OpenAIClient.EmbedQuery(ctx, searchReq.Query)
		if err != nil {
			deps.Logger.Error("Failed to generate query embedding", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to generate query embedding",
			})
			return
		}

		// Step 3: Perform vector search
		maxChunks := deps.Config.Retrieval.MaxChunks
		searchResults, err := deps.ChromaClient.Search(queryEmbedding, maxChunks, filteredDocIDs)
		if err != nil {
			deps.Logger.Error("ChromaDB search failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Vector search failed",
			})
			return
		}

		// Step 4: Apply intelligent fallback logic
		fallbackTriggered := false
		fallbackReason := ""

		if len(filteredDocIDs) > 0 {
			// Calculate average similarity score for quality assessment
			var totalScore float64
			for _, result := range searchResults {
				similarity := 1.0 - result.Distance
				totalScore += similarity
			}

			var avgScore float64
			if len(searchResults) > 0 {
				avgScore = totalScore / float64(len(searchResults))
			}

			// Check if fallback is needed based on count or average score
			needsFallback := false
			if len(searchResults) < deps.Config.Retrieval.FallbackThreshold {
				fallbackReason = fmt.Sprintf("insufficient results (%d < %d)", len(searchResults), deps.Config.Retrieval.FallbackThreshold)
				needsFallback = true
			} else if avgScore < deps.Config.Retrieval.FallbackScoreThreshold {
				fallbackReason = fmt.Sprintf("low average similarity score (%.3f < %.3f)", avgScore, deps.Config.Retrieval.FallbackScoreThreshold)
				needsFallback = true
			}

			if needsFallback {
				deps.Logger.Info("Applying fallback search without document ID filter",
					zap.Int("initial_results", len(searchResults)),
					zap.Float64("avg_score", avgScore),
					zap.String("reason", fallbackReason),
					zap.Int("fallback_threshold", deps.Config.Retrieval.FallbackThreshold),
					zap.Float64("fallback_score_threshold", deps.Config.Retrieval.FallbackScoreThreshold),
				)

				fallbackResults, err := deps.ChromaClient.Search(queryEmbedding, maxChunks, nil)
				if err != nil {
					deps.Logger.Error("Fallback search failed", zap.Error(err))
				} else {
					searchResults = fallbackResults
					fallbackTriggered = true
					deps.Logger.Info("Fallback search completed",
						zap.Int("fallback_results", len(searchResults)),
						zap.String("reason", fallbackReason),
					)
				}
			}
		}

		// Step 5: Filter results by confidence threshold
		confidenceThreshold := deps.Config.Retrieval.ConfidenceThreshold
		var filteredResults []chroma.SearchResult
		for _, result := range searchResults {
			// Convert distance to similarity score (lower distance = higher similarity)
			similarity := 1.0 - result.Distance
			if similarity >= confidenceThreshold {
				filteredResults = append(filteredResults, result)
			}
		}

		// Step 6: Format response
		chunks := make([]SearchChunk, len(filteredResults))
		for i, result := range filteredResults {
			// Convert metadata to interface{} map
			metadataMap := make(map[string]interface{})
			for k, v := range result.Metadata {
				metadataMap[k] = v
			}

			chunks[i] = SearchChunk{
				Text:     result.Content,
				Score:    1.0 - result.Distance, // Convert distance to similarity score
				DocID:    result.ID,
				Metadata: metadataMap,
			}
		}

		processingTime := time.Since(start)
		deps.Logger.Info("Search completed successfully",
			zap.String("query", searchReq.Query),
			zap.Int("total_results", len(searchResults)),
			zap.Int("filtered_results", len(filteredResults)),
			zap.Float64("confidence_threshold", confidenceThreshold),
			zap.Bool("fallback_triggered", fallbackTriggered),
			zap.String("fallback_reason", fallbackReason),
			zap.Duration("processing_time", processingTime),
		)

		response := SearchResponse{
			Chunks:            chunks,
			Count:             len(chunks),
			Query:             searchReq.Query,
			FallbackTriggered: fallbackTriggered,
			FallbackReason:    fallbackReason,
		}

		c.JSON(http.StatusOK, response)
	}
}
