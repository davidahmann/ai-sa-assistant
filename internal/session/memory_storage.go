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
	"fmt"
	"sync"
	"time"
)

// MemoryStorage provides in-memory session storage with LRU eviction
type MemoryStorage struct {
	sessions    map[string]*Session
	userIndex   map[string][]string // Maps user ID to session IDs
	maxSessions int
	mutex       sync.RWMutex
	accessTime  map[string]time.Time // Track access time for LRU
}

// NewMemoryStorage creates a new in-memory session storage
func NewMemoryStorage(maxSessions int) *MemoryStorage {
	return &MemoryStorage{
		sessions:    make(map[string]*Session),
		userIndex:   make(map[string][]string),
		maxSessions: maxSessions,
		accessTime:  make(map[string]time.Time),
	}
}

// Get retrieves a session by ID
func (m *MemoryStorage) Get(_ context.Context, sessionID string) (*Session, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	// Update access time for LRU
	m.accessTime[sessionID] = time.Now()

	// Return a copy to prevent external modification
	sessionCopy := *session
	sessionCopy.Messages = make([]Message, len(session.Messages))
	copy(sessionCopy.Messages, session.Messages)

	return &sessionCopy, nil
}

// Set stores a session with optional TTL
func (m *MemoryStorage) Set(_ context.Context, session *Session, ttl time.Duration) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Check if we need to evict sessions
	if len(m.sessions) >= m.maxSessions {
		if err := m.evictOldestSession(); err != nil {
			return fmt.Errorf("failed to evict session: %w", err)
		}
	}

	// Store a copy to prevent external modification
	sessionCopy := *session
	sessionCopy.Messages = make([]Message, len(session.Messages))
	copy(sessionCopy.Messages, session.Messages)

	// If TTL is provided, update expiry time
	if ttl > 0 {
		sessionCopy.ExpiresAt = time.Now().Add(ttl)
	}

	m.sessions[session.ID] = &sessionCopy
	m.accessTime[session.ID] = time.Now()

	// Update user index
	m.updateUserIndex(session.UserID, session.ID)

	return nil
}

// Delete removes a session
func (m *MemoryStorage) Delete(_ context.Context, sessionID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Remove from user index
	m.removeFromUserIndex(session.UserID, sessionID)

	// Remove from storage
	delete(m.sessions, sessionID)
	delete(m.accessTime, sessionID)

	return nil
}

// List returns all sessions for a user
func (m *MemoryStorage) List(_ context.Context, userID string) ([]*Session, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	sessionIDs, exists := m.userIndex[userID]
	if !exists {
		return []*Session{}, nil
	}

	var sessions []*Session
	for _, sessionID := range sessionIDs {
		if session, exists := m.sessions[sessionID]; exists {
			// Return a copy to prevent external modification
			sessionCopy := *session
			sessionCopy.Messages = make([]Message, len(session.Messages))
			copy(sessionCopy.Messages, session.Messages)
			sessions = append(sessions, &sessionCopy)
		}
	}

	return sessions, nil
}

// Exists checks if a session exists
func (m *MemoryStorage) Exists(_ context.Context, sessionID string) (bool, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	_, exists := m.sessions[sessionID]
	return exists, nil
}

// UpdateExpiry updates the expiry time for a session
func (m *MemoryStorage) UpdateExpiry(_ context.Context, sessionID string, expiresAt time.Time) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.ExpiresAt = expiresAt
	m.accessTime[sessionID] = time.Now()

	return nil
}

// Cleanup removes expired sessions
func (m *MemoryStorage) Cleanup(_ context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	var expiredSessions []string

	// Find expired sessions
	for sessionID, session := range m.sessions {
		if session.ExpiresAt.Before(now) {
			expiredSessions = append(expiredSessions, sessionID)
		}
	}

	// Remove expired sessions
	for _, sessionID := range expiredSessions {
		if session, exists := m.sessions[sessionID]; exists {
			m.removeFromUserIndex(session.UserID, sessionID)
			delete(m.sessions, sessionID)
			delete(m.accessTime, sessionID)
		}
	}

	return nil
}

// Close closes the storage backend
func (m *MemoryStorage) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Clear all data
	m.sessions = make(map[string]*Session)
	m.userIndex = make(map[string][]string)
	m.accessTime = make(map[string]time.Time)

	return nil
}

// updateUserIndex updates the user index with a new session
func (m *MemoryStorage) updateUserIndex(userID, sessionID string) {
	sessionIDs, exists := m.userIndex[userID]
	if !exists {
		m.userIndex[userID] = []string{sessionID}
		return
	}

	// Check if session already exists in index
	for _, id := range sessionIDs {
		if id == sessionID {
			return
		}
	}

	// Add session to index
	m.userIndex[userID] = append(sessionIDs, sessionID)
}

// removeFromUserIndex removes a session from the user index
func (m *MemoryStorage) removeFromUserIndex(userID, sessionID string) {
	sessionIDs, exists := m.userIndex[userID]
	if !exists {
		return
	}

	// Remove session from index
	var newSessionIDs []string
	for _, id := range sessionIDs {
		if id != sessionID {
			newSessionIDs = append(newSessionIDs, id)
		}
	}

	if len(newSessionIDs) == 0 {
		delete(m.userIndex, userID)
	} else {
		m.userIndex[userID] = newSessionIDs
	}
}

// evictOldestSession removes the least recently used session
func (m *MemoryStorage) evictOldestSession() error {
	if len(m.sessions) == 0 {
		return nil
	}

	var oldestSessionID string
	var oldestTime time.Time

	// Find the oldest session by access time
	for sessionID, accessTime := range m.accessTime {
		if oldestSessionID == "" || accessTime.Before(oldestTime) {
			oldestSessionID = sessionID
			oldestTime = accessTime
		}
	}

	// Remove the oldest session
	if oldestSessionID != "" {
		if session, exists := m.sessions[oldestSessionID]; exists {
			m.removeFromUserIndex(session.UserID, oldestSessionID)
			delete(m.sessions, oldestSessionID)
			delete(m.accessTime, oldestSessionID)
		}
	}

	return nil
}

// GetStats returns storage statistics
func (m *MemoryStorage) GetStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stats := map[string]interface{}{
		"total_sessions":   len(m.sessions),
		"max_sessions":     m.maxSessions,
		"total_users":      len(m.userIndex),
		"memory_usage_est": m.estimateMemoryUsage(),
	}

	return stats
}

// estimateMemoryUsage provides a rough estimate of memory usage
func (m *MemoryStorage) estimateMemoryUsage() string {
	totalMessages := 0
	totalContent := 0

	for _, session := range m.sessions {
		totalMessages += len(session.Messages)
		for _, message := range session.Messages {
			totalContent += len(message.Content)
		}
	}

	// Rough estimate: session metadata + message content
	estimatedBytes := len(m.sessions)*200 + totalContent

	switch {
	case estimatedBytes < 1024:
		return fmt.Sprintf("%d bytes", estimatedBytes)
	case estimatedBytes < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(estimatedBytes)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(estimatedBytes)/(1024*1024))
	}
}
