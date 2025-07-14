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

package performance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/your-org/ai-sa-assistant/internal/chroma"
	"github.com/your-org/ai-sa-assistant/internal/chunker"
	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/metadata"
	"github.com/your-org/ai-sa-assistant/internal/openai"
)

const (
	maxMemoryUsageBytes = 2 * 1024 * 1024 * 1024 // 2GB limit
	maxProcessingTime   = 10 * time.Minute       // 10 minutes for large documents
	testCollectionName  = "performance_test_collection"
)

// TestLargeDocumentProcessing tests document processing with large files (100MB+)
func TestLargeDocumentProcessing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large document test in short mode")
	}

	if !servicesReady(t) {
		t.Skip("Services not available for large document testing")
	}

	// Create test configuration
	cfg := createTestConfig(t)
	logger := createTestLogger(t)

	// Create a large test document (100MB+)
	testFile := createLargeTestDocument(t, 100*1024*1024) // 100MB
	defer os.Remove(testFile)

	// Track memory usage
	var memBefore, memAfter runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	start := time.Now()

	// Process the large document
	stats, err := processLargeDocument(t, cfg, testFile, logger)

	processingTime := time.Since(start)
	runtime.ReadMemStats(&memAfter)

	// Assert processing completed successfully
	require.NoError(t, err, "Large document processing should not fail")
	assert.Greater(t, stats.ChunksCreated, 0, "Should create chunks from large document")
	assert.Less(t, processingTime, maxProcessingTime, "Processing should complete within time limit")

	// Memory usage checks
	memoryUsed := memAfter.Alloc - memBefore.Alloc
	t.Logf("Memory usage: %d bytes (%.2f MB)", memoryUsed, float64(memoryUsed)/1024/1024)
	assert.Less(t, memoryUsed, uint64(maxMemoryUsageBytes), "Memory usage should be within limits")

	// Performance logging
	t.Logf("Large document processing results:")
	t.Logf("  Document size: %.2f MB", float64(100*1024*1024)/1024/1024)
	t.Logf("  Processing time: %v", processingTime)
	t.Logf("  Chunks created: %d", stats.ChunksCreated)
	t.Logf("  Embeddings generated: %d", stats.EmbeddingsGenerated)
	t.Logf("  Memory used: %.2f MB", float64(memoryUsed)/1024/1024)
	t.Logf("  Processing rate: %.2f MB/s", float64(100*1024*1024)/processingTime.Seconds()/1024/1024)
}

// TestBatchEmbeddingGeneration tests embedding generation for 10,000+ chunks
func TestBatchEmbeddingGeneration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping batch embedding test in short mode")
	}

	if !servicesReady(t) {
		t.Skip("Services not available for batch embedding testing")
	}

	cfg := createTestConfig(t)
	logger := createTestLogger(t)

	// Create OpenAI client
	openaiClient, err := openai.NewClient(cfg.OpenAI.APIKey, logger)
	require.NoError(t, err, "Failed to create OpenAI client")

	// Generate 10,000 test chunks
	chunks := generateTestChunks(10000, 100) // 10k chunks, 100 words each

	var memBefore, memAfter runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	start := time.Now()

	// Process chunks in batches to avoid API limits
	batchSize := 100
	totalEmbeddings := 0

	for i := 0; i < len(chunks); i += batchSize {
		end := i + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}

		batch := chunks[i:end]

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		response, err := openaiClient.EmbedTexts(ctx, batch)
		cancel()

		require.NoError(t, err, "Batch embedding generation should not fail")
		totalEmbeddings += len(response.Embeddings)

		// Log progress every 1000 chunks
		if (i+batchSize)%1000 == 0 {
			t.Logf("Processed %d/%d chunks", i+batchSize, len(chunks))
		}
	}

	processingTime := time.Since(start)
	runtime.ReadMemStats(&memAfter)

	// Assertions
	assert.Equal(t, len(chunks), totalEmbeddings, "All chunks should have embeddings")
	assert.Less(t, processingTime, maxProcessingTime, "Batch processing should complete within time limit")

	// Memory usage checks
	memoryUsed := memAfter.Alloc - memBefore.Alloc
	t.Logf("Batch embedding results:")
	t.Logf("  Total chunks: %d", len(chunks))
	t.Logf("  Total embeddings: %d", totalEmbeddings)
	t.Logf("  Processing time: %v", processingTime)
	t.Logf("  Memory used: %.2f MB", float64(memoryUsed)/1024/1024)
	t.Logf("  Processing rate: %.2f chunks/sec", float64(len(chunks))/processingTime.Seconds())
}

