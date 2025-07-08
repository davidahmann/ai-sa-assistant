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
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "service": "retrieve"})
	})

	// Main search endpoint
	router.POST("/search", func(c *gin.Context) {
		// TODO: Implement search logic
		// 1. Parse request (query, filters)
		// 2. Query metadata store if filters present
		// 3. Embed query using OpenAI
		// 4. Search ChromaDB
		// 5. Apply fallback logic if needed
		// 6. Return results

		c.JSON(http.StatusOK, gin.H{
			"message": "Search endpoint not yet implemented",
			"query":   c.Request.Header.Get("query"),
		})
	})

	logger.Info("Starting retrieve service", zap.String("chroma_url", cfg.Chroma.URL))
	if err := router.Run(":8080"); err != nil {
		logger.Fatal("Failed to start server", zap.Error(err))
	}
}
