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

package learning

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewAdaptiveParameterManager(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	analytics := NewAnalytics(db, logger)

	apm := NewAdaptiveParameterManager(analytics, logger)
	assert.NotNil(t, apm)
	assert.Equal(t, analytics, apm.analytics)
	assert.Equal(t, logger, apm.logger)

	// Should initialize with default parameters
	params := apm.GetCurrentParameters()
	defaultParams := analytics.getDefaultParameters()
	assert.Equal(t, defaultParams, params)
}

func TestGetCurrentParameters(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	analytics := NewAnalytics(db, logger)
	apm := NewAdaptiveParameterManager(analytics, logger)

	params := apm.GetCurrentParameters()
	assert.NotNil(t, params)
	assert.Greater(t, params.RetrievalThreshold, 0.0)
	assert.Greater(t, params.FallbackThreshold, 0.0)
}

func TestGetParametersForQuery(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	analytics := NewAnalytics(db, logger)
	apm := NewAdaptiveParameterManager(analytics, logger)

	testCases := []struct {
		query          string
		expectedAdjust bool
		description    string
	}{
		{
			query:          "How to migrate 100 VMs to AWS?",
			expectedAdjust: true,
			description:    "Migration query should adjust chunk limit",
		},
		{
			query:          "Security compliance requirements for HIPAA",
			expectedAdjust: true,
			description:    "Security query should adjust retrieval threshold",
		},
		{
			query:          "Hybrid cloud connectivity options",
			expectedAdjust: true,
			description:    "Hybrid query should adjust web search threshold",
		},
		{
			query:          "Disaster recovery planning",
			expectedAdjust: true,
			description:    "DR query should adjust chunk limit and fallback",
		},
		{
			query:          "Cost optimization strategies",
			expectedAdjust: true,
			description:    "Cost query should adjust web search threshold",
		},
		{
			query:          "General question about cloud",
			expectedAdjust: false,
			description:    "General query should use default parameters",
		},
	}

	defaultParams := apm.GetCurrentParameters()

	for _, tc := range testCases {
		params := apm.GetParametersForQuery(tc.query)

		if tc.expectedAdjust {
			// At least one parameter should be different from default
			different := params.RetrievalThreshold != defaultParams.RetrievalThreshold ||
				params.FallbackThreshold != defaultParams.FallbackThreshold ||
				params.TemperatureAdjust != defaultParams.TemperatureAdjust ||
				params.ChunkLimitAdjust != defaultParams.ChunkLimitAdjust ||
				params.WebSearchThreshold != defaultParams.WebSearchThreshold

			assert.True(t, different, tc.description)
		} else {
			// Parameters should be the same as default for general queries
			assert.Equal(t, defaultParams, params, tc.description)
		}
	}
}

func TestCalculateAdaptiveParameters(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	analytics := NewAnalytics(db, logger)
	apm := NewAdaptiveParameterManager(analytics, logger)

	// Test with declining quality trend
	decliningInsights := &Insights{
		QueryPatterns: map[string]float64{
			"migration": 0.3, // Low satisfaction
			"security":  0.4,
		},
		KnowledgeGaps: []KnowledgeGap{
			{Topic: "security", Severity: 0.8, NegativeFeedback: 5},
		},
		ResponseQualityTrend: -0.3, // Declining
	}

	params := apm.calculateAdaptiveParameters(decliningInsights)
	defaultParams := apm.analytics.getDefaultParameters()

	// Should be more conservative with declining quality
	assert.LessOrEqual(t, params.RetrievalThreshold, defaultParams.RetrievalThreshold)
	assert.LessOrEqual(t, params.FallbackThreshold, defaultParams.FallbackThreshold)

	// Should lower web search threshold due to knowledge gaps
	assert.Less(t, params.WebSearchThreshold, defaultParams.WebSearchThreshold)

	// Test with improving quality trend
	improvingInsights := &Insights{
		QueryPatterns: map[string]float64{
			"migration": 0.9, // High satisfaction
			"security":  0.8,
		},
		KnowledgeGaps:        []KnowledgeGap{}, // No gaps
		ResponseQualityTrend: 0.3,              // Improving
	}

	params = apm.calculateAdaptiveParameters(improvingInsights)

	// Should be more aggressive with improving quality
	assert.GreaterOrEqual(t, params.RetrievalThreshold, defaultParams.RetrievalThreshold)
	assert.GreaterOrEqual(t, params.FallbackThreshold, defaultParams.FallbackThreshold)
}

func TestSmoothParameterTransition(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	analytics := NewAnalytics(db, logger)
	apm := NewAdaptiveParameterManager(analytics, logger)

	current := Parameters{
		RetrievalThreshold: 0.5,
		FallbackThreshold:  0.3,
		TemperatureAdjust:  0.0,
		ChunkLimitAdjust:   0,
		WebSearchThreshold: 0.6,
	}

	target := Parameters{
		RetrievalThreshold: 0.9,
		FallbackThreshold:  0.7,
		TemperatureAdjust:  0.2,
		ChunkLimitAdjust:   5,
		WebSearchThreshold: 0.2,
	}

	smoothed := apm.smoothParameterTransition(current, target)

	// Should move towards target but not reach it (smoothing factor = 0.3)
	assert.Greater(t, smoothed.RetrievalThreshold, current.RetrievalThreshold)
	assert.Less(t, smoothed.RetrievalThreshold, target.RetrievalThreshold)

	assert.Greater(t, smoothed.FallbackThreshold, current.FallbackThreshold)
	assert.Less(t, smoothed.FallbackThreshold, target.FallbackThreshold)

	assert.Greater(t, smoothed.TemperatureAdjust, current.TemperatureAdjust)
	assert.Less(t, smoothed.TemperatureAdjust, target.TemperatureAdjust)

	assert.Greater(t, smoothed.ChunkLimitAdjust, current.ChunkLimitAdjust)
	assert.Less(t, smoothed.ChunkLimitAdjust, target.ChunkLimitAdjust)

	assert.Less(t, smoothed.WebSearchThreshold, current.WebSearchThreshold)
	assert.Greater(t, smoothed.WebSearchThreshold, target.WebSearchThreshold)
}

