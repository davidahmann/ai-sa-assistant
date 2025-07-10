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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// GenerateSessionID generates a unique session identifier
func GenerateSessionID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails
		return fmt.Sprintf("session_%d", time.Now().UnixNano())
	}
	return "session_" + hex.EncodeToString(bytes)
}

// GenerateMessageID generates a unique message identifier
func GenerateMessageID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails
		return fmt.Sprintf("msg_%d", time.Now().UnixNano())
	}
	return "msg_" + hex.EncodeToString(bytes)
}

const (
	// DefaultConversationTitle is used when no content is available for title generation
	DefaultConversationTitle = "New Conversation"
)

// GenerateTitle generates a conversation title from the first user message
func GenerateTitle(content string) string {
	// Clean up the content
	content = strings.TrimSpace(content)
	if content == "" {
		return DefaultConversationTitle
	}

	// Remove bot mentions and common prefixes
	content = removeBotMentions(content)
	content = strings.TrimSpace(content)

	// Truncate to reasonable length
	const maxTitleLength = 60
	if utf8.RuneCountInString(content) > maxTitleLength {
		runes := []rune(content)
		content = string(runes[:maxTitleLength]) + "..."
	}

	// Ensure it starts with a capital letter
	if len(content) > 0 {
		runes := []rune(content)
		if len(runes) > 0 && runes[0] >= 'a' && runes[0] <= 'z' {
			runes[0] = runes[0] - 'a' + 'A'
			content = string(runes)
		}
	}

	// Remove multiple spaces
	content = regexp.MustCompile(`\s+`).ReplaceAllString(content, " ")

	if content == "" {
		return DefaultConversationTitle
	}

	return content
}

// removeBotMentions removes common bot mention patterns from text
func removeBotMentions(text string) string {
	// Common bot mention patterns
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)@[\w-]*(?:sa|assistant|bot)[\w-]*\s*`),
		regexp.MustCompile(`(?i)hey\s+(?:sa|assistant|bot)[\w-]*\s*`),
		regexp.MustCompile(`(?i)hi\s+(?:sa|assistant|bot)[\w-]*\s*`),
		regexp.MustCompile(`(?i)hello\s+(?:sa|assistant|bot)[\w-]*\s*`),
	}

	for _, pattern := range patterns {
		text = pattern.ReplaceAllString(text, "")
	}

	return strings.TrimSpace(text)
}

// EstimateTokenCount provides a rough estimate of token count (4 characters ≈ 1 token)
func EstimateTokenCount(text string) int {
	const tokenEstimateRatio = 4
	return utf8.RuneCountInString(text) / tokenEstimateRatio
}

// ValidateSessionID validates a session ID format
func ValidateSessionID(sessionID string) bool {
	if sessionID == "" {
		return false
	}

	// Check for valid session ID format
	matched, err := regexp.MatchString(`^session_[a-f0-9]{32}$`, sessionID)
	if err != nil {
		return false
	}
	return matched
}

// ValidateMessageID validates a message ID format
func ValidateMessageID(messageID string) bool {
	if messageID == "" {
		return false
	}

	// Check for valid message ID format
	matched, err := regexp.MatchString(`^msg_[a-f0-9]{16}$`, messageID)
	if err != nil {
		return false
	}
	return matched
}

// SanitizeUserInput sanitizes user input for safe storage
func SanitizeUserInput(input string) string {
	// Remove control characters
	input = regexp.MustCompile(`[\x00-\x1F\x7F]`).ReplaceAllString(input, "")

	// Limit length
	const maxInputLength = 10000
	if utf8.RuneCountInString(input) > maxInputLength {
		runes := []rune(input)
		input = string(runes[:maxInputLength])
	}

	return strings.TrimSpace(input)
}

// FormatTimestamp formats a timestamp for display
func FormatTimestamp(t time.Time) string {
	return t.Format("2006-01-02 15:04:05 UTC")
}

// ParseTimestamp parses a timestamp string
func ParseTimestamp(s string) (time.Time, error) {
	return time.Parse("2006-01-02 15:04:05 UTC", s)
}

// IsExpired checks if a session is expired
func IsExpired(session *Session) bool {
	return session.ExpiresAt.Before(time.Now())
}

// CalculateSessionAge calculates the age of a session
func CalculateSessionAge(session *Session) time.Duration {
	return time.Since(session.CreatedAt)
}

// GetLastActivity returns the timestamp of the last activity in a session
func GetLastActivity(session *Session) time.Time {
	if len(session.Messages) == 0 {
		return session.CreatedAt
	}

	lastMessage := session.Messages[len(session.Messages)-1]
	return lastMessage.Timestamp
}

// TruncateMessages truncates messages to fit within a token limit
func TruncateMessages(messages []Message, maxTokens int) []Message {
	if len(messages) == 0 {
		return messages
	}

	totalTokens := 0
	for _, msg := range messages {
		totalTokens += msg.TokenCount
	}

	if totalTokens <= maxTokens {
		return messages
	}

	// Start from the end and work backwards
	var result []Message
	currentTokens := 0

	for i := len(messages) - 1; i >= 0; i-- {
		if currentTokens+messages[i].TokenCount > maxTokens {
			break
		}
		result = append([]Message{messages[i]}, result...)
		currentTokens += messages[i].TokenCount
	}

	return result
}

// GetRecentMessages returns the N most recent messages
func GetRecentMessages(messages []Message, count int) []Message {
	if len(messages) <= count {
		return messages
	}
	return messages[len(messages)-count:]
}

// CountTokensInMessages calculates total tokens in a slice of messages
func CountTokensInMessages(messages []Message) int {
	total := 0
	for _, msg := range messages {
		total += msg.TokenCount
	}
	return total
}

// FilterMessagesByRole filters messages by role
func FilterMessagesByRole(messages []Message, role MessageRole) []Message {
	var filtered []Message
	for _, msg := range messages {
		if msg.Role == role {
			filtered = append(filtered, msg)
		}
	}
	return filtered
}

// BuildConversationContext builds context string from messages for LLM prompts
func BuildConversationContext(messages []Message) string {
	if len(messages) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("## Previous Conversation Context\n\n")

	for _, msg := range messages {
		roleStr := toTitle(string(msg.Role))
		builder.WriteString(fmt.Sprintf("**%s**: %s\n\n", roleStr, msg.Content))
	}

	return builder.String()
}

// toTitle converts the first character of a string to uppercase (replacement for deprecated strings.Title)
func toTitle(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

// GetUserQueryFromMessages extracts the most recent user query from messages
func GetUserQueryFromMessages(messages []Message) string {
	// Find the most recent user message
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == UserRole {
			return messages[i].Content
		}
	}
	return ""
}

// ValidateUserID validates a user ID format
func ValidateUserID(userID string) bool {
	if userID == "" {
		return false
	}

	// Allow alphanumeric, hyphens, underscores, colons, and periods (for IP addresses)
	matched, err := regexp.MatchString(`^[a-zA-Z0-9_:.-]+$`, userID)
	if err != nil {
		return false
	}

	// Check length limits
	return matched && len(userID) >= 1 && len(userID) <= 100
}

// ExtractUserIDFromContext extracts user ID from various context sources
func ExtractUserIDFromContext(teamsUserID, headerUserID, clientIP string) string {
	// Priority order: Teams user ID, header user ID, client IP
	if teamsUserID != "" && ValidateUserID(teamsUserID) {
		return teamsUserID
	}

	if headerUserID != "" && ValidateUserID(headerUserID) {
		return headerUserID
	}

	if clientIP != "" {
		return fmt.Sprintf("ip:%s", clientIP)
	}

	return "anonymous"
}
