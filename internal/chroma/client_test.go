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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strings"

	"go.uber.org/zap"
)

// mockServer creates a test HTTP server with configurable responses
func mockServer(responses map[string]func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	mux := http.NewServeMux()

	for path, handler := range responses {
		mux.HandleFunc(path, handler)
	}

	return httptest.NewServer(mux)
}

// testLogger creates a no-op logger for testing
func testLogger() *zap.Logger {
	return zap.NewNop()
}

// containsString checks if a string contains a substring
func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestNewClient(t *testing.T) {
	client := NewClient("http://localhost:8000", "test-collection")

	if client.baseURL != "http://localhost:8000" {
		t.Errorf("Expected baseURL to be 'http://localhost:8000', got %s", client.baseURL)
	}

	if client.collection != "test-collection" {
		t.Errorf("Expected collection to be 'test-collection', got %s", client.collection)
	}

	if client.maxRetries != 3 {
		t.Errorf("Expected maxRetries to be 3, got %d", client.maxRetries)
	}

	if client.baseRetryDelay != time.Second {
		t.Errorf("Expected baseRetryDelay to be 1 second, got %v", client.baseRetryDelay)
	}
}

func TestNewClientWithOptions(t *testing.T) {
	logger := testLogger()
	client := NewClientWithOptions("http://test:8000", "custom-collection", logger, 5, 2*time.Second)

	if client.baseURL != "http://test:8000" {
		t.Errorf("Expected baseURL to be 'http://test:8000', got %s", client.baseURL)
	}

	if client.collection != "custom-collection" {
		t.Errorf("Expected collection to be 'custom-collection', got %s", client.collection)
	}

	if client.maxRetries != 5 {
		t.Errorf("Expected maxRetries to be 5, got %d", client.maxRetries)
	}

	if client.baseRetryDelay != 2*time.Second {
		t.Errorf("Expected baseRetryDelay to be 2 seconds, got %v", client.baseRetryDelay)
	}
}

func TestHealthCheck_Success(t *testing.T) {
	server := mockServer(map[string]func(w http.ResponseWriter, r *http.Request){
		"/api/v1/heartbeat": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	})
	defer server.Close()

	client := NewClientWithOptions(server.URL, "test", testLogger(), 1, 100*time.Millisecond)

	err := client.HealthCheck()
	if err != nil {
		t.Errorf("Expected health check to succeed, got error: %v", err)
	}
}

func TestHealthCheck_Failure(t *testing.T) {
	server := mockServer(map[string]func(w http.ResponseWriter, r *http.Request){
		"/api/v1/heartbeat": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
	})
	defer server.Close()

	client := NewClientWithOptions(server.URL, "test", testLogger(), 1, 100*time.Millisecond)

	err := client.HealthCheck()
	if err == nil {
		t.Error("Expected health check to fail, got nil error")
	}
}

func TestAddDocuments_Success(t *testing.T) {
	server := mockServer(map[string]func(w http.ResponseWriter, r *http.Request){
		"/api/v1/collections/test/add": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("Expected POST request, got %s", r.Method)
			}

			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Expected Content-Type to be application/json, got %s", r.Header.Get("Content-Type"))
			}

			w.WriteHeader(http.StatusOK)
		},
	})
	defer server.Close()

	client := NewClientWithOptions(server.URL, "test", testLogger(), 1, 100*time.Millisecond)

	documents := []Document{
		{
			ID:      "doc1",
			Content: "Test document 1",
			Metadata: map[string]string{
				"type": "test",
			},
		},
		{
			ID:      "doc2",
			Content: "Test document 2",
			Metadata: map[string]string{
				"type": "test",
			},
		},
	}

	embeddings := [][]float32{
		{0.1, 0.2, 0.3},
		{0.4, 0.5, 0.6},
	}

	err := client.AddDocuments(documents, embeddings)
	if err != nil {
		t.Errorf("Expected AddDocuments to succeed, got error: %v", err)
	}
}

func TestAddDocuments_ChromaError(t *testing.T) {
	server := mockServer(map[string]func(w http.ResponseWriter, r *http.Request){
		"/api/v1/collections/test/add": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ChromaError{
				Detail: "Invalid document format",
				Type:   "ValidationError",
			})
		},
	})
	defer server.Close()

	client := NewClientWithOptions(server.URL, "test", testLogger(), 1, 100*time.Millisecond)

	documents := []Document{{ID: "doc1", Content: "test"}}
	embeddings := [][]float32{{0.1, 0.2, 0.3}}

	err := client.AddDocuments(documents, embeddings)
	if err == nil {
		t.Error("Expected AddDocuments to fail, got nil error")
	}

	// Error should contain ChromaError details due to retry wrapping
	if !containsString(err.Error(), "Invalid document format") {
		t.Errorf("Expected error to contain 'Invalid document format', got %s", err.Error())
	}
	if !containsString(err.Error(), "ValidationError") {
		t.Errorf("Expected error to contain 'ValidationError', got %s", err.Error())
	}
}

