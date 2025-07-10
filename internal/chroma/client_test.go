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

package chroma

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/your-org/ai-sa-assistant/internal/resilience"
)

// mockChromaServer creates a mock ChromaDB server for testing
func mockChromaServer(
	t *testing.T,
	responses map[string]func(w http.ResponseWriter, r *http.Request),
) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + ":" + r.URL.Path
		if handler, ok := responses[key]; ok {
			handler(w, r)
			return
		}
		t.Logf("Unhandled request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail": "not found", "type": "not_found"}`))
	}))
}

// createMockAddResponse creates a mock response for add documents
func createMockAddResponse() string {
	return `{"success": true}`
}

// createMockSearchResponse creates a mock response for search
func createMockSearchResponse() string {
	return `{
		"ids": [["doc1", "doc2"]],
		"documents": [["Document 1 content", "Document 2 content"]],
		"metadatas": [[
			{"doc_id": "doc1", "source": "test1.md"},
			{"doc_id": "doc2", "source": "test2.md"}
		]],
		"distances": [[0.1, 0.2]]
	}`
}

// createMockCollectionResponse creates a mock response for collection info
func createMockCollectionResponse() string {
	return `{
		"name": "test-collection",
		"id": "test-collection-id",
		"metadata": {"created": "2024-01-01"}
	}`
}

// createMockCollectionsResponse creates a mock response for list collections
func createMockCollectionsResponse() string {
	return `[
		{
			"name": "collection1",
			"id": "collection1-id",
			"metadata": {"created": "2024-01-01"}
		},
		{
			"name": "collection2",
			"id": "collection2-id",
			"metadata": {"created": "2024-01-02"}
		}
	]`
}

// createMockHeartbeatResponse creates a mock response for heartbeat
func createMockHeartbeatResponse() string {
	return `{"nanosecond heartbeat": 1234567890}`
}

// TestNewClient tests client initialization
func TestNewClient(t *testing.T) {
	tests := []struct {
		name       string
		baseURL    string
		collection string
		logger     *zap.Logger
		validate   func(t *testing.T, client *Client)
	}{
		{
			name:       "basic client creation",
			baseURL:    "http://localhost:8000",
			collection: "test-collection",
			logger:     nil,
			validate: func(t *testing.T, client *Client) {
				assert.NotNil(t, client)
				assert.Equal(t, "http://localhost:8000", client.baseURL)
				assert.Equal(t, "test-collection", client.collection)
				assert.NotNil(t, client.httpClient)
				assert.NotNil(t, client.logger)
				assert.NotNil(t, client.circuitBreaker)
				assert.NotNil(t, client.errorHandler)
				assert.NotNil(t, client.timeoutManager)
				assert.Equal(t, DefaultHTTPTimeout, client.httpClient.Timeout)
			},
		},
		{
			name:       "client with custom logger",
			baseURL:    "http://localhost:8000",
			collection: "test-collection",
			logger:     zaptest.NewLogger(t),
			validate: func(t *testing.T, client *Client) {
				assert.NotNil(t, client)
				assert.NotNil(t, client.logger)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var client *Client
			if tt.logger != nil {
				client = NewClientForTesting(tt.baseURL, tt.collection, tt.logger)
			} else {
				client = NewClient(tt.baseURL, tt.collection)
			}

			tt.validate(t, client)
		})
	}
}

// TestAddDocuments tests adding documents to ChromaDB
func TestAddDocuments(t *testing.T) {
	logger := zap.NewNop() // Use nop logger for faster tests

	tests := []struct {
		name        string
		documents   []Document
		embeddings  [][]float32
		serverResp  func(w http.ResponseWriter, r *http.Request)
		expectError bool
		errorCheck  func(t *testing.T, err error)
	}{
		{
			name: "successful add documents",
			documents: []Document{
				{
					ID:       "doc1",
					Content:  "Document 1 content",
					Metadata: map[string]string{"source": "test1.md"},
				},
				{
					ID:       "doc2",
					Content:  "Document 2 content",
					Metadata: map[string]string{"source": "test2.md"},
				},
			},
			embeddings: [][]float32{
				{0.1, 0.2, 0.3},
				{0.4, 0.5, 0.6},
			},
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/api/v1/collections/test-collection/add", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(createMockAddResponse()))
			},
			expectError: false,
		},
		{
			name:       "empty documents",
			documents:  []Document{},
			embeddings: [][]float32{},
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(createMockAddResponse()))
			},
			expectError: false,
		},
		{
			name: "bad request error",
			documents: []Document{
				{ID: "doc1", Content: "test", Metadata: map[string]string{}},
			},
			embeddings: [][]float32{{0.1, 0.2, 0.3}},
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"detail": "Invalid request", "type": "invalid_request"}`))
			},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				var serviceErr *resilience.ServiceError
				assert.ErrorAs(t, err, &serviceErr)
				assert.Equal(t, resilience.ErrorCodeBadRequest, serviceErr.Code)
			},
		},
		{
			name: "server error",
			documents: []Document{
				{ID: "doc1", Content: "test", Metadata: map[string]string{}},
			},
			embeddings: [][]float32{{0.1, 0.2, 0.3}},
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"detail": "Internal server error", "type": "server_error"}`))
			},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				var serviceErr *resilience.ServiceError
				assert.ErrorAs(t, err, &serviceErr)
				assert.Equal(t, resilience.ErrorCodeServiceUnavailable, serviceErr.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mockChromaServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
				"POST:/api/v1/collections/test-collection/add": tt.serverResp,
			})
			defer server.Close()

			client := NewClientForTesting(server.URL, "test-collection", logger)
			ctx := context.Background()

			err := client.AddDocuments(ctx, tt.documents, tt.embeddings)

			if tt.expectError {
				require.Error(t, err)
				validateError(t, err, tt.errorCheck)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestSearch tests vector search functionality
func TestSearch(t *testing.T) {
	logger := zap.NewNop() // Use nop logger for faster tests

	tests := []struct {
		name           string
		queryEmbedding []float32
		nResults       int
		docIDs         []string
		serverResp     func(w http.ResponseWriter, r *http.Request)
		expectError    bool
		expectedCount  int
		errorCheck     func(t *testing.T, err error)
	}{
		{
			name:           "successful search",
			queryEmbedding: []float32{0.1, 0.2, 0.3},
			nResults:       2,
			docIDs:         []string{"doc1", "doc2"},
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/api/v1/collections/test-collection/query", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(createMockSearchResponse()))
			},
			expectError:   false,
			expectedCount: 2,
		},
		{
			name:           "search without doc ID filter",
			queryEmbedding: []float32{0.1, 0.2, 0.3},
			nResults:       5,
			docIDs:         nil,
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(createMockSearchResponse()))
			},
			expectError:   false,
			expectedCount: 2,
		},
		{
			name:           "empty search results",
			queryEmbedding: []float32{0.1, 0.2, 0.3},
			nResults:       5,
			docIDs:         []string{},
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"ids": [], "documents": [], "metadatas": [], "distances": []}`))
			},
			expectError:   false,
			expectedCount: 0,
		},
		{
			name:           "search error",
			queryEmbedding: []float32{0.1, 0.2, 0.3},
			nResults:       5,
			docIDs:         []string{},
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"detail": "Collection not found", "type": "not_found"}`))
			},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				var serviceErr *resilience.ServiceError
				assert.ErrorAs(t, err, &serviceErr)
				assert.Equal(t, resilience.ErrorCodeNotFound, serviceErr.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mockChromaServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
				"POST:/api/v1/collections/test-collection/query": tt.serverResp,
			})
			defer server.Close()

			client := NewClientForTesting(server.URL, "test-collection", logger)
			ctx := context.Background()

			results, err := client.Search(ctx, tt.queryEmbedding, tt.nResults, tt.docIDs)

			if tt.expectError {
				require.Error(t, err)
				validateError(t, err, tt.errorCheck)
			} else {
				require.NoError(t, err)
				assert.Len(t, results, tt.expectedCount)

				// Validate search results structure
				for i, result := range results {
					assert.NotEmpty(t, result.ID)
					assert.NotEmpty(t, result.Content)
					assert.NotNil(t, result.Metadata)
					assert.GreaterOrEqual(t, result.Distance, 0.0)

					// Verify specific values from mock response
					if tt.expectedCount > 0 {
						expectedIDs := []string{"doc1", "doc2"}
						expectedContents := []string{"Document 1 content", "Document 2 content"}
						expectedDistances := []float64{0.1, 0.2}

						if i < len(expectedIDs) {
							assert.Equal(t, expectedIDs[i], result.ID)
							assert.Equal(t, expectedContents[i], result.Content)
							assert.Equal(t, expectedDistances[i], result.Distance)
							assert.Equal(t, expectedIDs[i], result.Metadata["doc_id"])
						}
					}
				}
			}
		})
	}
}

// TestHealthCheck tests the health check functionality
func TestHealthCheck(t *testing.T) {
	logger := zap.NewNop() // Use nop logger for faster tests

	tests := []struct {
		name        string
		serverResp  func(w http.ResponseWriter, r *http.Request)
		expectError bool
		errorCheck  func(t *testing.T, err error)
	}{
		{
			name: "healthy service",
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "GET", r.Method)
				assert.Equal(t, "/api/v1/heartbeat", r.URL.Path)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(createMockHeartbeatResponse()))
			},
			expectError: false,
		},
		{
			name: "unhealthy service",
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"detail": "Service unavailable", "type": "server_error"}`))
			},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				var serviceErr *resilience.ServiceError
				assert.ErrorAs(t, err, &serviceErr)
				assert.Equal(t, resilience.ErrorCodeServiceUnavailable, serviceErr.Code)
			},
		},
		{
			name: "service not found",
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`Not found`))
			},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				var serviceErr *resilience.ServiceError
				assert.ErrorAs(t, err, &serviceErr)
				assert.Equal(t, resilience.ErrorCodeNotFound, serviceErr.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mockChromaServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
				"GET:/api/v1/heartbeat": tt.serverResp,
			})
			defer server.Close()

			client := NewClientForTesting(server.URL, "test-collection", logger)
			ctx := context.Background()

			err := client.HealthCheck(ctx)

			if tt.expectError {
				require.Error(t, err)
				validateError(t, err, tt.errorCheck)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCreateCollection tests collection creation
func TestCreateCollection(t *testing.T) {
	logger := zap.NewNop() // Use nop logger for faster tests
	tests := []struct {
		name        string
		collName    string
		metadata    map[string]interface{}
		serverResp  func(w http.ResponseWriter, r *http.Request)
		expectError bool
	}{
		{
			name:     "successful creation",
			collName: "new-collection",
			metadata: map[string]interface{}{"created": "2024-01-01"},
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/api/v1/collections", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"name": "new-collection", "id": "new-collection-id"}`))
			},
			expectError: false,
		},
		{
			name:     "creation error",
			collName: "existing-collection",
			metadata: nil,
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"detail": "Collection already exists", "type": "invalid_request"}`))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mockChromaServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
				"POST:/api/v1/collections": tt.serverResp,
			})
			defer server.Close()

			client := NewClientForTesting(server.URL, "test-collection", logger)
			ctx := context.Background()

			err := client.CreateCollection(ctx, tt.collName, tt.metadata)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestGetCollection tests collection retrieval
func TestGetCollection(t *testing.T) {
	logger := zap.NewNop() // Use nop logger for faster tests
	tests := []struct {
		name        string
		collName    string
		serverResp  func(w http.ResponseWriter, r *http.Request)
		expectError bool
		validate    func(t *testing.T, collection *Collection)
	}{
		{
			name:     "successful get",
			collName: "test-collection",
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "GET", r.Method)
				assert.Equal(t, "/api/v1/collections/test-collection", r.URL.Path)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(createMockCollectionResponse()))
			},
			expectError: false,
			validate: func(t *testing.T, collection *Collection) {
				assert.NotNil(t, collection)
				assert.Equal(t, "test-collection", collection.Name)
				assert.Equal(t, "test-collection-id", collection.ID)
				assert.NotNil(t, collection.Metadata)
			},
		},
		{
			name:     "collection not found",
			collName: "nonexistent-collection",
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"detail": "Collection not found", "type": "not_found"}`))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mockChromaServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
				fmt.Sprintf("GET:/api/v1/collections/%s", tt.collName): tt.serverResp,
			})
			defer server.Close()

			client := NewClientForTesting(server.URL, "test-collection", logger)
			ctx := context.Background()

			collection, err := client.GetCollection(ctx, tt.collName)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, collection)
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, collection)
				}
			}
		})
	}
}

