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
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "service": "teamsbot"})
	})

	// Teams webhook endpoint
	router.POST("/teams-webhook", func(c *gin.Context) {
		// TODO: Implement Teams webhook logic
		// 1. Parse Teams message
		// 2. Extract user query
		// 3. Call retrieve service
		// 4. Conditionally call websearch service
		// 5. Call synthesize service
		// 6. Generate diagram image if needed
		// 7. Create Adaptive Card
		// 8. Post to Teams webhook

		c.JSON(http.StatusOK, gin.H{
			"message":     "Teams webhook not yet implemented",
			"webhook_url": cfg.Teams.WebhookURL,
		})
	})

	// Feedback endpoint
	router.POST("/teams-feedback", func(c *gin.Context) {
		// TODO: Implement feedback logging
		// 1. Parse feedback payload
		// 2. Log to file or database
		// 3. Return success response

		c.JSON(http.StatusOK, gin.H{
			"message": "Feedback received",
		})
	})

	logger.Info("Starting teamsbot service",
		zap.String("retrieve_url", cfg.Services.RetrieveURL),
		zap.String("synthesize_url", cfg.Services.SynthesizeURL),
		zap.String("websearch_url", cfg.Services.WebSearchURL))

	if err := router.Run(":8080"); err != nil {
		logger.Fatal("Failed to start server", zap.Error(err))
	}
}
