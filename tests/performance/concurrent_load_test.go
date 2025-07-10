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

	"github.com/stretchr/testify/assert"
)

const (
	maxConcurrentRequests = 20
	maxResponseTimeMS     = 30000 // 30 seconds
	minSuccessRate        = 0.95  // 95% success rate
)

// ConcurrentLoadStats tracks performance metrics for concurrent operations
type ConcurrentLoadStats struct {
	TotalRequests  int
	SuccessfulReqs int
	FailedReqs     int
	AverageTime    time.Duration
	MinTime        time.Duration
	MaxTime        time.Duration
	TotalTime      time.Duration
	SuccessRate    float64
	RequestsPerSec float64
	Errors         []string
}

// TestConcurrentTeamsWebhookProcessing tests concurrent Teams webhook processing
func TestConcurrentTeamsWebhookProcessing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent Teams webhook test in short mode")
	}

	if !servicesReady(t) {
		t.Skip("Services not available for concurrent Teams webhook testing")
	}

	// Test scenarios with varying concurrency levels
	concurrencyLevels := []int{5, 10, 15, 20}

	for _, concurrency := range concurrencyLevels {
		t.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(t *testing.T) {
			testConcurrentTeamsRequests(t, concurrency)
		})
	}
}

func testConcurrentTeamsRequests(t *testing.T, concurrency int) {
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	// Test queries for different scenarios
	testQueries := []string{
		"@SA-Assistant Generate a basic AWS migration plan",
		"@SA-Assistant Design a simple Azure hybrid architecture",
		"@SA-Assistant Create a disaster recovery overview",
		"@SA-Assistant Provide security compliance guidance",
		"@SA-Assistant Help with cloud cost optimization",
	}

	var wg sync.WaitGroup
	results := make(chan RequestResult, concurrency)

	startTime := time.Now()

	// Launch concurrent requests
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(requestID int) {
			defer wg.Done()

			query := testQueries[requestID%len(testQueries)]
			result := makeTeamsWebhookRequest(client, query, requestID)
			results <- result
		}(i)
	}

	// Wait for all requests to complete
	wg.Wait()
	close(results)

	totalTime := time.Since(startTime)

	// Analyze results
	stats := analyzeResults(results, totalTime)

	// Assertions
	assert.GreaterOrEqual(t, stats.SuccessRate, minSuccessRate,
		"Success rate should be at least %v%%", minSuccessRate*100)
	assert.Less(t, stats.MaxTime, time.Duration(maxResponseTimeMS)*time.Millisecond,
		"Maximum response time should be under %v ms", maxResponseTimeMS)
	assert.Equal(t, concurrency, stats.TotalRequests,
		"Total requests should match concurrency level")

	// Log performance metrics
	t.Logf("Concurrent Teams webhook results (concurrency=%d):", concurrency)
	t.Logf("  Total requests: %d", stats.TotalRequests)
	t.Logf("  Successful: %d", stats.SuccessfulReqs)
	t.Logf("  Failed: %d", stats.FailedReqs)
	t.Logf("  Success rate: %.2f%%", stats.SuccessRate*100)
	t.Logf("  Average response time: %v", stats.AverageTime)
	t.Logf("  Min response time: %v", stats.MinTime)
	t.Logf("  Max response time: %v", stats.MaxTime)
	t.Logf("  Requests per second: %.2f", stats.RequestsPerSec)
	t.Logf("  Total execution time: %v", totalTime)

	// Log errors if any
	if len(stats.Errors) > 0 {
		t.Logf("  Errors encountered:")
		for i, err := range stats.Errors {
			if i < 5 { // Log first 5 errors
				t.Logf("    - %s", err)
			}
		}
		if len(stats.Errors) > 5 {
			t.Logf("    ... and %d more errors", len(stats.Errors)-5)
		}
	}
}

