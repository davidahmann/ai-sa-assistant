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

// Package session provides session management functionality for conversation memory
// and multi-turn query processing in the AI SA Assistant. It supports both in-memory
// and Redis-based session storage with configurable expiration.
package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// StorageType represents the type of storage backend for sessions
type StorageType string

const (
	// MemoryStorageType uses in-memory storage for sessions
	MemoryStorageType StorageType = "memory"
	// RedisStorageType uses Redis for session storage
	RedisStorageType StorageType = "redis"
)

// Config holds configuration for session management
type Config struct {
	StorageType     StorageType   `json:"storage_type"`
	RedisURL        string        `json:"redis_url,omitempty"`
	DefaultTTL      time.Duration `json:"default_ttl"`
	MaxSessions     int           `json:"max_sessions"`
	CleanupInterval time.Duration `json:"cleanup_interval"`
}

// DefaultConfig returns default session configuration
func DefaultConfig() Config {
	return Config{
		StorageType:     MemoryStorageType,
		DefaultTTL:      30 * time.Minute,
		MaxSessions:     1000,
		CleanupInterval: 5 * time.Minute,
	}
}

// Session represents a conversation session with its metadata and history
type Session struct {
	ID         string            `json:"id"`
	UserID     string            `json:"user_id"`
	Title      string            `json:"title"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
	ExpiresAt  time.Time         `json:"expires_at"`
	Messages   []Message         `json:"messages"`
	Metadata   map[string]string `json:"metadata"`
	TokenCount int               `json:"token_count"`
	Status     SessionStatus     `json:"status"`
}

// SessionStatus represents the status of a session
type SessionStatus string

const (
	// SessionActive indicates an active session
	SessionActive SessionStatus = "active"
	// SessionExpired indicates an expired session
	SessionExpired SessionStatus = "expired"
	// SessionClosed indicates a closed session
	SessionClosed SessionStatus = "closed"
)

// Message represents a single message in a conversation
type Message struct {
	ID         string                 `json:"id"`
	Role       MessageRole            `json:"role"`
	Content    string                 `json:"content"`
	Timestamp  time.Time              `json:"timestamp"`
	TokenCount int                    `json:"token_count"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// MessageRole represents the role of a message sender
type MessageRole string

const (
	// UserRole indicates a message from the user
	UserRole MessageRole = "user"
	// AssistantRole indicates a message from the assistant
	AssistantRole MessageRole = "assistant"
	// SystemRole indicates a system message
	SystemRole MessageRole = "system"
)

// Storage defines the interface for session storage backends
type Storage interface {
	// Get retrieves a session by ID
	Get(ctx context.Context, sessionID string) (*Session, error)
	// Set stores a session with optional TTL
	Set(ctx context.Context, session *Session, ttl time.Duration) error
	// Delete removes a session
	Delete(ctx context.Context, sessionID string) error
	// List returns all sessions for a user
	List(ctx context.Context, userID string) ([]*Session, error)
	// Exists checks if a session exists
	Exists(ctx context.Context, sessionID string) (bool, error)
	// UpdateExpiry updates the expiry time for a session
	UpdateExpiry(ctx context.Context, sessionID string, expiresAt time.Time) error
	// Cleanup removes expired sessions
	Cleanup(ctx context.Context) error
	// Close closes the storage backend
	Close() error
}

// Manager handles session lifecycle and storage operations
type Manager struct {
	storage Storage
	config  Config
	logger  *zap.Logger
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

// NewManager creates a new session manager with the specified storage backend
func NewManager(config Config, logger *zap.Logger) (*Manager, error) {
	var storage Storage
	var err error

	switch config.StorageType {
	case MemoryStorageType:
		storage = NewMemoryStorage(config.MaxSessions)
	case RedisStorageType:
		storage, err = NewRedisStorage(config.RedisURL, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Redis storage: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", config.StorageType)
	}

	manager := &Manager{
		storage: storage,
		config:  config,
		logger:  logger,
		stopCh:  make(chan struct{}),
	}

	// Start cleanup goroutine
	if config.CleanupInterval > 0 {
		manager.wg.Add(1)
		go manager.cleanupLoop()
	}

	return manager, nil
}

// CreateSession creates a new session for a user
func (m *Manager) CreateSession(ctx context.Context, userID string) (*Session, error) {
	sessionID := GenerateSessionID()
	now := time.Now()

	session := &Session{
		ID:         sessionID,
		UserID:     userID,
		Title:      "New Conversation",
		CreatedAt:  now,
		UpdatedAt:  now,
		ExpiresAt:  now.Add(m.config.DefaultTTL),
		Messages:   []Message{},
		Metadata:   make(map[string]string),
		TokenCount: 0,
		Status:     SessionActive,
	}

	if err := m.storage.Set(ctx, session, m.config.DefaultTTL); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	m.logger.Info("Created new session",
		zap.String("session_id", sessionID),
		zap.String("user_id", userID))

	return session, nil
}

// GetSession retrieves a session by ID
func (m *Manager) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	session, err := m.storage.Get(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Check if session is expired
	if session.ExpiresAt.Before(time.Now()) {
		session.Status = SessionExpired
		return session, nil
	}

	return session, nil
}

// UpdateSession updates an existing session
func (m *Manager) UpdateSession(ctx context.Context, session *Session) error {
	session.UpdatedAt = time.Now()

	if err := m.storage.Set(ctx, session, m.config.DefaultTTL); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	return nil
}

// AddMessage adds a message to a session
func (m *Manager) AddMessage(ctx context.Context, sessionID string, role MessageRole, content string, metadata map[string]interface{}) error {
	session, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	if session.Status != SessionActive {
		return fmt.Errorf("cannot add message to inactive session")
	}

	message := Message{
		ID:         GenerateMessageID(),
		Role:       role,
		Content:    content,
		Timestamp:  time.Now(),
		TokenCount: EstimateTokenCount(content),
		Metadata:   metadata,
	}

	session.Messages = append(session.Messages, message)
	session.TokenCount += message.TokenCount

	// Update session title if this is the first user message
	if role == UserRole && len(session.Messages) == 1 {
		session.Title = GenerateTitle(content)
	}

	// Extend session expiry
	session.ExpiresAt = time.Now().Add(m.config.DefaultTTL)

	if err := m.UpdateSession(ctx, session); err != nil {
		return fmt.Errorf("failed to update session with new message: %w", err)
	}

	m.logger.Debug("Added message to session",
		zap.String("session_id", sessionID),
		zap.String("role", string(role)),
		zap.Int("token_count", message.TokenCount))

	return nil
}

// GetConversationHistory returns conversation history for context building
func (m *Manager) GetConversationHistory(ctx context.Context, sessionID string, maxMessages int) ([]Message, error) {
	session, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	messages := session.Messages
	if len(messages) > maxMessages {
		// Return the most recent messages
		messages = messages[len(messages)-maxMessages:]
	}

	return messages, nil
}

// ListUserSessions returns all sessions for a user
func (m *Manager) ListUserSessions(ctx context.Context, userID string) ([]*Session, error) {
	sessions, err := m.storage.List(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list user sessions: %w", err)
	}

	return sessions, nil
}

// DeleteSession removes a session
func (m *Manager) DeleteSession(ctx context.Context, sessionID string) error {
	if err := m.storage.Delete(ctx, sessionID); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	m.logger.Info("Deleted session", zap.String("session_id", sessionID))
	return nil
}

// ExtendSession extends the expiry time of a session
func (m *Manager) ExtendSession(ctx context.Context, sessionID string) error {
	session, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	session.ExpiresAt = time.Now().Add(m.config.DefaultTTL)

	if err := m.UpdateSession(ctx, session); err != nil {
		return fmt.Errorf("failed to extend session: %w", err)
	}

	return nil
}

// CloseSession marks a session as closed
func (m *Manager) CloseSession(ctx context.Context, sessionID string) error {
	session, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	session.Status = SessionClosed
	session.UpdatedAt = time.Now()

	if err := m.UpdateSession(ctx, session); err != nil {
		return fmt.Errorf("failed to close session: %w", err)
	}

	m.logger.Info("Closed session", zap.String("session_id", sessionID))
	return nil
}

// cleanupLoop runs periodic cleanup of expired sessions
func (m *Manager) cleanupLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := m.storage.Cleanup(ctx); err != nil {
				m.logger.Error("Failed to cleanup expired sessions", zap.Error(err))
			}
			cancel()
		case <-m.stopCh:
			return
		}
	}
}

// Close gracefully closes the session manager
func (m *Manager) Close() error {
	close(m.stopCh)
	m.wg.Wait()

	if err := m.storage.Close(); err != nil {
		return fmt.Errorf("failed to close storage: %w", err)
	}

	return nil
}

// GetStats returns session statistics
func (m *Manager) GetStats(ctx context.Context) (map[string]interface{}, error) {
	// This is a simplified version - in a production system you'd want more detailed metrics
	stats := map[string]interface{}{
		"storage_type": string(m.config.StorageType),
		"max_sessions": m.config.MaxSessions,
		"default_ttl":  m.config.DefaultTTL.String(),
	}

	return stats, nil
}
