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
	"encoding/json"
	"strings"
	"testing"

	"github.com/your-org/ai-sa-assistant/internal/config"
)

func TestConfigurationValidation(t *testing.T) {
	tests := []struct {
		name                       string
		fallbackThreshold          int
		fallbackScoreThreshold     float64
		expectedFallbackThreshold  int
		expectedFallbackScoreThreshold float64
	}{
		{
			name:                       "Default configuration values",
			fallbackThreshold:          3,
			fallbackScoreThreshold:     0.7,
			expectedFallbackThreshold:  3,
			expectedFallbackScoreThreshold: 0.7,
		},
		{
			name:                       "Custom configuration values",
			fallbackThreshold:          5,
			fallbackScoreThreshold:     0.8,
			expectedFallbackThreshold:  5,
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
				t.Errorf("Expected fallback score threshold %f, got %f", tt.expectedFallbackScoreThreshold, cfg.Retrieval.FallbackScoreThreshold)
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
			} else {
				if err == nil && searchReq.Query != "" {
					t.Error("Expected invalid request, but got valid one")
				}
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
				t.Errorf("Expected unmarshaled fallback triggered %v, got %v", tt.fallbackTriggered, unmarshaledResponse.FallbackTriggered)
			}

			// Check if fallback_reason is included in JSON when expected
			if tt.fallbackTriggered && !strings.Contains(string(jsonData), "fallback_reason") {
				t.Error("Expected fallback_reason field in JSON when fallback is triggered")
			}
		})
	}
}
