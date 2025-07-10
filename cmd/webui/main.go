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

// Package main provides the web UI service for the AI SA Assistant.
// It provides a modern chat interface with sidebar navigation as an alternative to Teams integration.
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/diagram"
	"github.com/your-org/ai-sa-assistant/internal/health"
	"github.com/your-org/ai-sa-assistant/internal/teams"
	"go.uber.org/zap"
)

const (
	// DefaultPort is the default port for the web UI service
	DefaultPort = "8080"
	// HealthCheckTimeout is the timeout for health check requests
	HealthCheckTimeout = 10 * time.Second
	// RequestTimeout is the timeout for API requests
	RequestTimeout = 30 * time.Second
)

// ChatMessage represents a chat message in the conversation
type ChatMessage struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"` // "user" or "assistant"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// Conversation represents a conversation with metadata
type Conversation struct {
	ID           string        `json:"id"`
	Title        string        `json:"title"`
	Messages     []ChatMessage `json:"messages"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	MessageCount int           `json:"message_count"`
}

// ChatRequest represents an incoming chat request
type ChatRequest struct {
	Message        string `json:"message" binding:"required"`
	ConversationID string `json:"conversation_id"`
}

// ChatResponse represents a chat response
type ChatResponse struct {
	Message        ChatMessage `json:"message"`
	ConversationID string      `json:"conversation_id"`
	Error          string      `json:"error,omitempty"`
}

// WebUIServer represents the web UI server
type WebUIServer struct {
	config        *config.Config
	logger        *zap.Logger
	orchestrator  *teams.Orchestrator
	conversations map[string]*Conversation
	healthManager *health.Manager
}

func main() {
	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()

	// Check if running in test mode
	testMode := os.Getenv("TEST_MODE") == "true" || os.Getenv("CI") == "true"

	var cfg *config.Config
	var err error

	if testMode {
		cfg, err = config.LoadWithOptions(config.LoadOptions{
			ConfigPath:       "./configs/config.yaml",
			EnableHotReload:  false,
			Environment:      "test",
			ValidateRequired: false,
			TestMode:         true,
		})
	} else {
		cfg, err = config.Load("./configs/config.yaml")
	}

	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	// Initialize health manager
	healthManager := health.NewManager("webui", "1.0.0", logger)

	// Initialize diagram renderer (required for orchestrator)
	diagramConfig := diagram.RendererConfig{
		MermaidInkURL:  cfg.Diagram.MermaidInkURL,
		Timeout:        time.Duration(cfg.Diagram.Timeout) * time.Second,
		CacheExpiry:    time.Duration(cfg.Diagram.CacheExpiry) * time.Hour,
		EnableCaching:  cfg.Diagram.EnableCaching,
		MaxDiagramSize: cfg.Diagram.MaxDiagramSize,
	}
	diagramRenderer := diagram.NewRenderer(diagramConfig, logger)

	// Initialize orchestrator (reuse Teams Bot orchestration logic)
	orchestrator := teams.NewOrchestrator(cfg, healthManager, diagramRenderer, logger)

	// Create web UI server
	server := &WebUIServer{
		config:        cfg,
		logger:        logger,
		orchestrator:  orchestrator,
		conversations: make(map[string]*Conversation),
		healthManager: healthManager,
	}

	// Set up Gin router
	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// Serve static files
	router.Static("/static", "./static")
	router.LoadHTMLGlob("templates/*")

	// Routes
	router.GET("/", server.handleHomePage)
	router.GET("/health", server.handleHealth)
	router.POST("/chat", server.handleChat)
	router.GET("/conversations", server.handleGetConversations)
	router.POST("/conversations", server.handleCreateConversation)
	router.GET("/conversations/:id", server.handleGetConversation)
	router.DELETE("/conversations/:id", server.handleDeleteConversation)

	// Determine port
	port := os.Getenv("PORT")
	if port == "" {
		port = DefaultPort
	}

	logger.Info("Starting Web UI server",
		zap.String("port", port),
		zap.String("service", "webui"),
	)

	if err := router.Run(":" + port); err != nil {
		logger.Fatal("Failed to start server", zap.Error(err))
	}
}

// handleHomePage serves the main chat interface
func (s *WebUIServer) handleHomePage(c *gin.Context) {
	c.HTML(http.StatusOK, "chat.html", gin.H{
		"title": "AI SA Assistant",
	})
}

// handleHealth returns the health status
func (s *WebUIServer) handleHealth(c *gin.Context) {
	// Use the health manager's HTTP handler
	s.healthManager.HTTPHandler().ServeHTTP(c.Writer, c.Request)
}

// handleChat processes a chat message
func (s *WebUIServer) handleChat(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ChatResponse{
			Error: "Invalid request format",
		})
		return
	}

	// Get or create conversation
	conversation := s.getOrCreateConversation(req.ConversationID)

	// Add user message
	userMessage := ChatMessage{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Role:      "user",
		Content:   req.Message,
		Timestamp: time.Now(),
	}
	conversation.Messages = append(conversation.Messages, userMessage)
	conversation.MessageCount++
	conversation.UpdatedAt = time.Now()

	// Generate title if this is the first message
	if conversation.Title == "" {
		conversation.Title = s.generateConversationTitle(req.Message)
	}

	// For now, return a simple response until orchestrator integration is complete
	// TODO: Process message through orchestrator when full integration is implemented
	// TODO: Integrate with actual orchestrator logic
	assistantMessage := ChatMessage{
		ID:   fmt.Sprintf("%d", time.Now().UnixNano()),
		Role: "assistant",
		Content: fmt.Sprintf("I received your message: %s. Full orchestration will be implemented in subsequent issues.",
			req.Message),
		Timestamp: time.Now(),
	}

	conversation.Messages = append(conversation.Messages, assistantMessage)
	conversation.MessageCount++
	conversation.UpdatedAt = time.Now()

	s.conversations[conversation.ID] = conversation

	c.JSON(http.StatusOK, ChatResponse{
		Message:        assistantMessage,
		ConversationID: conversation.ID,
	})
}

// handleGetConversations returns all conversations
func (s *WebUIServer) handleGetConversations(c *gin.Context) {
	conversations := make([]*Conversation, 0, len(s.conversations))
	for _, conv := range s.conversations {
		conversations = append(conversations, conv)
	}
	c.JSON(http.StatusOK, conversations)
}

// handleCreateConversation creates a new conversation
func (s *WebUIServer) handleCreateConversation(c *gin.Context) {
	conversation := s.createNewConversation()
	c.JSON(http.StatusCreated, conversation)
}

// handleGetConversation returns a specific conversation
func (s *WebUIServer) handleGetConversation(c *gin.Context) {
	id := c.Param("id")
	conversation, exists := s.conversations[id]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Conversation not found"})
		return
	}
	c.JSON(http.StatusOK, conversation)
}

// handleDeleteConversation deletes a conversation
func (s *WebUIServer) handleDeleteConversation(c *gin.Context) {
	id := c.Param("id")
	if _, exists := s.conversations[id]; !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Conversation not found"})
		return
	}
	delete(s.conversations, id)
	c.JSON(http.StatusOK, gin.H{"message": "Conversation deleted"})
}

// getOrCreateConversation gets an existing conversation or creates a new one
func (s *WebUIServer) getOrCreateConversation(id string) *Conversation {
	if id != "" {
		if conv, exists := s.conversations[id]; exists {
			return conv
		}
	}
	return s.createNewConversation()
}

// createNewConversation creates a new conversation
func (s *WebUIServer) createNewConversation() *Conversation {
	now := time.Now()
	id := fmt.Sprintf("conv_%d", now.UnixNano())

	conversation := &Conversation{
		ID:           id,
		Title:        "New Conversation",
		Messages:     make([]ChatMessage, 0),
		CreatedAt:    now,
		UpdatedAt:    now,
		MessageCount: 0,
	}

	s.conversations[id] = conversation
	return conversation
}

const maxTitleLength = 50

// generateConversationTitle generates a title from the first message
func (s *WebUIServer) generateConversationTitle(message string) string {
	if len(message) > maxTitleLength {
		return message[:47] + "..."
	}
	return message
}