// TestConcurrentRetrievalRequests tests concurrent retrieval API requests
func TestConcurrentRetrievalRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent retrieval test in short mode")
	}

	if !servicesReady(t) {
		t.Skip("Services not available for concurrent retrieval testing")
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	concurrency := 15
	var wg sync.WaitGroup
	results := make(chan RequestResult, concurrency)

	// Test queries for retrieval
	testQueries := []string{
		"AWS migration strategies",
		"Azure hybrid architecture",
		"disaster recovery planning",
		"security compliance requirements",
		"cloud cost optimization techniques",
		"monitoring and alerting setup",
		"backup and recovery procedures",
		"network security best practices",
	}

	startTime := time.Now()

	// Launch concurrent retrieval requests
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(requestID int) {
			defer wg.Done()

			query := testQueries[requestID%len(testQueries)]
			result := makeRetrievalRequest(client, query, requestID)
			results <- result
		}(i)
	}

	wg.Wait()
	close(results)

	totalTime := time.Since(startTime)
	stats := analyzeResults(results, totalTime)

	// Assertions
	assert.GreaterOrEqual(t, stats.SuccessRate, minSuccessRate,
		"Retrieval success rate should be at least %v%%", minSuccessRate*100)
	assert.Less(t, stats.MaxTime, 10*time.Second,
		"Maximum retrieval response time should be under 10 seconds")

	// Log results
	t.Logf("Concurrent retrieval results (concurrency=%d):", concurrency)
	t.Logf("  Success rate: %.2f%%", stats.SuccessRate*100)
	t.Logf("  Average response time: %v", stats.AverageTime)
	t.Logf("  Max response time: %v", stats.MaxTime)
	t.Logf("  Requests per second: %.2f", stats.RequestsPerSec)
}

// TestConcurrentSynthesisRequests tests concurrent synthesis API requests
func TestConcurrentSynthesisRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent synthesis test in short mode")
	}

	if !servicesReady(t) {
		t.Skip("Services not available for concurrent synthesis testing")
	}

	client := &http.Client{
		Timeout: 45 * time.Second,
	}

	concurrency := 10 // Lower concurrency for synthesis due to API limits
	var wg sync.WaitGroup
	results := make(chan RequestResult, concurrency)

	startTime := time.Now()

	// Launch concurrent synthesis requests
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(requestID int) {
			defer wg.Done()

			result := makeSynthesisRequest(client, requestID)
			results <- result
		}(i)
	}

	wg.Wait()
	close(results)

	totalTime := time.Since(startTime)
	stats := analyzeResults(results, totalTime)

	// Assertions - slightly lower success rate for synthesis due to API complexity
	assert.GreaterOrEqual(t, stats.SuccessRate, 0.8,
		"Synthesis success rate should be at least 80%")
	assert.Less(t, stats.MaxTime, 45*time.Second,
		"Maximum synthesis response time should be under 45 seconds")

	// Log results
	t.Logf("Concurrent synthesis results (concurrency=%d):", concurrency)
	t.Logf("  Success rate: %.2f%%", stats.SuccessRate*100)
	t.Logf("  Average response time: %v", stats.AverageTime)
	t.Logf("  Max response time: %v", stats.MaxTime)
	t.Logf("  Requests per second: %.2f", stats.RequestsPerSec)
}

// TestConcurrentWebSearchRequests tests concurrent web search API requests
func TestConcurrentWebSearchRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent web search test in short mode")
	}

	if !servicesReady(t) {
		t.Skip("Services not available for concurrent web search testing")
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	concurrency := 12
	var wg sync.WaitGroup
	results := make(chan RequestResult, concurrency)

	// Test queries that should trigger web search
	testQueries := []string{
		"latest AWS announcements 2024",
		"recent Azure updates December 2024",
		"new GCP features 2024",
		"current cloud security trends",
		"latest migration tools 2024",
		"recent compliance updates",
	}

	startTime := time.Now()

	// Launch concurrent web search requests
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(requestID int) {
			defer wg.Done()

			query := testQueries[requestID%len(testQueries)]
			result := makeWebSearchRequest(client, query, requestID)
			results <- result
		}(i)
	}

	wg.Wait()
	close(results)

	totalTime := time.Since(startTime)
	stats := analyzeResults(results, totalTime)

	// Assertions
	assert.GreaterOrEqual(t, stats.SuccessRate, 0.8,
		"Web search success rate should be at least 80%")
	assert.Less(t, stats.MaxTime, 30*time.Second,
		"Maximum web search response time should be under 30 seconds")

	// Log results
	t.Logf("Concurrent web search results (concurrency=%d):", concurrency)
	t.Logf("  Success rate: %.2f%%", stats.SuccessRate*100)
	t.Logf("  Average response time: %v", stats.AverageTime)
	t.Logf("  Max response time: %v", stats.MaxTime)
	t.Logf("  Requests per second: %.2f", stats.RequestsPerSec)
}

