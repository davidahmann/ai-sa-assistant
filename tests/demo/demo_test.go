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

//go:build demo

package demo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// DemoScenario represents a demo scenario test case
type DemoScenario struct {
	Name        string
	Query       string
	ExpectedKey string
	Timeout     time.Duration
}

// TestDemoScenarios tests all 4 demo scenarios
func TestDemoScenarios(t *testing.T) {
	scenarios := []DemoScenario{
		{
			Name:        "AWS_Lift_and_Shift",
			Query:       "@SA-Assistant Generate a high-level lift-and-shift plan for migrating 120 on-prem Windows and Linux VMs to AWS, including EC2 instance recommendations, VPC/subnet topology, and the latest AWS MGN best practices from Q2 2025.",
			ExpectedKey: "AWS",
			Timeout:     30 * time.Second,
		},
		{
			Name:        "Azure_Hybrid_Architecture",
			Query:       "@SA-Assistant Outline a hybrid reference architecture connecting our on-prem VMware environment to Azure, covering ExpressRoute configuration, VMware HCX migration, active-active failover, and June 2025 Azure Hybrid announcements.",
			ExpectedKey: "Azure",
			Timeout:     30 * time.Second,
		},
		{
			Name:        "Azure_Disaster_Recovery",
			Query:       "@SA-Assistant Design a DR solution in Azure for critical workloads with RTO = 2 hours and RPO = 15 minutes, including geo-replication options, failover orchestration, and cost-optimized standby.",
			ExpectedKey: "disaster recovery",
			Timeout:     30 * time.Second,
		},
		{
			Name:        "Security_Compliance",
			Query:       "@SA-Assistant Summarize HIPAA and GDPR encryption, logging, and policy enforcement requirements for our AWS landing zone, and include any recent AWS compliance feature updates.",
			ExpectedKey: "HIPAA",
			Timeout:     30 * time.Second,
		},
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			start := time.Now()

			// Create Teams webhook request
			request := map[string]interface{}{
				"text": scenario.Query,
			}
			body, _ := json.Marshal(request)

			resp, err := client.Post("http://localhost:8080/webhook", "application/json", bytes.NewBuffer(body))
			if err != nil {
				t.Fatalf("Failed to call webhook: %v", err)
			}
			defer resp.Body.Close()

			duration := time.Since(start)
			t.Logf("Scenario %s completed in %v", scenario.Name, duration)

			// Validate response time
			if duration > scenario.Timeout {
				t.Errorf("Scenario %s took too long: %v > %v", scenario.Name, duration, scenario.Timeout)
			}

			// Validate HTTP status
			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
			}

			// Validate response contains expected content
			var responseBody map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&responseBody); err != nil {
				t.Errorf("Failed to decode response: %v", err)
			}

			// Check if response contains expected keywords
			responseStr := fmt.Sprintf("%v", responseBody)
			if !strings.Contains(strings.ToLower(responseStr), strings.ToLower(scenario.ExpectedKey)) {
				t.Errorf("Response does not contain expected key '%s'", scenario.ExpectedKey)
			}
		})
	}
}

// TestDemoResponseStructure tests the structure of demo responses
func TestDemoResponseStructure(t *testing.T) {
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	request := map[string]interface{}{
		"text": "@SA-Assistant Generate a simple AWS migration plan",
	}
	body, _ := json.Marshal(request)

	resp, err := client.Post("http://localhost:8080/webhook", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("Failed to call webhook: %v", err)
	}
	defer resp.Body.Close()

	var responseBody map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&responseBody); err != nil {
		t.Errorf("Failed to decode response: %v", err)
	}

	// Expected response structure for Teams Adaptive Cards
	expectedFields := []string{"type", "body", "actions"}

	for _, field := range expectedFields {
		if _, ok := responseBody[field]; !ok {
			t.Errorf("Response missing required field: %s", field)
		}
	}
}

// TestDemoPerformance tests performance characteristics
func TestDemoPerformance(t *testing.T) {
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	const maxResponseTime = 30 * time.Second
	const testIterations = 3

	query := "@SA-Assistant Generate a basic AWS security assessment"

	var totalDuration time.Duration

	for i := 0; i < testIterations; i++ {
		start := time.Now()

		request := map[string]interface{}{
			"text": query,
		}
		body, _ := json.Marshal(request)

		resp, err := client.Post("http://localhost:8080/webhook", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to call webhook on iteration %d: %v", i+1, err)
		}
		resp.Body.Close()

		duration := time.Since(start)
		totalDuration += duration

		t.Logf("Iteration %d completed in %v", i+1, duration)

		if duration > maxResponseTime {
			t.Errorf("Iteration %d exceeded max response time: %v > %v", i+1, duration, maxResponseTime)
		}
	}

	avgDuration := totalDuration / testIterations
	t.Logf("Average response time: %v", avgDuration)

	if avgDuration > maxResponseTime {
		t.Errorf("Average response time exceeded maximum: %v > %v", avgDuration, maxResponseTime)
	}
}

// TestDemoErrorHandling tests error handling in demo scenarios
func TestDemoErrorHandling(t *testing.T) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Test invalid request
	t.Run("InvalidRequest", func(t *testing.T) {
		resp, err := client.Post("http://localhost:8080/webhook", "application/json", bytes.NewBuffer([]byte("invalid json")))
		if err != nil {
			t.Fatalf("Failed to call webhook: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			t.Error("Expected error status for invalid JSON, got 200 OK")
		}
	})

	// Test empty request
	t.Run("EmptyRequest", func(t *testing.T) {
		request := map[string]interface{}{
			"text": "",
		}
		body, _ := json.Marshal(request)

		resp, err := client.Post("http://localhost:8080/webhook", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to call webhook: %v", err)
		}
		defer resp.Body.Close()

		// Should handle empty requests gracefully
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected graceful handling of empty request, got %d", resp.StatusCode)
		}
	})
}
