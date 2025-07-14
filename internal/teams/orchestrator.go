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

// Package teams provides orchestration logic for backend service coordination
// in the Teams bot webhook handler, including service health validation,
// fallback handling, and async processing.
package teams

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/diagram"
	"github.com/your-org/ai-sa-assistant/internal/health"
	"github.com/your-org/ai-sa-assistant/internal/session"
	"github.com/your-org/ai-sa-assistant/internal/streaming"
	"github.com/your-org/ai-sa-assistant/internal/synth"
	"go.uber.org/zap"
)

const (
	// DefaultHTTPTimeout is the default timeout for HTTP requests
	DefaultHTTPTimeout = 30 * time.Second
	// FallbackScore is the score assigned to fallback responses
	FallbackScore = 0.5
	// MaxFallbackChunks is the maximum number of chunks to include in fallback responses
	MaxFallbackChunks = 3
)

// OrchestrationResult represents the result of orchestrating backend services
type OrchestrationResult struct {
	Response           *synth.SynthesisResponse
	Error              error
	ServicesTested     []string
	ServicesUsed       []string
	FallbackUsed       bool
	ExecutionTimeMs    int64
	HealthChecksPassed bool
	Query              string               // Query that was processed
	PresetUsed         string               // Regeneration preset used (if any)
	ProcessingTime     time.Duration        // Total processing time
	Errors             []string             // List of errors encountered
	Metrics            OrchestrationMetrics // Detailed metrics
}

// OrchestrationMetrics contains detailed timing and performance metrics
type OrchestrationMetrics struct {
	StartTime     time.Time
	EndTime       time.Time
	RetrievalTime time.Duration
	WebSearchTime time.Duration
	SynthesisTime time.Duration
}

// ServiceStatus represents the health status of a service
type ServiceStatus struct {
	Name      string
	Healthy   bool
	Error     error
	Timestamp time.Time
}

// Orchestrator handles backend service orchestration for Teams webhook
type Orchestrator struct {
	config          *config.Config
	healthManager   *health.Manager
	diagramRenderer *diagram.Renderer
	sessionManager  *session.Manager
	logger          *zap.Logger
	httpClient      *http.Client
}

// NewOrchestrator creates a new service orchestrator
func NewOrchestrator(
	cfg *config.Config,
	healthManager *health.Manager,
	diagramRenderer *diagram.Renderer,
	sessionManager *session.Manager,
	logger *zap.Logger,
) *Orchestrator {
	return &Orchestrator{
		config:          cfg,
		healthManager:   healthManager,
		diagramRenderer: diagramRenderer,
		sessionManager:  sessionManager,
		logger:          logger,
		httpClient: &http.Client{
			Timeout: DefaultHTTPTimeout,
		},
	}
}

// ProcessQuery orchestrates the full service pipeline for a user query with session management
func (o *Orchestrator) ProcessQuery(ctx context.Context, query string, userID string) *OrchestrationResult {
	return o.ProcessQueryWithStreaming(ctx, query, userID, nil)
}

