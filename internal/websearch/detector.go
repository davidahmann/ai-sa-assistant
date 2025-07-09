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

// Package websearch provides keyword detection and web search functionality
// for the AI SA Assistant. It handles intelligent detection of freshness keywords
// to determine when queries require current, time-sensitive information.
package websearch

import (
	"regexp"
	"strings"
)

// DetectionResult contains details about freshness keyword detection
type DetectionResult struct {
	NeedsFreshInfo   bool     `json:"needs_fresh_info"`
	MatchedKeywords  []string `json:"matched_keywords"`
	MatchedPatterns  []string `json:"matched_patterns"`
	ConfidenceScore  float64  `json:"confidence_score"`
	DetectionReasons []string `json:"detection_reasons"`
}

// DetectionConfig contains configuration for freshness detection
type DetectionConfig struct {
	TemporalKeywords    []string `json:"temporal_keywords"`
	ReleaseKeywords     []string `json:"release_keywords"`
	EventKeywords       []string `json:"event_keywords"`
	EnableDateDetection bool     `json:"enable_date_detection"`
}

const (
	// Confidence score weights for different keyword types
	temporalKeywordWeight = 0.3
	releaseKeywordWeight  = 0.25
	eventKeywordWeight    = 0.2
	freshnessThreshold    = 0.2
	historicalYearPenalty = 0.3
	quarterDateScore      = 0.4
	monthDateScore        = 0.3
	recentYearScore       = 0.5
)

var (
	// defaultTemporalKeywords are keywords that indicate need for recent information
	defaultTemporalKeywords = []string{
		"latest", "recent", "recently", "update", "updates", "new", "current", "fresh",
		"today", "yesterday", "this week", "this month", "this year",
		"now", "currently", "presently", "up-to-date", "live",
	}

	// defaultReleaseKeywords are keywords related to product releases
	defaultReleaseKeywords = []string{
		"preview", "ga", "general availability", "announced", "released",
		"launch", "beta", "alpha", "rc", "release candidate",
		"feature", "rollout", "deployment", "patch",
	}

	// defaultEventKeywords are keywords related to conferences and events
	defaultEventKeywords = []string{
		"reinvent", "re:invent", "ignite", "build", "connect", "summit",
		"conference", "keynote", "announcement", "unveil",
		"demo", "showcase", "presentation",
	}

	// Date patterns for matching quarterly and yearly references
	quarterYearPattern = regexp.MustCompile(`(?i)\bQ[1-4]\s+20\d{2}\b`)
	monthYearPattern   = regexp.MustCompile(
		`(?i)\b(january|february|march|april|may|june|july|august|` +
			`september|october|november|december)\s+20\d{2}\b`)
	recentYearPattern     = regexp.MustCompile(`\b202[4-9]\b`)           // Recent years that likely need fresh info
	historicalYearPattern = regexp.MustCompile(`\b20(0\d|1\d|2[0-3])\b`) // Years 2000-2023
)

// DetectFreshnessNeeds analyzes a query to determine if it requires fresh, current information
func DetectFreshnessNeeds(query string, config DetectionConfig) DetectionResult {
	result := DetectionResult{
		NeedsFreshInfo:   false,
		MatchedKeywords:  []string{},
		MatchedPatterns:  []string{},
		ConfidenceScore:  0.0,
		DetectionReasons: []string{},
	}

	queryLower := strings.ToLower(query)

	// Use provided config or defaults
	temporalKeywords := config.TemporalKeywords
	if len(temporalKeywords) == 0 {
		temporalKeywords = defaultTemporalKeywords
	}

	releaseKeywords := config.ReleaseKeywords
	if len(releaseKeywords) == 0 {
		releaseKeywords = defaultReleaseKeywords
	}

	eventKeywords := config.EventKeywords
	if len(eventKeywords) == 0 {
		eventKeywords = defaultEventKeywords
	}

	// Check for temporal keywords
	temporalMatches := checkKeywords(queryLower, temporalKeywords)
	if len(temporalMatches) > 0 {
		result.MatchedKeywords = append(result.MatchedKeywords, temporalMatches...)
		result.ConfidenceScore += temporalKeywordWeight * float64(len(temporalMatches))
		result.DetectionReasons = append(result.DetectionReasons,
			"Found temporal keywords indicating need for current information")
	}

	// Check for release keywords
	releaseMatches := checkKeywords(queryLower, releaseKeywords)
	if len(releaseMatches) > 0 {
		result.MatchedKeywords = append(result.MatchedKeywords, releaseMatches...)
		result.ConfidenceScore += releaseKeywordWeight * float64(len(releaseMatches))
		result.DetectionReasons = append(result.DetectionReasons, "Found release-related keywords")
	}

	// Check for event keywords
	eventMatches := checkKeywords(queryLower, eventKeywords)
	if len(eventMatches) > 0 {
		result.MatchedKeywords = append(result.MatchedKeywords, eventMatches...)
		result.ConfidenceScore += eventKeywordWeight * float64(len(eventMatches))
		result.DetectionReasons = append(result.DetectionReasons, "Found event-related keywords")
	}

	// Check for date patterns if enabled
	if config.EnableDateDetection {
		dateScore := checkDatePatterns(query, &result)
		result.ConfidenceScore += dateScore
	}

	// Determine if fresh information is needed
	result.NeedsFreshInfo = result.ConfidenceScore >= freshnessThreshold

	// Cap confidence score at 1.0
	if result.ConfidenceScore > 1.0 {
		result.ConfidenceScore = 1.0
	}

	return result
}

