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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	maxDemoResponseTime = 30 * time.Second // 30-second target for complex queries
	minDemoSuccessRate  = 0.8              // 80% success rate for demo scenarios
	maxConcurrentDemos  = 5                // Maximum concurrent demo scenarios
)

// DemoScenario represents a complete demo scenario
type DemoScenario struct {
	Name               string
	Query              string
	Description        string
	ExpectedComponents []string // Expected components in response
}

// DemoResult represents the result of a demo scenario execution
type DemoResult struct {
	Scenario     DemoScenario
	Success      bool
	ResponseTime time.Duration
	StatusCode   int
	ResponseSize int
	Components   []string // Components found in response
	Error        error
}

// E2EPerformanceStats tracks end-to-end performance statistics
type E2EPerformanceStats struct {
	TotalScenarios      int
	SuccessfulScenarios int
	FailedScenarios     int
	AverageResponseTime time.Duration
	MaxResponseTime     time.Duration
	MinResponseTime     time.Duration
	SuccessRate         float64
	ScenariosPerSecond  float64
	TotalDataTransfer   int64
}

// TestCompleteeDemoScenariosUnderLoad tests complete demo scenarios under load
func TestCompleteDemoScenariosUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping complete demo scenarios test in short mode")
	}

	if !servicesReady(t) {
		t.Skip("Services not available for complete demo scenarios testing")
	}

	scenarios := getStandardDemoScenarios()

	// Test each scenario individually first
	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			testSingleDemoScenario(t, scenario)
		})
	}
}

func testSingleDemoScenario(t *testing.T, scenario DemoScenario) {
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	result := executeDemoScenario(client, scenario, 0)

	// Assertions
	assert.True(t, result.Success, "Demo scenario should succeed: %s", scenario.Name)
	assert.Less(t, result.ResponseTime, maxDemoResponseTime,
		"Demo response time should be under 30 seconds for %s", scenario.Name)
	assert.Equal(t, http.StatusOK, result.StatusCode,
		"Demo should return 200 OK for %s", scenario.Name)
	assert.Greater(t, result.ResponseSize, 100,
		"Demo should return substantial response for %s", scenario.Name)

	// Check for expected components
	for _, expectedComponent := range scenario.ExpectedComponents {
		found := false
		for _, component := range result.Components {
			if strings.Contains(component, expectedComponent) {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected component '%s' not found in %s response",
			expectedComponent, scenario.Name)
	}

	// Log results
	t.Logf("Demo scenario '%s' results:", scenario.Name)
	t.Logf("  Success: %t", result.Success)
	t.Logf("  Response time: %v", result.ResponseTime)
	t.Logf("  Response size: %d bytes", result.ResponseSize)
	t.Logf("  Components found: %d", len(result.Components))

	if result.Error != nil {
		t.Logf("  Error: %v", result.Error)
	}
}

// TestThirtySecondTargetComplexQueries tests 30-second target for complex queries
func TestThirtySecondTargetComplexQueries(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping 30-second target test in short mode")
	}

	if !servicesReady(t) {
		t.Skip("Services not available for 30-second target testing")
	}

	complexScenarios := getComplexDemoScenarios()
	client := &http.Client{
		Timeout: 45 * time.Second, // Allow some buffer beyond 30 seconds
	}

	var totalTime time.Duration
	var successCount int
	var within30Seconds int

	for i, scenario := range complexScenarios {
		t.Logf("Testing complex scenario %d/%d: %s", i+1, len(complexScenarios), scenario.Name)

		result := executeDemoScenario(client, scenario, i)
		totalTime += result.ResponseTime

		if result.Success {
			successCount++
		}

		if result.ResponseTime <= maxDemoResponseTime {
			within30Seconds++
		}

		// Log individual results
		t.Logf("  Response time: %v (within target: %t)",
			result.ResponseTime, result.ResponseTime <= maxDemoResponseTime)
		t.Logf("  Success: %t", result.Success)
		t.Logf("  Response size: %d bytes", result.ResponseSize)
	}

	// Calculate statistics
	averageTime := totalTime / time.Duration(len(complexScenarios))
	successRate := float64(successCount) / float64(len(complexScenarios))
	targetComplianceRate := float64(within30Seconds) / float64(len(complexScenarios))

	// Assertions
	assert.GreaterOrEqual(t, targetComplianceRate, 0.8,
		"At least 80%% of complex queries should complete within 30 seconds")
	assert.GreaterOrEqual(t, successRate, minDemoSuccessRate,
		"Success rate should be at least %v%%", minDemoSuccessRate*100)
	assert.Less(t, averageTime, maxDemoResponseTime,
		"Average response time should be under 30 seconds")

	// Log summary
	t.Logf("Complex queries performance summary:")
	t.Logf("  Total scenarios: %d", len(complexScenarios))
	t.Logf("  Successful: %d", successCount)
	t.Logf("  Within 30 seconds: %d", within30Seconds)
	t.Logf("  Success rate: %.2f%%", successRate*100)
	t.Logf("  Target compliance rate: %.2f%%", targetComplianceRate*100)
	t.Logf("  Average response time: %v", averageTime)
}

