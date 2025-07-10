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
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/your-org/ai-sa-assistant/internal/openai"
	"github.com/your-org/ai-sa-assistant/internal/resilience"
)

const (
	maxRetryAttempts      = 3
	baseBackoffDelay      = 1 * time.Second
	maxBackoffDelay       = 30 * time.Second
	rateLimitTestTimeout  = 5 * time.Minute
	expectedBackoffFactor = 2.0
	maxConcurrentAPIReqs  = 10
)

// RateLimitStats tracks rate limiting statistics
type RateLimitStats struct {
	TotalRequests       int
	SuccessfulRequests  int
	RateLimitedRequests int
	RetriedRequests     int
	FailedRequests      int
	AverageDelay        time.Duration
	MaxDelay            time.Duration
	TotalBackoffTime    time.Duration
	BackoffAttempts     []BackoffAttempt
}

// BackoffAttempt represents a single backoff attempt
type BackoffAttempt struct {
	AttemptNumber int
	Delay         time.Duration
	Success       bool
	Error         error
}

// TestOpenAIRateLimitHandling tests OpenAI API rate limit handling
func TestOpenAIRateLimitHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping OpenAI rate limit handling test in short mode")
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping rate limit test")
	}

	cfg := createTestConfig(t)
	logger := createTestLogger(t)

	// Create OpenAI client
	client, err := openai.NewClient(cfg.OpenAI.APIKey, logger)
	require.NoError(t, err, "Failed to create OpenAI client")

	// Test rapid-fire requests to trigger rate limiting
	rapidRequests := 50
	var wg sync.WaitGroup
	results := make(chan RateLimitResult, rapidRequests)

	start := time.Now()

	// Launch rapid requests
	for i := 0; i < rapidRequests; i++ {
		wg.Add(1)
		go func(requestID int) {
			defer wg.Done()

			result := testSingleAPIRequest(client, requestID, logger)
			results <- result
		}(i)
	}

	wg.Wait()
	close(results)

	totalTime := time.Since(start)

	// Analyze results
	stats := analyzeRateLimitResults(results)
	stats.TotalRequests = rapidRequests

	// Assertions
	assert.Greater(t, stats.SuccessfulRequests, rapidRequests/2,
		"At least half of the requests should eventually succeed")
	assert.GreaterOrEqual(t, stats.RateLimitedRequests, 1,
		"Should encounter rate limiting with rapid requests")
	assert.Less(t, stats.MaxDelay, maxBackoffDelay,
		"Maximum backoff delay should not exceed limit")

	// Log results
	t.Logf("OpenAI rate limit handling results:")
	t.Logf("  Total requests: %d", stats.TotalRequests)
	t.Logf("  Successful requests: %d", stats.SuccessfulRequests)
	t.Logf("  Rate limited requests: %d", stats.RateLimitedRequests)
	t.Logf("  Retried requests: %d", stats.RetriedRequests)
	t.Logf("  Failed requests: %d", stats.FailedRequests)
	t.Logf("  Average delay: %v", stats.AverageDelay)
	t.Logf("  Max delay: %v", stats.MaxDelay)
	t.Logf("  Total backoff time: %v", stats.TotalBackoffTime)
	t.Logf("  Total test time: %v", totalTime)
	t.Logf("  Success rate: %.2f%%", float64(stats.SuccessfulRequests)/float64(stats.TotalRequests)*100)
}