func TestSearch_Success(t *testing.T) {
	expectedResponse := SearchResponse{
		IDs: [][]string{
			{"doc1", "doc2"},
		},
		Documents: [][]string{
			{"Document 1 content", "Document 2 content"},
		},
		Metadatas: [][]map[string]interface{}{
			{
				{"type": "test", "category": "demo"},
				{"type": "test", "category": "example"},
			},
		},
		Distances: [][]float64{
			{0.1, 0.3},
		},
	}

	server := mockServer(map[string]func(w http.ResponseWriter, r *http.Request){
		"/api/v1/collections/test/query": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("Expected POST request, got %s", r.Method)
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(expectedResponse)
		},
	})
	defer server.Close()

	client := NewClientWithOptions(server.URL, "test", testLogger(), 1, 100*time.Millisecond)

	queryEmbedding := []float32{0.1, 0.2, 0.3}
	results, err := client.Search(queryEmbedding, 2, nil)

	if err != nil {
		t.Errorf("Expected Search to succeed, got error: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	if results[0].ID != "doc1" {
		t.Errorf("Expected first result ID to be 'doc1', got %s", results[0].ID)
	}

	if results[0].Content != "Document 1 content" {
		t.Errorf("Expected first result content to be 'Document 1 content', got %s", results[0].Content)
	}

	if results[0].Distance != 0.1 {
		t.Errorf("Expected first result distance to be 0.1, got %f", results[0].Distance)
	}

	if results[0].Metadata["type"] != "test" {
		t.Errorf("Expected first result metadata type to be 'test', got %s", results[0].Metadata["type"])
	}
}

func TestSearch_WithDocIDFilter(t *testing.T) {
	var receivedRequest SearchRequest

	server := mockServer(map[string]func(w http.ResponseWriter, r *http.Request){
		"/api/v1/collections/test/query": func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&receivedRequest)

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(SearchResponse{
				IDs:       [][]string{{"doc1"}},
				Documents: [][]string{{"Document 1 content"}},
				Metadatas: [][]map[string]interface{}{{{}}},
				Distances: [][]float64{{0.1}},
			})
		},
	})
	defer server.Close()

	client := NewClientWithOptions(server.URL, "test", testLogger(), 1, 100*time.Millisecond)

	queryEmbedding := []float32{0.1, 0.2, 0.3}
	docIDs := []string{"doc1", "doc2"}

	_, err := client.Search(queryEmbedding, 1, docIDs)

	if err != nil {
		t.Errorf("Expected Search to succeed, got error: %v", err)
	}

	// Verify that the where clause was included
	if receivedRequest.Where == nil {
		t.Error("Expected where clause to be included in search request")
	}

	docIDFilter, ok := receivedRequest.Where["doc_id"].(map[string]interface{})
	if !ok {
		t.Error("Expected doc_id filter in where clause")
	}

	inClause, ok := docIDFilter["$in"].([]interface{})
	if !ok {
		t.Error("Expected $in clause in doc_id filter")
	}

	if len(inClause) != 2 {
		t.Errorf("Expected 2 doc IDs in filter, got %d", len(inClause))
	}

	// Convert back to strings for comparison
	inStrings := make([]string, len(inClause))
	for i, v := range inClause {
		if str, ok := v.(string); ok {
			inStrings[i] = str
		} else {
			t.Errorf("Expected string in $in clause, got %T", v)
		}
	}

	if inStrings[0] != "doc1" || inStrings[1] != "doc2" {
		t.Errorf("Expected doc ID filter to contain ['doc1', 'doc2'], got %v", inStrings)
	}
}

func TestCreateCollection_Success(t *testing.T) {
	var receivedRequest CreateCollectionRequest

	server := mockServer(map[string]func(w http.ResponseWriter, r *http.Request){
		"/api/v1/collections": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("Expected POST request, got %s", r.Method)
			}

			json.NewDecoder(r.Body).Decode(&receivedRequest)
			w.WriteHeader(http.StatusOK)
		},
	})
	defer server.Close()

	client := NewClientWithOptions(server.URL, "test", testLogger(), 1, 100*time.Millisecond)

	metadata := map[string]interface{}{
		"description": "Test collection",
		"version":     "1.0",
	}

	err := client.CreateCollection("new-collection", metadata)

	if err != nil {
		t.Errorf("Expected CreateCollection to succeed, got error: %v", err)
	}

	if receivedRequest.Name != "new-collection" {
		t.Errorf("Expected collection name to be 'new-collection', got %s", receivedRequest.Name)
	}

	if receivedRequest.Metadata["description"] != "Test collection" {
		t.Errorf("Expected description metadata, got %v", receivedRequest.Metadata)
	}
}