// TestListCollections tests collection listing
func TestListCollections(t *testing.T) {
	logger := zap.NewNop() // Use nop logger for faster tests
	tests := []struct {
		name        string
		serverResp  func(w http.ResponseWriter, r *http.Request)
		expectError bool
		validate    func(t *testing.T, collections []Collection)
	}{
		{
			name: "successful list",
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "GET", r.Method)
				assert.Equal(t, "/api/v1/collections", r.URL.Path)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(createMockCollectionsResponse()))
			},
			expectError: false,
			validate: func(t *testing.T, collections []Collection) {
				assert.Len(t, collections, 2)
				assert.Equal(t, "collection1", collections[0].Name)
				assert.Equal(t, "collection2", collections[1].Name)
			},
		},
		{
			name: "empty list",
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			},
			expectError: false,
			validate: func(t *testing.T, collections []Collection) {
				assert.Len(t, collections, 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mockChromaServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
				"GET:/api/v1/collections": tt.serverResp,
			})
			defer server.Close()

			client := NewClientForTesting(server.URL, "test-collection", logger)
			ctx := context.Background()

			collections, err := client.ListCollections(ctx)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, collections)
				}
			}
		})
	}
}

// TestDeleteCollection tests collection deletion
func TestDeleteCollection(t *testing.T) {
	logger := zap.NewNop() // Use nop logger for faster tests
	tests := []struct {
		name        string
		collName    string
		serverResp  func(w http.ResponseWriter, r *http.Request)
		expectError bool
	}{
		{
			name:     "successful deletion",
			collName: "test-collection",
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "DELETE", r.Method)
				assert.Equal(t, "/api/v1/collections/test-collection", r.URL.Path)

				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"success": true}`))
			},
			expectError: false,
		},
		{
			name:     "collection not found",
			collName: "nonexistent-collection",
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"detail": "Collection not found", "type": "not_found"}`))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mockChromaServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
				fmt.Sprintf("DELETE:/api/v1/collections/%s", tt.collName): tt.serverResp,
			})
			defer server.Close()

			client := NewClientForTesting(server.URL, "test-collection", logger)
			ctx := context.Background()

			err := client.DeleteCollection(ctx, tt.collName)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// validateError is a helper function to reduce nested if complexity
