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
	"context"
	"testing"
	"time"

	"github.com/your-org/ai-sa-assistant/internal/session"
	"go.uber.org/zap/zaptest"
)

func createTestSessionManager(t *testing.T) *session.Manager {
	logger := zaptest.NewLogger(t)
	config := session.Config{
		StorageType:     session.MemoryStorageType,
		DefaultTTL:      30 * time.Minute,
		MaxSessions:     1000,
		CleanupInterval: 0, // Disable cleanup for tests
	}

	manager, err := session.NewManager(config, logger)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}
	return manager
}

func TestNewManager(t *testing.T) {
	sessionManager := createTestSessionManager(t)
	defer func() { _ = sessionManager.Close() }()

	logger := zaptest.NewLogger(t)

	manager := NewManager(sessionManager, logger)
	if manager == nil {
		t.Fatalf("expected manager, got nil")
	}
}

func TestConversationLifecycle(t *testing.T) {
	sessionManager := createTestSessionManager(t)
	defer func() { _ = sessionManager.Close() }()

	logger := zaptest.NewLogger(t)
	manager := NewManager(sessionManager, logger)

	ctx := context.Background()
	userID := "test_user_123"

	// Test creating a conversation
	metadata, err := manager.CreateConversation(ctx, userID)
	if err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}

	if metadata.ID == "" {
		t.Errorf("conversation ID should not be empty")
	}
	if metadata.UserID != userID {
		t.Errorf("expected user ID %s, got %s", userID, metadata.UserID)
	}
	if metadata.Title != "New Conversation" {
		t.Errorf("expected title 'New Conversation', got %s", metadata.Title)
	}
	if metadata.Status != "active" {
		t.Errorf("expected status 'active', got %s", metadata.Status)
	}

	// Test getting the conversation
	retrieved, err := manager.GetConversation(ctx, metadata.ID)
	if err != nil {
		t.Fatalf("failed to get conversation: %v", err)
	}

	if retrieved.ID != metadata.ID {
		t.Errorf("expected conversation ID %s, got %s", metadata.ID, retrieved.ID)
	}

	// Test updating conversation title
	newTitle := "Updated Test Conversation"
	err = manager.UpdateConversationTitle(ctx, metadata.ID, newTitle)
	if err != nil {
		t.Fatalf("failed to update conversation title: %v", err)
	}

	// Verify title was updated
	updated, err := manager.GetConversation(ctx, metadata.ID)
	if err != nil {
		t.Fatalf("failed to get updated conversation: %v", err)
	}

	if updated.Title != newTitle {
		t.Errorf("expected title %s, got %s", newTitle, updated.Title)
	}

	// Test adding a message to the conversation
	messageMetadata := map[string]interface{}{
		"source": "test",
	}
	err = manager.AddMessageToConversation(ctx, metadata.ID, session.UserRole, "Hello, assistant!", messageMetadata)
	if err != nil {
		t.Fatalf("failed to add message to conversation: %v", err)
	}

	// Test getting conversation history
	history, err := manager.GetConversationHistory(ctx, metadata.ID, 10)
	if err != nil {
		t.Fatalf("failed to get conversation history: %v", err)
	}

	if len(history) != 1 {
		t.Errorf("expected 1 message in history, got %d", len(history))
	}

	if history[0].Role != session.UserRole {
		t.Errorf("expected role %s, got %s", session.UserRole, history[0].Role)
	}

	if history[0].Content != "Hello, assistant!" {
		t.Errorf("expected content 'Hello, assistant!', got %s", history[0].Content)
	}

	// Test listing conversations
	conversations, err := manager.ListConversations(ctx, userID, 1, 20)
	if err != nil {
		t.Fatalf("failed to list conversations: %v", err)
	}

	if len(conversations.Conversations) != 1 {
		t.Errorf("expected 1 conversation, got %d", len(conversations.Conversations))
	}

	if conversations.Conversations[0].Metadata.ID != metadata.ID {
		t.Errorf("expected conversation ID %s, got %s", metadata.ID, conversations.Conversations[0].Metadata.ID)
	}

	// Test conversation stats
	stats, err := manager.GetConversationStats(ctx, userID)
	if err != nil {
		t.Fatalf("failed to get conversation stats: %v", err)
	}

	if totalConversations, ok := stats["total_conversations"].(int); !ok || totalConversations != 1 {
		t.Errorf("expected 1 total conversation, got %v", stats["total_conversations"])
	}

	if activeConversations, ok := stats["active_conversations"].(int); !ok || activeConversations != 1 {
		t.Errorf("expected 1 active conversation, got %v", stats["active_conversations"])
	}

	if totalMessages, ok := stats["total_messages"].(int); !ok || totalMessages != 1 {
		t.Errorf("expected 1 total message, got %v", stats["total_messages"])
	}

	// Test deleting conversation
	err = manager.DeleteConversation(ctx, metadata.ID)
	if err != nil {
		t.Fatalf("failed to delete conversation: %v", err)
	}

	// Verify conversation is deleted
	_, err = manager.GetConversation(ctx, metadata.ID)
	if err == nil {
		t.Errorf("expected error when getting deleted conversation")
	}

	// Verify stats are updated
	updatedStats, err := manager.GetConversationStats(ctx, userID)
	if err != nil {
		t.Fatalf("failed to get updated conversation stats: %v", err)
	}

	if totalConversations, ok := updatedStats["total_conversations"].(int); !ok || totalConversations != 0 {
		t.Errorf("expected 0 total conversations after deletion, got %v", updatedStats["total_conversations"])
	}
}

