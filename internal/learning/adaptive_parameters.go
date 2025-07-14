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
	"context"
	"database/sql"
	"math"
	"sync"
	"time"

	"go.uber.org/zap"
)

// AdaptiveParameterManager manages dynamic parameter adjustments
type AdaptiveParameterManager struct {
	analytics    *Analytics
	logger       *zap.Logger
	parameters   Parameters
	lastUpdate   time.Time
	mu           sync.RWMutex
	updateTicker *time.Ticker
}

// NewAdaptiveParameterManager creates a new adaptive parameter manager
func NewAdaptiveParameterManager(analytics *Analytics, logger *zap.Logger) *AdaptiveParameterManager {
	return &AdaptiveParameterManager{
		analytics:  analytics,
		logger:     logger,
		parameters: analytics.getDefaultParameters(),
		lastUpdate: time.Now(),
	}
}

// Start begins the adaptive parameter management process
func (apm *AdaptiveParameterManager) Start(ctx context.Context, updateInterval time.Duration) {
	apm.updateTicker = time.NewTicker(updateInterval)

	// Initial parameter load
	apm.updateParameters()

	go func() {
		for {
			select {
			case <-ctx.Done():
				apm.updateTicker.Stop()
				return
			case <-apm.updateTicker.C:
				apm.updateParameters()
			}
		}
	}()
}

// updateParameters updates parameters based on learning insights
func (apm *AdaptiveParameterManager) updateParameters() {
	apm.mu.Lock()
	defer apm.mu.Unlock()

	insights, err := apm.analytics.GetInsights()
	if err != nil {
		apm.logger.Error("Failed to get learning insights", zap.Error(err))
		return
	}

	// Update parameters based on insights
	newParams := apm.calculateAdaptiveParameters(insights)

	// Apply gradual changes to avoid instability
	apm.parameters = apm.smoothParameterTransition(apm.parameters, newParams)
	apm.lastUpdate = time.Now()

	apm.logger.Info("Parameters updated",
		zap.Float64("retrieval_threshold", apm.parameters.RetrievalThreshold),
		zap.Float64("fallback_threshold", apm.parameters.FallbackThreshold),
		zap.Float64("temperature_adjust", apm.parameters.TemperatureAdjust),
		zap.Int("chunk_limit_adjust", apm.parameters.ChunkLimitAdjust),
		zap.Float64("web_search_threshold", apm.parameters.WebSearchThreshold))
}

// calculateAdaptiveParameters calculates new parameters based on insights
func (apm *AdaptiveParameterManager) calculateAdaptiveParameters(insights *Insights) Parameters {
	params := apm.analytics.getDefaultParameters()

	// Adjust retrieval threshold based on overall quality trend
	if insights.ResponseQualityTrend < -0.2 {
		// Quality is declining, be more conservative
		params.RetrievalThreshold = math.Max(0.5, params.RetrievalThreshold-0.05)
		params.FallbackThreshold = math.Max(0.3, params.FallbackThreshold-0.05)
	} else if insights.ResponseQualityTrend > 0.2 {
		// Quality is improving, be more aggressive
		params.RetrievalThreshold = math.Min(0.9, params.RetrievalThreshold+0.05)
		params.FallbackThreshold = math.Min(0.7, params.FallbackThreshold+0.05)
	}

	// Adjust parameters based on query patterns
	totalQueries := 0
	negativeQueries := 0
	for queryType, satisfaction := range insights.QueryPatterns {
		totalQueries++
		if satisfaction < 0.6 {
			negativeQueries++
			// Adjust parameters for specific query types
			params = apm.adjustForQueryType(params, queryType, satisfaction)
		}
	}

	// Adjust web search threshold based on knowledge gaps
	if len(insights.KnowledgeGaps) > 0 {
		severitySum := 0.0
		for _, gap := range insights.KnowledgeGaps {
			severitySum += gap.Severity
		}
		avgSeverity := severitySum / float64(len(insights.KnowledgeGaps))

		// Lower web search threshold if we have knowledge gaps
		params.WebSearchThreshold = math.Max(0.3, params.WebSearchThreshold-avgSeverity*0.1)
	}

	return params
}

// adjustForQueryType adjusts parameters for specific query types
func (apm *AdaptiveParameterManager) adjustForQueryType(params Parameters, queryType string, satisfaction float64) Parameters {
	adjustment := (0.6 - satisfaction) * 0.1 // Scale adjustment based on dissatisfaction

	switch queryType {
	case "migration":
		// Migration queries need more comprehensive responses
		chunkAdjust := int(adjustment * 10)
		if chunkAdjust == 0 && satisfaction < 0.6 {
			chunkAdjust = 1 // Ensure at least some adjustment
		}
		params.ChunkLimitAdjust += chunkAdjust
		params.TemperatureAdjust -= adjustment * 0.1
	case "security":
		// Security queries need high accuracy
		params.RetrievalThreshold += adjustment
		params.TemperatureAdjust -= adjustment * 0.2
	case "hybrid":
		// Hybrid queries often need current information
		params.WebSearchThreshold -= adjustment
	case "disaster-recovery":
		// DR queries need comprehensive planning
		chunkAdjust := int(adjustment * 15)
		if chunkAdjust == 0 && satisfaction < 0.6 {
			chunkAdjust = 2 // Ensure at least some adjustment
		}
		params.ChunkLimitAdjust += chunkAdjust
		params.FallbackThreshold -= adjustment
	}

	return params
}