// TestDatabaseConnectionPooling tests database connection pooling under load
func TestDatabaseConnectionPooling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database connection pooling test in short mode")
	}

	if !servicesReady(t) {
		t.Skip("Services not available for database connection pooling testing")
	}

	client := &http.Client{
		Timeout: 20 * time.Second,
	}

	// Test with high concurrency to stress connection pooling
	concurrency := 25
	var wg sync.WaitGroup
	results := make(chan RequestResult, concurrency)

	startTime := time.Now()

	// Launch concurrent requests that will stress the database
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(requestID int) {
			defer wg.Done()

			// Make multiple sequential requests to test connection reuse
			for j := 0; j < 3; j++ {
				query := fmt.Sprintf("cloud architecture pattern %d", requestID)
				result := makeRetrievalRequest(client, query, requestID*10+j)
				results <- result
			}
		}(i)
	}

	wg.Wait()
	close(results)

	totalTime := time.Since(startTime)
	stats := analyzeResults(results, totalTime)

	// Assertions
	assert.GreaterOrEqual(t, stats.SuccessRate, 0.9,
		"Database connection pooling success rate should be at least 90%")
	assert.Less(t, stats.MaxTime, 15*time.Second,
		"Maximum response time with connection pooling should be under 15 seconds")

	// Log results
	t.Logf("Database connection pooling results (concurrency=%d, requests=%d):", concurrency, concurrency*3)
	t.Logf("  Success rate: %.2f%%", stats.SuccessRate*100)
	t.Logf("  Average response time: %v", stats.AverageTime)
	t.Logf("  Max response time: %v", stats.MaxTime)
	t.Logf("  Requests per second: %.2f", stats.RequestsPerSec)
}

// TestResponseTimeConsistency tests response time consistency under load
func TestResponseTimeConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping response time consistency test in short mode")
	}

	if !servicesReady(t) {
		t.Skip("Services not available for response time consistency testing")
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	// Test with moderate concurrency over longer duration
	concurrency := 8
	requestsPerWorker := 5
	var wg sync.WaitGroup
	results := make(chan RequestResult, concurrency*requestsPerWorker)

	startTime := time.Now()

	// Launch workers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < requestsPerWorker; j++ {
				query := fmt.Sprintf("@SA-Assistant Generate a migration plan for scenario %d", workerID*requestsPerWorker+j)
				result := makeTeamsWebhookRequest(client, query, workerID*requestsPerWorker+j)
				results <- result

				// Small delay between requests to simulate real usage
				time.Sleep(500 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	close(results)

	totalTime := time.Since(startTime)
	stats := analyzeResults(results, totalTime)

	// Calculate response time consistency metrics
	responseTimes := make([]time.Duration, 0, len(results))
	for result := range results {
		if result.Error == nil {
			responseTimes = append(responseTimes, result.ResponseTime)
		}
	}

	// Calculate standard deviation
	mean := stats.AverageTime
	var variance float64
	for _, rt := range responseTimes {
		diff := float64(rt - mean)
		variance += diff * diff
	}
	variance /= float64(len(responseTimes))
	stdDev := time.Duration(variance) // Approximate standard deviation

	// Assertions
	assert.GreaterOrEqual(t, stats.SuccessRate, 0.9,
		"Response time consistency success rate should be at least 90%")

	// Response times should be relatively consistent (std dev < 50% of mean)
	maxStdDev := time.Duration(float64(mean) * 0.5)
	assert.Less(t, stdDev, maxStdDev,
		"Response time standard deviation should be less than 50%% of mean")

	// Log results
	t.Logf("Response time consistency results:")
	t.Logf("  Total requests: %d", stats.TotalRequests)
	t.Logf("  Success rate: %.2f%%", stats.SuccessRate*100)
	t.Logf("  Average response time: %v", stats.AverageTime)
	t.Logf("  Min response time: %v", stats.MinTime)
	t.Logf("  Max response time: %v", stats.MaxTime)
	t.Logf("  Standard deviation: %v", stdDev)
	t.Logf("  Coefficient of variation: %.2f", float64(stdDev)/float64(mean))
}

// Helper types and functions

type RequestResult struct {
	RequestID    int
	ResponseTime time.Duration
	StatusCode   int
	Error        error
}

func makeTeamsWebhookRequest(client *http.Client, query string, requestID int) RequestResult {
	request := map[string]interface{}{
		"text": query,
		"type": "message",
		"from": map[string]interface{}{
			"id":   fmt.Sprintf("test_user_%d", requestID),
			"name": "Test User",
		},
	}

	body, _ := json.Marshal(request)

	start := time.Now()
	resp, err := client.Post("http://localhost:8080/teams-webhook", "application/json", bytes.NewBuffer(body))
	responseTime := time.Since(start)

	result := RequestResult{
		RequestID:    requestID,
		ResponseTime: responseTime,
		Error:        err,
	}

	if err == nil {
		result.StatusCode = resp.StatusCode
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log error but don't fail the test
			fmt.Printf("Failed to close response body: %v\n", closeErr)
		}
	}

	return result
}

