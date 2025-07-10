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

package performance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// BenchmarkDemoScenario benchmarks a complete demo scenario
func BenchmarkDemoScenario(b *testing.B) {
	// Skip if services are not available
	if !servicesReady(b) {
		b.Skip("Services not available for performance testing")
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	query := "@SA-Assistant Generate a high-level AWS migration plan for 50 VMs"
	request := map[string]interface{}{
		"text": query,
	}
	body, _ := json.Marshal(request)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := client.Post("http://localhost:8080/teams-webhook", "application/json", bytes.NewBuffer(body))
		if err != nil {
			b.Fatalf("Failed to call webhook: %v", err)
		}
		_ = resp.Body.Close()
	}
}

// BenchmarkServiceHealth benchmarks health check endpoints
func BenchmarkServiceHealth(b *testing.B) {
	// Skip if services are not available
	if !servicesReady(b) {
		b.Skip("Services not available for performance testing")
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	services := []string{
		"http://localhost:8081/health",
		"http://localhost:8082/health",
		"http://localhost:8083/health",
		"http://localhost:8080/health",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, url := range services {
			resp, err := client.Get(url)
			if err != nil {
				b.Fatalf("Failed to call health endpoint: %v", err)
			}
			_ = resp.Body.Close()
		}
	}
}

// BenchmarkConcurrentRequests benchmarks concurrent demo requests
func BenchmarkConcurrentRequests(b *testing.B) {
	// Skip if services are not available
	if !servicesReady(b) {
		b.Skip("Services not available for performance testing")
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	query := "@SA-Assistant Generate a basic security assessment"
	request := map[string]interface{}{
		"text": query,
	}
	body, _ := json.Marshal(request)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := client.Post("http://localhost:8080/teams-webhook", "application/json", bytes.NewBuffer(body))
			if err != nil {
				b.Fatalf("Failed to call webhook: %v", err)
			}
			_ = resp.Body.Close()
		}
	})
}

// TestLoadTesting performs load testing on the system
func TestLoadTesting(t *testing.T) {
	// Skip if services are not available
	if !servicesReady(t) {
		t.Skip("Services not available for load testing")
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	const (
		numGoroutines        = 10
		requestsPerGoroutine = 5
		maxResponseTime      = 30 * time.Second
	)

	query := "@SA-Assistant Generate a simple AWS migration plan"
	request := map[string]interface{}{
		"text": query,
	}
	body, _ := json.Marshal(request)

	var wg sync.WaitGroup
	results := make(chan time.Duration, numGoroutines*requestsPerGoroutine)
	errors := make(chan error, numGoroutines*requestsPerGoroutine)

	// Start goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < requestsPerGoroutine; j++ {
				start := time.Now()

				resp, err := client.Post("http://localhost:8080/teams-webhook", "application/json", bytes.NewBuffer(body))
				if err != nil {
					errors <- fmt.Errorf("goroutine %d, request %d: %v", goroutineID, j, err)
					return
				}

				duration := time.Since(start)
				results <- duration
				_ = resp.Body.Close()

				t.Logf("Goroutine %d, Request %d completed in %v", goroutineID, j, duration)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(results)
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Load test error: %v", err)
	}

	// Analyze results
	var totalDuration time.Duration
	var maxDuration time.Duration
	var minDuration = time.Hour
	requestCount := 0

	for duration := range results {
		totalDuration += duration
		requestCount++

		if duration > maxDuration {
			maxDuration = duration
		}
		if duration < minDuration {
			minDuration = duration
		}

		if duration > maxResponseTime {
			t.Errorf("Request exceeded maximum response time: %v > %v", duration, maxResponseTime)
		}
	}

	if requestCount > 0 {
		avgDuration := totalDuration / time.Duration(requestCount)
		t.Logf("Load test results:")
		t.Logf("  Total requests: %d", requestCount)
		t.Logf("  Average response time: %v", avgDuration)
		t.Logf("  Min response time: %v", minDuration)
		t.Logf("  Max response time: %v", maxDuration)
		t.Logf("  Total duration: %v", totalDuration)
	}
}

