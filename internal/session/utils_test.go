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
	"strings"
	"testing"
	"time"
)

func TestGenerateSessionID(t *testing.T) {
	// Test that session IDs are generated
	id1 := GenerateSessionID()
	id2 := GenerateSessionID()

	if id1 == "" {
		t.Errorf("session ID should not be empty")
	}
	if id2 == "" {
		t.Errorf("session ID should not be empty")
	}
	if id1 == id2 {
		t.Errorf("session IDs should be unique, got %s and %s", id1, id2)
	}

	// Test format
	if !strings.HasPrefix(id1, "session_") {
		t.Errorf("session ID should start with 'session_', got %s", id1)
	}
}

func TestGenerateMessageID(t *testing.T) {
	// Test that message IDs are generated
	id1 := GenerateMessageID()
	id2 := GenerateMessageID()

	if id1 == "" {
		t.Errorf("message ID should not be empty")
	}
	if id2 == "" {
		t.Errorf("message ID should not be empty")
	}
	if id1 == id2 {
		t.Errorf("message IDs should be unique, got %s and %s", id1, id2)
	}

	// Test format
	if !strings.HasPrefix(id1, "msg_") {
		t.Errorf("message ID should start with 'msg_', got %s", id1)
	}
}

func TestGenerateTitle(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "empty content",
			content:  "",
			expected: "New Conversation",
		},
		{
			name:     "whitespace only",
			content:  "   \t\n   ",
			expected: "New Conversation",
		},
		{
			name:     "simple message",
			content:  "hello world",
			expected: "Hello world",
		},
		{
			name:     "message with bot mention",
			content:  "@SA-Assistant help me with AWS",
			expected: "Help me with AWS",
		},
		{
			name:     "long message truncation",
			content:  "This is a very long message that should be truncated because it exceeds the maximum title length limit",
			expected: "This is a very long message that should be truncated because...",
		},
		{
			name:     "multiple spaces",
			content:  "hello    world    with    spaces",
			expected: "Hello world with spaces",
		},
		{
			name:     "mixed case preservation",
			content:  "AWS Migration Plan",
			expected: "AWS Migration Plan",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateTitle(tt.content)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestEstimateTokenCount(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "empty text",
			text:     "",
			expected: 0,
		},
		{
			name:     "short text",
			text:     "hello",
			expected: 1, // 5 chars / 4 = 1.25, rounded down to 1
		},
		{
			name:     "medium text",
			text:     "hello world",
			expected: 2, // 11 chars / 4 = 2.75, rounded down to 2
		},
		{
			name:     "longer text",
			text:     "This is a longer piece of text for testing token estimation",
			expected: 14, // 59 chars / 4 = 14.75, rounded down to 14
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EstimateTokenCount(tt.text)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestValidateSessionID(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		expected  bool
	}{
		{
			name:      "empty ID",
			sessionID: "",
			expected:  false,
		},
		{
			name:      "valid session ID",
			sessionID: "session_1234567890abcdef1234567890abcdef",
			expected:  true,
		},
		{
			name:      "wrong prefix",
			sessionID: "sess_1234567890abcdef1234567890abcdef",
			expected:  false,
		},
		{
			name:      "too short",
			sessionID: "session_123",
			expected:  false,
		},
		{
			name:      "invalid characters",
			sessionID: "session_1234567890abcdef1234567890ABCDEF",
			expected:  false,
		},
		{
			name:      "no underscore",
			sessionID: "session1234567890abcdef1234567890abcdef",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateSessionID(tt.sessionID)
			if result != tt.expected {
				t.Errorf("expected %v, got %v for ID %s", tt.expected, result, tt.sessionID)
			}
		})
	}
}

func TestValidateMessageID(t *testing.T) {
	tests := []struct {
		name      string
		messageID string
		expected  bool
	}{
		{
			name:      "empty ID",
			messageID: "",
			expected:  false,
		},
		{
			name:      "valid message ID",
			messageID: "msg_1234567890abcdef",
			expected:  true,
		},
		{
			name:      "wrong prefix",
			messageID: "message_1234567890abcdef",
			expected:  false,
		},
		{
			name:      "too short",
			messageID: "msg_123",
			expected:  false,
		},
		{
			name:      "invalid characters",
			messageID: "msg_1234567890ABCDEF",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateMessageID(tt.messageID)
			if result != tt.expected {
				t.Errorf("expected %v, got %v for ID %s", tt.expected, result, tt.messageID)
			}
		})
	}
}

func TestValidateUserID(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		expected bool
	}{
		{
			name:     "empty ID",
			userID:   "",
			expected: false,
		},
		{
			name:     "valid alphanumeric",
			userID:   "user123",
			expected: true,
		},
		{
			name:     "valid with underscore",
			userID:   "test_user_123",
			expected: true,
		},
		{
			name:     "valid with hyphen",
			userID:   "test-user-123",
			expected: true,
		},
		{
			name:     "valid with colon (IP)",
			userID:   "ip:192.168.1.1",
			expected: true,
		},
		{
			name:     "invalid characters",
			userID:   "user@domain.com",
			expected: false,
		},
		{
			name:     "too long",
			userID:   strings.Repeat("a", 101),
			expected: false,
		},
		{
			name:     "valid length at boundary",
			userID:   strings.Repeat("a", 100),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateUserID(tt.userID)
			if result != tt.expected {
				t.Errorf("expected %v, got %v for ID %s", tt.expected, result, tt.userID)
			}
		})
	}
}

func TestSanitizeUserInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal text",
			input:    "Hello world",
			expected: "Hello world",
		},
		{
			name:     "text with control characters",
			input:    "Hello\x00\x01world",
			expected: "Helloworld",
		},
		{
			name:     "text with whitespace",
			input:    "  Hello world  ",
			expected: "Hello world",
		},
		{
			name:     "very long text",
			input:    strings.Repeat("a", 10001),
			expected: strings.Repeat("a", 10000),
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeUserInput(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestSessionUtilityFunctions(t *testing.T) {
	// Create a test session
	session := &Session{
		ID:        "test_session",
		UserID:    "test_user",
		CreatedAt: time.Now().Add(-1 * time.Hour),
		ExpiresAt: time.Now().Add(30 * time.Minute),
		Messages: []Message{
			{
				ID:        "msg1",
				Role:      UserRole,
				Content:   "Hello",
				Timestamp: time.Now().Add(-30 * time.Minute),
			},
			{
				ID:        "msg2",
				Role:      AssistantRole,
				Content:   "Hi there!",
				Timestamp: time.Now().Add(-25 * time.Minute),
			},
		},
	}

	// Test IsExpired
	if IsExpired(session) {
		t.Errorf("session should not be expired")
	}

	// Test CalculateSessionAge
	age := CalculateSessionAge(session)
	if age <= 0 {
		t.Errorf("session age should be positive, got %v", age)
	}

	// Test GetLastActivity
	lastActivity := GetLastActivity(session)
	expectedTime := session.Messages[1].Timestamp
	if !lastActivity.Equal(expectedTime) {
		t.Errorf("expected last activity %v, got %v", expectedTime, lastActivity)
	}

	// Test CountTokensInMessages
	for i := range session.Messages {
		session.Messages[i].TokenCount = EstimateTokenCount(session.Messages[i].Content)
	}
	totalTokens := CountTokensInMessages(session.Messages)
	expectedTokens := EstimateTokenCount("Hello") + EstimateTokenCount("Hi there!")
	if totalTokens != expectedTokens {
		t.Errorf("expected %d tokens, got %d", expectedTokens, totalTokens)
	}

	// Test FilterMessagesByRole
	userMessages := FilterMessagesByRole(session.Messages, UserRole)
	if len(userMessages) != 1 {
		t.Errorf("expected 1 user message, got %d", len(userMessages))
	}
	if userMessages[0].Role != UserRole {
		t.Errorf("expected user role, got %s", userMessages[0].Role)
	}

	// Test GetRecentMessages
	recentMessages := GetRecentMessages(session.Messages, 1)
	if len(recentMessages) != 1 {
		t.Errorf("expected 1 recent message, got %d", len(recentMessages))
	}
	if recentMessages[0].ID != "msg2" {
		t.Errorf("expected most recent message, got %s", recentMessages[0].ID)
	}

	// Test BuildConversationContext
	context := BuildConversationContext(session.Messages)
	if !strings.Contains(context, "User") || !strings.Contains(context, "Assistant") {
		t.Errorf("context should contain role labels")
	}
	if !strings.Contains(context, "Hello") || !strings.Contains(context, "Hi there!") {
		t.Errorf("context should contain message content")
	}

	// Test GetUserQueryFromMessages
	lastQuery := GetUserQueryFromMessages(session.Messages)
	if lastQuery != "Hello" {
		t.Errorf("expected 'Hello', got %s", lastQuery)
	}
}

func TestTruncateMessages(t *testing.T) {
	messages := []Message{
		{ID: "1", Content: "Message 1", TokenCount: 5},
		{ID: "2", Content: "Message 2", TokenCount: 5},
		{ID: "3", Content: "Message 3", TokenCount: 5},
		{ID: "4", Content: "Message 4", TokenCount: 5},
	}

	// Test truncation
	truncated := TruncateMessages(messages, 12)
	if len(truncated) != 2 {
		t.Errorf("expected 2 messages after truncation, got %d", len(truncated))
	}

	// Should keep the most recent messages
	if truncated[0].ID != "3" || truncated[1].ID != "4" {
		t.Errorf("should keep most recent messages, got IDs %s and %s", truncated[0].ID, truncated[1].ID)
	}

	// Test no truncation needed
	notTruncated := TruncateMessages(messages, 25)
	if len(notTruncated) != 4 {
		t.Errorf("expected 4 messages when no truncation needed, got %d", len(notTruncated))
	}

	// Test empty messages
	emptyTruncated := TruncateMessages([]Message{}, 10)
	if len(emptyTruncated) != 0 {
		t.Errorf("expected 0 messages for empty input, got %d", len(emptyTruncated))
	}
}

func TestExtractUserIDFromContext(t *testing.T) {
	tests := []struct {
		name         string
		teamsUserID  string
		headerUserID string
		clientIP     string
		expected     string
	}{
		{
			name:         "teams user ID priority",
			teamsUserID:  "teams_user_123",
			headerUserID: "header_user_456",
			clientIP:     "192.168.1.1",
			expected:     "teams_user_123",
		},
		{
			name:         "header user ID fallback",
			teamsUserID:  "",
			headerUserID: "header_user_456",
			clientIP:     "192.168.1.1",
			expected:     "header_user_456",
		},
		{
			name:         "client IP fallback",
			teamsUserID:  "",
			headerUserID: "",
			clientIP:     "192.168.1.1",
			expected:     "ip:192.168.1.1",
		},
		{
			name:         "anonymous fallback",
			teamsUserID:  "",
			headerUserID: "",
			clientIP:     "",
			expected:     "anonymous",
		},
		{
			name:         "invalid teams user ID",
			teamsUserID:  "invalid@user",
			headerUserID: "valid_user",
			clientIP:     "192.168.1.1",
			expected:     "valid_user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractUserIDFromContext(tt.teamsUserID, tt.headerUserID, tt.clientIP)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}
