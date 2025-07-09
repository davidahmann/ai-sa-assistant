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

// Package chroma provides a client for interacting with ChromaDB vector database.
// It implements functionality for storing, retrieving, and searching document embeddings
// with support for metadata filtering and collection management.
package chroma

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"go.uber.org/zap"
)

const (
	// DefaultRetryAttempts defines the default number of retry attempts
	DefaultRetryAttempts = 3
	// DefaultHTTPTimeout defines the default HTTP client timeout
	DefaultHTTPTimeout = 30 * time.Second
	// HTTPClientErrorStatus defines the HTTP status code threshold for client errors
	HTTPClientErrorStatus = 400
	// ExponentialBackoffBase defines the base for exponential backoff calculations
	ExponentialBackoffBase = 2
)

// Client wraps the ChromaDB REST API
type Client struct {
	baseURL        string
	collection     string
	httpClient     *http.Client
	logger         *zap.Logger
	maxRetries     int
	baseRetryDelay time.Duration
}

// NewClient creates a new ChromaDB client with default settings
func NewClient(baseURL, collection string) *Client {
	logger, _ := zap.NewProduction()
	return NewClientWithOptions(baseURL, collection, logger, DefaultRetryAttempts, time.Second)
}

// NewClientWithOptions creates a new ChromaDB client with custom settings
func NewClientWithOptions(
	baseURL, collection string, logger *zap.Logger, maxRetries int, baseRetryDelay time.Duration,
) *Client {
	return &Client{
		baseURL:        baseURL,
		collection:     collection,
		httpClient:     &http.Client{Timeout: DefaultHTTPTimeout},
		logger:         logger,
		maxRetries:     maxRetries,
		baseRetryDelay: baseRetryDelay,
	}
}

// Document represents a document in ChromaDB
type Document struct {
	ID       string            `json:"id"`
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata"`
}

// SearchResult represents a search result from ChromaDB
type SearchResult struct {
	ID       string            `json:"id"`
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata"`
	Distance float64           `json:"distance"`
}

// AddDocumentsRequest represents the request to add documents
type AddDocumentsRequest struct {
	Documents []Document          `json:"documents"`
	Metadatas []map[string]string `json:"metadatas"`
	IDs       []string            `json:"ids"`
}

// SearchRequest represents a search request
type SearchRequest struct {
	QueryEmbeddings [][]float32            `json:"query_embeddings"`
	NResults        int                    `json:"n_results"`
	Where           map[string]interface{} `json:"where,omitempty"`
}

// SearchResponse represents the response from a search
type SearchResponse struct {
	IDs       [][]string                 `json:"ids"`
	Documents [][]string                 `json:"documents"`
	Metadatas [][]map[string]interface{} `json:"metadatas"`
	Distances [][]float64                `json:"distances"`
}

// Collection represents a ChromaDB collection
type Collection struct {
	Name     string                 `json:"name"`
	ID       string                 `json:"id"`
	Metadata map[string]interface{} `json:"metadata"`
}