// TestMultipleConcurrentDemos tests performance with multiple concurrent demos
func TestMultipleConcurrentDemos(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent demos test in short mode")
	}

	if !servicesReady(t) {
		t.Skip("Services not available for concurrent demos testing")
	}

	scenarios := getStandardDemoScenarios()
	concurrencyLevels := []int{2, 3, 5}

	for _, concurrency := range concurrencyLevels {
		t.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(t *testing.T) {
			testConcurrentDemoExecution(t, scenarios, concurrency)
		})
	}
}

func testConcurrentDemoExecution(t *testing.T, scenarios []DemoScenario, concurrency int) {
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	var wg sync.WaitGroup
	results := make(chan DemoResult, concurrency*len(scenarios))

	start := time.Now()

	// Launch concurrent demo executions
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j, scenario := range scenarios {
				requestID := workerID*len(scenarios) + j
				result := executeDemoScenario(client, scenario, requestID)
				results <- result
			}
		}(i)
	}

	wg.Wait()
	close(results)

	totalTime := time.Since(start)

	// Analyze concurrent execution results
	stats := analyzeDemoResults(results, totalTime)

	// Assertions
	assert.GreaterOrEqual(t, stats.SuccessRate, minDemoSuccessRate,
		"Concurrent demo success rate should be at least %v%%", minDemoSuccessRate*100)
	assert.Less(t, stats.MaxResponseTime, maxDemoResponseTime*2,
		"Maximum response time should not exceed 60 seconds under load")
	assert.Greater(t, stats.ScenariosPerSecond, 0.1,
		"Should maintain reasonable throughput")

	// Log results
	t.Logf("Concurrent demos results (concurrency=%d):", concurrency)
	t.Logf("  Total scenarios: %d", stats.TotalScenarios)
	t.Logf("  Successful: %d", stats.SuccessfulScenarios)
	t.Logf("  Failed: %d", stats.FailedScenarios)
	t.Logf("  Success rate: %.2f%%", stats.SuccessRate*100)
	t.Logf("  Average response time: %v", stats.AverageResponseTime)
	t.Logf("  Max response time: %v", stats.MaxResponseTime)
	t.Logf("  Min response time: %v", stats.MinResponseTime)
	t.Logf("  Scenarios per second: %.2f", stats.ScenariosPerSecond)
	t.Logf("  Total data transfer: %.2f MB", float64(stats.TotalDataTransfer)/1024/1024)
	t.Logf("  Total execution time: %v", totalTime)
}

