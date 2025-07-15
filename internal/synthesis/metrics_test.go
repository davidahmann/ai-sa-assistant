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

package synthesis

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestMetricsCollector_RecordCodeGeneration(t *testing.T) {
	logger := zap.NewNop()
	alertTriggered := false

	alertCallback := func(alertType, message string, metadata map[string]interface{}) {
		alertTriggered = true
	}

	mc := NewMetricsCollector(logger, alertCallback)

	// Record successful code generation
	mc.RecordCodeGeneration("migration", "terraform", true, 100*time.Millisecond, 0.8)

	if mc.CodeGenMetrics.TotalRequests != 1 {
		t.Errorf("Expected 1 total request, got %d", mc.CodeGenMetrics.TotalRequests)
	}

	if mc.CodeGenMetrics.SuccessfulCodes != 1 {
		t.Errorf("Expected 1 successful code, got %d", mc.CodeGenMetrics.SuccessfulCodes)
	}

	if mc.CodeGenMetrics.SuccessRate != 1.0 {
		t.Errorf("Expected success rate of 1.0, got %f", mc.CodeGenMetrics.SuccessRate)
	}

	// Check language metrics
	langMetrics := mc.CodeGenMetrics.ByLanguage["terraform"]
	if langMetrics.TotalGenerated != 1 {
		t.Errorf("Expected 1 terraform generation, got %d", langMetrics.TotalGenerated)
	}

	// Check domain metrics
	domainMetrics := mc.CodeGenMetrics.ByDomain["migration"]
	if domainMetrics.TotalQueries != 1 {
		t.Errorf("Expected 1 migration query, got %d", domainMetrics.TotalQueries)
	}

	if alertTriggered {
		t.Error("Alert should not be triggered for successful generation")
	}
}

func TestMetricsCollector_RecordCodeGeneration_Failure(t *testing.T) {
	logger := zap.NewNop()
	alertTriggered := false

	alertCallback := func(alertType, message string, metadata map[string]interface{}) {
		alertTriggered = true
		if alertType != "CODE_GENERATION_FAILURE" {
			t.Errorf("Expected CODE_GENERATION_FAILURE alert, got %s", alertType)
		}
	}

	mc := NewMetricsCollector(logger, alertCallback)

	// Record multiple failures to trigger alert
	for i := 0; i < 10; i++ {
		mc.RecordCodeGeneration("migration", "terraform", false, 100*time.Millisecond, 0.0)
	}

	if mc.CodeGenMetrics.TotalRequests != 10 {
		t.Errorf("Expected 10 total requests, got %d", mc.CodeGenMetrics.TotalRequests)
	}

	if mc.CodeGenMetrics.FailedCodes != 10 {
		t.Errorf("Expected 10 failed codes, got %d", mc.CodeGenMetrics.FailedCodes)
	}

	if mc.CodeGenMetrics.SuccessRate != 0.0 {
		t.Errorf("Expected success rate of 0.0, got %f", mc.CodeGenMetrics.SuccessRate)
	}

	if !alertTriggered {
		t.Error("Alert should be triggered for high failure rate")
	}
}

func TestMetricsCollector_RecordDiagramGeneration(t *testing.T) {
	logger := zap.NewNop()
	alertTriggered := false

	alertCallback := func(alertType, message string, metadata map[string]interface{}) {
		alertTriggered = true
	}

	mc := NewMetricsCollector(logger, alertCallback)

	// Record successful diagram generation
	mc.RecordDiagramGeneration("mermaid", true, 200*time.Millisecond)

	if mc.DiagramMetrics.TotalRequests != 1 {
		t.Errorf("Expected 1 total request, got %d", mc.DiagramMetrics.TotalRequests)
	}

	if mc.DiagramMetrics.SuccessfulDiagrams != 1 {
		t.Errorf("Expected 1 successful diagram, got %d", mc.DiagramMetrics.SuccessfulDiagrams)
	}

	if mc.DiagramMetrics.SuccessRate != 1.0 {
		t.Errorf("Expected success rate of 1.0, got %f", mc.DiagramMetrics.SuccessRate)
	}

	// Check type metrics
	typeMetrics := mc.DiagramMetrics.ByType["mermaid"]
	if typeMetrics.TotalGenerated != 1 {
		t.Errorf("Expected 1 mermaid generation, got %d", typeMetrics.TotalGenerated)
	}

	if alertTriggered {
		t.Error("Alert should not be triggered for successful generation")
	}
}

