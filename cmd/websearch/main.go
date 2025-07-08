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
