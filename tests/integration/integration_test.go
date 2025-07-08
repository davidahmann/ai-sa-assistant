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

//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestServiceHealthEndpoints tests that all services respond to health checks
func TestServiceHealthEndpoints(t *testing.T) {
	services := map[string]string{
		"retrieve":   "http://localhost:8081/health",
		"websearch":  "http://localhost:8083/health",
		"synthesize": "http://localhost:8082/health",
		"teamsbot":   "http://localhost:8080/health",
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	for serviceName, url := range services {
		t.Run(fmt.Sprintf("Health_%s", serviceName), func(t *testing.T) {
			resp, err := client.Get(url)
			if err != nil {
				t.Fatalf("Failed to call %s health endpoint: %v", serviceName, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
			}

			var healthResponse map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&healthResponse); err != nil {
				t.Errorf("Failed to decode health response: %v", err)
			}

			if status, ok := healthResponse["status"]; !ok || status != "healthy" {
				t.Errorf("Expected status 'healthy', got %v", status)
			}
		})
	}
}

// TestServiceInteraction tests basic interaction between services
func TestServiceInteraction(t *testing.T) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Test retrieve service search endpoint
	t.Run("RetrieveService_Search", func(t *testing.T) {
		searchRequest := map[string]interface{}{
			"query": "test query",
			"filters": map[string]string{
				"scenario": "test",
			},
		}
		body, _ := json.Marshal(searchRequest)

		resp, err := client.Post("http://localhost:8081/search", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to call retrieve search endpoint: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
		}
	})

	// Test websearch service
	t.Run("WebSearchService", func(t *testing.T) {
		searchRequest := map[string]interface{}{
			"query": "test query with recent updates",
		}
		body, _ := json.Marshal(searchRequest)

		resp, err := client.Post("http://localhost:8083/search", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to call websearch endpoint: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
		}
	})

	// Test synthesize service
	t.Run("SynthesizeService", func(t *testing.T) {
		synthesizeRequest := map[string]interface{}{
			"query":   "test query",
			"context": []string{"test context"},
			"sources": []string{"test source"},
		}
		body, _ := json.Marshal(synthesizeRequest)

		resp, err := client.Post("http://localhost:8082/synthesize", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to call synthesize endpoint: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
		}
	})
}

// TestChromaDBIntegration tests ChromaDB connectivity
func TestChromaDBIntegration(t *testing.T) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Test ChromaDB heartbeat
	resp, err := client.Get("http://localhost:8000/api/v1/heartbeat")
	if err != nil {
		t.Fatalf("Failed to connect to ChromaDB: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("ChromaDB heartbeat failed: %d", resp.StatusCode)
	}

	// Test ChromaDB version
	resp, err = client.Get("http://localhost:8000/api/v1/version")
	if err != nil {
		t.Fatalf("Failed to get ChromaDB version: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("ChromaDB version check failed: %d", resp.StatusCode)
	}
}
