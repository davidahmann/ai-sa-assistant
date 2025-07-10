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

package session

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

func TestNewManager(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "valid memory storage config",
			config: Config{
				StorageType:     MemoryStorageType,
				DefaultTTL:      30 * time.Minute,
				MaxSessions:     1000,
				CleanupInterval: 5 * time.Minute,
			},
			expectError: false,
		},
		{
			name: "redis storage config (should fail in demo)",
			config: Config{
				StorageType:     RedisStorageType,
				RedisURL:        "redis://localhost:6379",
				DefaultTTL:      30 * time.Minute,
				MaxSessions:     1000,
				CleanupInterval: 5 * time.Minute,
			},
			expectError: true,
		},
		{
			name: "invalid storage type",
			config: Config{
				StorageType:     "invalid",
				DefaultTTL:      30 * time.Minute,
				MaxSessions:     1000,
				CleanupInterval: 5 * time.Minute,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(tt.config, logger)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if manager == nil {
				t.Errorf("expected manager but got nil")
				return
			}

			// Clean up
			err = manager.Close()
			if err != nil {
				t.Errorf("failed to close manager: %v", err)
			}
		})
	}
}

func TestSessionLifecycle(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := Config{
		StorageType:     MemoryStorageType,
		DefaultTTL:      30 * time.Minute,
		MaxSessions:     1000,
		CleanupInterval: 0, // Disable cleanup for tests
	}

	manager, err := NewManager(config, logger)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	ctx := context.Background()
	userID := "test_user_123"

	// Test creating a session
	session, err := manager.CreateSession(ctx, userID)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if session.ID == "" {
		t.Errorf("session ID should not be empty")
	}
	if session.UserID != userID {
		t.Errorf("expected user ID %s, got %s", userID, session.UserID)
	}
	if session.Status != SessionActive {
		t.Errorf("expected status %s, got %s", SessionActive, session.Status)
	}
	if len(session.Messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(session.Messages))
	}

	// Test retrieving the session
	retrievedSession, err := manager.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	if retrievedSession.ID != session.ID {
		t.Errorf("expected session ID %s, got %s", session.ID, retrievedSession.ID)
	}

	// Test adding a message
	err = manager.AddMessage(ctx, session.ID, UserRole, "Hello, AI assistant!", nil)
	if err != nil {
		t.Fatalf("failed to add message: %v", err)
	}

	// Verify message was added
	updatedSession, err := manager.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to get updated session: %v", err)
	}

	if len(updatedSession.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(updatedSession.Messages))
	}

	if updatedSession.Messages[0].Role != UserRole {
		t.Errorf("expected role %s, got %s", UserRole, updatedSession.Messages[0].Role)
	}

	if updatedSession.Messages[0].Content != "Hello, AI assistant!" {
		t.Errorf("expected content 'Hello, AI assistant!', got %s", updatedSession.Messages[0].Content)
	}

	// Test session title generation
	if updatedSession.Title == "New Conversation" {
		t.Errorf("expected title to be generated from first message, but got default title")
	}

	// Test adding assistant response
	err = manager.AddMessage(ctx, session.ID, AssistantRole, "Hello! How can I help you?", nil)
	if err != nil {
		t.Fatalf("failed to add assistant message: %v", err)
	}

	// Test conversation history
	history, err := manager.GetConversationHistory(ctx, session.ID, 10)
	if err != nil {
		t.Fatalf("failed to get conversation history: %v", err)
	}

	if len(history) != 2 {
		t.Errorf("expected 2 messages in history, got %d", len(history))
	}

	// Test listing user sessions
	sessions, err := manager.ListUserSessions(ctx, userID)
	if err != nil {
		t.Fatalf("failed to list user sessions: %v", err)
	}

	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}

	// Test extending session
	err = manager.ExtendSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to extend session: %v", err)
	}

	// Test closing session
	err = manager.CloseSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to close session: %v", err)
	}

	closedSession, err := manager.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to get closed session: %v", err)
	}

	if closedSession.Status != SessionClosed {
		t.Errorf("expected status %s, got %s", SessionClosed, closedSession.Status)
	}

	// Test deleting session
	err = manager.DeleteSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to delete session: %v", err)
	}

	// Verify session is deleted
	_, err = manager.GetSession(ctx, session.ID)
	if err == nil {
		t.Errorf("expected error when getting deleted session")
	}
}

