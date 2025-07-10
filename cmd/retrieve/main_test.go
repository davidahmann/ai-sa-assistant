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
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"

	"github.com/your-org/ai-sa-assistant/internal/chroma"
	"github.com/your-org/ai-sa-assistant/internal/classifier"
	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/health"
)

func TestConfigurationValidation(t *testing.T) {
	tests := []struct {
		name                           string
		fallbackThreshold              int
		fallbackScoreThreshold         float64
		expectedFallbackThreshold      int
		expectedFallbackScoreThreshold float64
	}{
		{
			name:                           "Default configuration values",
			fallbackThreshold:              3,
			fallbackScoreThreshold:         0.7,
			expectedFallbackThreshold:      3,
			expectedFallbackScoreThreshold: 0.7,
		},
		{
			name:                           "Custom configuration values",
			fallbackThreshold:              5,
			fallbackScoreThreshold:         0.8,
			expectedFallbackThreshold:      5,
			expectedFallbackScoreThreshold: 0.8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Retrieval: config.RetrievalConfig{
					MaxChunks:              5,
					FallbackThreshold:      tt.fallbackThreshold,
					ConfidenceThreshold:    0.7,
					FallbackScoreThreshold: tt.fallbackScoreThreshold,
				},
			}

			if cfg.Retrieval.FallbackThreshold != tt.expectedFallbackThreshold {
				t.Errorf("Expected fallback threshold %d, got %d", tt.expectedFallbackThreshold, cfg.Retrieval.FallbackThreshold)
			}

			if cfg.Retrieval.FallbackScoreThreshold != tt.expectedFallbackScoreThreshold {
				t.Errorf("Expected fallback score threshold %f, got %f",
					tt.expectedFallbackScoreThreshold, cfg.Retrieval.FallbackScoreThreshold)
			}
		})
	}
}

func TestSearchRequestValidation(t *testing.T) {
	tests := []struct {
		name          string
		requestBody   string
		expectedValid bool
	}{
		{
			name:          "Valid request with query",
			requestBody:   `{"query": "test query"}`,
			expectedValid: true,
		},
		{
			name:          "Valid request with query and filters",
			requestBody:   `{"query": "test query", "filters": {"platform": "aws"}}`,
			expectedValid: true,
		},
		{
			name:          "Invalid request - missing query",
			requestBody:   `{"filters": {"platform": "aws"}}`,
			expectedValid: false,
		},
		{
			name:          "Invalid request - empty query",
			requestBody:   `{"query": ""}`,
			expectedValid: false,
		},
		{
			name:          "Invalid JSON",
			requestBody:   `{"query": "test"`,
			expectedValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var searchReq SearchRequest
			err := json.Unmarshal([]byte(tt.requestBody), &searchReq)

			if tt.expectedValid {
				if err != nil {
					t.Errorf("Expected valid JSON, got error: %v", err)
				}
				if searchReq.Query == "" {
					t.Error("Expected non-empty query")
				}
			} else if err == nil && searchReq.Query != "" {
				t.Error("Expected invalid request, but got valid one")
			}
		})
	}
}

func TestSearchResponse(t *testing.T) {
	tests := []struct {
		name                  string
		chunks                []SearchChunk
		fallbackTriggered     bool
		fallbackReason        string
		expectedCount         int
		expectedFallbackField bool
	}{
		{
			name: "Response with fallback triggered",
			chunks: []SearchChunk{
				{Text: "chunk1", Score: 0.8, DocID: "doc1"},
				{Text: "chunk2", Score: 0.7, DocID: "doc2"},
			},
			fallbackTriggered:     true,
			fallbackReason:        "insufficient results (2 < 3)",
			expectedCount:         2,
			expectedFallbackField: true,
		},
		{
			name: "Response without fallback",
			chunks: []SearchChunk{
				{Text: "chunk1", Score: 0.9, DocID: "doc1"},
				{Text: "chunk2", Score: 0.8, DocID: "doc2"},
				{Text: "chunk3", Score: 0.7, DocID: "doc3"},
			},
			fallbackTriggered:     false,
			fallbackReason:        "",
			expectedCount:         3,
			expectedFallbackField: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := SearchResponse{
				Chunks:            tt.chunks,
				Count:             len(tt.chunks),
				Query:             "test query",
				FallbackTriggered: tt.fallbackTriggered,
				FallbackReason:    tt.fallbackReason,
			}

			if response.Count != tt.expectedCount {
				t.Errorf("Expected count %d, got %d", tt.expectedCount, response.Count)
			}

			if response.FallbackTriggered != tt.expectedFallbackField {
				t.Errorf("Expected fallback triggered %v, got %v", tt.expectedFallbackField, response.FallbackTriggered)
			}

			if tt.fallbackTriggered && response.FallbackReason != tt.fallbackReason {
				t.Errorf("Expected fallback reason '%s', got '%s'", tt.fallbackReason, response.FallbackReason)
			}

			// Test JSON marshaling
			jsonData, err := json.Marshal(response)
			if err != nil {
				t.Fatalf("Failed to marshal response: %v", err)
			}

			var unmarshaledResponse SearchResponse
			err = json.Unmarshal(jsonData, &unmarshaledResponse)
			if err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}

			if unmarshaledResponse.FallbackTriggered != tt.fallbackTriggered {
				t.Errorf("Expected unmarshaled fallback triggered %v, got %v",
					tt.fallbackTriggered, unmarshaledResponse.FallbackTriggered)
			}

			// Check if fallback_reason is included in JSON when expected
			if tt.fallbackTriggered && !strings.Contains(string(jsonData), "fallback_reason") {
				t.Error("Expected fallback_reason field in JSON when fallback is triggered")
			}
		})
	}
}