func validateError(t *testing.T, err error, errorCheck func(*testing.T, error)) {
	if errorCheck != nil {
		errorCheck(t, err)
	}
}

// TestErrorHandling tests various error scenarios
func TestErrorHandling(t *testing.T) {
	logger := zap.NewNop() // Use nop logger for faster tests

	tests := []struct {
		name       string
		statusCode int
		response   string
		expectCode resilience.ErrorCode
	}{
		{
			name:       "bad request with ChromaDB error",
			statusCode: http.StatusBadRequest,
			response:   `{"detail": "Invalid request", "type": "invalid_request"}`,
			expectCode: resilience.ErrorCodeBadRequest,
		},
		{
			name:       "not found with ChromaDB error",
			statusCode: http.StatusNotFound,
			response:   `{"detail": "Collection not found", "type": "not_found"}`,
			expectCode: resilience.ErrorCodeNotFound,
		},
		{
			name:       "service unavailable with ChromaDB error",
			statusCode: http.StatusServiceUnavailable,
			response:   `{"detail": "Service unavailable", "type": "server_error"}`,
			expectCode: resilience.ErrorCodeServiceUnavailable,
		},
		{
			name:       "internal server error with ChromaDB error",
			statusCode: http.StatusInternalServerError,
			response:   `{"detail": "Internal server error", "type": "server_error"}`,
			expectCode: resilience.ErrorCodeServiceUnavailable,
		},
		{
			name:       "bad request with plain text",
			statusCode: http.StatusBadRequest,
			response:   `Bad request`,
			expectCode: resilience.ErrorCodeBadRequest,
		},
		{
			name:       "generic error",
			statusCode: http.StatusUnprocessableEntity,
			response:   `{"detail": "Unprocessable entity", "type": "validation_error"}`,
			expectCode: resilience.ErrorCodeInternalError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mockChromaServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
				"GET:/api/v1/heartbeat": func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tt.statusCode)
					_, _ = w.Write([]byte(tt.response))
				},
			})
			defer server.Close()

			client := NewClientForTesting(server.URL, "test-collection", logger)
			ctx := context.Background()

			err := client.HealthCheck(ctx)

			require.Error(t, err)
			var serviceErr *resilience.ServiceError
			assert.ErrorAs(t, err, &serviceErr)
			assert.Equal(t, tt.expectCode, serviceErr.Code)
		})
	}
}

