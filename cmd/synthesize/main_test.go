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
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/health"
	internalopenai "github.com/your-org/ai-sa-assistant/internal/openai"
	"github.com/your-org/ai-sa-assistant/internal/synth"
)

// mockOpenAIServer creates a mock OpenAI server for testing
func mockOpenAIServer(t testing.TB, responses map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Mock server received request: %s %s", r.Method, r.URL.Path)

		// Handle both /v1/embeddings and /embeddings paths
		if r.URL.Path == "/v1/embeddings" || r.URL.Path == "/embeddings" {
			if response, ok := responses["embeddings"]; ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(response))
				return
			}
		}
		// Handle both /v1/chat/completions and /chat/completions paths
		if r.URL.Path == "/v1/chat/completions" || r.URL.Path == "/chat/completions" {
			if response, ok := responses["chat"]; ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(response))
				return
			}
		}

		t.Logf("Mock server: no response found for path %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": "not found"}`))
	}))
}

// createMockChatResponse creates a mock chat completion response
func createMockChatResponse() string {
	return `{
		"id": "chatcmpl-test",
		"object": "chat.completion",
		"created": 1234567890,
		"model": "gpt-4o",
		"choices": [
			{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "This is a comprehensive test response for synthesis testing."
				},
				"finish_reason": "stop"
			}
		],
		"usage": {
			"prompt_tokens": 100,
			"completion_tokens": 50,
			"total_tokens": 150
		}
	}`
}

// createMockEmbeddingResponse creates a mock embedding response
func createMockEmbeddingResponse() string {
	// Create a mock embedding with 1536 dimensions (as expected by the client)
	embedding := make([]string, 1536)
	for i := 0; i < 1536; i++ {
		embedding[i] = "0.1"
	}

	return `{
		"object": "list",
		"data": [
			{
				"object": "embedding",
				"embedding": [` + strings.Join(embedding, ",") + `],
				"index": 0
			}
		],
		"model": "text-embedding-3-small",
		"usage": {
			"prompt_tokens": 10,
			"total_tokens": 10
		}
	}`
}

// createTestConfig creates a test configuration
func createTestConfig() *config.Config {
	return &config.Config{
		OpenAI: config.OpenAIConfig{
			APIKey:   "sk-test-api-key-12345678901234567890",
			Endpoint: "https://api.openai.com",
		},
		Synthesis: config.SynthesisConfig{
			Model:       "gpt-4o",
			MaxTokens:   2000,
			Temperature: 0.3,
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
	}
}

// createTestOpenAIClient creates a test OpenAI client with mock server
func createTestOpenAIClient(mockServerURL string, logger *zap.Logger) *internalopenai.Client {
	openaiConfig := openai.DefaultConfig("sk-test-api-key-12345678901234567890")
	openaiConfig.BaseURL = mockServerURL
	return internalopenai.NewClientWithConfig(openaiConfig, logger)
}

// TestValidateSynthesisRequest tests the synthesis request validation
func TestValidateSynthesisRequest(t *testing.T) {
	tests := []struct {
		name        string
		request     SynthesisRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid request with chunks",
			request: SynthesisRequest{
				Query: "Test query",
				Chunks: []ChunkItem{
					{
						Text:     "Test chunk content",
						DocID:    "doc1",
						SourceID: "source1",
					},
				},
			},
			expectError: false,
		},
		{
			name: "Valid request with web results",
			request: SynthesisRequest{
				Query: "Test query",
				WebResults: []WebResult{
					{
						Title:   "Test Title",
						Snippet: "Test snippet",
						URL:     "https://example.com",
					},
				},
			},
			expectError: false,
		},
		{
			name: "Empty query",
			request: SynthesisRequest{
				Query: "",
				Chunks: []ChunkItem{
					{Text: "Test", DocID: "doc1"},
				},
			},
			expectError: true,
			errorMsg:    "query cannot be empty",
		},
		{
			name: "Query too long",
			request: SynthesisRequest{
				Query: strings.Repeat("a", MaxQueryLength+1),
				Chunks: []ChunkItem{
					{Text: "Test", DocID: "doc1"},
				},
			},
			expectError: true,
			errorMsg:    "query is too long",
		},
		{
			name: "No chunks or web results",
			request: SynthesisRequest{
				Query: "Test query",
			},
			expectError: true,
			errorMsg:    "at least one chunk or web result must be provided",
		},
		{
			name: "Empty chunk text",
			request: SynthesisRequest{
				Query: "Test query",
				Chunks: []ChunkItem{
					{Text: "", DocID: "doc1"},
				},
			},
			expectError: true,
			errorMsg:    "chunk 0 text cannot be empty",
		},
		{
			name: "Empty chunk DocID",
			request: SynthesisRequest{
				Query: "Test query",
				Chunks: []ChunkItem{
					{Text: "Test", DocID: ""},
				},
			},
			expectError: true,
			errorMsg:    "chunk 0 doc_id cannot be empty",
		},
		{
			name: "Web result with no title or snippet",
			request: SynthesisRequest{
				Query: "Test query",
				WebResults: []WebResult{
					{URL: "https://example.com"},
				},
			},
			expectError: true,
			errorMsg:    "web result 0 must have either title or snippet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSynthesisRequest(tt.request)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestConvertChunksToContextItems tests chunk conversion
func TestConvertChunksToContextItems(t *testing.T) {
	tests := []struct {
		name   string
		chunks []ChunkItem
		want   int
	}{
		{
			name:   "Empty chunks",
			chunks: []ChunkItem{},
			want:   0,
		},
		{
			name: "Single chunk with SourceID",
			chunks: []ChunkItem{
				{Text: "Test", DocID: "doc1", SourceID: "source1"},
			},
			want: 1,
		},
		{
			name: "Multiple chunks",
			chunks: []ChunkItem{
				{Text: "Test1", DocID: "doc1", SourceID: "source1"},
				{Text: "Test2", DocID: "doc2", SourceID: "source2"},
			},
			want: 2,
		},
		{
			name: "Chunk without SourceID",
			chunks: []ChunkItem{
				{Text: "Test", DocID: "doc1"},
			},
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertChunksToContextItems(tt.chunks)
			assert.Len(t, result, tt.want)

			for i, chunk := range tt.chunks {
				assert.Equal(t, chunk.Text, result[i].Content)
				if chunk.SourceID != "" {
					assert.Equal(t, chunk.SourceID, result[i].SourceID)
				} else {
					assert.Equal(t, chunk.DocID, result[i].SourceID)
				}
				assert.Equal(t, float64(1.0), result[i].Score)
				assert.Equal(t, 1, result[i].Priority)
			}
		})
	}
}

// TestConvertWebResults tests web result conversion
func TestConvertWebResults(t *testing.T) {
	tests := []struct {
		name           string
		webResults     []WebResult
		wantStrings    []string
		wantSourceURLs []string
	}{
		{
			name:           "Empty web results",
			webResults:     []WebResult{},
			wantStrings:    []string{},
			wantSourceURLs: []string{},
		},
		{
			name: "Complete web result",
			webResults: []WebResult{
				{
					Title:   "Test Title",
					Snippet: "Test snippet",
					URL:     "https://example.com",
				},
			},
			wantStrings: []string{
				"Title: Test Title\nSnippet: Test snippet\nURL: https://example.com",
			},
			wantSourceURLs: []string{"https://example.com"},
		},
		{
			name: "Title only",
			webResults: []WebResult{
				{
					Title: "Test Title",
					URL:   "https://example.com",
				},
			},
			wantStrings: []string{
				"Title: Test Title\nURL: https://example.com",
			},
			wantSourceURLs: []string{"https://example.com"},
		},
		{
			name: "Snippet only",
			webResults: []WebResult{
				{
					Snippet: "Test snippet",
					URL:     "https://example.com",
				},
			},
			wantStrings: []string{
				"Snippet: Test snippet\nURL: https://example.com",
			},
			wantSourceURLs: []string{"https://example.com"},
		},
		{
			name: "No URL",
			webResults: []WebResult{
				{
					Title:   "Test Title",
					Snippet: "Test snippet",
				},
			},
			wantStrings: []string{
				"Title: Test Title\nSnippet: Test snippet\nURL: ",
			},
			wantSourceURLs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strings, urls := convertWebResults(tt.webResults)
			assert.Equal(t, tt.wantStrings, strings)
			assert.Equal(t, tt.wantSourceURLs, urls)
		})
	}
}

// TestIsValidWebSourceURL tests web source URL validation
func TestIsValidWebSourceURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "Valid HTTP URL", url: "http://example.com", want: true},
		{name: "Valid HTTPS URL", url: "https://example.com", want: true},
		{name: "Empty URL", url: "", want: false},
		{name: "Whitespace URL", url: "   ", want: false},
		{name: "Invalid scheme", url: "ftp://example.com", want: false},
		{name: "No scheme", url: "example.com", want: false},
		{name: "URL with whitespace", url: "  https://example.com  ", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidWebSourceURL(tt.url)
			assert.Equal(t, tt.want, result)
		})
	}
}

// TestInitializeLogger tests logger initialization
func TestInitializeLogger(t *testing.T) {
	tests := []struct {
		name   string
		config *config.Config
		want   bool
	}{
		{
			name: "JSON logger",
			config: &config.Config{
				Logging: config.LoggingConfig{
					Level:  "info",
					Format: "json",
					Output: "stdout",
				},
			},
			want: true,
		},
		{
			name: "Development logger",
			config: &config.Config{
				Logging: config.LoggingConfig{
					Level:  "debug",
					Format: "console",
					Output: "stdout",
				},
			},
			want: true,
		},
		{
			name: "File output",
			config: &config.Config{
				Logging: config.LoggingConfig{
					Level:  "warn",
					Format: "json",
					Output: "file",
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := initializeLogger(tt.config)
			if tt.want {
				assert.NoError(t, err)
				assert.NotNil(t, logger)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// TestSetupHealthChecks tests health check setup
func TestSetupHealthChecks(t *testing.T) {
	cfg := createTestConfig()
	logger := zaptest.NewLogger(t)

	// Mock OpenAI server
	mockServer := mockOpenAIServer(t, map[string]string{
		"embeddings": createMockEmbeddingResponse(),
		"chat":       createMockChatResponse(),
	})
	defer mockServer.Close()

	// Create OpenAI client with mock server
	openaiClient := createTestOpenAIClient(mockServer.URL, logger)

	// Create health manager
	manager := health.NewManager("synthesize", "1.0.0", logger)

	// Setup health checks
	setupHealthChecks(manager, cfg, openaiClient)

	// Test health check
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := manager.Check(ctx)
	assert.NotNil(t, result)
}

// TestSynthesisHandler tests the synthesis HTTP handler
func TestSynthesisHandler(t *testing.T) {
	cfg := createTestConfig()
	logger := zaptest.NewLogger(t)

	// Mock OpenAI server
	mockServer := mockOpenAIServer(t, map[string]string{
		"chat": createMockChatResponse(),
	})
	defer mockServer.Close()

	// Create OpenAI client with mock server
	openaiClient := createTestOpenAIClient(mockServer.URL, logger)

	// Create handler
	handler := createSynthesisHandler(cfg, logger, openaiClient)

	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		request        SynthesisRequest
		expectedStatus int
		checkResponse  func(t *testing.T, response map[string]interface{})
	}{
		{
			name: "Valid synthesis request",
			request: SynthesisRequest{
				Query: "Test query",
				Chunks: []ChunkItem{
					{Text: "Test chunk", DocID: "doc1", SourceID: "source1"},
				},
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				assert.Contains(t, response, "main_text")
				assert.Contains(t, response, "metadata")

				if metadata, ok := response["metadata"].(map[string]interface{}); ok {
					assert.Contains(t, metadata, "total_tokens")
					assert.Contains(t, metadata, "processing_time")
					assert.Contains(t, metadata, "model")
				}
			},
		},
		{
			name: "Invalid request - empty query",
			request: SynthesisRequest{
				Query: "",
				Chunks: []ChunkItem{
					{Text: "Test chunk", DocID: "doc1"},
				},
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				assert.Contains(t, response, "error")
				assert.Contains(t, response, "details")
			},
		},
		{
			name: "Invalid request - no chunks or web results",
			request: SynthesisRequest{
				Query: "Test query",
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				assert.Contains(t, response, "error")
				assert.Contains(t, response, "details")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request
			reqBody, err := json.Marshal(tt.request)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, "/synthesize", bytes.NewBuffer(reqBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			// Create response recorder
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			// Call handler
			handler(c)

			// Check status
			assert.Equal(t, tt.expectedStatus, w.Code)

			// Parse response
			var response map[string]interface{}
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			// Check response
			if tt.checkResponse != nil {
				tt.checkResponse(t, response)
			}
		})
	}
}

// TestSynthesisHandlerWithWebResults tests synthesis with web results
func TestSynthesisHandlerWithWebResults(t *testing.T) {
	cfg := createTestConfig()
	logger := zaptest.NewLogger(t)

	// Mock OpenAI server
	mockServer := mockOpenAIServer(t, map[string]string{
		"chat": createMockChatResponse(),
	})
	defer mockServer.Close()

	// Create OpenAI client with mock server
	openaiClient := createTestOpenAIClient(mockServer.URL, logger)

	// Create handler
	handler := createSynthesisHandler(cfg, logger, openaiClient)

	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	request := SynthesisRequest{
		Query: "Test query with web results",
		WebResults: []WebResult{
			{
				Title:   "Test Web Result",
				Snippet: "This is a test web result",
				URL:     "https://example.com",
			},
		},
	}

	// Create request
	reqBody, err := json.Marshal(request)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "/synthesize", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	// Create response recorder
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Call handler
	handler(c)

	// Check status
	assert.Equal(t, http.StatusOK, w.Code)

	// Parse response
	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify response structure
	assert.Contains(t, response, "main_text")
	assert.Contains(t, response, "metadata")

	if metadata, ok := response["metadata"].(map[string]interface{}); ok {
		assert.Contains(t, metadata, "total_tokens")
		assert.Contains(t, metadata, "processing_time")
		assert.Contains(t, metadata, "model")
		assert.Equal(t, cfg.Synthesis.Model, metadata["model"])
	}
}

// TestSetupRouter tests router setup
func TestSetupRouter(t *testing.T) {
	cfg := createTestConfig()
	logger := zaptest.NewLogger(t)

	// Mock OpenAI server
	mockServer := mockOpenAIServer(t, map[string]string{
		"embeddings": createMockEmbeddingResponse(),
		"chat":       createMockChatResponse(),
	})
	defer mockServer.Close()

	// Create OpenAI client with mock server
	openaiClient := createTestOpenAIClient(mockServer.URL, logger)

	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Setup router
	router := setupRouter(cfg, logger, openaiClient)

	// Test health endpoint
	req, err := http.NewRequest(http.MethodGet, "/health", nil)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Health check should return 200 (now that we fixed the embedding response)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test synthesis endpoint exists
	req, err = http.NewRequest(http.MethodPost, "/synthesize", nil)
	require.NoError(t, err)

	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return 400 for invalid request, not 404
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestErrorHandling tests error handling scenarios
func TestErrorHandling(t *testing.T) {
	cfg := createTestConfig()
	logger := zaptest.NewLogger(t)

	// Mock OpenAI server that returns errors
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "Internal server error"}`))
	}))
	defer mockServer.Close()

	// Create OpenAI client with mock server
	openaiClient := createTestOpenAIClient(mockServer.URL, logger)

	// Create handler
	handler := createSynthesisHandler(cfg, logger, openaiClient)

	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	request := SynthesisRequest{
		Query: "Test query",
		Chunks: []ChunkItem{
			{Text: "Test chunk", DocID: "doc1"},
		},
	}

	// Create request
	reqBody, err := json.Marshal(request)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "/synthesize", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	// Create response recorder
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Call handler
	handler(c)

	// Should return 500 for OpenAI error
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Parse response
	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Contains(t, response, "error")
	assert.Contains(t, response, "details")
}

