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

// Package teams provides Teams message parsing and validation functionality
// for the AI SA Assistant Teams bot integration.
package teams

import (
	"fmt"
	"html"
	"regexp"
	"strings"

	"go.uber.org/zap"
)

const (
	// MaxQueryLength defines the maximum allowed query length
	MaxQueryLength = 4000
	// MinQueryLength defines the minimum allowed query length
	MinQueryLength = 3
	// BotMentionPattern is the pattern for detecting bot mentions
	BotMentionPattern = `@\s*SA-Assistant\s*`
)

// Message represents a Teams webhook message payload
type Message struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text"`
	TextFormat   string                 `json:"textFormat,omitempty"`
	Timestamp    string                 `json:"timestamp,omitempty"`
	Locale       string                 `json:"locale,omitempty"`
	Recipient    *Recipient             `json:"recipient,omitempty"`
	From         *From                  `json:"from,omitempty"`
	Conversation *Conversation          `json:"conversation,omitempty"`
	Entities     []interface{}          `json:"entities,omitempty"`
	ChannelData  map[string]interface{} `json:"channelData,omitempty"`
}

// Recipient represents the Teams message recipient
type Recipient struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// From represents the Teams message sender
type From struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Conversation represents the Teams conversation context
type Conversation struct {
	ID               string `json:"id"`
	Name             string `json:"name,omitempty"`
	IsGroup          bool   `json:"isGroup,omitempty"`
	TenantID         string `json:"tenantId,omitempty"`
	ConversationType string `json:"conversationType,omitempty"`
}

// ParsedQuery represents a parsed and validated Teams query
type ParsedQuery struct {
	Query           string
	OriginalText    string
	IsBotMentioned  bool
	IsDirectMessage bool
	UserID          string
	UserName        string
	ConversationID  string
	TenantID        string
	Timestamp       string
	Locale          string
}

// MessageParser handles Teams message parsing and validation
type MessageParser struct {
	logger            *zap.Logger
	botMentionPattern *regexp.Regexp
}

// NewMessageParser creates a new Teams message parser
func NewMessageParser(logger *zap.Logger) *MessageParser {
	botMentionRegex := regexp.MustCompile(BotMentionPattern)

	return &MessageParser{
		logger:            logger,
		botMentionPattern: botMentionRegex,
	}
}

// ParseMessage parses and validates a Teams webhook message
func (mp *MessageParser) ParseMessage(message *Message) (*ParsedQuery, error) {
	if message == nil {
		return nil, fmt.Errorf("message cannot be nil")
	}

	// Validate basic message structure
	if err := mp.validateMessageStructure(message); err != nil {
		return nil, fmt.Errorf("invalid message structure: %w", err)
	}

	// Extract and clean the query text
	query, err := mp.extractQuery(message.Text)
	if err != nil {
		return nil, fmt.Errorf("failed to extract query: %w", err)
	}

	// Determine if this is a bot mention or direct message
	isBotMentioned := mp.isBotMentioned(message.Text)
	isDirectMessage := mp.isDirectMessage(message)

	// Extract user and conversation information
	userID, userName := mp.extractUserInfo(message.From)
	conversationID := mp.extractConversationID(message.Conversation)
	tenantID := mp.extractTenantID(message.Conversation)

	mp.logger.Info("Parsed Teams message",
		zap.String("query", query),
		zap.Bool("bot_mentioned", isBotMentioned),
		zap.Bool("direct_message", isDirectMessage),
		zap.String("user_id", userID),
		zap.String("conversation_id", conversationID))

	return &ParsedQuery{
		Query:           query,
		OriginalText:    message.Text,
		IsBotMentioned:  isBotMentioned,
		IsDirectMessage: isDirectMessage,
		UserID:          userID,
		UserName:        userName,
		ConversationID:  conversationID,
		TenantID:        tenantID,
		Timestamp:       message.Timestamp,
		Locale:          message.Locale,
	}, nil
}