// checkKeywords checks for keyword matches in the query
func checkKeywords(queryLower string, keywords []string) []string {
	var matches []string
	for _, keyword := range keywords {
		keywordLower := strings.ToLower(keyword)

		// Use word boundaries for all keywords to avoid partial matches
		var pattern string
		if strings.Contains(keywordLower, " ") {
			// For multi-word keywords, escape each word and join with \s+
			words := strings.Fields(keywordLower)
			var escapedWords []string
			for _, word := range words {
				escapedWords = append(escapedWords, regexp.QuoteMeta(word))
			}
			pattern = `\b` + strings.Join(escapedWords, `\s+`) + `\b`
		} else {
			// For single words, use simple word boundaries
			pattern = `\b` + regexp.QuoteMeta(keywordLower) + `\b`
		}

		wordPattern := regexp.MustCompile(pattern)
		if wordPattern.MatchString(queryLower) {
			matches = append(matches, keyword)
		}
	}
	return matches
}

// checkDatePatterns checks for date patterns that indicate freshness needs
func checkDatePatterns(query string, result *DetectionResult) float64 {
	score := 0.0

	// Check for quarterly patterns (Q1 2025, Q2 2025, etc.)
	if quarterYearPattern.MatchString(query) {
		matches := quarterYearPattern.FindAllString(query, -1)
		result.MatchedPatterns = append(result.MatchedPatterns, matches...)
		score += quarterDateScore
		result.DetectionReasons = append(result.DetectionReasons, "Found quarterly date patterns")
	}

	// Check for month/year patterns (June 2025, etc.)
	if monthYearPattern.MatchString(query) {
		matches := monthYearPattern.FindAllString(query, -1)
		result.MatchedPatterns = append(result.MatchedPatterns, matches...)
		score += monthDateScore
		result.DetectionReasons = append(result.DetectionReasons, "Found month/year date patterns")
	}

	// Check for recent years (2024+) - these indicate freshness needs
	if recentYearPattern.MatchString(query) {
		matches := recentYearPattern.FindAllString(query, -1)
		result.MatchedPatterns = append(result.MatchedPatterns, matches...)
		score += recentYearScore
		result.DetectionReasons = append(result.DetectionReasons, "Found recent year patterns")
	}

	// Check for historical years (2000-2023) - these reduce confidence for freshness
	if historicalYearPattern.MatchString(query) {
		matches := historicalYearPattern.FindAllString(query, -1)
		result.MatchedPatterns = append(result.MatchedPatterns, matches...)
		// Subtract from score to indicate this is historical, not fresh
		score -= historicalYearPenalty
		result.DetectionReasons = append(result.DetectionReasons,
			"Found historical year patterns (reduces freshness confidence)")
	}

	return score
}

// DefaultDetectionConfig returns a default configuration for freshness detection
func DefaultDetectionConfig() DetectionConfig {
	return DetectionConfig{
		TemporalKeywords:    defaultTemporalKeywords,
		ReleaseKeywords:     defaultReleaseKeywords,
		EventKeywords:       defaultEventKeywords,
		EnableDateDetection: true,
	}
}

// ConfigFromSlice creates a DetectionConfig from a simple keyword slice (for backward compatibility)
func ConfigFromSlice(keywords []string) DetectionConfig {
	// Categorize keywords based on common patterns
	var temporal, release, event []string

	for _, keyword := range keywords {
		lower := strings.ToLower(keyword)
		switch {
		case contains(defaultTemporalKeywords, lower):
			temporal = append(temporal, keyword)
		case contains(defaultReleaseKeywords, lower):
			release = append(release, keyword)
		case contains(defaultEventKeywords, lower):
			event = append(event, keyword)
		default:
			// Default to temporal for uncategorized keywords
			temporal = append(temporal, keyword)
		}
	}

	// Add defaults if categories are empty
	if len(temporal) == 0 {
		temporal = defaultTemporalKeywords
	}
	if len(release) == 0 {
		release = defaultReleaseKeywords
	}
	if len(event) == 0 {
		event = defaultEventKeywords
	}

	return DetectionConfig{
		TemporalKeywords:    temporal,
		ReleaseKeywords:     release,
		EventKeywords:       event,
		EnableDateDetection: true,
	}
}

// contains checks if a slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}
