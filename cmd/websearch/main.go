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

// Package main provides the web search service for the AI SA Assistant.
// It handles live web searches to supplement internal knowledge with fresh information.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sashabaranov/go-openai"
	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/health"
	openaiPkg "github.com/your-org/ai-sa-assistant/internal/openai"
	"github.com/your-org/ai-sa-assistant/internal/websearch"
	"go.uber.org/zap"
)

const (
	searchCacheTimeout   = 5 * time.Minute
	defaultMaxResults    = 3
	maxQueryLength       = 500
	searchRequestTimeout = 30 * time.Second
	healthCheckTimeout   = 5 * time.Second
	defaultMaxTokens     = 800
	defaultTemperature   = 0.3
	searchPrompt         = `You are a web search assistant. Based on the following query, ` +
		`provide search results with current, relevant information from reliable sources. ` +
		`Focus on official documentation, product announcements, and authoritative technical content.

Query: %s

Return your response as a JSON array with this exact structure:
[
  {
    "title": "Source title",
    "snippet": "Relevant excerpt or summary (2-3 sentences)",
    "url": "https://example.com/source",
    "timestamp": "2025-01-01"
  }
]

Provide up to 3 high-quality, relevant results. Focus on recent information ` +
		`when possible, especially for technology-related queries.`
)

// SearchRequest represents a web search request
type SearchRequest struct {
	Query       string `json:"query" binding:"required"`
	ForceSearch *bool  `json:"force_search,omitempty"`
}

// SearchResult represents a single web search result
type SearchResult struct {
	Title     string `json:"title"`
	Snippet   string `json:"snippet"`
	URL       string `json:"url"`
	Timestamp string `json:"timestamp"`
}

// SearchResponse represents the complete response from a web search operation
type SearchResponse struct {
	Results   []SearchResult `json:"results"`
	Source    string         `json:"source"`
	Timestamp string         `json:"timestamp"`
	Cached    bool           `json:"cached"`
}

// CacheEntry represents a cached search response with expiration
type CacheEntry struct {
	Response  SearchResponse
	ExpiresAt time.Time
}

// WebSearchService provides web search functionality using OpenAI
type WebSearchService struct {
	config          *config.Config
	logger          *zap.Logger
	openaiClient    *openaiPkg.Client
	cache           map[string]*CacheEntry
	cacheMutex      sync.RWMutex
	detectionConfig websearch.DetectionConfig
}

// NewWebSearchService creates a new web search service instance
func NewWebSearchService(cfg *config.Config, logger *zap.Logger) (*WebSearchService, error) {
	client, err := openaiPkg.NewClient(cfg.OpenAI.APIKey, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI client: %w", err)
	}

	// Create detection config from existing freshness keywords
	detectionConfig := websearch.ConfigFromSlice(cfg.WebSearch.FreshnessKeywords)

	return &WebSearchService{
		config:          cfg,
		logger:          logger,
		openaiClient:    client,
		cache:           make(map[string]*CacheEntry),
		cacheMutex:      sync.RWMutex{},
		detectionConfig: detectionConfig,
	}, nil
}

func (s *WebSearchService) detectFreshnessKeywords(query string) bool {
	result := websearch.DetectFreshnessNeeds(query, s.detectionConfig)

	if result.NeedsFreshInfo {
		s.logger.Debug("Freshness needs detected",
			zap.String("query", query),
			zap.Strings("matched_keywords", result.MatchedKeywords),
			zap.Strings("matched_patterns", result.MatchedPatterns),
			zap.Float64("confidence_score", result.ConfidenceScore),
			zap.Strings("detection_reasons", result.DetectionReasons),
		)
	} else {
		s.logger.Debug("No freshness needs detected",
			zap.String("query", query),
			zap.Float64("confidence_score", result.ConfidenceScore),
		)
	}

	return result.NeedsFreshInfo
}

func (s *WebSearchService) getCachedResult(query string) (*SearchResponse, bool) {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	entry, exists := s.cache[query]
	if !exists {
		return nil, false
	}

	if time.Now().After(entry.ExpiresAt) {
		delete(s.cache, query)
		return nil, false
	}

	response := entry.Response
	response.Cached = true
	return &response, true
}

func (s *WebSearchService) setCachedResult(query string, response SearchResponse) {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()

	s.cache[query] = &CacheEntry{
		Response:  response,
		ExpiresAt: time.Now().Add(searchCacheTimeout),
	}
}

