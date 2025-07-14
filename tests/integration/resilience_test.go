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

// Package integration provides integration tests for resilience patterns
// across the AI SA Assistant services, including fallback pipelines and
// external service failure simulation.
package integration

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"github.com/your-org/ai-sa-assistant/internal/resilience"
)

// MockChromaClient simulates ChromaDB behavior for testing
type MockChromaClient struct {
	mock.Mock
	shouldFail bool
	failCount  int
}

func (m *MockChromaClient) Query(ctx context.Context, query string, limit int) ([]map[string]interface{}, error) {
	args := m.Called(ctx, query, limit)
	if m.shouldFail {
		m.failCount++
		return nil, errors.New("ChromaDB connection failed")
	}
	return args.Get(0).([]map[string]interface{}), args.Error(1)
}

func (m *MockChromaClient) SetShouldFail(shouldFail bool) {
	m.shouldFail = shouldFail
	m.failCount = 0
}

func (m *MockChromaClient) GetFailCount() int {
	return m.failCount
}

// MockMetadataStore simulates SQLite metadata behavior for testing
type MockMetadataStore struct {
	mock.Mock
	shouldFail bool
	failCount  int
}

func (m *MockMetadataStore) FilterDocuments(ctx context.Context, filters map[string]interface{}) ([]string, error) {
	args := m.Called(ctx, filters)
	if m.shouldFail {
		m.failCount++
		return nil, errors.New("SQLite database lock timeout")
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockMetadataStore) SetShouldFail(shouldFail bool) {
	m.shouldFail = shouldFail
	m.failCount = 0
}

func (m *MockMetadataStore) GetFailCount() int {
	return m.failCount
}

// MockWebSearchClient simulates web search behavior for testing
type MockWebSearchClient struct {
	mock.Mock
	shouldFail bool
	failCount  int
}

func (m *MockWebSearchClient) Search(ctx context.Context, query string, maxResults int) ([]map[string]interface{}, error) {
	args := m.Called(ctx, query, maxResults)
	if m.shouldFail {
		m.failCount++
		return nil, errors.New("web search service unavailable")
	}
	return args.Get(0).([]map[string]interface{}), args.Error(1)
}

func (m *MockWebSearchClient) SetShouldFail(shouldFail bool) {
	m.shouldFail = shouldFail
	m.failCount = 0
}

func (m *MockWebSearchClient) GetFailCount() int {
	return m.failCount
}

// RetrievalPipeline simulates the hybrid retrieval pipeline
type RetrievalPipeline struct {
	chromaClient    *MockChromaClient
	metadataStore   *MockMetadataStore
	webSearchClient *MockWebSearchClient
	circuitBreaker  *resilience.CircuitBreaker
	logger          *zap.Logger
}

func NewRetrievalPipeline(logger *zap.Logger) *RetrievalPipeline {
	config := resilience.DefaultCircuitBreakerConfig("retrieval")
	config.MaxFailures = 3
	config.ResetTimeout = 1 * time.Minute

	return &RetrievalPipeline{
		chromaClient:    &MockChromaClient{},
		metadataStore:   &MockMetadataStore{},
		webSearchClient: &MockWebSearchClient{},
		circuitBreaker:  resilience.NewCircuitBreaker(config, logger),
		logger:          logger,
	}
}

func (rp *RetrievalPipeline) Search(ctx context.Context, query string, filters map[string]interface{}) ([]map[string]interface{}, error) {
	var results []map[string]interface{}

	// Phase 1: Metadata filtering
	_, err := rp.metadataStore.FilterDocuments(ctx, filters)
	if err != nil {
		rp.logger.Warn("Metadata filtering failed, falling back to direct vector search", zap.Error(err))
		// Fallback to broader search - metadata filtering failed
	}

	// Phase 2: Vector search with circuit breaker
	err = rp.circuitBreaker.Execute(ctx, func(ctx context.Context) error {
		var searchErr error
		results, searchErr = rp.chromaClient.Query(ctx, query, 10)
		return searchErr
	})

	if err != nil {
		rp.logger.Warn("Vector search failed", zap.Error(err))

		// Phase 3: Fallback to web search if freshness detected
		if rp.detectFreshness(query) {
			rp.logger.Info("Freshness detected, falling back to web search")
			webResults, webErr := rp.webSearchClient.Search(ctx, query, 3)
			if webErr != nil {
				rp.logger.Error("Web search also failed", zap.Error(webErr))
				return nil, resilience.NewServiceUnavailableError("All search methods failed", err)
			}
			return webResults, nil
		}

		return nil, resilience.NewServiceUnavailableError("Vector search failed and no fallback available", err)
	}

	// Phase 4: Check if results are insufficient
	if len(results) < 3 {
		rp.logger.Info("Insufficient results, attempting broader search")

		// Remove docID filtering for broader search
		broaderResults, broaderErr := rp.chromaClient.Query(ctx, query, 10)
		if broaderErr == nil && len(broaderResults) > len(results) {
			results = broaderResults
		}
	}

	return results, nil
}

func (rp *RetrievalPipeline) detectFreshness(query string) bool {
	keywords := []string{"2025", "latest", "recent", "Q1", "Q2", "Q3", "Q4"}
	queryLower := strings.ToLower(query)

	for _, keyword := range keywords {
		if strings.Contains(queryLower, keyword) {
			return true
		}
	}
	return false
}

// TestFallbackPipeline tests the complete fallback pipeline
func TestFallbackPipeline(t *testing.T) {
	logger := zap.NewNop()
	pipeline := NewRetrievalPipeline(logger)

	// Setup mock expectations for successful case
	pipeline.chromaClient.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(
		[]map[string]interface{}{
			{"id": "doc1", "content": "result1"},
			{"id": "doc2", "content": "result2"},
			{"id": "doc3", "content": "result3"},
		}, nil)

	pipeline.metadataStore.On("FilterDocuments", mock.Anything, mock.Anything).Return(
		[]string{"doc1", "doc2", "doc3"}, nil)

	ctx := context.Background()
	results, err := pipeline.Search(ctx, "test query", map[string]interface{}{"scenario": "migration"})

	assert.NoError(t, err)
	assert.Len(t, results, 3)

	// Verify mocks were called
	pipeline.chromaClient.AssertExpectations(t)
	pipeline.metadataStore.AssertExpectations(t)
}

// TestMetadataFilteringFailure tests metadata filtering failure with fallback
func TestMetadataFilteringFailure(t *testing.T) {
	logger := zap.NewNop()
	pipeline := NewRetrievalPipeline(logger)

	// Setup metadata store to fail
	pipeline.metadataStore.SetShouldFail(true)
	pipeline.metadataStore.On("FilterDocuments", mock.Anything, mock.Anything).Return(
		[]string{}, errors.New("database locked"))

	// Setup ChromaDB to succeed
	pipeline.chromaClient.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(
		[]map[string]interface{}{
			{"id": "doc1", "content": "result1"},
		}, nil)

	ctx := context.Background()
	results, err := pipeline.Search(ctx, "test query", map[string]interface{}{"scenario": "migration"})

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, 1, pipeline.metadataStore.GetFailCount())

	// Verify fallback was used
	pipeline.chromaClient.AssertExpectations(t)
	pipeline.metadataStore.AssertExpectations(t)
}

