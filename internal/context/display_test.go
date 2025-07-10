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

package context

import (
	"testing"
	"time"

	"github.com/your-org/ai-sa-assistant/internal/synth"
)

func TestFormatContextDisplay(t *testing.T) {
	// Create test response with mock data
	response := synth.SynthesisResponse{
		MainText:    "This is a test response",
		DiagramCode: "graph TD\n  A --> B",
		ContextSources: []synth.ContextSourceInfo{
			{
				SourceID:   "aws-migration-guide.md",
				Title:      "AWS Migration Guide",
				Confidence: 0.85,
				Relevance:  0.9,
				ChunkIndex: 0,
				Preview:    "This guide covers best practices for migrating to AWS...",
				SourceType: "guide",
				TokenCount: 150,
				Used:       true,
			},
		},
		WebSources: []synth.WebSourceInfo{
			{
				URL:        "https://aws.amazon.com/migration",
				Title:      "AWS Migration Best Practices",
				Snippet:    "Learn about the latest migration strategies...",
				Confidence: 0.8,
				Freshness:  "recent",
				Domain:     "aws.amazon.com",
				Used:       true,
			},
		},
		ProcessingStats: synth.ProcessingStats{
			TotalProcessingTime: 2500,
			RetrievalTime:      800,
			WebSearchTime:      700,
			SynthesisTime:      1000,
			InputTokens:        1200,
			OutputTokens:       800,
			TotalTokens:        2000,
			ModelUsed:          "GPT-4",
			Temperature:        0.3,
		},
		PipelineDecision: synth.PipelineDecisionInfo{
			QueryType:              "TechnicalQuery",
			FallbackSearchUsed:     false,
			WebSearchTriggered:     true,
			FreshnessKeywords:      []string{"latest", "2024"},
			ArchitectureDiagram:    true,
			CodeGenerated:          false,
			ContextItemsFiltered:   5,
			ContextItemsUsed:       3,
			Reasoning:              "Query required fresh information about migration practices",
		},
	}

	config := DefaultDisplayConfig()
	result := FormatContextDisplay(response, config)

	// Test that all components are generated
	if result.SourceSummary.HTML == "" {
		t.Error("Expected HTML source summary to be generated")
	}

	if result.SourceSummary.Text == "" {
		t.Error("Expected text source summary to be generated")
	}

	if result.SourceSummary.Markdown == "" {
		t.Error("Expected markdown source summary to be generated")
	}

	if result.PipelineVisibility.HTML == "" {
		t.Error("Expected HTML pipeline visibility to be generated")
	}

	if result.ProcessingStats.TotalTime == "" {
		t.Error("Expected processing time to be formatted")
	}

	if result.TrustIndicators.OverallScore == 0 {
		t.Error("Expected trust indicators to be calculated")
	}
}

func TestCalculateTrustIndicators(t *testing.T) {
	response := synth.SynthesisResponse{
		ContextSources: []synth.ContextSourceInfo{
			{Confidence: 0.9, Used: true},
			{Confidence: 0.8, Used: true},
		},
		WebSources: []synth.WebSourceInfo{
			{Confidence: 0.85, Used: true},
		},
		PipelineDecision: synth.PipelineDecisionInfo{
			WebSearchTriggered: true,
		},
	}

	indicators := calculateTrustIndicators(response)

	expectedSourceQuality := (0.9 + 0.8 + 0.85) / 3
	tolerance := 0.001
	if indicators.SourceQuality < expectedSourceQuality-tolerance || indicators.SourceQuality > expectedSourceQuality+tolerance {
		t.Errorf("Expected source quality %.3f, got %.3f", expectedSourceQuality, indicators.SourceQuality)
	}

	if indicators.ConfidenceLevel != "High" {
		t.Errorf("Expected confidence level 'High', got '%s'", indicators.ConfidenceLevel)
	}

	if indicators.Freshness != "Recent" {
		t.Errorf("Expected freshness 'Recent', got '%s'", indicators.Freshness)
	}

	if len(indicators.TrustBadges) == 0 {
		t.Error("Expected trust badges to be generated")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		ms       int
		expected string
	}{
		{0, "< 1ms"},
		{500, "500ms"},
		{1500, "1.5s"},
		{2000, "2.0s"},
	}

	for _, test := range tests {
		result := formatDuration(time.Duration(test.ms) * time.Millisecond)
		if result != test.expected {
			t.Errorf("For %dms, expected '%s', got '%s'", test.ms, test.expected, result)
		}
	}
}

// Note: Utility functions like extractDomainFromURL, detectSourceType, and extractTitleFromSourceID
// are tested in the synth package where they are defined. This package focuses on display formatting.