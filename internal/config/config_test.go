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

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
openai:
  apikey: "sk-test-key"  # pragma: allowlist secret
  endpoint: "https://api.openai.com/v1"
teams:
  webhook_url: "https://test.webhook.com/test"  # pragma: allowlist secret
services:
  retrieve_url: "http://retrieve:8081"
  websearch_url: "http://websearch:8083"
  synthesize_url: "http://synthesize:8082"
chroma:
  url: "http://chromadb:8000"
  collection_name: "test_collection"
metadata:
  db_path: "./test_metadata.db"
retrieval:
  max_chunks: 10
  fallback_threshold: 5
  confidence_threshold: 0.8
  fallback_score_threshold: 0.6
websearch:
  max_results: 5
  freshness_keywords: ["latest", "recent"]
synthesis:
  model: "gpt-4o"
  max_tokens: 1000
  temperature: 0.2
logging:
  level: "debug"
  format: "json"
  output: "stdout"
feedback:
  storage_type: "file"
  file_path: "./test_feedback.log"
  db_path: "./test_feedback.db"
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	config, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test basic configuration loading
	if config.OpenAI.APIKey != "sk-test-key" {
		t.Errorf("Expected OpenAI API key 'sk-test-key', got '%s'", config.OpenAI.APIKey)
	}

	if config.Teams.WebhookURL != "https://test.webhook.com/test" {
		t.Errorf("Expected Teams webhook URL 'https://test.webhook.com/test', got '%s'", config.Teams.WebhookURL)
	}

	if config.Retrieval.MaxChunks != 10 {
		t.Errorf("Expected retrieval max_chunks 10, got %d", config.Retrieval.MaxChunks)
	}

	if config.Synthesis.Temperature != 0.2 {
		t.Errorf("Expected synthesis temperature 0.2, got %f", config.Synthesis.Temperature)
	}
}

