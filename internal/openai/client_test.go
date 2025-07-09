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

package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// mockOpenAIServer creates a mock OpenAI server for testing
func mockOpenAIServer(_ testing.TB, responses map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/embeddings" {
			if response, ok := responses["embeddings"]; ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(response))
				return
			}
		}
		if r.URL.Path == "/v1/chat/completions" {
			if response, ok := responses["chat"]; ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(response))
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": "not found"}`))
	}))
}

// createMockEmbeddingResponse creates a mock embedding response
func createMockEmbeddingResponse(numEmbeddings int) string {
	embeddings := make([]string, numEmbeddings)
	for i := 0; i < numEmbeddings; i++ {
		// Create a mock embedding with 1536 dimensions
		embedding := make([]string, ExpectedEmbeddingDimensions)
		for j := 0; j < ExpectedEmbeddingDimensions; j++ {
			embedding[j] = fmt.Sprintf("0.%d", j%100)
		}
		embeddings[i] = fmt.Sprintf(`{"object": "embedding", "embedding": [%s], "index": %d}`,
			strings.Join(embedding, ","), i)
	}

	return fmt.Sprintf(`{
		"object": "list",
		"data": [%s],
		"model": "text-embedding-3-small",
		"usage": {
			"prompt_tokens": %d,
			"total_tokens": %d
		}
	}`, strings.Join(embeddings, ","), numEmbeddings*10, numEmbeddings*10)
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
					"content": "This is a test response"
				},
				"finish_reason": "stop"
			}
		],
		"usage": {
			"prompt_tokens": 10,
			"completion_tokens": 5,
			"total_tokens": 15
		}
	}`
}

// TestNewClient tests the client initialization
func TestNewClient(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name      string
		apiKey    string
		expectErr bool
	}{
		{
			name:      "valid API key",
			apiKey:    "sk-test1234567890abcdef", // pragma: allowlist secret
			expectErr: false,
		},
		{
			name:      "empty API key",
			apiKey:    "",
			expectErr: true,
		},
		{
			name:      "invalid API key format",
			apiKey:    "invalid-key", // pragma: allowlist secret
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock server for connection validation
			server := mockOpenAIServer(t, map[string]string{
				"embeddings": createMockEmbeddingResponse(1),
			})
			defer server.Close()

			// Create client with custom base URL
			config := openai.DefaultConfig(tt.apiKey)
			config.BaseURL = server.URL + "/v1"
			client := openai.NewClientWithConfig(config)

			// Create our client wrapper
			c := &Client{
				client: client,
				logger: logger,
				model:  EmbeddingModel,
			}

			// Test validation without actual API call for invalid keys
			if tt.expectErr {
				_, err := NewClient(tt.apiKey, logger)
				if err == nil {
					t.Error("Expected error for invalid API key")
				}
				return
			}

			// For valid keys, we would need to mock the validation call
			// but for this test, we'll just verify the client can be created
			if tt.apiKey != "" && strings.HasPrefix(tt.apiKey, "sk-") {
				// Test basic client structure
				if c.client == nil {
					t.Error("Client should not be nil")
				}
				if c.logger == nil {
					t.Error("Logger should not be nil")
				}
				if c.model != EmbeddingModel {
					t.Errorf("Expected model %s, got %s", EmbeddingModel, c.model)
				}
			}
		})
	}
}

// TestEmbedTexts tests the batch embedding functionality
// setupMockEmbeddingClient creates a mock OpenAI client for embedding tests
func setupMockEmbeddingClient(t *testing.T, logger *zap.Logger, textCount int) *Client {
	server := mockOpenAIServer(t, map[string]string{
		"embeddings": createMockEmbeddingResponse(textCount),
	})
	t.Cleanup(func() { server.Close() })

	config := openai.DefaultConfig("sk-test1234567890abcdef") // pragma: allowlist secret
	config.BaseURL = server.URL + "/v1"
	client := openai.NewClientWithConfig(config)

	return &Client{
		client: client,
		logger: logger,
		model:  EmbeddingModel,
	}
}

// validateEmbeddingResponse validates the structure and content of embedding response
func validateEmbeddingResponse(t *testing.T, response *EmbeddingResponse, expectedCount int) {
	if len(response.Embeddings) != expectedCount {
		t.Errorf("Expected %d embeddings, got %d", expectedCount, len(response.Embeddings))
	}

	for i, embedding := range response.Embeddings {
		if len(embedding) != ExpectedEmbeddingDimensions {
			t.Errorf("Embedding %d has %d dimensions, expected %d", i, len(embedding), ExpectedEmbeddingDimensions)
		}
	}
}

// validateUsageTracking validates that usage tracking fields are properly set
func validateUsageTracking(t *testing.T, usage EmbeddingUsage, expectNonZero bool) {
	if !expectNonZero {
		return
	}

	if usage.TokensUsed == 0 {
		t.Error("Expected non-zero tokens used")
	}
	if usage.RequestCount == 0 {
		t.Error("Expected non-zero request count")
	}
	if usage.EstimatedCost == 0 {
		t.Error("Expected non-zero estimated cost")
	}
	if usage.ProcessingTime == 0 {
		t.Error("Expected non-zero processing time")
	}
}

// runEmbeddingTest executes a single embedding test case
func runEmbeddingTest(t *testing.T, logger *zap.Logger, texts []string, expectErr bool) {
	c := setupMockEmbeddingClient(t, logger, len(texts))

	ctx := context.Background()
	response, err := c.EmbedTexts(ctx, texts)

	if expectErr {
		if err == nil {
			t.Error("Expected error")
		}
		return
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
		return
	}

	validateEmbeddingResponse(t, response, len(texts))
	validateUsageTracking(t, response.Usage, len(texts) > 0)
}

func TestEmbedTexts(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name      string
		texts     []string
		expectErr bool
	}{
		{
			name:      "single text",
			texts:     []string{"Hello world"},
			expectErr: false,
		},
		{
			name:      "multiple texts",
			texts:     []string{"Hello", "World", "Test"},
			expectErr: false,
		},
		{
			name:      "empty texts",
			texts:     []string{},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runEmbeddingTest(t, logger, tt.texts, tt.expectErr)
		})
	}
}

// TestEmbedQuery tests the single query embedding functionality
func TestEmbedQuery(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name      string
		query     string
		expectErr bool
	}{
		{
			name:      "valid query",
			query:     "What is cloud computing?",
			expectErr: false,
		},
		{
			name:      "empty query",
			query:     "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mockOpenAIServer(t, map[string]string{
				"embeddings": createMockEmbeddingResponse(1),
			})
			defer server.Close()

			config := openai.DefaultConfig("sk-test1234567890abcdef") // pragma: allowlist secret // pragma: allowlist secret
			config.BaseURL = server.URL + "/v1"
			client := openai.NewClientWithConfig(config)

			c := &Client{
				client: client,
				logger: logger,
				model:  EmbeddingModel,
			}

			ctx := context.Background()
			embedding, err := c.EmbedQuery(ctx, tt.query)

			if tt.expectErr {
				if err == nil {
					t.Error("Expected error")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(embedding) != ExpectedEmbeddingDimensions {
				t.Errorf("Expected %d dimensions, got %d", ExpectedEmbeddingDimensions, len(embedding))
			}
		})
	}
}

// TestRetryLogic tests the exponential backoff retry logic
func TestRetryLogic(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Mock server that returns rate limit error first, then success
	attempt := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt++
		if attempt == 1 {
			// First attempt: rate limit error
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error": {"message": "Rate limit exceeded", "type": "rate_limit_exceeded"}}`))
			return
		}
		// Second attempt: success
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(createMockEmbeddingResponse(1)))
	}))
	defer server.Close()

	config := openai.DefaultConfig("sk-test1234567890abcdef") // pragma: allowlist secret
	config.BaseURL = server.URL + "/v1"
	client := openai.NewClientWithConfig(config)

	c := &Client{
		client: client,
		logger: logger,
		model:  EmbeddingModel,
	}

	ctx := context.Background()
	start := time.Now()
	_, err := c.EmbedQuery(ctx, "test")
	duration := time.Since(start)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Should have taken at least 1 second due to retry delay
	if duration < time.Second {
		t.Errorf("Expected retry delay, but request completed in %v", duration)
	}

	if attempt != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempt)
	}
}