func TestDeleteCollection_Success(t *testing.T) {
	server := mockServer(map[string]func(w http.ResponseWriter, r *http.Request){
		"/api/v1/collections/test-collection": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "DELETE" {
				t.Errorf("Expected DELETE request, got %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
		},
	})
	defer server.Close()

	client := NewClientWithOptions(server.URL, "test", testLogger(), 1, 100*time.Millisecond)

	err := client.DeleteCollection("test-collection")

	if err != nil {
		t.Errorf("Expected DeleteCollection to succeed, got error: %v", err)
	}
}

func TestGetCollection_Success(t *testing.T) {
	expectedCollection := Collection{
		Name: "test-collection",
		ID:   "collection-123",
		Metadata: map[string]interface{}{
			"description": "Test collection",
			"created_at":  "2024-01-01T00:00:00Z",
		},
	}

	server := mockServer(map[string]func(w http.ResponseWriter, r *http.Request){
		"/api/v1/collections/test-collection": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Errorf("Expected GET request, got %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(expectedCollection)
		},
	})
	defer server.Close()

	client := NewClientWithOptions(server.URL, "test", testLogger(), 1, 100*time.Millisecond)

	collection, err := client.GetCollection("test-collection")

	if err != nil {
		t.Errorf("Expected GetCollection to succeed, got error: %v", err)
	}

	if collection.Name != "test-collection" {
		t.Errorf("Expected collection name to be 'test-collection', got %s", collection.Name)
	}

	if collection.ID != "collection-123" {
		t.Errorf("Expected collection ID to be 'collection-123', got %s", collection.ID)
	}
}

func TestListCollections_Success(t *testing.T) {
	expectedCollections := []Collection{
		{
			Name: "collection1",
			ID:   "id1",
			Metadata: map[string]interface{}{
				"type": "documents",
			},
		},
		{
			Name: "collection2",
			ID:   "id2",
			Metadata: map[string]interface{}{
				"type": "embeddings",
			},
		},
	}

	server := mockServer(map[string]func(w http.ResponseWriter, r *http.Request){
		"/api/v1/collections": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Errorf("Expected GET request, got %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(expectedCollections)
		},
	})
	defer server.Close()

	client := NewClientWithOptions(server.URL, "test", testLogger(), 1, 100*time.Millisecond)

	collections, err := client.ListCollections()

	if err != nil {
		t.Errorf("Expected ListCollections to succeed, got error: %v", err)
	}

	if len(collections) != 2 {
		t.Errorf("Expected 2 collections, got %d", len(collections))
	}

	if collections[0].Name != "collection1" {
		t.Errorf("Expected first collection name to be 'collection1', got %s", collections[0].Name)
	}

	if collections[1].Name != "collection2" {
		t.Errorf("Expected second collection name to be 'collection2', got %s", collections[1].Name)
	}
}

func TestRetryWithBackoff_Success(t *testing.T) {
	callCount := 0
	server := mockServer(map[string]func(w http.ResponseWriter, r *http.Request){
		"/api/v1/heartbeat": func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if callCount < 3 {
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		},
	})
	defer server.Close()

	client := NewClientWithOptions(server.URL, "test", testLogger(), 3, 10*time.Millisecond)

	err := client.HealthCheck()

	if err != nil {
		t.Errorf("Expected retry to succeed eventually, got error: %v", err)
	}

	if callCount != 3 {
		t.Errorf("Expected 3 calls (2 retries), got %d", callCount)
	}
}

func TestRetryWithBackoff_MaxRetriesExceeded(t *testing.T) {
	callCount := 0
	server := mockServer(map[string]func(w http.ResponseWriter, r *http.Request){
		"/api/v1/heartbeat": func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.WriteHeader(http.StatusInternalServerError)
		},
	})
	defer server.Close()

	client := NewClientWithOptions(server.URL, "test", testLogger(), 2, 10*time.Millisecond)

	err := client.HealthCheck()

	if err == nil {
		t.Error("Expected retry to fail after max retries, got nil error")
	}

	if callCount != 3 { // initial + 2 retries
		t.Errorf("Expected 3 calls (initial + 2 retries), got %d", callCount)
	}
}

func TestMakeRequest_ChromaError(t *testing.T) {
	server := mockServer(map[string]func(w http.ResponseWriter, r *http.Request){
		"/api/v1/test": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ChromaError{
				Detail: "Invalid request format",
				Type:   "ValidationError",
			})
		},
	})
	defer server.Close()

	client := NewClientWithOptions(server.URL, "test", testLogger(), 1, 100*time.Millisecond)

	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/test", server.URL), http.NoBody)
	_, err := client.makeRequest(req)

	if err == nil {
		t.Error("Expected makeRequest to return error, got nil")
	}

	// makeRequest should return ChromaError directly (not wrapped by retry)
	chromaErr, ok := err.(ChromaError)
	if !ok {
		t.Errorf("Expected ChromaError, got %T: %v", err, err)
	} else {
		if chromaErr.Detail != "Invalid request format" {
			t.Errorf("Expected detail 'Invalid request format', got %s", chromaErr.Detail)
		}
	}
}

func TestChromeError_Error(t *testing.T) {
	err := ChromaError{
		Detail: "Collection not found",
		Type:   "NotFoundError",
	}

	expected := "ChromaDB error [NotFoundError]: Collection not found"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}
}