// TestChromaDBBatchInsertion tests ChromaDB batch insertion performance
func TestChromaDBBatchInsertion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping ChromaDB batch insertion test in short mode")
	}

	if !servicesReady(t) {
		t.Skip("Services not available for ChromaDB batch insertion testing")
	}

	cfg := createTestConfig(t)
	_ = createTestLogger(t)

	// Create ChromaDB client
	chromaClient := chroma.NewClient(cfg.Chroma.URL, testCollectionName)

	// Health check
	ctx := context.Background()
	err := chromaClient.HealthCheck(ctx)
	require.NoError(t, err, "ChromaDB should be healthy")

	// Create test collection
	err = chromaClient.CreateCollection(ctx, testCollectionName, map[string]interface{}{
		"description": "Performance test collection",
		"created_at":  time.Now().Format(time.RFC3339),
	})
	if err != nil {
		t.Logf("Collection creation failed (may already exist): %v", err)
	}

	// Cleanup collection after test
	defer func() {
		if err := chromaClient.DeleteCollection(ctx, testCollectionName); err != nil {
			t.Logf("Failed to cleanup test collection: %v", err)
		}
	}()

	// Generate test documents and embeddings
	documentCount := 5000
	documents := make([]chroma.Document, documentCount)
	embeddings := make([][]float32, documentCount)

	for i := 0; i < documentCount; i++ {
		documents[i] = chroma.Document{
			ID:      fmt.Sprintf("perf_test_doc_%d", i),
			Content: fmt.Sprintf("This is performance test document number %d with some content for testing batch insertion capabilities of ChromaDB.", i),
			Metadata: map[string]string{
				"doc_id":     fmt.Sprintf("perf_test_%d", i),
				"test_type":  "batch_insertion",
				"batch_num":  fmt.Sprintf("%d", i/100),
				"created_at": time.Now().Format(time.RFC3339),
			},
		}
		// Generate fake embeddings (1536 dimensions like OpenAI)
		embeddings[i] = generateFakeEmbedding(1536)
	}

	var memBefore, memAfter runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	start := time.Now()

	// Batch insert documents
	batchSize := 100
	for i := 0; i < len(documents); i += batchSize {
		end := i + batchSize
		if end > len(documents) {
			end = len(documents)
		}

		batch := documents[i:end]
		batchEmbeddings := embeddings[i:end]

		err = chromaClient.AddDocuments(ctx, batch, batchEmbeddings)
		require.NoError(t, err, "Batch insertion should not fail")

		// Log progress
		if (i+batchSize)%500 == 0 {
			t.Logf("Inserted %d/%d documents", i+batchSize, len(documents))
		}
	}

	insertTime := time.Since(start)
	runtime.ReadMemStats(&memAfter)

	// Test retrieval performance
	start = time.Now()
	queryEmbedding := generateFakeEmbedding(1536)
	results, err := chromaClient.Search(ctx, queryEmbedding, 10, nil)
	queryTime := time.Since(start)

	require.NoError(t, err, "Query should not fail")
	assert.Greater(t, len(results), 0, "Should return results")

	// Performance logging
	memoryUsed := memAfter.Alloc - memBefore.Alloc
	t.Logf("ChromaDB batch insertion results:")
	t.Logf("  Total documents: %d", len(documents))
	t.Logf("  Insertion time: %v", insertTime)
	t.Logf("  Query time: %v", queryTime)
	t.Logf("  Memory used: %.2f MB", float64(memoryUsed)/1024/1024)
	t.Logf("  Insertion rate: %.2f docs/sec", float64(len(documents))/insertTime.Seconds())
}

