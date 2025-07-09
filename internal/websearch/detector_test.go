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

package websearch

import (
	"testing"
)

func TestDetectFreshnessNeeds(t *testing.T) {
	tests := []struct {
		name                  string
		query                 string
		config                DetectionConfig
		expectedNeedsFresh    bool
		expectedMinKeywords   int
		expectedMinPatterns   int
		expectedMinReasons    int
		expectedMinConfidence float64
	}{
		// Temporal keywords tests
		{
			name:                  "latest keyword detection",
			query:                 "What are the latest features in AWS?",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinKeywords:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},
		{
			name:                  "recent keyword detection",
			query:                 "Show me recent updates to Azure services",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinKeywords:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},
		{
			name:                  "current keyword detection",
			query:                 "What is the current status of Google Cloud?",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinKeywords:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},
		{
			name:                  "new keyword detection",
			query:                 "Tell me about new AWS services",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinKeywords:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},
		{
			name:                  "update keyword detection",
			query:                 "Any updates to Kubernetes recently?",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinKeywords:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},

		// Date pattern tests
		{
			name:                  "quarterly pattern Q1 2025",
			query:                 "What happened in Q1 2025 for cloud computing?",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinPatterns:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},
		{
			name:                  "quarterly pattern Q2 2025",
			query:                 "Show Q2 2025 AWS announcements",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinPatterns:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},
		{
			name:                  "quarterly pattern Q3 2025",
			query:                 "Q3 2025 technology trends",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinPatterns:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},
		{
			name:                  "quarterly pattern Q4 2025",
			query:                 "Q4 2025 cloud security updates",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinPatterns:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},
		{
			name:                  "month/year pattern June 2025",
			query:                 "June 2025 Azure announcements",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinPatterns:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},
		{
			name:                  "year pattern 2025",
			query:                 "2025 cloud computing predictions",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinPatterns:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},

		// Release keywords tests
		{
			name:                  "preview keyword detection",
			query:                 "AWS Lambda preview features",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinKeywords:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},
		{
			name:                  "GA keyword detection",
			query:                 "When did this service go GA?",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinKeywords:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},
		{
			name:                  "announced keyword detection",
			query:                 "Recently announced cloud services",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinKeywords:   2, // "recently" + "announced"
			expectedMinReasons:    2,
			expectedMinConfidence: 0.2,
		},
		{
			name:                  "released keyword detection",
			query:                 "What was released at the conference?",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinKeywords:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},

		// Event keywords tests
		{
			name:                  "reinvent keyword detection",
			query:                 "AWS re:Invent 2025 announcements",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinKeywords:   1,
			expectedMinPatterns:   1, // "2025"
			expectedMinReasons:    2,
			expectedMinConfidence: 0.2,
		},
		{
			name:                  "ignite keyword detection",
			query:                 "Microsoft Ignite cloud updates",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinKeywords:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},
		{
			name:                  "build keyword detection",
			query:                 "Google Cloud Next build session",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinKeywords:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},
		{
			name:                  "connect keyword detection",
			query:                 "AWS Connect service updates",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinKeywords:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},

		// Case-insensitive tests
		{
			name:                  "case insensitive LATEST",
			query:                 "LATEST AWS features",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinKeywords:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},
		{
			name:                  "case insensitive q2 2025",
			query:                 "q2 2025 azure updates",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinPatterns:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},
		{
			name:                  "case insensitive PREVIEW",
			query:                 "PREVIEW features in cloud",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinKeywords:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},
		{
			name:                  "case insensitive REINVENT",
			query:                 "REINVENT announcements",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinKeywords:   1,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.2,
		},

		// No freshness needed tests
		{
			name:                  "no freshness keywords",
			query:                 "Explain cloud computing basics",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    false,
			expectedMinKeywords:   0,
			expectedMinPatterns:   0,
			expectedMinReasons:    0,
			expectedMinConfidence: 0.0,
		},
		{
			name:                  "historical query",
			query:                 "What was announced in 2020?",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    false, // 2020 is not recent enough
			expectedMinKeywords:   0,
			expectedMinPatterns:   0,
			expectedMinReasons:    0,
			expectedMinConfidence: -1.0, // Negative confidence is fine for historical queries
		},

		// Complex queries with multiple indicators
		{
			name:                  "multiple temporal keywords",
			query:                 "What are the latest and most recent updates?",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinKeywords:   2,
			expectedMinReasons:    1,
			expectedMinConfidence: 0.4,
		},
		{
			name:                  "temporal and date patterns",
			query:                 "Latest Q2 2025 AWS updates",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinKeywords:   1,
			expectedMinPatterns:   1,
			expectedMinReasons:    2,
			expectedMinConfidence: 0.5,
		},
		{
			name:                  "all categories combined",
			query:                 "Latest Q2 2025 AWS preview features announced at re:Invent",
			config:                DefaultDetectionConfig(),
			expectedNeedsFresh:    true,
			expectedMinKeywords:   3, // "latest", "preview", "announced", "reinvent"
			expectedMinPatterns:   1, // "Q2 2025"
			expectedMinReasons:    3,
			expectedMinConfidence: 0.8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectFreshnessNeeds(tt.query, tt.config)

			// Check freshness needs
			if result.NeedsFreshInfo != tt.expectedNeedsFresh {
				t.Errorf("DetectFreshnessNeeds() NeedsFreshInfo = %v, want %v",
					result.NeedsFreshInfo, tt.expectedNeedsFresh)
			}

			// Check matched keywords count
			if len(result.MatchedKeywords) < tt.expectedMinKeywords {
				t.Errorf("DetectFreshnessNeeds() matched keywords = %d, want at least %d. Keywords: %v",
					len(result.MatchedKeywords), tt.expectedMinKeywords, result.MatchedKeywords)
			}

			// Check matched patterns count
			if len(result.MatchedPatterns) < tt.expectedMinPatterns {
				t.Errorf("DetectFreshnessNeeds() matched patterns = %d, want at least %d. Patterns: %v",
					len(result.MatchedPatterns), tt.expectedMinPatterns, result.MatchedPatterns)
			}

			// Check detection reasons count
			if len(result.DetectionReasons) < tt.expectedMinReasons {
				t.Errorf("DetectFreshnessNeeds() detection reasons = %d, want at least %d. Reasons: %v",
					len(result.DetectionReasons), tt.expectedMinReasons, result.DetectionReasons)
			}

			// Check confidence score
			if result.ConfidenceScore < tt.expectedMinConfidence {
				t.Errorf("DetectFreshnessNeeds() confidence score = %f, want at least %f",
					result.ConfidenceScore, tt.expectedMinConfidence)
			}

			// Ensure confidence score doesn't exceed 1.0
			if result.ConfidenceScore > 1.0 {
				t.Errorf("DetectFreshnessNeeds() confidence score = %f, should not exceed 1.0",
					result.ConfidenceScore)
			}
		})
	}
}