// CreateCollectionRequest represents a request to create a collection
type CreateCollectionRequest struct {
	Name     string                 `json:"name"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Error represents an error response from ChromaDB
type Error struct {
	Detail string `json:"detail"`
	Type   string `json:"type"`
}

func (e Error) Error() string {
	return fmt.Sprintf("ChromaDB error [%s]: %s", e.Type, e.Detail)
}

// retryWithBackoff executes a function with exponential backoff retry logic
func (c *Client) retryWithBackoff(operation func() error, operationName string) error {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(math.Pow(ExponentialBackoffBase, float64(attempt-1))) * c.baseRetryDelay
			c.logger.Info("Retrying operation after delay",
				zap.String("operation", operationName),
				zap.Int("attempt", attempt),
				zap.Duration("delay", delay))
			time.Sleep(delay)
		}

		if err := operation(); err != nil {
			lastErr = err
			c.logger.Warn("Operation failed, will retry",
				zap.String("operation", operationName),
				zap.Int("attempt", attempt),
				zap.Error(err))
			continue
		}

		if attempt > 0 {
			c.logger.Info("Operation succeeded after retry",
				zap.String("operation", operationName),
				zap.Int("attempt", attempt))
		}
		return nil
	}

	c.logger.Error("Operation failed after all retries",
		zap.String("operation", operationName),
		zap.Int("max_retries", c.maxRetries),
		zap.Error(lastErr))
	return fmt.Errorf("operation failed after %d retries: %w", c.maxRetries, lastErr)
}

// makeRequest performs an HTTP request with structured error handling
func (c *Client) makeRequest(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}

	if resp.StatusCode >= HTTPClientErrorStatus {
		defer func() {
			if err := resp.Body.Close(); err != nil {
				c.logger.Debug("Failed to close response body", zap.Error(err))
			}
		}()
		body, _ := io.ReadAll(resp.Body)

		var chromaErr Error
		if json.Unmarshal(body, &chromaErr) == nil {
			return nil, chromaErr
		}

		return nil, fmt.Errorf("ChromaDB returned status %d: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}

// AddDocuments adds documents with embeddings to ChromaDB
func (c *Client) AddDocuments(documents []Document, embeddings [][]float32) error {
	c.logger.Info("Adding documents to ChromaDB",
		zap.String("collection", c.collection),
		zap.Int("document_count", len(documents)),
		zap.Int("embedding_count", len(embeddings)))

	return c.retryWithBackoff(func() error {
		url := fmt.Sprintf("%s/api/v1/collections/%s/add", c.baseURL, c.collection)

		// Prepare request payload
		var metadatas []map[string]string
		var ids []string
		var docTexts []string

		for _, doc := range documents {
			metadatas = append(metadatas, doc.Metadata)
			ids = append(ids, doc.ID)
			docTexts = append(docTexts, doc.Content)
		}

		payload := map[string]interface{}{
			"documents":  docTexts,
			"metadatas":  metadatas,
			"ids":        ids,
			"embeddings": embeddings,
		}

		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := c.makeRequest(req)
		if err != nil {
			return err
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				c.logger.Debug("Failed to close response body", zap.Error(err))
			}
		}()

		c.logger.Info("Successfully added documents",
			zap.String("collection", c.collection),
			zap.Int("document_count", len(documents)))

		return nil
	}, "AddDocuments")
}

// Search performs a vector search in ChromaDB
func (c *Client) Search(queryEmbedding []float32, nResults int, docIDs []string) ([]SearchResult, error) {
	c.logger.Info("Performing vector search",
		zap.String("collection", c.collection),
		zap.Int("n_results", nResults),
		zap.Int("doc_id_filter_count", len(docIDs)))

	var results []SearchResult
	err := c.retryWithBackoff(func() error {
		// Build search request
		searchReq := c.buildSearchRequest(queryEmbedding, nResults, docIDs)

		// Execute search request
		searchResp, err := c.executeSearchRequest(searchReq)
		if err != nil {
			return err
		}

		// Process search response
		results = c.processSearchResponse(searchResp)

		c.logger.Info("Search completed successfully",
			zap.String("collection", c.collection),
			zap.Int("results_returned", len(results)))

		return nil
	}, "Search")

	return results, err
}

// buildSearchRequest creates a search request with optional document ID filtering
func (c *Client) buildSearchRequest(queryEmbedding []float32, nResults int, docIDs []string) SearchRequest {
	searchReq := SearchRequest{
		QueryEmbeddings: [][]float32{queryEmbedding},
		NResults:        nResults,
	}

	// Add document ID filter if provided
	if len(docIDs) > 0 {
		searchReq.Where = map[string]interface{}{
			"doc_id": map[string]interface{}{
				"$in": docIDs,
			},
		}
	}

	return searchReq
}

// executeSearchRequest executes the search request and returns the response
func (c *Client) executeSearchRequest(searchReq SearchRequest) (SearchResponse, error) {
	url := fmt.Sprintf("%s/api/v1/collections/%s/query", c.baseURL, c.collection)

	jsonPayload, err := json.Marshal(searchReq)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("failed to marshal search request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return SearchResponse{}, fmt.Errorf("failed to create search request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.makeRequest(req)
	if err != nil {
		return SearchResponse{}, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Debug("Failed to close response body", zap.Error(err))
		}
	}()

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return SearchResponse{}, fmt.Errorf("failed to decode search response: %w", err)
	}

	return searchResp, nil
}

// processSearchResponse converts the search response to SearchResult slice
func (c *Client) processSearchResponse(searchResp SearchResponse) []SearchResult {
	results := []SearchResult{}
	if len(searchResp.IDs) == 0 {
		return results
	}

	for i, id := range searchResp.IDs[0] {
		result := SearchResult{
			ID:       id,
			Content:  searchResp.Documents[0][i],
			Distance: searchResp.Distances[0][i],
		}

		// Convert metadata if available
		result.Metadata = c.convertMetadata(searchResp.Metadatas, i)
		results = append(results, result)
	}

	return results
}

// convertMetadata converts metadata from the search response to string map
func (c *Client) convertMetadata(metadatas [][]map[string]interface{}, index int) map[string]string {
	if len(metadatas) == 0 || len(metadatas[0]) <= index {
		return nil
	}

	metadata := make(map[string]string)
	for k, v := range metadatas[0][index] {
		if str, ok := v.(string); ok {
			metadata[k] = str
		}
	}

	return metadata
}

// HealthCheck checks if ChromaDB is healthy
func (c *Client) HealthCheck() error {
	c.logger.Info("Performing health check", zap.String("url", c.baseURL))

	return c.retryWithBackoff(func() error {
		url := fmt.Sprintf("%s/api/v1/heartbeat", c.baseURL)

		resp, err := c.httpClient.Get(url)
		if err != nil {
			return fmt.Errorf("failed to check ChromaDB health: %w", err)
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				c.logger.Debug("Failed to close response body", zap.Error(err))
			}
		}()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("ChromaDB health check failed with status %d", resp.StatusCode)
		}

		c.logger.Info("Health check successful")
		return nil
	}, "HealthCheck")
}

// CreateCollection creates a new collection in ChromaDB
func (c *Client) CreateCollection(name string, metadata map[string]interface{}) error {
	c.logger.Info("Creating collection",
		zap.String("collection_name", name),
		zap.Any("metadata", metadata))

	return c.retryWithBackoff(func() error {
		url := fmt.Sprintf("%s/api/v1/collections", c.baseURL)

		req := CreateCollectionRequest{
			Name:     name,
			Metadata: metadata,
		}

		jsonPayload, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("failed to marshal create collection request: %w", err)
		}

		httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
		if err != nil {
			return fmt.Errorf("failed to create HTTP request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.makeRequest(httpReq)
		if err != nil {
			return err
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				c.logger.Debug("Failed to close response body", zap.Error(err))
			}
		}()

		c.logger.Info("Collection created successfully", zap.String("collection_name", name))
		return nil
	}, "CreateCollection")
}

// DeleteCollection deletes a collection from ChromaDB
func (c *Client) DeleteCollection(name string) error {
	c.logger.Info("Deleting collection", zap.String("collection_name", name))

	return c.retryWithBackoff(func() error {
		url := fmt.Sprintf("%s/api/v1/collections/%s", c.baseURL, name)

		req, err := http.NewRequest("DELETE", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create delete request: %w", err)
		}

		resp, err := c.makeRequest(req)
		if err != nil {
			return err
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				c.logger.Debug("Failed to close response body", zap.Error(err))
			}
		}()

		c.logger.Info("Collection deleted successfully", zap.String("collection_name", name))
		return nil
	}, "DeleteCollection")
}

// GetCollection retrieves collection information from ChromaDB
func (c *Client) GetCollection(name string) (*Collection, error) {
	c.logger.Info("Getting collection info", zap.String("collection_name", name))

	var collection *Collection
	err := c.retryWithBackoff(func() error {
		url := fmt.Sprintf("%s/api/v1/collections/%s", c.baseURL, name)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create get request: %w", err)
		}

		resp, err := c.makeRequest(req)
		if err != nil {
			return err
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				c.logger.Debug("Failed to close response body", zap.Error(err))
			}
		}()

		if err := json.NewDecoder(resp.Body).Decode(&collection); err != nil {
			return fmt.Errorf("failed to decode collection response: %w", err)
		}

		c.logger.Info("Collection info retrieved successfully", zap.String("collection_name", name))
		return nil
	}, "GetCollection")

	return collection, err
}

// ListCollections retrieves all collections from ChromaDB
func (c *Client) ListCollections() ([]Collection, error) {
	c.logger.Info("Listing all collections")

	var collections []Collection
	err := c.retryWithBackoff(func() error {
		url := fmt.Sprintf("%s/api/v1/collections", c.baseURL)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create list request: %w", err)
		}

		resp, err := c.makeRequest(req)
		if err != nil {
			return err
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				c.logger.Debug("Failed to close response body", zap.Error(err))
			}
		}()

		if err := json.NewDecoder(resp.Body).Decode(&collections); err != nil {
			return fmt.Errorf("failed to decode collections response: %w", err)
		}

		c.logger.Info("Collections listed successfully", zap.Int("collection_count", len(collections)))
		return nil
	}, "ListCollections")

	return collections, err
}