func TestSessionExpiry(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := Config{
		StorageType:     MemoryStorageType,
		DefaultTTL:      100 * time.Millisecond, // Very short TTL for testing
		MaxSessions:     1000,
		CleanupInterval: 0, // Disable cleanup for tests
	}

	manager, err := NewManager(config, logger)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	ctx := context.Background()
	userID := "test_user_expiry"

	// Create a session
	session, err := manager.CreateSession(ctx, userID)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Wait for session to expire
	time.Sleep(200 * time.Millisecond)

	// Try to get the session - it should be marked as expired
	expiredSession, err := manager.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to get expired session: %v", err)
	}

	if expiredSession.Status != SessionExpired {
		t.Errorf("expected status %s, got %s", SessionExpired, expiredSession.Status)
	}
}

func TestConversationHistoryLimits(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := Config{
		StorageType:     MemoryStorageType,
		DefaultTTL:      30 * time.Minute,
		MaxSessions:     1000,
		CleanupInterval: 0,
	}

	manager, err := NewManager(config, logger)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	ctx := context.Background()
	userID := "test_user_history"

	// Create a session
	session, err := manager.CreateSession(ctx, userID)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Add multiple messages
	for i := 0; i < 10; i++ {
		role := UserRole
		content := "User message"
		if i%2 == 1 {
			role = AssistantRole
			content = "Assistant response"
		}

		err = manager.AddMessage(ctx, session.ID, role, content, nil)
		if err != nil {
			t.Fatalf("failed to add message %d: %v", i, err)
		}
	}

	// Test getting limited history
	history, err := manager.GetConversationHistory(ctx, session.ID, 5)
	if err != nil {
		t.Fatalf("failed to get conversation history: %v", err)
	}

	if len(history) != 5 {
		t.Errorf("expected 5 messages in limited history, got %d", len(history))
	}

	// Test getting full history
	fullHistory, err := manager.GetConversationHistory(ctx, session.ID, 20)
	if err != nil {
		t.Fatalf("failed to get full conversation history: %v", err)
	}

	if len(fullHistory) != 10 {
		t.Errorf("expected 10 messages in full history, got %d", len(fullHistory))
	}
}

func TestSessionStats(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := Config{
		StorageType:     MemoryStorageType,
		DefaultTTL:      30 * time.Minute,
		MaxSessions:     1000,
		CleanupInterval: 0,
	}

	manager, err := NewManager(config, logger)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	ctx := context.Background()

	// Test getting stats
	stats, err := manager.GetStats(ctx)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats["storage_type"] != string(MemoryStorageType) {
		t.Errorf("expected storage_type %s, got %v", MemoryStorageType, stats["storage_type"])
	}

	if stats["max_sessions"] != 1000 {
		t.Errorf("expected max_sessions 1000, got %v", stats["max_sessions"])
	}
}

func TestInvalidOperations(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := Config{
		StorageType:     MemoryStorageType,
		DefaultTTL:      30 * time.Minute,
		MaxSessions:     1000,
		CleanupInterval: 0,
	}

	manager, err := NewManager(config, logger)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	ctx := context.Background()

	// Test getting non-existent session
	_, err = manager.GetSession(ctx, "non-existent-session")
	if err == nil {
		t.Errorf("expected error when getting non-existent session")
	}

	// Test adding message to non-existent session
	err = manager.AddMessage(ctx, "non-existent-session", UserRole, "test", nil)
	if err == nil {
		t.Errorf("expected error when adding message to non-existent session")
	}

	// Test deleting non-existent session
	err = manager.DeleteSession(ctx, "non-existent-session")
	if err == nil {
		t.Errorf("expected error when deleting non-existent session")
	}

	// Create a session and close it
	session, err := manager.CreateSession(ctx, "test_user")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	err = manager.CloseSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to close session: %v", err)
	}

	// Test adding message to closed session
	err = manager.AddMessage(ctx, session.ID, UserRole, "test", nil)
	if err == nil {
		t.Errorf("expected error when adding message to closed session")
	}
}