func TestDetectionConfigCustomization(t *testing.T) {
	// Test custom configuration
	customConfig := DetectionConfig{
		TemporalKeywords:    []string{"custom", "temporal"},
		ReleaseKeywords:     []string{"custom", "release"},
		EventKeywords:       []string{"custom", "event"},
		EnableDateDetection: true,
	}

	tests := []struct {
		name           string
		query          string
		config         DetectionConfig
		expectedResult bool
	}{
		{
			name:           "custom temporal keyword",
			query:          "Show me custom cloud services",
			config:         customConfig,
			expectedResult: true,
		},
		{
			name:           "custom release keyword",
			query:          "Custom release notes",
			config:         customConfig,
			expectedResult: true,
		},
		{
			name:           "custom event keyword",
			query:          "Custom event announcements",
			config:         customConfig,
			expectedResult: true,
		},
		{
			name:           "default keyword not in custom config",
			query:          "Latest updates", // "latest" not in custom config
			config:         customConfig,
			expectedResult: false,
		},
		{
			name:           "date detection still works with custom config",
			query:          "Q1 2025 trends",
			config:         customConfig,
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectFreshnessNeeds(tt.query, tt.config)
			if result.NeedsFreshInfo != tt.expectedResult {
				t.Errorf("DetectFreshnessNeeds() with custom config = %v, want %v",
					result.NeedsFreshInfo, tt.expectedResult)
			}
		})
	}
}