// ProcessQueryWithStreaming orchestrates the full service pipeline with optional streaming progress
func (o *Orchestrator) ProcessQueryWithStreaming(ctx context.Context, query string, userID string, eventStream *streaming.EventStream) *OrchestrationResult {
	startTime := time.Now()
	result := &OrchestrationResult{
		ServicesTested: []string{},
		ServicesUsed:   []string{},
		FallbackUsed:   false,
	}

	orchestrationID := fmt.Sprintf("orch_%d", startTime.UnixNano())
	o.logger.Info("Starting query orchestration",
		zap.String("query", query),
		zap.String("user_id", userID),
		zap.String("orchestration_id", orchestrationID))

	// Emit initial progress if streaming is enabled
	if eventStream != nil {
		eventStream.EmitProgress(streaming.StageQueryAnalysis, "🔍 Analyzing query for metadata filters...", 5, map[string]interface{}{
			"orchestration_id": orchestrationID,
			"query":            query,
		})
	}

	// Step 0: Handle session management
	sessionID, conversationHistory, err := o.handleSessionManagement(ctx, userID, query)
	if err != nil {
		o.logger.Warn("Session management failed, continuing without context", zap.Error(err))
		sessionID = ""
		conversationHistory = []session.Message{}
	}

	// Step 1: Validate service health
	if eventStream != nil {
		eventStream.EmitProgress(streaming.StageQueryAnalysis, "⚡ Validating service health...", 10, nil)
	}

	if !o.validateServiceHealthWithStreaming(ctx, result, eventStream) {
		result.Error = fmt.Errorf("service health validation failed")
		result.ExecutionTimeMs = time.Since(startTime).Milliseconds()
		if eventStream != nil {
			eventStream.EmitError(streaming.StageQueryAnalysis, "❌ Service health check failed", result.Error, nil)
		}
		return result
	}

	// Step 2: Call retrieve service with fallback
	if eventStream != nil {
		eventStream.EmitProgress(streaming.StageMetadataFilter, "📊 Searching metadata database...", 15, nil)
	}

	retrieveResponse, err := o.callRetrieveServiceWithFallbackStreaming(ctx, query, result, eventStream)
	if err != nil {
		result.Error = fmt.Errorf("retrieve service failed: %w", err)
		result.ExecutionTimeMs = time.Since(startTime).Milliseconds()
		if eventStream != nil {
			eventStream.EmitError(streaming.StageVectorSearch, "❌ Retrieval failed", err, nil)
		}
		return result
	}

	// Step 3: Conditionally call web search service
	var webResults []string
	needsWeb := o.needsFreshness(query)
	if needsWeb {
		if eventStream != nil {
			eventStream.EmitProgress(streaming.StageFreshnessDetection, "🌐 Freshness keywords detected, triggering web search...", 60, map[string]interface{}{
				"freshness_detected": true,
			})
		}
		webResults = o.callWebSearchServiceWithFallbackStreaming(ctx, query, result, eventStream)
	} else if eventStream != nil {
		eventStream.EmitProgress(streaming.StageFreshnessDetection, "✓ No freshness keywords detected", 60, map[string]interface{}{
			"freshness_detected": false,
		})
	}

	// Step 4: Call synthesize service with fallback (including conversation context)
	if eventStream != nil {
		eventStream.EmitProgress(streaming.StageSynthesis, "🤖 Synthesizing response with GPT-4o...", 75, map[string]interface{}{
			"context_items": len(retrieveResponse.Chunks),
			"web_results":   len(webResults),
		})
	}

	synthesizeResponse, err := o.callSynthesizeServiceWithFallbackStreaming(
		ctx, query, retrieveResponse, webResults, conversationHistory, result, eventStream,
	)
	if err != nil {
		result.Error = fmt.Errorf("synthesize service failed: %w", err)
		result.ExecutionTimeMs = time.Since(startTime).Milliseconds()
		if eventStream != nil {
			eventStream.EmitError(streaming.StageSynthesis, "❌ Synthesis failed", err, nil)
		}
		return result
	}

	// Step 5: Render diagram if present
	if synthesizeResponse.DiagramCode != "" {
		if eventStream != nil {
			eventStream.EmitProgress(streaming.StageDiagramRendering, "📊 Rendering architecture diagram...", 90, nil)
		}
		o.renderDiagramWithFallbackStreaming(ctx, synthesizeResponse, result, eventStream)
	}

	result.Response = synthesizeResponse
	result.ExecutionTimeMs = time.Since(startTime).Milliseconds()

	// Step 6: Store response in session (if session management is working)
	if sessionID != "" {
		if err := o.storeResponseInSession(ctx, sessionID, query, synthesizeResponse); err != nil {
			o.logger.Warn("Failed to store response in session",
				zap.String("session_id", sessionID), zap.Error(err))
		}
	}

	if eventStream != nil {
		eventStream.EmitProgress(streaming.StageComplete, "✅ Processing complete", 100, map[string]interface{}{
			"execution_time_ms": result.ExecutionTimeMs,
			"services_used":     result.ServicesUsed,
			"fallback_used":     result.FallbackUsed,
		})
	}

	o.logger.Info("Query orchestration completed",
		zap.String("query", query),
		zap.String("user_id", userID),
		zap.String("session_id", sessionID),
		zap.Int64("execution_time_ms", result.ExecutionTimeMs),
		zap.Strings("services_used", result.ServicesUsed),
		zap.Bool("fallback_used", result.FallbackUsed))

	return result
}

