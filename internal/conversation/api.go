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

package conversation

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/your-org/ai-sa-assistant/internal/session"
	"go.uber.org/zap"
)

// APIHandler handles HTTP requests for conversation management
type APIHandler struct {
	manager *Manager
	logger  *zap.Logger
}

// NewAPIHandler creates a new conversation API handler
func NewAPIHandler(manager *Manager, logger *zap.Logger) *APIHandler {
	return &APIHandler{
		manager: manager,
		logger:  logger,
	}
}

// RegisterRoutes registers conversation API routes with the Gin router
func (h *APIHandler) RegisterRoutes(router *gin.Engine) {
	api := router.Group("/api/v1/conversations")
	{
		api.GET("", h.listConversations)
		api.POST("", h.createConversation)
		api.GET("/:id", h.getConversation)
		api.PUT("/:id/title", h.updateConversationTitle)
		api.DELETE("/:id", h.deleteConversation)
		api.GET("/:id/history", h.getConversationHistory)
		api.GET("/search", h.searchConversations)
		api.GET("/stats", h.getConversationStats)
	}
}

// CreateConversationRequest represents a request to create a new conversation
type CreateConversationRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

// UpdateTitleRequest represents a request to update conversation title
type UpdateTitleRequest struct {
	Title string `json:"title" binding:"required"`
}

// SearchRequest represents a search request
type SearchRequest struct {
	Query  string `form:"q" binding:"required"`
	Limit  int    `form:"limit"`
	UserID string `form:"user_id" binding:"required"`
}

// listConversations handles GET /api/v1/conversations
func (h *APIHandler) listConversations(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id parameter is required"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	conversationList, err := h.manager.ListConversations(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		h.logger.Error("Failed to list conversations", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list conversations"})
		return
	}

	c.JSON(http.StatusOK, conversationList)
}

// createConversation handles POST /api/v1/conversations
func (h *APIHandler) createConversation(c *gin.Context) {
	var req CreateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	if !session.ValidateUserID(req.UserID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID format"})
		return
	}

	metadata, err := h.manager.CreateConversation(c.Request.Context(), req.UserID)
	if err != nil {
		h.logger.Error("Failed to create conversation", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create conversation"})
		return
	}

	c.JSON(http.StatusCreated, metadata)
}

// getConversation handles GET /api/v1/conversations/:id
func (h *APIHandler) getConversation(c *gin.Context) {
	conversationID := c.Param("id")
	if !session.ValidateSessionID(conversationID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid conversation ID format"})
		return
	}

	metadata, err := h.manager.GetConversation(c.Request.Context(), conversationID)
	if err != nil {
		h.logger.Error("Failed to get conversation", zap.String("id", conversationID), zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "Conversation not found"})
		return
	}

	c.JSON(http.StatusOK, metadata)
}

// updateConversationTitle handles PUT /api/v1/conversations/:id/title
func (h *APIHandler) updateConversationTitle(c *gin.Context) {
	conversationID := c.Param("id")
	if !session.ValidateSessionID(conversationID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid conversation ID format"})
		return
	}

	var req UpdateTitleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	// Sanitize title
	title := strings.TrimSpace(req.Title)
	if len(title) == 0 || len(title) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Title must be between 1 and 200 characters"})
		return
	}

	if err := h.manager.UpdateConversationTitle(c.Request.Context(), conversationID, title); err != nil {
		h.logger.Error("Failed to update conversation title",
			zap.String("id", conversationID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update conversation title"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Title updated successfully"})
}

