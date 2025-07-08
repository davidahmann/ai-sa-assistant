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
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "service": "synthesize"})
	})

	// Synthesis endpoint
	router.POST("/synthesize", func(c *gin.Context) {
		// TODO: Implement synthesis logic
		// 1. Parse request (query, context chunks, web results)
		// 2. Build comprehensive prompt
		// 3. Call OpenAI Chat Completion API
		// 4. Parse response (main_text, diagram_code, code_snippets)
		// 5. Extract sources
		// 6. Return structured response

		c.JSON(http.StatusOK, gin.H{
			"message":    "Synthesis endpoint not yet implemented",
			"model":      cfg.Synthesis.Model,
			"max_tokens": cfg.Synthesis.MaxTokens,
		})
	})

	logger.Info("Starting synthesize service", zap.String("model", cfg.Synthesis.Model))
	if err := router.Run(":8080"); err != nil {
		logger.Fatal("Failed to start server", zap.Error(err))
	}
}
