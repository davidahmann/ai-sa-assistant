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
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/conversation"
	"github.com/your-org/ai-sa-assistant/internal/diagram"
	"github.com/your-org/ai-sa-assistant/internal/health"
	"github.com/your-org/ai-sa-assistant/internal/session"
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
	config              *config.Config
	logger              *zap.Logger
	orchestrator        *teams.Orchestrator
	sessionManager      *session.Manager
	conversationManager *conversation.Manager
	healthManager       *health.Manager
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

	// Initialize session manager for conversation memory
	sessionConfig := session.Config{
		StorageType:     session.StorageType(cfg.Session.StorageType),
		DefaultTTL:      time.Duration(cfg.Session.DefaultTTL) * time.Minute,
		MaxSessions:     cfg.Session.MaxSessions,
		CleanupInterval: time.Duration(cfg.Session.CleanupInterval) * time.Minute,
	}
	sessionManager, err := session.NewManager(sessionConfig, logger)
	if err != nil {
		logger.Fatal("Failed to initialize session manager", zap.Error(err))
	}

	// Initialize conversation manager
	conversationManager := conversation.NewManager(sessionManager, logger)

	// Initialize orchestrator (reuse Teams Bot orchestration logic)
	orchestrator := teams.NewOrchestrator(cfg, healthManager, diagramRenderer, sessionManager, logger)

	// Create web UI server
	server := &WebUIServer{
		config:              cfg,
		logger:              logger,
		orchestrator:        orchestrator,
		sessionManager:      sessionManager,
		conversationManager: conversationManager,
		healthManager:       healthManager,
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
	router.PUT("/conversations/:id", server.handleUpdateConversation)
	router.DELETE("/conversations/:id", server.handleDeleteConversation)
	router.GET("/conversations/:id/export", server.handleExportConversation)
	router.POST("/conversations/import", server.handleImportConversation)

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
	ctx := c.Request.Context()
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ChatResponse{
			Error: "Invalid request format",
		})
		return
	}

	// Get or create session
	sess, err := s.getOrCreateSession(ctx, req.ConversationID)
	if err != nil {
		s.logger.Error("Failed to get or create session", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ChatResponse{
			Error: "Failed to create conversation session",
		})
		return
	}

	// Add user message to session
	if err := s.sessionManager.AddMessage(ctx, sess.ID, session.UserRole, req.Message, nil); err != nil {
		s.logger.Error("Failed to add user message", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ChatResponse{
			Error: "Failed to save message",
		})
		return
	}

	// Process message through orchestrator (which handles session management internally)
	result := s.orchestrator.ProcessQuery(ctx, req.Message, sess.UserID)
	if result.Error != nil {
		s.logger.Error("Failed to process query through orchestrator", zap.Error(result.Error))
		c.JSON(http.StatusInternalServerError, ChatResponse{
			Error: "Failed to generate response",
		})
		return
	}

	if result.Response == nil {
		s.logger.Error("Orchestrator returned nil response")
		c.JSON(http.StatusInternalServerError, ChatResponse{
			Error: "Failed to generate response",
		})
		return
	}

	// Convert response to chat message format
	assistantMessage := ChatMessage{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Role:      "assistant",
		Content:   result.Response.MainText,
		Timestamp: time.Now(),
	}

	c.JSON(http.StatusOK, ChatResponse{
		Message:        assistantMessage,
		ConversationID: sess.ID,
	})
}

// handleGetConversations returns all conversations for the current user
func (s *WebUIServer) handleGetConversations(c *gin.Context) {
	ctx := c.Request.Context()
	userID := s.getUserID(c) // Default user for demo

	// Get conversation list from conversation manager
	conversationList, err := s.conversationManager.ListConversations(ctx, userID, 1, 50)
	if err != nil {
		s.logger.Error("Failed to list conversations", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load conversations"})
		return
	}

	// Convert to webui format
	webConversations := make([]Conversation, len(conversationList.Conversations))
	for i, convSummary := range conversationList.Conversations {
		webConversations[i] = Conversation{
			ID:           convSummary.Metadata.ID,
			Title:        convSummary.Metadata.Title,
			Messages:     []ChatMessage{}, // Don't load full messages in list view
			CreatedAt:    convSummary.Metadata.CreatedAt,
			UpdatedAt:    convSummary.Metadata.UpdatedAt,
			MessageCount: convSummary.Metadata.MessageCount,
		}
	}

	c.JSON(http.StatusOK, webConversations)
}

// handleCreateConversation creates a new conversation
func (s *WebUIServer) handleCreateConversation(c *gin.Context) {
	ctx := c.Request.Context()
	userID := s.getUserID(c)

	// Create new session
	sess, err := s.sessionManager.CreateSession(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to create session", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create conversation"})
		return
	}

	// Convert to webui format
	conversation := Conversation{
		ID:           sess.ID,
		Title:        sess.Title,
		Messages:     []ChatMessage{},
		CreatedAt:    sess.CreatedAt,
		UpdatedAt:    sess.UpdatedAt,
		MessageCount: 0,
	}

	c.JSON(http.StatusCreated, conversation)
}