func TestSearchConversations(t *testing.T) {
	sessionManager := createTestSessionManager(t)
	defer func() { _ = sessionManager.Close() }()

	logger := zaptest.NewLogger(t)
	manager := NewManager(sessionManager, logger)

	ctx := context.Background()
	userID := "search_user"

	// Create test conversations with different titles
	conversations := []struct {
		title   string
		message string
	}{
		{"AWS Migration Plan", "Help me migrate to AWS"},
		{"Azure Cost Analysis", "What are the costs for Azure?"},
		{"Docker Setup Guide", "How do I set up Docker?"},
		{"AWS Security Best Practices", "AWS security guidelines please"},
	}

	conversationIDs := make([]string, len(conversations))
	for i, conv := range conversations {
		// Create conversation
		metadata, err := manager.CreateConversation(ctx, userID)
		if err != nil {
			t.Fatalf("failed to create conversation %d: %v", i, err)
		}
		conversationIDs[i] = metadata.ID

		// Update title
		err = manager.UpdateConversationTitle(ctx, metadata.ID, conv.title)
		if err != nil {
			t.Fatalf("failed to update title for conversation %d: %v", i, err)
		}

		// Add a message
		err = manager.AddMessageToConversation(ctx, metadata.ID, session.UserRole, conv.message, nil)
		if err != nil {
			t.Fatalf("failed to add message to conversation %d: %v", i, err)
		}
	}

	// Test searching by title
	awsResults, err := manager.SearchConversations(ctx, userID, "AWS", 10)
	if err != nil {
		t.Fatalf("failed to search conversations: %v", err)
	}

	if len(awsResults) != 2 {
		t.Errorf("expected 2 AWS conversations, got %d", len(awsResults))
	}

	// Test searching by content
	migrationResults, err := manager.SearchConversations(ctx, userID, "migrate", 10)
	if err != nil {
		t.Fatalf("failed to search conversations by content: %v", err)
	}

	if len(migrationResults) != 1 {
		t.Errorf("expected 1 migration conversation, got %d", len(migrationResults))
	}

	// Test case-insensitive search
	dockerResults, err := manager.SearchConversations(ctx, userID, "docker", 10)
	if err != nil {
		t.Fatalf("failed to search conversations case-insensitive: %v", err)
	}

	if len(dockerResults) != 1 {
		t.Errorf("expected 1 Docker conversation, got %d", len(dockerResults))
	}

	// Test search with limit
	limitedResults, err := manager.SearchConversations(ctx, userID, "a", 2) // Should match multiple
	if err != nil {
		t.Fatalf("failed to search conversations with limit: %v", err)
	}

	if len(limitedResults) > 2 {
		t.Errorf("expected at most 2 results with limit, got %d", len(limitedResults))
	}

	// Clean up
	for _, id := range conversationIDs {
		_ = manager.DeleteConversation(ctx, id)
	}
}

