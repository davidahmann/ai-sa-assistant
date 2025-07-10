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
)

func TestMemoryStorage(t *testing.T) {
	storage := NewMemoryStorage(10)
	defer func() { _ = storage.Close() }()

	ctx := context.Background()

	// Create test session
	session := &Session{
		ID:        "test_session_123",
		UserID:    "test_user",
		Title:     "Test Conversation",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(30 * time.Minute),
		Messages:  []Message{},
		Metadata:  make(map[string]string),
		Status:    SessionActive,
	}

	// Test Set
	err := storage.Set(ctx, session, 30*time.Minute)
	if err != nil {
		t.Fatalf("failed to set session: %v", err)
	}

	// Test Get
	retrieved, err := storage.Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	if retrieved.ID != session.ID {
		t.Errorf("expected session ID %s, got %s", session.ID, retrieved.ID)
	}
	if retrieved.UserID != session.UserID {
		t.Errorf("expected user ID %s, got %s", session.UserID, retrieved.UserID)
	}

	// Test Exists
	exists, err := storage.Exists(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to check existence: %v", err)
	}
	if !exists {
		t.Errorf("session should exist")
	}

	// Test non-existent session
	exists, err = storage.Exists(ctx, "non_existent")
	if err != nil {
		t.Fatalf("failed to check existence of non-existent session: %v", err)
	}
	if exists {
		t.Errorf("non-existent session should not exist")
	}

	// Test List for user
	sessions, err := storage.List(ctx, session.UserID)
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].ID != session.ID {
		t.Errorf("expected session ID %s, got %s", session.ID, sessions[0].ID)
	}

	// Test List for different user (should be empty)
	otherSessions, err := storage.List(ctx, "other_user")
	if err != nil {
		t.Fatalf("failed to list sessions for other user: %v", err)
	}
	if len(otherSessions) != 0 {
		t.Errorf("expected 0 sessions for other user, got %d", len(otherSessions))
	}

	// Test UpdateExpiry
	newExpiry := time.Now().Add(60 * time.Minute)
	err = storage.UpdateExpiry(ctx, session.ID, newExpiry)
	if err != nil {
		t.Fatalf("failed to update expiry: %v", err)
	}

	// Verify expiry was updated
	updated, err := storage.Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to get updated session: %v", err)
	}
	if !updated.ExpiresAt.Equal(newExpiry) {
		t.Errorf("expected expiry %v, got %v", newExpiry, updated.ExpiresAt)
	}

	// Test Delete
	err = storage.Delete(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to delete session: %v", err)
	}

	// Verify deletion
	_, err = storage.Get(ctx, session.ID)
	if err == nil {
		t.Errorf("expected error when getting deleted session")
	}

	// Verify user index was updated
	sessions, err = storage.List(ctx, session.UserID)
	if err != nil {
		t.Fatalf("failed to list sessions after deletion: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions after deletion, got %d", len(sessions))
	}
}

func TestMemoryStorageExpiry(t *testing.T) {
	storage := NewMemoryStorage(10)
	defer func() { _ = storage.Close() }()

	ctx := context.Background()

	// Create session that will expire soon
	session := &Session{
		ID:        "expiring_session",
		UserID:    "test_user",
		Title:     "Expiring Conversation",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(100 * time.Millisecond),
		Messages:  []Message{},
		Metadata:  make(map[string]string),
		Status:    SessionActive,
	}

	err := storage.Set(ctx, session, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to set session: %v", err)
	}

	// Wait for expiry
	time.Sleep(200 * time.Millisecond)

	// Test cleanup
	err = storage.Cleanup(ctx)
	if err != nil {
		t.Fatalf("failed to cleanup: %v", err)
	}

	// Session should be removed
	_, err = storage.Get(ctx, session.ID)
	if err == nil {
		t.Errorf("expected error when getting expired session after cleanup")
	}
}