// handleGetConversation returns a specific conversation with full message history
func (s *WebUIServer) handleGetConversation(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")

	// Get session
	sess, err := s.sessionManager.GetSession(ctx, id)
	if err != nil {
		s.logger.Error("Failed to get session", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "Conversation not found"})
		return
	}

	// Convert messages to webui format
	messages := make([]ChatMessage, len(sess.Messages))
	for i, msg := range sess.Messages {
		messages[i] = ChatMessage{
			ID:        msg.ID,
			Role:      string(msg.Role),
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		}
	}

	conversation := Conversation{
		ID:           sess.ID,
		Title:        sess.Title,
		Messages:     messages,
		CreatedAt:    sess.CreatedAt,
		UpdatedAt:    sess.UpdatedAt,
		MessageCount: len(sess.Messages),
	}

	c.JSON(http.StatusOK, conversation)
}

// handleUpdateConversation updates conversation metadata (e.g., title)
func (s *WebUIServer) handleUpdateConversation(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")

	var updateReq struct {
		Title string `json:"title"`
	}
	if err := c.ShouldBindJSON(&updateReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	if err := s.conversationManager.UpdateConversationTitle(ctx, id, updateReq.Title); err != nil {
		s.logger.Error("Failed to update conversation title", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update conversation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Conversation updated"})
}

// handleDeleteConversation deletes a conversation
func (s *WebUIServer) handleDeleteConversation(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")

	if err := s.conversationManager.DeleteConversation(ctx, id); err != nil {
		s.logger.Error("Failed to delete conversation", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete conversation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Conversation deleted"})
}

// handleExportConversation exports a conversation to JSON format
func (s *WebUIServer) handleExportConversation(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")

	// Get session with full history
	sess, err := s.sessionManager.GetSession(ctx, id)
	if err != nil {
		s.logger.Error("Failed to get session for export", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "Conversation not found"})
		return
	}

	// Create export format
	exportData := map[string]interface{}{
		"id":            sess.ID,
		"title":         sess.Title,
		"created_at":    sess.CreatedAt,
		"updated_at":    sess.UpdatedAt,
		"message_count": len(sess.Messages),
		"messages":      sess.Messages,
		"metadata":      sess.Metadata,
		"exported_at":   time.Now(),
		"version":       "1.0",
	}

	// Set filename header
	filename := fmt.Sprintf("conversation_%s_%s.json",
		strings.ReplaceAll(sess.Title, " ", "_"),
		sess.CreatedAt.Format("2006-01-02"))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "application/json")

	c.JSON(http.StatusOK, exportData)
}

// handleImportConversation imports a conversation from JSON format
func (s *WebUIServer) handleImportConversation(c *gin.Context) {
	ctx := c.Request.Context()
	userID := s.getUserID(c)

	var importData map[string]interface{}
	if err := c.ShouldBindJSON(&importData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid import format"})
		return
	}

	// Validate import data
	title, ok := importData["title"].(string)
	if !ok {
		title = "Imported Conversation"
	}

	// Create new session for imported conversation
	sess, err := s.sessionManager.CreateSession(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to create session for import", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to import conversation"})
		return
	}

	// Update session title
	if err := s.conversationManager.UpdateConversationTitle(ctx, sess.ID, title); err != nil {
		s.logger.Warn("Failed to update imported conversation title", zap.Error(err))
	}

	// Import messages if available
	if messages, ok := importData["messages"].([]interface{}); ok {
		for _, msgInterface := range messages {
			if msgMap, ok := msgInterface.(map[string]interface{}); ok {
				if role, ok := msgMap["role"].(string); ok {
					if content, ok := msgMap["content"].(string); ok {
						// Add message to session
						var msgRole session.MessageRole
						switch role {
						case "user":
							msgRole = session.UserRole
						case "assistant":
							msgRole = session.AssistantRole
						default:
							msgRole = session.UserRole
						}
						if err := s.sessionManager.AddMessage(ctx, sess.ID, msgRole, content, nil); err != nil {
							s.logger.Warn("Failed to import message", zap.Error(err))
						}
					}
				}
			}
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":         "Conversation imported successfully",
		"conversation_id": sess.ID,
	})
}

// getOrCreateSession gets an existing session or creates a new one
func (s *WebUIServer) getOrCreateSession(ctx context.Context, id string) (*session.Session, error) {
	if id != "" {
		if sess, err := s.sessionManager.GetSession(ctx, id); err == nil {
			return sess, nil
		}
	}
	return s.sessionManager.CreateSession(ctx, s.getUserID(nil))
}

// getUserID returns a user ID for session management (simplified for demo)
func (s *WebUIServer) getUserID(c *gin.Context) string {
	// In a real application, this would extract user ID from authentication
	// For demo purposes, use a default user
	return "demo-user"
}
