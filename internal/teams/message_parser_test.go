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

package teams

import (
	"strings"
	"testing"

	"go.uber.org/zap/zaptest"
)

func TestNewMessageParser(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewMessageParser(logger)

	if parser == nil {
		t.Fatal("Expected parser to be created, got nil")
	}

	if parser.logger == nil {
		t.Error("Expected logger to be set")
	}

	if parser.botMentionPattern == nil {
		t.Error("Expected bot mention pattern to be compiled")
	}
}

func TestParseMessage_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewMessageParser(logger)

	tests := []struct {
		name    string
		message *Message
		want    *ParsedQuery
	}{
		{
			name: "basic_direct_message",
			message: &Message{
				Type: "message",
				Text: "Generate a lift-and-shift plan for 120 VMs",
				From: &From{
					ID:   "user123",
					Name: "John Doe",
				},
				Conversation: &Conversation{
					ID:               "conv123",
					ConversationType: "personal",
					IsGroup:          false,
					TenantID:         "tenant123",
				},
				Timestamp: "2023-12-01T10:00:00Z",
				Locale:    "en-US",
			},
			want: &ParsedQuery{
				Query:           "Generate a lift-and-shift plan for 120 VMs",
				OriginalText:    "Generate a lift-and-shift plan for 120 VMs",
				IsBotMentioned:  false,
				IsDirectMessage: true,
				UserID:          "user123",
				UserName:        "John Doe",
				ConversationID:  "conv123",
				TenantID:        "tenant123",
				Timestamp:       "2023-12-01T10:00:00Z",
				Locale:          "en-US",
			},
		},
		{
			name: "bot_mention_in_channel",
			message: &Message{
				Type: "message",
				Text: "@SA-Assistant Design a hybrid architecture for Azure",
				From: &From{
					ID:   "user456",
					Name: "Jane Smith",
				},
				Conversation: &Conversation{
					ID:               "channel789",
					ConversationType: "channel",
					IsGroup:          true,
					TenantID:         "tenant456",
				},
				Timestamp: "2023-12-01T11:00:00Z",
			},
			want: &ParsedQuery{
				Query:           "Design a hybrid architecture for Azure",
				OriginalText:    "@SA-Assistant Design a hybrid architecture for Azure",
				IsBotMentioned:  true,
				IsDirectMessage: false,
				UserID:          "user456",
				UserName:        "Jane Smith",
				ConversationID:  "channel789",
				TenantID:        "tenant456",
				Timestamp:       "2023-12-01T11:00:00Z",
				Locale:          "",
			},
		},
		{
			name: "bot_mention_with_spacing",
			message: &Message{
				Type: "message",
				Text: "@ SA-Assistant   Help with DR planning",
				From: &From{
					ID:   "user789",
					Name: "Bob Wilson",
				},
				Conversation: &Conversation{
					ID:               "conv456",
					ConversationType: "groupChat",
					IsGroup:          true,
				},
			},
			want: &ParsedQuery{
				Query:           "Help with DR planning",
				OriginalText:    "@ SA-Assistant   Help with DR planning",
				IsBotMentioned:  true,
				IsDirectMessage: false,
				UserID:          "user789",
				UserName:        "Bob Wilson",
				ConversationID:  "conv456",
				TenantID:        "",
				Timestamp:       "",
				Locale:          "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.ParseMessage(tt.message)
			if err != nil {
				t.Errorf("ParseMessage() error = %v", err)
				return
			}

			if got.Query != tt.want.Query {
				t.Errorf("ParseMessage() query = %v, want %v", got.Query, tt.want.Query)
			}

			if got.IsBotMentioned != tt.want.IsBotMentioned {
				t.Errorf("ParseMessage() IsBotMentioned = %v, want %v", got.IsBotMentioned, tt.want.IsBotMentioned)
			}

			if got.IsDirectMessage != tt.want.IsDirectMessage {
				t.Errorf("ParseMessage() IsDirectMessage = %v, want %v", got.IsDirectMessage, tt.want.IsDirectMessage)
			}

			if got.UserID != tt.want.UserID {
				t.Errorf("ParseMessage() UserID = %v, want %v", got.UserID, tt.want.UserID)
			}

			if got.ConversationID != tt.want.ConversationID {
				t.Errorf("ParseMessage() ConversationID = %v, want %v", got.ConversationID, tt.want.ConversationID)
			}
		})
	}
}

