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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/ai-sa-assistant/internal/metadata"
)


// ResourceUsageStats tracks resource usage metrics
type ResourceUsageStats struct {
	MemoryUsage    MemoryStats
	GoroutineCount int
	FileHandles    int
	NetworkConns   int
	GCStats        GCStats
	DiskIO         DiskIOStats
}

// MemoryStats tracks memory usage statistics
type MemoryStats struct {
	AllocBytes      uint64
	TotalAllocBytes uint64
	SysBytes        uint64
	HeapBytes       uint64
	StackBytes      uint64
	GCCycles        uint32
}

// GCStats tracks garbage collection statistics
type GCStats struct {
	NumGC       uint32
	PauseTotal  time.Duration
	PauseNs     []uint64
	LastGC      time.Time
	NextGC      uint64
	MemoryFreed uint64
}

// DiskIOStats tracks disk I/O statistics
type DiskIOStats struct {
	ReadBytes    uint64
	WriteBytes   uint64
	ReadOps      uint64
	WriteOps     uint64
	SQLiteReads  uint64
	SQLiteWrites uint64
}

// TestVectorSearchMemoryUsage tests memory usage during vector search operations
func TestVectorSearchMemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping vector search memory usage test in short mode")
	}

	if !servicesReady(t) {
		t.Skip("Services not available for vector search memory usage testing")
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Baseline memory measurement
	runtime.GC()
	var memBefore, memAfter runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	// Perform multiple vector searches with different query sizes
	testQueries := []string{
		"simple query",
		"more complex query about cloud architecture and migration strategies",
		"very detailed and comprehensive query about enterprise cloud migration including security considerations, compliance requirements, network architecture, data migration strategies, application modernization, cost optimization, and performance monitoring",
		strings.Repeat("comprehensive enterprise cloud migration strategy ", 50), // Long query
	}

	var totalRequests int
	var successfulRequests int

	for i := 0; i < 25; i++ { // Multiple rounds of searches
		for _, query := range testQueries {
			result := makeRetrievalRequest(client, query, totalRequests)
			totalRequests++

			if result.Error == nil && result.StatusCode >= 200 && result.StatusCode < 300 {
				successfulRequests++
			}

			// Force GC periodically to test memory cleanup
			if totalRequests%10 == 0 {
				runtime.GC()
			}
		}
	}

	// Final memory measurement
	runtime.GC()
	runtime.ReadMemStats(&memAfter)

	// Calculate memory usage
	memoryUsed := memAfter.Alloc - memBefore.Alloc
	memoryGrowth := memAfter.TotalAlloc - memBefore.TotalAlloc

	// Assertions
	assert.Greater(t, successfulRequests, totalRequests/2,
		"At least half of the requests should succeed")
	assert.Less(t, memoryUsed, uint64(500*1024*1024),
		"Memory usage should be less than 500MB")

	// Memory growth should be reasonable relative to work done
	if totalRequests <= 0 {
		t.Fatal("Invalid total requests count")
	}
	avgMemoryPerRequest := memoryGrowth / uint64(totalRequests)
	assert.Less(t, avgMemoryPerRequest, uint64(10*1024*1024),
		"Average memory per request should be less than 10MB")

	// Log results
	t.Logf("Vector search memory usage results:")
	t.Logf("  Total requests: %d", totalRequests)
	t.Logf("  Successful requests: %d", successfulRequests)
	t.Logf("  Memory used: %.2f MB", float64(memoryUsed)/1024/1024)
	t.Logf("  Total memory growth: %.2f MB", float64(memoryGrowth)/1024/1024)
	t.Logf("  Average memory per request: %.2f KB", float64(avgMemoryPerRequest)/1024)
	t.Logf("  GC cycles: %d", memAfter.NumGC-memBefore.NumGC)
	t.Logf("  Heap size: %.2f MB", float64(memAfter.HeapAlloc)/1024/1024)
}