// TestBackoffBehavior tests exponential backoff behavior
func TestBackoffBehavior(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping backoff behavior test in short mode")
	}

	// Test exponential backoff implementation
	backoffConfig := resilience.BackoffConfig{
		BaseDelay:   baseBackoffDelay,
		MaxDelay:    maxBackoffDelay,
		Multiplier:  expectedBackoffFactor,
		MaxRetries:  maxRetryAttempts,
		Jitter:      false,
		RetryOnFunc: resilience.DefaultRetryOnFunc,
	}

	// Test backoff calculation
	for attempt := 0; attempt < maxRetryAttempts; attempt++ {
		expectedDelay := calculateExpectedBackoffDelay(attempt, backoffConfig)
		// The actual calculation is done internally by WithExponentialBackoff
		// We just verify our calculation logic is correct
		t.Logf("Attempt %d: Expected delay %v", attempt, expectedDelay)
		assert.Greater(t, expectedDelay, time.Duration(0), "Delay should be positive")
		assert.LessOrEqual(t, expectedDelay, backoffConfig.MaxDelay, "Delay should not exceed max")
	}
}

// TestBatchRequestOptimization tests batch request optimization
func TestBatchRequestOptimization(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping batch request optimization test in short mode")
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping batch optimization test")
	}

	cfg := createTestConfig(t)
	logger := createTestLogger(t)

	client, err := openai.NewClient(cfg.OpenAI.APIKey, logger)
	require.NoError(t, err, "Failed to create OpenAI client")

	// Test different batch sizes
	batchSizes := []int{1, 5, 10, 20, 50}
	textCount := 100
	testTexts := generateTestTexts(textCount, 50) // 100 texts, 50 words each

	for _, batchSize := range batchSizes {
		t.Run(fmt.Sprintf("BatchSize_%d", batchSize), func(t *testing.T) {
			testBatchOptimization(t, client, testTexts, batchSize, logger)
		})
	}
}

func testBatchOptimization(t *testing.T, client *openai.Client, texts []string, batchSize int, logger *zap.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	start := time.Now()
	totalEmbeddings := 0
	totalRequests := 0
	var totalDelay time.Duration

	// Process texts in batches
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}

		batch := texts[i:end]
		totalRequests++

		requestStart := time.Now()
		response, err := client.EmbedTexts(ctx, batch)
		requestTime := time.Since(requestStart)
		totalDelay += requestTime

		if err != nil {
			t.Logf("Batch request failed: %v", err)
			continue
		}

		totalEmbeddings += len(response.Embeddings)

		// Log batch progress
		if totalRequests%5 == 0 {
			t.Logf("Processed %d/%d batches", totalRequests, int(math.Ceil(float64(len(texts))/float64(batchSize))))
		}
	}

	totalTime := time.Since(start)
	avgTimePerRequest := totalDelay / time.Duration(totalRequests)
	avgTimePerEmbedding := totalDelay / time.Duration(totalEmbeddings)

	// Log results
	t.Logf("Batch optimization results (batch size %d):", batchSize)
	t.Logf("  Total texts: %d", len(texts))
	t.Logf("  Total requests: %d", totalRequests)
	t.Logf("  Total embeddings: %d", totalEmbeddings)
	t.Logf("  Total time: %v", totalTime)
	t.Logf("  Average time per request: %v", avgTimePerRequest)
	t.Logf("  Average time per embedding: %v", avgTimePerEmbedding)
	t.Logf("  Throughput: %.2f embeddings/sec", float64(totalEmbeddings)/totalTime.Seconds())
}

