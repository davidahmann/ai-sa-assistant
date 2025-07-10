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

//go:build integration
// +build integration

// Package integration provides integration tests for the conversation memory system.
// These tests verify end-to-end functionality including multi-turn conversations,
// context preservation, and session management across service boundaries.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/your-org/ai-sa-assistant/internal/conversation"
	"github.com/your-org/ai-sa-assistant/internal/session"
	"go.uber.org/zap/zaptest"
)

// TestSetup holds the test infrastructure
type TestSetup struct {
	sessionManager      *session.Manager
	conversationManager *conversation.Manager
	apiHandler          *conversation.APIHandler
	router              *gin.Engine
	server              *httptest.Server
}

// SetupIntegrationTest creates a test environment for integration testing
func SetupIntegrationTest(t *testing.T) *TestSetup {
	logger := zaptest.NewLogger(t)

	// Set test mode to avoid OpenAI API requirements
	_ = os.Setenv("TEST_MODE", "true")
	defer func() { _ = os.Unsetenv("TEST_MODE") }()

	// Create session manager
	sessionConfig := session.Config{
		StorageType:     session.MemoryStorageType,
		DefaultTTL:      30 * time.Minute,
		MaxSessions:     1000,
		CleanupInterval: 0, // Disable cleanup for tests
	}

	sessionManager, err := session.NewManager(sessionConfig, logger)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	// Create conversation manager
	conversationManager := conversation.NewManager(sessionManager, logger)

	// Create API handler
	apiHandler := conversation.NewAPIHandler(conversationManager, logger)

	// Set up Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())

	// Register conversation API routes
	apiHandler.RegisterRoutes(router)

	// Create test server
	server := httptest.NewServer(router)

	return &TestSetup{
		sessionManager:      sessionManager,
		conversationManager: conversationManager,
		apiHandler:          apiHandler,
		router:              router,
		server:              server,
	}
}

// Cleanup cleans up test resources
func (ts *TestSetup) Cleanup() {
	ts.server.Close()
	_ = ts.sessionManager.Close()
}

