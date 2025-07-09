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

// Package main provides the retrieval service API for the AI SA Assistant.
// It handles hybrid search combining metadata filtering and vector search.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/your-org/ai-sa-assistant/internal/chroma"
	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/health"
	"github.com/your-org/ai-sa-assistant/internal/metadata"
	"github.com/your-org/ai-sa-assistant/internal/openai"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	// DefaultRetryAttempts defines the default number of retry attempts
	DefaultRetryAttempts = 3
	// HealthCheckTimeout defines the timeout for health checks
	HealthCheckTimeout = 5 * time.Second
	// SearchRequestTimeout defines the timeout for search requests
	SearchRequestTimeout = 30 * time.Second
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
	SourceID string                 `json:"source_id"`
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
	defer func() {
		if err := deps.MetadataStore.Close(); err != nil {
			logger.Warn("Failed to close metadata store", zap.Error(err))
		}
	}()

	// Set Gin mode based on log level
	if cfg.Logging.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.Default()

	// Initialize health check manager
	healthManager := health.NewManager("retrieve", "1.0.0", logger)
	setupHealthChecks(healthManager, deps)

	// Health check endpoint with dependency health checks
	router.GET("/health", gin.WrapH(healthManager.HTTPHandler()))

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
		DefaultRetryAttempts,
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

// setupHealthChecks configures health checks for the retrieve service
func setupHealthChecks(manager *health.Manager, deps *ServiceDependencies) {
	// ChromaDB health check
	manager.AddCheckerFunc("chroma", func(ctx context.Context) health.CheckResult {
		if err := deps.ChromaClient.HealthCheck(); err != nil {
			return health.CheckResult{
				Status:    health.StatusUnhealthy,
				Error:     fmt.Sprintf("ChromaDB health check failed: %v", err),
				Timestamp: time.Now(),
			}
		}
		return health.CheckResult{
			Status:    health.StatusHealthy,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"url":        deps.Config.Chroma.URL,
				"collection": deps.Config.Chroma.CollectionName,
			},
		}
	})

	// OpenAI health check
	manager.AddCheckerFunc("openai", func(ctx context.Context) health.CheckResult {
		if _, err := deps.OpenAIClient.EmbedQuery(ctx, "health check"); err != nil {
			return health.CheckResult{
				Status:    health.StatusUnhealthy,
				Error:     fmt.Sprintf("OpenAI health check failed: %v", err),
				Timestamp: time.Now(),
			}
		}
		return health.CheckResult{
			Status:    health.StatusHealthy,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"endpoint": deps.Config.OpenAI.Endpoint,
			},
		}
	})

	// Metadata store health check
	manager.AddCheckerFunc("metadata", func(ctx context.Context) health.CheckResult {
		if _, err := deps.MetadataStore.GetStats(); err != nil {
			return health.CheckResult{
				Status:    health.StatusUnhealthy,
				Error:     fmt.Sprintf("Metadata store health check failed: %v", err),
				Timestamp: time.Now(),
			}
		}
		return health.CheckResult{
			Status:    health.StatusHealthy,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"db_path": deps.Config.Metadata.DBPath,
			},
		}
	})

	// Set timeout for health checks
	manager.SetTimeout(HealthCheckTimeout)
}

// validateSearchRequest validates and parses the incoming search request
func validateSearchRequest(c *gin.Context, logger *zap.Logger) (SearchRequest, bool) {
	var searchReq SearchRequest
	if err := c.ShouldBindJSON(&searchReq); err != nil {
		logger.Error("Invalid search request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format: " + err.Error(),
		})
		return searchReq, false
	}

	if searchReq.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Query parameter is required",
		})
		return searchReq, false
	}

	logger.Info("Processing search request",
		zap.String("query", searchReq.Query),
		zap.Any("filters", searchReq.Filters),
	)

	return searchReq, true
}

// applyMetadataFilters applies metadata filters if present in the request
func applyMetadataFilters(searchReq SearchRequest, deps *ServiceDependencies) ([]string, error) {
	if len(searchReq.Filters) == 0 {
		return nil, nil
	}

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

	filteredDocIDs, err := deps.MetadataStore.FilterDocuments(filterOpts)
	if err != nil {
		return nil, err
	}

	deps.Logger.Info("Applied metadata filters",
		zap.Any("filters", filterOpts),
		zap.Int("filtered_doc_count", len(filteredDocIDs)),
	)

	return filteredDocIDs, nil
}

