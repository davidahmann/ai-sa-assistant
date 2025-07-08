package chroma

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client wraps the ChromaDB REST API
type Client struct {
	baseURL    string
	collection string
	httpClient *http.Client
}

// NewClient creates a new ChromaDB client
func NewClient(baseURL, collection string) *Client {
	return &Client{
		baseURL:    baseURL,
		collection: collection,
		httpClient: &http.Client{Timeout: 30 * time.Second},
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

// AddDocuments adds documents with embeddings to ChromaDB
func (c *Client) AddDocuments(documents []Document, embeddings [][]float32) error {
	url := fmt.Sprintf("%s/api/v1/collections/%s/add", c.baseURL, c.collection)

	// Prepare request payload
	var metadatas []map[string]string
	var ids []string

	for _, doc := range documents {
		metadatas = append(metadatas, doc.Metadata)
		ids = append(ids, doc.ID)
	}

	payload := map[string]interface{}{
		"documents":  documents,
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ChromaDB returned status %d", resp.StatusCode)
	}

	return nil
}

// Search performs a vector search in ChromaDB
func (c *Client) Search(queryEmbedding []float32, nResults int, docIDs []string) ([]SearchResult, error) {
	url := fmt.Sprintf("%s/api/v1/collections/%s/query", c.baseURL, c.collection)

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

	jsonPayload, err := json.Marshal(searchReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create search request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ChromaDB search returned status %d", resp.StatusCode)
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	// Convert response to SearchResult slice
	var results []SearchResult
	if len(searchResp.IDs) > 0 {
		for i, id := range searchResp.IDs[0] {
			result := SearchResult{
				ID:       id,
				Content:  searchResp.Documents[0][i],
				Distance: searchResp.Distances[0][i],
			}

			// Convert metadata
			if len(searchResp.Metadatas) > 0 && len(searchResp.Metadatas[0]) > i {
				result.Metadata = make(map[string]string)
				for k, v := range searchResp.Metadatas[0][i] {
					if str, ok := v.(string); ok {
						result.Metadata[k] = str
					}
				}
			}

			results = append(results, result)
		}
	}

	return results, nil
}

// HealthCheck checks if ChromaDB is healthy
func (c *Client) HealthCheck() error {
	url := fmt.Sprintf("%s/api/v1/heartbeat", c.baseURL)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("failed to check ChromaDB health: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ChromaDB health check failed with status %d", resp.StatusCode)
	}

	return nil
}
