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
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

const (
	testChromaDBURL = "http://localhost:8001"
)

// Integration test setup functions

// servicesAvailable checks if all required services are available
func servicesAvailable() bool {
	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	services := []string{
		"http://localhost:8000/api/v1/heartbeat", // ChromaDB (production)
		testChromaDBURL + "/api/v1/heartbeat",    // ChromaDB (test)
		"http://localhost:8081/health",           // Retrieve service
		"http://localhost:8082/health",           // Synthesize service
		"http://localhost:8083/health",           // Web search service
		"http://localhost:8080/health",           // Teams bot service
	}

	chromaAvailable := false
	servicesAvailable := 0

	for _, url := range services {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			cancel()
			continue
		}

		resp, err := client.Do(req)
		cancel()
		if err != nil {
			continue
		}
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			if url == "http://localhost:8000/api/v1/heartbeat" || url == testChromaDBURL+"/api/v1/heartbeat" {
				chromaAvailable = true
			} else {
				servicesAvailable++
			}
		}
	}

	// For basic integration tests, we need at least ChromaDB
	// For full integration tests, we need ChromaDB and at least 3 other services
	if chromaAvailable && servicesAvailable >= 3 {
		return true // Full integration test environment
	}
	
	// Allow ChromaDB-only tests if CHROMADB_ONLY_TESTS is set
	if chromaAvailable && os.Getenv("CHROMADB_ONLY_TESTS") == "true" {
		return true
	}
	
	return false
}

// getChromaURL returns the appropriate ChromaDB URL based on what's available
func getChromaURL() string {
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	// Check test port first (8001)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	req, err := http.NewRequestWithContext(ctx, "GET", testChromaDBURL+"/api/v1/heartbeat", nil)
	if err == nil {
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			cancel()
			return testChromaDBURL
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
	}
	cancel()

	// Check production port (8000)
	ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)
	req, err = http.NewRequestWithContext(ctx, "GET", "http://localhost:8000/api/v1/heartbeat", nil)
	if err == nil {
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			cancel()
			return "http://localhost:8000"
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
	}
	cancel()

	// Default to production port
	return "http://localhost:8000"
}

// skipIfNoServices skips the test if required services are not available
func skipIfNoServices(t *testing.T) {
	if !servicesAvailable() {
		t.Skip("Required services not available for integration testing")
	}
}