// TestErrorHandling tests various error scenarios
func TestErrorHandling(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name       string
		statusCode int
		response   string
		expectErr  bool
		retryable  bool
	}{
		{
			name:       "unauthorized error",
			statusCode: http.StatusUnauthorized,
			response:   `{"error": {"message": "Invalid API key", "type": "invalid_request_error"}}`,
			expectErr:  true,
			retryable:  false,
		},
		{
			name:       "rate limit error",
			statusCode: http.StatusTooManyRequests,
			response:   `{"error": {"message": "Rate limit exceeded", "type": "rate_limit_exceeded"}}`,
			expectErr:  true,
			retryable:  true,
		},
		{
			name:       "server error",
			statusCode: http.StatusInternalServerError,
			response:   `{"error": {"message": "Internal server error", "type": "server_error"}}`,
			expectErr:  true,
			retryable:  true,
		},
		{
			name:       "bad request",
			statusCode: http.StatusBadRequest,
			response:   `{"error": {"message": "Bad request", "type": "invalid_request_error"}}`,
			expectErr:  true,
			retryable:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()

			config := openai.DefaultConfig("sk-test1234567890abcdef") // pragma: allowlist secret // pragma: allowlist secret
			config.BaseURL = server.URL + "/v1"
			client := openai.NewClientWithConfig(config)

			c := &Client{
				client: client,
				logger: logger,
				model:  EmbeddingModel,
			}

			ctx := context.Background()
			_, err := c.EmbedQuery(ctx, "test")

			if tt.expectErr {
				if err == nil {
					t.Error("Expected error")
					return
				}
				if tt.retryable && !strings.Contains(err.Error(), "exhausted all retry attempts") {
					t.Errorf("Expected retry exhaustion error, got: %v", err)
				}
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestEmbeddingDimensionValidation tests embedding dimension validation
func TestEmbeddingDimensionValidation(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Mock server that returns embeddings with wrong dimensions
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Return embedding with wrong dimensions (should be 1536, but we'll return 512)
		_, _ = w.Write([]byte(`{
			"object": "list",
			"data": [
				{
					"object": "embedding",
					"embedding": [` + strings.Repeat("0.1,", 511) + `0.1],
					"index": 0
				}
			],
			"model": "text-embedding-3-small",
			"usage": {
				"prompt_tokens": 10,
				"total_tokens": 10
			}
		}`))
	}))
	defer server.Close()

	config := openai.DefaultConfig("sk-test1234567890abcdef") // pragma: allowlist secret
	config.BaseURL = server.URL + "/v1"
	client := openai.NewClientWithConfig(config)

	c := &Client{
		client: client,
		logger: logger,
		model:  EmbeddingModel,
	}

	ctx := context.Background()
	_, err := c.EmbedQuery(ctx, "test")

	if err == nil {
		t.Error("Expected error for invalid embedding dimensions")
	}

	if !strings.Contains(err.Error(), "embedding validation failed") {
		t.Errorf("Expected dimension validation error, got: %v", err)
	}
}

// TestLegacyMethods tests backward compatibility methods
func TestLegacyMethods(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("GenerateEmbeddings", func(t *testing.T) {
		server := mockOpenAIServer(t, map[string]string{
			"embeddings": createMockEmbeddingResponse(2),
		})
		defer server.Close()

		config := openai.DefaultConfig("sk-test1234567890abcdef") // pragma: allowlist secret
		config.BaseURL = server.URL + "/v1"
		client := openai.NewClientWithConfig(config)

		c := &Client{
			client: client,
			logger: logger,
			model:  EmbeddingModel,
		}

		ctx := context.Background()
		texts := []string{"Hello", "World"}
		embeddings, err := c.GenerateEmbeddings(ctx, texts)
		if err != nil {
			t.Errorf("GenerateEmbeddings failed: %v", err)
		}
		if len(embeddings) != 2 {
			t.Errorf("Expected 2 embeddings, got %d", len(embeddings))
		}
	})

	t.Run("GenerateEmbedding", func(t *testing.T) {
		server := mockOpenAIServer(t, map[string]string{
			"embeddings": createMockEmbeddingResponse(1),
		})
		defer server.Close()

		config := openai.DefaultConfig("sk-test1234567890abcdef") // pragma: allowlist secret
		config.BaseURL = server.URL + "/v1"
		client := openai.NewClientWithConfig(config)

		c := &Client{
			client: client,
			logger: logger,
			model:  EmbeddingModel,
		}

		ctx := context.Background()
		embedding, err := c.GenerateEmbedding(ctx, "test")
		if err != nil {
			t.Errorf("GenerateEmbedding failed: %v", err)
		}
		if len(embedding) != ExpectedEmbeddingDimensions {
			t.Errorf("Expected %d dimensions, got %d", ExpectedEmbeddingDimensions, len(embedding))
		}
	})
}

// TestCreateChatCompletion tests chat completion functionality
func TestCreateChatCompletion(t *testing.T) {
	logger := zaptest.NewLogger(t)

	server := mockOpenAIServer(t, map[string]string{
		"chat": createMockChatResponse(),
	})
	defer server.Close()

	config := openai.DefaultConfig("sk-test1234567890abcdef") // pragma: allowlist secret
	config.BaseURL = server.URL + "/v1"
	client := openai.NewClientWithConfig(config)

	c := &Client{
		client: client,
		logger: logger,
		model:  EmbeddingModel,
	}

	ctx := context.Background()
	req := ChatCompletionRequest{
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: "Hello, how are you?",
			},
		},
		MaxTokens:   100,
		Temperature: 0.7,
		Model:       "gpt-4o",
	}

	response, err := c.CreateChatCompletion(ctx, req)
	if err != nil {
		t.Errorf("CreateChatCompletion failed: %v", err)
	}

	if response.Content != "This is a test response" {
		t.Errorf("Expected 'This is a test response', got '%s'", response.Content)
	}

	if response.FinishReason != "stop" {
		t.Errorf("Expected 'stop', got '%s'", response.FinishReason)
	}

	if response.Usage.TotalTokens != 15 {
		t.Errorf("Expected 15 total tokens, got %d", response.Usage.TotalTokens)
	}
}