func (s *WebSearchService) performSearch(ctx context.Context, query string) (*SearchResponse, error) {
	if len(query) > maxQueryLength {
		query = query[:maxQueryLength]
	}

	prompt := fmt.Sprintf(searchPrompt, query)

	req := openaiPkg.ChatCompletionRequest{
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		MaxTokens:   defaultMaxTokens,
		Temperature: defaultTemperature,
		Model:       "gpt-4o",
	}

	s.logger.Debug("Sending search request to OpenAI",
		zap.String("query", query),
		zap.String("model", req.Model),
	)

	resp, err := s.openaiClient.CreateChatCompletion(ctx, req)
	if err != nil {
		s.logger.Error("Failed to get OpenAI response", zap.Error(err))
		return nil, fmt.Errorf("search API error: %w", err)
	}

	var results []SearchResult
	if err := json.Unmarshal([]byte(resp.Content), &results); err != nil {
		s.logger.Warn("Failed to parse OpenAI response as JSON, treating as text",
			zap.Error(err),
			zap.String("content", resp.Content),
		)

		results = []SearchResult{
			{
				Title:     "Web Search Result",
				Snippet:   resp.Content,
				URL:       "https://openai.com",
				Timestamp: time.Now().Format("2006-01-02"),
			},
		}
	}

	maxResults := s.config.WebSearch.MaxResults
	if maxResults <= 0 {
		maxResults = defaultMaxResults
	}

	if len(results) > maxResults {
		results = results[:maxResults]
	}

	response := SearchResponse{
		Results:   results,
		Source:    "openai-web-search",
		Timestamp: time.Now().Format(time.RFC3339),
		Cached:    false,
	}

	s.logger.Info("Search completed successfully",
		zap.String("query", query),
		zap.Int("results_count", len(results)),
		zap.Int("completion_tokens", resp.Usage.CompletionTokens),
	)

	return &response, nil
}

func (s *WebSearchService) handleSearch(c *gin.Context) {
	var req SearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.logger.Warn("Invalid search request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	if strings.TrimSpace(req.Query) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Query cannot be empty"})
		return
	}

	needsSearch := req.ForceSearch != nil && *req.ForceSearch
	if !needsSearch {
		needsSearch = s.detectFreshnessKeywords(req.Query)
	}

	if !needsSearch {
		s.logger.Debug("No freshness keywords detected, returning empty results",
			zap.String("query", req.Query),
		)
		c.JSON(http.StatusOK, SearchResponse{
			Results:   []SearchResult{},
			Source:    "no-search-needed",
			Timestamp: time.Now().Format(time.RFC3339),
			Cached:    false,
		})
		return
	}

	if cached, found := s.getCachedResult(req.Query); found {
		s.logger.Debug("Returning cached search result",
			zap.String("query", req.Query),
		)
		c.JSON(http.StatusOK, cached)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), searchRequestTimeout)
	defer cancel()

	response, err := s.performSearch(ctx, req.Query)
	if err != nil {
		s.logger.Error("Search failed", zap.Error(err), zap.String("query", req.Query))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Search service temporarily unavailable"})
		return
	}

	s.setCachedResult(req.Query, *response)
	c.JSON(http.StatusOK, response)
}

// setupHealthChecks configures health checks for the websearch service
func (s *WebSearchService) setupHealthChecks(manager *health.Manager) {
	// OpenAI health check
	manager.AddCheckerFunc("openai", func(ctx context.Context) health.CheckResult {
		if s.openaiClient == nil {
			return health.CheckResult{
				Status:    health.StatusUnhealthy,
				Error:     "OpenAI client not initialized",
				Timestamp: time.Now(),
			}
		}

		if _, err := s.openaiClient.EmbedQuery(ctx, "health check"); err != nil {
			return health.CheckResult{
				Status:    health.StatusDegraded,
				Error:     fmt.Sprintf("OpenAI connectivity check failed: %v", err),
				Timestamp: time.Now(),
			}
		}

		return health.CheckResult{
			Status:    health.StatusHealthy,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"endpoint": "openai-api",
			},
		}
	})

	// Web search cache health check
	manager.AddCheckerFunc("cache", func(_ context.Context) health.CheckResult {
		s.cacheMutex.RLock()
		cacheSize := len(s.cache)
		s.cacheMutex.RUnlock()

		return health.CheckResult{
			Status:    health.StatusHealthy,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"cache_size":    cacheSize,
				"cache_timeout": searchCacheTimeout.String(),
			},
		}
	})

	// Freshness detection health check
	manager.AddCheckerFunc("freshness_detector", func(_ context.Context) health.CheckResult {
		keywordCount := len(s.config.WebSearch.FreshnessKeywords)

		return health.CheckResult{
			Status:    health.StatusHealthy,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"keywords_count": keywordCount,
				"max_results":    s.config.WebSearch.MaxResults,
			},
		}
	})

	// Set timeout for health checks
	manager.SetTimeout(healthCheckTimeout)
}

func main() {
	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()

	cfg, err := config.Load("./configs/config.yaml")
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	service, err := NewWebSearchService(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to create web search service", zap.Error(err))
	}

	router := gin.Default()

	// Initialize health check manager
	healthManager := health.NewManager("websearch", "1.0.0", logger)
	service.setupHealthChecks(healthManager)

	router.GET("/health", gin.WrapH(healthManager.HTTPHandler()))
	router.POST("/search", service.handleSearch)

	logger.Info("Starting websearch service",
		zap.Int("max_results", cfg.WebSearch.MaxResults),
		zap.Int("freshness_keywords_count", len(cfg.WebSearch.FreshnessKeywords)),
		zap.Duration("cache_timeout", searchCacheTimeout),
	)

	if err := router.Run(":8083"); err != nil {
		logger.Fatal("Failed to start server", zap.Error(err))
	}
}