// deleteConversation handles DELETE /api/v1/conversations/:id
func (h *APIHandler) deleteConversation(c *gin.Context) {
	conversationID := c.Param("id")
	if !session.ValidateSessionID(conversationID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid conversation ID format"})
		return
	}

	if err := h.manager.DeleteConversation(c.Request.Context(), conversationID); err != nil {
		h.logger.Error("Failed to delete conversation",
			zap.String("id", conversationID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete conversation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Conversation deleted successfully"})
}

// getConversationHistory handles GET /api/v1/conversations/:id/history
func (h *APIHandler) getConversationHistory(c *gin.Context) {
	conversationID := c.Param("id")
	if !session.ValidateSessionID(conversationID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid conversation ID format"})
		return
	}

	maxMessages, _ := strconv.Atoi(c.DefaultQuery("max_messages", "50"))
	if maxMessages <= 0 || maxMessages > 200 {
		maxMessages = 50
	}

	messages, err := h.manager.GetConversationHistory(c.Request.Context(), conversationID, maxMessages)
	if err != nil {
		h.logger.Error("Failed to get conversation history",
			zap.String("id", conversationID), zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "Conversation not found"})
		return
	}

	response := gin.H{
		"conversation_id": conversationID,
		"messages":        messages,
		"count":           len(messages),
		"max_messages":    maxMessages,
	}

	c.JSON(http.StatusOK, response)
}

// searchConversations handles GET /api/v1/conversations/search
func (h *APIHandler) searchConversations(c *gin.Context) {
	var req SearchRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid query parameters", "details": err.Error()})
		return
	}

	if !session.ValidateUserID(req.UserID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID format"})
		return
	}

	if req.Limit <= 0 || req.Limit > 50 {
		req.Limit = 10
	}

	conversations, err := h.manager.SearchConversations(c.Request.Context(), req.UserID, req.Query, req.Limit)
	if err != nil {
		h.logger.Error("Failed to search conversations", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to search conversations"})
		return
	}

	response := gin.H{
		"query":         req.Query,
		"conversations": conversations,
		"count":         len(conversations),
		"limit":         req.Limit,
	}

	c.JSON(http.StatusOK, response)
}

// getConversationStats handles GET /api/v1/conversations/stats
func (h *APIHandler) getConversationStats(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id parameter is required"})
		return
	}

	if !session.ValidateUserID(userID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID format"})
		return
	}

	stats, err := h.manager.GetConversationStats(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get conversation stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get conversation stats"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// AddMessageRequest represents a request to add a message to a conversation
type AddMessageRequest struct {
	Role     string                 `json:"role" binding:"required"`
	Content  string                 `json:"content" binding:"required"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// addMessage handles POST /api/v1/conversations/:id/messages
func (h *APIHandler) addMessage(c *gin.Context) {
	conversationID := c.Param("id")
	if !session.ValidateSessionID(conversationID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid conversation ID format"})
		return
	}

	var req AddMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	// Validate role
	var role session.MessageRole
	switch strings.ToLower(req.Role) {
	case "user":
		role = session.UserRole
	case "assistant":
		role = session.AssistantRole
	case "system":
		role = session.SystemRole
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role. Must be 'user', 'assistant', or 'system'"})
		return
	}

	// Sanitize content
	content := session.SanitizeUserInput(req.Content)
	if content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Message content cannot be empty"})
		return
	}

	if err := h.manager.AddMessageToConversation(c.Request.Context(), conversationID, role, content, req.Metadata); err != nil {
		h.logger.Error("Failed to add message to conversation",
			zap.String("id", conversationID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add message"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Message added successfully"})
}

// HealthCheckResponse represents the health check response
type HealthCheckResponse struct {
	Status  string                 `json:"status"`
	Service string                 `json:"service"`
	Version string                 `json:"version"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// healthCheck handles GET /api/v1/conversations/health
func (h *APIHandler) healthCheck(c *gin.Context) {
	response := HealthCheckResponse{
		Status:  "healthy",
		Service: "conversation-api",
		Version: "1.0.0",
		Details: map[string]interface{}{
			"timestamp": c.Request.Header.Get("X-Request-Time"),
		},
	}

	c.JSON(http.StatusOK, response)
}

// ErrorResponse represents a standard error response
type ErrorResponse struct {
	Error   string                 `json:"error"`
	Code    string                 `json:"code,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// writeErrorResponse writes a standardized error response
func (h *APIHandler) writeErrorResponse(c *gin.Context, statusCode int, message string, details map[string]interface{}) {
	response := ErrorResponse{
		Error:   message,
		Details: details,
	}

	// Add request ID if available
	if requestID := c.GetHeader("X-Request-ID"); requestID != "" {
		if response.Details == nil {
			response.Details = make(map[string]interface{})
		}
		response.Details["request_id"] = requestID
	}

	c.JSON(statusCode, response)
}

// Middleware for request logging and error handling
func (h *APIHandler) RequestLoggingMiddleware() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		var statusColor, methodColor, resetColor string
		if param.IsOutputColor() {
			statusColor = param.StatusCodeColor()
			methodColor = param.MethodColor()
			resetColor = param.ResetColor()
		}

		return fmt.Sprintf("%s[CONVERSATION-API]%s %v |%s %3d %s| %13v | %15s |%s %-7s %s %#v\n%s",
			methodColor, resetColor,
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			statusColor, param.StatusCode, resetColor,
			param.Latency,
			param.ClientIP,
			methodColor, param.Method, resetColor,
			param.Path,
			param.ErrorMessage,
		)
	})
}

// CORS middleware for conversation API
func (h *APIHandler) CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Header("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