// generateQueryEmbedding generates an embedding for the search query
func generateQueryEmbedding(ctx context.Context, query string, deps *ServiceDependencies) ([]float32, error) {
	return deps.OpenAIClient.EmbedQuery(ctx, query)
}

// performVectorSearchWithFallback performs vector search with intelligent fallback logic
func performVectorSearchWithFallback(
	queryEmbedding []float32,
	filteredDocIDs []string,
	deps *ServiceDependencies,
) ([]chroma.SearchResult, bool, string, error) {
	maxChunks := deps.Config.Retrieval.MaxChunks
	searchResults, err := deps.ChromaClient.Search(queryEmbedding, maxChunks, filteredDocIDs)
	if err != nil {
		return nil, false, "", err
	}

	// Apply fallback logic only if we have filtered doc IDs
	if len(filteredDocIDs) == 0 {
		return searchResults, false, "", nil
	}

	fallbackTriggered, fallbackReason := shouldApplyFallback(searchResults, deps.Config.Retrieval)
	if !fallbackTriggered {
		return searchResults, false, "", nil
	}

	// Perform fallback search
	fallbackResults, err := performFallbackSearch(queryEmbedding, maxChunks, fallbackReason, deps)
	if err != nil {
		return searchResults, false, "", nil // Return original results if fallback fails
	}

	return fallbackResults, true, fallbackReason, nil
}

// shouldApplyFallback determines if fallback search is needed
func shouldApplyFallback(searchResults []chroma.SearchResult, config config.RetrievalConfig) (bool, string) {
	if len(searchResults) < config.FallbackThreshold {
		return true, fmt.Sprintf("insufficient results (%d < %d)", len(searchResults), config.FallbackThreshold)
	}

	// Calculate average similarity score
	var totalScore float64
	for _, result := range searchResults {
		similarity := 1.0 - result.Distance
		totalScore += similarity
	}

	if len(searchResults) == 0 {
		return false, ""
	}

	avgScore := totalScore / float64(len(searchResults))
	if avgScore < config.FallbackScoreThreshold {
		return true, fmt.Sprintf("low average similarity score (%.3f < %.3f)", avgScore, config.FallbackScoreThreshold)
	}

	return false, ""
}

// performFallbackSearch performs the fallback search without document ID filter
func performFallbackSearch(
	queryEmbedding []float32,
	maxChunks int,
	reason string,
	deps *ServiceDependencies,
) ([]chroma.SearchResult, error) {
	deps.Logger.Info("Applying fallback search without document ID filter",
		zap.String("reason", reason),
		zap.Int("fallback_threshold", deps.Config.Retrieval.FallbackThreshold),
		zap.Float64("fallback_score_threshold", deps.Config.Retrieval.FallbackScoreThreshold),
	)

	fallbackResults, err := deps.ChromaClient.Search(queryEmbedding, maxChunks, nil)
	if err != nil {
		deps.Logger.Error("Fallback search failed", zap.Error(err))
		return nil, err
	}

	deps.Logger.Info("Fallback search completed",
		zap.Int("fallback_results", len(fallbackResults)),
		zap.String("reason", reason),
	)

	return fallbackResults, nil
}

// buildSearchResponse filters results by confidence and builds the final response
func buildSearchResponse(
	searchResults []chroma.SearchResult,
	query string,
	fallbackTriggered bool,
	fallbackReason string,
	deps *ServiceDependencies,
) SearchResponse {
	confidenceThreshold := deps.Config.Retrieval.ConfidenceThreshold
	var filteredResults []chroma.SearchResult
	for _, result := range searchResults {
		similarity := 1.0 - result.Distance
		if similarity >= confidenceThreshold {
			filteredResults = append(filteredResults, result)
		}
	}

	chunks := make([]SearchChunk, len(filteredResults))
	for i, result := range filteredResults {
		metadataMap := make(map[string]interface{})
		for k, v := range result.Metadata {
			metadataMap[k] = v
		}

		// Extract document ID from chunk ID to lookup source metadata
		docID := extractDocIDFromChunkID(result.ID)
		sourceID := getSourceIDForDocument(docID, deps)

		chunks[i] = SearchChunk{
			Text:     result.Content,
			Score:    1.0 - result.Distance,
			DocID:    result.ID,
			SourceID: sourceID,
			Metadata: metadataMap,
		}
	}

	return SearchResponse{
		Chunks:            chunks,
		Count:             len(chunks),
		Query:             query,
		FallbackTriggered: fallbackTriggered,
		FallbackReason:    fallbackReason,
	}
}

