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
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/your-org/ai-sa-assistant/internal/config"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()

	cfg, err := config.Load("./configs/config.yaml")
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	router := gin.Default()

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "service": "websearch"})
	})

	// Web search endpoint
	router.POST("/search", func(c *gin.Context) {
		// TODO: Implement web search logic
		// 1. Parse request (query)
		// 2. Detect freshness keywords
		// 3. Call external search API (OpenAI browsing or other)
		// 4. Return structured results

		c.JSON(http.StatusOK, gin.H{
			"message":            "Web search endpoint not yet implemented",
			"freshness_keywords": cfg.WebSearch.FreshnessKeywords,
		})
	})

	logger.Info("Starting websearch service")
	if err := router.Run(":8080"); err != nil {
		logger.Fatal("Failed to start server", zap.Error(err))
	}
}