// TestGarbageCollectionBehavior tests garbage collection behavior with large responses
func TestGarbageCollectionBehavior(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping garbage collection behavior test in short mode")
	}

	if !servicesReady(t) {
		t.Skip("Services not available for garbage collection behavior testing")
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	// Force GC and get baseline stats
	runtime.GC()
	var gcBefore, gcAfter runtime.MemStats
	runtime.ReadMemStats(&gcBefore)

	// Generate large responses to test GC behavior
	largeQueries := []string{
		"@SA-Assistant Generate a comprehensive AWS migration plan with detailed architecture diagrams, step-by-step procedures, security considerations, and cost analysis",
		"@SA-Assistant Create a complete disaster recovery plan including backup strategies, failover procedures, testing protocols, and documentation",
		"@SA-Assistant Design a hybrid cloud architecture with detailed network topology, security zones, data flow diagrams, and integration patterns",
	}

	var totalRequests int
	var totalResponseSize uint64

	for i := 0; i < 10; i++ { // Generate multiple large responses
		for _, query := range largeQueries {
			start := time.Now()

			request := map[string]interface{}{
				"text": query,
				"type": "message",
			}
			body, _ := json.Marshal(request)

			resp, err := client.Post("http://localhost:8080/teams-webhook", "application/json", bytes.NewBuffer(body))
			if err == nil {
				// Read response body to simulate memory allocation
				buffer := make([]byte, 1024)
				for {
					n, err := resp.Body.Read(buffer)
					if n >= 0 {
						totalResponseSize += uint64(n)
					}
					if err != nil {
						break
					}
				}
				resp.Body.Close()
			}

			totalRequests++

			// Track memory usage
			var memDuring runtime.MemStats
			runtime.ReadMemStats(&memDuring)

			t.Logf("Request %d: Response size: %d bytes, Heap: %.2f MB, GC cycles: %d",
				totalRequests, totalResponseSize, float64(memDuring.HeapAlloc)/1024/1024, memDuring.NumGC)

			duration := time.Since(start)
			t.Logf("  Duration: %v", duration)

			// Force GC every few requests to test cleanup
			if totalRequests%3 == 0 {
				runtime.GC()
			}
		}
	}

	// Final GC and measurements
	runtime.GC()
	runtime.ReadMemStats(&gcAfter)

	// Calculate GC statistics
	gcCycles := gcAfter.NumGC - gcBefore.NumGC
	totalGCPause := gcAfter.PauseTotalNs - gcBefore.PauseTotalNs
	if gcCycles == 0 {
		t.Fatal("No GC cycles observed")
	}
	var avgGCPause time.Duration
	if gcCycles > 0 && totalGCPause <= uint64(1<<63-1) {
		pausePerCycle := totalGCPause / uint64(gcCycles)
		if pausePerCycle <= uint64(1<<63-1) {
			avgGCPause = time.Duration(pausePerCycle)
		} else {
			avgGCPause = time.Duration(0)
		}
	} else {
		avgGCPause = time.Duration(0)
	}

	// Assertions
	assert.Greater(t, gcCycles, uint32(5),
		"Should have triggered multiple GC cycles")
	assert.Less(t, avgGCPause, 50*time.Millisecond,
		"Average GC pause should be less than 50ms")
	assert.Less(t, gcAfter.HeapAlloc, uint64(1024*1024*1024),
		"Final heap size should be less than 1GB")

	// Log results
	t.Logf("Garbage collection behavior results:")
	t.Logf("  Total requests: %d", totalRequests)
	t.Logf("  Total response size: %.2f MB", float64(totalResponseSize)/1024/1024)
	t.Logf("  GC cycles triggered: %d", gcCycles)
	if totalGCPause <= uint64(1<<63-1) {
		t.Logf("  Total GC pause time: %v", time.Duration(totalGCPause))
	} else {
		t.Logf("  Total GC pause time: %d ns (overflow)", totalGCPause)
	}
	t.Logf("  Average GC pause: %v", avgGCPause)
	t.Logf("  Final heap size: %.2f MB", float64(gcAfter.HeapAlloc)/1024/1024)
	t.Logf("  Memory freed: %.2f MB", float64(gcAfter.TotalAlloc-gcBefore.TotalAlloc)/1024/1024)
}