func TestEnvironmentVariableOverrides(t *testing.T) {
	// Create temporary config file with default values
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
openai:
  apikey: "sk-default-key"
teams:
  webhook_url: "https://default.webhook.com/test"
chroma:
  url: "http://default:8000"
metadata:
  db_path: "./default_metadata.db"
logging:
  level: "info"
  format: "json"
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	// Set environment variables
	_ = os.Setenv("OPENAI_API_KEY", "sk-env-key")
	_ = os.Setenv("TEAMS_WEBHOOK_URL", "https://env.webhook.com/test")
	_ = os.Setenv("CHROMA_URL", "http://env:8000")
	_ = os.Setenv("LOG_LEVEL", "debug")
	_ = os.Setenv("LOG_FORMAT", "text")

	defer func() {
		_ = os.Unsetenv("OPENAI_API_KEY")
		_ = os.Unsetenv("TEAMS_WEBHOOK_URL")
		_ = os.Unsetenv("CHROMA_URL")
		_ = os.Unsetenv("LOG_LEVEL")
		_ = os.Unsetenv("LOG_FORMAT")
	}()

	config, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test environment variable overrides
	if config.OpenAI.APIKey != "sk-env-key" {
		t.Errorf("Expected OpenAI API key from env 'sk-env-key', got '%s'", config.OpenAI.APIKey)
	}

	if config.Teams.WebhookURL != "https://env.webhook.com/test" {
		t.Errorf("Expected Teams webhook URL from env 'https://env.webhook.com/test', got '%s'", config.Teams.WebhookURL)
	}

	if config.Chroma.URL != "http://env:8000" {
		t.Errorf("Expected Chroma URL from env 'http://env:8000', got '%s'", config.Chroma.URL)
	}

	if config.Logging.Level != "debug" {
		t.Errorf("Expected log level from env 'debug', got '%s'", config.Logging.Level)
	}

	if config.Logging.Format != "text" {
		t.Errorf("Expected log format from env 'text', got '%s'", config.Logging.Format)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name          string
		config        Config
		expectedError bool
		errorContains string
	}{
		{
			name: "Valid configuration",
			config: Config{
				OpenAI: OpenAIConfig{
					APIKey:   "sk-test-key",
					Endpoint: "https://api.openai.com/v1",
				},
				Teams: TeamsConfig{
					WebhookURL: "https://test.webhook.com/test",
				},
				Chroma: ChromaConfig{
					URL:            "http://chromadb:8000",
					CollectionName: "test_collection",
				},
				Metadata: MetadataConfig{
					DBPath: "./test_metadata.db",
				},
				Retrieval: RetrievalConfig{
					MaxChunks:              5,
					FallbackThreshold:      3,
					ConfidenceThreshold:    0.7,
					FallbackScoreThreshold: 0.7,
				},
				WebSearch: WebSearchConfig{
					MaxResults: 3,
				},
				Synthesis: SynthesisConfig{
					Model:       "gpt-4o",
					MaxTokens:   2000,
					Temperature: 0.3,
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
					Output: "stdout",
				},
				Feedback: FeedbackConfig{
					StorageType: "file",
					FilePath:    "./feedback.log",
				},
			},
			expectedError: false,
		},
		{
			name: "Missing OpenAI API key",
			config: Config{
				OpenAI: OpenAIConfig{
					APIKey: "",
				},
				Teams: TeamsConfig{
					WebhookURL: "https://test.webhook.com/test",
				},
				Chroma: ChromaConfig{
					URL: "http://chromadb:8000",
				},
				Metadata: MetadataConfig{
					DBPath: "./test_metadata.db",
				},
				Retrieval: RetrievalConfig{
					MaxChunks:              5,
					FallbackThreshold:      3,
					ConfidenceThreshold:    0.7,
					FallbackScoreThreshold: 0.7,
				},
				WebSearch: WebSearchConfig{
					MaxResults: 3,
				},
				Synthesis: SynthesisConfig{
					MaxTokens:   2000,
					Temperature: 0.3,
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
				},
				Feedback: FeedbackConfig{
					StorageType: "file",
				},
			},
			expectedError: true,
			errorContains: "OpenAI API key is required",
		},
		{
			name: "Missing Teams webhook URL",
			config: Config{
				OpenAI: OpenAIConfig{
					APIKey: "sk-test-key",
				},
				Teams: TeamsConfig{
					WebhookURL: "",
				},
				Chroma: ChromaConfig{
					URL: "http://chromadb:8000",
				},
				Metadata: MetadataConfig{
					DBPath: "./test_metadata.db",
				},
				Retrieval: RetrievalConfig{
					MaxChunks:              5,
					FallbackThreshold:      3,
					ConfidenceThreshold:    0.7,
					FallbackScoreThreshold: 0.7,
				},
				WebSearch: WebSearchConfig{
					MaxResults: 3,
				},
				Synthesis: SynthesisConfig{
					MaxTokens:   2000,
					Temperature: 0.3,
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
				},
				Feedback: FeedbackConfig{
					StorageType: "file",
				},
			},
			expectedError: true,
			errorContains: "Teams webhook URL is required",
		},
		{
			name: "Invalid max_chunks",
			config: Config{
				OpenAI: OpenAIConfig{
					APIKey: "sk-test-key",
				},
				Teams: TeamsConfig{
					WebhookURL: "https://test.webhook.com/test",
				},
				Chroma: ChromaConfig{
					URL: "http://chromadb:8000",
				},
				Metadata: MetadataConfig{
					DBPath: "./test_metadata.db",
				},
				Retrieval: RetrievalConfig{
					MaxChunks: 0,
				},
				WebSearch: WebSearchConfig{
					MaxResults: 3,
				},
				Synthesis: SynthesisConfig{
					MaxTokens:   2000,
					Temperature: 0.3,
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
				},
				Feedback: FeedbackConfig{
					StorageType: "file",
				},
			},
			expectedError: true,
			errorContains: "max_chunks must be greater than 0",
		},
		{
			name: "Invalid confidence threshold",
			config: Config{
				OpenAI: OpenAIConfig{
					APIKey: "sk-test-key",
				},
				Teams: TeamsConfig{
					WebhookURL: "https://test.webhook.com/test",
				},
				Chroma: ChromaConfig{
					URL: "http://chromadb:8000",
				},
				Metadata: MetadataConfig{
					DBPath: "./test_metadata.db",
				},
				Retrieval: RetrievalConfig{
					MaxChunks:           5,
					FallbackThreshold:   3,
					ConfidenceThreshold: 1.5,
				},
				WebSearch: WebSearchConfig{
					MaxResults: 3,
				},
				Synthesis: SynthesisConfig{
					MaxTokens:   2000,
					Temperature: 0.3,
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
				},
				Feedback: FeedbackConfig{
					StorageType: "file",
				},
			},
			expectedError: true,
			errorContains: "confidence_threshold must be between 0 and 1",
		},
		{
			name: "Invalid log level",
			config: Config{
				OpenAI: OpenAIConfig{
					APIKey: "sk-test-key",
				},
				Teams: TeamsConfig{
					WebhookURL: "https://test.webhook.com/test",
				},
				Chroma: ChromaConfig{
					URL: "http://chromadb:8000",
				},
				Metadata: MetadataConfig{
					DBPath: "./test_metadata.db",
				},
				Retrieval: RetrievalConfig{
					MaxChunks:              5,
					FallbackThreshold:      3,
					ConfidenceThreshold:    0.7,
					FallbackScoreThreshold: 0.7,
				},
				WebSearch: WebSearchConfig{
					MaxResults: 3,
				},
				Synthesis: SynthesisConfig{
					MaxTokens:   2000,
					Temperature: 0.3,
				},
				Logging: LoggingConfig{
					Level:  "invalid",
					Format: "json",
				},
				Feedback: FeedbackConfig{
					StorageType: "file",
				},
			},
			expectedError: true,
			errorContains: "log level must be one of",
		},
		{
			name: "Invalid synthesis temperature",
			config: Config{
				OpenAI: OpenAIConfig{
					APIKey: "sk-test-key",
				},
				Teams: TeamsConfig{
					WebhookURL: "https://test.webhook.com/test",
				},
				Chroma: ChromaConfig{
					URL: "http://chromadb:8000",
				},
				Metadata: MetadataConfig{
					DBPath: "./test_metadata.db",
				},
				Retrieval: RetrievalConfig{
					MaxChunks:              5,
					FallbackThreshold:      3,
					ConfidenceThreshold:    0.7,
					FallbackScoreThreshold: 0.7,
				},
				WebSearch: WebSearchConfig{
					MaxResults: 3,
				},
				Synthesis: SynthesisConfig{
					MaxTokens:   2000,
					Temperature: 3.0,
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
				},
				Feedback: FeedbackConfig{
					StorageType: "file",
				},
			},
			expectedError: true,
			errorContains: "temperature must be between 0 and 2",
		},
		{
			name: "Invalid fallback score threshold",
			config: Config{
				OpenAI: OpenAIConfig{
					APIKey: "sk-test-key",
				},
				Teams: TeamsConfig{
					WebhookURL: "https://test.webhook.com/test",
				},
				Chroma: ChromaConfig{
					URL: "http://chromadb:8000",
				},
				Metadata: MetadataConfig{
					DBPath: "./test_metadata.db",
				},
				Retrieval: RetrievalConfig{
					MaxChunks:              5,
					FallbackThreshold:      3,
					ConfidenceThreshold:    0.7,
					FallbackScoreThreshold: 1.5,
				},
				WebSearch: WebSearchConfig{
					MaxResults: 3,
				},
				Synthesis: SynthesisConfig{
					MaxTokens:   2000,
					Temperature: 0.3,
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
				},
				Feedback: FeedbackConfig{
					StorageType: "file",
				},
			},
			expectedError: true,
			errorContains: "fallback_score_threshold must be between 0 and 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(&tt.config)

			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected validation error, but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain '%s', but got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no validation error, but got: %v", err)
				}
			}
		})
	}
}