func TestMultiTurnConversationFlow(t *testing.T) {
	setup := SetupIntegrationTest(t)
	defer setup.Cleanup()

	userID := "integration_test_user"
	baseURL := setup.server.URL

	// Step 1: Create a new conversation
	createReq := map[string]interface{}{
		"user_id": userID,
	}
	createBody, _ := json.Marshal(createReq)

	resp, err := http.Post(baseURL+"/api/v1/conversations", "application/json", bytes.NewBuffer(createBody))
	if err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.StatusCode)
	}

	var conversationMeta struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&conversationMeta); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	conversationID := conversationMeta.ID

	// Step 2: Add first user message
	ctx := context.Background()
	firstMessage := "I need help designing a cloud architecture for a microservices application"
	err = setup.conversationManager.AddMessageToConversation(ctx, conversationID, session.UserRole, firstMessage, nil)
	if err != nil {
		t.Fatalf("failed to add first message: %v", err)
	}

	// Step 3: Verify conversation title was auto-generated
	conversationResp, err := http.Get(baseURL + "/api/v1/conversations/" + conversationID)
	if err != nil {
		t.Fatalf("failed to get conversation: %v", err)
	}
	defer func() { _ = conversationResp.Body.Close() }()

	var updatedConversation struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(conversationResp.Body).Decode(&updatedConversation); err != nil {
		t.Fatalf("failed to decode conversation: %v", err)
	}

	// Title might be truncated due to length limits
	if !strings.Contains(updatedConversation.Title, "cloud architecture") {
		t.Errorf("title should contain 'cloud architecture', got %s", updatedConversation.Title)
	}

	// Step 4: Add assistant response
	assistantMessage := "I'd be happy to help! Let's start by understanding your requirements. What technologies are you currently using?"
	err = setup.conversationManager.AddMessageToConversation(ctx, conversationID, session.AssistantRole, assistantMessage, nil)
	if err != nil {
		t.Fatalf("failed to add assistant message: %v", err)
	}

	// Step 5: Add follow-up user message
	followupMessage := "We're using Docker containers and want to deploy on AWS. The application has 5 microservices."
	err = setup.conversationManager.AddMessageToConversation(ctx, conversationID, session.UserRole, followupMessage, nil)
	if err != nil {
		t.Fatalf("failed to add followup message: %v", err)
	}

	// Step 6: Get conversation history to verify context preservation
	historyResp, err := http.Get(baseURL + "/api/v1/conversations/" + conversationID + "/history?max_messages=10")
	if err != nil {
		t.Fatalf("failed to get conversation history: %v", err)
	}
	defer func() { _ = historyResp.Body.Close() }()

	var historyResponse struct {
		ConversationID string            `json:"conversation_id"`
		Messages       []session.Message `json:"messages"`
		Count          int               `json:"count"`
	}
	if err := json.NewDecoder(historyResp.Body).Decode(&historyResponse); err != nil {
		t.Fatalf("failed to decode history response: %v", err)
	}

	// Verify all messages are preserved
	if len(historyResponse.Messages) != 3 {
		t.Errorf("expected 3 messages in history, got %d", len(historyResponse.Messages))
	}

	// Verify message order and content
	expectedMessages := []struct {
		role    session.MessageRole
		content string
	}{
		{session.UserRole, firstMessage},
		{session.AssistantRole, assistantMessage},
		{session.UserRole, followupMessage},
	}

	for i, expected := range expectedMessages {
		if i >= len(historyResponse.Messages) {
			t.Errorf("missing message at index %d", i)
			continue
		}

		msg := historyResponse.Messages[i]
		if msg.Role != expected.role {
			t.Errorf("message %d: expected role %s, got %s", i, expected.role, msg.Role)
		}
		if msg.Content != expected.content {
			t.Errorf("message %d: expected content %s, got %s", i, expected.content, msg.Content)
		}
	}

	// Step 7: Test conversation statistics
	statsResp, err := http.Get(baseURL + "/api/v1/conversations/stats?user_id=" + userID)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}
	defer func() { _ = statsResp.Body.Close() }()

	var stats map[string]interface{}
	if err := json.NewDecoder(statsResp.Body).Decode(&stats); err != nil {
		t.Fatalf("failed to decode stats: %v", err)
	}

	// Verify stats reflect the conversation
	if totalConversations, ok := stats["total_conversations"].(float64); !ok || int(totalConversations) != 1 {
		t.Errorf("expected 1 total conversation, got %v", stats["total_conversations"])
	}

	if totalMessages, ok := stats["total_messages"].(float64); !ok || int(totalMessages) != 3 {
		t.Errorf("expected 3 total messages, got %v", stats["total_messages"])
	}

	// Step 8: Test conversation search
	searchResp, err := http.Get(baseURL + "/api/v1/conversations/search?user_id=" + userID + "&q=microservices&limit=10")
	if err != nil {
		t.Fatalf("failed to search conversations: %v", err)
	}
	defer func() { _ = searchResp.Body.Close() }()

	var searchResults struct {
		Query         string                             `json:"query"`
		Conversations []conversation.ConversationSummary `json:"conversations"`
		Count         int                                `json:"count"`
	}
	if err := json.NewDecoder(searchResp.Body).Decode(&searchResults); err != nil {
		t.Fatalf("failed to decode search results: %v", err)
	}

	// Should find the conversation since it contains "microservices"
	if len(searchResults.Conversations) != 1 {
		t.Errorf("expected 1 search result, got %d", len(searchResults.Conversations))
	}

	if len(searchResults.Conversations) > 0 && searchResults.Conversations[0].Metadata.ID != conversationID {
		t.Errorf("search result should match our conversation ID, expected %s, got %s", conversationID, searchResults.Conversations[0].Metadata.ID)
	}
}

