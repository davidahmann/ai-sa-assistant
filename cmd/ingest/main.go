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

// Package main provides the document ingestion CLI tool for the AI SA Assistant.
// It processes documents, generates embeddings, and loads them into ChromaDB for retrieval.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/your-org/ai-sa-assistant/internal/chroma"
	"github.com/your-org/ai-sa-assistant/internal/chunker"
	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/metadata"
	"github.com/your-org/ai-sa-assistant/internal/openai"
)

const (
	defaultChunkSize      = 500
	defaultChunkSizeWords = 500
	maxConcurrentChunks   = 10
)

// IngestionPipeline represents the ingestion pipeline configuration
type IngestionPipeline struct {
	openaiClient  *openai.Client
	chromaClient  *chroma.Client
	metadataStore *metadata.Store
	logger        *zap.Logger
	chunkSize     int
}

// IngestionStats represents statistics from the ingestion process
type IngestionStats struct {
	ProcessedCount int
	SuccessCount   int
	FailureCount   int
	TotalChunks    int
	SkippedCount   int
}

var (
	docsPath     string
	configPath   string
	chunkSize    int
	forceReindex bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "ingest",
		Short: "AI SA Assistant Document Ingestion Tool",
		Long: `A command-line tool that orchestrates the complete document processing pipeline:
- Parsing and chunking documents
- Generating embeddings via OpenAI API
- Storing embeddings and metadata in ChromaDB
- Initializing SQLite metadata database

This tool is essential for populating the knowledge base with synthetic documents.`,
		RunE: runIngestionCommand,
	}

	rootCmd.Flags().StringVarP(&docsPath, "docs-path", "d", "./docs", "Path to documents directory")
	rootCmd.Flags().StringVarP(&configPath, "config", "c", "./configs/config.yaml", "Path to configuration file")
	rootCmd.Flags().IntVarP(&chunkSize, "chunk-size", "s", defaultChunkSize, "Chunk size in words")
	rootCmd.Flags().BoolVarP(&forceReindex, "force-reindex", "f", false, "Force re-indexing of all documents")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runIngestionCommand(_ *cobra.Command, _ []string) error {
	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()

	// Check if running in test mode
	testMode := os.Getenv("TEST_MODE") == "true" || os.Getenv("CI") == "true"

	var cfg *config.Config
	var err error

	if testMode {
		cfg, err = config.LoadWithOptions(config.LoadOptions{
			ConfigPath:       configPath,
			EnableHotReload:  false,
			Environment:      "test",
			ValidateRequired: false,
			TestMode:         true,
		})
	} else {
		cfg, err = config.Load(configPath)
	}
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	logger.Info("Starting ingestion service",
		zap.String("docs_path", docsPath),
		zap.String("chroma_url", cfg.Chroma.URL),
		zap.String("collection_name", cfg.Chroma.CollectionName),
		zap.Int("chunk_size", chunkSize),
		zap.Bool("force_reindex", forceReindex))

	stats, err := runIngestionPipeline(cfg, docsPath, chunkSize, forceReindex, logger)
	if err != nil {
		logger.Fatal("Ingestion pipeline failed", zap.Error(err))
	}

	logger.Info("Ingestion completed successfully",
		zap.Int("total_processed", stats.ProcessedCount),
		zap.Int("successful", stats.SuccessCount),
		zap.Int("failed", stats.FailureCount),
		zap.Int("skipped", stats.SkippedCount),
		zap.Int("total_chunks", stats.TotalChunks))

	return nil
}

func runIngestionPipeline(
	cfg *config.Config,
	docsPath string,
	chunkSize int,
	_ bool,
	logger *zap.Logger,
) (*IngestionStats, error) {
	ctx := context.Background()

	// Initialize OpenAI client
	openaiClient, err := openai.NewClient(cfg.OpenAI.APIKey, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize OpenAI client: %w", err)
	}

	// Initialize ChromaDB client
	chromaClient := chroma.NewClient(cfg.Chroma.URL, cfg.Chroma.CollectionName)

	// Health check ChromaDB
	if err := chromaClient.HealthCheck(ctx); err != nil {
		return nil, fmt.Errorf("ChromaDB health check failed: %w", err)
	}

	// Create collection if it doesn't exist
	if err := chromaClient.CreateCollection(ctx, cfg.Chroma.CollectionName, map[string]interface{}{
		"description": "AI SA Assistant document embeddings",
		"created_at":  time.Now().Format(time.RFC3339),
	}); err != nil {
		logger.Warn("Failed to create collection (may already exist)", zap.Error(err))
	}

	// Initialize metadata store
	metadataStore, err := metadata.NewStore(cfg.Metadata.DBPath, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize metadata store: %w", err)
	}
	defer func() {
		if err := metadataStore.Close(); err != nil {
			logger.Warn("Failed to close metadata store", zap.Error(err))
		}
	}()

	// Load metadata from JSON file
	metadataPath := filepath.Join(docsPath, "metadata.json")
	if err := metadataStore.LoadFromJSON(metadataPath); err != nil {
		return nil, fmt.Errorf("failed to load metadata from JSON: %w", err)
	}

	// Create pipeline
	pipeline := &IngestionPipeline{
		openaiClient:  openaiClient,
		chromaClient:  chromaClient,
		metadataStore: metadataStore,
		logger:        logger,
		chunkSize:     chunkSize,
	}

	// Get all metadata entries
	allMetadata, err := metadataStore.GetAllMetadata()
	if err != nil {
		return nil, fmt.Errorf("failed to get all metadata: %w", err)
	}

	// Process documents with improved error handling
	stats := &IngestionStats{}

	for _, entry := range allMetadata {
		// Skip external documents (they don't have local files)
		if entry.Path == "external" {
			logger.Debug("Skipping external document", zap.String("doc_id", entry.DocID))
			stats.SkippedCount++
			continue
		}

		// Check if document file exists
		fullPath := filepath.Join(docsPath, "..", entry.Path)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			logger.Warn("Document file not found", zap.String("doc_id", entry.DocID), zap.String("path", fullPath))
			stats.SkippedCount++
			continue
		}

		logger.Info("Processing document", zap.String("doc_id", entry.DocID), zap.String("title", entry.Title))
		stats.ProcessedCount++

		chunks, err := pipeline.processDocument(ctx, entry, fullPath)
		if err != nil {
			logger.Error("Failed to process document", zap.String("doc_id", entry.DocID), zap.Error(err))
			stats.FailureCount++
			// Continue processing other documents instead of failing completely
			continue
		}

		stats.TotalChunks += chunks
		stats.SuccessCount++

		logger.Info("Document processed successfully",
			zap.String("doc_id", entry.DocID),
			zap.Int("chunks_created", chunks))
	}

	// Print summary
	logger.Info("Ingestion pipeline completed",
		zap.Int("documents_processed", stats.ProcessedCount),
		zap.Int("successful", stats.SuccessCount),
		zap.Int("failed", stats.FailureCount),
		zap.Int("skipped", stats.SkippedCount),
		zap.Int("total_chunks", stats.TotalChunks))

	// Get metadata store statistics
	dbStats, err := metadataStore.GetStats()
	if err != nil {
		logger.Warn("Failed to get metadata statistics", zap.Error(err))
	} else {
		logger.Info("Metadata store statistics", zap.Any("stats", dbStats))
	}

	// Return error if no documents were successfully processed
	if stats.SuccessCount == 0 && stats.ProcessedCount > 0 {
		return stats, fmt.Errorf("no documents were successfully processed")
	}

	return stats, nil
}