func TestMaskSensitiveValues(t *testing.T) {
	config := &Config{
		OpenAI: OpenAIConfig{
			APIKey: "sk-test-1234567890abcdef", // pragma: allowlist secret
		},
		Teams: TeamsConfig{
			WebhookURL: "https://test.webhook.com/secret-token-123456789", // pragma: allowlist secret
		},
	}

	masked := config.MaskSensitiveValues()

	// Original config should remain unchanged
	if config.OpenAI.APIKey != "sk-test-1234567890abcdef" {
		t.Errorf("Original config API key should remain unchanged")
	}

	// Masked config should have sensitive values masked
	expectedAPIKey := "sk-test-" + "****************"
	if masked.OpenAI.APIKey != expectedAPIKey {
		t.Errorf("Expected masked API key '%s', got '%s'", expectedAPIKey, masked.OpenAI.APIKey)
	}

	// Calculate expected masked webhook URL based on actual length
	webhookURL := "https://test.webhook.com/secret-token-123456789"
	expectedWebhookURL := webhookURL[:8] + strings.Repeat("*", len(webhookURL)-8)
	if masked.Teams.WebhookURL != expectedWebhookURL {
		t.Errorf("Expected masked webhook URL '%s', got '%s'", expectedWebhookURL, masked.Teams.WebhookURL)
	}
}