// TestInvalidJSON tests handling of invalid JSON
func TestInvalidJSON(t *testing.T) {
	cfg := createTestConfig()
	logger := zaptest.NewLogger(t)

	// Mock OpenAI server (doesn't matter since we won't reach it)
	mockServer := mockOpenAIServer(t, map[string]string{
		"chat": createMockChatResponse(),
	})
	defer mockServer.Close()

	// Create OpenAI client with mock server
	openaiClient := createTestOpenAIClient(mockServer.URL, logger)

	// Create handler
	handler := createSynthesisHandler(cfg, logger, openaiClient)

	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Create request with invalid JSON
	req, err := http.NewRequest(http.MethodPost, "/synthesize", bytes.NewBuffer([]byte("invalid json")))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	// Create response recorder
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Call handler
	handler(c)

	// Should return 400 for invalid JSON
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Parse response
	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Contains(t, response, "error")
	assert.Equal(t, "Invalid request format", response["error"])
}

// TestValidateSynthesisSourceMetadata tests source metadata validation
func TestValidateSynthesisSourceMetadata(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name          string
		contextItems  []synth.ContextItem
		webSourceURLs []string
		expectError   bool
	}{
		{
			name: "Valid sources",
			contextItems: []synth.ContextItem{
				{Content: "Test content", SourceID: "source1"},
			},
			webSourceURLs: []string{"https://example.com"},
			expectError:   false,
		},
		{
			name:          "No sources",
			contextItems:  []synth.ContextItem{},
			webSourceURLs: []string{},
			expectError:   true,
		},
		{
			name: "Empty content",
			contextItems: []synth.ContextItem{
				{Content: "", SourceID: "source1"},
			},
			webSourceURLs: []string{},
			expectError:   true,
		},
		{
			name: "Invalid web URL",
			contextItems: []synth.ContextItem{
				{Content: "Test content", SourceID: "source1"},
			},
			webSourceURLs: []string{"not-a-url"},
			expectError:   false, // Invalid URLs are warnings, not errors
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSynthesisSourceMetadata(tt.contextItems, tt.webSourceURLs, logger)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestConfigurationLoading tests configuration loading scenarios
func TestConfigurationLoading(t *testing.T) {
	// Test with environment variables
	originalAPIKey := os.Getenv("OPENAI_API_KEY")
	defer func() {
		if originalAPIKey != "" {
			_ = os.Setenv("OPENAI_API_KEY", originalAPIKey)
		} else {
			_ = os.Unsetenv("OPENAI_API_KEY")
		}
	}()

	// Set test environment variable
	_ = os.Setenv("OPENAI_API_KEY", "sk-test-env-key-12345678901234567890")

	// This would normally load from config file or environment
	// In a real test, we'd use a test config file
	logger := zaptest.NewLogger(t)

	// Mock OpenAI server
	mockServer := mockOpenAIServer(t, map[string]string{
		"chat": createMockChatResponse(),
	})
	defer mockServer.Close()

	// Verify we can setup services
	openaiClient := createTestOpenAIClient(mockServer.URL, logger)
	assert.NotNil(t, openaiClient)
}