// TestSQLiteMetadataPerformance tests SQLite metadata operations with large datasets
func TestSQLiteMetadataPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping SQLite metadata performance test in short mode")
	}

	_ = createTestConfig(t)
	logger := createTestLogger(t)

	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_metadata.db")

	// Create metadata store
	metadataStore, err := metadata.NewStore(dbPath, logger)
	require.NoError(t, err, "Failed to create metadata store")
	defer metadataStore.Close()

	// Generate large dataset
	entryCount := 10000
	entries := make([]metadata.Entry, entryCount)

	for i := 0; i < entryCount; i++ {
		entries[i] = metadata.Entry{
			DocID:         fmt.Sprintf("perf_test_doc_%d", i),
			Title:         fmt.Sprintf("Performance Test Document %d", i),
			Path:          fmt.Sprintf("docs/perf_test_%d.md", i),
			Platform:      []string{"aws", "azure", "gcp"}[i%3],
			Scenario:      []string{"migration", "hybrid", "disaster-recovery", "security"}[i%4],
			Type:          []string{"playbook", "runbook", "sow"}[i%3],
			SourceURL:     fmt.Sprintf("https://example.com/doc_%d", i),
			Difficulty:    []string{"beginner", "intermediate", "advanced"}[i%3],
			EstimatedTime: fmt.Sprintf("%d-hours", (i%8)+1),
			Tags:          []string{fmt.Sprintf("tag_%d", i%5), fmt.Sprintf("category_%d", i%3)},
		}
	}

	var memBefore, memAfter runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	start := time.Now()

	// Batch insert metadata
	for i := 0; i < len(entries); i += 100 {
		end := i + 100
		if end > len(entries) {
			end = len(entries)
		}

		batch := entries[i:end]
		for j := range batch {
			err = metadataStore.AddMetadata(batch[j])
			require.NoError(t, err, "Metadata insertion should not fail")
		}

		// Log progress
		if (i+100)%1000 == 0 {
			t.Logf("Inserted %d/%d metadata entries", i+100, len(entries))
		}
	}

	insertTime := time.Since(start)
	runtime.ReadMemStats(&memAfter)

	// Test various query patterns
	queryTests := []struct {
		name        string
		queryFunc   func() ([]metadata.Entry, error)
		description string
	}{
		{
			name: "GetByPlatform",
			queryFunc: func() ([]metadata.Entry, error) {
				docIDs, err := metadataStore.FilterDocuments(metadata.FilterOptions{Platform: "aws"})
				if err != nil {
					return nil, err
				}
				var results []metadata.Entry
				for _, docID := range docIDs {
					entry, err := metadataStore.GetMetadataByDocID(docID)
					if err != nil {
						return nil, err
					}
					if entry != nil {
						results = append(results, *entry)
					}
				}
				return results, nil
			},
			description: "Query by platform",
		},
		{
			name: "GetByScenario",
			queryFunc: func() ([]metadata.Entry, error) {
				docIDs, err := metadataStore.FilterDocuments(metadata.FilterOptions{Scenario: "migration"})
				if err != nil {
					return nil, err
				}
				var results []metadata.Entry
				for _, docID := range docIDs {
					entry, err := metadataStore.GetMetadataByDocID(docID)
					if err != nil {
						return nil, err
					}
					if entry != nil {
						results = append(results, *entry)
					}
				}
				return results, nil
			},
			description: "Query by scenario",
		},
		{
			name: "GetByType",
			queryFunc: func() ([]metadata.Entry, error) {
				docIDs, err := metadataStore.FilterDocuments(metadata.FilterOptions{Type: "playbook"})
				if err != nil {
					return nil, err
				}
				var results []metadata.Entry
				for _, docID := range docIDs {
					entry, err := metadataStore.GetMetadataByDocID(docID)
					if err != nil {
						return nil, err
					}
					if entry != nil {
						results = append(results, *entry)
					}
				}
				return results, nil
			},
			description: "Query by type",
		},
	}

	for _, tt := range queryTests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Now()
			results, err := tt.queryFunc()
			queryTime := time.Since(start)

			require.NoError(t, err, "Query should not fail")
			assert.Greater(t, len(results), 0, "Should return results")

			t.Logf("%s: %d results in %v", tt.description, len(results), queryTime)
		})
	}

	// Performance logging
	memoryUsed := memAfter.Alloc - memBefore.Alloc
	t.Logf("SQLite metadata performance results:")
	t.Logf("  Total entries: %d", len(entries))
	t.Logf("  Insertion time: %v", insertTime)
	t.Logf("  Memory used: %.2f MB", float64(memoryUsed)/1024/1024)
	t.Logf("  Insertion rate: %.2f entries/sec", float64(len(entries))/insertTime.Seconds())
}