// TestChromaDBFailureWithWebSearchFallback tests ChromaDB failure with web search fallback
func TestChromaDBFailureWithWebSearchFallback(t *testing.T) {
	logger := zap.NewNop()
	pipeline := NewRetrievalPipeline(logger)

	// Setup ChromaDB to fail
	pipeline.chromaClient.SetShouldFail(true)
	pipeline.chromaClient.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(
		[]map[string]interface{}{}, errors.New("connection refused"))

	// Setup metadata store to succeed
	pipeline.metadataStore.On("FilterDocuments", mock.Anything, mock.Anything).Return(
		[]string{"doc1", "doc2"}, nil)

	// Setup web search to succeed
	pipeline.webSearchClient.On("Search", mock.Anything, mock.Anything, mock.Anything).Return(
		[]map[string]interface{}{
			{"url": "https://example.com", "title": "Latest Updates"},
		}, nil)

	ctx := context.Background()
	// Use query with freshness keyword to trigger web search fallback
	results, err := pipeline.Search(ctx, "latest AWS updates 2025", map[string]interface{}{"scenario": "migration"})

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Contains(t, results[0]["url"], "https://example.com")

	// Verify web search was used as fallback
	pipeline.webSearchClient.AssertExpectations(t)
}

// TestInsufficientResultsBroaderSearch tests fallback to broader search when results are insufficient
func TestInsufficientResultsBroaderSearch(t *testing.T) {
	logger := zap.NewNop()
	pipeline := NewRetrievalPipeline(logger)

	// Setup metadata store to succeed
	pipeline.metadataStore.On("FilterDocuments", mock.Anything, mock.Anything).Return(
		[]string{"doc1", "doc2"}, nil)

	// Setup ChromaDB to return insufficient results on first call, more on second
	pipeline.chromaClient.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(
		[]map[string]interface{}{
			{"id": "doc1", "content": "result1"},
		}, nil).Once()

	pipeline.chromaClient.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(
		[]map[string]interface{}{
			{"id": "doc1", "content": "result1"},
			{"id": "doc2", "content": "result2"},
			{"id": "doc3", "content": "result3"},
			{"id": "doc4", "content": "result4"},
		}, nil).Once()

	ctx := context.Background()
	results, err := pipeline.Search(ctx, "test query", map[string]interface{}{"scenario": "migration"})

	assert.NoError(t, err)
	assert.Len(t, results, 4) // Should get broader results

	// Verify ChromaDB was called twice (initial + broader search)
	pipeline.chromaClient.AssertExpectations(t)
}