// isServiceHealthy checks if a specific service is healthy
func (o *Orchestrator) isServiceHealthy(ctx context.Context, serviceName string) bool {
	var url string
	switch serviceName {
	case "retrieve":
		url = o.config.Services.RetrieveURL + "/health"
	case "synthesize":
		url = o.config.Services.SynthesizeURL + "/health"
	case "websearch":
		url = o.config.Services.WebSearchURL + "/health"
	default:
		return false
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode == http.StatusOK
}

// validateServiceHealthWithStreaming checks the health of required services with streaming progress
func (o *Orchestrator) validateServiceHealthWithStreaming(ctx context.Context, result *OrchestrationResult, eventStream *streaming.EventStream) bool {
	// Skip health checks in demo/development environment
	if os.Getenv("ENVIRONMENT") == "development" || os.Getenv("SKIP_HEALTH_CHECKS") == "true" {
		if eventStream != nil {
			eventStream.EmitProgress(streaming.StageQueryAnalysis, "⚡ Skipping health checks (demo mode)...", 15, nil)
		}
		o.logger.Info("Skipping service health checks in demo environment")
		result.HealthChecksPassed = true
		result.ServicesTested = []string{"retrieve", "synthesize", "websearch"}
		return true
	}

	requiredServices := []string{"retrieve", "synthesize"}
	healthyServices := 0

	for i, serviceName := range requiredServices {
		result.ServicesTested = append(result.ServicesTested, serviceName)

		if eventStream != nil {
			progress := 10 + (i * 2) // 10, 12% progress
			eventStream.EmitProgress(streaming.StageQueryAnalysis, fmt.Sprintf("⚡ Checking %s service...", serviceName), progress, nil)
		}

		if o.isServiceHealthy(ctx, serviceName) {
			healthyServices++
			o.logger.Debug("Service health check passed",
				zap.String("service", serviceName))
		} else {
			o.logger.Warn("Service health check failed",
				zap.String("service", serviceName))
		}
	}

	// Check websearch service health (optional)
	result.ServicesTested = append(result.ServicesTested, "websearch")
	if eventStream != nil {
		eventStream.EmitProgress(streaming.StageQueryAnalysis, "⚡ Checking websearch service...", 14, nil)
	}

	if o.isServiceHealthy(ctx, "websearch") {
		o.logger.Debug("Web search service health check passed")
	} else {
		o.logger.Warn("Web search service health check failed")
	}

	result.HealthChecksPassed = healthyServices == len(requiredServices)
	return result.HealthChecksPassed
}

// callRetrieveServiceWithFallback calls the retrieve service with fallback logic
func (o *Orchestrator) callRetrieveServiceWithFallback(
	ctx context.Context,
	query string,
	result *OrchestrationResult,
) (*RetrieveResponse, error) {
	o.logger.Info("Calling retrieve service", zap.String("query", query))

	reqBody := map[string]string{"query": query}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.config.Services.RetrieveURL+"/search", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		o.logger.Error("Retrieve service request failed", zap.Error(err))
		return o.fallbackRetrieveResponse(query, result), nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		o.logger.Error("Retrieve service returned error",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response_body", string(body)))
		return o.fallbackRetrieveResponse(query, result), nil
	}

	var retrieveResponse RetrieveResponse
	if err := json.NewDecoder(resp.Body).Decode(&retrieveResponse); err != nil {
		o.logger.Error("Failed to decode retrieve response", zap.Error(err))
		return o.fallbackRetrieveResponse(query, result), nil
	}

	result.ServicesUsed = append(result.ServicesUsed, "retrieve")
	o.logger.Info("Retrieve service call successful",
		zap.String("query", query),
		zap.Int("chunks_returned", len(retrieveResponse.Chunks)))

	return &retrieveResponse, nil
}

// callRetrieveServiceWithFallbackStreaming calls the retrieve service with fallback logic and streaming progress
func (o *Orchestrator) callRetrieveServiceWithFallbackStreaming(
	ctx context.Context,
	query string,
	result *OrchestrationResult,
	eventStream *streaming.EventStream,
) (*RetrieveResponse, error) {
	o.logger.Info("Calling retrieve service", zap.String("query", query))

	if eventStream != nil {
		eventStream.EmitProgress(streaming.StageEmbeddings, "🧠 Generating embeddings for query...", 25, nil)
	}

	reqBody := map[string]string{"query": query}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	if eventStream != nil {
		eventStream.EmitProgress(streaming.StageVectorSearch, "🔎 Vector search in ChromaDB...", 40, nil)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.config.Services.RetrieveURL+"/search", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		o.logger.Error("Retrieve service request failed", zap.Error(err))
		if eventStream != nil {
			eventStream.EmitProgress(streaming.StageVectorSearch, "⚠️ Retrieve service failed, using fallback...", 45, map[string]interface{}{
				"fallback_triggered": true,
			})
		}
		return o.fallbackRetrieveResponse(query, result), nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		o.logger.Error("Retrieve service returned error",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response_body", string(body)))
		if eventStream != nil {
			eventStream.EmitProgress(streaming.StageVectorSearch, "⚠️ Retrieve service error, using fallback...", 45, map[string]interface{}{
				"fallback_triggered": true,
				"status_code":        resp.StatusCode,
			})
		}
		return o.fallbackRetrieveResponse(query, result), nil
	}

	var retrieveResponse RetrieveResponse
	if err := json.NewDecoder(resp.Body).Decode(&retrieveResponse); err != nil {
		o.logger.Error("Failed to decode retrieve response", zap.Error(err))
		if eventStream != nil {
			eventStream.EmitProgress(streaming.StageVectorSearch, "⚠️ Failed to parse response, using fallback...", 45, map[string]interface{}{
				"fallback_triggered": true,
			})
		}
		return o.fallbackRetrieveResponse(query, result), nil
	}

	result.ServicesUsed = append(result.ServicesUsed, "retrieve")

	if eventStream != nil {
		eventStream.EmitProgress(streaming.StageVectorSearch, "✓ Retrieved relevant documents", 55, map[string]interface{}{
			"chunks_found": len(retrieveResponse.Chunks),
		})
	}

	o.logger.Info("Retrieve service call successful",
		zap.String("query", query),
		zap.Int("chunks_returned", len(retrieveResponse.Chunks)))

	return &retrieveResponse, nil
}

// fallbackRetrieveResponse provides a fallback response when retrieve service fails
func (o *Orchestrator) fallbackRetrieveResponse(query string, result *OrchestrationResult) *RetrieveResponse {
	o.logger.Warn("Using fallback retrieve response")
	result.FallbackUsed = true

	// Return a minimal response that can still be processed
	return &RetrieveResponse{
		Chunks: []RetrieveChunk{
			{
				Text: "I apologize, but I'm currently experiencing issues accessing my knowledge base. " +
					"I can still help you with general cloud architecture guidance.",
				Score:    FallbackScore,
				DocID:    "fallback",
				SourceID: "fallback",
				Metadata: map[string]interface{}{
					"fallback": true,
				},
			},
		},
		Count:             1,
		Query:             query,
		FallbackTriggered: true,
		FallbackReason:    "retrieve service unavailable",
	}
}

// callWebSearchServiceWithFallback calls the web search service with fallback logic
func (o *Orchestrator) callWebSearchServiceWithFallback(
	ctx context.Context,
	query string,
	result *OrchestrationResult,
) []string {
	o.logger.Info("Calling web search service", zap.String("query", query))

	reqBody := map[string]string{"query": query}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		o.logger.Error("Failed to marshal web search request", zap.Error(err))
		return []string{}
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		o.config.Services.WebSearchURL+"/search",
		bytes.NewBuffer(jsonBody),
	)
	if err != nil {
		o.logger.Error("Failed to create web search request", zap.Error(err))
		return []string{}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		o.logger.Warn("Web search service request failed, continuing without web results", zap.Error(err))
		return []string{}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		o.logger.Warn("Web search service returned error, continuing without web results",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response_body", string(body)))
		return []string{}
	}

	var webResponse struct {
		Results []string `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&webResponse); err != nil {
		o.logger.Warn("Failed to decode web search response, continuing without web results", zap.Error(err))
		return []string{}
	}

	result.ServicesUsed = append(result.ServicesUsed, "websearch")
	o.logger.Info("Web search service call successful",
		zap.String("query", query),
		zap.Int("results_returned", len(webResponse.Results)))

	return webResponse.Results
}

// callWebSearchServiceWithFallbackStreaming calls the web search service with fallback logic and streaming progress
func (o *Orchestrator) callWebSearchServiceWithFallbackStreaming(
	ctx context.Context,
	query string,
	result *OrchestrationResult,
	eventStream *streaming.EventStream,
) []string {
	o.logger.Info("Calling web search service", zap.String("query", query))

	if eventStream != nil {
		eventStream.EmitProgress(streaming.StageWebSearch, "🔍 Searching web for latest updates...", 62, nil)
	}

	reqBody := map[string]string{"query": query}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		o.logger.Error("Failed to marshal web search request", zap.Error(err))
		if eventStream != nil {
			eventStream.EmitProgress(streaming.StageWebSearch, "⚠️ Web search request failed", 65, map[string]interface{}{
				"error": "failed to marshal request",
			})
		}
		return []string{}
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		o.config.Services.WebSearchURL+"/search",
		bytes.NewBuffer(jsonBody),
	)
	if err != nil {
		o.logger.Error("Failed to create web search request", zap.Error(err))
		if eventStream != nil {
			eventStream.EmitProgress(streaming.StageWebSearch, "⚠️ Web search request failed", 65, map[string]interface{}{
				"error": "failed to create request",
			})
		}
		return []string{}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		o.logger.Warn("Web search service request failed, continuing without web results", zap.Error(err))
		if eventStream != nil {
			eventStream.EmitProgress(streaming.StageWebSearch, "⚠️ Web search unavailable, continuing...", 65, map[string]interface{}{
				"error": "service request failed",
			})
		}
		return []string{}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		o.logger.Warn("Web search service returned error, continuing without web results",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response_body", string(body)))
		if eventStream != nil {
			eventStream.EmitProgress(streaming.StageWebSearch, "⚠️ Web search error, continuing...", 65, map[string]interface{}{
				"status_code": resp.StatusCode,
			})
		}
		return []string{}
	}

	var webResponse struct {
		Results []string `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&webResponse); err != nil {
		o.logger.Warn("Failed to decode web search response, continuing without web results", zap.Error(err))
		if eventStream != nil {
			eventStream.EmitProgress(streaming.StageWebSearch, "⚠️ Failed to parse web results", 65, map[string]interface{}{
				"error": "decode failed",
			})
		}
		return []string{}
	}

	result.ServicesUsed = append(result.ServicesUsed, "websearch")

	if eventStream != nil {
		eventStream.EmitProgress(streaming.StageWebSearch, "✓ Web search complete", 70, map[string]interface{}{
			"results_found": len(webResponse.Results),
		})
	}

	o.logger.Info("Web search service call successful",
		zap.String("query", query),
		zap.Int("results_returned", len(webResponse.Results)))

	return webResponse.Results
}

// callSynthesizeServiceWithFallbackStreaming calls the synthesize service with fallback logic and streaming progress
func (o *Orchestrator) callSynthesizeServiceWithFallbackStreaming(
	ctx context.Context,
	query string,
	retrieveResponse *RetrieveResponse,
	webResults []string,
	conversationHistory []session.Message,
	result *OrchestrationResult,
	eventStream *streaming.EventStream,
) (*synth.SynthesisResponse, error) {
	o.logger.Info("Calling synthesize service", zap.String("query", query))

	if eventStream != nil {
		eventStream.EmitProgress(streaming.StageSynthesis, "🔧 Preparing synthesis request...", 78, nil)
	}

	// Convert retrieve chunks to synthesis format
	contextItems := o.convertRetrieveChunksToContextItems(retrieveResponse.Chunks)
	synthesizeRequest := o.createSynthesizeRequest(query, contextItems, webResults, conversationHistory)

	jsonBody, err := json.Marshal(synthesizeRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal synthesize request: %w", err)
	}

	if eventStream != nil {
		eventStream.EmitProgress(streaming.StageSynthesis, "🤖 Generating response with GPT-4o...", 82, map[string]interface{}{
			"model": "gpt-4o",
		})
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		o.config.Services.SynthesizeURL+"/synthesize",
		bytes.NewBuffer(jsonBody),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create synthesize request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		o.logger.Error("Synthesize service request failed", zap.Error(err))
		if eventStream != nil {
			eventStream.EmitProgress(streaming.StageSynthesis, "⚠️ Synthesis service failed, using fallback...", 85, map[string]interface{}{
				"fallback_triggered": true,
			})
		}
		return o.fallbackSynthesizeResponse(query, retrieveResponse, conversationHistory, result), nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		o.logger.Error("Synthesize service returned error",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response_body", string(body)))
		if eventStream != nil {
			eventStream.EmitProgress(streaming.StageSynthesis, "⚠️ Synthesis service error, using fallback...", 85, map[string]interface{}{
				"fallback_triggered": true,
				"status_code":        resp.StatusCode,
			})
		}
		return o.fallbackSynthesizeResponse(query, retrieveResponse, conversationHistory, result), nil
	}

	var synthesizeResponse synth.SynthesisResponse
	if err := json.NewDecoder(resp.Body).Decode(&synthesizeResponse); err != nil {
		o.logger.Error("Failed to decode synthesize response", zap.Error(err))
		if eventStream != nil {
			eventStream.EmitProgress(streaming.StageSynthesis, "⚠️ Failed to parse synthesis response, using fallback...", 85, map[string]interface{}{
				"fallback_triggered": true,
			})
		}
		return o.fallbackSynthesizeResponse(query, retrieveResponse, conversationHistory, result), nil
	}

	result.ServicesUsed = append(result.ServicesUsed, "synthesize")

	if eventStream != nil {
		eventStream.EmitProgress(streaming.StageSynthesis, "✓ Response synthesis complete", 88, map[string]interface{}{
			"response_length": len(synthesizeResponse.MainText),
			"has_diagram":     synthesizeResponse.DiagramCode != "",
			"code_snippets":   len(synthesizeResponse.CodeSnippets),
		})
	}

	o.logger.Info("Synthesize service call successful",
		zap.String("query", query),
		zap.Int("main_text_length", len(synthesizeResponse.MainText)))

	return &synthesizeResponse, nil
}

// fallbackSynthesizeResponse provides a fallback response when synthesize service fails
func (o *Orchestrator) fallbackSynthesizeResponse(
	query string,
	retrieveResponse *RetrieveResponse,
	_ []session.Message,
	result *OrchestrationResult,
) *synth.SynthesisResponse {
	o.logger.Warn("Using fallback synthesize response")
	result.FallbackUsed = true

	// Create a basic response using the retrieve results
	var mainText string
	if len(retrieveResponse.Chunks) > 0 {
		mainText = fmt.Sprintf("I found some relevant information for your query: %s\n\n", query)
		for i, chunk := range retrieveResponse.Chunks {
			if i >= MaxFallbackChunks { // Limit to first few chunks
				break
			}
			mainText += fmt.Sprintf("• %s\n", chunk.Text)
		}
		mainText += "\n*Note: I'm experiencing issues with my synthesis service and providing you with raw information.*"
	} else {
		mainText = fmt.Sprintf(
			"I apologize, but I'm currently experiencing technical difficulties and "+
				"cannot provide a comprehensive response to your query: %s",
			query,
		)
	}

	// Extract sources from retrieve response
	var sources []string
	for _, chunk := range retrieveResponse.Chunks {
		if chunk.SourceID != "" && chunk.SourceID != "fallback" {
			sources = append(sources, chunk.SourceID)
		}
	}

	return &synth.SynthesisResponse{
		MainText:     mainText,
		DiagramCode:  "",
		CodeSnippets: []synth.CodeSnippet{},
		Sources:      sources,
		DiagramURL:   "",
	}
}

// renderDiagramWithFallbackStreaming renders diagrams with fallback logic and streaming progress
func (o *Orchestrator) renderDiagramWithFallbackStreaming(
	ctx context.Context,
	response *synth.SynthesisResponse,
	result *OrchestrationResult,
	eventStream *streaming.EventStream,
) {
	o.logger.Info("Rendering diagram", zap.String("diagram_code_length", fmt.Sprintf("%d", len(response.DiagramCode))))

	if eventStream != nil {
		eventStream.EmitProgress(streaming.StageDiagramRendering, "🎨 Converting diagram to image...", 92, nil)
	}

	diagramURL, fallbackText, err := o.diagramRenderer.RenderDiagramWithFallback(ctx, response.DiagramCode)
	if err != nil {
		o.logger.Warn("Failed to render diagram", zap.Error(err))
		result.FallbackUsed = true
		if eventStream != nil {
			eventStream.EmitProgress(streaming.StageDiagramRendering, "⚠️ Diagram rendering failed, using text fallback", 95, map[string]interface{}{
				"fallback_triggered": true,
				"error":              err.Error(),
			})
		}
	}

	if fallbackText != "" {
		response.MainText += "\n\n" + fallbackText
		response.DiagramCode = ""
		result.FallbackUsed = true
		if eventStream != nil {
			eventStream.EmitProgress(streaming.StageDiagramRendering, "✓ Diagram rendered as text", 95, map[string]interface{}{
				"fallback_used": true,
			})
		}
	} else if eventStream != nil {
		eventStream.EmitProgress(streaming.StageDiagramRendering, "✓ Diagram rendered successfully", 95, map[string]interface{}{
			"diagram_url": diagramURL,
		})
	}

	response.DiagramURL = diagramURL
	o.logger.Info("Diagram rendering completed",
		zap.String("diagram_url", diagramURL),
		zap.Bool("fallback_used", fallbackText != ""))
}

// needsFreshness checks if the query needs fresh web search results
func (o *Orchestrator) needsFreshness(query string) bool {
	queryLower := strings.ToLower(query)
	for _, keyword := range o.config.WebSearch.FreshnessKeywords {
		if strings.Contains(queryLower, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

// convertRetrieveChunksToContextItems converts retrieve chunks to synthesis context items
func (o *Orchestrator) convertRetrieveChunksToContextItems(chunks []RetrieveChunk) []synth.ContextItem {
	contextItems := make([]synth.ContextItem, len(chunks))
	for i, chunk := range chunks {
		sourceID := chunk.SourceID
		if sourceID == "" {
			sourceID = chunk.DocID
		}

		contextItems[i] = synth.ContextItem{
			Content:  chunk.Text,
			SourceID: sourceID,
			Score:    chunk.Score,
			Priority: 1,
		}
	}
	return contextItems
}

// createSynthesizeRequest creates a synthesis request from context and web results
func (o *Orchestrator) createSynthesizeRequest(
	query string,
	contextItems []synth.ContextItem,
	webResults []string,
	conversationHistory []session.Message,
) SynthesizeRequest {
	chunks := make([]SynthesizeChunkItem, len(contextItems))
	for i, item := range contextItems {
		chunks[i] = SynthesizeChunkItem{
			Text:     item.Content,
			DocID:    item.SourceID,
			SourceID: item.SourceID,
		}
	}

	webResultItems := make([]SynthesizeWebResult, len(webResults))
	for i, result := range webResults {
		lines := strings.Split(result, "\n")
		var title, snippet, url string
		for _, line := range lines {
			switch {
			case strings.HasPrefix(line, "Title: "):
				title = strings.TrimPrefix(line, "Title: ")
			case strings.HasPrefix(line, "Snippet: "):
				snippet = strings.TrimPrefix(line, "Snippet: ")
			case strings.HasPrefix(line, "URL: "):
				url = strings.TrimPrefix(line, "URL: ")
			}
		}
		webResultItems[i] = SynthesizeWebResult{
			Title:   title,
			Snippet: snippet,
			URL:     url,
		}
	}

	return SynthesizeRequest{
		Query:               query,
		Chunks:              chunks,
		WebResults:          webResultItems,
		ConversationHistory: conversationHistory,
	}
}

// RetrieveResponse represents the response from the retrieve service
type RetrieveResponse struct {
	Chunks            []RetrieveChunk `json:"chunks"`
	Count             int             `json:"count"`
	Query             string          `json:"query"`
	FallbackTriggered bool            `json:"fallback_triggered"`
	FallbackReason    string          `json:"fallback_reason,omitempty"`
}

// RetrieveChunk represents a chunk from the retrieve service
type RetrieveChunk struct {
	Text     string                 `json:"text"`
	Score    float64                `json:"score"`
	DocID    string                 `json:"doc_id"`
	SourceID string                 `json:"source_id"`
	Metadata map[string]interface{} `json:"metadata"`
}

// SynthesizeChunkItem represents a chunk item for synthesis request
type SynthesizeChunkItem struct {
	Text     string `json:"text"`
	DocID    string `json:"doc_id"`
	SourceID string `json:"source_id"`
}

// SynthesizeWebResult represents a web result for synthesis request
type SynthesizeWebResult struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	URL     string `json:"url"`
}

// SynthesizeRequest represents a request to the synthesis service
type SynthesizeRequest struct {
	Query               string                `json:"query"`
	Chunks              []SynthesizeChunkItem `json:"chunks"`
	WebResults          []SynthesizeWebResult `json:"web_results"`
	ConversationHistory []session.Message     `json:"conversation_history,omitempty"`
}

// handleSessionManagement manages session creation and conversation history retrieval
func (o *Orchestrator) handleSessionManagement(
	ctx context.Context,
	userID, query string,
) (string, []session.Message, error) {
	if o.sessionManager == nil {
		return "", []session.Message{}, fmt.Errorf("session manager not initialized")
	}

	// Try to get or create a session for the user
	userSessions, err := o.sessionManager.ListUserSessions(ctx, userID)
	if err != nil {
		return "", []session.Message{}, fmt.Errorf("failed to list user sessions: %w", err)
	}

	var currentSession *session.Session

	// Find the most recent active session
	for _, sess := range userSessions {
		if sess.Status == session.SessionActive {
			// Use the most recently updated active session
			if currentSession == nil || sess.UpdatedAt.After(currentSession.UpdatedAt) {
				currentSession = sess
			}
		}
	}

	// If no active session found, create a new one
	if currentSession == nil {
		newSession, err := o.sessionManager.CreateSession(ctx, userID)
		if err != nil {
			return "", []session.Message{}, fmt.Errorf("failed to create session: %w", err)
		}
		currentSession = newSession
	}

	// Add the user query to the session
	metadata := map[string]interface{}{
		"timestamp": ctx.Value("timestamp"),
		"source":    "teams",
	}

	if err := o.sessionManager.AddMessage(ctx, currentSession.ID, session.UserRole, query, metadata); err != nil {
		o.logger.Warn("Failed to add user message to session", zap.Error(err))
	}

	// Get conversation history (limited by config)
	maxHistory := o.config.Session.MaxHistoryLength
	if maxHistory <= 0 {
		maxHistory = 10 // Default fallback
	}

	history, err := o.sessionManager.GetConversationHistory(ctx, currentSession.ID, maxHistory)
	if err != nil {
		o.logger.Warn("Failed to get conversation history", zap.Error(err))
		history = []session.Message{}
	}

	return currentSession.ID, history, nil
}

// storeResponseInSession stores the assistant's response in the session
func (o *Orchestrator) storeResponseInSession(
	ctx context.Context,
	sessionID, _ string,
	response *synth.SynthesisResponse,
) error {
	if o.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}

	// Format the response content for storage
	responseContent := response.MainText

	// Include diagram information if present
	if response.DiagramURL != "" {
		responseContent += fmt.Sprintf("\n\n[Diagram: %s]", response.DiagramURL)
	}

	// Include code snippets if present
	if len(response.CodeSnippets) > 0 {
		responseContent += "\n\nCode snippets included:"
		for _, snippet := range response.CodeSnippets {
			responseContent += fmt.Sprintf("\n- %s code snippet", snippet.Language)
		}
	}

	// Add metadata about the response
	metadata := map[string]interface{}{
		"has_diagram":   response.DiagramCode != "",
		"has_code":      len(response.CodeSnippets) > 0,
		"source_count":  len(response.Sources),
		"timestamp":     ctx.Value("timestamp"),
		"response_type": "synthesis",
	}

	// Store the assistant response
	return o.sessionManager.AddMessage(ctx, sessionID, session.AssistantRole, responseContent, metadata)
}

// ProcessRegenerationQuery processes a regeneration request with specific parameters
func (o *Orchestrator) ProcessRegenerationQuery(ctx context.Context, query, preset, previousResponse string) (*OrchestrationResult, error) {
	startTime := time.Now()
	o.logger.Info("Processing regeneration query",
		zap.String("query", query),
		zap.String("preset", preset),
		zap.Bool("has_previous_response", previousResponse != ""))

	// Create a specialized orchestration result for regeneration
	result := &OrchestrationResult{
		Query:      query,
		PresetUsed: preset,
		Metrics: OrchestrationMetrics{
			StartTime: startTime,
		},
		Errors: []string{},
	}

	// Step 1: Retrieve context (same as normal query)
	o.logger.Debug("Retrieving context for regeneration")
	retrieveResp, err := o.callRetrieveServiceWithFallback(ctx, query, result)
	if err != nil {
		o.logger.Error("Failed to retrieve context for regeneration", zap.Error(err))
		result.Errors = append(result.Errors, fmt.Sprintf("Context retrieval failed: %v", err))
		return result, err
	}

	// Step 2: Web search (if applicable)
	webResults := []string{}
	needsWeb := o.needsFreshness(query)
	if needsWeb {
		o.logger.Debug("Performing web search for regeneration", zap.Bool("freshness_detected", true))
		webResults = o.callWebSearchServiceWithFallback(ctx, query, result)
	}

	// Step 3: Call regeneration endpoint with custom parameters
	o.logger.Debug("Calling regeneration service", zap.String("preset", preset))
	regenerationResp, err := o.callRegenerationService(ctx, query, preset, previousResponse, retrieveResp, webResults, result)
	if err != nil {
		o.logger.Error("Failed to call regeneration service", zap.Error(err))
		result.Errors = append(result.Errors, fmt.Sprintf("Regeneration failed: %v", err))
		return result, err
	}

	// Step 4: Parse and validate response
	if regenerationResp == nil {
		err = fmt.Errorf("regeneration service returned empty response")
		result.Errors = append(result.Errors, err.Error())
		return result, err
	}

	result.Response = regenerationResp
	result.ProcessingTime = time.Since(result.Metrics.StartTime)
	result.Metrics.EndTime = time.Now()
	result.ExecutionTimeMs = result.ProcessingTime.Milliseconds()

	o.logger.Info("Regeneration query processing completed",
		zap.String("preset", preset),
		zap.Duration("processing_time", result.ProcessingTime),
		zap.Int("response_length", len(regenerationResp.MainText)),
		zap.Bool("has_diagram", regenerationResp.DiagramCode != ""),
		zap.Int("sources_count", len(regenerationResp.Sources)))

	return result, nil
}

// callRegenerationService calls the synthesis service with regeneration parameters
func (o *Orchestrator) callRegenerationService(_ context.Context, query, preset, previousResponse string, retrieveResp *RetrieveResponse, webResults []string, result *OrchestrationResult) (*synth.SynthesisResponse, error) {
	// Build regeneration request
	regenRequest := map[string]interface{}{
		"query":                query,
		"previous_response":    previousResponse,
		"chunks":               []map[string]interface{}{},
		"web_results":          []map[string]interface{}{},
		"conversation_history": []map[string]interface{}{},
		"parameters": map[string]interface{}{
			"preset": preset,
		},
	}

	// Add chunks from retrieve response
	if retrieveResp != nil {
		for _, chunk := range retrieveResp.Chunks {
			regenRequest["chunks"] = append(regenRequest["chunks"].([]map[string]interface{}), map[string]interface{}{
				"text":      chunk.Text,
				"doc_id":    chunk.DocID,
				"source_id": chunk.SourceID,
			})
		}
	}

	// Add web results (simple string format)
	for i, webResult := range webResults {
		regenRequest["web_results"] = append(regenRequest["web_results"].([]map[string]interface{}), map[string]interface{}{
			"title":   fmt.Sprintf("Web Result %d", i+1),
			"snippet": webResult,
			"url":     "",
		})
	}

	// Make HTTP request to regeneration endpoint
	reqBody, err := json.Marshal(regenRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal regeneration request: %w", err)
	}

	regenerateURL := fmt.Sprintf("%s/regenerate", strings.TrimSuffix(o.config.Services.SynthesizeURL, "/"))

	// Validate URL before making request to prevent SSRF
	if !strings.HasPrefix(regenerateURL, "http://") && !strings.HasPrefix(regenerateURL, "https://") {
		return nil, fmt.Errorf("invalid URL scheme for regeneration service: %s", regenerateURL)
	}

	startTime := time.Now()

	// Create HTTP client with timeout for security
	client := &http.Client{
		Timeout: DefaultHTTPTimeout,
	}
	resp, err := client.Post(regenerateURL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to call regeneration service: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			o.logger.Debug("Failed to close response body", zap.Error(closeErr))
		}
	}()

	// Record metrics
	result.Metrics.SynthesisTime = time.Since(startTime)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("regeneration service returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var regenResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&regenResponse); err != nil {
		return nil, fmt.Errorf("failed to parse regeneration response: %w", err)
	}

	// Convert to SynthesisResponse
	mainText, _ := regenResponse["main_text"].(string)
	diagramCode, _ := regenResponse["diagram_code"].(string)
	sources := []string{}
	if sourceList, ok := regenResponse["sources"].([]interface{}); ok {
		for _, source := range sourceList {
			if sourceStr, ok := source.(string); ok {
				sources = append(sources, sourceStr)
			}
		}
	}

	codeSnippets := []synth.CodeSnippet{}
	if snippetList, ok := regenResponse["code_snippets"].([]interface{}); ok {
		for _, snippet := range snippetList {
			if snippetMap, ok := snippet.(map[string]interface{}); ok {
				codeSnippets = append(codeSnippets, synth.CodeSnippet{
					Language: snippetMap["language"].(string),
					Code:     snippetMap["code"].(string),
				})
			}
		}
	}

	return &synth.SynthesisResponse{
		MainText:     mainText,
		DiagramCode:  diagramCode,
		CodeSnippets: codeSnippets,
		Sources:      sources,
	}, nil
}