// TestGracefulLargeDocumentHandling tests graceful handling of extremely large documents
func TestGracefulLargeDocumentHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping graceful large document handling test in short mode")
	}

	cfg := createTestConfig(t)
	logger := createTestLogger(t)

	// Test with progressively larger documents
	testSizes := []int{
		50 * 1024 * 1024,  // 50MB
		100 * 1024 * 1024, // 100MB
		200 * 1024 * 1024, // 200MB
		500 * 1024 * 1024, // 500MB
	}

	for _, size := range testSizes {
		t.Run(fmt.Sprintf("Size_%dMB", size/1024/1024), func(t *testing.T) {
			// Create test document
			testFile := createLargeTestDocument(t, size)
			defer os.Remove(testFile)

			var memBefore, memAfter runtime.MemStats
			runtime.ReadMemStats(&memBefore)

			start := time.Now()

			// Process document
			stats, err := processLargeDocument(t, cfg, testFile, logger)
			processingTime := time.Since(start)

			runtime.ReadMemStats(&memAfter)
			memoryUsed := memAfter.Alloc - memBefore.Alloc

			// Assert graceful handling
			if size <= 200*1024*1024 { // Up to 200MB should succeed
				require.NoError(t, err, "Document processing should succeed for reasonable sizes")
				assert.Greater(t, stats.ChunksCreated, 0, "Should create chunks")
			} else {
				// For very large documents, either succeed or fail gracefully
				if err != nil {
					t.Logf("Large document (%d MB) failed gracefully: %v", size/1024/1024, err)
				} else {
					t.Logf("Large document (%d MB) processed successfully", size/1024/1024)
				}
			}

			// Memory usage should be reasonable
			assert.Less(t, memoryUsed, uint64(maxMemoryUsageBytes), "Memory usage should be within limits")

			t.Logf("Document size: %d MB, Processing time: %v, Memory used: %.2f MB, Chunks: %d",
				size/1024/1024, processingTime, float64(memoryUsed)/1024/1024, stats.ChunksCreated)
		})
	}
}

// Helper types and functions

type ProcessingStats struct {
	ChunksCreated       int
	EmbeddingsGenerated int
	ProcessingTime      time.Duration
	MemoryUsed          uint64
}

func createTestConfig(_ *testing.T) *config.Config {
	return &config.Config{
		OpenAI: config.OpenAIConfig{
			APIKey: os.Getenv("OPENAI_API_KEY"),
		},
		Chroma: config.ChromaConfig{
			URL:            "http://localhost:8000",
			CollectionName: testCollectionName,
		},
		Metadata: config.MetadataConfig{
			DBPath: "test_metadata.db",
		},
	}
}

