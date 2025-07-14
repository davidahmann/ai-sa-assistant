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

// Package context provides context display and formatting functionality
// for the AI SA Assistant, including source citations, pipeline visibility,
// and trust indicators for RAG responses.
package context

import (
	"fmt"
	"strings"
	"time"

	"github.com/your-org/ai-sa-assistant/internal/synth"
)

// DisplayConfig controls how context information is displayed
type DisplayConfig struct {
	ShowConfidenceScores  bool
	ShowTokenUsage        bool
	ShowProcessingTime    bool
	ShowPipelineDecisions bool
	CompactView           bool
	MaxPreviewLength      int
}

// DefaultDisplayConfig returns default configuration for context display
func DefaultDisplayConfig() DisplayConfig {
	return DisplayConfig{
		ShowConfidenceScores:  true,
		ShowTokenUsage:        true,
		ShowProcessingTime:    true,
		ShowPipelineDecisions: true,
		CompactView:           false,
		MaxPreviewLength:      150,
	}
}

// SourceSummary represents a formatted summary of sources used in the response
type SourceSummary struct {
	HTML     string `json:"html"`
	Text     string `json:"text"`
	Markdown string `json:"markdown"`
}

// PipelineVisibility represents formatted pipeline decision information
type PipelineVisibility struct {
	HTML     string `json:"html"`
	Text     string `json:"text"`
	Markdown string `json:"markdown"`
}

// DisplayResult contains all formatted context information
type DisplayResult struct {
	SourceSummary      SourceSummary       `json:"source_summary"`
	PipelineVisibility PipelineVisibility  `json:"pipeline_visibility"`
	ProcessingStats    ProcessingStatsView `json:"processing_stats"`
	TrustIndicators    TrustIndicators     `json:"trust_indicators"`
}

// ProcessingStatsView represents formatted processing statistics
type ProcessingStatsView struct {
	TotalTime      string `json:"total_time"`
	TokenUsage     string `json:"token_usage"`
	EstimatedCost  string `json:"estimated_cost"`
	ModelInfo      string `json:"model_info"`
	PerformanceBar string `json:"performance_bar"`
}

// TrustIndicators represents trust and quality indicators
type TrustIndicators struct {
	OverallScore    float64  `json:"overall_score"`
	SourceQuality   float64  `json:"source_quality"`
	Freshness       string   `json:"freshness"`
	ConfidenceLevel string   `json:"confidence_level"`
	TrustBadges     []string `json:"trust_badges"`
}

// FormatContextDisplay creates comprehensive formatted context display
func FormatContextDisplay(response synth.SynthesisResponse, config DisplayConfig) DisplayResult {
	return DisplayResult{
		SourceSummary:      formatSourceSummary(response, config),
		PipelineVisibility: formatPipelineVisibility(response, config),
		ProcessingStats:    formatProcessingStats(response.ProcessingStats, config),
		TrustIndicators:    calculateTrustIndicators(response),
	}
}