func makeRetrievalRequest(client *http.Client, query string, requestID int) RequestResult {
	request := map[string]interface{}{
		"query": query,
		"filters": map[string]interface{}{
			"platform": "aws",
		},
		"limit": 10,
	}

	body, _ := json.Marshal(request)

	start := time.Now()
	resp, err := client.Post("http://localhost:8081/retrieve", "application/json", bytes.NewBuffer(body))
	responseTime := time.Since(start)

	result := RequestResult{
		RequestID:    requestID,
		ResponseTime: responseTime,
		Error:        err,
	}

	if err == nil {
		result.StatusCode = resp.StatusCode
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log error but don't fail the test
			fmt.Printf("Failed to close response body: %v\n", closeErr)
		}
	}

	return result
}

func makeSynthesisRequest(client *http.Client, requestID int) RequestResult {
	request := map[string]interface{}{
		"query": fmt.Sprintf("Generate a cloud migration plan for request %d", requestID),
		"context": []map[string]interface{}{
			{
				"content": "AWS migration best practices",
				"source":  "internal_docs",
			},
		},
		"web_results": []map[string]interface{}{
			{
				"content": "Latest AWS migration tools",
				"source":  "aws.amazon.com",
			},
		},
	}

	body, _ := json.Marshal(request)

	start := time.Now()
	resp, err := client.Post("http://localhost:8082/synthesize", "application/json", bytes.NewBuffer(body))
	responseTime := time.Since(start)

	result := RequestResult{
		RequestID:    requestID,
		ResponseTime: responseTime,
		Error:        err,
	}

	if err == nil {
		result.StatusCode = resp.StatusCode
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log error but don't fail the test
			fmt.Printf("Failed to close response body: %v\n", closeErr)
		}
	}

	return result
}

func makeWebSearchRequest(client *http.Client, query string, requestID int) RequestResult {
	request := map[string]interface{}{
		"query": query,
	}

	body, _ := json.Marshal(request)

	start := time.Now()
	resp, err := client.Post("http://localhost:8083/search", "application/json", bytes.NewBuffer(body))
	responseTime := time.Since(start)

	result := RequestResult{
		RequestID:    requestID,
		ResponseTime: responseTime,
		Error:        err,
	}

	if err == nil {
		result.StatusCode = resp.StatusCode
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log error but don't fail the test
			fmt.Printf("Failed to close response body: %v\n", closeErr)
		}
	}

	return result
}

func analyzeResults(results <-chan RequestResult, totalTime time.Duration) *ConcurrentLoadStats {
	var stats ConcurrentLoadStats
	var responseTimes []time.Duration

	stats.MinTime = time.Hour // Initialize with large value

	for result := range results {
		stats.TotalRequests++

		if result.Error == nil && result.StatusCode >= 200 && result.StatusCode < 300 {
			stats.SuccessfulReqs++
			responseTimes = append(responseTimes, result.ResponseTime)

			stats.TotalTime += result.ResponseTime

			if result.ResponseTime > stats.MaxTime {
				stats.MaxTime = result.ResponseTime
			}
			if result.ResponseTime < stats.MinTime {
				stats.MinTime = result.ResponseTime
			}
		} else {
			stats.FailedReqs++
			if result.Error != nil {
				stats.Errors = append(stats.Errors, fmt.Sprintf("Request %d: %v", result.RequestID, result.Error))
			} else {
				stats.Errors = append(stats.Errors, fmt.Sprintf("Request %d: HTTP %d", result.RequestID, result.StatusCode))
			}
		}
	}

	if stats.SuccessfulReqs > 0 {
		stats.AverageTime = stats.TotalTime / time.Duration(stats.SuccessfulReqs)
	}

	stats.SuccessRate = float64(stats.SuccessfulReqs) / float64(stats.TotalRequests)

	if totalTime > 0 {
		stats.RequestsPerSec = float64(stats.TotalRequests) / totalTime.Seconds()
	}

	return &stats
}