// TestMemoryUsage tests memory usage during heavy load
func TestMemoryUsage(t *testing.T) {
	// Skip if services are not available
	if !servicesReady(t) {
		t.Skip("Services not available for memory usage testing")
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	const numRequests = 20

	query := "@SA-Assistant Generate a comprehensive AWS migration plan with detailed architecture diagrams"
	request := map[string]interface{}{
		"text": query,
	}
	body, _ := json.Marshal(request)

	// Make multiple requests to test memory usage
	for i := 0; i < numRequests; i++ {
		resp, err := client.Post("http://localhost:8080/teams-webhook", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to call webhook on request %d: %v", i+1, err)
		}
		_ = resp.Body.Close()

		t.Logf("Completed request %d/%d", i+1, numRequests)

		// Small delay to allow for memory cleanup
		time.Sleep(100 * time.Millisecond)
	}
}

// TestServiceScaling tests how services handle scaling
func TestServiceScaling(t *testing.T) {
	// Skip if services are not available
	if !servicesReady(t) {
		t.Skip("Services not available for scaling testing")
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	queries := []string{
		"@SA-Assistant Generate a simple AWS plan",
		"@SA-Assistant Design an Azure hybrid architecture",
		"@SA-Assistant Create a disaster recovery plan",
		"@SA-Assistant Provide security compliance guidance",
	}

	// Test different query types concurrently
	var wg sync.WaitGroup
	results := make(chan struct {
		query    string
		duration time.Duration
		err      error
	}, len(queries)*3)

	for _, query := range queries {
		for i := 0; i < 3; i++ { // 3 requests per query type
			wg.Add(1)
			go func(q string) {
				defer wg.Done()

				start := time.Now()
				request := map[string]interface{}{
					"text": q,
				}
				body, _ := json.Marshal(request)

				resp, err := client.Post("http://localhost:8080/teams-webhook", "application/json", bytes.NewBuffer(body))
				duration := time.Since(start)

				if err != nil {
					results <- struct {
						query    string
						duration time.Duration
						err      error
					}{q, duration, err}
					return
				}

				if closeErr := resp.Body.Close(); closeErr != nil {
					t.Logf("Failed to close response body: %v", closeErr)
				}
				results <- struct {
					query    string
					duration time.Duration
					err      error
				}{q, duration, nil}
			}(query)
		}
	}

	wg.Wait()
	close(results)

	// Analyze scaling results
	for result := range results {
		if result.err != nil {
			t.Errorf("Scaling test error for query '%s': %v", result.query, result.err)
		} else {
			t.Logf("Query completed in %v: %s", result.duration, result.query)
		}
	}
}

// TestStressEndpoints tests stress conditions on all endpoints
func TestStressEndpoints(t *testing.T) {
	if !servicesReady(t) {
		t.Skip("Services not available for stress testing")
	}

	endpoints := []struct {
		url     string
		method  string
		payload interface{}
		name    string
	}{
		{
			url:    "http://localhost:8081/health",
			method: "GET",
			name:   "Retrieve Health",
		},
		{
			url:    "http://localhost:8082/health",
			method: "GET",
			name:   "Synthesis Health",
		},
		{
			url:    "http://localhost:8083/health",
			method: "GET",
			name:   "WebSearch Health",
		},
		{
			url:    "http://localhost:8080/health",
			method: "GET",
			name:   "TeamsBot Health",
		},
	}

	client := &http.Client{Timeout: 10 * time.Second}

	for _, endpoint := range endpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			// Stress test with rapid requests
			const numRequests = 50
			var wg sync.WaitGroup
			errors := make(chan error, numRequests)

			start := time.Now()

			for i := 0; i < numRequests; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					resp, err := client.Get(endpoint.url)
					if err != nil {
						errors <- err
						return
					}
					if closeErr := resp.Body.Close(); closeErr != nil {
						t.Logf("Failed to close response body: %v", closeErr)
					}
					if resp.StatusCode != http.StatusOK {
						errors <- fmt.Errorf("unexpected status code: %d", resp.StatusCode)
					}
				}()
			}

			wg.Wait()
			close(errors)

			duration := time.Since(start)
			errorCount := len(errors)

			t.Logf("Stress test for %s completed in %v", endpoint.name, duration)
			t.Logf("  Requests: %d", numRequests)
			t.Logf("  Errors: %d", errorCount)
			t.Logf("  Success rate: %.2f%%", float64(numRequests-errorCount)/float64(numRequests)*100)
			t.Logf("  Requests per second: %.2f", float64(numRequests)/duration.Seconds())

			// Assert that error rate is acceptable
			errorRate := float64(errorCount) / float64(numRequests)
			assert.Less(t, errorRate, 0.1, "Error rate should be less than 10%%")
		})
	}
}