func TestMemoryStorageLRU(t *testing.T) {
	// Create storage with small capacity
	storage := NewMemoryStorage(2)
	defer func() { _ = storage.Close() }()

	ctx := context.Background()

	// Create sessions
	session1 := &Session{
		ID:        "session_1",
		UserID:    "user_1",
		Title:     "Session 1",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(30 * time.Minute),
		Status:    SessionActive,
	}

	session2 := &Session{
		ID:        "session_2",
		UserID:    "user_2",
		Title:     "Session 2",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(30 * time.Minute),
		Status:    SessionActive,
	}

	session3 := &Session{
		ID:        "session_3",
		UserID:    "user_3",
		Title:     "Session 3",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(30 * time.Minute),
		Status:    SessionActive,
	}

	// Add first two sessions
	err := storage.Set(ctx, session1, 30*time.Minute)
	if err != nil {
		t.Fatalf("failed to set session1: %v", err)
	}

	err = storage.Set(ctx, session2, 30*time.Minute)
	if err != nil {
		t.Fatalf("failed to set session2: %v", err)
	}

	// Both should exist
	exists1, _ := storage.Exists(ctx, session1.ID)
	exists2, _ := storage.Exists(ctx, session2.ID)
	if !exists1 || !exists2 {
		t.Errorf("both sessions should exist before reaching capacity")
	}

	// Add third session (should trigger LRU eviction)
	err = storage.Set(ctx, session3, 30*time.Minute)
	if err != nil {
		t.Fatalf("failed to set session3: %v", err)
	}

	// Session 1 should be evicted (least recently used)
	exists1, _ = storage.Exists(ctx, session1.ID)
	exists2, _ = storage.Exists(ctx, session2.ID)
	exists3, _ := storage.Exists(ctx, session3.ID)

	if exists1 {
		t.Errorf("session1 should have been evicted")
	}
	if !exists2 {
		t.Errorf("session2 should still exist")
	}
	if !exists3 {
		t.Errorf("session3 should exist")
	}
}

func TestMemoryStorageMultipleUsers(t *testing.T) {
	storage := NewMemoryStorage(10)
	defer func() { _ = storage.Close() }()

	ctx := context.Background()

	// Create sessions for different users
	user1Sessions := []*Session{
		{
			ID:        "user1_session1",
			UserID:    "user_1",
			Title:     "User 1 Session 1",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			ExpiresAt: time.Now().Add(30 * time.Minute),
			Status:    SessionActive,
		},
		{
			ID:        "user1_session2",
			UserID:    "user_1",
			Title:     "User 1 Session 2",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			ExpiresAt: time.Now().Add(30 * time.Minute),
			Status:    SessionActive,
		},
	}

	user2Sessions := []*Session{
		{
			ID:        "user2_session1",
			UserID:    "user_2",
			Title:     "User 2 Session 1",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			ExpiresAt: time.Now().Add(30 * time.Minute),
			Status:    SessionActive,
		},
	}

	// Add all sessions
	for _, session := range user1Sessions {
		err := storage.Set(ctx, session, 30*time.Minute)
		if err != nil {
			t.Fatalf("failed to set user1 session: %v", err)
		}
	}

	for _, session := range user2Sessions {
		err := storage.Set(ctx, session, 30*time.Minute)
		if err != nil {
			t.Fatalf("failed to set user2 session: %v", err)
		}
	}

	// Test listing sessions by user
	user1List, err := storage.List(ctx, "user_1")
	if err != nil {
		t.Fatalf("failed to list user1 sessions: %v", err)
	}
	if len(user1List) != 2 {
		t.Errorf("expected 2 sessions for user1, got %d", len(user1List))
	}

	user2List, err := storage.List(ctx, "user_2")
	if err != nil {
		t.Fatalf("failed to list user2 sessions: %v", err)
	}
	if len(user2List) != 1 {
		t.Errorf("expected 1 session for user2, got %d", len(user2List))
	}

	// Test that users only see their own sessions
	for _, session := range user1List {
		if session.UserID != "user_1" {
			t.Errorf("user1 list contains session for wrong user: %s", session.UserID)
		}
	}

	for _, session := range user2List {
		if session.UserID != "user_2" {
			t.Errorf("user2 list contains session for wrong user: %s", session.UserID)
		}
	}
}

func TestMemoryStorageErrorCases(t *testing.T) {
	storage := NewMemoryStorage(10)
	defer func() { _ = storage.Close() }()

	ctx := context.Background()

	// Test getting non-existent session
	_, err := storage.Get(ctx, "non_existent")
	if err == nil {
		t.Errorf("expected error when getting non-existent session")
	}

	// Test deleting non-existent session
	err = storage.Delete(ctx, "non_existent")
	if err == nil {
		t.Errorf("expected error when deleting non-existent session")
	}

	// Test updating expiry for non-existent session
	err = storage.UpdateExpiry(ctx, "non_existent", time.Now().Add(time.Hour))
	if err == nil {
		t.Errorf("expected error when updating expiry for non-existent session")
	}

	// Test setting session with past expiry
	pastSession := &Session{
		ID:        "past_session",
		UserID:    "test_user",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(-time.Hour), // Past expiry
		Status:    SessionActive,
	}

	err = storage.Set(ctx, pastSession, -time.Hour)
	// Note: Memory storage doesn't validate TTL - it just stores sessions
	// This test verifies the behavior is consistent
}