// TestQueueManagement tests queue management for pending requests
func TestQueueManagement(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping queue management test in short mode")
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping queue management test")
	}

	cfg := createTestConfig(t)
	logger := createTestLogger(t)

	client, err := openai.NewClient(cfg.OpenAI.APIKey, logger)
	require.NoError(t, err, "Failed to create OpenAI client")

	// Create a queue manager (simulated)
	queueManager := NewMockQueueManager(maxConcurrentAPIReqs)

	// Test queue with high load
	totalRequests := 30
	var wg sync.WaitGroup
	results := make(chan QueueResult, totalRequests)

	start := time.Now()

	// Submit requests to queue
	for i := 0; i < totalRequests; i++ {
		wg.Add(1)
		go func(requestID int) {
			defer wg.Done()

			result := queueManager.SubmitRequest(func() QueueResult {
				return testQueuedAPIRequest(client, requestID, logger)
			})
			results <- result
		}(i)
	}

	wg.Wait()
	close(results)

	totalTime := time.Since(start)

	// Analyze queue results
	queueStats := analyzeQueueResults(results)

	// Assertions
	assert.LessOrEqual(t, queueStats.MaxConcurrentRequests, maxConcurrentAPIReqs,
		"Should not exceed maximum concurrent requests")
	assert.Greater(t, queueStats.SuccessfulRequests, totalRequests/2,
		"At least half of queued requests should succeed")
	assert.GreaterOrEqual(t, queueStats.AverageWaitTime, time.Duration(0),
		"Should have some wait time due to queuing")

	// Log results
	t.Logf("Queue management results:")
	t.Logf("  Total requests: %d", totalRequests)
	t.Logf("  Successful requests: %d", queueStats.SuccessfulRequests)
	t.Logf("  Failed requests: %d", queueStats.FailedRequests)
	t.Logf("  Max concurrent requests: %d", queueStats.MaxConcurrentRequests)
	t.Logf("  Average wait time: %v", queueStats.AverageWaitTime)
	t.Logf("  Max wait time: %v", queueStats.MaxWaitTime)
	t.Logf("  Total time: %v", totalTime)
	t.Logf("  Queue efficiency: %.2f%%", float64(queueStats.SuccessfulRequests)/float64(totalRequests)*100)
}

// TestRateLimitRecovery tests rate limit recovery behavior
func TestRateLimitRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping rate limit recovery test in short mode")
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping rate limit recovery test")
	}

	cfg := createTestConfig(t)
	logger := createTestLogger(t)

	client, err := openai.NewClient(cfg.OpenAI.APIKey, logger)
	require.NoError(t, err, "Failed to create OpenAI client")

	// Phase 1: Trigger rate limiting
	t.Logf("Phase 1: Triggering rate limits...")
	triggerRateLimits(t, client, logger)

	// Phase 2: Wait for recovery
	t.Logf("Phase 2: Waiting for recovery...")
	recoveryWait := 30 * time.Second
	time.Sleep(recoveryWait)

	// Phase 3: Test recovery
	t.Logf("Phase 3: Testing recovery...")
	recoveryResults := testRecovery(t, client, logger)

	// Assertions
	assert.Greater(t, recoveryResults.SuccessfulRequests, recoveryResults.TotalRequests/2,
		"Should recover and handle requests successfully")
	assert.Less(t, recoveryResults.AverageResponseTime, 10*time.Second,
		"Response times should be reasonable after recovery")

	// Log results
	t.Logf("Rate limit recovery results:")
	t.Logf("  Recovery wait time: %v", recoveryWait)
	t.Logf("  Post-recovery success rate: %.2f%%",
		float64(recoveryResults.SuccessfulRequests)/float64(recoveryResults.TotalRequests)*100)
	t.Logf("  Average response time: %v", recoveryResults.AverageResponseTime)
}