// Test initializeLogger function
func TestInitializeLogger(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
	}{
		{
			name: "JSON logger with info level",
			config: &config.Config{
				Logging: config.LoggingConfig{
					Level:  "info",
					Format: "json",
					Output: "stdout",
				},
			},
			expectError: false,
		},
		{
			name: "Console logger with debug level",
			config: &config.Config{
				Logging: config.LoggingConfig{
					Level:  "debug",
					Format: "console",
					Output: "stdout",
				},
			},
			expectError: false,
		},
		{
			name: "File output logger",
			config: &config.Config{
				Logging: config.LoggingConfig{
					Level:  "warn",
					Format: "json",
					Output: "file",
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := initializeLogger(tt.config)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, logger)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, logger)
				_ = logger.Sync()
			}
		})
	}
}

// Test validateSearchRequest function
func TestValidateSearchRequestHandler(t *testing.T) {
	logger := zaptest.NewLogger(t)
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		requestBody  string
		expectedCode int
		expectValid  bool
	}{
		{
			name:         "Valid request with query only",
			requestBody:  `{"query": "test query"}`,
			expectedCode: 0,
			expectValid:  true,
		},
		{
			name:         "Valid request with query and filters",
			requestBody:  `{"query": "test query", "filters": {"platform": "aws"}}`,
			expectedCode: 0,
			expectValid:  true,
		},
		{
			name:         "Invalid request - missing query",
			requestBody:  `{"filters": {"platform": "aws"}}`,
			expectedCode: http.StatusBadRequest,
			expectValid:  false,
		},
		{
			name:         "Invalid request - empty query",
			requestBody:  `{"query": ""}`,
			expectedCode: http.StatusBadRequest,
			expectValid:  false,
		},
		{
			name:         "Invalid JSON",
			requestBody:  `{"query": "test"`,
			expectedCode: http.StatusBadRequest,
			expectValid:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/search", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			searchReq, valid := validateSearchRequest(c, logger)

			assert.Equal(t, tt.expectValid, valid)
			if tt.expectedCode != 0 {
				assert.Equal(t, tt.expectedCode, w.Code)
			}
			if tt.expectValid {
				assert.NotEmpty(t, searchReq.Query)
			}
		})
	}
}

// Test generateQueryEmbedding function in test mode
func TestGenerateQueryEmbedding(t *testing.T) {
	ctx := context.Background()

	// Test when OpenAI client is nil (test mode)
	deps := &ServiceDependencies{
		Logger: zaptest.NewLogger(t),
	}

	embedding, err := generateQueryEmbedding(ctx, "test query", deps)
	assert.NoError(t, err)
	assert.Equal(t, OpenAIEmbeddingDimension, len(embedding))

	// Verify mock embedding has expected values
	for _, val := range embedding {
		assert.Equal(t, float32(0.1), val)
	}
}

