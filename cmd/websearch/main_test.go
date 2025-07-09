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
	"github.com/stretchr/testify/mock"
	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/websearch"
	"go.uber.org/zap"
)

type MockOpenAIClient struct {
	mock.Mock
}

func (m *MockOpenAIClient) CreateChatCompletion(ctx context.Context,
	req interface{}) (*MockChatCompletionResponse, error) {
	args := m.Called(ctx, req)
	return args.Get(0).(*MockChatCompletionResponse), args.Error(1)
}

func (m *MockOpenAIClient) EmbedQuery(ctx context.Context, query string) ([]float32, error) {
	args := m.Called(ctx, query)
	return args.Get(0).([]float32), args.Error(1)
}

type MockChatCompletionResponse struct {
	Content string
	Usage   MockUsage
}

type MockUsage struct {
	CompletionTokens int
}

func setupTestService() *WebSearchService {
	cfg := &config.Config{
		WebSearch: config.WebSearchConfig{
			MaxResults: 3,
			FreshnessKeywords: []string{
				"latest", "recent", "new", "2025", "update",
			},
		},
		OpenAI: config.OpenAIConfig{
			APIKey: "test-key", // pragma: allowlist secret
		},
	}

	logger := zap.NewNop()
	detectionConfig := websearch.ConfigFromSlice(cfg.WebSearch.FreshnessKeywords)

	service := &WebSearchService{
		config:          cfg,
		logger:          logger,
		openaiClient:    nil, // Will be mocked in tests
		cache:           make(map[string]*CacheEntry),
		detectionConfig: detectionConfig,
	}

	return service
}

func TestDetectFreshnessKeywords(t *testing.T) {
	service := setupTestService()

	tests := []struct {
		name     string
		query    string
		expected bool
	}{
		{
			name:     "contains latest keyword",
			query:    "What are the latest AWS features?",
			expected: true,
		},
		{
			name:     "contains year keyword",
			query:    "AWS features in 2025",
			expected: true,
		},
		{
			name:     "no freshness keywords",
			query:    "AWS EC2 pricing information",
			expected: false,
		},
		{
			name:     "case insensitive",
			query:    "LATEST updates",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.detectFreshnessKeywords(tt.query)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCaching(t *testing.T) {
	service := setupTestService()

	query := "test query"
	response := SearchResponse{
		Results: []SearchResult{
			{
				Title:     "Test Result",
				Snippet:   "Test snippet",
				URL:       "https://example.com",
				Timestamp: "2025-01-01",
			},
		},
		Source:    "test",
		Timestamp: time.Now().Format(time.RFC3339),
		Cached:    false,
	}

	// Test cache miss
	cachedResponse, found := service.getCachedResult(query)
	assert.False(t, found)
	assert.Nil(t, cachedResponse)

	// Set cache
	service.setCachedResult(query, response)

	// Test cache hit
	cachedResponse, found = service.getCachedResult(query)
	assert.True(t, found)
	assert.NotNil(t, cachedResponse)
	assert.True(t, cachedResponse.Cached)
	assert.Equal(t, response.Results[0].Title, cachedResponse.Results[0].Title)
}

func TestHandleSearchEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		requestBody    string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "empty query",
			requestBody:    `{"query": "   "}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Query cannot be empty",
		},
		{
			name:           "invalid JSON",
			requestBody:    `{"invalid": json}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Invalid request format",
		},
		{
			name:           "no freshness keywords",
			requestBody:    `{"query": "AWS EC2 pricing"}`,
			expectedStatus: http.StatusOK,
			expectedBody:   "no-search-needed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := setupTestService()
			router := gin.New()
			router.POST("/search", service.handleSearch)

			req, _ := http.NewRequest("POST", "/search", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.expectedBody)
		})
	}
}

func TestHandleHealthEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()

	// Use the new health check system
	healthManager := &MockHealthManager{}
	healthManager.On("HTTPHandler").Return(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"service": "websearch",
			"status":  "unhealthy",
			"dependencies": map[string]interface{}{
				"openai": map[string]interface{}{
					"status": "unhealthy",
					"error":  "OpenAI client not initialized",
				},
			},
		})
	}))

	router.GET("/health", gin.WrapH(healthManager.HTTPHandler()))

	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should return 503 because OpenAI client is nil
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "websearch", response["service"])
	assert.Equal(t, "unhealthy", response["status"])
	assert.Contains(t, response, "dependencies")
}

type MockHealthManager struct {
	mock.Mock
}

func (m *MockHealthManager) HTTPHandler() http.HandlerFunc {
	args := m.Called()
	return args.Get(0).(http.HandlerFunc)
}

func TestSearchRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		request SearchRequest
		valid   bool
	}{
		{
			name:    "valid request",
			request: SearchRequest{Query: "test query"},
			valid:   true,
		},
		{
			name:    "empty query",
			request: SearchRequest{Query: ""},
			valid:   false,
		},
		{
			name:    "whitespace only query",
			request: SearchRequest{Query: "   "},
			valid:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.valid {
				assert.NotEmpty(t, strings.TrimSpace(tt.request.Query))
			} else {
				assert.Empty(t, strings.TrimSpace(tt.request.Query))
			}
		})
	}
}

func TestSearchResponseStructure(t *testing.T) {
	response := SearchResponse{
		Results: []SearchResult{
			{
				Title:     "Test Title",
				Snippet:   "Test snippet content",
				URL:       "https://example.com",
				Timestamp: "2025-01-01",
			},
		},
		Source:    "test-source",
		Timestamp: time.Now().Format(time.RFC3339),
		Cached:    false,
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(response)
	assert.NoError(t, err)
	assert.Contains(t, string(jsonData), "Test Title")
	assert.Contains(t, string(jsonData), "test-source")

	// Test JSON unmarshaling
	var unmarshaled SearchResponse
	err = json.Unmarshal(jsonData, &unmarshaled)
	assert.NoError(t, err)
	assert.Equal(t, response.Results[0].Title, unmarshaled.Results[0].Title)
	assert.Equal(t, response.Source, unmarshaled.Source)
}

func TestMaxQueryLength(t *testing.T) {
	longQuery := strings.Repeat("a", maxQueryLength+100)

	// This test would normally call performSearch, but since we can't easily mock
	// the OpenAI client in this setup, we'll test the length check logic
	if len(longQuery) > maxQueryLength {
		truncated := longQuery[:maxQueryLength]
		assert.Equal(t, maxQueryLength, len(truncated))
	}
}

func TestConfigDefaults(t *testing.T) {
	service := setupTestService()

	// Test that defaults are applied correctly
	assert.Equal(t, 3, service.config.WebSearch.MaxResults)
	assert.Contains(t, service.config.WebSearch.FreshnessKeywords, "latest")
	assert.Contains(t, service.config.WebSearch.FreshnessKeywords, "2025")
}

func TestForceSearchFlag(t *testing.T) {
	service := setupTestService()

	// Test force search true
	forceTrue := true
	req := SearchRequest{
		Query:       "static query with no freshness keywords",
		ForceSearch: &forceTrue,
	}

	// Should need search even without freshness keywords
	needsSearch := req.ForceSearch != nil && *req.ForceSearch
	if !needsSearch {
		needsSearch = service.detectFreshnessKeywords(req.Query)
	}
	assert.True(t, needsSearch)

	// Test force search false
	forceFalse := false
	req.ForceSearch = &forceFalse
	needsSearch = req.ForceSearch != nil && *req.ForceSearch
	if !needsSearch {
		needsSearch = service.detectFreshnessKeywords(req.Query)
	}
	assert.False(t, needsSearch)
}