func TestMetricsCollector_RecordDiagramGeneration_Failure(t *testing.T) {
	logger := zap.NewNop()
	alertTriggered := false

	alertCallback := func(alertType, message string, metadata map[string]interface{}) {
		alertTriggered = true
		if alertType != "DIAGRAM_GENERATION_FAILURE" {
			t.Errorf("Expected DIAGRAM_GENERATION_FAILURE alert, got %s", alertType)
		}
	}

	mc := NewMetricsCollector(logger, alertCallback)

	// Record multiple failures to trigger alert
	for i := 0; i < 10; i++ {
		mc.RecordDiagramGeneration("mermaid", false, 100*time.Millisecond)
	}

	if mc.DiagramMetrics.TotalRequests != 10 {
		t.Errorf("Expected 10 total requests, got %d", mc.DiagramMetrics.TotalRequests)
	}

	if mc.DiagramMetrics.FailedDiagrams != 10 {
		t.Errorf("Expected 10 failed diagrams, got %d", mc.DiagramMetrics.FailedDiagrams)
	}

	if mc.DiagramMetrics.SuccessRate != 0.0 {
		t.Errorf("Expected success rate of 0.0, got %f", mc.DiagramMetrics.SuccessRate)
	}

	if !alertTriggered {
		t.Error("Alert should be triggered for high failure rate")
	}
}

func TestMetricsCollector_RecordResponseQuality(t *testing.T) {
	logger := zap.NewNop()
	alertTriggered := false

	alertCallback := func(alertType, message string, metadata map[string]interface{}) {
		alertTriggered = true
	}

	mc := NewMetricsCollector(logger, alertCallback)

	// Record high quality response
	mc.RecordResponseQuality(0.9, 5*time.Second, true, true)

	if mc.QualityMetrics.TotalResponses != 1 {
		t.Errorf("Expected 1 total response, got %d", mc.QualityMetrics.TotalResponses)
	}

	if mc.QualityMetrics.AvgQualityScore != 0.9 {
		t.Errorf("Expected avg quality score of 0.9, got %f", mc.QualityMetrics.AvgQualityScore)
	}

	if mc.QualityMetrics.HighQualityRate != 100.0 {
		t.Errorf("Expected high quality rate of 100%%, got %f", mc.QualityMetrics.HighQualityRate)
	}

	if alertTriggered {
		t.Error("Alert should not be triggered for high quality response")
	}
}

func TestMetricsCollector_RecordResponseQuality_LowQuality(t *testing.T) {
	logger := zap.NewNop()
	alertTriggered := false

	alertCallback := func(alertType, message string, metadata map[string]interface{}) {
		alertTriggered = true
		if alertType != "QUALITY_SCORE_LOW" {
			t.Errorf("Expected QUALITY_SCORE_LOW alert, got %s", alertType)
		}
	}

	mc := NewMetricsCollector(logger, alertCallback)

	// Record low quality response
	mc.RecordResponseQuality(0.4, 5*time.Second, false, false)

	if mc.QualityMetrics.TotalResponses != 1 {
		t.Errorf("Expected 1 total response, got %d", mc.QualityMetrics.TotalResponses)
	}

	if mc.QualityMetrics.AvgQualityScore != 0.4 {
		t.Errorf("Expected avg quality score of 0.4, got %f", mc.QualityMetrics.AvgQualityScore)
	}

	if mc.QualityMetrics.PoorQualityRate != 100.0 {
		t.Errorf("Expected poor quality rate of 100%%, got %f", mc.QualityMetrics.PoorQualityRate)
	}

	if !alertTriggered {
		t.Error("Alert should be triggered for low quality response")
	}
}

func TestMetricsCollector_RecordResponseQuality_HighResponseTime(t *testing.T) {
	logger := zap.NewNop()
	alertTriggered := false

	alertCallback := func(alertType, message string, metadata map[string]interface{}) {
		alertTriggered = true
		if alertType != "RESPONSE_TIME_HIGH" {
			t.Errorf("Expected RESPONSE_TIME_HIGH alert, got %s", alertType)
		}
	}

	mc := NewMetricsCollector(logger, alertCallback)

	// Record high response time
	mc.RecordResponseQuality(0.8, 35*time.Second, true, true)

	if mc.QualityMetrics.AvgResponseTime != 35000.0 {
		t.Errorf("Expected avg response time of 35000ms, got %f", mc.QualityMetrics.AvgResponseTime)
	}

	if !alertTriggered {
		t.Error("Alert should be triggered for high response time")
	}
}

func TestMetricsCollector_GetMetrics(t *testing.T) {
	logger := zap.NewNop()
	mc := NewMetricsCollector(logger, nil)

	// Record some metrics
	mc.RecordCodeGeneration("migration", "terraform", true, 100*time.Millisecond, 0.8)
	mc.RecordDiagramGeneration("mermaid", true, 200*time.Millisecond)
	mc.RecordResponseQuality(0.9, 5*time.Second, true, true)

	metrics := mc.GetMetrics()

	if metrics["code_generation"] == nil {
		t.Error("Expected code_generation metrics to be present")
	}

	if metrics["diagram_generation"] == nil {
		t.Error("Expected diagram_generation metrics to be present")
	}

	if metrics["response_quality"] == nil {
		t.Error("Expected response_quality metrics to be present")
	}

	if metrics["alerting_config"] == nil {
		t.Error("Expected alerting_config to be present")
	}

	if metrics["collected_at"] == nil {
		t.Error("Expected collected_at timestamp to be present")
	}
}