// TestCompleteSystemFailure tests graceful degradation when all systems fail
func TestCompleteSystemFailure(t *testing.T) {
	logger := zap.NewNop()
	pipeline := NewRetrievalPipeline(logger)

	// Setup all systems to fail
	pipeline.metadataStore.SetShouldFail(true)
	pipeline.metadataStore.On("FilterDocuments", mock.Anything, mock.Anything).Return(
		[]string{}, errors.New("database error"))

	pipeline.chromaClient.SetShouldFail(true)
	pipeline.chromaClient.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(
		[]map[string]interface{}{}, errors.New("connection failed"))

	pipeline.webSearchClient.SetShouldFail(true)
	pipeline.webSearchClient.On("Search", mock.Anything, mock.Anything, mock.Anything).Return(
		[]map[string]interface{}{}, errors.New("service unavailable"))

	ctx := context.Background()
	results, err := pipeline.Search(ctx, "latest test query 2025", map[string]interface{}{"scenario": "migration"})

	assert.Error(t, err)
	assert.Nil(t, results)

	// Verify error contains expected information (ServiceUnavailableError is used by default)
	assert.Contains(t, err.Error(), "All search methods failed")
}

// TestCircuitBreakerIntegration tests circuit breaker integration with retry logic
func TestCircuitBreakerIntegration(t *testing.T) {
	logger := zap.NewNop()
	pipeline := NewRetrievalPipeline(logger)

	// Setup ChromaDB to fail consistently to trigger circuit breaker
	pipeline.chromaClient.SetShouldFail(true)
	pipeline.chromaClient.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(
		[]map[string]interface{}{}, errors.New("connection failed"))

	// Setup metadata store to succeed
	pipeline.metadataStore.On("FilterDocuments", mock.Anything, mock.Anything).Return(
		[]string{"doc1", "doc2"}, nil)

	ctx := context.Background()

	// Make multiple requests to trigger circuit breaker
	for i := 0; i < 5; i++ {
		_, err := pipeline.Search(ctx, "test query", map[string]interface{}{"scenario": "migration"})
		assert.Error(t, err)
	}

	// Verify circuit breaker is now open
	assert.Equal(t, resilience.CircuitOpen, pipeline.circuitBreaker.GetState())

	// Next request should fail fast
	start := time.Now()
	_, err := pipeline.Search(ctx, "test query", map[string]interface{}{"scenario": "migration"})
	duration := time.Since(start)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, resilience.ErrCircuitBreakerOpen) ||
		strings.Contains(err.Error(), "circuit breaker"))

	// Should fail fast (much quicker than normal request)
	assert.Less(t, duration, 100*time.Millisecond)
}