// formatSourceSummary creates formatted source citation summary
func formatSourceSummary(response synth.SynthesisResponse, config DisplayConfig) SourceSummary {
	var htmlBuilder, textBuilder, markdownBuilder strings.Builder

	// Count sources by type
	internalSources := len(response.ContextSources)
	webSources := len(response.WebSources)
	totalSources := internalSources + webSources

	if totalSources == 0 {
		emptyMsg := "No sources were used for this response."
		return SourceSummary{
			HTML:     fmt.Sprintf(`<div class="source-summary empty">%s</div>`, emptyMsg),
			Text:     emptyMsg,
			Markdown: fmt.Sprintf("*%s*", emptyMsg),
		}
	}

	// HTML version
	htmlBuilder.WriteString(`<div class="source-summary">`)
	htmlBuilder.WriteString(`<div class="source-header">`)
	htmlBuilder.WriteString(fmt.Sprintf(`<h4>📋 Context Sources Used (%d total)</h4>`, totalSources))
	htmlBuilder.WriteString(`</div>`)

	// Internal documents section
	if internalSources > 0 {
		htmlBuilder.WriteString(formatInternalSourcesHTML(response.ContextSources, config))
	}

	// Web sources section
	if webSources > 0 {
		htmlBuilder.WriteString(formatWebSourcesHTML(response.WebSources, config))
	}

	// LLM synthesis section
	htmlBuilder.WriteString(formatLLMSynthesisHTML(response.ProcessingStats))
	htmlBuilder.WriteString(`</div>`)

	// Text version
	textBuilder.WriteString(fmt.Sprintf("📋 Context Sources Used (%d total):\n", totalSources))
	if internalSources > 0 {
		textBuilder.WriteString(formatInternalSourcesText(response.ContextSources, config))
	}
	if webSources > 0 {
		textBuilder.WriteString(formatWebSourcesText(response.WebSources, config))
	}
	textBuilder.WriteString(formatLLMSynthesisText(response.ProcessingStats))

	// Markdown version
	markdownBuilder.WriteString(fmt.Sprintf("## 📋 Context Sources Used (%d total)\n\n", totalSources))
	if internalSources > 0 {
		markdownBuilder.WriteString(formatInternalSourcesMarkdown(response.ContextSources, config))
	}
	if webSources > 0 {
		markdownBuilder.WriteString(formatWebSourcesMarkdown(response.WebSources, config))
	}
	markdownBuilder.WriteString(formatLLMSynthesisMarkdown(response.ProcessingStats))

	return SourceSummary{
		HTML:     htmlBuilder.String(),
		Text:     textBuilder.String(),
		Markdown: markdownBuilder.String(),
	}
}

// formatInternalSourcesHTML formats internal document sources as HTML
func formatInternalSourcesHTML(sources []synth.ContextSourceInfo, config DisplayConfig) string {
	var builder strings.Builder

	builder.WriteString(`<div class="internal-sources">`)
	builder.WriteString(fmt.Sprintf(`<h5>📄 Internal Documents (%d chunks)</h5>`, len(sources)))
	builder.WriteString(`<ul class="source-list">`)

	for _, source := range sources {
		usedIcon := "✓"
		usedClass := "used"
		if !source.Used {
			usedIcon = "○"
			usedClass = "unused"
		}

		builder.WriteString(fmt.Sprintf(`<li class="source-item %s">`, usedClass))
		builder.WriteString(fmt.Sprintf(`<div class="source-info">%s <strong>%s</strong>`, usedIcon, source.Title))

		if config.ShowConfidenceScores {
			builder.WriteString(fmt.Sprintf(` (confidence: %.2f)`, source.Confidence))
		}

		builder.WriteString(`</div>`)

		if !config.CompactView && source.Preview != "" {
			preview := source.Preview
			if len(preview) > config.MaxPreviewLength {
				preview = preview[:config.MaxPreviewLength] + "..."
			}
			builder.WriteString(fmt.Sprintf(`<div class="source-preview">%s</div>`, preview))
		}

		if config.ShowTokenUsage {
			builder.WriteString(fmt.Sprintf(
				`<div class="source-meta">%d tokens • %s</div>`,
				source.TokenCount,
				source.SourceType,
			))
		}

		builder.WriteString(`</li>`)
	}

	builder.WriteString(`</ul></div>`)
	return builder.String()
}

// formatWebSourcesHTML formats web search sources as HTML
func formatWebSourcesHTML(sources []synth.WebSourceInfo, config DisplayConfig) string {
	var builder strings.Builder

	builder.WriteString(`<div class="web-sources">`)
	builder.WriteString(fmt.Sprintf(`<h5>🌐 Web Search Results (%d articles)</h5>`, len(sources)))
	builder.WriteString(`<ul class="source-list">`)

	for _, source := range sources {
		usedIcon := "✓"
		usedClass := "used"
		if !source.Used {
			usedIcon = "○"
			usedClass = "unused"
		}

		builder.WriteString(fmt.Sprintf(`<li class="source-item %s">`, usedClass))
		builder.WriteString(fmt.Sprintf(
			`<div class="source-info">%s <a href="%s" target="_blank">%s</a>`,
			usedIcon,
			source.URL,
			source.Title,
		))

		if config.ShowConfidenceScores {
			builder.WriteString(fmt.Sprintf(` (confidence: %.2f)`, source.Confidence))
		}

		builder.WriteString(`</div>`)

		if !config.CompactView && source.Snippet != "" {
			snippet := source.Snippet
			if len(snippet) > config.MaxPreviewLength {
				snippet = snippet[:config.MaxPreviewLength] + "..."
			}
			builder.WriteString(fmt.Sprintf(`<div class="source-preview">%s</div>`, snippet))
		}

		builder.WriteString(fmt.Sprintf(`<div class="source-meta">%s • %s</div>`, source.Domain, source.Freshness))
		builder.WriteString(`</li>`)
	}

	builder.WriteString(`</ul></div>`)
	return builder.String()
}