func TestMetricsCollector_ResetMetrics(t *testing.T) {
	logger := zap.NewNop()
	mc := NewMetricsCollector(logger, nil)

	// Record some metrics
	mc.RecordCodeGeneration("migration", "terraform", true, 100*time.Millisecond, 0.8)
	mc.RecordDiagramGeneration("mermaid", true, 200*time.Millisecond)
	mc.RecordResponseQuality(0.9, 5*time.Second, true, true)

	// Verify metrics are recorded
	if mc.CodeGenMetrics.TotalRequests != 1 {
		t.Errorf("Expected 1 total request before reset, got %d", mc.CodeGenMetrics.TotalRequests)
	}

	// Reset metrics
	mc.ResetMetrics()

	// Verify metrics are reset
	if mc.CodeGenMetrics.TotalRequests != 0 {
		t.Errorf("Expected 0 total requests after reset, got %d", mc.CodeGenMetrics.TotalRequests)
	}

	if mc.DiagramMetrics.TotalRequests != 0 {
		t.Errorf("Expected 0 diagram requests after reset, got %d", mc.DiagramMetrics.TotalRequests)
	}

	if mc.QualityMetrics.TotalResponses != 0 {
		t.Errorf("Expected 0 quality responses after reset, got %d", mc.QualityMetrics.TotalResponses)
	}
}

func TestMetricsCollector_HealthCheck(t *testing.T) {
	logger := zap.NewNop()
	mc := NewMetricsCollector(logger, nil)

	// Test healthy state
	healthy, status, metadata := mc.HealthCheck(context.Background())

	if !healthy {
		t.Error("Expected healthy state with no metrics")
	}

	if status != "healthy" {
		t.Errorf("Expected healthy status, got %s", status)
	}

	if metadata == nil {
		t.Error("Expected metadata to be present")
	}

	// Record some successful metrics
	mc.RecordCodeGeneration("migration", "terraform", true, 100*time.Millisecond, 0.8)
	mc.RecordDiagramGeneration("mermaid", true, 200*time.Millisecond)
	mc.RecordResponseQuality(0.9, 5*time.Second, true, true)

	healthy, status, metadata = mc.HealthCheck(context.Background())

	if !healthy {
		t.Error("Expected healthy state with good metrics")
	}

	if status != "healthy" {
		t.Errorf("Expected healthy status, got %s", status)
	}

	// Record failures to make it unhealthy
	for i := 0; i < 10; i++ {
		mc.RecordCodeGeneration("migration", "terraform", false, 100*time.Millisecond, 0.0)
	}

	healthy, status, metadata = mc.HealthCheck(context.Background())

	if healthy {
		t.Error("Expected unhealthy state with poor metrics")
	}

	if status != "degraded" {
		t.Errorf("Expected degraded status, got %s", status)
	}

	if metadata["code_generation_success_rate"] == nil {
		t.Error("Expected code_generation_success_rate in metadata")
	}
}

func TestMetricsCollector_ConcurrentAccess(t *testing.T) {
	logger := zap.NewNop()
	mc := NewMetricsCollector(logger, nil)

	// Test concurrent access
	done := make(chan bool)

	// Concurrent writers
	go func() {
		for i := 0; i < 100; i++ {
			mc.RecordCodeGeneration("migration", "terraform", true, 100*time.Millisecond, 0.8)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			mc.RecordDiagramGeneration("mermaid", true, 200*time.Millisecond)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			mc.RecordResponseQuality(0.9, 5*time.Second, true, true)
		}
		done <- true
	}()

	// Concurrent readers
	go func() {
		for i := 0; i < 100; i++ {
			_ = mc.GetMetrics()
		}
		done <- true
	}()

	// Wait for all goroutines to complete
	for i := 0; i < 4; i++ {
		<-done
	}

	// Verify final counts
	if mc.CodeGenMetrics.TotalRequests != 100 {
		t.Errorf("Expected 100 code generation requests, got %d", mc.CodeGenMetrics.TotalRequests)
	}

	if mc.DiagramMetrics.TotalRequests != 100 {
		t.Errorf("Expected 100 diagram generation requests, got %d", mc.DiagramMetrics.TotalRequests)
	}

	if mc.QualityMetrics.TotalResponses != 100 {
		t.Errorf("Expected 100 quality responses, got %d", mc.QualityMetrics.TotalResponses)
	}
}
