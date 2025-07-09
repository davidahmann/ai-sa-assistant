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

// Package config provides configuration management for the AI SA Assistant.
// It handles loading and validation of configuration from files and environment variables
// using Viper, with support for OpenAI API keys, service endpoints, and other settings.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

const (
	// DefaultMaxChunks is the default maximum number of results to return from a vector search
	DefaultMaxChunks = 5
	// DefaultFallbackThreshold is the threshold below which fallback search is triggered
	DefaultFallbackThreshold = 3
	// DefaultConfidenceThreshold defines the default confidence threshold for search results
	DefaultConfidenceThreshold = 0.7
	// DefaultFallbackScoreThreshold defines the default score threshold for fallback search
	DefaultFallbackScoreThreshold = 0.7
	// DefaultMaxWebSearchResults defines the default maximum number of web search results
	DefaultMaxWebSearchResults = 3
	// DefaultMaxTokens defines the default maximum number of tokens for responses
	DefaultMaxTokens = 2000
	// DefaultTemperature defines the default temperature for LLM responses
	DefaultTemperature = 0.3

	// MaskedValueMinLength is the minimum length for masking config values when displaying them
	MaskedValueMinLength = 8
	// MaskedValueKeepChars defines how many characters to keep visible when masking config values
	MaskedValueKeepChars = 8

	// DefaultDiagramTimeoutSeconds is the default timeout for diagram generation in seconds
	DefaultDiagramTimeoutSeconds = 30
	// DefaultDiagramCacheExpiryHours is the default cache expiry time for diagrams in hours
	DefaultDiagramCacheExpiryHours = 24
	// DefaultMaxDiagramSize is the default maximum size for diagrams in bytes
	DefaultMaxDiagramSize = 10240
)

var (
	// ErrMissingRequiredField is returned when a required configuration field is missing
	ErrMissingRequiredField = errors.New("missing required configuration field")
	// ErrInvalidConfigValue is returned when a configuration value is invalid
	ErrInvalidConfigValue = errors.New("invalid configuration value")
)

// Config represents the complete application configuration
type Config struct {
	OpenAI    OpenAIConfig    `mapstructure:"openai"`
	Teams     TeamsConfig     `mapstructure:"teams"`
	Services  ServicesConfig  `mapstructure:"services"`
	Chroma    ChromaConfig    `mapstructure:"chroma"`
	Metadata  MetadataConfig  `mapstructure:"metadata"`
	Retrieval RetrievalConfig `mapstructure:"retrieval"`
	WebSearch WebSearchConfig `mapstructure:"websearch"`
	Synthesis SynthesisConfig `mapstructure:"synthesis"`
	Diagram   DiagramConfig   `mapstructure:"diagram"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	Feedback  FeedbackConfig  `mapstructure:"feedback"`
}

// OpenAIConfig contains OpenAI API configuration
type OpenAIConfig struct {
	APIKey   string `mapstructure:"apikey"`
	Endpoint string `mapstructure:"endpoint"`
}

// TeamsConfig contains Microsoft Teams configuration
type TeamsConfig struct {
	WebhookURL    string `mapstructure:"webhook_url"`
	WebhookSecret string `mapstructure:"webhook_secret"`
}

// ServicesConfig contains internal service URLs
type ServicesConfig struct {
	RetrieveURL   string `mapstructure:"retrieve_url"`
	WebSearchURL  string `mapstructure:"websearch_url"`
	SynthesizeURL string `mapstructure:"synthesize_url"`
}

// ChromaConfig contains ChromaDB configuration
type ChromaConfig struct {
	URL            string `mapstructure:"url"`
	CollectionName string `mapstructure:"collection_name"`
}

// MetadataConfig contains metadata store configuration
type MetadataConfig struct {
	DBPath string `mapstructure:"db_path"`
}

// RetrievalConfig contains retrieval-specific settings
type RetrievalConfig struct {
	MaxChunks              int     `mapstructure:"max_chunks"`
	FallbackThreshold      int     `mapstructure:"fallback_threshold"`
	ConfidenceThreshold    float64 `mapstructure:"confidence_threshold"`
	FallbackScoreThreshold float64 `mapstructure:"fallback_score_threshold"`
}

// WebSearchConfig contains web search configuration
type WebSearchConfig struct {
	MaxResults        int      `mapstructure:"max_results"`
	FreshnessKeywords []string `mapstructure:"freshness_keywords"`
}

// SynthesisConfig contains synthesis service configuration
type SynthesisConfig struct {
	Model       string  `mapstructure:"model"`
	MaxTokens   int     `mapstructure:"max_tokens"`
	Temperature float64 `mapstructure:"temperature"`
}

// DiagramConfig contains diagram rendering configuration
type DiagramConfig struct {
	MermaidInkURL  string `mapstructure:"mermaid_ink_url"`
	Timeout        int    `mapstructure:"timeout_seconds"`
	CacheExpiry    int    `mapstructure:"cache_expiry_hours"`
	EnableCaching  bool   `mapstructure:"enable_caching"`
	MaxDiagramSize int    `mapstructure:"max_diagram_size"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

// FeedbackConfig contains feedback storage configuration
type FeedbackConfig struct {
	StorageType string `mapstructure:"storage_type"`
	FilePath    string `mapstructure:"file_path"`
	DBPath      string `mapstructure:"db_path"`
}

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("configuration validation failed for field '%s': %s", e.Field, e.Message)
}