func TestAdjustForQueryType(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	analytics := NewAnalytics(db, logger)
	apm := NewAdaptiveParameterManager(analytics, logger)

	baseParams := apm.analytics.getDefaultParameters()

	testCases := []struct {
		queryType    string
		satisfaction float64
		checkField   string
		expectChange bool
	}{
		{
			queryType:    "migration",
			satisfaction: 0.3, // Low satisfaction
			checkField:   "ChunkLimitAdjust",
			expectChange: true,
		},
		{
			queryType:    "security",
			satisfaction: 0.2, // Very low satisfaction
			checkField:   "RetrievalThreshold",
			expectChange: true,
		},
		{
			queryType:    "hybrid",
			satisfaction: 0.4, // Low satisfaction
			checkField:   "WebSearchThreshold",
			expectChange: true,
		},
		{
			queryType:    "disaster-recovery",
			satisfaction: 0.3, // Low satisfaction
			checkField:   "ChunkLimitAdjust",
			expectChange: true,
		},
	}

	for _, tc := range testCases {
		adjustedParams := apm.adjustForQueryType(baseParams, tc.queryType, tc.satisfaction)

		switch tc.checkField {
		case "ChunkLimitAdjust":
			if tc.expectChange {
				assert.NotEqual(t, baseParams.ChunkLimitAdjust, adjustedParams.ChunkLimitAdjust,
					"ChunkLimitAdjust should change for %s", tc.queryType)
			}
		case "RetrievalThreshold":
			if tc.expectChange {
				assert.NotEqual(t, baseParams.RetrievalThreshold, adjustedParams.RetrievalThreshold,
					"RetrievalThreshold should change for %s", tc.queryType)
			}
		case "WebSearchThreshold":
			if tc.expectChange {
				assert.NotEqual(t, baseParams.WebSearchThreshold, adjustedParams.WebSearchThreshold,
					"WebSearchThreshold should change for %s", tc.queryType)
			}
		}
	}
}

func TestRecordParameterEffectiveness(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	analytics := NewAnalytics(db, logger)
	apm := NewAdaptiveParameterManager(analytics, logger)

	params := apm.GetCurrentParameters()
	err := apm.RecordParameterEffectiveness(
		"test query",
		params,
		100*time.Millisecond,
		0.8,
	)

	// Should not return error (currently just logs)
	assert.NoError(t, err)
}

func TestGetParameterStats(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	analytics := NewAnalytics(db, logger)
	apm := NewAdaptiveParameterManager(analytics, logger)

	stats := apm.GetParameterStats()
	assert.NotNil(t, stats)
	assert.Contains(t, stats, "current_parameters")
	assert.Contains(t, stats, "last_update")
	assert.Contains(t, stats, "uptime")

	params, ok := stats["current_parameters"].(Parameters)
	assert.True(t, ok)
	assert.Greater(t, params.RetrievalThreshold, 0.0)
}

func TestParameterAPI(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	analytics := NewAnalytics(db, logger)
	apm := NewAdaptiveParameterManager(analytics, logger)
	api := NewParameterAPI(apm, db, logger)

	// Test GetParameters
	params, err := api.GetParameters()
	assert.NoError(t, err)
	assert.NotNil(t, params)
	assert.Greater(t, params.RetrievalThreshold, 0.0)

	// Test GetParametersForQuery
	params, err = api.GetParametersForQuery("migration query")
	assert.NoError(t, err)
	assert.NotNil(t, params)

	// Test GetParameterStats
	stats, err := api.GetParameterStats()
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Contains(t, stats, "current_parameters")

	// Test ForceParameterUpdate
	err = api.ForceParameterUpdate()
	assert.NoError(t, err)

	// Test ResetParameters
	err = api.ResetParameters()
	assert.NoError(t, err)

	// Parameters should be reset to defaults
	params, err = api.GetParameters()
	assert.NoError(t, err)
	defaultParams := analytics.getDefaultParameters()
	assert.Equal(t, defaultParams, params)
}

func TestAdaptiveParameterManagerConcurrency(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	analytics := NewAnalytics(db, logger)
	apm := NewAdaptiveParameterManager(analytics, logger)

	// Test concurrent access to parameters
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()

			// Read parameters
			_ = apm.GetCurrentParameters()

			// Get query-specific parameters
			_ = apm.GetParametersForQuery("test query")

			// Record effectiveness
			_ = apm.RecordParameterEffectiveness(
				"test query",
				apm.GetCurrentParameters(),
				time.Millisecond,
				0.5,
			)
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic or deadlock
	assert.True(t, true)
}

func BenchmarkGetParametersForQuery(b *testing.B) {
	db := setupTestDB(b)
	logger := zap.NewNop()
	analytics := NewAnalytics(db, logger)
	apm := NewAdaptiveParameterManager(analytics, logger)

	queries := []string{
		"How to migrate to AWS?",
		"Security compliance requirements",
		"Hybrid cloud setup",
		"Disaster recovery planning",
		"Cost optimization strategies",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		query := queries[i%len(queries)]
		_ = apm.GetParametersForQuery(query)
	}
}
