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

//go:build performance

package performance

import (
	"os"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"

	"github.com/your-org/ai-sa-assistant/internal/config"
)

// TestConfig holds test configuration
type TestConfig struct {
	OpenAI struct {
		APIKey string
	}
	ChromaDB struct {
		URL string
	}
	Services struct {
		RetrieveURL   string
		SynthesizeURL string
		WebSearchURL  string
		TeamsBotURL   string
	}
}

// createTestConfig creates a test configuration
func createTestConfig(t *testing.T) *TestConfig {
	cfg := &TestConfig{}

	// Load OpenAI API key from environment
	cfg.OpenAI.APIKey = os.Getenv("OPENAI_API_KEY")
	if cfg.OpenAI.APIKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping API-dependent test")
	}

	// Set default service URLs
	cfg.ChromaDB.URL = getDefaultString("CHROMADB_URL", "http://localhost:8000")
	cfg.Services.RetrieveURL = getDefaultString("RETRIEVE_SERVICE_URL", "http://localhost:8081")
	cfg.Services.SynthesizeURL = getDefaultString("SYNTHESIZE_SERVICE_URL", "http://localhost:8082")
	cfg.Services.WebSearchURL = getDefaultString("WEBSEARCH_SERVICE_URL", "http://localhost:8083")
	cfg.Services.TeamsBotURL = getDefaultString("TEAMSBOT_SERVICE_URL", "http://localhost:8080")

	return cfg
}

// createTestLogger creates a test logger
func createTestLogger(t *testing.T) *zap.Logger {
	// Use zaptest for testing to get better integration with testing.T
	return zaptest.NewLogger(t, zaptest.Level(zapcore.InfoLevel))
}

// getDefaultString returns environment variable value or default
func getDefaultString(envVar, defaultValue string) string {
	if value := os.Getenv(envVar); value != "" {
		return value
	}
	return defaultValue
}

// MockConfig creates a mock config for testing
func MockConfig() *config.Config {
	return &config.Config{
		OpenAI: config.OpenAIConfig{
			APIKey: "test-key",
		},
		ChromaDB: config.ChromaDBConfig{
			URL: "http://localhost:8000",
		},
		Services: config.ServicesConfig{
			RetrieveURL:   "http://localhost:8081",
			SynthesizeURL: "http://localhost:8082",
			WebSearchURL:  "http://localhost:8083",
			TeamsBotURL:   "http://localhost:8080",
		},
	}
}

// MockLogger creates a mock logger for testing
func MockLogger() *zap.Logger {
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(zap.ErrorLevel) // Reduce log noise in tests
	logger, _ := config.Build()
	return logger
}

// servicesReady checks if all required services are available for performance testing
func servicesReady(t testing.TB) bool {
	// Skip if running in short mode
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
		return false
	}

	// Skip if required environment variables are not set
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Logf("Skipping performance test: OPENAI_API_KEY not set")
		return false
	}

	// For performance tests, we don't require external services to be running
	// We can use mock services instead
	return true
}
