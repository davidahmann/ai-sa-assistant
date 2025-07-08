package config

import (
	"os"

	"github.com/spf13/viper"
)

type Config struct {
	OpenAI    OpenAIConfig    `mapstructure:"openai"`
	Teams     TeamsConfig     `mapstructure:"teams"`
	Services  ServicesConfig  `mapstructure:"services"`
	Chroma    ChromaConfig    `mapstructure:"chroma"`
	Metadata  MetadataConfig  `mapstructure:"metadata"`
	Retrieval RetrievalConfig `mapstructure:"retrieval"`
	WebSearch WebSearchConfig `mapstructure:"websearch"`
	Synthesis SynthesisConfig `mapstructure:"synthesis"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	Feedback  FeedbackConfig  `mapstructure:"feedback"`
}

type OpenAIConfig struct {
	APIKey string `mapstructure:"apikey"`
}

type TeamsConfig struct {
	WebhookURL string `mapstructure:"webhook_url"`
}

type ServicesConfig struct {
	RetrieveURL   string `mapstructure:"retrieve_url"`
	WebSearchURL  string `mapstructure:"websearch_url"`
	SynthesizeURL string `mapstructure:"synthesize_url"`
}

type ChromaConfig struct {
	URL            string `mapstructure:"url"`
	CollectionName string `mapstructure:"collection_name"`
}

type MetadataConfig struct {
	DBPath string `mapstructure:"db_path"`
}

type RetrievalConfig struct {
	MaxChunks           int     `mapstructure:"max_chunks"`
	FallbackThreshold   int     `mapstructure:"fallback_threshold"`
	ConfidenceThreshold float64 `mapstructure:"confidence_threshold"`
}

type WebSearchConfig struct {
	MaxResults        int      `mapstructure:"max_results"`
	FreshnessKeywords []string `mapstructure:"freshness_keywords"`
}

type SynthesisConfig struct {
	Model       string  `mapstructure:"model"`
	MaxTokens   int     `mapstructure:"max_tokens"`
	Temperature float64 `mapstructure:"temperature"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

type FeedbackConfig struct {
	StorageType string `mapstructure:"storage_type"`
	FilePath    string `mapstructure:"file_path"`
	DBPath      string `mapstructure:"db_path"`
}

// Load loads configuration from file and environment variables
// Environment variables take precedence over config file values
func Load(configPath string) (*Config, error) {
	// Set default values
	viper.SetDefault("chroma.collection_name", "cloud_assistant")
	viper.SetDefault("retrieval.max_chunks", 5)
	viper.SetDefault("retrieval.fallback_threshold", 3)
	viper.SetDefault("retrieval.confidence_threshold", 0.7)
	viper.SetDefault("synthesis.model", "gpt-4o")
	viper.SetDefault("synthesis.max_tokens", 2000)
	viper.SetDefault("synthesis.temperature", 0.3)
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "json")
	viper.SetDefault("feedback.storage_type", "file")

	// Load config file
	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		viper.AddConfigPath("./configs")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		// Config file not found is not an error if env vars are set
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	// Environment variable overrides
	viper.AutomaticEnv()

	// Specific environment variable mappings
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		viper.Set("openai.apikey", apiKey)
	}
	if webhookURL := os.Getenv("TEAMS_WEBHOOK_URL"); webhookURL != "" {
		viper.Set("teams.webhook_url", webhookURL)
	}
	if configPathEnv := os.Getenv("CONFIG_PATH"); configPathEnv != "" {
		viper.SetConfigFile(configPathEnv)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}

	return &config, nil
}