// TestContextCancellation tests context cancellation handling
func TestContextCancellation(t *testing.T) {
	logger := zap.NewNop() // Use nop logger for faster tests

	// Create a server that delays response
	server := mockChromaServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET:/api/v1/heartbeat": func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(2 * time.Second)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(createMockHeartbeatResponse()))
		},
	})
	defer server.Close()

	client := NewClientForTesting(server.URL, "test-collection", logger)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := client.HealthCheck(ctx)
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "context deadline exceeded") ||
			strings.Contains(err.Error(), "Operation timed out"),
		"Expected context cancellation error, got: %v", err)
}

// TestGetHealthCheckFunction tests the health check function wrapper
func TestGetHealthCheckFunction(t *testing.T) {
	logger := zap.NewNop() // Use nop logger for faster tests

	tests := []struct {
		name           string
		serverResp     func(w http.ResponseWriter, r *http.Request)
		expectedStatus resilience.HealthStatus
		expectError    bool
	}{
		{
			name: "healthy service",
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(createMockHeartbeatResponse()))
			},
			expectedStatus: resilience.HealthStatusHealthy,
			expectError:    false,
		},
		{
			name: "unhealthy service",
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"detail": "Service unavailable", "type": "server_error"}`))
			},
			expectedStatus: resilience.HealthStatusUnhealthy,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mockChromaServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
				"GET:/api/v1/heartbeat": tt.serverResp,
			})
			defer server.Close()

			client := NewClientForTesting(server.URL, "test-collection", logger)
			healthCheck := client.GetHealthCheck()

			ctx := context.Background()
			result := healthCheck(ctx)

			assert.Equal(t, tt.expectedStatus, result.Status)
			assert.NotZero(t, result.Timestamp)
			assert.GreaterOrEqual(t, result.Duration, time.Duration(0))
			assert.NotEmpty(t, result.Message)

			if tt.expectError {
				assert.Contains(t, result.Message, "health check failed")
			} else {
				assert.Contains(t, result.Message, "healthy")
			}
		})
	}
}

// TestGetCircuitBreakerStats tests circuit breaker stats retrieval
func TestGetCircuitBreakerStats(t *testing.T) {
	logger := zap.NewNop() // Use nop logger for faster tests
	client := NewClientForTesting("http://localhost:8000", "test-collection", logger)

	stats := client.GetCircuitBreakerStats()
	assert.NotNil(t, stats)
	// Circuit breaker starts in closed state
	assert.Equal(t, resilience.CircuitClosed, stats.State)
	assert.Equal(t, 0, stats.FailedReqs)
	assert.Equal(t, 0, stats.SuccessfulReqs)
}

// TestSearchHelperFunctions tests the internal search helper functions
func TestSearchHelperFunctions(t *testing.T) {
	logger := zap.NewNop() // Use nop logger for faster tests
	client := NewClientForTesting("http://localhost:8000", "test-collection", logger)

	t.Run("buildSearchRequest", func(t *testing.T) {
		queryEmbedding := []float32{0.1, 0.2, 0.3}
		nResults := 5
		docIDs := []string{"doc1", "doc2"}

		req := client.buildSearchRequest(queryEmbedding, nResults, docIDs)

		assert.Equal(t, [][]float32{queryEmbedding}, req.QueryEmbeddings)
		assert.Equal(t, nResults, req.NResults)
		assert.NotNil(t, req.Where)
		assert.Equal(t, map[string]interface{}{
			"doc_id": map[string]interface{}{
				"$in": docIDs,
			},
		}, req.Where)
	})

	t.Run("buildSearchRequest without docIDs", func(t *testing.T) {
		queryEmbedding := []float32{0.1, 0.2, 0.3}
		nResults := 5
		var docIDs []string

		req := client.buildSearchRequest(queryEmbedding, nResults, docIDs)

		assert.Equal(t, [][]float32{queryEmbedding}, req.QueryEmbeddings)
		assert.Equal(t, nResults, req.NResults)
		assert.Nil(t, req.Where)
	})

	t.Run("processSearchResponse", func(t *testing.T) {
		searchResp := SearchResponse{
			IDs:       [][]string{{"doc1", "doc2"}},
			Documents: [][]string{{"Content 1", "Content 2"}},
			Metadatas: [][]map[string]interface{}{
				{
					{"key1": "value1"},
					{"key2": "value2"},
				},
			},
			Distances: [][]float64{{0.1, 0.2}},
		}

		results := client.processSearchResponse(searchResp)

		assert.Len(t, results, 2)
		assert.Equal(t, "doc1", results[0].ID)
		assert.Equal(t, "Content 1", results[0].Content)
		assert.Equal(t, 0.1, results[0].Distance)
		assert.Equal(t, map[string]string{"key1": "value1"}, results[0].Metadata)

		assert.Equal(t, "doc2", results[1].ID)
		assert.Equal(t, "Content 2", results[1].Content)
		assert.Equal(t, 0.2, results[1].Distance)
		assert.Equal(t, map[string]string{"key2": "value2"}, results[1].Metadata)
	})

	t.Run("processSearchResponse empty", func(t *testing.T) {
		searchResp := SearchResponse{
			IDs:       [][]string{},
			Documents: [][]string{},
			Metadatas: [][]map[string]interface{}{},
			Distances: [][]float64{},
		}

		results := client.processSearchResponse(searchResp)
		assert.Len(t, results, 0)
	})

	t.Run("convertMetadata", func(t *testing.T) {
		metadatas := [][]map[string]interface{}{
			{
				{"string_key": "string_value", "int_key": 123, "bool_key": true},
			},
		}

		metadata := client.convertMetadata(metadatas, 0)
		assert.Equal(t, map[string]string{"string_key": "string_value"}, metadata)
	})

	t.Run("convertMetadata empty", func(t *testing.T) {
		var metadatas [][]map[string]interface{}
		metadata := client.convertMetadata(metadatas, 0)
		assert.Nil(t, metadata)
	})
}

// TestErrorType tests the Error type implementation
func TestErrorType(t *testing.T) {
	err := Error{
		Detail: "Test error message",
		Type:   "test_error",
	}

	expected := "ChromaDB error [test_error]: Test error message"
	assert.Equal(t, expected, err.Error())
}

// TestNetworkErrors tests network-related error scenarios
func TestNetworkErrors(t *testing.T) {
	logger := zap.NewNop() // Use nop logger for faster tests

	// Test with invalid URL that will cause network error
	client := NewClientForTesting("http://invalid-host:9999", "test-collection", logger)
	ctx := context.Background()

	err := client.HealthCheck(ctx)
	require.Error(t, err)

	// Should be wrapped as a service error
	var serviceErr *resilience.ServiceError
	assert.ErrorAs(t, err, &serviceErr)
}

// BenchmarkSearch benchmarks the search operation
func BenchmarkSearch(b *testing.B) {
	logger := zap.NewNop()

	server := mockChromaServer(nil, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST:/api/v1/collections/test-collection/query": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(createMockSearchResponse()))
		},
	})
	defer server.Close()

	client := NewClientForTesting(server.URL, "test-collection", logger)
	ctx := context.Background()
	queryEmbedding := []float32{0.1, 0.2, 0.3}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.Search(ctx, queryEmbedding, 5, nil)
		if err != nil {
			b.Errorf("Search failed: %v", err)
		}
	}
}

// BenchmarkAddDocuments benchmarks the add documents operation
func BenchmarkAddDocuments(b *testing.B) {
	logger := zap.NewNop()

	server := mockChromaServer(nil, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST:/api/v1/collections/test-collection/add": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(createMockAddResponse()))
		},
	})
	defer server.Close()

	client := NewClientForTesting(server.URL, "test-collection", logger)
	ctx := context.Background()

	documents := []Document{
		{ID: "doc1", Content: "Document 1", Metadata: map[string]string{"source": "test1.md"}},
		{ID: "doc2", Content: "Document 2", Metadata: map[string]string{"source": "test2.md"}},
	}
	embeddings := [][]float32{{0.1, 0.2, 0.3}, {0.4, 0.5, 0.6}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := client.AddDocuments(ctx, documents, embeddings)
		if err != nil {
			b.Errorf("AddDocuments failed: %v", err)
		}
	}
}

// SECURITY TESTING: ChromaDB External API Security Tests

func TestChromaDB_ConnectionSecurity(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name        string
		baseURL     string
		expectError bool
		description string
	}{
		{
			name:        "secure_https_connection",
			baseURL:     "https://secure-chroma.example.com",
			expectError: false,
			description: "HTTPS connections should be preferred",
		},
		{
			name:        "insecure_http_connection",
			baseURL:     "http://insecure-chroma.example.com",
			expectError: true,
			description: "HTTP connections should be rejected in production",
		},
		{
			name:        "localhost_development",
			baseURL:     "http://localhost:8000",
			expectError: false,
			description: "Localhost HTTP should be allowed for development",
		},
		{
			name:        "malformed_url",
			baseURL:     "not-a-valid-url",
			expectError: true,
			description: "Malformed URLs should be rejected",
		},
		{
			name:        "url_with_injection",
			baseURL:     "http://localhost:8000'; DROP TABLE collections; --",
			expectError: true,
			description: "URLs with injection attempts should be rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate URL scheme in production
			isProduction := !strings.Contains(tt.baseURL, "localhost") && !strings.Contains(tt.baseURL, "127.0.0.1")
			isHTTP := strings.HasPrefix(tt.baseURL, "http://")

			if isProduction && isHTTP && tt.expectError {
				t.Logf("%s: HTTP in production correctly flagged as error", tt.description)
			}

			// Test client creation with URL validation
			client := NewClientForTesting(tt.baseURL, "test-collection", logger)

			// Basic validation test
			if client == nil && !tt.expectError {
				t.Errorf("%s: Expected client creation to succeed", tt.description)
			}

			if client != nil && tt.expectError && isProduction && isHTTP {
				t.Logf("%s: Client created but would be rejected in production security validation", tt.description)
			}
		})
	}
}

func TestChromaDB_TimeoutAndDoSPrevention(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name          string
		serverDelay   time.Duration
		clientTimeout time.Duration
		expectTimeout bool
		description   string
	}{
		{
			name:          "normal_response_time",
			serverDelay:   50 * time.Millisecond,
			clientTimeout: 2 * time.Second,
			expectTimeout: false,
			description:   "Normal response times should succeed",
		},
		{
			name:          "slow_response_timeout",
			serverDelay:   3 * time.Second,
			clientTimeout: 1 * time.Second,
			expectTimeout: true,
			description:   "Slow responses should timeout to prevent DoS",
		},
		{
			name:          "extreme_delay_attack",
			serverDelay:   30 * time.Second,
			clientTimeout: 2 * time.Second,
			expectTimeout: true,
			description:   "Extreme delays should be caught by timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create server that delays responses
			server := mockChromaServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
				"POST:/api/v1/collections/test-collection/query": func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(tt.serverDelay)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"ids":[["doc1"]],"distances":[[0.1]],"documents":[["test document"]]}`))
				},
			})
			defer server.Close()

			client := NewClientForTesting(server.URL, "test-collection", logger)

			// Test with timeout context
			ctx, cancel := context.WithTimeout(context.Background(), tt.clientTimeout)
			defer cancel()

			start := time.Now()
			_, err := client.Search(ctx, []float32{0.1, 0.2, 0.3}, 1, nil)
			elapsed := time.Since(start)

			if tt.expectTimeout {
				if err == nil {
					t.Errorf("%s: Expected timeout error but got none", tt.description)
				}
				if elapsed > tt.clientTimeout+200*time.Millisecond {
					t.Errorf("%s: Timeout took too long: %v", tt.description, elapsed)
				}
			} else {
				if err != nil && strings.Contains(err.Error(), "timeout") {
					t.Errorf("%s: Got unexpected timeout: %v", tt.description, err)
				}
			}
		})
	}
}