// TestDiskIOPerformance tests disk I/O performance with SQLite operations
func TestDiskIOPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping disk I/O performance test in short mode")
	}

	_ = createTestConfig(t)
	logger := createTestLogger(t)

	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_diskio.db")

	// Track file operations
	var diskIOBefore, diskIOAfter DiskIOStats
	diskIOBefore = getDiskIOStats(dbPath)

	// Create metadata store
	metadataStore, err := metadata.NewStore(dbPath, logger)
	require.NoError(t, err, "Failed to create metadata store")
	defer metadataStore.Close()

	// Perform intensive disk I/O operations
	entryCount := 5000
	batchSize := 100

	start := time.Now()

	for i := 0; i < entryCount; i += batchSize {
		// Create batch of entries
		entries := make([]metadata.Entry, batchSize)
		for j := 0; j < batchSize; j++ {
			entries[j] = metadata.Entry{
				DocID:         fmt.Sprintf("diskio_test_%d", i+j),
				Title:         fmt.Sprintf("Disk I/O Test Document %d", i+j),
				Path:          fmt.Sprintf("docs/diskio_test_%d.md", i+j),
				Platform:      []string{"aws", "azure", "gcp"}[(i+j)%3],
				Scenario:      []string{"migration", "hybrid", "dr", "security"}[(i+j)%4],
				Type:          "playbook",
				SourceURL:     fmt.Sprintf("https://example.com/doc_%d", i+j),
				Tags:          []string{fmt.Sprintf("tag_%d", (i+j)%5)},
				Difficulty:    "intermediate",
				EstimatedTime: "2-hours",
			}
		}

		// Insert batch
		for k := range entries {
			err = metadataStore.AddMetadata(entries[k])
			require.NoError(t, err, "Failed to insert entry")
		}

		// Perform reads
		if i%500 == 0 {
			_, err = metadataStore.FilterDocuments(metadata.FilterOptions{Platform: "aws"})
			require.NoError(t, err, "Failed to read by platform")

			_, err = metadataStore.FilterDocuments(metadata.FilterOptions{Scenario: "migration"})
			require.NoError(t, err, "Failed to read by scenario")
		}
	}

	totalTime := time.Since(start)
	diskIOAfter = getDiskIOStats(dbPath)

	// Calculate I/O statistics
	readOps := diskIOAfter.ReadOps - diskIOBefore.ReadOps
	writeOps := diskIOAfter.WriteOps - diskIOBefore.WriteOps
	readBytes := diskIOAfter.ReadBytes - diskIOBefore.ReadBytes
	writeBytes := diskIOAfter.WriteBytes - diskIOBefore.WriteBytes

	// Assertions
	if entryCount <= 0 {
		t.Fatal("Invalid entry count")
	}
	var minWriteOps uint64
	if entryCount > 0 {
		// Safe conversion to avoid integer overflow
		expectedOps := entryCount / 10
		if expectedOps >= 0 {
			minWriteOps = uint64(expectedOps)
		}
	}
	assert.Greater(t, writeOps, minWriteOps,
		"Should have performed significant write operations")
	assert.Greater(t, readOps, uint64(0),
		"Should have performed read operations")
	assert.Less(t, totalTime, 30*time.Second,
		"Disk I/O operations should complete within 30 seconds")

	// Log results
	t.Logf("Disk I/O performance results:")
	t.Logf("  Total entries: %d", entryCount)
	t.Logf("  Total time: %v", totalTime)
	t.Logf("  Read operations: %d", readOps)
	t.Logf("  Write operations: %d", writeOps)
	t.Logf("  Bytes read: %d", readBytes)
	t.Logf("  Bytes written: %d", writeBytes)
	t.Logf("  Operations per second: %.2f", float64(readOps+writeOps)/totalTime.Seconds())
	t.Logf("  Throughput: %.2f KB/s", float64(readBytes+writeBytes)/totalTime.Seconds()/1024)
}

// TestNetworkConnectionUsage tests network connection usage and pooling
func TestNetworkConnectionUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network connection usage test in short mode")
	}

	if !servicesReady(t) {
		t.Skip("Services not available for network connection usage testing")
	}

	// Test multiple clients with different connection pools
	clients := []*http.Client{
		{Timeout: 30 * time.Second}, // Default client
		{Timeout: 30 * time.Second, Transport: &http.Transport{MaxIdleConns: 10}},
		{Timeout: 30 * time.Second, Transport: &http.Transport{MaxIdleConns: 50}},
	}

	for i, client := range clients {
		t.Run(fmt.Sprintf("Client_%d", i), func(t *testing.T) {
			testNetworkConnectionsForClient(t, client, i)
		})
	}
}

