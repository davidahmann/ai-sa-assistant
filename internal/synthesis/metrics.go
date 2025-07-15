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

// Package synthesis provides monitoring and metrics for the synthesis service
package synthesis

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CodeGenerationMetrics tracks code generation success rates and performance
type CodeGenerationMetrics struct {
	TotalRequests   int64                      `json:"total_requests"`
	SuccessfulCodes int64                      `json:"successful_codes"`
	FailedCodes     int64                      `json:"failed_codes"`
	SuccessRate     float64                    `json:"success_rate"`
	ByLanguage      map[string]LanguageMetrics `json:"by_language"`
	ByDomain        map[string]DomainMetrics   `json:"by_domain"`
	AverageGenTime  float64                    `json:"average_generation_time_ms"`
	LastReset       time.Time                  `json:"last_reset"`
}

// LanguageMetrics tracks metrics for specific programming languages
type LanguageMetrics struct {
	TotalGenerated int64   `json:"total_generated"`
	SuccessRate    float64 `json:"success_rate"`
	AvgQuality     float64 `json:"average_quality_score"`
}

// DomainMetrics tracks metrics for specific query domains
type DomainMetrics struct {
	TotalQueries int64   `json:"total_queries"`
	CodeGenRate  float64 `json:"code_generation_rate"`
	DiagramRate  float64 `json:"diagram_generation_rate"`
	QualityScore float64 `json:"average_quality_score"`
}

// DiagramGenerationMetrics tracks diagram generation performance
type DiagramGenerationMetrics struct {
	TotalRequests      int64                         `json:"total_requests"`
	SuccessfulDiagrams int64                         `json:"successful_diagrams"`
	FailedDiagrams     int64                         `json:"failed_diagrams"`
	SuccessRate        float64                       `json:"success_rate"`
	AvgRenderTime      float64                       `json:"average_render_time_ms"`
	ByType             map[string]DiagramTypeMetrics `json:"by_type"`
	LastReset          time.Time                     `json:"last_reset"`
}

// DiagramTypeMetrics tracks metrics for specific diagram types
type DiagramTypeMetrics struct {
	TotalGenerated int64   `json:"total_generated"`
	SuccessRate    float64 `json:"success_rate"`
	AvgRenderTime  float64 `json:"average_render_time_ms"`
}

// ResponseQualityMetrics tracks overall response quality
type ResponseQualityMetrics struct {
	TotalResponses         int64     `json:"total_responses"`
	AvgQualityScore        float64   `json:"average_quality_score"`
	HighQualityCount       int64     `json:"high_quality_count"`
	AcceptableQualityCount int64     `json:"acceptable_quality_count"`
	PoorQualityCount       int64     `json:"poor_quality_count"`
	HighQualityRate        float64   `json:"high_quality_rate"`       // > 0.8
	AcceptableQualityRate  float64   `json:"acceptable_quality_rate"` // > 0.6
	PoorQualityRate        float64   `json:"poor_quality_rate"`       // < 0.6
	AvgResponseTime        float64   `json:"average_response_time_ms"`
	LastReset              time.Time `json:"last_reset"`
}

// AlertingConfig defines thresholds for alerting
type AlertingConfig struct {
	CodeGenFailureThreshold    float64 `json:"code_generation_failure_threshold"`
	DiagramGenFailureThreshold float64 `json:"diagram_generation_failure_threshold"`
	QualityScoreThreshold      float64 `json:"quality_score_threshold"`
	ResponseTimeThreshold      float64 `json:"response_time_threshold_ms"`
}