// smoothParameterTransition applies gradual parameter changes
func (apm *AdaptiveParameterManager) smoothParameterTransition(current, target Parameters) Parameters {
	const smoothingFactor = 0.3 // How much to move towards target (0-1)

	return Parameters{
		RetrievalThreshold: current.RetrievalThreshold +
			(target.RetrievalThreshold-current.RetrievalThreshold)*smoothingFactor,
		FallbackThreshold: current.FallbackThreshold +
			(target.FallbackThreshold-current.FallbackThreshold)*smoothingFactor,
		TemperatureAdjust: current.TemperatureAdjust +
			(target.TemperatureAdjust-current.TemperatureAdjust)*smoothingFactor,
		ChunkLimitAdjust: current.ChunkLimitAdjust +
			int(float64(target.ChunkLimitAdjust-current.ChunkLimitAdjust)*smoothingFactor),
		WebSearchThreshold: current.WebSearchThreshold +
			(target.WebSearchThreshold-current.WebSearchThreshold)*smoothingFactor,
	}
}

// GetCurrentParameters returns the current adaptive parameters
func (apm *AdaptiveParameterManager) GetCurrentParameters() Parameters {
	apm.mu.RLock()
	defer apm.mu.RUnlock()
	return apm.parameters
}

// GetParametersForQuery returns parameters optimized for a specific query
func (apm *AdaptiveParameterManager) GetParametersForQuery(query string) Parameters {
	apm.mu.RLock()
	defer apm.mu.RUnlock()

	params := apm.parameters
	queryType := classifyQuery(query)

	// Apply query-specific adjustments
	switch queryType {
	case "migration":
		params.ChunkLimitAdjust += 2
		params.TemperatureAdjust = math.Max(0, params.TemperatureAdjust-0.1)
	case "security":
		params.RetrievalThreshold = math.Min(0.9, params.RetrievalThreshold+0.1)
		params.TemperatureAdjust = math.Max(0, params.TemperatureAdjust-0.2)
	case "hybrid":
		params.WebSearchThreshold = math.Max(0.3, params.WebSearchThreshold-0.1)
	case "disaster-recovery":
		params.ChunkLimitAdjust += 3
		params.FallbackThreshold = math.Max(0.3, params.FallbackThreshold-0.1)
	case "cost-optimization":
		params.WebSearchThreshold = math.Max(0.2, params.WebSearchThreshold-0.2)
	}

	return params
}

// RecordParameterEffectiveness records how effective current parameters were
func (apm *AdaptiveParameterManager) RecordParameterEffectiveness(
	query string,
	params Parameters,
	responseTime time.Duration,
	userSatisfaction float64,
) error {
	// This would store parameter effectiveness data for future learning
	// For now, we'll just log it
	apm.logger.Info("Parameter effectiveness recorded",
		zap.String("query", query),
		zap.Duration("response_time", responseTime),
		zap.Float64("user_satisfaction", userSatisfaction),
		zap.Float64("retrieval_threshold", params.RetrievalThreshold),
		zap.Float64("fallback_threshold", params.FallbackThreshold))

	return nil
}

// GetParameterStats returns statistics about parameter usage and effectiveness
func (apm *AdaptiveParameterManager) GetParameterStats() map[string]interface{} {
	apm.mu.RLock()
	defer apm.mu.RUnlock()

	return map[string]interface{}{
		"current_parameters": apm.parameters,
		"last_update":        apm.lastUpdate,
		"uptime":             time.Since(apm.lastUpdate),
	}
}

// ParameterAPI provides HTTP API for parameter management
type ParameterAPI struct {
	manager *AdaptiveParameterManager
	db      *sql.DB
	logger  *zap.Logger
}

// NewParameterAPI creates a new parameter API
func NewParameterAPI(manager *AdaptiveParameterManager, db *sql.DB, logger *zap.Logger) *ParameterAPI {
	return &ParameterAPI{
		manager: manager,
		db:      db,
		logger:  logger,
	}
}

// GetParameters returns current parameters
func (api *ParameterAPI) GetParameters() (Parameters, error) {
	return api.manager.GetCurrentParameters(), nil
}

// GetParametersForQuery returns parameters optimized for a query
func (api *ParameterAPI) GetParametersForQuery(query string) (Parameters, error) {
	return api.manager.GetParametersForQuery(query), nil
}

// GetParameterStats returns parameter statistics
func (api *ParameterAPI) GetParameterStats() (map[string]interface{}, error) {
	return api.manager.GetParameterStats(), nil
}

// ForceParameterUpdate forces an immediate parameter update
func (api *ParameterAPI) ForceParameterUpdate() error {
	api.manager.updateParameters()
	api.logger.Info("Parameter update forced via API")
	return nil
}

// ResetParameters resets parameters to defaults
func (api *ParameterAPI) ResetParameters() error {
	api.manager.mu.Lock()
	defer api.manager.mu.Unlock()

	api.manager.parameters = api.manager.analytics.getDefaultParameters()
	api.manager.lastUpdate = time.Now()

	api.logger.Info("Parameters reset to defaults")
	return nil
}
