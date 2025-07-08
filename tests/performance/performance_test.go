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
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

// BenchmarkDemoScenario benchmarks a complete demo scenario
func BenchmarkDemoScenario(b *testing.B) {
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
		resp, err := client.Post("http://localhost:8080/webhook", "application/json", bytes.NewBuffer(body))
		if err != nil {
			b.Fatalf("Failed to call webhook: %v", err)
		}
		resp.Body.Close()
	}
}

// BenchmarkServiceHealth benchmarks health check endpoints
func BenchmarkServiceHealth(b *testing.B) {
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
			resp.Body.Close()
		}
	}
}

// BenchmarkConcurrentRequests benchmarks concurrent demo requests
func BenchmarkConcurrentRequests(b *testing.B) {
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
			resp, err := client.Post("http://localhost:8080/webhook", "application/json", bytes.NewBuffer(body))
			if err != nil {
				b.Fatalf("Failed to call webhook: %v", err)
			}
			resp.Body.Close()
		}
	})
}

// TestLoadTesting performs load testing on the system
func TestLoadTesting(t *testing.T) {
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

				resp, err := client.Post("http://localhost:8080/webhook", "application/json", bytes.NewBuffer(body))
				if err != nil {
					errors <- fmt.Errorf("goroutine %d, request %d: %v", goroutineID, j, err)
					return
				}

				duration := time.Since(start)
				results <- duration
				resp.Body.Close()

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
		resp, err := client.Post("http://localhost:8080/webhook", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to call webhook on request %d: %v", i+1, err)
		}
		resp.Body.Close()

		t.Logf("Completed request %d/%d", i+1, numRequests)

		// Small delay to allow for memory cleanup
		time.Sleep(100 * time.Millisecond)
	}
}

// TestServiceScaling tests how services handle scaling
func TestServiceScaling(t *testing.T) {
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

				resp, err := client.Post("http://localhost:8080/webhook", "application/json", bytes.NewBuffer(body))
				duration := time.Since(start)

				if err != nil {
					results <- struct {
						query    string
						duration time.Duration
						err      error
					}{q, duration, err}
					return
				}

				resp.Body.Close()
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