// formatLLMSynthesisHTML formats LLM synthesis information as HTML
func formatLLMSynthesisHTML(stats synth.ProcessingStats) string {
	return fmt.Sprintf(`
		<div class="llm-synthesis">
			<h5>🤖 LLM Synthesis</h5>
			<div class="synthesis-info">
				<div class="model-info">Model: %s</div>
				<div class="token-info">Tokens: %d input / %d output</div>
				<div class="temp-info">Temperature: %.1f</div>
			</div>
		</div>
	`, stats.ModelUsed, stats.InputTokens, stats.OutputTokens, stats.Temperature)
}

// formatInternalSourcesText formats internal document sources as plain text
func formatInternalSourcesText(sources []synth.ContextSourceInfo, config DisplayConfig) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("┌─ Internal Documents (%d chunks)\n", len(sources)))

	for i, source := range sources {
		usedIcon := "├─"
		if i == len(sources)-1 {
			usedIcon = "└─"
		}

		builder.WriteString(fmt.Sprintf("%s %s", usedIcon, source.Title))

		if config.ShowConfidenceScores {
			builder.WriteString(fmt.Sprintf(" (confidence: %.2f)", source.Confidence))
		}

		builder.WriteString("\n")
	}

	return builder.String()
}

// formatWebSourcesText formats web search sources as plain text
func formatWebSourcesText(sources []synth.WebSourceInfo, _ DisplayConfig) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("├─ Web Search Results (%d articles)\n", len(sources)))

	for i, source := range sources {
		usedIcon := "│  ├─"
		if i == len(sources)-1 {
			usedIcon = "│  └─"
		}

		builder.WriteString(fmt.Sprintf("%s %s (%s)\n", usedIcon, source.Title, source.URL))
	}

	return builder.String()
}

// formatLLMSynthesisText formats LLM synthesis information as plain text
func formatLLMSynthesisText(stats synth.ProcessingStats) string {
	return fmt.Sprintf("└─ LLM Synthesis\n   ├─ Model: %s\n   ├─ Tokens: %d input / %d output\n   └─ Temperature: %.1f\n",
		stats.ModelUsed, stats.InputTokens, stats.OutputTokens, stats.Temperature)
}

// formatInternalSourcesMarkdown formats internal document sources as Markdown
func formatInternalSourcesMarkdown(sources []synth.ContextSourceInfo, config DisplayConfig) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("### 📄 Internal Documents (%d chunks)\n\n", len(sources)))

	for _, source := range sources {
		usedIcon := "✅"
		if !source.Used {
			usedIcon = "⭕"
		}

		builder.WriteString(fmt.Sprintf("- %s **%s**", usedIcon, source.Title))

		if config.ShowConfidenceScores {
			builder.WriteString(fmt.Sprintf(" (confidence: %.2f)", source.Confidence))
		}

		builder.WriteString("\n")
	}

	builder.WriteString("\n")
	return builder.String()
}

// formatWebSourcesMarkdown formats web search sources as Markdown
func formatWebSourcesMarkdown(sources []synth.WebSourceInfo, config DisplayConfig) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("### 🌐 Web Search Results (%d articles)\n\n", len(sources)))

	for _, source := range sources {
		usedIcon := "✅"
		if !source.Used {
			usedIcon = "⭕"
		}

		builder.WriteString(fmt.Sprintf("- %s [%s](%s)", usedIcon, source.Title, source.URL))

		if config.ShowConfidenceScores {
			builder.WriteString(fmt.Sprintf(" (confidence: %.2f)", source.Confidence))
		}

		builder.WriteString("\n")
	}

	builder.WriteString("\n")
	return builder.String()
}