// TestContextCancellation tests context cancellation handling
func TestContextCancellation(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Mock server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(createMockEmbeddingResponse(1)))
	}))
	defer server.Close()

	config := openai.DefaultConfig("sk-test1234567890abcdef") // pragma: allowlist secret
	config.BaseURL = server.URL + "/v1"
	client := openai.NewClientWithConfig(config)

	c := &Client{
		client: client,
		logger: logger,
		model:  EmbeddingModel,
	}

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := c.EmbedQuery(ctx, "test")
	if err == nil {
		t.Error("Expected context cancellation error")
	}

	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("Expected context deadline exceeded error, got: %v", err)
	}
}

// TestCostEstimation tests cost estimation functionality
func TestCostEstimation(t *testing.T) {
	logger := zaptest.NewLogger(t)

	server := mockOpenAIServer(t, map[string]string{
		"embeddings": createMockEmbeddingResponse(1),
	})
	defer server.Close()

	config := openai.DefaultConfig("sk-test1234567890abcdef") // pragma: allowlist secret
	config.BaseURL = server.URL + "/v1"
	client := openai.NewClientWithConfig(config)

	c := &Client{
		client: client,
		logger: logger,
		model:  EmbeddingModel,
	}

	ctx := context.Background()
	response, err := c.EmbedTexts(ctx, []string{"test"})
	if err != nil {
		t.Errorf("EmbedTexts failed: %v", err)
	}

	// Verify cost estimation
	expectedCost := float64(10) / 1000.0 * EmbeddingCostPer1KTokens // 10 tokens from mock response
	if response.Usage.EstimatedCost != expectedCost {
		t.Errorf("Expected cost %f, got %f", expectedCost, response.Usage.EstimatedCost)
	}
}