func TestSessionExpiration(t *testing.T) {
	setup := SetupIntegrationTest(t)
	defer setup.Cleanup()

	userID := "expiration_test_user"
	ctx := context.Background()

	// Create a session with very short TTL for testing
	shortTTLConfig := session.Config{
		StorageType:     session.MemoryStorageType,
		DefaultTTL:      100 * time.Millisecond, // Very short for testing
		MaxSessions:     1000,
		CleanupInterval: 0,
	}

	shortSessionManager, err := session.NewManager(shortTTLConfig, zaptest.NewLogger(t))
	if err != nil {
		t.Fatalf("failed to create short TTL session manager: %v", err)
	}
	defer func() { _ = shortSessionManager.Close() }()

	// Create conversation
	sess, err := shortSessionManager.CreateSession(ctx, userID)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Add a message
	err = shortSessionManager.AddMessage(ctx, sess.ID, session.UserRole, "Test message", nil)
	if err != nil {
		t.Fatalf("failed to add message: %v", err)
	}

	// Verify session is active
	activeSession, err := shortSessionManager.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("failed to get active session: %v", err)
	}

	if activeSession.Status != session.SessionActive {
		t.Errorf("expected active session, got %s", activeSession.Status)
	}

	// Wait for expiration
	time.Sleep(200 * time.Millisecond)

	// Verify session is expired
	expiredSession, err := shortSessionManager.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("failed to get expired session: %v", err)
	}

	if expiredSession.Status != session.SessionExpired {
		t.Errorf("expected expired session, got %s", expiredSession.Status)
	}

	// Verify cannot add messages to expired session
	err = shortSessionManager.AddMessage(ctx, sess.ID, session.UserRole, "Another message", nil)
	if err == nil {
		t.Errorf("expected error when adding message to expired session")
	}
}

func TestConcurrentConversations(t *testing.T) {
	setup := SetupIntegrationTest(t)
	defer setup.Cleanup()

	ctx := context.Background()
	numUsers := 5
	messagesPerUser := 3

	// Create conversations for multiple users concurrently
	userChannels := make([]chan error, numUsers)
	for i := 0; i < numUsers; i++ {
		userChannels[i] = make(chan error, 1)
		userID := fmt.Sprintf("concurrent_user_%d", i)

		go func(userID string, ch chan error) {
			defer close(ch)

			// Create conversation
			metadata, err := setup.conversationManager.CreateConversation(ctx, userID)
			if err != nil {
				ch <- fmt.Errorf("failed to create conversation for %s: %w", userID, err)
				return
			}

			// Add multiple messages
			for j := 0; j < messagesPerUser; j++ {
				role := session.UserRole
				content := fmt.Sprintf("Message %d from %s", j+1, userID)
				if j%2 == 1 {
					role = session.AssistantRole
					content = fmt.Sprintf("Response %d to %s", j, userID)
				}

				err = setup.conversationManager.AddMessageToConversation(ctx, metadata.ID, role, content, nil)
				if err != nil {
					ch <- fmt.Errorf("failed to add message for %s: %w", userID, err)
					return
				}

				// Small delay to simulate real usage
				time.Sleep(10 * time.Millisecond)
			}

			ch <- nil
		}(userID, userChannels[i])
	}

	// Wait for all goroutines to complete and check for errors
	for i, ch := range userChannels {
		err := <-ch
		if err != nil {
			t.Errorf("user %d failed: %v", i, err)
		}
	}

	// Verify each user has their own isolated conversations
	for i := 0; i < numUsers; i++ {
		userID := fmt.Sprintf("concurrent_user_%d", i)

		conversations, err := setup.conversationManager.ListConversations(ctx, userID, 1, 20)
		if err != nil {
			t.Errorf("failed to list conversations for user %d: %v", i, err)
			continue
		}

		if len(conversations.Conversations) != 1 {
			t.Errorf("user %d should have exactly 1 conversation, got %d", i, len(conversations.Conversations))
			continue
		}

		// Verify message count
		conversation := conversations.Conversations[0]
		if conversation.MessageCount != messagesPerUser {
			t.Errorf("user %d should have %d messages, got %d", i, messagesPerUser, conversation.MessageCount)
		}
	}
}