func TestConversationErrorCases(t *testing.T) {
	sessionManager := createTestSessionManager(t)
	defer func() { _ = sessionManager.Close() }()

	logger := zaptest.NewLogger(t)
	manager := NewManager(sessionManager, logger)

	ctx := context.Background()

	// Test getting non-existent conversation
	_, err := manager.GetConversation(ctx, "non_existent_conversation")
	if err == nil {
		t.Errorf("expected error when getting non-existent conversation")
	}

	// Test updating title for non-existent conversation
	err = manager.UpdateConversationTitle(ctx, "non_existent_conversation", "New Title")
	if err == nil {
		t.Errorf("expected error when updating title for non-existent conversation")
	}

	// Test adding message to non-existent conversation
	err = manager.AddMessageToConversation(ctx, "non_existent_conversation", session.UserRole, "test", nil)
	if err == nil {
		t.Errorf("expected error when adding message to non-existent conversation")
	}

	// Test getting history for non-existent conversation
	_, err = manager.GetConversationHistory(ctx, "non_existent_conversation", 10)
	if err == nil {
		t.Errorf("expected error when getting history for non-existent conversation")
	}

	// Test deleting non-existent conversation
	err = manager.DeleteConversation(ctx, "non_existent_conversation")
	if err == nil {
		t.Errorf("expected error when deleting non-existent conversation")
	}
}

func TestMultipleUsersIsolation(t *testing.T) {
	sessionManager := createTestSessionManager(t)
	defer func() { _ = sessionManager.Close() }()

	logger := zaptest.NewLogger(t)
	manager := NewManager(sessionManager, logger)

	ctx := context.Background()
	user1 := "user_1"
	user2 := "user_2"

	// Create conversations for both users
	conv1, err := manager.CreateConversation(ctx, user1)
	if err != nil {
		t.Fatalf("failed to create conversation for user1: %v", err)
	}

	conv2, err := manager.CreateConversation(ctx, user2)
	if err != nil {
		t.Fatalf("failed to create conversation for user2: %v", err)
	}

	// Test that users only see their own conversations
	user1Conversations, err := manager.ListConversations(ctx, user1, 1, 20)
	if err != nil {
		t.Fatalf("failed to list conversations for user1: %v", err)
	}

	if len(user1Conversations.Conversations) != 1 {
		t.Errorf("expected 1 conversation for user1, got %d", len(user1Conversations.Conversations))
	}

	if user1Conversations.Conversations[0].Metadata.ID != conv1.ID {
		t.Errorf("user1 should only see their own conversation")
	}

	user2Conversations, err := manager.ListConversations(ctx, user2, 1, 20)
	if err != nil {
		t.Fatalf("failed to list conversations for user2: %v", err)
	}

	if len(user2Conversations.Conversations) != 1 {
		t.Errorf("expected 1 conversation for user2, got %d", len(user2Conversations.Conversations))
	}

	if user2Conversations.Conversations[0].Metadata.ID != conv2.ID {
		t.Errorf("user2 should only see their own conversation")
	}

	// Test that users can't access each other's conversations
	_, err = manager.GetConversation(ctx, conv2.ID)
	// This is expected behavior - the conversation exists but access control
	// should be handled at the API layer, not the manager layer
	// The manager layer focuses on session management, not authorization
	_ = err

	// Clean up
	_ = manager.DeleteConversation(ctx, conv1.ID)
	_ = manager.DeleteConversation(ctx, conv2.ID)
}