// TestPerformanceWithLargeKnowledgeBase tests performance with large knowledge base
func TestPerformanceWithLargeKnowledgeBase(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large knowledge base performance test in short mode")
	}

	if !servicesReady(t) {
		t.Skip("Services not available for large knowledge base testing")
	}

	// Test scenarios that would trigger extensive knowledge base searches
	knowledgeIntensiveScenarios := []DemoScenario{
		{
			Name:               "ComprehensiveMultiCloudStrategy",
			Query:              "@SA-Assistant Design a comprehensive multi-cloud strategy covering AWS, Azure, and GCP with detailed migration paths, security frameworks, cost optimization, and governance models",
			Description:        "Tests performance with queries requiring extensive knowledge base search",
			ExpectedComponents: []string{"AWS", "Azure", "GCP", "migration", "security", "cost"},
		},
		{
			Name:               "DetailedComplianceFramework",
			Query:              "@SA-Assistant Create a detailed compliance framework covering HIPAA, GDPR, SOX, PCI-DSS with implementation guides, audit procedures, and monitoring strategies",
			Description:        "Tests performance with compliance-heavy queries",
			ExpectedComponents: []string{"HIPAA", "GDPR", "compliance", "audit", "monitoring"},
		},
		{
			Name:               "EnterpriseArchitectureBlueprint",
			Query:              "@SA-Assistant Generate an enterprise architecture blueprint with microservices design, API management, data architecture, security layers, and DevOps pipelines",
			Description:        "Tests performance with architecture-heavy queries",
			ExpectedComponents: []string{"microservices", "API", "architecture", "security", "DevOps"},
		},
	}

	client := &http.Client{
		Timeout: 90 * time.Second, // Longer timeout for knowledge-intensive queries
	}

	var results []DemoResult

	for i, scenario := range knowledgeIntensiveScenarios {
		t.Logf("Testing knowledge-intensive scenario %d/%d: %s", i+1, len(knowledgeIntensiveScenarios), scenario.Name)

		result := executeDemoScenario(client, scenario, i)
		results = append(results, result)

		// Log individual results
		t.Logf("  Response time: %v", result.ResponseTime)
		t.Logf("  Success: %t", result.Success)
		t.Logf("  Response size: %d bytes", result.ResponseSize)
		t.Logf("  Components found: %d", len(result.Components))
	}

	// Analyze results
	var totalTime time.Duration
	var successCount int
	var totalSize int64

	for _, result := range results {
		totalTime += result.ResponseTime
		if result.Success {
			successCount++
		}
		totalSize += int64(result.ResponseSize)
	}

	averageTime := totalTime / time.Duration(len(results))
	successRate := float64(successCount) / float64(len(results))

	// Assertions
	assert.GreaterOrEqual(t, successRate, 0.7,
		"Knowledge-intensive queries should have at least 70%% success rate")
	assert.Less(t, averageTime, 60*time.Second,
		"Average response time should be under 60 seconds for knowledge-intensive queries")

	// Log summary
	t.Logf("Large knowledge base performance summary:")
	t.Logf("  Total scenarios: %d", len(results))
	t.Logf("  Successful: %d", successCount)
	t.Logf("  Success rate: %.2f%%", successRate*100)
	t.Logf("  Average response time: %v", averageTime)
	t.Logf("  Total response size: %.2f MB", float64(totalSize)/1024/1024)
}

// TestPerformanceDegradationGracefully tests performance degradation gracefully
func TestPerformanceDegradationGracefully(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance degradation test in short mode")
	}

	if !servicesReady(t) {
		t.Skip("Services not available for performance degradation testing")
	}

	scenarios := getStandardDemoScenarios()
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	// Test with increasing load levels
	loadLevels := []struct {
		name        string
		concurrency int
		iterations  int
	}{
		{"Low Load", 1, 3},
		{"Medium Load", 2, 3},
		{"High Load", 3, 3},
		{"Peak Load", 5, 2},
	}

	performanceMetrics := make(map[string]*E2EPerformanceStats)

	for _, load := range loadLevels {
		t.Run(load.name, func(t *testing.T) {
			stats := testLoadLevel(t, client, scenarios, load.concurrency, load.iterations)
			performanceMetrics[load.name] = stats
		})
	}

	// Analyze performance degradation
	analyzeDegradation(t, performanceMetrics)
}