func TestConversationContextPreservation(t *testing.T) {
	setup := SetupIntegrationTest(t)
	defer setup.Cleanup()

	ctx := context.Background()
	userID := "context_test_user"

	// Create conversation
	metadata, err := setup.conversationManager.CreateConversation(ctx, userID)
	if err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}

	// Simulate a multi-turn conversation about AWS architecture
	conversationFlow := []struct {
		role    session.MessageRole
		content string
	}{
		{session.UserRole, "I need to design an AWS architecture for a web application"},
		{session.AssistantRole, "I can help with that! What's your expected traffic volume?"},
		{session.UserRole, "About 1000 concurrent users"},
		{session.AssistantRole, "For 1000 concurrent users, I recommend using Application Load Balancer with Auto Scaling"},
		{session.UserRole, "What about the database?"},
		{session.AssistantRole, "For the database, consider RDS with Multi-AZ deployment for high availability"},
		{session.UserRole, "How should I handle file storage?"},
		{session.AssistantRole, "Use S3 for static assets and CloudFront CDN for global distribution"},
	}

	// Add all messages to the conversation
	for _, turn := range conversationFlow {
		err = setup.conversationManager.AddMessageToConversation(ctx, metadata.ID, turn.role, turn.content, nil)
		if err != nil {
			t.Fatalf("failed to add message: %v", err)
		}
	}

	// Get conversation history
	history, err := setup.conversationManager.GetConversationHistory(ctx, metadata.ID, 20)
	if err != nil {
		t.Fatalf("failed to get conversation history: %v", err)
	}

	// Verify all context is preserved
	if len(history) != len(conversationFlow) {
		t.Errorf("expected %d messages in history, got %d", len(conversationFlow), len(history))
	}

	// Verify conversation coherence - each message should be in order
	for i, expectedTurn := range conversationFlow {
		if i >= len(history) {
			t.Errorf("missing message at index %d", i)
			continue
		}

		actualMessage := history[i]
		if actualMessage.Role != expectedTurn.role {
			t.Errorf("message %d: expected role %s, got %s", i, expectedTurn.role, actualMessage.Role)
		}
		if actualMessage.Content != expectedTurn.content {
			t.Errorf("message %d: expected content %s, got %s", i, expectedTurn.content, actualMessage.Content)
		}
	}

	// Test context building for LLM prompts
	contextString := session.BuildConversationContext(history)
	if contextString == "" {
		t.Errorf("conversation context should not be empty")
	}

	// Verify context contains key information from the conversation
	expectedKeywords := []string{"AWS", "architecture", "1000 concurrent users", "Application Load Balancer", "RDS", "S3", "CloudFront"}
	for _, keyword := range expectedKeywords {
		if !strings.Contains(contextString, keyword) {
			t.Errorf("conversation context should contain keyword: %s", keyword)
		}
	}
}

func TestConversationTokenManagement(t *testing.T) {
	setup := SetupIntegrationTest(t)
	defer setup.Cleanup()

	ctx := context.Background()
	userID := "token_test_user"

	// Create conversation
	metadata, err := setup.conversationManager.CreateConversation(ctx, userID)
	if err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}

	// Add messages and verify token counting
	messages := []string{
		"Short message", // ~3 tokens
		"This is a longer message with more words to test token counting",                                                                      // ~12 tokens
		"A very detailed and comprehensive explanation that would consume significantly more tokens when processed by a language model system", // ~20+ tokens
	}

	totalExpectedTokens := 0
	for i, content := range messages {
		role := session.UserRole
		if i%2 == 1 {
			role = session.AssistantRole
		}

		err = setup.conversationManager.AddMessageToConversation(ctx, metadata.ID, role, content, nil)
		if err != nil {
			t.Fatalf("failed to add message %d: %v", i, err)
		}

		totalExpectedTokens += session.EstimateTokenCount(content)
	}

	// Get updated conversation metadata
	updatedMetadata, err := setup.conversationManager.GetConversation(ctx, metadata.ID)
	if err != nil {
		t.Fatalf("failed to get updated conversation: %v", err)
	}

	// Verify token count tracking
	if updatedMetadata.TokenCount <= 0 {
		t.Errorf("conversation should have positive token count, got %d", updatedMetadata.TokenCount)
	}

	// Verify token count is reasonable (within expected range)
	if updatedMetadata.TokenCount < totalExpectedTokens-5 || updatedMetadata.TokenCount > totalExpectedTokens+5 {
		t.Errorf("expected token count around %d, got %d", totalExpectedTokens, updatedMetadata.TokenCount)
	}

	// Test message truncation for token limits
	history, err := setup.conversationManager.GetConversationHistory(ctx, metadata.ID, 20)
	if err != nil {
		t.Fatalf("failed to get conversation history: %v", err)
	}

	// Test truncating messages to fit token limit
	truncated := session.TruncateMessages(history, 20) // Small limit
	if len(truncated) >= len(history) {
		t.Errorf("truncated messages should be fewer than original")
	}

	// Verify truncated messages are the most recent ones
	if len(truncated) > 0 {
		lastTruncated := truncated[len(truncated)-1]
		lastOriginal := history[len(history)-1]
		if lastTruncated.ID != lastOriginal.ID {
			t.Errorf("truncated messages should keep the most recent messages")
		}
	}
}