func TestParseMessage_Errors(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewMessageParser(logger)

	tests := []struct {
		name    string
		message *Message
		wantErr string
	}{
		{
			name:    "nil_message",
			message: nil,
			wantErr: "message cannot be nil",
		},
		{
			name: "empty_type",
			message: &Message{
				Text: "Some query",
			},
			wantErr: "invalid message structure",
		},
		{
			name: "empty_text",
			message: &Message{
				Type: "message",
				Text: "",
			},
			wantErr: "invalid message structure",
		},
		{
			name: "unsupported_type",
			message: &Message{
				Type: "unsupported",
				Text: "Some query",
			},
			wantErr: "invalid message structure",
		},
		{
			name: "query_too_short",
			message: &Message{
				Type: "message",
				Text: "hi",
			},
			wantErr: "failed to extract query",
		},
		{
			name: "query_too_long",
			message: &Message{
				Type: "message",
				Text: strings.Repeat("a", MaxQueryLength+1),
			},
			wantErr: "failed to extract query",
		},
		{
			name: "dangerous_content_script",
			message: &Message{
				Type: "message",
				Text: "Generate plan with <script>alert('xss')</script>",
			},
			wantErr: "failed to extract query",
		},
		{
			name: "dangerous_content_javascript",
			message: &Message{
				Type: "message",
				Text: "Execute javascript:alert(document.cookie)",
			},
			wantErr: "failed to extract query",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.ParseMessage(tt.message)
			if err == nil {
				t.Errorf("ParseMessage() expected error but got none")
				return
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ParseMessage() error = %v, want error containing %v", err, tt.wantErr)
			}
		})
	}
}