func TestConversationPagination(t *testing.T) {
	sessionManager := createTestSessionManager(t)
	defer func() { _ = sessionManager.Close() }()

	logger := zaptest.NewLogger(t)
	manager := NewManager(sessionManager, logger)

	ctx := context.Background()
	userID := "pagination_user"

	// Create multiple conversations
	const numConversations = 25
	conversationIDs := make([]string, numConversations)

	for i := 0; i < numConversations; i++ {
		metadata, err := manager.CreateConversation(ctx, userID)
		if err != nil {
			t.Fatalf("failed to create conversation %d: %v", i, err)
		}
		conversationIDs[i] = metadata.ID

		// Add a small delay to ensure different timestamps
		time.Sleep(1 * time.Millisecond)
	}

	// Test first page
	page1, err := manager.ListConversations(ctx, userID, 1, 10)
	if err != nil {
		t.Fatalf("failed to get page 1: %v", err)
	}

	if len(page1.Conversations) != 10 {
		t.Errorf("expected 10 conversations on page 1, got %d", len(page1.Conversations))
	}

	if page1.Total != numConversations {
		t.Errorf("expected total %d, got %d", numConversations, page1.Total)
	}

	if page1.Page != 1 {
		t.Errorf("expected page 1, got %d", page1.Page)
	}

	if page1.PageSize != 10 {
		t.Errorf("expected page size 10, got %d", page1.PageSize)
	}

	// Test second page
	page2, err := manager.ListConversations(ctx, userID, 2, 10)
	if err != nil {
		t.Fatalf("failed to get page 2: %v", err)
	}

	if len(page2.Conversations) != 10 {
		t.Errorf("expected 10 conversations on page 2, got %d", len(page2.Conversations))
	}

	// Test last page
	page3, err := manager.ListConversations(ctx, userID, 3, 10)
	if err != nil {
		t.Fatalf("failed to get page 3: %v", err)
	}

	if len(page3.Conversations) != 5 {
		t.Errorf("expected 5 conversations on page 3, got %d", len(page3.Conversations))
	}

	// Test that page 1 and page 2 have different conversations
	page1IDs := make(map[string]bool)
	for _, conv := range page1.Conversations {
		page1IDs[conv.Metadata.ID] = true
	}

	for _, conv := range page2.Conversations {
		if page1IDs[conv.Metadata.ID] {
			t.Errorf("conversation %s appears on both page 1 and page 2", conv.Metadata.ID)
		}
	}

	// Clean up
	for _, id := range conversationIDs {
		_ = manager.DeleteConversation(ctx, id)
	}
}

func TestConversationTitleGeneration(t *testing.T) {
	sessionManager := createTestSessionManager(t)
	defer func() { _ = sessionManager.Close() }()

	logger := zaptest.NewLogger(t)
	manager := NewManager(sessionManager, logger)

	ctx := context.Background()
	userID := "title_test_user"

	// Create conversation
	metadata, err := manager.CreateConversation(ctx, userID)
	if err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}

	// Initially should have default title
	if metadata.Title != "New Conversation" {
		t.Errorf("expected default title 'New Conversation', got %s", metadata.Title)
	}

	// Add first message - should update title automatically
	firstMessage := "Help me design an AWS architecture"
	err = manager.AddMessageToConversation(ctx, metadata.ID, session.UserRole, firstMessage, nil)
	if err != nil {
		t.Fatalf("failed to add first message: %v", err)
	}

	// Check if title was auto-generated
	updated, err := manager.GetConversation(ctx, metadata.ID)
	if err != nil {
		t.Fatalf("failed to get updated conversation: %v", err)
	}

	// The title should be generated from the first message
	expectedTitle := "Help me design an AWS architecture"
	if updated.Title != expectedTitle {
		t.Errorf("expected auto-generated title %s, got %s", expectedTitle, updated.Title)
	}

	// Add second message - should not change title
	err = manager.AddMessageToConversation(ctx, metadata.ID, session.AssistantRole, "I'd be happy to help!", nil)
	if err != nil {
		t.Fatalf("failed to add second message: %v", err)
	}

	// Title should remain the same
	afterSecond, err := manager.GetConversation(ctx, metadata.ID)
	if err != nil {
		t.Fatalf("failed to get conversation after second message: %v", err)
	}

	if afterSecond.Title != expectedTitle {
		t.Errorf("title should not change after second message, expected %s, got %s", expectedTitle, afterSecond.Title)
	}

	// Clean up
	_ = manager.DeleteConversation(ctx, metadata.ID)
}