func testLoadLevel(_ *testing.T, client *http.Client, scenarios []DemoScenario, concurrency, iterations int) *E2EPerformanceStats {
	var wg sync.WaitGroup
	results := make(chan DemoResult, concurrency*iterations*len(scenarios))

	start := time.Now()

	for iter := 0; iter < iterations; iter++ {
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(workerID, iteration int) {
				defer wg.Done()

				for j, scenario := range scenarios {
					requestID := iteration*concurrency*len(scenarios) + workerID*len(scenarios) + j
					result := executeDemoScenario(client, scenario, requestID)
					results <- result
				}
			}(i, iter)
		}
	}

	wg.Wait()
	close(results)

	totalTime := time.Since(start)
	return analyzeDemoResults(results, totalTime)
}

// TestPerformanceMonitoringAndAlerting tests performance monitoring and alerting
func TestPerformanceMonitoringAndAlerting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance monitoring test in short mode")
	}

	if !servicesReady(t) {
		t.Skip("Services not available for performance monitoring testing")
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	scenarios := getStandardDemoScenarios()

	// Run scenarios while monitoring performance metrics
	monitoringResults := make([]PerformanceMetric, 0)

	for i, scenario := range scenarios {
		startTime := time.Now()
		result := executeDemoScenario(client, scenario, i)

		metric := PerformanceMetric{
			Timestamp:    startTime,
			Scenario:     scenario.Name,
			ResponseTime: result.ResponseTime,
			Success:      result.Success,
			ResponseSize: result.ResponseSize,
		}

		monitoringResults = append(monitoringResults, metric)

		// Check for performance alerts
		alerts := checkPerformanceAlerts(metric)
		if len(alerts) > 0 {
			t.Logf("Performance alerts for %s:", scenario.Name)
			for _, alert := range alerts {
				t.Logf("  - %s", alert)
			}
		}
	}

	// Analyze monitoring data
	avgResponseTime := calculateAverageResponseTime(monitoringResults)
	successRate := calculateSuccessRate(monitoringResults)

	// Log monitoring summary
	t.Logf("Performance monitoring summary:")
	t.Logf("  Average response time: %v", avgResponseTime)
	t.Logf("  Success rate: %.2f%%", successRate*100)
	t.Logf("  Total metrics collected: %d", len(monitoringResults))

	// Assertions
	assert.Less(t, avgResponseTime, maxDemoResponseTime,
		"Average response time should be within target")
	assert.GreaterOrEqual(t, successRate, minDemoSuccessRate,
		"Success rate should meet minimum threshold")
}

// Helper types and functions

type PerformanceMetric struct {
	Timestamp    time.Time
	Scenario     string
	ResponseTime time.Duration
	Success      bool
	ResponseSize int
}