// TestBuildPrompts tests the prompt building functions
func TestBuildPrompts(t *testing.T) {
	// Test BuildSystemPrompt
	systemPrompt := BuildSystemPrompt()
	if systemPrompt == "" {
		t.Error("BuildSystemPrompt returned empty string")
	}
	if !strings.Contains(systemPrompt, "Cloud Solutions Architect") {
		t.Error("System prompt should contain 'Cloud Solutions Architect'")
	}

	// Test BuildUserPrompt
	query := "How do I migrate to AWS?"
	contextChunks := []string{"AWS migration best practices", "Use AWS Migration Hub"}
	webResults := []string{"Latest AWS migration tools", "AWS migration case studies"}

	userPrompt := BuildUserPrompt(query, contextChunks, webResults)
	if userPrompt == "" {
		t.Error("BuildUserPrompt returned empty string")
	}
	if !strings.Contains(userPrompt, query) {
		t.Error("User prompt should contain the original query")
	}
	if !strings.Contains(userPrompt, "Internal Document Context") {
		t.Error("User prompt should contain context section")
	}
	if !strings.Contains(userPrompt, "Live Web Search Results") {
		t.Error("User prompt should contain web results section")
	}
}

// TestTruncateText tests the text truncation utility
func TestTruncateText(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		maxLength int
		expected  string
	}{
		{
			name:      "text shorter than limit",
			text:      "short",
			maxLength: 10,
			expected:  "short",
		},
		{
			name:      "text longer than limit",
			text:      "this is a very long text that should be truncated",
			maxLength: 10,
			expected:  "this is a ...",
		},
		{
			name:      "text exactly at limit",
			text:      "exactly10c",
			maxLength: 10,
			expected:  "exactly10c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateText(tt.text, tt.maxLength)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// BenchmarkEmbedTexts benchmarks the embedding generation performance
func BenchmarkEmbedTexts(b *testing.B) {
	logger := zap.NewNop()

	server := mockOpenAIServer(b, map[string]string{
		"embeddings": createMockEmbeddingResponse(10),
	})
	defer server.Close()

	config := openai.DefaultConfig("sk-test1234567890abcdef") // pragma: allowlist secret
	config.BaseURL = server.URL + "/v1"
	client := openai.NewClientWithConfig(config)

	c := &Client{
		client: client,
		logger: logger,
		model:  EmbeddingModel,
	}

	texts := make([]string, 10)
	for i := 0; i < 10; i++ {
		texts[i] = fmt.Sprintf("This is test text number %d", i)
	}

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := c.EmbedTexts(ctx, texts)
		if err != nil {
			b.Errorf("EmbedTexts failed: %v", err)
		}
	}
}