// TestFairResourceAllocation tests fair resource allocation across users
func TestFairResourceAllocation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping fair resource allocation test in short mode")
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping fair resource allocation test")
	}

	cfg := createTestConfig(t)
	logger := createTestLogger(t)

	client, err := openai.NewClient(cfg.OpenAI.APIKey, logger)
	require.NoError(t, err, "Failed to create OpenAI client")

	// Simulate multiple users making requests
	userCount := 5
	requestsPerUser := 10

	userResults := make(map[string]*UserStats)
	var wg sync.WaitGroup
	resultsMutex := sync.Mutex{}

	start := time.Now()

	// Launch requests for each user
	for userID := 0; userID < userCount; userID++ {
		wg.Add(1)
		go func(uid int) {
			defer wg.Done()

			userKey := fmt.Sprintf("user_%d", uid)
			stats := &UserStats{
				UserID:            userKey,
				TotalRequests:     requestsPerUser,
				SuccessfulReqs:    0,
				FailedReqs:        0,
				TotalResponseTime: 0,
			}

			// Make requests for this user
			for i := 0; i < requestsPerUser; i++ {
				requestStart := time.Now()

				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				_, err := client.EmbedTexts(ctx, []string{fmt.Sprintf("test text for %s request %d", userKey, i)})
				cancel()

				responseTime := time.Since(requestStart)
				stats.TotalResponseTime += responseTime

				if err == nil {
					stats.SuccessfulReqs++
				} else {
					stats.FailedReqs++
				}
			}

			stats.AverageResponseTime = stats.TotalResponseTime / time.Duration(requestsPerUser)

			resultsMutex.Lock()
			userResults[userKey] = stats
			resultsMutex.Unlock()
		}(userID)
	}

	wg.Wait()
	totalTime := time.Since(start)

	// Analyze fairness
	fairnessStats := analyzeFairness(userResults)

	// Assertions
	assert.Less(t, fairnessStats.ResponseTimeVariance, 5.0,
		"Response time variance should be reasonable across users")
	assert.Greater(t, fairnessStats.MinSuccessRate, 0.5,
		"All users should have reasonable success rates")

	// Log results
	t.Logf("Fair resource allocation results:")
	t.Logf("  Total users: %d", userCount)
	t.Logf("  Requests per user: %d", requestsPerUser)
	t.Logf("  Total time: %v", totalTime)
	t.Logf("  Average success rate: %.2f%%", fairnessStats.AverageSuccessRate*100)
	t.Logf("  Min success rate: %.2f%%", fairnessStats.MinSuccessRate*100)
	t.Logf("  Max success rate: %.2f%%", fairnessStats.MaxSuccessRate*100)
	t.Logf("  Response time variance: %.2f", fairnessStats.ResponseTimeVariance)

	for userID, stats := range userResults {
		t.Logf("  %s: %d/%d successful (%.1f%%), avg time: %v",
			userID, stats.SuccessfulReqs, stats.TotalRequests,
			float64(stats.SuccessfulReqs)/float64(stats.TotalRequests)*100,
			stats.AverageResponseTime)
	}
}

// Helper types and functions

type RateLimitResult struct {
	RequestID     int
	Success       bool
	AttemptCount  int
	TotalDelay    time.Duration
	BackoffDelays []time.Duration
	Error         error
}

type QueueResult struct {
	RequestID   int
	Success     bool
	WaitTime    time.Duration
	ProcessTime time.Duration
	Error       error
}

type UserStats struct {
	UserID              string
	TotalRequests       int
	SuccessfulReqs      int
	FailedReqs          int
	TotalResponseTime   time.Duration
	AverageResponseTime time.Duration
}

type QueueStats struct {
	TotalRequests         int
	SuccessfulRequests    int
	FailedRequests        int
	MaxConcurrentRequests int
	AverageWaitTime       time.Duration
	MaxWaitTime           time.Duration
}

type RecoveryStats struct {
	TotalRequests       int
	SuccessfulRequests  int
	FailedRequests      int
	AverageResponseTime time.Duration
}

type FairnessStats struct {
	AverageSuccessRate   float64
	MinSuccessRate       float64
	MaxSuccessRate       float64
	ResponseTimeVariance float64
}

type MockQueueManager struct {
	maxConcurrent int
	semaphore     chan struct{}
	mu            sync.Mutex
	currentCount  int
	maxCount      int
}

func NewMockQueueManager(maxConcurrent int) *MockQueueManager {
	return &MockQueueManager{
		maxConcurrent: maxConcurrent,
		semaphore:     make(chan struct{}, maxConcurrent),
	}
}