func testNetworkConnectionsForClient(t *testing.T, client *http.Client, clientID int) {
	// Track goroutines before
	goroutinesBefore := runtime.NumGoroutine()

	// Make concurrent requests to test connection pooling
	concurrency := 20
	var wg sync.WaitGroup
	results := make(chan RequestResult, concurrency)

	start := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(requestID int) {
			defer wg.Done()

			// Make multiple requests per goroutine
			for j := 0; j < 3; j++ {
				query := fmt.Sprintf("network test query %d-%d", requestID, j)
				result := makeRetrievalRequest(client, query, requestID*10+j)
				results <- result
			}
		}(i)
	}

	wg.Wait()
	close(results)

	totalTime := time.Since(start)
	goroutinesAfter := runtime.NumGoroutine()

	// Analyze results
	var successCount, errorCount int
	for result := range results {
		if result.Error == nil && result.StatusCode >= 200 && result.StatusCode < 300 {
			successCount++
		} else {
			errorCount++
		}
	}

	totalRequests := concurrency * 3
	successRate := float64(successCount) / float64(totalRequests)
	goroutineGrowth := goroutinesAfter - goroutinesBefore

	// Assertions
	assert.GreaterOrEqual(t, successRate, 0.9,
		"Network connection success rate should be at least 90%")
	assert.Less(t, goroutineGrowth, 50,
		"Goroutine growth should be reasonable")
	assert.Less(t, totalTime, 45*time.Second,
		"Network operations should complete within 45 seconds")

	// Log results
	t.Logf("Network connection usage results (client %d):", clientID)
	t.Logf("  Total requests: %d", totalRequests)
	t.Logf("  Successful requests: %d", successCount)
	t.Logf("  Failed requests: %d", errorCount)
	t.Logf("  Success rate: %.2f%%", successRate*100)
	t.Logf("  Total time: %v", totalTime)
	t.Logf("  Goroutines before: %d", goroutinesBefore)
	t.Logf("  Goroutines after: %d", goroutinesAfter)
	t.Logf("  Goroutine growth: %d", goroutineGrowth)
}

// TestResourceCleanupAfterRequests tests resource cleanup after request completion
func TestResourceCleanupAfterRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping resource cleanup test in short mode")
	}

	if !servicesReady(t) {
		t.Skip("Services not available for resource cleanup testing")
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Baseline measurements
	runtime.GC()
	var memBefore, memAfter runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	goroutinesBefore := runtime.NumGoroutine()

	// Perform multiple rounds of requests
	rounds := 5
	requestsPerRound := 20

	for round := 0; round < rounds; round++ {
		t.Logf("Starting round %d/%d", round+1, rounds)

		// Make concurrent requests
		var wg sync.WaitGroup
		for i := 0; i < requestsPerRound; i++ {
			wg.Add(1)
			go func(requestID int) {
				defer wg.Done()

				query := fmt.Sprintf("@SA-Assistant Generate plan for round %d request %d", round, requestID)
				result := makeTeamsWebhookRequest(client, query, requestID)

				if result.Error != nil {
					t.Logf("Request failed: %v", result.Error)
				}
			}(i)
		}
		wg.Wait()

		// Force cleanup
		runtime.GC()
		time.Sleep(100 * time.Millisecond) // Allow cleanup to complete

		// Measure resources after each round
		var memDuring runtime.MemStats
		runtime.ReadMemStats(&memDuring)
		goroutinesDuring := runtime.NumGoroutine()

		t.Logf("Round %d completed:", round+1)
		t.Logf("  Heap size: %.2f MB", float64(memDuring.HeapAlloc)/1024/1024)
		t.Logf("  Goroutines: %d", goroutinesDuring)
		t.Logf("  GC cycles: %d", memDuring.NumGC)
	}

	// Final measurements
	runtime.GC()
	runtime.ReadMemStats(&memAfter)
	goroutinesAfter := runtime.NumGoroutine()

	// Calculate resource usage
	memoryGrowth := memAfter.HeapAlloc - memBefore.HeapAlloc
	goroutineGrowth := goroutinesAfter - goroutinesBefore

	// Assertions
	assert.Less(t, memoryGrowth, uint64(100*1024*1024),
		"Memory growth should be less than 100MB after cleanup")
	assert.Less(t, goroutineGrowth, 20,
		"Goroutine growth should be minimal after cleanup")

	// Log final results
	t.Logf("Resource cleanup results:")
	t.Logf("  Total rounds: %d", rounds)
	t.Logf("  Requests per round: %d", requestsPerRound)
	t.Logf("  Memory growth: %.2f MB", float64(memoryGrowth)/1024/1024)
	t.Logf("  Goroutine growth: %d", goroutineGrowth)
	t.Logf("  Final heap size: %.2f MB", float64(memAfter.HeapAlloc)/1024/1024)
	t.Logf("  GC cycles: %d", memAfter.NumGC-memBefore.NumGC)
}