// formatLLMSynthesisMarkdown formats LLM synthesis information as Markdown
func formatLLMSynthesisMarkdown(stats synth.ProcessingStats) string {
	return fmt.Sprintf(
		"### 🤖 LLM Synthesis\n\n- **Model**: %s\n- **Tokens**: %d input / %d output\n- **Temperature**: %.1f\n\n",
		stats.ModelUsed,
		stats.InputTokens,
		stats.OutputTokens,
		stats.Temperature,
	)
}

// formatPipelineVisibility creates formatted pipeline decision information
func formatPipelineVisibility(response synth.SynthesisResponse, config DisplayConfig) PipelineVisibility {
	if !config.ShowPipelineDecisions {
		return PipelineVisibility{}
	}

	pipeline := response.PipelineDecision

	var htmlBuilder, textBuilder, markdownBuilder strings.Builder

	// HTML version
	htmlBuilder.WriteString(`<div class="pipeline-visibility">`)
	htmlBuilder.WriteString(`<h4>🔍 Pipeline Decisions</h4>`)
	htmlBuilder.WriteString(`<div class="pipeline-info">`)

	htmlBuilder.WriteString(fmt.Sprintf(`<div class="query-analysis">Query Type: <span class="query-type">%s</span></div>`, pipeline.QueryType))

	if len(pipeline.MetadataFiltersApplied) > 0 {
		htmlBuilder.WriteString(fmt.Sprintf(`<div class="filters">Metadata Filters: %s</div>`, strings.Join(pipeline.MetadataFiltersApplied, ", ")))
	}

	if pipeline.FallbackSearchUsed {
		htmlBuilder.WriteString(`<div class="fallback">🔄 Fallback search used due to insufficient initial results</div>`)
	}

	if pipeline.WebSearchTriggered {
		htmlBuilder.WriteString(`<div class="web-search">🌐 Web search triggered for fresh information</div>`)
		if len(pipeline.FreshnessKeywords) > 0 {
			htmlBuilder.WriteString(fmt.Sprintf(`<div class="freshness">Freshness keywords: %s</div>`, strings.Join(pipeline.FreshnessKeywords, ", ")))
		}
	}

	htmlBuilder.WriteString(fmt.Sprintf(`<div class="context-stats">Context: %d items filtered → %d used</div>`, pipeline.ContextItemsFiltered, pipeline.ContextItemsUsed))

	if pipeline.Reasoning != "" {
		htmlBuilder.WriteString(fmt.Sprintf(`<div class="reasoning">Reasoning: %s</div>`, pipeline.Reasoning))
	}

	htmlBuilder.WriteString(`</div></div>`)

	// Text version
	textBuilder.WriteString("🔍 Pipeline Decisions:\n")
	textBuilder.WriteString(fmt.Sprintf("- Query Type: %s\n", pipeline.QueryType))

	if len(pipeline.MetadataFiltersApplied) > 0 {
		textBuilder.WriteString(fmt.Sprintf("- Metadata Filters: %s\n", strings.Join(pipeline.MetadataFiltersApplied, ", ")))
	}

	if pipeline.FallbackSearchUsed {
		textBuilder.WriteString("- Fallback search used\n")
	}

	if pipeline.WebSearchTriggered {
		textBuilder.WriteString("- Web search triggered\n")
	}

	textBuilder.WriteString(fmt.Sprintf("- Context: %d filtered → %d used\n", pipeline.ContextItemsFiltered, pipeline.ContextItemsUsed))

	// Markdown version
	markdownBuilder.WriteString("## 🔍 Pipeline Decisions\n\n")
	markdownBuilder.WriteString(fmt.Sprintf("- **Query Type**: %s\n", pipeline.QueryType))

	if len(pipeline.MetadataFiltersApplied) > 0 {
		markdownBuilder.WriteString(fmt.Sprintf("- **Metadata Filters**: %s\n", strings.Join(pipeline.MetadataFiltersApplied, ", ")))
	}

	if pipeline.FallbackSearchUsed {
		markdownBuilder.WriteString("- **Fallback Search**: Used due to insufficient initial results\n")
	}

	if pipeline.WebSearchTriggered {
		markdownBuilder.WriteString("- **Web Search**: Triggered for fresh information\n")
		if len(pipeline.FreshnessKeywords) > 0 {
			markdownBuilder.WriteString(fmt.Sprintf("- **Freshness Keywords**: %s\n", strings.Join(pipeline.FreshnessKeywords, ", ")))
		}
	}

	markdownBuilder.WriteString(fmt.Sprintf("- **Context Usage**: %d items filtered → %d used\n\n", pipeline.ContextItemsFiltered, pipeline.ContextItemsUsed))

	return PipelineVisibility{
		HTML:     htmlBuilder.String(),
		Text:     textBuilder.String(),
		Markdown: markdownBuilder.String(),
	}
}