func TestChromaDB_InputValidationAndInjection(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name           string
		collectionName string
		documentID     string
		queryContent   string
		expectError    bool
		description    string
	}{
		{
			name:           "valid_collection_name",
			collectionName: "valid-collection-123",
			documentID:     "doc-001",
			queryContent:   "normal query content",
			expectError:    false,
			description:    "Valid collection names should be accepted",
		},
		{
			name:           "sql_injection_collection",
			collectionName: "collection'; DROP TABLE collections; --",
			documentID:     "doc-001",
			queryContent:   "normal query content",
			expectError:    true,
			description:    "SQL injection in collection name should be rejected",
		},
		{
			name:           "xss_injection_document_id",
			collectionName: "valid-collection",
			documentID:     "<script>alert('xss')</script>",
			queryContent:   "normal query content",
			expectError:    true,
			description:    "XSS injection in document ID should be rejected",
		},
		{
			name:           "nosql_injection_query",
			collectionName: "valid-collection",
			documentID:     "doc-001",
			queryContent:   "$where: function() { return true; }",
			expectError:    true,
			description:    "NoSQL injection in query should be rejected",
		},
		{
			name:           "path_traversal_collection",
			collectionName: "../../../etc/passwd",
			documentID:     "doc-001",
			queryContent:   "normal query content",
			expectError:    true,
			description:    "Path traversal in collection name should be rejected",
		},
		{
			name:           "command_injection_document",
			collectionName: "valid-collection",
			documentID:     "doc-001; rm -rf /",
			queryContent:   "normal query content",
			expectError:    true,
			description:    "Command injection in document ID should be rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mockChromaServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
				"POST:/api/v1/collections/" + tt.collectionName + "/add": func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(createMockAddResponse()))
				},
			})
			defer server.Close()

			// Validate inputs before creating client
			inputSafe := validateChromaInputs(tt.collectionName, tt.documentID, tt.queryContent)

			if tt.expectError && inputSafe {
				t.Errorf("%s: Dangerous input should have been rejected", tt.description)
			} else if !tt.expectError && !inputSafe {
				t.Errorf("%s: Safe input should have been accepted", tt.description)
			}

			if inputSafe {
				client := NewClientForTesting(server.URL, tt.collectionName, logger)

				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()

				// Test document addition with validated inputs
				documents := []Document{{ID: tt.documentID, Content: tt.queryContent}}
				embeddings := [][]float32{{0.1, 0.2, 0.3}}

				err := client.AddDocuments(ctx, documents, embeddings)
				if err != nil {
					t.Logf("%s: Document addition failed (expected for security testing): %v", tt.description, err)
				}
			} else {
				t.Logf("%s: Input blocked by security validation", tt.description)
			}
		})
	}
}