func TestDateDetectionDisabled(t *testing.T) {
	configWithoutDateDetection := DetectionConfig{
		TemporalKeywords:    defaultTemporalKeywords,
		ReleaseKeywords:     defaultReleaseKeywords,
		EventKeywords:       defaultEventKeywords,
		EnableDateDetection: false,
	}

	result := DetectFreshnessNeeds("Q2 2025 updates", configWithoutDateDetection)

	// Should not detect date patterns when disabled
	if len(result.MatchedPatterns) > 0 {
		t.Errorf("Expected no matched patterns when date detection disabled, got %v",
			result.MatchedPatterns)
	}

	// Should still work for temporal keywords
	if !result.NeedsFreshInfo {
		t.Errorf("Expected freshness detection for 'updates' keyword even with date detection disabled")
	}
}

func TestConfigFromSlice(t *testing.T) {
	keywords := []string{"latest", "preview", "reinvent", "custom"}

	config := ConfigFromSlice(keywords)

	// Check that keywords are categorized properly
	if len(config.TemporalKeywords) == 0 {
		t.Error("Expected temporal keywords to be populated")
	}
	if len(config.ReleaseKeywords) == 0 {
		t.Error("Expected release keywords to be populated")
	}
	if len(config.EventKeywords) == 0 {
		t.Error("Expected event keywords to be populated")
	}

	// Check that date detection is enabled by default
	if !config.EnableDateDetection {
		t.Error("Expected date detection to be enabled by default")
	}
}

func TestDefaultDetectionConfig(t *testing.T) {
	config := DefaultDetectionConfig()

	// Check that all keyword categories are populated
	if len(config.TemporalKeywords) == 0 {
		t.Error("Expected default temporal keywords to be populated")
	}
	if len(config.ReleaseKeywords) == 0 {
		t.Error("Expected default release keywords to be populated")
	}
	if len(config.EventKeywords) == 0 {
		t.Error("Expected default event keywords to be populated")
	}

	// Check that date detection is enabled by default
	if !config.EnableDateDetection {
		t.Error("Expected date detection to be enabled by default")
	}
}

func TestWordBoundaryMatching(t *testing.T) {
	config := DefaultDetectionConfig()

	tests := []struct {
		name           string
		query          string
		expectedResult bool
		description    string
	}{
		{
			name:           "exact word match",
			query:          "Show me the latest updates",
			expectedResult: true,
			description:    "Should match 'latest' as a complete word",
		},
		{
			name:           "partial word match should not trigger",
			query:          "This is the greatest enhancement ever",
			expectedResult: false,
			description:    "Should not match 'latest' within 'greatest'",
		},
		{
			name:           "word at beginning",
			query:          "latest features in cloud",
			expectedResult: true,
			description:    "Should match word at beginning of sentence",
		},
		{
			name:           "word at end",
			query:          "cloud features that are latest",
			expectedResult: true,
			description:    "Should match word at end of sentence",
		},
		{
			name:           "multi-word phrase",
			query:          "general availability status",
			expectedResult: true,
			description:    "Should match multi-word phrases",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectFreshnessNeeds(tt.query, config)
			if result.NeedsFreshInfo != tt.expectedResult {
				t.Errorf("%s: DetectFreshnessNeeds() = %v, want %v",
					tt.description, result.NeedsFreshInfo, tt.expectedResult)
			}
		})
	}
}