// formatProcessingStats creates formatted processing statistics
func formatProcessingStats(stats synth.ProcessingStats, config DisplayConfig) ProcessingStatsView {
	if !config.ShowProcessingTime && !config.ShowTokenUsage {
		return ProcessingStatsView{}
	}

	result := ProcessingStatsView{}

	if config.ShowProcessingTime {
		totalTime := time.Duration(stats.TotalProcessingTime) * time.Millisecond
		result.TotalTime = formatDuration(totalTime)
		result.PerformanceBar = createPerformanceBar(stats)
	}

	if config.ShowTokenUsage {
		result.TokenUsage = fmt.Sprintf("%d input + %d output = %d total tokens",
			stats.InputTokens, stats.OutputTokens, stats.TotalTokens)

		if stats.EstimatedCost > 0 {
			result.EstimatedCost = fmt.Sprintf("$%.4f", stats.EstimatedCost)
		}
	}

	result.ModelInfo = fmt.Sprintf("%s (temp: %.1f)", stats.ModelUsed, stats.Temperature)

	return result
}

// calculateTrustIndicators calculates trust and quality indicators
func calculateTrustIndicators(response synth.SynthesisResponse) TrustIndicators {
	indicators := TrustIndicators{
		TrustBadges: []string{},
	}

	// Calculate source quality score
	sourceQuality := 0.0
	totalSources := 0

	for _, source := range response.ContextSources {
		sourceQuality += source.Confidence
		totalSources++
	}

	for _, source := range response.WebSources {
		sourceQuality += source.Confidence
		totalSources++
	}

	if totalSources > 0 {
		indicators.SourceQuality = sourceQuality / float64(totalSources)
	}

	// Calculate overall trust score
	indicators.OverallScore = indicators.SourceQuality

	// Add trust badges based on quality
	if indicators.SourceQuality >= 0.8 {
		indicators.TrustBadges = append(indicators.TrustBadges, "High Quality Sources")
	}

	if len(response.ContextSources) > 0 {
		indicators.TrustBadges = append(indicators.TrustBadges, "Internal Documentation")
	}

	if response.PipelineDecision.WebSearchTriggered {
		indicators.TrustBadges = append(indicators.TrustBadges, "Fresh Information")
		indicators.Freshness = "Recent"
	} else {
		indicators.Freshness = "Standard"
	}

	// Determine confidence level
	switch {
	case indicators.OverallScore >= 0.8:
		indicators.ConfidenceLevel = "High"
	case indicators.OverallScore >= 0.6:
		indicators.ConfidenceLevel = "Medium"
	default:
		indicators.ConfidenceLevel = "Low"
	}

	return indicators
}

// formatDuration formats a duration in a human-readable format
func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return "< 1ms"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// createPerformanceBar creates a visual performance indicator
func createPerformanceBar(stats synth.ProcessingStats) string {
	total := stats.TotalProcessingTime
	if total == 0 {
		return ""
	}

	retrieval := float64(stats.RetrievalTime) / float64(total) * 100
	websearch := float64(stats.WebSearchTime) / float64(total) * 100
	synthesis := float64(stats.SynthesisTime) / float64(total) * 100

	return fmt.Sprintf("Retrieval: %.0f%% | Web Search: %.0f%% | Synthesis: %.0f%%",
		retrieval, websearch, synthesis)
}
