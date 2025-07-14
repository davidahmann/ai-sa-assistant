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

// Package conversation provides conversation metadata management and REST API
// functionality for the AI SA Assistant. It handles conversation listing,
// creation, updates, and deletion with proper session integration.
package conversation

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/your-org/ai-sa-assistant/internal/session"
	"go.uber.org/zap"
)

// Metadata represents the metadata for a conversation
type Metadata struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	UserID       string    `json:"user_id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	LastActivity time.Time `json:"last_activity"`
	MessageCount int       `json:"message_count"`
	TokenCount   int       `json:"token_count"`
	Status       string    `json:"status"`
	Tags         []string  `json:"tags,omitempty"`
}

// Summary provides a brief summary of conversation content
type Summary struct {
	Metadata     Metadata `json:"metadata"`
	FirstMessage string   `json:"first_message,omitempty"`
	LastMessage  string   `json:"last_message,omitempty"`
	MessageCount int      `json:"message_count"`
	Duration     string   `json:"duration"`
}

// List represents a paginated list of conversations
type List struct {
	Conversations []Summary `json:"conversations"`
	Total         int       `json:"total"`
	Page          int       `json:"page"`
	PageSize      int       `json:"page_size"`
	HasMore       bool      `json:"has_more"`
}

// Manager handles conversation metadata operations
type Manager struct {
	sessionManager *session.Manager
	logger         *zap.Logger
}

// NewManager creates a new conversation manager
func NewManager(sessionManager *session.Manager, logger *zap.Logger) *Manager {
	return &Manager{
		sessionManager: sessionManager,
		logger:         logger,
	}
}

// CreateConversation creates a new conversation session
func (m *Manager) CreateConversation(ctx context.Context, userID string) (*Metadata, error) {
	sess, err := m.sessionManager.CreateSession(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	metadata := &Metadata{
		ID:           sess.ID,
		Title:        sess.Title,
		UserID:       sess.UserID,
		CreatedAt:    sess.CreatedAt,
		UpdatedAt:    sess.UpdatedAt,
		LastActivity: sess.UpdatedAt,
		MessageCount: len(sess.Messages),
		TokenCount:   sess.TokenCount,
		Status:       string(sess.Status),
		Tags:         []string{},
	}

	m.logger.Info("Created new conversation",
		zap.String("conversation_id", metadata.ID),
		zap.String("user_id", userID))

	return metadata, nil
}

// GetConversation retrieves conversation metadata by ID
func (m *Manager) GetConversation(ctx context.Context, conversationID string) (*Metadata, error) {
	sess, err := m.sessionManager.GetSession(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	lastActivity := sess.UpdatedAt
	if len(sess.Messages) > 0 {
		lastMessage := sess.Messages[len(sess.Messages)-1]
		lastActivity = lastMessage.Timestamp
	}

	metadata := &Metadata{
		ID:           sess.ID,
		Title:        sess.Title,
		UserID:       sess.UserID,
		CreatedAt:    sess.CreatedAt,
		UpdatedAt:    sess.UpdatedAt,
		LastActivity: lastActivity,
		MessageCount: len(sess.Messages),
		TokenCount:   sess.TokenCount,
		Status:       string(sess.Status),
		Tags:         extractTagsFromMetadata(sess.Metadata),
	}

	return metadata, nil
}

// ListConversations returns a paginated list of conversations for a user
func (m *Manager) ListConversations(ctx context.Context, userID string, page, pageSize int) (*List, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	sessions, err := m.sessionManager.ListUserSessions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list user sessions: %w", err)
	}

	// Convert sessions to conversation summaries
	var summaries []Summary
	for _, sess := range sessions {
		summary := m.createSummary(sess)
		summaries = append(summaries, summary)
	}

	// Sort by last activity (most recent first)
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Metadata.LastActivity.After(summaries[j].Metadata.LastActivity)
	})

	// Apply pagination
	total := len(summaries)
	startIdx := (page - 1) * pageSize
	endIdx := startIdx + pageSize

	if startIdx >= total {
		return &List{
			Conversations: []Summary{},
			Total:         total,
			Page:          page,
			PageSize:      pageSize,
			HasMore:       false,
		}, nil
	}

	if endIdx > total {
		endIdx = total
	}

	paginatedSummaries := summaries[startIdx:endIdx]
	hasMore := endIdx < total

	return &List{
		Conversations: paginatedSummaries,
		Total:         total,
		Page:          page,
		PageSize:      pageSize,
		HasMore:       hasMore,
	}, nil
}

// UpdateConversationTitle updates the title of a conversation
func (m *Manager) UpdateConversationTitle(ctx context.Context, conversationID, newTitle string) error {
	sess, err := m.sessionManager.GetSession(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	sess.Title = newTitle
	sess.UpdatedAt = time.Now()

	if err := m.sessionManager.UpdateSession(ctx, sess); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	m.logger.Info("Updated conversation title",
		zap.String("conversation_id", conversationID),
		zap.String("new_title", newTitle))

	return nil
}

// DeleteConversation deletes a conversation
func (m *Manager) DeleteConversation(ctx context.Context, conversationID string) error {
	if err := m.sessionManager.DeleteSession(ctx, conversationID); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	m.logger.Info("Deleted conversation",
		zap.String("conversation_id", conversationID))

	return nil
}

// GetConversationHistory returns the full message history for a conversation
func (m *Manager) GetConversationHistory(
	ctx context.Context,
	conversationID string,
	maxMessages int,
) ([]session.Message, error) {
	if maxMessages <= 0 {
		maxMessages = 50 // Default limit
	}

	messages, err := m.sessionManager.GetConversationHistory(ctx, conversationID, maxMessages)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation history: %w", err)
	}

	return messages, nil
}

// AddMessageToConversation adds a message to a conversation
func (m *Manager) AddMessageToConversation(
	ctx context.Context,
	conversationID string,
	role session.MessageRole,
	content string,
	metadata map[string]interface{},
) error {
	if err := m.sessionManager.AddMessage(ctx, conversationID, role, content, metadata); err != nil {
		return fmt.Errorf("failed to add message to conversation: %w", err)
	}

	m.logger.Debug("Added message to conversation",
		zap.String("conversation_id", conversationID),
		zap.String("role", string(role)))

	return nil
}

// SearchConversations searches conversations by title or content
func (m *Manager) SearchConversations(
	ctx context.Context,
	userID, query string,
	limit int,
) ([]Summary, error) {
	if limit <= 0 {
		limit = 10
	}

	sessions, err := m.sessionManager.ListUserSessions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list user sessions: %w", err)
	}

	var matchingSummaries []Summary
	queryLower := strings.ToLower(query)

	for _, sess := range sessions {
		// Check if title matches
		if strings.Contains(strings.ToLower(sess.Title), queryLower) {
			summary := m.createSummary(sess)
			matchingSummaries = append(matchingSummaries, summary)
			continue
		}

		// Check if any message content matches
		for _, message := range sess.Messages {
			if strings.Contains(strings.ToLower(message.Content), queryLower) {
				summary := m.createSummary(sess)
				matchingSummaries = append(matchingSummaries, summary)
				break
			}
		}

		if len(matchingSummaries) >= limit {
			break
		}
	}

	// Sort by relevance (title matches first, then by last activity)
	sort.Slice(matchingSummaries, func(i, j int) bool {
		titleMatchI := strings.Contains(strings.ToLower(matchingSummaries[i].Metadata.Title), queryLower)
		titleMatchJ := strings.Contains(strings.ToLower(matchingSummaries[j].Metadata.Title), queryLower)

		if titleMatchI && !titleMatchJ {
			return true
		}
		if !titleMatchI && titleMatchJ {
			return false
		}

		return matchingSummaries[i].Metadata.LastActivity.After(matchingSummaries[j].Metadata.LastActivity)
	})

	if len(matchingSummaries) > limit {
		matchingSummaries = matchingSummaries[:limit]
	}

	return matchingSummaries, nil
}

// GetConversationStats returns statistics about conversations for a user
func (m *Manager) GetConversationStats(ctx context.Context, userID string) (map[string]interface{}, error) {
	sessions, err := m.sessionManager.ListUserSessions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list user sessions: %w", err)
	}

	totalConversations := len(sessions)
	totalMessages := 0
	totalTokens := 0
	activeSessions := 0

	var oldestSession, newestSession time.Time
	if totalConversations > 0 {
		oldestSession = sessions[0].CreatedAt
		newestSession = sessions[0].CreatedAt
	}

	for _, sess := range sessions {
		totalMessages += len(sess.Messages)
		totalTokens += sess.TokenCount

		if sess.Status == session.SessionActive {
			activeSessions++
		}

		if sess.CreatedAt.Before(oldestSession) {
			oldestSession = sess.CreatedAt
		}
		if sess.CreatedAt.After(newestSession) {
			newestSession = sess.CreatedAt
		}
	}

	avgMessagesPerConversation := 0.0
	if totalConversations > 0 {
		avgMessagesPerConversation = float64(totalMessages) / float64(totalConversations)
	}

	stats := map[string]interface{}{
		"total_conversations":           totalConversations,
		"active_conversations":          activeSessions,
		"total_messages":                totalMessages,
		"total_tokens":                  totalTokens,
		"avg_messages_per_conversation": avgMessagesPerConversation,
	}

	if totalConversations > 0 {
		stats["oldest_conversation"] = oldestSession.Format(time.RFC3339)
		stats["newest_conversation"] = newestSession.Format(time.RFC3339)
	}

	return stats, nil
}

// createSummary creates a conversation summary from a session
func (m *Manager) createSummary(sess *session.Session) Summary {
	lastActivity := sess.UpdatedAt
	var firstMessage, lastMessage string

	if len(sess.Messages) > 0 {
		firstMessage = truncateMessage(sess.Messages[0].Content, 100)
		lastMessageObj := sess.Messages[len(sess.Messages)-1]
		lastMessage = truncateMessage(lastMessageObj.Content, 100)
		lastActivity = lastMessageObj.Timestamp
	}

	duration := time.Since(sess.CreatedAt).String()

	metadata := Metadata{
		ID:           sess.ID,
		Title:        sess.Title,
		UserID:       sess.UserID,
		CreatedAt:    sess.CreatedAt,
		UpdatedAt:    sess.UpdatedAt,
		LastActivity: lastActivity,
		MessageCount: len(sess.Messages),
		TokenCount:   sess.TokenCount,
		Status:       string(sess.Status),
		Tags:         extractTagsFromMetadata(sess.Metadata),
	}

	return Summary{
		Metadata:     metadata,
		FirstMessage: firstMessage,
		LastMessage:  lastMessage,
		MessageCount: len(sess.Messages),
		Duration:     duration,
	}
}

// truncateMessage truncates a message to a specified length
func truncateMessage(content string, maxLength int) string {
	if len(content) <= maxLength {
		return content
	}
	return content[:maxLength] + "..."
}

// extractTagsFromMetadata extracts tags from session metadata
func extractTagsFromMetadata(metadata map[string]string) []string {
	if tagsStr, exists := metadata["tags"]; exists {
		if tagsStr != "" {
			return strings.Split(tagsStr, ",")
		}
	}
	return []string{}
}

// FormatDuration formats a duration for display
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return "less than a minute"
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}