func getStandardDemoScenarios() []DemoScenario {
	return []DemoScenario{
		{
			Name:               "AWSLiftAndShift",
			Query:              "@SA-Assistant Generate a high-level lift-and-shift plan for migrating 120 on-prem Windows and Linux VMs to AWS, including EC2 instance recommendations, VPC/subnet topology, and the latest AWS MGN best practices",
			Description:        "Standard AWS migration demo",
			ExpectedComponents: []string{"AWS", "EC2", "VPC", "migration", "MGN"},
		},
		{
			Name:               "AzureHybridArchitecture",
			Query:              "@SA-Assistant Outline a hybrid reference architecture connecting our on-prem VMware environment to Azure, covering ExpressRoute configuration, VMware HCX migration, active-active failover",
			Description:        "Standard Azure hybrid demo",
			ExpectedComponents: []string{"Azure", "hybrid", "ExpressRoute", "VMware", "HCX"},
		},
		{
			Name:               "AzureDisasterRecovery",
			Query:              "@SA-Assistant Design a DR solution in Azure for critical workloads with RTO = 2 hours and RPO = 15 minutes, including geo-replication options, failover orchestration, and cost-optimized standby",
			Description:        "Standard disaster recovery demo",
			ExpectedComponents: []string{"Azure", "disaster recovery", "RTO", "RPO", "failover"},
		},
		{
			Name:               "SecurityCompliance",
			Query:              "@SA-Assistant Summarize HIPAA and GDPR encryption, logging, and policy enforcement requirements for our AWS landing zone, and include any recent AWS compliance feature updates",
			Description:        "Standard security compliance demo",
			ExpectedComponents: []string{"HIPAA", "GDPR", "encryption", "compliance", "AWS"},
		},
	}
}

func getComplexDemoScenarios() []DemoScenario {
	return []DemoScenario{
		{
			Name:               "ComplexAWSMigration",
			Query:              "@SA-Assistant Generate a comprehensive AWS migration strategy for 500+ VMs across multiple data centers, including network topology, security zones, data migration strategies, application dependencies, rollback procedures, cost optimization, and detailed timeline with risk mitigation",
			Description:        "Complex AWS migration requiring extensive processing",
			ExpectedComponents: []string{"AWS", "migration", "network", "security", "data", "cost"},
		},
		{
			Name:               "EnterpriseMultiCloudStrategy",
			Query:              "@SA-Assistant Design an enterprise multi-cloud strategy spanning AWS, Azure, and GCP with unified identity management, cross-cloud networking, data synchronization, disaster recovery across clouds, cost governance, and compliance frameworks",
			Description:        "Complex multi-cloud strategy requiring extensive synthesis",
			ExpectedComponents: []string{"multi-cloud", "AWS", "Azure", "GCP", "identity", "networking"},
		},
		{
			Name:               "ComprehensiveSecurityFramework",
			Query:              "@SA-Assistant Create a comprehensive security framework covering zero-trust architecture, identity and access management, network segmentation, data encryption, threat detection, incident response, compliance automation, and security monitoring across hybrid environments",
			Description:        "Complex security framework requiring detailed analysis",
			ExpectedComponents: []string{"zero-trust", "identity", "encryption", "threat detection", "compliance"},
		},
	}
}

func executeDemoScenario(client *http.Client, scenario DemoScenario, requestID int) DemoResult {
	request := map[string]interface{}{
		"text": scenario.Query,
		"type": "message",
		"from": map[string]interface{}{
			"id":   fmt.Sprintf("perf_test_user_%d", requestID),
			"name": "Performance Test User",
		},
	}

	body, _ := json.Marshal(request)

	start := time.Now()
	resp, err := client.Post("http://localhost:8080/teams-webhook", "application/json", bytes.NewBuffer(body))
	responseTime := time.Since(start)

	result := DemoResult{
		Scenario:     scenario,
		ResponseTime: responseTime,
		Error:        err,
	}

	if err != nil {
		result.Success = false
		return result
	}

	result.StatusCode = resp.StatusCode
	result.Success = resp.StatusCode >= 200 && resp.StatusCode < 300

	// Read response body
	responseBody := make([]byte, 0)
	buffer := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			responseBody = append(responseBody, buffer[:n]...)
		}
		if err != nil {
			break
		}
	}
	resp.Body.Close()

	result.ResponseSize = len(responseBody)

	// Extract components from response
	responseStr := string(responseBody)
	for _, expectedComponent := range scenario.ExpectedComponents {
		if strings.Contains(strings.ToLower(responseStr), strings.ToLower(expectedComponent)) {
			result.Components = append(result.Components, expectedComponent)
		}
	}

	return result
}