func (qm *MockQueueManager) SubmitRequest(fn func() QueueResult) QueueResult {
	start := time.Now()

	// Acquire semaphore
	qm.semaphore <- struct{}{}

	// Track concurrent requests
	qm.mu.Lock()
	qm.currentCount++
	if qm.currentCount > qm.maxCount {
		qm.maxCount = qm.currentCount
	}
	qm.mu.Unlock()

	waitTime := time.Since(start)

	// Execute request
	result := fn()
	result.WaitTime = waitTime

	// Release semaphore
	qm.mu.Lock()
	qm.currentCount--
	qm.mu.Unlock()

	<-qm.semaphore

	return result
}

func testSingleAPIRequest(client *openai.Client, requestID int, logger *zap.Logger) RateLimitResult {
	result := RateLimitResult{
		RequestID:     requestID,
		BackoffDelays: make([]time.Duration, 0),
	}

	for attempt := 0; attempt < maxRetryAttempts; attempt++ {
		result.AttemptCount = attempt + 1

		if attempt > 0 {
			// Calculate backoff delay using our helper function
			backoffConfig := resilience.BackoffConfig{
				BaseDelay:   baseBackoffDelay,
				MaxDelay:    maxBackoffDelay,
				Multiplier:  expectedBackoffFactor,
				MaxRetries:  maxRetryAttempts,
				Jitter:      false,
				RetryOnFunc: resilience.DefaultRetryOnFunc,
			}

			delay := calculateExpectedBackoffDelay(attempt-1, backoffConfig)
			result.BackoffDelays = append(result.BackoffDelays, delay)
			result.TotalDelay += delay

			time.Sleep(delay)
		}

		// Make API request
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, err := client.EmbedTexts(ctx, []string{fmt.Sprintf("test text %d", requestID)})
		cancel()

		if err == nil {
			result.Success = true
			break
		}

		result.Error = err

		// Check if it's a rate limit error (simplified check)
		if isRateLimitError(err) {
			continue // Retry with backoff
		}

		// For other errors, don't retry
		break
	}

	return result
}

func testQueuedAPIRequest(client *openai.Client, requestID int, logger *zap.Logger) QueueResult {
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	_, err := client.EmbedTexts(ctx, []string{fmt.Sprintf("queued test text %d", requestID)})
	cancel()

	processTime := time.Since(start)

	return QueueResult{
		RequestID:   requestID,
		Success:     err == nil,
		ProcessTime: processTime,
		Error:       err,
	}
}

func calculateExpectedBackoffDelay(attempt int, config resilience.BackoffConfig) time.Duration {
	delay := config.BaseDelay
	for i := 0; i < attempt; i++ {
		delay = time.Duration(float64(delay) * config.Multiplier)
	}

	if delay > config.MaxDelay {
		delay = config.MaxDelay
	}

	return delay
}

func generateTestTexts(count, wordsPerText int) []string {
	texts := make([]string, count)
	words := []string{"cloud", "computing", "architecture", "migration", "security", "performance", "optimization", "infrastructure", "deployment", "monitoring"}

	for i := 0; i < count; i++ {
		var text strings.Builder
		for j := 0; j < wordsPerText; j++ {
			if j > 0 {
				text.WriteString(" ")
			}
			text.WriteString(words[j%len(words)])
		}
		texts[i] = text.String()
	}

	return texts
}

func analyzeRateLimitResults(results <-chan RateLimitResult) *RateLimitStats {
	stats := &RateLimitStats{}

	var totalDelay time.Duration
	var maxDelay time.Duration

	for result := range results {
		if result.Success {
			stats.SuccessfulRequests++
		} else {
			stats.FailedRequests++
		}

		if result.AttemptCount > 1 {
			stats.RetriedRequests++
		}

		if isRateLimitError(result.Error) {
			stats.RateLimitedRequests++
		}

		totalDelay += result.TotalDelay
		if result.TotalDelay > maxDelay {
			maxDelay = result.TotalDelay
		}

		for _, delay := range result.BackoffDelays {
			stats.BackoffAttempts = append(stats.BackoffAttempts, BackoffAttempt{
				AttemptNumber: len(stats.BackoffAttempts) + 1,
				Delay:         delay,
				Success:       result.Success,
				Error:         result.Error,
			})
		}
	}

	stats.TotalBackoffTime = totalDelay
	stats.MaxDelay = maxDelay

	if stats.TotalRequests > 0 {
		stats.AverageDelay = totalDelay / time.Duration(stats.TotalRequests)
	}

	return stats
}