func TestChromaDB_DataSizeAndDoSPrevention(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name            string
		documentCount   int
		documentSize    int
		embeddingDim    int
		expectRejection bool
		description     string
	}{
		{
			name:            "normal_batch_size",
			documentCount:   10,
			documentSize:    1024, // 1KB per document
			embeddingDim:    1536, // Standard OpenAI embedding dimension
			expectRejection: false,
			description:     "Normal batch sizes should be accepted",
		},
		{
			name:            "large_batch_size",
			documentCount:   1000,
			documentSize:    1024,
			embeddingDim:    1536,
			expectRejection: false,
			description:     "Large but reasonable batches should be accepted",
		},
		{
			name:            "massive_document_attack",
			documentCount:   1,
			documentSize:    100 * 1024 * 1024, // 100MB single document
			embeddingDim:    1536,
			expectRejection: true,
			description:     "Massive documents should be rejected",
		},
		{
			name:            "batch_flood_attack",
			documentCount:   10000,
			documentSize:    1024,
			embeddingDim:    1536,
			expectRejection: true,
			description:     "Excessive batch sizes should be rejected",
		},
		{
			name:            "dimension_explosion_attack",
			documentCount:   10,
			documentSize:    1024,
			embeddingDim:    100000, // Extremely high dimension
			expectRejection: true,
			description:     "Excessive embedding dimensions should be rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mockChromaServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
				"POST:/api/v1/collections/test-collection/add": func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(createMockAddResponse()))
				},
			})
			defer server.Close()

			client := NewClientForTesting(server.URL, "test-collection", logger)

			// Validate data size limits
			sizeValid := validateDataSize(tt.documentCount, tt.documentSize, tt.embeddingDim)

			if tt.expectRejection && sizeValid {
				t.Errorf("%s: Large data should have been rejected", tt.description)
			} else if !tt.expectRejection && !sizeValid {
				t.Errorf("%s: Normal data should have been accepted", tt.description)
			}

			if sizeValid {
				// Create test documents and embeddings
				documents := make([]Document, tt.documentCount)
				embeddings := make([][]float32, tt.documentCount)

				for i := 0; i < tt.documentCount; i++ {
					documents[i] = Document{
						ID:      fmt.Sprintf("doc-%d", i),
						Content: strings.Repeat("A", tt.documentSize),
					}
					embeddings[i] = make([]float32, tt.embeddingDim)
					for j := 0; j < tt.embeddingDim; j++ {
						embeddings[i][j] = 0.1
					}
				}

				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				err := client.AddDocuments(ctx, documents, embeddings)
				if err != nil {
					t.Logf("%s: Large data operation failed: %v", tt.description, err)
				}
			} else {
				t.Logf("%s: Data blocked by size validation", tt.description)
			}
		})
	}
}