// TestMemoryLeakDetection tests for memory leaks in long-running operations
func TestMemoryLeakDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory leak detection test in short mode")
	}

	if !servicesReady(t) {
		t.Skip("Services not available for memory leak detection testing")
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Track memory usage over time
	memoryReadings := make([]uint64, 0)

	// Perform operations over extended period
	iterations := 50
	for i := 0; i < iterations; i++ {
		// Make a request
		query := fmt.Sprintf("@SA-Assistant Generate plan %d", i)
		result := makeTeamsWebhookRequest(client, query, i)

		if result.Error != nil {
			t.Logf("Request %d failed: %v", i, result.Error)
		}

		// Force GC and measure memory
		runtime.GC()
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		memoryReadings = append(memoryReadings, mem.HeapAlloc)

		// Log progress
		if i%10 == 0 {
			t.Logf("Iteration %d: Heap size: %.2f MB", i, float64(mem.HeapAlloc)/1024/1024)
		}
	}

	// Analyze memory trend
	if len(memoryReadings) > 10 {
		// Calculate slope of memory usage over time
		startAvg := calculateAverage(memoryReadings[:10])
		endAvg := calculateAverage(memoryReadings[len(memoryReadings)-10:])

		memoryGrowthPercentage := (float64(endAvg) - float64(startAvg)) / float64(startAvg) * 100

		// Assertions
		assert.Less(t, memoryGrowthPercentage, 50.0,
			"Memory growth should be less than 50% over test duration")

		// Log results
		t.Logf("Memory leak detection results:")
		t.Logf("  Total iterations: %d", iterations)
		t.Logf("  Starting average memory: %.2f MB", float64(startAvg)/1024/1024)
		t.Logf("  Ending average memory: %.2f MB", float64(endAvg)/1024/1024)
		t.Logf("  Memory growth: %.2f%%", memoryGrowthPercentage)
		t.Logf("  Max memory usage: %.2f MB", float64(maxUint64(memoryReadings))/1024/1024)
		t.Logf("  Min memory usage: %.2f MB", float64(minUint64(memoryReadings))/1024/1024)
	}
}

// Helper functions

func getDiskIOStats(dbPath string) DiskIOStats {
	// This is a simplified version - in a real implementation,
	// you would use system calls to get actual disk I/O statistics
	stats := DiskIOStats{}

	if fileInfo, err := os.Stat(dbPath); err == nil {
		if size := fileInfo.Size(); size >= 0 {
			stats.WriteBytes = uint64(size)
			stats.WriteOps = 1
		}
	}

	return stats
}

func calculateAverage(values []uint64) uint64 {
	if len(values) == 0 {
		return 0
	}

	var sum uint64
	for _, v := range values {
		sum += v
	}

	return sum / uint64(len(values))
}

func maxUint64(values []uint64) uint64 {
	if len(values) == 0 {
		return 0
	}

	maxVal := values[0]
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}

	return maxVal
}

func minUint64(values []uint64) uint64 {
	if len(values) == 0 {
		return 0
	}

	minVal := values[0]
	for _, v := range values {
		if v < minVal {
			minVal = v
		}
	}

	return minVal
}