// TestExternalServiceFailureSimulation tests various external service failures
func TestExternalServiceFailureSimulation(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func(*RetrievalPipeline)
		expectedError string
		expectedCode  resilience.ErrorCode
	}{
		{
			name: "OpenAI API rate limit",
			setupFunc: func(pipeline *RetrievalPipeline) {
				pipeline.chromaClient.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(
					[]map[string]interface{}{}, errors.New("rate limit exceeded"))
			},
			expectedError: "search failed",
			expectedCode:  resilience.ErrorCodeServiceUnavailable,
		},
		{
			name: "OpenAI API timeout",
			setupFunc: func(pipeline *RetrievalPipeline) {
				pipeline.chromaClient.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(
					[]map[string]interface{}{}, errors.New("request timeout"))
			},
			expectedError: "search failed",
			expectedCode:  resilience.ErrorCodeServiceUnavailable,
		},
		{
			name: "OpenAI API server error",
			setupFunc: func(pipeline *RetrievalPipeline) {
				pipeline.chromaClient.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(
					[]map[string]interface{}{}, errors.New("internal server error"))
			},
			expectedError: "search failed",
			expectedCode:  resilience.ErrorCodeServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zap.NewNop()
			pipeline := NewRetrievalPipeline(logger)

			// Setup metadata store to succeed
			pipeline.metadataStore.On("FilterDocuments", mock.Anything, mock.Anything).Return(
				[]string{"doc1", "doc2"}, nil)

			// Setup specific failure condition
			tt.setupFunc(pipeline)

			ctx := context.Background()
			_, err := pipeline.Search(ctx, "test query", map[string]interface{}{"scenario": "migration"})

			assert.Error(t, err)

			// Verify error is properly wrapped
			errorHandler := resilience.NewErrorHandler(logger)
			serviceErr := errorHandler.WrapError(err, "searching documents")
			assert.Contains(t, strings.ToLower(serviceErr.Message), strings.ToLower(tt.expectedError))
			assert.Equal(t, tt.expectedCode, serviceErr.Code)
		})
	}
}

// TestTeamsWebhookFailure tests Teams webhook delivery failure handling
func TestTeamsWebhookFailure(t *testing.T) {
	// Create a test server that simulates Teams webhook failures
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Simulate webhook failure
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("Bad Gateway"))
	}))
	defer server.Close()

	logger := zap.NewNop()
	config := resilience.DefaultBackoffConfig()
	config.BaseDelay = 10 * time.Millisecond
	config.MaxRetries = 2

	client := &http.Client{Timeout: 5 * time.Second}

	// Simulate webhook POST with retry logic
	attempts := 0
	err := resilience.WithExponentialBackoff(context.Background(), logger, config, func(ctx context.Context) error {
		attempts++

		req, err := http.NewRequestWithContext(ctx, "POST", server.URL, strings.NewReader("test payload"))
		if err != nil {
			return err
		}

		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return errors.New("webhook delivery failed")
		}

		return nil
	})

	assert.Error(t, err)
	assert.Equal(t, 3, attempts) // Initial + 2 retries
	assert.Contains(t, err.Error(), "webhook delivery failed")
}