func createTestLogger(t *testing.T) *zap.Logger {
	logger, err := zap.NewDevelopment()
	require.NoError(t, err, "Failed to create test logger")
	return logger
}

func createLargeTestDocument(t *testing.T, sizeBytes int) string {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, fmt.Sprintf("large_test_doc_%d.md", sizeBytes))

	// Create content
	content := generateLargeMarkdownContent(sizeBytes)

	err := os.WriteFile(filename, []byte(content), 0600)
	require.NoError(t, err, "Failed to create large test document")

	return filename
}

func generateLargeMarkdownContent(sizeBytes int) string {
	var content strings.Builder

	// Header
	content.WriteString("# Large Test Document\n\n")
	content.WriteString("This is a large document generated for performance testing.\n\n")

	// Calculate how much content we need
	baseContent := `## Section %d

This is section %d of the large test document. It contains detailed information about cloud architecture, migration strategies, and best practices. The content is designed to be realistic and representative of actual documentation that would be processed by the AI SA Assistant.

### Subsection %d.1

Here we discuss the technical implementation details, including:
- Infrastructure requirements
- Security considerations
- Performance optimization
- Cost analysis
- Risk assessment

### Subsection %d.2

This subsection covers operational aspects:
- Monitoring and alerting
- Backup and recovery
- Scalability planning
- Maintenance procedures
- Troubleshooting guides

`

	sectionNum := 1
	for content.Len() < sizeBytes {
		sectionContent := fmt.Sprintf(baseContent, sectionNum, sectionNum, sectionNum, sectionNum)
		content.WriteString(sectionContent)
		sectionNum++
	}

	return content.String()
}

func processLargeDocument(_ *testing.T, cfg *config.Config, filePath string, logger *zap.Logger) (*ProcessingStats, error) {
	ctx := context.Background()

	// Read document
	// Validate file path to prevent path traversal attacks
	if !filepath.IsAbs(filePath) {
		return nil, fmt.Errorf("file path must be absolute: %s", filePath)
	}
	cleanPath := filepath.Clean(filePath)
	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read document: %w", err)
	}

	// Parse and chunk
	cleanContent := chunker.ParseMarkdown(string(content))
	chunks := chunker.Splitter(cleanContent, 500)

	stats := &ProcessingStats{
		ChunksCreated: len(chunks),
	}

	// Only generate embeddings for a subset to avoid API costs
	maxChunksForEmbedding := 100
	if len(chunks) > maxChunksForEmbedding {
		chunks = chunks[:maxChunksForEmbedding]
	}

	// Generate embeddings
	if cfg.OpenAI.APIKey != "" {
		openaiClient, err := openai.NewClient(cfg.OpenAI.APIKey, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create OpenAI client: %w", err)
		}

		response, err := openaiClient.EmbedTexts(ctx, chunks)
		if err != nil {
			return nil, fmt.Errorf("failed to generate embeddings: %w", err)
		}

		stats.EmbeddingsGenerated = len(response.Embeddings)
	}

	return stats, nil
}

func generateTestChunks(count, wordsPerChunk int) []string {
	chunks := make([]string, count)
	words := []string{"cloud", "architecture", "migration", "security", "performance", "optimization", "infrastructure", "deployment", "monitoring", "scaling"}

	for i := 0; i < count; i++ {
		var chunk strings.Builder
		for j := 0; j < wordsPerChunk; j++ {
			if j > 0 {
				chunk.WriteString(" ")
			}
			chunk.WriteString(words[j%len(words)])
		}
		chunks[i] = chunk.String()
	}

	return chunks
}

func generateFakeEmbedding(dimensions int) []float32 {
	embedding := make([]float32, dimensions)
	for i := range embedding {
		embedding[i] = float32(i) / float32(dimensions)
	}
	return embedding
}