func TestExtractQuery(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewMessageParser(logger)

	tests := []struct {
		name    string
		text    string
		want    string
		wantErr bool
	}{
		{
			name: "simple_query",
			text: "Generate a migration plan",
			want: "Generate a migration plan",
		},
		{
			name: "query_with_bot_mention",
			text: "@SA-Assistant Create hybrid architecture",
			want: "Create hybrid architecture",
		},
		{
			name: "query_with_html_entities",
			text: "Generate plan for &lt;100 VMs&gt;",
			want: "Generate plan for <100 VMs>",
		},
		{
			name: "query_with_extra_whitespace",
			text: "   \n\t  Generate plan  \n  ",
			want: "Generate plan",
		},
		{
			name:    "empty_text",
			text:    "",
			wantErr: true,
		},
		{
			name:    "only_whitespace",
			text:    "   \n\t   ",
			wantErr: true,
		},
		{
			name:    "only_bot_mention",
			text:    "@SA-Assistant",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.extractQuery(tt.text)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractQuery() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && got != tt.want {
				t.Errorf("extractQuery() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsBotMentioned(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewMessageParser(logger)

	tests := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "exact_mention",
			text: "@SA-Assistant generate plan",
			want: true,
		},
		{
			name: "mention_with_spaces",
			text: "@ SA-Assistant generate plan",
			want: true,
		},
		{
			name: "mention_at_end",
			text: "generate plan @SA-Assistant",
			want: true,
		},
		{
			name: "case_sensitive_no_match",
			text: "@sa-assistant generate plan",
			want: false,
		},
		{
			name: "partial_mention",
			text: "@SA generate plan",
			want: false,
		},
		{
			name: "no_mention",
			text: "generate plan",
			want: false,
		},
		{
			name: "mention_in_middle",
			text: "please @SA-Assistant help me",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parser.isBotMentioned(tt.text)
			if got != tt.want {
				t.Errorf("isBotMentioned() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsDirectMessage(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewMessageParser(logger)

	tests := []struct {
		name    string
		message *Message
		want    bool
	}{
		{
			name: "personal_conversation",
			message: &Message{
				Conversation: &Conversation{
					ConversationType: "personal",
					IsGroup:          false,
				},
			},
			want: true,
		},
		{
			name: "non_group_conversation",
			message: &Message{
				Conversation: &Conversation{
					ConversationType: "chat",
					IsGroup:          false,
				},
			},
			want: true,
		},
		{
			name: "channel_conversation",
			message: &Message{
				Conversation: &Conversation{
					ConversationType: "channel",
					IsGroup:          true,
				},
			},
			want: false,
		},
		{
			name: "group_chat",
			message: &Message{
				Conversation: &Conversation{
					ConversationType: "groupChat",
					IsGroup:          true,
				},
			},
			want: false,
		},
		{
			name: "nil_conversation",
			message: &Message{
				Conversation: nil,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parser.isDirectMessage(tt.message)
			if got != tt.want {
				t.Errorf("isDirectMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldProcessMessage(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewMessageParser(logger)

	tests := []struct {
		name        string
		parsedQuery *ParsedQuery
		want        bool
	}{
		{
			name: "direct_message",
			parsedQuery: &ParsedQuery{
				Query:           "Generate plan",
				IsDirectMessage: true,
				IsBotMentioned:  false,
			},
			want: true,
		},
		{
			name: "bot_mentioned",
			parsedQuery: &ParsedQuery{
				Query:           "Generate plan",
				IsDirectMessage: false,
				IsBotMentioned:  true,
			},
			want: true,
		},
		{
			name: "both_direct_and_mentioned",
			parsedQuery: &ParsedQuery{
				Query:           "Generate plan",
				IsDirectMessage: true,
				IsBotMentioned:  true,
			},
			want: true,
		},
		{
			name: "neither_direct_nor_mentioned",
			parsedQuery: &ParsedQuery{
				Query:           "Generate plan",
				IsDirectMessage: false,
				IsBotMentioned:  false,
			},
			want: false,
		},
		{
			name:        "nil_query",
			parsedQuery: nil,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parser.ShouldProcessMessage(tt.parsedQuery)
			if got != tt.want {
				t.Errorf("ShouldProcessMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateQuerySafety(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewMessageParser(logger)

	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{
			name:    "safe_query",
			query:   "Generate a migration plan for 100 VMs",
			wantErr: false,
		},
		{
			name:    "query_with_numbers",
			query:   "Plan for 120 VMs with 16GB RAM each",
			wantErr: false,
		},
		{
			name:    "script_injection",
			query:   "Generate plan <script>alert('xss')</script>",
			wantErr: true,
		},
		{
			name:    "javascript_url",
			query:   "Click javascript:alert(document.cookie)",
			wantErr: true,
		},
		{
			name:    "vbscript_injection",
			query:   "Run vbscript:msgbox('test')",
			wantErr: true,
		},
		{
			name:    "data_url_injection",
			query:   "Load data:text/html,<script>alert(1)</script>",
			wantErr: true,
		},
		{
			name:    "event_handler",
			query:   "Image with onerror=alert(1)",
			wantErr: true,
		},
		{
			name:    "eval_function",
			query:   "Execute eval(maliciousCode)",
			wantErr: true,
		},
		{
			name:    "case_insensitive_script",
			query:   "Generate plan <SCRIPT>alert('test')</SCRIPT>",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.validateQuerySafety(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateQuerySafety() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSanitizeText(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewMessageParser(logger)

	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "normal_text",
			text: "Generate migration plan",
			want: "Generate migration plan",
		},
		{
			name: "multiple_spaces",
			text: "Generate    migration     plan",
			want: "Generate migration plan",
		},
		{
			name: "tabs_and_newlines",
			text: "Generate\t\nmigration\n\nplan",
			want: "Generate migration plan",
		},
		{
			name: "leading_trailing_whitespace",
			text: "  \n\tGenerate migration plan\t\n  ",
			want: "Generate migration plan",
		},
		{
			name: "control_characters",
			text: "Generate\x00\x01migration\x7Fplan",
			want: "Generate migration plan",
		},
		{
			name: "mixed_whitespace_and_control",
			text: "\x01  Generate\t\x02migration\n\x03plan  \x7F",
			want: "Generate migration plan",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parser.sanitizeText(tt.text)
			if got != tt.want {
				t.Errorf("sanitizeText() = %q, want %q", got, tt.want)
			}
		})
	}
}
