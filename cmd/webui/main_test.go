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

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/diagram"
	"github.com/your-org/ai-sa-assistant/internal/health"
	"github.com/your-org/ai-sa-assistant/internal/session"
	"github.com/your-org/ai-sa-assistant/internal/teams"
	"go.uber.org/zap"
)

func setupTestServer() *WebUIServer {
	logger := zap.NewNop()

	cfg := &config.Config{}

	healthManager := health.NewManager("webui-test", "1.0.0", logger)

	// Create minimal diagram config for testing
	diagramConfig := diagram.RendererConfig{}
	diagramRenderer := diagram.NewRenderer(diagramConfig, logger)

	// Create session manager
	sessionConfig := session.Config{
		StorageType:     session.MemoryStorageType,
		DefaultTTL:      30 * time.Minute,
		MaxSessions:     1000,
		CleanupInterval: 0,
	}
	sessionManager, err := session.NewManager(sessionConfig, logger)
	if err != nil {
		panic(err) // Should not happen in test
	}

	orchestrator := teams.NewOrchestrator(cfg, healthManager, diagramRenderer, sessionManager, logger)

	return &WebUIServer{
		config:        cfg,
		logger:        logger,
		orchestrator:  orchestrator,
		conversations: make(map[string]*Conversation),
		healthManager: healthManager,
	}
}

func TestHandleHealth(t *testing.T) {
	server := setupTestServer()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/health", server.handleHealth)

	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response, "status")
}

func TestHandleHomePage(t *testing.T) {
	server := setupTestServer()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.LoadHTMLGlob("templates/*")
	router.GET("/", server.handleHomePage)

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Note: This will return 200 in test because we're not loading actual templates
	// In a real test environment, we would mock the template rendering
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleCreateConversation(t *testing.T) {
	server := setupTestServer()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST("/conversations", server.handleCreateConversation)

	req, _ := http.NewRequest("POST", "/conversations", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var conversation Conversation
	err := json.Unmarshal(w.Body.Bytes(), &conversation)
	assert.NoError(t, err)
	assert.NotEmpty(t, conversation.ID)
	assert.Equal(t, "New Conversation", conversation.Title)
	assert.Equal(t, 0, conversation.MessageCount)
}

func TestHandleGetConversations(t *testing.T) {
	server := setupTestServer()
	gin.SetMode(gin.TestMode)

	// Create a test conversation
	conversation := server.createNewConversation()

	router := gin.New()
	router.GET("/conversations", server.handleGetConversations)

	req, _ := http.NewRequest("GET", "/conversations", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var conversations []*Conversation
	err := json.Unmarshal(w.Body.Bytes(), &conversations)
	assert.NoError(t, err)
	assert.Len(t, conversations, 1)
	assert.Equal(t, conversation.ID, conversations[0].ID)
}

func TestHandleGetConversation(t *testing.T) {
	server := setupTestServer()
	gin.SetMode(gin.TestMode)

	// Create a test conversation
	conversation := server.createNewConversation()

	router := gin.New()
	router.GET("/conversations/:id", server.handleGetConversation)

	// Test existing conversation
	req, _ := http.NewRequest("GET", "/conversations/"+conversation.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result Conversation
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, conversation.ID, result.ID)

	// Test non-existing conversation
	req, _ = http.NewRequest("GET", "/conversations/nonexistent", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleDeleteConversation(t *testing.T) {
	server := setupTestServer()
	gin.SetMode(gin.TestMode)

	// Create a test conversation
	conversation := server.createNewConversation()

	router := gin.New()
	router.DELETE("/conversations/:id", server.handleDeleteConversation)

	// Test deleting existing conversation
	req, _ := http.NewRequest("DELETE", "/conversations/"+conversation.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify conversation is deleted
	_, exists := server.conversations[conversation.ID]
	assert.False(t, exists)

	// Test deleting non-existing conversation
	req, _ = http.NewRequest("DELETE", "/conversations/nonexistent", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleChat(t *testing.T) {
	server := setupTestServer()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST("/chat", server.handleChat)

	// Test valid chat request
	chatReq := ChatRequest{
		Message:        "Hello, test message",
		ConversationID: "",
	}

	reqBody, _ := json.Marshal(chatReq)
	req, _ := http.NewRequest("POST", "/chat", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response ChatResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "assistant", response.Message.Role)
	assert.NotEmpty(t, response.ConversationID)

	// Test invalid request
	req, _ = http.NewRequest("POST", "/chat", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")

	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateNewConversation(t *testing.T) {
	server := setupTestServer()

	conversation := server.createNewConversation()

	assert.NotEmpty(t, conversation.ID)
	assert.Equal(t, "New Conversation", conversation.Title)
	assert.Equal(t, 0, conversation.MessageCount)
	assert.Equal(t, 0, len(conversation.Messages))
	assert.WithinDuration(t, time.Now(), conversation.CreatedAt, time.Second)
	assert.WithinDuration(t, time.Now(), conversation.UpdatedAt, time.Second)

	// Verify conversation is stored
	storedConv, exists := server.conversations[conversation.ID]
	assert.True(t, exists)
	assert.Equal(t, conversation.ID, storedConv.ID)
}

func TestGenerateConversationTitle(t *testing.T) {
	server := setupTestServer()

	// Test short message
	shortTitle := server.generateConversationTitle("Hello")
	assert.Equal(t, "Hello", shortTitle)

	// Test long message
	longMessage := "This is a very long message that should be truncated to fit within the title length limit"
	longTitle := server.generateConversationTitle(longMessage)
	assert.Equal(t, 50, len(longTitle))
	assert.Equal(t, "This is a very long message that should be trun...", longTitle)
}

func TestGetOrCreateConversation(t *testing.T) {
	server := setupTestServer()

	// Test creating new conversation with empty ID
	conv1 := server.getOrCreateConversation("")
	assert.NotEmpty(t, conv1.ID)

	// Test getting existing conversation
	conv2 := server.getOrCreateConversation(conv1.ID)
	assert.Equal(t, conv1.ID, conv2.ID)

	// Test creating new conversation with non-existent ID
	conv3 := server.getOrCreateConversation("nonexistent")
	assert.NotEmpty(t, conv3.ID)
	assert.NotEqual(t, conv1.ID, conv3.ID)
}