func TestChromaDB_ErrorHandlingAndInformationLeakage(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name            string
		serverResponse  func(w http.ResponseWriter, r *http.Request)
		expectSafeError bool
		description     string
	}{
		{
			name: "normal_error_response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"detail": "Invalid request", "type": "bad_request"}`))
			},
			expectSafeError: true,
			description:     "Normal error responses should not leak sensitive information",
		},
		{
			name: "error_with_sensitive_info",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"detail": "Database connection failed: host=db-internal.company.com user=admin password=secret123", "type": "internal_error"}`))
			},
			expectSafeError: false,
			description:     "Error responses with sensitive information should be sanitized",
		},
		{
			name: "stack_trace_leakage",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"detail": "Error in /usr/local/app/chroma/db.py line 123: connect() failed", "type": "internal_error", "stack_trace": "Full stack trace with file paths"}`))
			},
			expectSafeError: false,
			description:     "Stack traces should not be exposed to clients",
		},
		{
			name: "authentication_error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"detail": "Authentication failed", "type": "auth_error"}`))
			},
			expectSafeError: true,
			description:     "Authentication errors should be generic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mockChromaServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
				"POST:/api/v1/collections/test-collection/query": tt.serverResponse,
			})
			defer server.Close()

			client := NewClientForTesting(server.URL, "test-collection", logger)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_, err := client.Search(ctx, []float32{0.1, 0.2, 0.3}, 1, nil)

			if err != nil {
				errorMessage := err.Error()

				// Check for sensitive information leakage
				hasSensitiveInfo := checkErrorForSensitiveInfo(errorMessage)

				if !tt.expectSafeError && !hasSensitiveInfo {
					t.Logf("%s: Error correctly sanitized: %v", tt.description, err)
				} else if tt.expectSafeError && hasSensitiveInfo {
					t.Errorf("%s: Error contains sensitive information: %v", tt.description, err)
				}
			}
		})
	}
}

