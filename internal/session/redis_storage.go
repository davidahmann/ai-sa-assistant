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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

// RedisStorage provides Redis-based session storage
type RedisStorage struct {
	client RedisClient
	logger *zap.Logger
	prefix string
}

// RedisClient defines the interface for Redis operations
// This allows for easy testing and different Redis client implementations
type RedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	Del(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, keys ...string) (int64, error)
	Keys(ctx context.Context, pattern string) ([]string, error)
	Expire(ctx context.Context, key string, expiration time.Duration) error
	Close() error
}

// NewRedisStorage creates a new Redis session storage
func NewRedisStorage(_ string, _ *zap.Logger) (*RedisStorage, error) {
	// For now, we'll return an error since Redis client implementation
	// would require adding the go-redis dependency to go.mod
	// In a real implementation, you would:
	// 1. Add "github.com/go-redis/redis/v8" to go.mod
	// 2. Create a Redis client instance
	// 3. Test the connection

	return nil, fmt.Errorf("redis storage not implemented in demo - use memory storage instead")
}

// Get retrieves a session by ID
func (r *RedisStorage) Get(ctx context.Context, sessionID string) (*Session, error) {
	key := r.sessionKey(sessionID)

	data, err := r.client.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get session from Redis: %w", err)
	}

	if data == "" {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	var session Session
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

// Set stores a session with optional TTL
func (r *RedisStorage) Set(ctx context.Context, session *Session, ttl time.Duration) error {
	key := r.sessionKey(session.ID)

	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	if err := r.client.Set(ctx, key, data, ttl); err != nil {
		return fmt.Errorf("failed to set session in Redis: %w", err)
	}

	// Update user index
	if err := r.updateUserIndex(ctx, session.UserID, session.ID); err != nil {
		r.logger.Warn("Failed to update user index", zap.Error(err))
	}

	return nil
}

// Delete removes a session
func (r *RedisStorage) Delete(ctx context.Context, sessionID string) error {
	// Get session first to remove from user index
	session, err := r.Get(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session for deletion: %w", err)
	}

	key := r.sessionKey(sessionID)

	if err := r.client.Del(ctx, key); err != nil {
		return fmt.Errorf("failed to delete session from Redis: %w", err)
	}

	// Remove from user index
	if err := r.removeFromUserIndex(ctx, session.UserID, sessionID); err != nil {
		r.logger.Warn("Failed to remove from user index", zap.Error(err))
	}

	return nil
}

// List returns all sessions for a user
func (r *RedisStorage) List(ctx context.Context, userID string) ([]*Session, error) {
	userKey := r.userIndexKey(userID)

	sessionIDsData, err := r.client.Get(ctx, userKey)
	if err != nil {
		return []*Session{}, nil // Return empty list if user has no sessions
	}

	if sessionIDsData == "" {
		return []*Session{}, nil
	}

	var sessionIDs []string
	if err := json.Unmarshal([]byte(sessionIDsData), &sessionIDs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user session index: %w", err)
	}

	var sessions []*Session
	for _, sessionID := range sessionIDs {
		session, err := r.Get(ctx, sessionID)
		if err != nil {
			// Log error but continue with other sessions
			r.logger.Warn("Failed to get session from user index",
				zap.String("session_id", sessionID), zap.Error(err))
			continue
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

// Exists checks if a session exists
func (r *RedisStorage) Exists(ctx context.Context, sessionID string) (bool, error) {
	key := r.sessionKey(sessionID)

	count, err := r.client.Exists(ctx, key)
	if err != nil {
		return false, fmt.Errorf("failed to check session existence: %w", err)
	}

	return count > 0, nil
}

// UpdateExpiry updates the expiry time for a session
func (r *RedisStorage) UpdateExpiry(ctx context.Context, sessionID string, expiresAt time.Time) error {
	key := r.sessionKey(sessionID)

	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return fmt.Errorf("expiry time is in the past")
	}

	if err := r.client.Expire(ctx, key, ttl); err != nil {
		return fmt.Errorf("failed to update session expiry: %w", err)
	}

	return nil
}

// Cleanup removes expired sessions
func (r *RedisStorage) Cleanup(ctx context.Context) error {
	// Redis automatically removes expired keys, but we need to clean up user indexes
	pattern := r.userIndexKey("*")

	userKeys, err := r.client.Keys(ctx, pattern)
	if err != nil {
		return fmt.Errorf("failed to get user index keys: %w", err)
	}

	for _, userKey := range userKeys {
		if err := r.cleanupUserIndex(ctx, userKey); err != nil {
			r.logger.Warn("Failed to cleanup user index",
				zap.String("user_key", userKey), zap.Error(err))
		}
	}

	return nil
}

// Close closes the storage backend
func (r *RedisStorage) Close() error {
	return r.client.Close()
}

// sessionKey returns the Redis key for a session
func (r *RedisStorage) sessionKey(sessionID string) string {
	return fmt.Sprintf("%ssession:%s", r.prefix, sessionID)
}

// userIndexKey returns the Redis key for a user's session index
func (r *RedisStorage) userIndexKey(userID string) string {
	return fmt.Sprintf("%suser_index:%s", r.prefix, userID)
}

// updateUserIndex updates the user session index
func (r *RedisStorage) updateUserIndex(ctx context.Context, userID, sessionID string) error {
	userKey := r.userIndexKey(userID)

	// Get existing session IDs
	sessionIDsData, err := r.client.Get(ctx, userKey)
	var sessionIDs []string

	if err == nil && sessionIDsData != "" {
		if err := json.Unmarshal([]byte(sessionIDsData), &sessionIDs); err != nil {
			return fmt.Errorf("failed to unmarshal existing session IDs: %w", err)
		}
	}

	// Check if session ID already exists
	for _, id := range sessionIDs {
		if id == sessionID {
			return nil // Already exists
		}
	}

	// Add new session ID
	sessionIDs = append(sessionIDs, sessionID)

	// Marshal and store
	data, err := json.Marshal(sessionIDs)
	if err != nil {
		return fmt.Errorf("failed to marshal session IDs: %w", err)
	}

	// Set with a longer TTL than individual sessions
	if err := r.client.Set(ctx, userKey, data, 24*time.Hour); err != nil {
		return fmt.Errorf("failed to update user index: %w", err)
	}

	return nil
}

// removeFromUserIndex removes a session from the user index
func (r *RedisStorage) removeFromUserIndex(ctx context.Context, userID, sessionID string) error {
	userKey := r.userIndexKey(userID)

	// Get existing session IDs
	sessionIDsData, err := r.client.Get(ctx, userKey)
	if err != nil {
		return nil // User index doesn't exist
	}

	if sessionIDsData == "" {
		return nil
	}

	var sessionIDs []string
	if err := json.Unmarshal([]byte(sessionIDsData), &sessionIDs); err != nil {
		return fmt.Errorf("failed to unmarshal session IDs: %w", err)
	}

	// Remove session ID
	var newSessionIDs []string
	for _, id := range sessionIDs {
		if id != sessionID {
			newSessionIDs = append(newSessionIDs, id)
		}
	}

	// Update or delete user index
	if len(newSessionIDs) == 0 {
		if err := r.client.Del(ctx, userKey); err != nil {
			return fmt.Errorf("failed to delete empty user index: %w", err)
		}
	} else {
		data, err := json.Marshal(newSessionIDs)
		if err != nil {
			return fmt.Errorf("failed to marshal updated session IDs: %w", err)
		}

		if err := r.client.Set(ctx, userKey, data, 24*time.Hour); err != nil {
			return fmt.Errorf("failed to update user index: %w", err)
		}
	}

	return nil
}

// cleanupUserIndex removes references to expired sessions from user index
func (r *RedisStorage) cleanupUserIndex(ctx context.Context, userKey string) error {
	// Get session IDs from user index
	sessionIDsData, err := r.client.Get(ctx, userKey)
	if err != nil {
		return nil
	}

	if sessionIDsData == "" {
		return nil
	}

	var sessionIDs []string
	if err := json.Unmarshal([]byte(sessionIDsData), &sessionIDs); err != nil {
		return fmt.Errorf("failed to unmarshal session IDs: %w", err)
	}

	// Check which sessions still exist
	var validSessionIDs []string
	for _, sessionID := range sessionIDs {
		exists, err := r.Exists(ctx, sessionID)
		if err != nil {
			r.logger.Warn("Failed to check session existence during cleanup",
				zap.String("session_id", sessionID), zap.Error(err))
			continue
		}

		if exists {
			validSessionIDs = append(validSessionIDs, sessionID)
		}
	}

	// Update user index with valid sessions only
	if len(validSessionIDs) == 0 {
		if err := r.client.Del(ctx, userKey); err != nil {
			return fmt.Errorf("failed to delete empty user index: %w", err)
		}
	} else if len(validSessionIDs) != len(sessionIDs) {
		data, err := json.Marshal(validSessionIDs)
		if err != nil {
			return fmt.Errorf("failed to marshal valid session IDs: %w", err)
		}

		if err := r.client.Set(ctx, userKey, data, 24*time.Hour); err != nil {
			return fmt.Errorf("failed to update cleaned user index: %w", err)
		}
	}

	return nil
}

// extractUserIDFromKey extracts user ID from a user index key
func (r *RedisStorage) extractUserIDFromKey(key string) string {
	prefix := r.userIndexKey("")
	if strings.HasPrefix(key, prefix) {
		return strings.TrimPrefix(key, prefix)
	}
	return ""
}