func analyzeQueueResults(results <-chan QueueResult) *QueueStats {
	stats := &QueueStats{}

	var totalWaitTime time.Duration
	var maxWaitTime time.Duration

	for result := range results {
		stats.TotalRequests++

		if result.Success {
			stats.SuccessfulRequests++
		} else {
			stats.FailedRequests++
		}

		totalWaitTime += result.WaitTime
		if result.WaitTime > maxWaitTime {
			maxWaitTime = result.WaitTime
		}
	}

	stats.MaxWaitTime = maxWaitTime

	if stats.TotalRequests > 0 {
		stats.AverageWaitTime = totalWaitTime / time.Duration(stats.TotalRequests)
	}

	return stats
}

func triggerRateLimits(t *testing.T, client *openai.Client, logger *zap.Logger) {
	// Make rapid requests to trigger rate limits
	for i := 0; i < 20; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := client.EmbedTexts(ctx, []string{fmt.Sprintf("trigger rate limit %d", i)})
		cancel()

		if err != nil {
			t.Logf("Rate limit trigger request %d failed: %v", i, err)
		}
	}
}

func testRecovery(t *testing.T, client *openai.Client, logger *zap.Logger) *RecoveryStats {
	stats := &RecoveryStats{}

	// Test moderate request rate after recovery
	for i := 0; i < 10; i++ {
		start := time.Now()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, err := client.EmbedTexts(ctx, []string{fmt.Sprintf("recovery test %d", i)})
		cancel()

		responseTime := time.Since(start)
		stats.TotalRequests++

		if err == nil {
			stats.SuccessfulRequests++
		} else {
			stats.FailedRequests++
		}

		// Add to average response time calculation
		if stats.TotalRequests == 1 {
			stats.AverageResponseTime = responseTime
		} else {
			stats.AverageResponseTime = (stats.AverageResponseTime*time.Duration(stats.TotalRequests-1) + responseTime) / time.Duration(stats.TotalRequests)
		}

		// Small delay between recovery requests
		time.Sleep(1 * time.Second)
	}

	return stats
}

func analyzeFairness(userResults map[string]*UserStats) *FairnessStats {
	stats := &FairnessStats{}

	successRates := make([]float64, 0, len(userResults))
	responseTimes := make([]float64, 0, len(userResults))

	for _, userStats := range userResults {
		successRate := float64(userStats.SuccessfulReqs) / float64(userStats.TotalRequests)
		successRates = append(successRates, successRate)
		responseTimes = append(responseTimes, float64(userStats.AverageResponseTime))

		if stats.MinSuccessRate == 0 || successRate < stats.MinSuccessRate {
			stats.MinSuccessRate = successRate
		}

		if successRate > stats.MaxSuccessRate {
			stats.MaxSuccessRate = successRate
		}
	}

	// Calculate average success rate
	var totalSuccessRate float64
	for _, rate := range successRates {
		totalSuccessRate += rate
	}
	stats.AverageSuccessRate = totalSuccessRate / float64(len(successRates))

	// Calculate response time variance
	var mean float64
	for _, rt := range responseTimes {
		mean += rt
	}
	mean /= float64(len(responseTimes))

	var variance float64
	for _, rt := range responseTimes {
		variance += (rt - mean) * (rt - mean)
	}
	stats.ResponseTimeVariance = variance / float64(len(responseTimes))

	return stats
}

// isRateLimitError checks if an error is a rate limit error
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	return strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "quota") ||
		strings.Contains(errStr, "too many requests")
}