// validateMessageStructure validates the basic structure of a Teams message
func (mp *MessageParser) validateMessageStructure(message *Message) error {
	if message.Type == "" {
		return fmt.Errorf("message type is required")
	}

	if message.Text == "" {
		return fmt.Errorf("message text is required")
	}

	// Check for supported message types
	supportedTypes := []string{"message", "invoke", "event"}
	isSupported := false
	for _, supportedType := range supportedTypes {
		if message.Type == supportedType {
			isSupported = true
			break
		}
	}

	if !isSupported {
		return fmt.Errorf("unsupported message type: %s", message.Type)
	}

	return nil
}

// extractQuery extracts and sanitizes the user query from message text
func (mp *MessageParser) extractQuery(text string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("empty message text")
	}

	// Decode HTML entities
	query := html.UnescapeString(text)

	// Remove bot mentions
	query = mp.botMentionPattern.ReplaceAllString(query, "")

	// Clean up whitespace and newlines
	query = mp.sanitizeText(query)

	// Validate query length
	if len(query) < MinQueryLength {
		return "", fmt.Errorf("query too short (minimum %d characters)", MinQueryLength)
	}

	if len(query) > MaxQueryLength {
		return "", fmt.Errorf("query too long (maximum %d characters)", MaxQueryLength)
	}

	// Basic input validation to prevent common injection patterns
	if err := mp.validateQuerySafety(query); err != nil {
		return "", fmt.Errorf("unsafe query content: %w", err)
	}

	return query, nil
}

// sanitizeText cleans and normalizes text content
func (mp *MessageParser) sanitizeText(text string) string {
	// Remove excessive whitespace and normalize line endings
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)

	// Remove control characters except common ones
	text = regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]`).ReplaceAllString(text, "")

	return text
}

// validateQuerySafety performs basic validation to prevent injection attacks
func (mp *MessageParser) validateQuerySafety(query string) error {
	// Check for potential script injection patterns
	dangerousPatterns := []string{
		`<script`,
		`javascript:`,
		`vbscript:`,
		`data:text/html`,
		`onerror=`,
		`onload=`,
		`onclick=`,
		`eval\(`,
		`expression\(`,
	}

	queryLower := strings.ToLower(query)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(queryLower, pattern) {
			mp.logger.Warn("Potentially dangerous pattern detected in query",
				zap.String("pattern", pattern),
				zap.String("query", query))
			return fmt.Errorf("query contains potentially dangerous content")
		}
	}

	return nil
}

// isBotMentioned checks if the bot was mentioned in the message
func (mp *MessageParser) isBotMentioned(text string) bool {
	return mp.botMentionPattern.MatchString(text)
}

// isDirectMessage determines if this is a direct message to the bot
func (mp *MessageParser) isDirectMessage(message *Message) bool {
	if message.Conversation == nil {
		return false
	}

	// Direct messages typically have conversationType as "personal"
	// and are not group conversations
	return message.Conversation.ConversationType == "personal" ||
		(!message.Conversation.IsGroup && message.Conversation.ConversationType != "channel")
}

// extractUserInfo extracts user ID and name from the message sender
func (mp *MessageParser) extractUserInfo(from *From) (string, string) {
	if from == nil {
		return "", ""
	}
	return from.ID, from.Name
}

// extractConversationID extracts the conversation ID
func (mp *MessageParser) extractConversationID(conversation *Conversation) string {
	if conversation == nil {
		return ""
	}
	return conversation.ID
}

// extractTenantID extracts the tenant ID from conversation
func (mp *MessageParser) extractTenantID(conversation *Conversation) string {
	if conversation == nil {
		return ""
	}
	return conversation.TenantID
}

// ShouldProcessMessage determines if the message should be processed by the bot
func (mp *MessageParser) ShouldProcessMessage(parsedQuery *ParsedQuery) bool {
	if parsedQuery == nil {
		return false
	}

	// Process direct messages or messages where the bot is mentioned
	shouldProcess := parsedQuery.IsDirectMessage || parsedQuery.IsBotMentioned

	mp.logger.Debug("Message processing decision",
		zap.Bool("should_process", shouldProcess),
		zap.Bool("is_direct_message", parsedQuery.IsDirectMessage),
		zap.Bool("bot_mentioned", parsedQuery.IsBotMentioned),
		zap.String("query", parsedQuery.Query))

	return shouldProcess
}