// extractDocIDFromChunkID extracts the document ID from a chunk ID
// Chunk IDs are typically in the format "doc_id_chunk_N" where N is the chunk number
func extractDocIDFromChunkID(chunkID string) string {
	// Split by underscores and remove the last two parts ("chunk" and number)
	parts := strings.Split(chunkID, "_")
	const minParts = 3
	if len(parts) >= minParts {
		// Find the last occurrence of "chunk" to handle doc IDs that contain underscores
		const chunkOffset = 2
		for i := len(parts) - chunkOffset; i >= 0; i-- {
			if parts[i] == "chunk" {
				return strings.Join(parts[:i], "_")
			}
		}
	}
	// If no "chunk" pattern found, assume the whole ID is the document ID
	return chunkID
}

// getSourceIDForDocument retrieves the source ID (URL or title) for a document
func getSourceIDForDocument(docID string, deps *ServiceDependencies) string {
	// Look up document metadata to get source information
	metadataEntry, err := deps.MetadataStore.GetMetadataByDocID(docID)
	if err != nil {
		deps.Logger.Warn("Failed to get metadata for document",
			zap.String("doc_id", docID),
			zap.Error(err))
		// Fall back to doc ID if metadata lookup fails
		return docID
	}

	if metadataEntry == nil {
		deps.Logger.Debug("No metadata found for document", zap.String("doc_id", docID))
		// Fall back to doc ID if no metadata entry found
		return docID
	}

	// Prefer source URL if available, otherwise use title, otherwise fall back to doc ID
	if metadataEntry.SourceURL != "" {
		return metadataEntry.SourceURL
	}
	if metadataEntry.Title != "" {
		return metadataEntry.Title
	}
	return docID
}

// createSearchHandler creates the main search endpoint handler
func createSearchHandler(deps *ServiceDependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), SearchRequestTimeout)
		defer cancel()

		deps.Logger.Info("Search request received",
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.GetHeader("User-Agent")),
		)

		// Step 1: Validate request
		searchReq, valid := validateSearchRequest(c, deps.Logger)
		if !valid {
			return
		}

		// Step 2: Apply metadata filters
		filteredDocIDs, err := applyMetadataFilters(searchReq, deps)
		if err != nil {
			deps.Logger.Error("Failed to filter documents", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to filter documents",
			})
			return
		}

		// Step 3: Generate query embedding
		queryEmbedding, err := generateQueryEmbedding(ctx, searchReq.Query, deps)
		if err != nil {
			deps.Logger.Error("Failed to generate query embedding", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to generate query embedding",
			})
			return
		}

		// Step 4: Perform vector search with fallback
		searchResults, fallbackTriggered, fallbackReason, err := performVectorSearchWithFallback(
			queryEmbedding, filteredDocIDs, deps)
		if err != nil {
			deps.Logger.Error("Vector search failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Vector search failed",
			})
			return
		}

		// Step 5: Filter results by confidence and format response
		response := buildSearchResponse(searchResults, searchReq.Query, fallbackTriggered, fallbackReason, deps)

		processingTime := time.Since(start)
		deps.Logger.Info("Search completed successfully",
			zap.String("query", searchReq.Query),
			zap.Int("total_results", len(searchResults)),
			zap.Int("filtered_results", response.Count),
			zap.Float64("confidence_threshold", deps.Config.Retrieval.ConfidenceThreshold),
			zap.Bool("fallback_triggered", fallbackTriggered),
			zap.String("fallback_reason", fallbackReason),
			zap.Duration("processing_time", processingTime),
		)

		c.JSON(http.StatusOK, response)
	}
}