// validateFilePath ensures the file path is safe and within expected bounds
func validateFilePath(basePath, filePath string) error {
	// Clean and resolve the path
	cleanPath := filepath.Clean(filePath)
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// Ensure base path is absolute
	absBasePath, err := filepath.Abs(basePath)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute base path: %w", err)
	}

	// Check if the file path is within the base directory
	relPath, err := filepath.Rel(absBasePath, absPath)
	if err != nil {
		return fmt.Errorf("failed to compute relative path: %w", err)
	}

	// Prevent directory traversal attacks
	if strings.HasPrefix(relPath, "..") || strings.Contains(relPath, "/../") {
		return fmt.Errorf("invalid file path: directory traversal detected")
	}

	return nil
}

func (p *IngestionPipeline) processDocument(
	ctx context.Context,
	entry metadata.Entry,
	filePath string,
) (int, error) {
	// Validate file path for security
	if err := validateFilePath(".", filePath); err != nil {
		return 0, fmt.Errorf("invalid file path %s: %w", filePath, err)
	}

	// Read document content
	content, err := os.ReadFile(filePath) // #nosec G304 - path validated above
	if err != nil {
		return 0, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// Parse markdown content
	cleanContent := chunker.ParseMarkdown(string(content))

	// Split into chunks
	chunks := chunker.Splitter(cleanContent, p.chunkSize)

	if len(chunks) == 0 {
		p.logger.Warn("No chunks created for document", zap.String("doc_id", entry.DocID))
		return 0, nil
	}

	p.logger.Debug("Document chunked",
		zap.String("doc_id", entry.DocID),
		zap.Int("chunk_count", len(chunks)),
		zap.Int("original_length", len(cleanContent)))

	// Generate embeddings for all chunks
	embeddings, err := p.generateEmbeddings(ctx, chunks)
	if err != nil {
		return 0, fmt.Errorf("failed to generate embeddings: %w", err)
	}

	// Prepare documents for ChromaDB
	documents := make([]chroma.Document, len(chunks))
	for i, chunk := range chunks {
		documents[i] = chroma.Document{
			ID:      fmt.Sprintf("%s_chunk_%d", entry.DocID, i),
			Content: chunk,
			Metadata: map[string]string{
				"doc_id":         entry.DocID,
				"title":          entry.Title,
				"platform":       entry.Platform,
				"scenario":       entry.Scenario,
				"type":           entry.Type,
				"source_url":     entry.SourceURL,
				"path":           entry.Path,
				"difficulty":     entry.Difficulty,
				"estimated_time": entry.EstimatedTime,
				"chunk_index":    fmt.Sprintf("%d", i),
				"chunk_count":    fmt.Sprintf("%d", len(chunks)),
				"tags":           strings.Join(entry.Tags, ","),
			},
		}
	}

	// Store in ChromaDB
	if err := p.chromaClient.AddDocuments(ctx, documents, embeddings); err != nil {
		return 0, fmt.Errorf("failed to store documents in ChromaDB: %w", err)
	}

	return len(chunks), nil
}

func (p *IngestionPipeline) generateEmbeddings(ctx context.Context, chunks []string) ([][]float32, error) {
	p.logger.Debug("Generating embeddings", zap.Int("chunk_count", len(chunks)))

	response, err := p.openaiClient.EmbedTexts(ctx, chunks)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embeddings: %w", err)
	}

	p.logger.Debug("Embeddings generated successfully",
		zap.Int("embeddings_count", len(response.Embeddings)),
		zap.Int("tokens_used", response.Usage.TokensUsed),
		zap.Float64("estimated_cost", response.Usage.EstimatedCost),
		zap.Duration("processing_time", response.Usage.ProcessingTime))

	return response.Embeddings, nil
}