// MetricsCollector collects and tracks synthesis service metrics
type MetricsCollector struct {
	mu             sync.RWMutex
	CodeGenMetrics *CodeGenerationMetrics    `json:"code_generation"`
	DiagramMetrics *DiagramGenerationMetrics `json:"diagram_generation"`
	QualityMetrics *ResponseQualityMetrics   `json:"response_quality"`
	AlertingConfig *AlertingConfig           `json:"alerting_config"`
	logger         *zap.Logger
	alertCallback  func(string, string, map[string]interface{})
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(logger *zap.Logger, alertCallback func(string, string, map[string]interface{})) *MetricsCollector {
	return &MetricsCollector{
		CodeGenMetrics: &CodeGenerationMetrics{
			ByLanguage: make(map[string]LanguageMetrics),
			ByDomain:   make(map[string]DomainMetrics),
			LastReset:  time.Now(),
		},
		DiagramMetrics: &DiagramGenerationMetrics{
			ByType:    make(map[string]DiagramTypeMetrics),
			LastReset: time.Now(),
		},
		QualityMetrics: &ResponseQualityMetrics{
			LastReset: time.Now(),
		},
		AlertingConfig: &AlertingConfig{
			CodeGenFailureThreshold:    0.20,  // Alert if failure rate > 20%
			DiagramGenFailureThreshold: 0.15,  // Alert if failure rate > 15%
			QualityScoreThreshold:      0.60,  // Alert if quality score < 60%
			ResponseTimeThreshold:      30000, // Alert if response time > 30s
		},
		logger:        logger,
		alertCallback: alertCallback,
	}
}

// RecordCodeGeneration records code generation metrics
func (mc *MetricsCollector) RecordCodeGeneration(domain, language string, success bool, genTime time.Duration, quality float64) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.CodeGenMetrics.TotalRequests++

	if success {
		mc.CodeGenMetrics.SuccessfulCodes++
	} else {
		mc.CodeGenMetrics.FailedCodes++
	}

	// Update success rate
	mc.CodeGenMetrics.SuccessRate = float64(mc.CodeGenMetrics.SuccessfulCodes) / float64(mc.CodeGenMetrics.TotalRequests)

	// Update language metrics
	langMetrics := mc.CodeGenMetrics.ByLanguage[language]
	langMetrics.TotalGenerated++
	if success {
		langMetrics.SuccessRate = float64(langMetrics.TotalGenerated) / float64(langMetrics.TotalGenerated)
		langMetrics.AvgQuality = (langMetrics.AvgQuality + quality) / 2
	}
	mc.CodeGenMetrics.ByLanguage[language] = langMetrics

	// Update domain metrics
	domainMetrics := mc.CodeGenMetrics.ByDomain[domain]
	domainMetrics.TotalQueries++
	domainMetrics.CodeGenRate = float64(mc.CodeGenMetrics.SuccessfulCodes) / float64(mc.CodeGenMetrics.TotalRequests)
	mc.CodeGenMetrics.ByDomain[domain] = domainMetrics

	// Update average generation time
	mc.CodeGenMetrics.AverageGenTime = (mc.CodeGenMetrics.AverageGenTime + float64(genTime.Milliseconds())) / 2

	// Check for alerting
	if mc.CodeGenMetrics.SuccessRate < (1.0 - mc.AlertingConfig.CodeGenFailureThreshold) {
		mc.triggerAlert("CODE_GENERATION_FAILURE", "Code generation failure rate exceeded threshold", map[string]interface{}{
			"success_rate": mc.CodeGenMetrics.SuccessRate,
			"threshold":    mc.AlertingConfig.CodeGenFailureThreshold,
			"domain":       domain,
			"language":     language,
		})
	}
}

// RecordDiagramGeneration records diagram generation metrics
func (mc *MetricsCollector) RecordDiagramGeneration(diagramType string, success bool, renderTime time.Duration) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.DiagramMetrics.TotalRequests++

	if success {
		mc.DiagramMetrics.SuccessfulDiagrams++
	} else {
		mc.DiagramMetrics.FailedDiagrams++
	}

	// Update success rate
	mc.DiagramMetrics.SuccessRate = float64(mc.DiagramMetrics.SuccessfulDiagrams) / float64(mc.DiagramMetrics.TotalRequests)

	// Update type metrics
	typeMetrics := mc.DiagramMetrics.ByType[diagramType]
	typeMetrics.TotalGenerated++
	if success {
		typeMetrics.SuccessRate = float64(typeMetrics.TotalGenerated) / float64(typeMetrics.TotalGenerated)
		typeMetrics.AvgRenderTime = (typeMetrics.AvgRenderTime + float64(renderTime.Milliseconds())) / 2
	}
	mc.DiagramMetrics.ByType[diagramType] = typeMetrics

	// Update average render time
	mc.DiagramMetrics.AvgRenderTime = (mc.DiagramMetrics.AvgRenderTime + float64(renderTime.Milliseconds())) / 2

	// Check for alerting
	if mc.DiagramMetrics.SuccessRate < (1.0 - mc.AlertingConfig.DiagramGenFailureThreshold) {
		mc.triggerAlert("DIAGRAM_GENERATION_FAILURE", "Diagram generation failure rate exceeded threshold", map[string]interface{}{
			"success_rate": mc.DiagramMetrics.SuccessRate,
			"threshold":    mc.AlertingConfig.DiagramGenFailureThreshold,
			"type":         diagramType,
		})
	}
}