func analyzeDemoResults(results <-chan DemoResult, totalTime time.Duration) *E2EPerformanceStats {
	stats := &E2EPerformanceStats{
		MinResponseTime: time.Hour, // Initialize with large value
	}

	var totalResponseTime time.Duration
	var totalDataTransfer int64

	for result := range results {
		stats.TotalScenarios++
		totalResponseTime += result.ResponseTime
		totalDataTransfer += int64(result.ResponseSize)

		if result.Success {
			stats.SuccessfulScenarios++
		} else {
			stats.FailedScenarios++
		}

		if result.ResponseTime > stats.MaxResponseTime {
			stats.MaxResponseTime = result.ResponseTime
		}
		if result.ResponseTime < stats.MinResponseTime {
			stats.MinResponseTime = result.ResponseTime
		}
	}

	if stats.TotalScenarios > 0 {
		stats.AverageResponseTime = totalResponseTime / time.Duration(stats.TotalScenarios)
		stats.SuccessRate = float64(stats.SuccessfulScenarios) / float64(stats.TotalScenarios)
	}

	if totalTime > 0 {
		stats.ScenariosPerSecond = float64(stats.TotalScenarios) / totalTime.Seconds()
	}

	stats.TotalDataTransfer = totalDataTransfer

	return stats
}

func analyzeDegradation(t *testing.T, performanceMetrics map[string]*E2EPerformanceStats) {
	loadLevels := []string{"Low Load", "Medium Load", "High Load", "Peak Load"}

	t.Logf("Performance degradation analysis:")

	for i, level := range loadLevels {
		stats := performanceMetrics[level]
		if stats == nil {
			continue
		}

		t.Logf("  %s:", level)
		t.Logf("    Success rate: %.2f%%", stats.SuccessRate*100)
		t.Logf("    Average response time: %v", stats.AverageResponseTime)
		t.Logf("    Scenarios per second: %.2f", stats.ScenariosPerSecond)

		// Check for graceful degradation
		if i > 0 {
			prevLevel := loadLevels[i-1]
			prevStats := performanceMetrics[prevLevel]
			if prevStats != nil {
				responseTimeDegradation := float64(stats.AverageResponseTime) / float64(prevStats.AverageResponseTime)
				successRateDegradation := stats.SuccessRate / prevStats.SuccessRate

				t.Logf("    Response time degradation: %.2fx", responseTimeDegradation)
				t.Logf("    Success rate degradation: %.2fx", successRateDegradation)

				// Assert graceful degradation
				assert.Less(t, responseTimeDegradation, 3.0,
					"Response time should not degrade more than 3x between %s and %s", prevLevel, level)
				assert.Greater(t, successRateDegradation, 0.7,
					"Success rate should not drop below 70%% of previous level between %s and %s", prevLevel, level)
			}
		}
	}
}

func checkPerformanceAlerts(metric PerformanceMetric) []string {
	var alerts []string

	if metric.ResponseTime > maxDemoResponseTime {
		alerts = append(alerts, fmt.Sprintf("Response time exceeded target: %v > %v", metric.ResponseTime, maxDemoResponseTime))
	}

	if !metric.Success {
		alerts = append(alerts, "Request failed")
	}

	if metric.ResponseSize < 100 {
		alerts = append(alerts, fmt.Sprintf("Response size unusually small: %d bytes", metric.ResponseSize))
	}

	return alerts
}

func calculateAverageResponseTime(metrics []PerformanceMetric) time.Duration {
	if len(metrics) == 0 {
		return 0
	}

	var total time.Duration
	for _, metric := range metrics {
		total += metric.ResponseTime
	}

	return total / time.Duration(len(metrics))
}

func calculateSuccessRate(metrics []PerformanceMetric) float64 {
	if len(metrics) == 0 {
		return 0
	}

	successCount := 0
	for _, metric := range metrics {
		if metric.Success {
			successCount++
		}
	}

	return float64(successCount) / float64(len(metrics))
}