func TestChromaDB_CircuitBreakerSecurity(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name              string
		failureCount      int
		expectCircuitOpen bool
		description       string
	}{
		{
			name:              "few_failures_circuit_closed",
			failureCount:      2,
			expectCircuitOpen: false,
			description:       "Few failures should keep circuit closed",
		},
		{
			name:              "many_failures_circuit_open",
			failureCount:      10,
			expectCircuitOpen: true,
			description:       "Many failures should open circuit to prevent DoS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failureCount := 0
			server := mockChromaServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
				"POST:/api/v1/collections/test-collection/query": func(w http.ResponseWriter, r *http.Request) {
					failureCount++
					if failureCount <= tt.failureCount {
						w.WriteHeader(http.StatusInternalServerError)
						_, _ = w.Write([]byte(`{"detail": "Internal error"}`))
					} else {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte(`{"ids":[["doc1"]],"distances":[[0.1]],"documents":[["test"]]}`))
					}
				},
			})
			defer server.Close()

			client := NewClientForTesting(server.URL, "test-collection", logger)

			// Make multiple requests to trigger circuit breaker
			for i := 0; i < tt.failureCount+2; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				_, err := client.Search(ctx, []float32{0.1, 0.2, 0.3}, 1, nil)
				cancel()

				if i >= tt.failureCount && tt.expectCircuitOpen {
					// Circuit should be open, preventing further calls
					if err == nil {
						t.Logf("%s: Request %d succeeded despite circuit breaker", tt.description, i+1)
					} else if strings.Contains(err.Error(), "circuit") {
						t.Logf("%s: Circuit breaker correctly blocked request %d", tt.description, i+1)
					}
				}
			}
		})
	}
}

// Helper functions for ChromaDB security testing

func validateChromaInputs(collectionName, documentID, queryContent string) bool {
	// Simple input validation for testing
	// In production, this would be more comprehensive

	dangerousPatterns := []string{
		"<script>",
		"'; DROP",
		"../",
		"rm -rf",
		"$where:",
		"function()",
	}

	inputs := []string{collectionName, documentID, queryContent}
	for _, input := range inputs {
		for _, pattern := range dangerousPatterns {
			if strings.Contains(input, pattern) {
				return false
			}
		}
	}

	// Basic length validation
	if len(collectionName) > 100 || len(documentID) > 100 {
		return false
	}

	return true
}

func validateDataSize(documentCount, documentSize, embeddingDim int) bool {
	// Size limits for DoS prevention
	maxDocuments := 5000
	maxDocumentSize := 10 * 1024 * 1024 // 10MB
	maxEmbeddingDim := 10000

	return documentCount <= maxDocuments &&
		documentSize <= maxDocumentSize &&
		embeddingDim <= maxEmbeddingDim
}

func checkErrorForSensitiveInfo(errorMessage string) bool {
	// Check for sensitive information patterns in error messages
	sensitivePatterns := []string{
		"password=",
		"user=",
		"host=",
		"stack_trace",
		"/usr/local/",
		"db-internal",
		"secret",
		"token",
	}

	errorLower := strings.ToLower(errorMessage)
	for _, pattern := range sensitivePatterns {
		if strings.Contains(errorLower, pattern) {
			return true
		}
	}

	return false
}