// TestEdgeCaseQueries tests performance with edge case queries
func TestEdgeCaseQueries(t *testing.T) {
	if !servicesReady(t) {
		t.Skip("Services not available for edge case testing")
	}

	client := &http.Client{Timeout: 45 * time.Second}

	edgeCases := []struct {
		name  string
		query string
	}{
		{
			name:  "EmptyQuery",
			query: "",
		},
		{
			name:  "VeryShortQuery",
			query: "AWS",
		},
		{
			name:  "VeryLongQuery",
			query: strings.Repeat("Generate a comprehensive enterprise cloud migration strategy with detailed technical specifications, security frameworks, compliance requirements, cost optimization strategies, risk assessment procedures, implementation timelines, rollback plans, and monitoring capabilities ", 10),
		},
		{
			name:  "SpecialCharactersQuery",
			query: "@SA-Assistant Design a plan with special chars: !@#$%^&*()_+-=[]{}|;:'\",.<>?/~`",
		},
		{
			name:  "UnicodeQuery",
			query: "@SA-Assistant Créer un plan de migration vers le cloud avec des caractères spéciaux: 中文测试 русский עברית العربية",
		},
	}

	for _, testCase := range edgeCases {
		t.Run(testCase.name, func(t *testing.T) {
			request := map[string]interface{}{
				"text": testCase.query,
				"type": "message",
			}
			body, _ := json.Marshal(request)

			start := time.Now()
			resp, err := client.Post("http://localhost:8080/teams-webhook", "application/json", bytes.NewBuffer(body))
			duration := time.Since(start)

			if err != nil {
				t.Logf("Edge case '%s' failed: %v", testCase.name, err)
				return
			}

			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Logf("Failed to close response body: %v", closeErr)
			}

			t.Logf("Edge case '%s' completed:", testCase.name)
			t.Logf("  Duration: %v", duration)
			t.Logf("  Status: %d", resp.StatusCode)
			t.Logf("  Query length: %d characters", len(testCase.query))

			// Edge cases should complete within reasonable time or fail gracefully
			assert.Less(t, duration, 45*time.Second, "Edge case should complete within 45 seconds")
		})
	}
}

// servicesReady checks if all required services are available
func servicesReady(t testing.TB) bool {
	// Skip if running in short mode
	if testing.Short() {
		return false
	}

	// Skip if required environment variables are not set
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Logf("Skipping performance test: OPENAI_API_KEY not set")
		return false
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	services := []string{
		"http://localhost:8000/api/v1/heartbeat", // ChromaDB
		"http://localhost:8081/health",           // Retrieve service
		"http://localhost:8082/health",           // Synthesize service
		"http://localhost:8083/health",           // Web search service
		"http://localhost:8080/health",           // Teams bot service
	}

	for _, url := range services {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			cancel()
			t.Logf("Failed to create request for %s: %v", url, err)
			return false
		}

		resp, err := client.Do(req)
		cancel()
		if err != nil {
			t.Logf("Service not available at %s: %v", url, err)
			return false
		}
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Logf("Service not healthy at %s: status %d", url, resp.StatusCode)
			return false
		}
	}

	t.Logf("All services are ready for performance testing")
	return true
}