// Test shouldApplyFallback function
func TestShouldApplyFallback(t *testing.T) {
	tests := []struct {
		name           string
		searchResults  []chroma.SearchResult
		config         config.RetrievalConfig
		expectFallback bool
		expectReason   string
	}{
		{
			name: "Insufficient results triggers fallback",
			searchResults: []chroma.SearchResult{
				{Distance: 0.2},
				{Distance: 0.3},
			},
			config: config.RetrievalConfig{
				FallbackThreshold:      3,
				FallbackScoreThreshold: 0.7,
			},
			expectFallback: true,
			expectReason:   "insufficient results (2 < 3)",
		},
		{
			name: "Low average score triggers fallback",
			searchResults: []chroma.SearchResult{
				{Distance: 0.8}, // similarity = 0.2
				{Distance: 0.7}, // similarity = 0.3
				{Distance: 0.6}, // similarity = 0.4
			},
			config: config.RetrievalConfig{
				FallbackThreshold:      3,
				FallbackScoreThreshold: 0.7,
			},
			expectFallback: true,
			expectReason:   "low average similarity score (0.300 < 0.700)",
		},
		{
			name: "Good results - no fallback",
			searchResults: []chroma.SearchResult{
				{Distance: 0.1}, // similarity = 0.9
				{Distance: 0.2}, // similarity = 0.8
				{Distance: 0.3}, // similarity = 0.7
			},
			config: config.RetrievalConfig{
				FallbackThreshold:      3,
				FallbackScoreThreshold: 0.7,
			},
			expectFallback: false,
		},
		{
			name:          "Empty results - triggers fallback",
			searchResults: []chroma.SearchResult{},
			config: config.RetrievalConfig{
				FallbackThreshold:      3,
				FallbackScoreThreshold: 0.7,
			},
			expectFallback: true,
			expectReason:   "insufficient results (0 < 3)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fallback, reason := shouldApplyFallback(tt.searchResults, tt.config)
			assert.Equal(t, tt.expectFallback, fallback)
			if tt.expectFallback {
				assert.Contains(t, reason, tt.expectReason)
			}
		})
	}
}

// Test extractDocIDFromChunkID function
func TestExtractDocIDFromChunkID(t *testing.T) {
	tests := []struct {
		name          string
		chunkID       string
		expectedDocID string
	}{
		{
			name:          "Standard chunk ID format",
			chunkID:       "aws-migration-guide_chunk_0",
			expectedDocID: "aws-migration-guide",
		},
		{
			name:          "Doc ID with underscores",
			chunkID:       "aws_best_practices_guide_chunk_1",
			expectedDocID: "aws_best_practices_guide",
		},
		{
			name:          "Complex doc ID",
			chunkID:       "security_compliance_aws_hybrid_chunk_2",
			expectedDocID: "security_compliance_aws_hybrid",
		},
		{
			name:          "No chunk pattern - return whole ID",
			chunkID:       "simple-doc",
			expectedDocID: "simple-doc",
		},
		{
			name:          "Empty string",
			chunkID:       "",
			expectedDocID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docID := extractDocIDFromChunkID(tt.chunkID)
			assert.Equal(t, tt.expectedDocID, docID)
		})
	}
}

// Test constants
func TestConstants(t *testing.T) {
	assert.Equal(t, 3, DefaultRetryAttempts)
	assert.Equal(t, 5*time.Second, HealthCheckTimeout)
	assert.Equal(t, 30*time.Second, SearchRequestTimeout)
	assert.Equal(t, 1536, OpenAIEmbeddingDimension)
}

// Test SearchRequest and SearchResponse types
func TestSearchRequestTypes(t *testing.T) {
	req := SearchRequest{
		Query: "test query",
		Filters: map[string]interface{}{
			"platform": "aws",
			"scenario": "migration",
		},
	}

	assert.Equal(t, "test query", req.Query)
	assert.Equal(t, "aws", req.Filters["platform"])
	assert.Equal(t, "migration", req.Filters["scenario"])
}

func TestSearchChunkTypes(t *testing.T) {
	chunk := SearchChunk{
		Text:     "test content",
		Score:    0.95,
		DocID:    "test-doc_chunk_0",
		SourceID: "https://example.com",
		Metadata: map[string]interface{}{
			"title": "Test Document",
		},
	}

	assert.Equal(t, "test content", chunk.Text)
	assert.Equal(t, 0.95, chunk.Score)
	assert.Equal(t, "test-doc_chunk_0", chunk.DocID)
	assert.Equal(t, "https://example.com", chunk.SourceID)
	assert.Equal(t, "Test Document", chunk.Metadata["title"])
}

// Test ServiceDependencies type
func TestServiceDependenciesTypes(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := &config.Config{
		Retrieval: config.RetrievalConfig{
			MaxChunks:              5,
			ConfidenceThreshold:    0.7,
			FallbackThreshold:      3,
			FallbackScoreThreshold: 0.7,
		},
	}

	deps := &ServiceDependencies{
		Logger:     logger,
		Config:     config,
		Classifier: classifier.NewQueryClassifier(),
	}

	assert.NotNil(t, deps.Logger)
	assert.NotNil(t, deps.Config)
	assert.NotNil(t, deps.Classifier)
	assert.Equal(t, 5, deps.Config.Retrieval.MaxChunks)
	assert.Equal(t, 0.7, deps.Config.Retrieval.ConfidenceThreshold)
}

// Test health check manager setup
func TestHealthCheckManagerSetup(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := health.NewManager("test-service", "1.0.0", logger)

	assert.NotNil(t, manager)

	// Test that the manager can handle basic health checks
	result := manager.Check(context.Background())
	assert.NotNil(t, result)
}