// LoadOptions contains options for configuration loading
type LoadOptions struct {
	ConfigPath       string
	EnableHotReload  bool
	Environment      string
	ValidateRequired bool
	TestMode         bool // Skip validation for CI/CD testing
}

// Load loads configuration from file and environment variables
// Environment variables take precedence over config file values
func Load(configPath string) (*Config, error) {
	return LoadWithOptions(LoadOptions{
		ConfigPath:       configPath,
		EnableHotReload:  false,
		Environment:      getEnvironment(),
		ValidateRequired: true,
	})
}

// LoadWithOptions loads configuration with additional options
func LoadWithOptions(opts LoadOptions) (*Config, error) {
	v := viper.New()

	// Set default values
	setDefaults(v)

	// Set configuration file path
	if err := setConfigFile(v, opts.ConfigPath); err != nil {
		return nil, fmt.Errorf("failed to set config file: %w", err)
	}

	// Enable environment variable overrides
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.SetEnvPrefix("SA_ASSISTANT")

	// Read configuration file
	if err := v.ReadInConfig(); err != nil {
		// Config file not found is not an error if env vars are set
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Set explicit environment variable mappings
	setEnvironmentMappings(v)

	// Unmarshal configuration
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if opts.ValidateRequired && !opts.TestMode {
		if err := validateConfig(&config); err != nil {
			return nil, fmt.Errorf("configuration validation failed: %w", err)
		}
	}

	return &config, nil
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	// OpenAI defaults
	v.SetDefault("openai.endpoint", "https://api.openai.com/v1")

	// Service defaults
	v.SetDefault("services.retrieve_url", "http://retrieve:8081")
	v.SetDefault("services.websearch_url", "http://websearch:8083")
	v.SetDefault("services.synthesize_url", "http://synthesize:8082")

	// ChromaDB defaults
	v.SetDefault("chroma.url", "http://chromadb:8000")
	v.SetDefault("chroma.collection_name", "cloud_assistant")

	// Metadata defaults
	v.SetDefault("metadata.db_path", "./metadata.db")

	// Retrieval defaults
	v.SetDefault("retrieval.max_chunks", DefaultMaxChunks)
	v.SetDefault("retrieval.fallback_threshold", DefaultFallbackThreshold)
	v.SetDefault("retrieval.confidence_threshold", DefaultConfidenceThreshold)
	v.SetDefault("retrieval.fallback_score_threshold", DefaultFallbackScoreThreshold)

	// Web search defaults
	v.SetDefault("websearch.max_results", DefaultMaxWebSearchResults)
	v.SetDefault("websearch.freshness_keywords", []string{
		"latest", "recent", "update", "new", "current", "announced", "release",
		"Q1 2025", "Q2 2025", "Q3 2025", "Q4 2025", "2025", "2024",
		"reinvent", "ignite", "build", "preview", "ga", "general availability",
		"compliance feature", "security update",
	})

	// Synthesis defaults
	v.SetDefault("synthesis.model", "gpt-4o")
	v.SetDefault("synthesis.max_tokens", DefaultMaxTokens)
	v.SetDefault("synthesis.temperature", DefaultTemperature)

	// Diagram defaults
	v.SetDefault("diagram.mermaid_ink_url", "https://mermaid.ink/img")
	v.SetDefault("diagram.timeout_seconds", DefaultDiagramTimeoutSeconds)
	v.SetDefault("diagram.cache_expiry_hours", DefaultDiagramCacheExpiryHours)
	v.SetDefault("diagram.enable_caching", true)
	v.SetDefault("diagram.max_diagram_size", DefaultMaxDiagramSize)

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")

	// Feedback defaults
	v.SetDefault("feedback.storage_type", "file")
	v.SetDefault("feedback.file_path", "./feedback.log")
	v.SetDefault("feedback.db_path", "./feedback.db")
}

// setConfigFile sets the configuration file path with fallback logic
func setConfigFile(v *viper.Viper, configPath string) error {
	// Check for CONFIG_PATH environment variable
	if envPath := os.Getenv("CONFIG_PATH"); envPath != "" {
		if _, err := os.Stat(envPath); err != nil {
			return fmt.Errorf("config file specified by CONFIG_PATH does not exist: %s", envPath)
		}
		v.SetConfigFile(envPath)
		return nil
	}

	// Use provided config path
	if configPath != "" {
		if _, err := os.Stat(configPath); err != nil {
			return fmt.Errorf("config file does not exist: %s", configPath)
		}
		v.SetConfigFile(configPath)
		return nil
	}

	// Default fallback locations
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./configs")
	v.AddConfigPath(".")

	// Check if config file exists in any of the paths
	configExists := false
	for _, path := range []string{"./configs/config.yaml", "./config.yaml"} {
		if _, err := os.Stat(path); err == nil {
			configExists = true
			break
		}
	}

	if !configExists {
		return fmt.Errorf("no config file found in default locations (./configs/config.yaml, ./config.yaml)")
	}

	return nil
}

// setEnvironmentMappings sets explicit environment variable mappings
func setEnvironmentMappings(v *viper.Viper) {
	// Map common environment variables
	envMappings := map[string]string{
		"OPENAI_API_KEY":       "openai.apikey", // pragma: allowlist secret
		"OPENAI_ENDPOINT":      "openai.endpoint",
		"TEAMS_WEBHOOK_URL":    "teams.webhook_url",
		"TEAMS_WEBHOOK_SECRET": "teams.webhook_secret", // pragma: allowlist secret
		"CHROMA_URL":           "chroma.url",
		"METADATA_DB_PATH":     "metadata.db_path",
		"LOG_LEVEL":            "logging.level",
		"LOG_FORMAT":           "logging.format",
		"LOG_OUTPUT":           "logging.output",
	}

	for envVar, configKey := range envMappings {
		if value := os.Getenv(envVar); value != "" {
			v.Set(configKey, value)
		}
	}
}

// validateConfig validates the configuration for required fields and valid values
func validateConfig(config *Config) error {
	var errors []ValidationError

	// Validate required fields
	if config.OpenAI.APIKey == "" {
		errors = append(errors, ValidationError{
			Field:   "openai.apikey",
			Message: "OpenAI API key is required. Set via config file or OPENAI_API_KEY environment variable",
		})
	}

	if config.Teams.WebhookURL == "" {
		errors = append(errors, ValidationError{
			Field:   "teams.webhook_url",
			Message: "Teams webhook URL is required. Set via config file or TEAMS_WEBHOOK_URL environment variable",
		})
	}

	// Validate URLs
	if config.Chroma.URL == "" {
		errors = append(errors, ValidationError{
			Field:   "chroma.url",
			Message: "ChromaDB URL is required",
		})
	}

	// Validate numeric values
	if config.Retrieval.MaxChunks <= 0 {
		errors = append(errors, ValidationError{
			Field:   "retrieval.max_chunks",
			Message: "max_chunks must be greater than 0",
		})
	}

	if config.Retrieval.FallbackThreshold < 0 {
		errors = append(errors, ValidationError{
			Field:   "retrieval.fallback_threshold",
			Message: "fallback_threshold must be greater than or equal to 0",
		})
	}

	if config.Retrieval.ConfidenceThreshold < 0 || config.Retrieval.ConfidenceThreshold > 1 {
		errors = append(errors, ValidationError{
			Field:   "retrieval.confidence_threshold",
			Message: "confidence_threshold must be between 0 and 1",
		})
	}

	if config.Retrieval.FallbackScoreThreshold < 0 || config.Retrieval.FallbackScoreThreshold > 1 {
		errors = append(errors, ValidationError{
			Field:   "retrieval.fallback_score_threshold",
			Message: "fallback_score_threshold must be between 0 and 1",
		})
	}

	if config.WebSearch.MaxResults <= 0 {
		errors = append(errors, ValidationError{
			Field:   "websearch.max_results",
			Message: "max_results must be greater than 0",
		})
	}

	if config.Synthesis.MaxTokens <= 0 {
		errors = append(errors, ValidationError{
			Field:   "synthesis.max_tokens",
			Message: "max_tokens must be greater than 0",
		})
	}

	if config.Synthesis.Temperature < 0 || config.Synthesis.Temperature > 2 {
		errors = append(errors, ValidationError{
			Field:   "synthesis.temperature",
			Message: "temperature must be between 0 and 2",
		})
	}

	// Validate diagram configuration
	if config.Diagram.MermaidInkURL == "" {
		errors = append(errors, ValidationError{
			Field:   "diagram.mermaid_ink_url",
			Message: "mermaid_ink_url is required",
		})
	}

	if config.Diagram.Timeout <= 0 {
		errors = append(errors, ValidationError{
			Field:   "diagram.timeout_seconds",
			Message: "timeout_seconds must be greater than 0",
		})
	}

	if config.Diagram.CacheExpiry <= 0 {
		errors = append(errors, ValidationError{
			Field:   "diagram.cache_expiry_hours",
			Message: "cache_expiry_hours must be greater than 0",
		})
	}

	if config.Diagram.MaxDiagramSize <= 0 {
		errors = append(errors, ValidationError{
			Field:   "diagram.max_diagram_size",
			Message: "max_diagram_size must be greater than 0",
		})
	}

	// Validate enum values
	validLogLevels := []string{"debug", "info", "warn", "error"}
	if !contains(validLogLevels, config.Logging.Level) {
		errors = append(errors, ValidationError{
			Field:   "logging.level",
			Message: fmt.Sprintf("log level must be one of: %s", strings.Join(validLogLevels, ", ")),
		})
	}

	validLogFormats := []string{"json", "text"}
	if !contains(validLogFormats, config.Logging.Format) {
		errors = append(errors, ValidationError{
			Field:   "logging.format",
			Message: fmt.Sprintf("log format must be one of: %s", strings.Join(validLogFormats, ", ")),
		})
	}

	validStorageTypes := []string{"file", "sqlite"}
	if !contains(validStorageTypes, config.Feedback.StorageType) {
		errors = append(errors, ValidationError{
			Field:   "feedback.storage_type",
			Message: fmt.Sprintf("storage type must be one of: %s", strings.Join(validStorageTypes, ", ")),
		})
	}

	// Validate file paths
	if config.Metadata.DBPath == "" {
		errors = append(errors, ValidationError{
			Field:   "metadata.db_path",
			Message: "metadata database path is required",
		})
	}

	// Validate directory existence for file paths
	if config.Metadata.DBPath != "" {
		if err := validateDirectoryExists(filepath.Dir(config.Metadata.DBPath)); err != nil {
			errors = append(errors, ValidationError{
				Field:   "metadata.db_path",
				Message: fmt.Sprintf("metadata database directory does not exist: %s", filepath.Dir(config.Metadata.DBPath)),
			})
		}
	}

	// Return all validation errors
	if len(errors) > 0 {
		var errorMessages []string
		for _, err := range errors {
			errorMessages = append(errorMessages, err.Error())
		}
		return fmt.Errorf("configuration validation failed:\n%s", strings.Join(errorMessages, "\n"))
	}

	return nil
}

// MaskSensitiveValues returns a copy of the config with sensitive values masked
func (c *Config) MaskSensitiveValues() *Config {
	masked := *c

	// Mask sensitive fields
	if masked.OpenAI.APIKey != "" {
		masked.OpenAI.APIKey = maskValue(masked.OpenAI.APIKey)
	}
	if masked.Teams.WebhookURL != "" {
		masked.Teams.WebhookURL = maskValue(masked.Teams.WebhookURL)
	}
	if masked.Teams.WebhookSecret != "" {
		masked.Teams.WebhookSecret = maskValue(masked.Teams.WebhookSecret)
	}

	return &masked
}

// maskValue masks sensitive values, showing only the first 8 characters
func maskValue(value string) string {
	if len(value) <= MaskedValueMinLength {
		return strings.Repeat("*", len(value))
	}
	return value[:MaskedValueKeepChars] + strings.Repeat("*", len(value)-MaskedValueKeepChars)
}

// contains checks if a slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// validateDirectoryExists checks if a directory exists
func validateDirectoryExists(path string) error {
	if path == "" || path == "." {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	return nil
}

// getEnvironment returns the current environment (development, production, etc.)
func getEnvironment() string {
	if env := os.Getenv("ENVIRONMENT"); env != "" {
		return env
	}
	if env := os.Getenv("ENV"); env != "" {
		return env
	}
	return "development"
}

// WatchConfig enables configuration hot-reloading for development
func WatchConfig(configPath string, callback func(*Config)) error {
	v := viper.New()

	// Set up configuration
	if err := setConfigFile(v, configPath); err != nil {
		return err
	}

	// Enable watching
	v.WatchConfig()
	v.OnConfigChange(func(e fsnotify.Event) {
		fmt.Printf("Config file changed: %s\n", e.Name)

		// Reload configuration
		config, err := LoadWithOptions(LoadOptions{
			ConfigPath:       configPath,
			EnableHotReload:  true,
			Environment:      getEnvironment(),
			ValidateRequired: true,
		})
		if err != nil {
			fmt.Printf("Failed to reload config: %v\n", err)
			return
		}

		// Call callback with new config
		callback(config)
	})

	return nil
}