func TestConfigPathEnvironmentVariable(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "custom_config.yaml")

	configContent := `
openai:
  apikey: "sk-custom-key"
teams:
  webhook_url: "https://custom.webhook.com/test"
chroma:
  url: "http://chromadb:8000"
metadata:
  db_path: "./metadata.db"
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	// Set CONFIG_PATH environment variable
	_ = os.Setenv("CONFIG_PATH", configPath)
	defer func() {
		_ = os.Unsetenv("CONFIG_PATH")
	}()

	config, err := Load("")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if config.OpenAI.APIKey != "sk-custom-key" {
		t.Errorf("Expected OpenAI API key from custom config 'sk-custom-key', got '%s'", config.OpenAI.APIKey)
	}
}

func TestLoadWithOptions(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
openai:
  apikey: "sk-test-key"  # pragma: allowlist secret
teams:
  webhook_url: "https://test.webhook.com/test"  # pragma: allowlist secret
chroma:
  url: "http://chromadb:8000"
metadata:
  db_path: "./metadata.db"
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	// Test with validation disabled
	config, err := LoadWithOptions(LoadOptions{
		ConfigPath:       configPath,
		ValidateRequired: false,
	})
	if err != nil {
		t.Fatalf("Failed to load config with options: %v", err)
	}

	if config.OpenAI.APIKey != "sk-test-key" {
		t.Errorf("Expected OpenAI API key 'sk-test-key', got '%s'", config.OpenAI.APIKey)
	}

	// Test with validation enabled and missing required field
	configContentInvalid := `
openai:
  apikey: ""
teams:
  webhook_url: "https://test.webhook.com/test"  # pragma: allowlist secret
chroma:
  url: "http://chromadb:8000"
metadata:
  db_path: "./metadata.db"
`

	configPathInvalid := filepath.Join(tmpDir, "config_invalid.yaml")
	err = os.WriteFile(configPathInvalid, []byte(configContentInvalid), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	_, err = LoadWithOptions(LoadOptions{
		ConfigPath:       configPathInvalid,
		ValidateRequired: true,
	})
	if err == nil {
		t.Error("Expected validation error for missing API key, but got none")
	}
}

func TestDefaultValues(t *testing.T) {
	// Create temporary config file with minimal required fields
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
openai:
  apikey: "sk-test-key"  # pragma: allowlist secret
teams:
  webhook_url: "https://test.webhook.com/test"  # pragma: allowlist secret
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	config, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test default values
	if config.OpenAI.Endpoint != "https://api.openai.com/v1" {
		t.Errorf("Expected default OpenAI endpoint 'https://api.openai.com/v1', got '%s'", config.OpenAI.Endpoint)
	}

	if config.Chroma.URL != "http://chromadb:8000" {
		t.Errorf("Expected default Chroma URL 'http://chromadb:8000', got '%s'", config.Chroma.URL)
	}

	if config.Chroma.CollectionName != "cloud_assistant" {
		t.Errorf("Expected default collection name 'cloud_assistant', got '%s'", config.Chroma.CollectionName)
	}

	if config.Retrieval.MaxChunks != 5 {
		t.Errorf("Expected default max_chunks 5, got %d", config.Retrieval.MaxChunks)
	}

	if config.Synthesis.Model != "gpt-4o" {
		t.Errorf("Expected default model 'gpt-4o', got '%s'", config.Synthesis.Model)
	}

	if config.Logging.Level != "info" {
		t.Errorf("Expected default log level 'info', got '%s'", config.Logging.Level)
	}
}

func TestGetEnvironment(t *testing.T) {
	// Test default environment
	env := getEnvironment()
	if env != "development" {
		t.Errorf("Expected default environment 'development', got '%s'", env)
	}

	// Test ENVIRONMENT variable
	_ = os.Setenv("ENVIRONMENT", "production")
	env = getEnvironment()
	if env != "production" {
		t.Errorf("Expected environment 'production', got '%s'", env)
	}
	_ = os.Unsetenv("ENVIRONMENT")

	// Test ENV variable
	_ = os.Setenv("ENV", "staging")
	env = getEnvironment()
	if env != "staging" {
		t.Errorf("Expected environment 'staging', got '%s'", env)
	}
	_ = os.Unsetenv("ENV")
}

func TestValidationError(t *testing.T) {
	err := ValidationError{
		Field:   "test.field",
		Message: "test error message",
	}

	expected := "configuration validation failed for field 'test.field': test error message"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}
}

func TestMaskValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Short value",
			input:    "short",
			expected: "*****",
		},
		{
			name:     "Long value",
			input:    "sk-test-1234567890abcdef",
			expected: "sk-test-" + "****************",
		},
		{
			name:     "Exactly 8 characters",
			input:    "12345678",
			expected: "********",
		},
		{
			name:     "9 characters",
			input:    "123456789",
			expected: "12345678" + "*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskValue(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestContains(t *testing.T) {
	slice := []string{"apple", "banana", "cherry"}

	if !contains(slice, "banana") {
		t.Error("Expected contains to return true for 'banana'")
	}

	if contains(slice, "grape") {
		t.Error("Expected contains to return false for 'grape'")
	}

	if contains([]string{}, "test") {
		t.Error("Expected contains to return false for empty slice")
	}
}