// RecordResponseQuality records response quality metrics
func (mc *MetricsCollector) RecordResponseQuality(qualityScore float64, responseTime time.Duration, hasCode, hasDiagram bool) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	prevTotal := mc.QualityMetrics.TotalResponses
	mc.QualityMetrics.TotalResponses++

	// Update average quality score using running average
	if prevTotal == 0 {
		mc.QualityMetrics.AvgQualityScore = qualityScore
	} else {
		mc.QualityMetrics.AvgQualityScore = (mc.QualityMetrics.AvgQualityScore*float64(prevTotal) + qualityScore) / float64(mc.QualityMetrics.TotalResponses)
	}

	// Update quality rate buckets (these are counters)
	if qualityScore > 0.8 {
		mc.QualityMetrics.HighQualityCount++
	} else if qualityScore > 0.6 {
		mc.QualityMetrics.AcceptableQualityCount++
	} else {
		mc.QualityMetrics.PoorQualityCount++
	}

	// Convert to percentages
	total := float64(mc.QualityMetrics.TotalResponses)
	mc.QualityMetrics.HighQualityRate = (float64(mc.QualityMetrics.HighQualityCount) / total) * 100
	mc.QualityMetrics.AcceptableQualityRate = (float64(mc.QualityMetrics.AcceptableQualityCount) / total) * 100
	mc.QualityMetrics.PoorQualityRate = (float64(mc.QualityMetrics.PoorQualityCount) / total) * 100

	// Update average response time using running average
	if prevTotal == 0 {
		mc.QualityMetrics.AvgResponseTime = float64(responseTime.Milliseconds())
	} else {
		mc.QualityMetrics.AvgResponseTime = (mc.QualityMetrics.AvgResponseTime*float64(prevTotal) + float64(responseTime.Milliseconds())) / float64(mc.QualityMetrics.TotalResponses)
	}

	// Check for alerting
	if mc.QualityMetrics.AvgQualityScore < mc.AlertingConfig.QualityScoreThreshold {
		mc.triggerAlert("QUALITY_SCORE_LOW", "Response quality score below threshold", map[string]interface{}{
			"quality_score": mc.QualityMetrics.AvgQualityScore,
			"threshold":     mc.AlertingConfig.QualityScoreThreshold,
			"has_code":      hasCode,
			"has_diagram":   hasDiagram,
		})
	}

	if mc.QualityMetrics.AvgResponseTime > mc.AlertingConfig.ResponseTimeThreshold {
		mc.triggerAlert("RESPONSE_TIME_HIGH", "Response time exceeded threshold", map[string]interface{}{
			"response_time": mc.QualityMetrics.AvgResponseTime,
			"threshold":     mc.AlertingConfig.ResponseTimeThreshold,
		})
	}
}

// GetMetrics returns current metrics snapshot
func (mc *MetricsCollector) GetMetrics() map[string]interface{} {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	return map[string]interface{}{
		"code_generation":    mc.CodeGenMetrics,
		"diagram_generation": mc.DiagramMetrics,
		"response_quality":   mc.QualityMetrics,
		"alerting_config":    mc.AlertingConfig,
		"collected_at":       time.Now(),
	}
}

// ResetMetrics resets all metrics counters
func (mc *MetricsCollector) ResetMetrics() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.CodeGenMetrics = &CodeGenerationMetrics{
		ByLanguage: make(map[string]LanguageMetrics),
		ByDomain:   make(map[string]DomainMetrics),
		LastReset:  time.Now(),
	}
	mc.DiagramMetrics = &DiagramGenerationMetrics{
		ByType:    make(map[string]DiagramTypeMetrics),
		LastReset: time.Now(),
	}
	mc.QualityMetrics = &ResponseQualityMetrics{
		LastReset: time.Now(),
	}

	mc.logger.Info("Synthesis service metrics reset")
}

// triggerAlert sends an alert when thresholds are exceeded
func (mc *MetricsCollector) triggerAlert(alertType, message string, metadata map[string]interface{}) {
	if mc.alertCallback != nil {
		mc.alertCallback(alertType, message, metadata)
	}

	mc.logger.Warn("Synthesis service alert triggered",
		zap.String("alert_type", alertType),
		zap.String("message", message),
		zap.Any("metadata", metadata),
	)
}

// HealthCheck performs a health check on synthesis service metrics
func (mc *MetricsCollector) HealthCheck(ctx context.Context) (bool, string, map[string]interface{}) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	healthy := true
	issues := make([]string, 0)
	metadata := make(map[string]interface{})

	// Check code generation health
	if mc.CodeGenMetrics.TotalRequests > 0 {
		if mc.CodeGenMetrics.SuccessRate < 0.8 {
			healthy = false
			issues = append(issues, "Code generation success rate below 80%")
		}
		metadata["code_generation_success_rate"] = mc.CodeGenMetrics.SuccessRate
		metadata["code_generation_requests"] = mc.CodeGenMetrics.TotalRequests
	}

	// Check diagram generation health
	if mc.DiagramMetrics.TotalRequests > 0 {
		if mc.DiagramMetrics.SuccessRate < 0.85 {
			healthy = false
			issues = append(issues, "Diagram generation success rate below 85%")
		}
		metadata["diagram_generation_success_rate"] = mc.DiagramMetrics.SuccessRate
		metadata["diagram_generation_requests"] = mc.DiagramMetrics.TotalRequests
	}

	// Check quality metrics health
	if mc.QualityMetrics.TotalResponses > 0 {
		if mc.QualityMetrics.AvgQualityScore < 0.6 {
			healthy = false
			issues = append(issues, "Average quality score below 60%")
		}
		metadata["average_quality_score"] = mc.QualityMetrics.AvgQualityScore
		metadata["total_responses"] = mc.QualityMetrics.TotalResponses
	}

	// Check response time health
	if mc.QualityMetrics.AvgResponseTime > 30000 {
		healthy = false
		issues = append(issues, "Average response time above 30 seconds")
	}
	metadata["average_response_time_ms"] = mc.QualityMetrics.AvgResponseTime

	status := "healthy"
	if !healthy {
		status = "degraded"
	}

	return healthy, status, metadata
}
