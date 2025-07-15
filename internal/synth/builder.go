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

// Package synth provides synthesis functionality for the AI SA Assistant.
// It handles prompt building, LLM response parsing, and content extraction
// including Mermaid diagrams, code snippets, and source citations.
package synth

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/your-org/ai-sa-assistant/internal/session"
)

// ContextItem represents a piece of context with its source
type ContextItem struct {
	Content  string  `json:"content"`
	SourceID string  `json:"source_id"`
	Score    float64 `json:"score,omitempty"`
	Priority int     `json:"priority,omitempty"`
}

// QueryType represents the type of query for optimization
type QueryType int

const (
	// TechnicalQuery indicates queries focused on implementation and architecture
	TechnicalQuery QueryType = iota
	// BusinessQuery indicates queries focused on cost, ROI, and business value
	BusinessQuery
	// GeneralQuery indicates general-purpose queries
	GeneralQuery
)

// PromptConfig holds configuration for prompt generation
type PromptConfig struct {
	MaxTokens       int
	MaxContextItems int
	MaxWebResults   int
	QueryType       QueryType
}

// Configuration constants
const (
	DefaultMaxTokens        = 6000
	DefaultMaxContextItems  = 10
	DefaultMaxWebResults    = 5
	DefaultMaxHistoryTokens = 1500 // Reserve 1500 tokens for conversation history
	MinimalPromptLength     = 100
	TokenEstimateRatio      = 4
	TruncationSafetyRatio   = 0.9
	MinCodeMatchGroups      = 3
	MinSourceMatchGroups    = 2
	MinURLLength            = 10
)

// DefaultPromptConfig returns default configuration
func DefaultPromptConfig() PromptConfig {
	return PromptConfig{
		MaxTokens:       DefaultMaxTokens,
		MaxContextItems: DefaultMaxContextItems,
		MaxWebResults:   DefaultMaxWebResults,
		QueryType:       GeneralQuery,
	}
}

// SynthesisRequest represents a request for synthesis
type SynthesisRequest struct {
	Query      string        `json:"query"`
	Context    []ContextItem `json:"context"`
	WebResults []string      `json:"web_results"`
}

// SynthesisResponse represents the structured response from synthesis
type SynthesisResponse struct {
	MainText         string               `json:"main_text"`
	DiagramCode      string               `json:"diagram_code"`
	DiagramURL       string               `json:"diagram_url,omitempty"`
	CodeSnippets     []CodeSnippet        `json:"code_snippets"`
	Sources          []string             `json:"sources"`
	ContextSources   []ContextSourceInfo  `json:"context_sources,omitempty"`
	WebSources       []WebSourceInfo      `json:"web_sources,omitempty"`
	ProcessingStats  ProcessingStats      `json:"processing_stats,omitempty"`
	PipelineDecision PipelineDecisionInfo `json:"pipeline_decision,omitempty"`
}

// CodeSnippet represents a code snippet with its language
type CodeSnippet struct {
	Language string `json:"language"`
	Code     string `json:"code"`
}

// ContextSourceInfo represents detailed information about a context source
type ContextSourceInfo struct {
	SourceID   string  `json:"source_id"`
	Title      string  `json:"title,omitempty"`
	Confidence float64 `json:"confidence"`
	Relevance  float64 `json:"relevance"`
	ChunkIndex int     `json:"chunk_index"`
	Preview    string  `json:"preview"`
	SourceType string  `json:"source_type"` // "internal_doc", "runbook", "playbook", etc.
	TokenCount int     `json:"token_count"`
	Used       bool    `json:"used"` // Whether this source was actually cited in the response
}

// WebSourceInfo represents detailed information about a web search source
type WebSourceInfo struct {
	URL        string  `json:"url"`
	Title      string  `json:"title,omitempty"`
	Snippet    string  `json:"snippet,omitempty"`
	Confidence float64 `json:"confidence"`
	Freshness  string  `json:"freshness,omitempty"`
	Domain     string  `json:"domain"`
	Used       bool    `json:"used"` // Whether this source was actually cited in the response
}

// ProcessingStats represents processing time and token usage statistics
type ProcessingStats struct {
	TotalProcessingTime int     `json:"total_processing_time_ms"`
	RetrievalTime       int     `json:"retrieval_time_ms"`
	WebSearchTime       int     `json:"web_search_time_ms"`
	SynthesisTime       int     `json:"synthesis_time_ms"`
	InputTokens         int     `json:"input_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	TotalTokens         int     `json:"total_tokens"`
	EstimatedCost       float64 `json:"estimated_cost_usd,omitempty"`
	ModelUsed           string  `json:"model_used"`
	Temperature         float64 `json:"temperature"`
}

// PipelineDecisionInfo represents information about pipeline decisions made during processing
type PipelineDecisionInfo struct {
	MetadataFiltersApplied []string `json:"metadata_filters_applied,omitempty"`
	FallbackSearchUsed     bool     `json:"fallback_search_used"`
	WebSearchTriggered     bool     `json:"web_search_triggered"`
	FreshnessKeywords      []string `json:"freshness_keywords,omitempty"`
	QueryType              string   `json:"query_type"`
	ArchitectureDiagram    bool     `json:"architecture_diagram_generated"`
	CodeGenerated          bool     `json:"code_generated"`
	ContextItemsFiltered   int      `json:"context_items_filtered"`
	ContextItemsUsed       int      `json:"context_items_used"`
	Reasoning              string   `json:"reasoning,omitempty"`
}

// BuildPrompt combines context into a comprehensive prompt for the LLM
func BuildPrompt(query string, contextItems []ContextItem, webResults []string) string {
	config := DefaultPromptConfig()
	config.QueryType = DetectQueryType(query)
	return BuildPromptWithConfig(query, contextItems, webResults, config)
}

// BuildPromptWithConversation combines context and conversation history into a comprehensive prompt for the LLM
func BuildPromptWithConversation(
	query string,
	contextItems []ContextItem,
	webResults []string,
	conversationHistory []session.Message,
) string {
	config := DefaultPromptConfig()
	config.QueryType = DetectQueryType(query)
	return BuildPromptWithConversationAndConfig(
		query, contextItems, webResults, conversationHistory, config,
	)
}

// PromptMessages represents separate system and user messages
type PromptMessages struct {
	SystemMessage string
	UserMessage   string
}

// BuildPromptMessages creates separate system and user messages with proper structure
func BuildPromptMessages(query string, contextItems []ContextItem, webResults []string) PromptMessages {
	config := DefaultPromptConfig()
	config.QueryType = DetectQueryType(query)
	return BuildPromptMessagesWithConfig(query, contextItems, webResults, config)
}

// BuildPromptMessagesWithConfig creates separate system and user messages with configuration
func BuildPromptMessagesWithConfig(query string, contextItems []ContextItem, webResults []string, config PromptConfig) PromptMessages {
	// Validate and deduplicate sources before processing
	validatedContext, err := ValidateAndDeduplicateSources(contextItems)
	if err != nil {
		// Log warning but continue with original context if validation fails
		validatedContext = contextItems
	}

	// Prioritize and limit context based on token constraints
	optimizedContext := PrioritizeContext(validatedContext, config.MaxContextItems)
	limitedWebResults := LimitWebResults(webResults, config.MaxWebResults)

	// Build system message
	systemMessage := buildSystemPrompt(config.QueryType)

	// Build user message
	var userMessage strings.Builder

	// User query with enhanced contextual emphasis
	userMessage.WriteString(fmt.Sprintf("User Query: %s\n\n", query))

	// Add contextual parameter extraction instructions
	userMessage.WriteString(buildContextualParameterInstructions(query))

	// Internal document context
	if len(optimizedContext) > 0 {
		userMessage.WriteString("--- Internal Document Context (PRIMARY SOURCE) ---\n")
		userMessage.WriteString("The following context chunks contain the most relevant and authoritative information for this query.\n")
		userMessage.WriteString("Base your response PRIMARILY on this context. Reference these chunks throughout your response.\n\n")
		for i, item := range optimizedContext {
			userMessage.WriteString(fmt.Sprintf("Context %d [%s]: %s\n\n", i+1, item.SourceID, item.Content))
		}
	}

	// Web search results with enhanced URL tracking
	if len(limitedWebResults) > 0 {
		userMessage.WriteString("--- Live Web Search Results ---\n")
		for i, result := range limitedWebResults {
			formattedResult := formatWebResultWithURL(i+1, result)
			userMessage.WriteString(formattedResult)
		}
	}

	// Add enhanced citation instructions
	userMessage.WriteString(buildEnhancedCitationInstructions())

	userMessage.WriteString("\nPlease provide your comprehensive response now:")

	finalUserMessage := userMessage.String()

	// Ensure token limits are respected (split between system and user messages)
	totalTokens := EstimateTokens(systemMessage) + EstimateTokens(finalUserMessage)
	if totalTokens > config.MaxTokens {
		// Reserve tokens for system message and truncate user message if needed
		systemTokens := EstimateTokens(systemMessage)
		availableUserTokens := config.MaxTokens - systemTokens
		if availableUserTokens > 0 {
			finalUserMessage = TruncateToTokenLimit(finalUserMessage, availableUserTokens)
		}
	}

	return PromptMessages{
		SystemMessage: systemMessage,
		UserMessage:   finalUserMessage,
	}
}

// ValidatePromptMessages validates that the messages structure is correct
func ValidatePromptMessages(messages PromptMessages) error {
	if strings.TrimSpace(messages.SystemMessage) == "" {
		return fmt.Errorf("system message cannot be empty")
	}

	if strings.TrimSpace(messages.UserMessage) == "" {
		return fmt.Errorf("user message cannot be empty")
	}

	// Validate that system message contains required SA persona
	if !strings.Contains(messages.SystemMessage, "Solutions Architect") {
		return fmt.Errorf("system message must contain Solutions Architect persona")
	}

	// Validate that user message contains the query
	if !strings.Contains(messages.UserMessage, "User Query:") {
		return fmt.Errorf("user message must contain user query")
	}

	// Validate that citation instructions are present
	if !strings.Contains(messages.UserMessage, "[source_id]") {
		return fmt.Errorf("user message must contain citation instructions")
	}

	return nil
}

// BuildPromptWithConfig combines context into a comprehensive prompt with configuration
// DEPRECATED: Use BuildPromptMessagesWithConfig instead for proper OpenAI API message structure
func BuildPromptWithConfig(query string, contextItems []ContextItem, webResults []string, config PromptConfig) string {
	messages := BuildPromptMessagesWithConfig(query, contextItems, webResults, config)
	return messages.SystemMessage + "\n\n" + messages.UserMessage
}

// BuildPromptWithConversationAndConfig combines context and conversation history
// into a comprehensive prompt with intelligent token allocation
func BuildPromptWithConversationAndConfig(
	query string,
	contextItems []ContextItem,
	webResults []string,
	conversationHistory []session.Message,
	config PromptConfig,
) string {
	// Validate and deduplicate sources before processing
	validatedContext, err := ValidateAndDeduplicateSources(contextItems)
	if err != nil {
		// Log warning but continue with original context if validation fails
		validatedContext = contextItems
	}

	// Calculate token allocation intelligently
	allocation := calculateTokenAllocation(config.MaxTokens, len(conversationHistory) > 0)

	// Build system prompt first to get baseline token usage
	systemPrompt := buildSystemPrompt(config.QueryType)
	baseTokens := EstimateTokens(systemPrompt + fmt.Sprintf("Current User Query: %s\n\n", query) + "Please provide your comprehensive response now:")

	// Allocate remaining tokens
	remainingTokens := config.MaxTokens - baseTokens
	if remainingTokens <= 0 {
		// If base prompt is too large, return minimal version
		return systemPrompt + fmt.Sprintf("User Query: %s\n\nPlease provide your response:", query)
	}

	var prompt strings.Builder
	prompt.WriteString(systemPrompt)

	// Add current user query with contextual parameter instructions
	prompt.WriteString(fmt.Sprintf("Current User Query: %s\n\n", query))
	prompt.WriteString(buildContextualParameterInstructions(query))

	// Add conversation history with allocated tokens
	conversationTokens := 0
	if len(conversationHistory) > 0 && allocation.ConversationTokens > 0 {
		prompt.WriteString("--- Previous Conversation Context ---\n")
		conversationContext := formatConversationHistoryWithTokenLimit(conversationHistory, allocation.ConversationTokens)
		prompt.WriteString(conversationContext)
		prompt.WriteString("\n")
		conversationTokens = EstimateTokens(conversationContext)
	}

	// Adjust context allocation based on actual conversation usage
	actualContextTokens := allocation.ContextTokens + (allocation.ConversationTokens - conversationTokens)
	if actualContextTokens < 0 {
		actualContextTokens = allocation.ContextTokens
	}

	// User query
	prompt.WriteString(fmt.Sprintf("Current User Query: %s\n\n", query))

	// Add context items within token limit
	if len(validatedContext) > 0 && actualContextTokens > 0 {
		contextSection := buildContextSectionWithTokenLimit(validatedContext, actualContextTokens)
		prompt.WriteString(contextSection)
	}

	// Add web results within remaining token budget
	if len(webResults) > 0 {
		remainingBudget := remainingTokens - EstimateTokens(prompt.String()) - 100 // Reserve some buffer
		if remainingBudget > 200 {                                                 // Only add web results if we have reasonable space
			webSection := buildWebResultsSectionWithTokenLimit(webResults, remainingBudget, config.MaxWebResults)
			prompt.WriteString(webSection)
		}
	}

	// Add conversation continuity instructions
	if len(conversationHistory) > 0 {
		prompt.WriteString("\nIMPORTANT: This is a continuation of an ongoing conversation. Please:\n")
		prompt.WriteString("- Reference previous context when relevant\n")
		prompt.WriteString("- Build upon earlier discussions\n")
		prompt.WriteString("- Maintain conversation continuity\n")
		prompt.WriteString("- Use phrases like 'As we discussed earlier' when appropriate\n\n")
	}

	// Add enhanced citation instructions
	prompt.WriteString(buildEnhancedCitationInstructions())

	prompt.WriteString("Please provide your comprehensive response now:")

	// Final safety check and truncation if needed
	finalPrompt := prompt.String()
	if EstimateTokens(finalPrompt) > config.MaxTokens {
		finalPrompt = TruncateToTokenLimit(finalPrompt, config.MaxTokens)
	}

	return finalPrompt
}

// TokenAllocation represents how tokens should be allocated across prompt sections
type TokenAllocation struct {
	ConversationTokens int
	ContextTokens      int
	WebResultsTokens   int
	SystemTokens       int
	BufferTokens       int
}

// calculateTokenAllocation calculates optimal token allocation based on prompt requirements
func calculateTokenAllocation(maxTokens int, hasConversationHistory bool) TokenAllocation {
	// Reserve tokens for system prompt and buffer
	systemTokens := maxTokens / 4  // ~25% for system instructions
	bufferTokens := maxTokens / 10 // ~10% buffer for final response instruction
	availableTokens := maxTokens - systemTokens - bufferTokens

	if !hasConversationHistory {
		// No conversation history - allocate more to context
		return TokenAllocation{
			ConversationTokens: 0,
			ContextTokens:      int(float64(availableTokens) * 0.8), // 80% to context
			WebResultsTokens:   int(float64(availableTokens) * 0.2), // 20% to web results
			SystemTokens:       systemTokens,
			BufferTokens:       bufferTokens,
		}
	}

	// With conversation history - balanced allocation
	return TokenAllocation{
		ConversationTokens: DefaultMaxHistoryTokens,
		ContextTokens:      int(float64(availableTokens-DefaultMaxHistoryTokens) * 0.7), // 70% of remaining to context
		WebResultsTokens:   int(float64(availableTokens-DefaultMaxHistoryTokens) * 0.3), // 30% of remaining to web
		SystemTokens:       systemTokens,
		BufferTokens:       bufferTokens,
	}
}

// buildContextSectionWithTokenLimit builds context section within token limit
func buildContextSectionWithTokenLimit(contextItems []ContextItem, maxTokens int) string {
	if len(contextItems) == 0 || maxTokens <= 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("--- Internal Document Context (PRIMARY SOURCE) ---\n")
	builder.WriteString("The following context chunks contain the most relevant and authoritative information for this query.\n")
	builder.WriteString("Base your response PRIMARILY on this context. Reference these chunks throughout your response.\n\n")
	currentTokens := EstimateTokens(builder.String())

	includedItems := 0
	for i, item := range contextItems {
		contextEntry := fmt.Sprintf("Context %d [%s]: %s\n\n", i+1, item.SourceID, item.Content)
		entryTokens := EstimateTokens(contextEntry)

		if currentTokens+entryTokens > maxTokens {
			// Try to include a truncated version if it's the first item and we have reasonable space
			if includedItems == 0 && maxTokens-currentTokens > 200 {
				availableTokens := maxTokens - currentTokens - EstimateTokens(fmt.Sprintf("Context %d [%s]: \n\n", i+1, item.SourceID))
				truncatedContent := truncateMessageContentToTokens(item.Content, availableTokens)
				builder.WriteString(fmt.Sprintf("Context %d [%s]: %s\n\n", i+1, item.SourceID, truncatedContent))
				includedItems++
			}
			break
		}

		builder.WriteString(contextEntry)
		currentTokens += entryTokens
		includedItems++
	}

	if includedItems == 0 {
		return ""
	}

	return builder.String()
}

// buildWebResultsSectionWithTokenLimit builds web results section within token limit
func buildWebResultsSectionWithTokenLimit(webResults []string, maxTokens int, maxResults int) string {
	if len(webResults) == 0 || maxTokens <= 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("--- Live Web Search Results ---\n")
	currentTokens := EstimateTokens(builder.String())

	limitedResults := LimitWebResults(webResults, maxResults)
	includedResults := 0

	for i, result := range limitedResults {
		formattedResult := formatWebResultWithURL(i+1, result)
		resultTokens := EstimateTokens(formattedResult)

		if currentTokens+resultTokens > maxTokens {
			break
		}

		builder.WriteString(formattedResult)
		currentTokens += resultTokens
		includedResults++
	}

	if includedResults == 0 {
		return ""
	}

	return builder.String()
}

// ParseResponse parses the LLM response into structured components
func ParseResponse(response string) SynthesisResponse {
	return ParseResponseWithSources(response, []string{})
}

// ParseResponseWithSources parses the LLM response into structured components with source validation
func ParseResponseWithSources(response string, availableSources []string) SynthesisResponse {
	return ParseResponseWithEnhancedMetadata(response, availableSources, nil, nil, ProcessingStats{}, PipelineDecisionInfo{}, "")
}

// ParseResponseWithQuery parses the LLM response with fallback diagram generation based on the query
func ParseResponseWithQuery(response string, availableSources []string, query string) SynthesisResponse {
	return ParseResponseWithEnhancedMetadata(response, availableSources, nil, nil, ProcessingStats{}, PipelineDecisionInfo{}, query)
}

// GenerateFallbackDiagram generates a basic fallback diagram when the LLM fails to produce one
func GenerateFallbackDiagram(query string) string {
	queryLower := strings.ToLower(query)

	// Check if this is an architecture query that warrants a diagram
	// Exclude business-only queries even if they contain architecture keywords
	if !DetectArchitectureQuery(query) || IsBusinessOnlyQuery(query) {
		return ""
	}

	// Generate fallback based on query characteristics
	if strings.Contains(queryLower, "aws") {
		return generateAWSFallbackDiagram(query)
	} else if strings.Contains(queryLower, "azure") {
		return generateAzureFallbackDiagram(query)
	} else if strings.Contains(queryLower, "migration") {
		return generateMigrationFallbackDiagram(query)
	} else if strings.Contains(queryLower, "disaster recovery") || strings.Contains(queryLower, "dr") {
		return generateDRFallbackDiagram(query)
	}

	// Generic cloud architecture fallback
	return generateGenericCloudFallbackDiagram(query)
}

// generateAWSFallbackDiagram generates a basic AWS architecture diagram
func generateAWSFallbackDiagram(query string) string {
	return `graph TD
    subgraph "AWS Cloud"
        subgraph "VPC"
            subgraph "Public Subnet"
                LB[Load Balancer]
                NAT[NAT Gateway]
            end
            subgraph "Private Subnet"
                APP[Application Servers]
                DB[Database]
            end
        end
        S3[S3 Storage]
    end
    Users[Users] --> LB
    LB --> APP
    APP --> DB
    APP --> S3`
}

// generateAzureFallbackDiagram generates a basic Azure architecture diagram
func generateAzureFallbackDiagram(query string) string {
	return `graph TD
    subgraph "Azure Subscription"
        subgraph "Resource Group"
            subgraph "Virtual Network"
                subgraph "Public Subnet"
                    AG[Application Gateway]
                    LB[Load Balancer]
                end
                subgraph "Private Subnet"
                    VM[Virtual Machines]
                    SQL[Azure SQL Database]
                end
            end
            Storage[Storage Account]
        end
    end
    Users[Users] --> AG
    AG --> VM
    VM --> SQL
    VM --> Storage`
}

// generateMigrationFallbackDiagram generates a basic migration diagram
func generateMigrationFallbackDiagram(query string) string {
	return `graph TD
    subgraph "On-Premises"
        OnPrem[Legacy Infrastructure]
        Data[Existing Data]
    end
    
    subgraph "Cloud"
        subgraph "Migration Services"
            MGN[Migration Service]
            DataSync[Data Sync]
        end
        subgraph "Target Environment"
            Cloud[Cloud Infrastructure]
            CloudData[Cloud Storage]
        end
    end
    
    OnPrem --> MGN
    Data --> DataSync
    MGN --> Cloud
    DataSync --> CloudData`
}

// generateDRFallbackDiagram generates a basic disaster recovery diagram
func generateDRFallbackDiagram(query string) string {
	return `graph TD
    subgraph "Primary Region"
        Primary[Primary Infrastructure]
        PrimaryDB[Primary Database]
    end
    
    subgraph "DR Region"
        DR[DR Infrastructure]
        DRDB[DR Database]
    end
    
    subgraph "Backup Storage"
        Backup[Backup Storage]
    end
    
    Primary --> DR
    PrimaryDB -.-> DRDB
    Primary --> Backup
    PrimaryDB --> Backup`
}

// generateGenericCloudFallbackDiagram generates a generic cloud architecture diagram
func generateGenericCloudFallbackDiagram(query string) string {
	return `graph TD
    subgraph "Cloud Platform"
        subgraph "Network Layer"
            LB[Load Balancer]
            FW[Firewall]
        end
        subgraph "Compute Layer"
            APP[Application Tier]
            API[API Gateway]
        end
        subgraph "Data Layer"
            DB[Database]
            Cache[Cache]
        end
        subgraph "Storage Layer"
            Storage[Object Storage]
            Files[File Storage]
        end
    end
    
    Users[Users] --> LB
    LB --> APP
    APP --> API
    API --> DB
    API --> Cache
    APP --> Storage
    DB --> Files`
}

// ParseResponseWithEnhancedMetadata parses the LLM response with full metadata including context sources, web sources, and processing stats
func ParseResponseWithEnhancedMetadata(
	response string,
	availableSources []string,
	contextItems []ContextItem,
	webResults []string,
	stats ProcessingStats,
	pipelineInfo PipelineDecisionInfo,
	query string,
) SynthesisResponse {
	result := SynthesisResponse{
		MainText:         response,
		CodeSnippets:     []CodeSnippet{},
		Sources:          []string{},
		ContextSources:   []ContextSourceInfo{},
		WebSources:       []WebSourceInfo{},
		ProcessingStats:  stats,
		PipelineDecision: pipelineInfo,
	}

	// Extract diagram code
	if diagramCode := extractMermaidDiagram(response); diagramCode != "" {
		result.DiagramCode = diagramCode
		result.PipelineDecision.ArchitectureDiagram = true
		// Remove diagram from main text
		result.MainText = removeMermaidDiagram(response)
	} else if query != "" {
		// Generate fallback diagram if LLM didn't produce one
		if fallbackDiagram := GenerateFallbackDiagram(query); fallbackDiagram != "" {
			result.DiagramCode = fallbackDiagram
			result.PipelineDecision.ArchitectureDiagram = true
		}
	}

	// Extract code snippets
	codeSnippets := extractCodeSnippets(response)
	result.CodeSnippets = codeSnippets
	result.PipelineDecision.CodeGenerated = len(codeSnippets) > 0

	// Validate code generation for migration queries
	if query != "" {
		validateCodeGeneration(query, codeSnippets)
	}

	// Remove code snippets from main text
	result.MainText = removeCodeSnippets(result.MainText)

	// Extract sources from citations
	citedSources := extractSources(response)

	// Build context source information
	if contextItems != nil {
		result.ContextSources = buildContextSourceInfo(contextItems, citedSources)
	}

	// Build web source information
	if webResults != nil {
		result.WebSources = buildWebSourceInfo(webResults, citedSources)
	}

	// Validate and filter sources to only include those that are available
	validSources := make([]string, 0)
	if len(availableSources) > 0 {
		availableSourceMap := make(map[string]bool)
		for _, source := range availableSources {
			availableSourceMap[source] = true
		}

		for _, source := range citedSources {
			if availableSourceMap[source] {
				validSources = append(validSources, source)
			}
		}
	} else {
		validSources = citedSources
	}

	result.Sources = uniqueStrings(validSources)

	// Clean up main text
	result.MainText = strings.TrimSpace(result.MainText)

	return result
}

// extractMermaidDiagram extracts Mermaid diagram code from the response
func extractMermaidDiagram(response string) string {
	// Look for mermaid code blocks
	mermaidRegex := regexp.MustCompile("```mermaid\\s*\\n([\\s\\S]*?)\\n```")
	matches := mermaidRegex.FindStringSubmatch(response)

	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Also try without language identifier
	mermaidRegex2 := regexp.MustCompile("```\\s*\\n(graph\\s+TD[\\s\\S]*?)\\n```")
	matches2 := mermaidRegex2.FindStringSubmatch(response)

	if len(matches2) > 1 {
		return strings.TrimSpace(matches2[1])
	}

	return ""
}

// removeMermaidDiagram removes Mermaid diagram blocks from text
func removeMermaidDiagram(text string) string {
	mermaidRegex := regexp.MustCompile("```mermaid\\s*\\n[\\s\\S]*?\\n```")
	text = mermaidRegex.ReplaceAllString(text, "")

	mermaidRegex2 := regexp.MustCompile("```\\s*\\n(graph\\s+TD[\\s\\S]*?)\\n```")
	text = mermaidRegex2.ReplaceAllString(text, "")

	return text
}

// extractCodeSnippets extracts code snippets from the response with security validation
func extractCodeSnippets(response string) []CodeSnippet {
	var snippets []CodeSnippet

	// Multiple regex patterns to handle different code block formats
	codeRegexes := []*regexp.Regexp{
		// Standard format: ```language\ncode\n```
		regexp.MustCompile("```([\\w.-]+)\\s*\\r?\\n([\\s\\S]*?)\\r?\\n```"),
		// Format with trailing whitespace: ```language\ncode\n```\n
		regexp.MustCompile("```([\\w.-]+)\\s*\\r?\\n([\\s\\S]*?)\\r?\\n```\\s*"),
		// Format with extra spaces around language: ```  language  \ncode\n```
		regexp.MustCompile("```\\s+([\\w.-]+)\\s+\\r?\\n([\\s\\S]*?)\\r?\\n```"),
		// Format with language and content on same line: ```language content```
		// This pattern allows for language followed by content with optional whitespace
		regexp.MustCompile("```([\\w.-]+)\\s+([\\s\\S]*?)\\s*```"),
	}

	// Track processed code blocks to avoid duplicates
	processedHashes := make(map[string]bool)

	for _, codeRegex := range codeRegexes {
		matches := codeRegex.FindAllStringSubmatch(response, -1)

		for _, match := range matches {
			if len(match) >= MinCodeMatchGroups {
				language := strings.TrimSpace(match[1])
				code := strings.TrimSpace(match[2])

				// Normalize line endings (Windows \r\n to Unix \n)
				code = strings.ReplaceAll(code, "\r\n", "\n")

				// Skip mermaid blocks (handled separately) and ensure language is not empty
				if language != "mermaid" && language != "" && code != "" {
					// Create hash to avoid duplicates
					codeHash := fmt.Sprintf("%s:%s", language, code[:min(len(code), 50)])
					if processedHashes[codeHash] {
						continue
					}
					processedHashes[codeHash] = true

					// Validate code for security issues
					if validateCodeSecurity(code, language) {
						// Normalize language identifier
						normalizedLanguage := normalizeLanguage(language)
						snippets = append(snippets, CodeSnippet{
							Language: normalizedLanguage,
							Code:     code,
						})
					}
				}
			}
		}
	}

	return snippets
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// removeCodeSnippets removes code blocks from text
func removeCodeSnippets(text string) string {
	// Multiple regex patterns to handle different code block formats
	codeRegexes := []*regexp.Regexp{
		// Standard format: ```language\ncode\n```
		regexp.MustCompile("```\\w*\\s*\\n[\\s\\S]*?\\n```"),
		// Format with optional whitespace: ```language code ```
		regexp.MustCompile("```\\w*\\s+[\\s\\S]*?\\s*```"),
		// Format without language on same line: ```\nlanguage\ncode\n```
		regexp.MustCompile("```\\s*\\n\\w*\\s*\\n[\\s\\S]*?\\n```"),
		// Format with trailing whitespace: ```language\ncode\n```\n
		regexp.MustCompile("```\\w*\\s*\\n[\\s\\S]*?\\n```\\s*"),
	}

	for _, codeRegex := range codeRegexes {
		text = codeRegex.ReplaceAllString(text, "")
	}

	return text
}

// extractSources extracts source citations from the response
func extractSources(response string) []string {
	var sources []string

	// Remove code blocks and mermaid diagrams first to avoid extracting node names
	textWithoutCode := removeCodeSnippets(response)
	textWithoutDiagrams := removeMermaidDiagram(textWithoutCode)

	// Regex to match [source_id] and [URL] patterns
	sourceRegex := regexp.MustCompile(`\[([^\]]+)\]`)
	matches := sourceRegex.FindAllStringSubmatch(textWithoutDiagrams, -1)

	for _, match := range matches {
		if len(match) >= MinSourceMatchGroups {
			source := strings.TrimSpace(match[1])
			if source != "" && isValidSourceCitation(source) {
				sources = append(sources, source)
			}
		}
	}

	return sources
}

// isValidSourceCitation checks if a citation is a valid source (not a diagram node or other bracket content)
func isValidSourceCitation(citation string) bool {
	// Skip common non-source brackets
	commonNonSources := []string{"graph", "subgraph", "end", "click", "style", "class", "classDef"}
	citationLower := strings.ToLower(citation)

	for _, nonSource := range commonNonSources {
		if citationLower == nonSource {
			return false
		}
	}

	// Skip if it looks like a Mermaid diagram node (single words or simple identifiers)
	if len(strings.Fields(citation)) == 1 && !strings.Contains(citation, ".") && !strings.Contains(citation, "/") {
		// Common technology/tool names are valid sources even if they're single words
		validSingleWordSources := []string{
			"terraform", "bash", "k8s", "kubernetes", "aws", "azure", "gcp", "docker",
			"ansible", "python", "java", "go", "rust", "typescript", "javascript",
		}
		for _, validSource := range validSingleWordSources {
			if citationLower == validSource {
				return true
			}
		}

		// If it's a single word without domain indicators, it's likely a diagram node
		// Unless it's a document ID pattern or URL
		if !strings.Contains(citation, "-") && !strings.Contains(citation, "_") && len(citation) < 20 {
			return false
		}
	}

	return true
}

// uniqueStrings removes duplicates from a string slice
func uniqueStrings(strings []string) []string {
	keys := make(map[string]bool)
	var unique []string

	for _, str := range strings {
		if !keys[str] {
			keys[str] = true
			unique = append(unique, str)
		}
	}

	return unique
}

// DetectFreshnessKeywords checks if the query contains keywords indicating need for fresh information
func DetectFreshnessKeywords(query string, keywords []string) bool {
	queryLower := strings.ToLower(query)

	for _, keyword := range keywords {
		if strings.Contains(queryLower, strings.ToLower(keyword)) {
			return true
		}
	}

	return false
}

// DetectQueryType analyzes the query to determine its type
func DetectQueryType(query string) QueryType {
	queryLower := strings.ToLower(query)

	// Technical query indicators
	technicalKeywords := []string{
		"architecture", "deploy", "configure", "implementation", "terraform",
		"aws", "azure", "gcp", "kubernetes", "docker", "microservices",
		"api", "database", "network", "security", "vpc", "subnet",
		"ec2", "s3", "lambda", "rds", "cloudformation", "ansible",
	}

	// Business query indicators
	businessKeywords := []string{
		"cost", "pricing", "roi", "budget", "savings", "business case",
		"timeline", "roadmap", "strategy", "compliance", "governance",
		"risk", "sla", "kpi", "metrics", "performance", "scalability",
	}

	technicalScore := 0
	businessScore := 0

	for _, keyword := range technicalKeywords {
		if strings.Contains(queryLower, keyword) {
			technicalScore++
		}
	}

	for _, keyword := range businessKeywords {
		if strings.Contains(queryLower, keyword) {
			businessScore++
		}
	}

	if technicalScore > businessScore {
		return TechnicalQuery
	} else if businessScore > technicalScore {
		return BusinessQuery
	}

	return GeneralQuery
}

// PrioritizeContext prioritizes and limits context items based on score and priority
func PrioritizeContext(contextItems []ContextItem, maxItems int) []ContextItem {
	// Sort by priority (higher first), then by score (higher first)
	sortedItems := make([]ContextItem, len(contextItems))
	copy(sortedItems, contextItems)

	// Simple bubble sort for priority and score
	for i := 0; i < len(sortedItems); i++ {
		for j := i + 1; j < len(sortedItems); j++ {
			if sortedItems[i].Priority < sortedItems[j].Priority ||
				(sortedItems[i].Priority == sortedItems[j].Priority && sortedItems[i].Score < sortedItems[j].Score) {
				sortedItems[i], sortedItems[j] = sortedItems[j], sortedItems[i]
			}
		}
	}

	// Return up to maxItems
	if len(sortedItems) > maxItems {
		return sortedItems[:maxItems]
	}
	return sortedItems
}

// LimitWebResults limits the number of web results
func LimitWebResults(webResults []string, maxResults int) []string {
	if len(webResults) <= maxResults {
		return webResults
	}
	return webResults[:maxResults]
}

// buildSystemPrompt creates system prompt based on query type
func buildSystemPrompt(queryType QueryType) string {
	basePrompt := `You are an expert Cloud Solutions Architect assistant. Your role is to help Solutions Architects with pre-sales research and planning.

CRITICAL CONTEXT PRIORITIZATION REQUIREMENTS:
- Your response MUST be based PRIMARILY on the provided Internal Document Context chunks
- The Internal Document Context contains the most relevant and authoritative information for this query
- PRIORITIZE information from the provided context chunks over your general knowledge
- Only supplement with general knowledge when the context is insufficient
- ALWAYS reference specific context chunks using [source_id] format throughout your response
- If the context contains specific details (VM counts, technologies, procedures), use those EXACT details
- Build your response around the context content, not generic cloud guidance

Your response MUST be extremely comprehensive, detailed, and implementation-focused. Provide:

1. A thorough, actionable answer with specific implementation steps and detailed explanations
2. Complete architecture diagrams using Mermaid.js graph TD syntax with comprehensive labeling
3. MANDATORY: Extensive code snippets and complete configuration files in proper code blocks
4. Specific commands, scripts, and step-by-step procedures with detailed explanations
5. In-depth analysis of options, trade-offs, and best practices
6. Comprehensive cost breakdowns and optimization strategies
7. Detailed timelines and project phases
8. Multiple implementation approaches with pros/cons
9. Always cite your sources using [source_id] format when referencing any information

RESPONSE LENGTH REQUIREMENTS:
- Minimum 2000 words for complex enterprise queries
- Provide exhaustive detail on all aspects requested
- Include comprehensive explanations, not just bullet points
- Expand on each major section with detailed sub-sections
- Provide multiple examples and use cases where applicable

CRITICAL CONTEXTUAL SPECIFICITY REQUIREMENTS:
- EXTRACT and USE specific numbers, quantities, and parameters from the user query
- REFERENCE exact specifications provided (VM counts, RTO/RPO times, storage sizes, etc.)
- TAILOR ALL recommendations to the specific technologies mentioned
- CALCULATE precise costs based on exact specifications provided
- CUSTOMIZE architecture diagrams to reflect specific requirements
- AVOID generic cloud migration language - make it specific to the user's exact scenario

MANDATORY COST CALCULATIONS FOR MIGRATION QUERIES:
- MUST provide specific cost estimates for the exact number of VMs mentioned
- MUST include monthly AWS infrastructure costs with specific instance types
- MUST calculate migration costs including AWS MGN usage
- MUST provide cost comparison between on-premises and AWS
- MUST include specific storage costs for databases and file systems
- MUST estimate data transfer costs for the migration
- MUST provide 3-year total cost of ownership (TCO) analysis
- MUST include cost optimization recommendations with dollar savings
- MUST use current AWS pricing (2024) for all calculations

COST CALCULATION EXAMPLE FORMAT:
### Cost Analysis for 120 VM Migration

**Monthly AWS Infrastructure Costs:**
- 120 x t3.medium instances: $3,168/month (120 x $26.40)
- RDS SQL Server (multi-AZ): $520/month
- EBS Storage (1TB per VM): $12,000/month
- Data Transfer: $450/month
- **Total Monthly Cost: $16,138**

**Migration Costs:**
- AWS MGN replication: $2,400 (120 VMs x $20 per VM)
- Professional services: $150,000
- **Total Migration Cost: $152,400**

**3-Year TCO Comparison:**
- On-premises (3 years): $2,160,000
- AWS (3 years): $1,631,328
- **Net Savings: $528,672 (24.5% reduction)**

CRITICAL REQUIREMENTS - Your response MUST include:
- Specific service configurations with exact parameters based on user requirements
- Complete code examples (not snippets) with full implementations using specific parameters
- Detailed step-by-step procedures with commands tailored to specific requirements
- Specific resource sizing and capacity planning based on exact user specifications
- Network configurations with IP ranges, subnets, and routing for specific scale
- Security configurations with exact policy definitions for specific technologies
- Monitoring and alerting configurations specific to mentioned technologies
- Troubleshooting procedures and common issues for specific scenarios
- Cost breakdowns with specific pricing estimates for exact specifications
- Implementation timelines with detailed task dependencies for specific scale
- Performance optimization parameters and tuning configurations
- Backup and disaster recovery procedures with specific recovery steps
- Validation and testing scripts with comprehensive test cases
- Operational runbooks with detailed maintenance procedures
- Capacity planning with growth projections and scaling triggers
- Security hardening checklists with specific configuration changes
- Compliance implementation steps with audit procedures
- Integration patterns with detailed API configurations
- Automation scripts for deployment and operational tasks

MANDATORY CODE GENERATION - Your response MUST include:
- Complete Terraform/ARM templates in proper ` + "`terraform`" + ` code blocks
- Full bash scripts with error handling in proper ` + "`bash`" + ` code blocks
- Exact CLI commands with all parameters in proper ` + "`bash`" + ` code blocks
- Complete configuration files (not partial examples) in proper ` + "`yaml`" + ` or ` + "`json`" + ` code blocks
- Specific instance types, storage configurations, and networking details in code
- Actual implementation workflows with dependencies in code

CRITICAL: DO NOT say "Below is a complete Terraform configuration" without actually providing the code block. 
CRITICAL: DO NOT say "Below is a sample bash script" without actually providing the code block.
CRITICAL: ALWAYS provide the actual code immediately after describing it.
CRITICAL: For migration queries, MUST provide actual working Terraform code for infrastructure setup.

EXAMPLE CORRECT FORMAT:
### Terraform Code for Landing Zone Setup

` + "```terraform" + `
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

resource "aws_vpc" "main" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true
  tags = {
    Name = "enterprise-migration-vpc"
  }
}
` + "```" + `
- Detailed validation and testing procedures in code

CODE BLOCK FORMATTING - ALWAYS use proper markdown code blocks:
- Terraform: ` + "```terraform" + ` ... ` + "```" + `
- Bash/Shell: ` + "```bash" + ` ... ` + "```" + `
- AWS CLI: ` + "```bash" + ` ... ` + "```" + `
- Azure CLI: ` + "```bash" + ` ... ` + "```" + `
- PowerShell: ` + "```powershell" + ` ... ` + "```" + `
- YAML: ` + "```yaml" + ` ... ` + "```" + `
- JSON: ` + "```json" + ` ... ` + "```" + `

FAILURE TO PROVIDE CODE BLOCKS IS UNACCEPTABLE - Every technical query requires implementation-ready code.

Guidelines:
- Be extremely specific and technical in your recommendations
- ALWAYS include implementation-ready code that can be executed immediately
- For diagrams: Use Mermaid.js graph TD syntax with detailed component specifications
- For code: Provide complete, production-ready configurations in proper code blocks
- Citations: End sentences with [source_id] or [URL] when using information from any source
- Focus on immediate implementation guidance with working examples

`

	// Add comprehensive diagram generation instructions
	diagramInstructions := buildDiagramInstructions(queryType)
	basePrompt += diagramInstructions

	// Add comprehensive code generation instructions
	codeInstructions := buildCodeGenerationInstructions(queryType)
	basePrompt += codeInstructions

	switch queryType {
	case TechnicalQuery:
		technicalFocus := `TECHNICAL FOCUS: Provide deep technical implementation details, complete code examples, configuration examples, architectural patterns, and comprehensive best practices. Include specific configurations, performance tuning, and operational procedures.

ENHANCED TECHNICAL DEPTH REQUIREMENTS:
- Provide extensive code comments explaining each configuration parameter
- Include multiple implementation approaches with trade-offs analysis
- Add comprehensive error handling and edge case management
- Include detailed performance benchmarking and optimization strategies
- Provide extensive logging and monitoring configurations
- Include comprehensive security scanning and vulnerability assessments
- Add detailed capacity planning with resource utilization metrics
- Include comprehensive backup and recovery validation procedures
- Provide detailed integration testing and validation scripts
- Include extensive troubleshooting guides with common failure scenarios

`
		return basePrompt + technicalFocus
	case BusinessQuery:
		businessFocus := `BUSINESS FOCUS: Provide detailed business value analysis, comprehensive cost breakdowns, cost considerations, ROI analysis, detailed timeline estimates, and strategic implications with specific metrics.

`
		return basePrompt + businessFocus
	case GeneralQuery:
		return basePrompt
	default:
		return basePrompt
	}
}

// buildContextualParameterInstructions creates instructions that emphasize using specific query parameters
func buildContextualParameterInstructions(query string) string {
	queryLower := strings.ToLower(query)
	var instructions strings.Builder

	// Check for specific parameters that need emphasis
	var foundParameters []string

	// Check for VM counts
	if vmMatches := regexp.MustCompile(`(\d+)\s+(?:vm|vms|virtual machines|servers?)`).FindAllStringSubmatch(queryLower, -1); vmMatches != nil {
		for _, match := range vmMatches {
			if len(match) > 1 {
				foundParameters = append(foundParameters, match[1]+" VMs")
			}
		}
	}

	// Check for RTO/RPO requirements
	if rtoMatches := regexp.MustCompile(`(?:rto|recovery time objective)[^0-9]*(\d+)\s*(?:hours?|minutes?|mins?|hrs?)`).FindAllStringSubmatch(queryLower, -1); rtoMatches != nil {
		for _, match := range rtoMatches {
			if len(match) > 1 {
				foundParameters = append(foundParameters, "RTO: "+match[1])
			}
		}
	}

	if rpoMatches := regexp.MustCompile(`(?:rpo|recovery point objective)[^0-9]*(\d+)\s*(?:hours?|minutes?|mins?|hrs?)`).FindAllStringSubmatch(queryLower, -1); rpoMatches != nil {
		for _, match := range rpoMatches {
			if len(match) > 1 {
				foundParameters = append(foundParameters, "RPO: "+match[1])
			}
		}
	}

	// Check for technologies
	techKeywords := []string{"windows", "linux", "sql server", ".net", "java", "vmware", "docker", "kubernetes"}
	for _, tech := range techKeywords {
		if strings.Contains(queryLower, tech) {
			foundParameters = append(foundParameters, strings.Title(tech))
		}
	}

	// Check for cloud providers
	cloudProviders := []string{"aws", "azure", "gcp", "google cloud"}
	for _, provider := range cloudProviders {
		if strings.Contains(queryLower, provider) {
			foundParameters = append(foundParameters, strings.ToUpper(provider))
		}
	}

	// Check for other specific numbers
	if numberMatches := regexp.MustCompile(`(\d+)\s*(?:tb|gb|mb|cpu|cores|vcpus|users|endpoints|connections)`).FindAllStringSubmatch(queryLower, -1); numberMatches != nil {
		for _, match := range numberMatches {
			if len(match) > 0 {
				foundParameters = append(foundParameters, match[0])
			}
		}
	}

	// If specific parameters are found, add targeted instructions
	if len(foundParameters) > 0 {
		instructions.WriteString("### CRITICAL CONTEXTUAL REQUIREMENTS ###\n")
		instructions.WriteString("The user query contains SPECIFIC PARAMETERS that MUST be directly addressed in your response:\n")

		for _, param := range foundParameters {
			instructions.WriteString(fmt.Sprintf("- %s\n", param))
		}

		instructions.WriteString("\n**MANDATORY RESPONSE REQUIREMENTS:**\n")
		instructions.WriteString("1. Reference these EXACT numbers and specifications in your response\n")
		instructions.WriteString("2. Base ALL calculations, sizing, and recommendations on these specific parameters\n")
		instructions.WriteString("3. Provide tailored solutions that directly address these requirements\n")
		instructions.WriteString("4. Include specific cost estimates based on these exact specifications\n")
		instructions.WriteString("5. Generate architecture diagrams that reflect these specific requirements\n")
		instructions.WriteString("6. Provide implementation code that uses these exact parameters\n")
		instructions.WriteString("\n**AVOID GENERIC RESPONSES** - Every recommendation must be specifically tailored to these parameters.\n\n")
	}

	return instructions.String()
}

// buildDiagramInstructions creates comprehensive Mermaid.js diagram generation instructions
func buildDiagramInstructions(_ QueryType) string {
	return buildDiagramHeader() +
		buildMermaidSyntaxInstructions() +
		buildCloudArchitecturePatterns() +
		buildDiagramQualityGuidelines() +
		buildDiagramFallbackInstructions()
}

// buildDiagramHeader creates the main header for diagram instructions
func buildDiagramHeader() string {
	return `
## MERMAID.JS DIAGRAM GENERATION INSTRUCTIONS

### When to Generate Diagrams
Generate architecture diagrams for queries involving:
- Cloud architecture design (AWS, Azure, GCP, hybrid)
- Migration planning and lift-and-shift scenarios
- Disaster recovery and backup strategies
- Network topology and security configurations
- Microservices and containerization architectures
- CI/CD pipeline designs
- Data flow and integration patterns

`
}

// buildMermaidSyntaxInstructions creates the syntax requirements section
func buildMermaidSyntaxInstructions() string {
	return `### Mermaid Syntax Requirements
- ALWAYS use "graph TD" (Top-Down) syntax for cloud architecture diagrams
- Enclose ALL diagram code in triple backticks with "mermaid" language identifier: ` + "```mermaid" + `
- Use descriptive node names with proper formatting
- Include subgraphs for logical groupings (environments, regions, services)
- Use appropriate arrow styles for different connection types

`
}

// EstimateTokens provides a rough estimate of token count (4 characters ≈ 1 token)
func EstimateTokens(text string) int {
	return utf8.RuneCountInString(text) / TokenEstimateRatio
}

// TruncateToTokenLimit truncates text to fit within token limit
func TruncateToTokenLimit(text string, maxTokens int) string {
	estimatedTokens := EstimateTokens(text)
	if estimatedTokens <= maxTokens {
		return text
	}

	// Calculate target character count (rough approximation)
	// Use safety ratio of the target to account for truncation notice
	targetChars := int(float64(maxTokens) * TokenEstimateRatio * TruncationSafetyRatio)
	runes := []rune(text)

	if len(runes) > targetChars {
		return string(runes[:targetChars]) + "...\n\n[Context truncated due to length limits]"
	}

	return text
}

// ValidatePrompt validates the completeness and structure of a prompt
func ValidatePrompt(prompt string) error {
	if err := validateBasicPromptStructure(prompt); err != nil {
		return err
	}

	if err := validatePromptContent(prompt); err != nil {
		return err
	}

	if err := validateDiagramInstructions(prompt); err != nil {
		return err
	}

	return validateCodeInstructions(prompt)
}

// validateBasicPromptStructure checks basic prompt requirements
func validateBasicPromptStructure(prompt string) error {
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("prompt cannot be empty")
	}

	if len(prompt) < MinimalPromptLength {
		return fmt.Errorf("prompt appears to be too short (< %d characters)", MinimalPromptLength)
	}

	return nil
}

// validatePromptContent checks for required content sections
func validatePromptContent(prompt string) error {
	requiredSections := map[string]string{
		"User Query:":         "user query section",
		"Solutions Architect": "Solutions Architect persona",
		"[source_id]":         "citation instructions",
	}

	for section, description := range requiredSections {
		if !strings.Contains(prompt, section) {
			return fmt.Errorf("prompt must contain %s", description)
		}
	}

	return nil
}

// validateDiagramInstructions checks for Mermaid diagram instruction requirements
func validateDiagramInstructions(prompt string) error {
	diagramRequirements := map[string]string{
		"MERMAID.JS DIAGRAM GENERATION INSTRUCTIONS": "Mermaid.js diagram generation instructions",
		"graph TD":   "graph TD syntax instructions",
		"```mermaid": "mermaid code block formatting instructions",
	}

	for requirement, description := range diagramRequirements {
		if !strings.Contains(prompt, requirement) {
			return fmt.Errorf("prompt must contain %s", description)
		}
	}

	return nil
}

// validateCodeInstructions checks for code generation instruction requirements
func validateCodeInstructions(prompt string) error {
	codeRequirements := map[string]string{
		"CODE GENERATION INSTRUCTIONS":    "code generation instructions",
		"terraform":                       "Terraform code generation instructions",
		"AWS CLI":                         "AWS CLI code generation instructions",
		"Azure CLI":                       "Azure CLI code generation instructions",
		"PowerShell":                      "PowerShell code generation instructions",
		"NEVER include hardcoded secrets": "security requirements for code generation", // pragma: allowlist secret
		"meaningful comments":             "code commenting requirements",
	}

	for requirement, description := range codeRequirements {
		if !strings.Contains(prompt, requirement) {
			return fmt.Errorf("prompt must contain %s", description)
		}
	}

	return nil
}

// DetectArchitectureQuery determines if a query is about architecture and warrants a diagram
func DetectArchitectureQuery(query string) bool {
	queryLower := strings.ToLower(query)

	// First check if it's a business-only query that shouldn't trigger architecture detection
	businessOnlyKeywords := []string{
		"cost", "pricing", "budget", "savings", "roi", "return on investment",
		"business case", "financial", "billing", "invoice", "payment",
		"timeline", "schedule", "project plan", "roadmap", "strategy",
		"compliance", "governance", "policy", "regulation", "audit",
		"risk", "security assessment", "vulnerability", "threat",
		"sla", "service level", "kpi", "metrics", "performance",
		"training", "certification", "documentation", "process",
		"team", "resources", "staffing", "skills", "expertise",
	}

	// Check if it's primarily business-focused
	businessScore := 0
	for _, keyword := range businessOnlyKeywords {
		if strings.Contains(queryLower, keyword) {
			businessScore++
		}
	}

	// If it has multiple business keywords, it's likely business-only
	if businessScore > 1 {
		return false
	}

	// If it has business keywords and no strong architecture indicators, it's business-only
	if businessScore > 0 && !hasStrongArchitectureIndicators(queryLower) {
		return false
	}

	architectureKeywords := []string{
		// Core architecture terms
		"architecture", "design", "topology", "infrastructure", "deployment",
		"migration", "lift-and-shift", "lift and shift", "disaster recovery",
		"backup", "replication", "failover", "high availability", "scalability",

		// Cloud platforms
		"aws", "azure", "gcp", "google cloud", "cloud architecture", "cloud migration", "cloud", "hybrid", "multi-cloud",
		"on-premises", "on-prem", "datacenter", "data center",

		// Networking
		"network", "vpc", "subnet", "security group", "firewall", "load balancer",
		"vpn", "expressroute", "direct connect", "peering", "gateway",

		// Services and components
		"microservices", "containers", "kubernetes", "docker", "serverless",
		"lambda", "functions", "api", "database", "storage", "cdn",

		// Integration patterns
		"integration", "data flow", "pipeline", "workflow", "orchestration",
		"messaging", "queue", "event", "streaming", "batch processing",

		// Specific implementations
		"terraform", "cloudformation", "ansible", "ci/cd", "devops",
		"monitoring", "logging", "observability", "security", "compliance",
	}

	for _, keyword := range architectureKeywords {
		if strings.Contains(queryLower, keyword) {
			return true
		}
	}

	return false
}

// hasStrongArchitectureIndicators checks for strong architecture-related terms
func hasStrongArchitectureIndicators(queryLower string) bool {
	strongIndicators := []string{
		"architecture", "design", "topology", "infrastructure", "deployment",
		"migration", "lift-and-shift", "disaster recovery", "backup",
		"replication", "failover", "high availability", "scalability",
		"network", "vpc", "subnet", "security group", "firewall",
		"load balancer", "microservices", "containers", "kubernetes",
		"docker", "serverless", "lambda", "terraform", "cloudformation",
	}

	for _, indicator := range strongIndicators {
		if strings.Contains(queryLower, indicator) {
			return true
		}
	}

	return false
}

// IsBusinessOnlyQuery determines if a query is purely business-focused without technical architecture
func IsBusinessOnlyQuery(query string) bool {
	queryLower := strings.ToLower(query)

	businessOnlyKeywords := []string{
		"cost", "pricing", "budget", "savings", "roi", "return on investment",
		"business case", "financial", "billing", "invoice", "payment",
		"timeline", "schedule", "project plan", "roadmap", "strategy",
		"compliance", "governance", "policy", "regulation", "audit",
		"risk", "security assessment", "vulnerability", "threat",
		"sla", "service level", "kpi", "metrics", "performance",
		"training", "certification", "documentation", "process",
		"team", "resources", "staffing", "skills", "expertise",
	}

	// Check if it's primarily business-focused
	businessScore := 0
	for _, keyword := range businessOnlyKeywords {
		if strings.Contains(queryLower, keyword) {
			businessScore++
		}
	}

	// If it has business keywords but no architecture keywords, it's business-only
	return businessScore > 0 && !DetectArchitectureQuery(query)
}

// buildCodeGenerationInstructions creates comprehensive code generation instructions
func buildCodeGenerationInstructions(_ QueryType) string {
	return buildCodeGenerationHeader() +
		buildTerraformInstructions() +
		buildAWSCLIInstructions() +
		buildAzureCLIInstructions() +
		buildPowerShellInstructions() +
		buildYAMLJSONInstructions() +
		buildCodeSecurityRequirements() +
		buildCodeTestingInstructions()
}

// buildCodeGenerationHeader creates the header section for code generation instructions
func buildCodeGenerationHeader() string {
	return `
## CODE GENERATION INSTRUCTIONS

### When to Generate Code
Generate code snippets for queries involving:
- Infrastructure deployment and configuration
- Cloud resource provisioning and management
- Migration and deployment automation
- Configuration management and orchestration
- Security implementations and compliance
- Monitoring, logging, and observability setup
- CI/CD pipeline configurations
- Backup, disaster recovery, and automation scripts

### Language and Tool Requirements

`
}

// buildTerraformInstructions creates Terraform-specific code generation instructions
func buildTerraformInstructions() string {
	return `#### Terraform (Infrastructure as Code) - MANDATORY FOR ALL INFRASTRUCTURE QUERIES
- MUST generate complete Terraform configurations for all infrastructure requests
- ALWAYS include provider configuration (AWS, Azure, GCP)
- MUST use meaningful resource names with proper naming conventions
- MUST include data sources for existing resources
- MUST add variable definitions and output values
- MANDATORY format: ` + "`terraform`" + ` code blocks

TERRAFORM CODE GENERATION REQUIREMENTS:
- Generate COMPLETE Terraform configurations, not partial examples
- Include ALL required resources for the requested infrastructure
- Add proper variable definitions and outputs
- Include tags and naming conventions
- Add security configurations (security groups, NACLs, etc.)

CRITICAL INSTRUCTION: When you write "### Terraform Code for Landing Zone Setup" or similar, 
you MUST immediately follow it with an actual terraform code block. DO NOT leave it empty.
Example:
### Terraform Code for Landing Zone Setup

` + "```terraform" + `
# Your actual Terraform code here
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}
# ... rest of actual code
` + "```" + `
- Include networking configurations (VPCs, subnets, route tables)
- Add monitoring and logging configurations

MANDATORY Example Pattern for AWS VPC:
` + "```terraform" + `
# Configure the AWS Provider
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

# Configure the AWS Provider
provider "aws" {
  region = var.aws_region
}

# Variables
variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "us-west-2"
}

variable "vpc_cidr" {
  description = "CIDR block for VPC"
  type        = string
  default     = "10.0.0.0/16"
}

variable "project_name" {
  description = "Project name for resource naming"
  type        = string
  default     = "my-project"
}

variable "environment" {
  description = "Environment name"
  type        = string
  default     = "production"
}

# Create VPC
resource "aws_vpc" "main" {
  cidr_block           = var.vpc_cidr
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name        = "$${var.project_name}-vpc"
    Environment = var.environment
  }
}

# Create Internet Gateway
resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id

  tags = {
    Name        = "$${var.project_name}-igw"
    Environment = var.environment
  }
}

# Create public subnet
resource "aws_subnet" "public" {
  vpc_id                  = aws_vpc.main.id
  cidr_block              = "10.0.1.0/24"
  availability_zone       = "$${var.aws_region}a"
  map_public_ip_on_launch = true

  tags = {
    Name        = "$${var.project_name}-public-subnet"
    Environment = var.environment
  }
}

# Create private subnet
resource "aws_subnet" "private" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = "10.0.2.0/24"
  availability_zone = "$${var.aws_region}a"

  tags = {
    Name        = "$${var.project_name}-private-subnet"
    Environment = var.environment
  }
}

# Create route table for public subnet
resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.main.id
  }

  tags = {
    Name        = "$${var.project_name}-public-rt"
    Environment = var.environment
  }
}

# Associate route table with public subnet
resource "aws_route_table_association" "public" {
  subnet_id      = aws_subnet.public.id
  route_table_id = aws_route_table.public.id
}

# Outputs
output "vpc_id" {
  description = "VPC ID"
  value       = aws_vpc.main.id
}

output "public_subnet_id" {
  description = "Public subnet ID"
  value       = aws_subnet.public.id
}

output "private_subnet_id" {
  description = "Private subnet ID"
  value       = aws_subnet.private.id
}
` + "```" + `

**CRITICAL: Every infrastructure query MUST include complete, executable Terraform code like the above example.**

`
}

// buildAWSCLIInstructions creates AWS CLI-specific code generation instructions
func buildAWSCLIInstructions() string {
	return `#### AWS CLI Commands
- Use for AWS resource management and automation
- Include profile and region specifications where applicable
- Use meaningful output formats (json, table, text)
- Include error handling and validation
- Format: ` + "`bash`" + ` or ` + "`aws`" + `

Example Pattern:
` + "```bash" + `
#!/bin/bash
# AWS CLI script for EC2 instance management

# Set default region and profile
export AWS_DEFAULT_REGION=us-west-2
export AWS_PROFILE=production

# Create EC2 instance
aws ec2 run-instances \
  --image-id ami-0abcdef1234567890 \
  --instance-type t3.medium \
  --key-name my-key-pair \
  --security-group-ids sg-0123456789abcdef0 \
  --subnet-id subnet-0123456789abcdef0 \
  --user-data file://user-data.sh \
  --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=MyInstance}]' \
  --output json

# Wait for instance to be running
aws ec2 wait instance-running --instance-ids i-0123456789abcdef0

# Get instance information
aws ec2 describe-instances \
  --instance-ids i-0123456789abcdef0 \
  --query 'Reservations[0].Instances[0].{ID:InstanceId,State:State.Name}' \
  --output table
` + "```" + `

`
}

// buildAzureCLIInstructions creates Azure CLI-specific code generation instructions
func buildAzureCLIInstructions() string {
	return `#### Azure CLI Commands
- Use for Azure resource management and automation
- Include subscription and resource group specifications
- Use meaningful output formats (json, table, yaml)
- Include resource tagging and naming conventions
- Format: ` + "`bash`" + ` or ` + "`azure`" + `

Example Pattern:
` + "```bash" + `
#!/bin/bash
# Azure CLI script for VM deployment

# Set default subscription and resource group
az account set --subscription "Production"
RESOURCE_GROUP="rg-production-eastus"
LOCATION="eastus"

# Create resource group if it doesn't exist
az group create --name $RESOURCE_GROUP --location $LOCATION

# Create virtual network
az network vnet create \
  --resource-group $RESOURCE_GROUP \
  --name vnet-production \
  --address-prefix 10.0.0.0/16 \
  --subnet-name subnet-web \
  --subnet-prefix 10.0.1.0/24

# Create virtual machine
az vm create \
  --resource-group $RESOURCE_GROUP \
  --name vm-web-01 \
  --image Ubuntu2204 \
  --admin-username azureuser \
  --generate-ssh-keys \
  --vnet-name vnet-production \
  --subnet subnet-web \
  --size Standard_B2s \
  --tags Environment=Production Team=WebDev
` + "```" + `

`
}

// buildPowerShellInstructions creates PowerShell-specific code generation instructions
func buildPowerShellInstructions() string {
	return `#### PowerShell (Azure/Windows Automation)
- Use for Windows-centric Azure automation
- Include error handling and progress indicators
- Use meaningful variable names and parameter validation
- Include logging and status reporting
- Format: ` + "`powershell`" + `

Example Pattern:
` + "```powershell" + `
# Azure PowerShell script for resource deployment
param(
    [Parameter(Mandatory=$true)]
    [string]$ResourceGroupName,

    [Parameter(Mandatory=$true)]
    [string]$Location,

    [Parameter(Mandatory=$false)]
    [string]$Environment = "Production"
)

# Connect to Azure (if not already connected)
if (-not (Get-AzContext)) {
    Write-Host "Connecting to Azure..." -ForegroundColor Yellow
    Connect-AzAccount
}

try {
    Write-Host "Starting deployment to $ResourceGroupName" -ForegroundColor Green

    # Create resource group
    $resourceGroup = New-AzResourceGroup -Name $ResourceGroupName -Location $Location -Force
    Write-Host "Resource group created: $($resourceGroup.ResourceGroupName)" -ForegroundColor Green

    # Deploy ARM template
    $deploymentResult = New-AzResourceGroupDeployment \
        -ResourceGroupName $ResourceGroupName \
        -TemplateFile "azuredeploy.json" \
        -Environment $Environment \
        -Verbose

    Write-Host "Deployment completed successfully" -ForegroundColor Green
}
catch {
    Write-Error "Deployment failed: $($_.Exception.Message)"
    Write-Host "Rolling back changes..." -ForegroundColor Yellow
    exit 1
}
finally {
    Write-Host "Cleanup completed" -ForegroundColor Blue
}
` + "```" + `

`
}

// buildYAMLJSONInstructions creates YAML/JSON configuration file instructions
func buildYAMLJSONInstructions() string {
	return `#### YAML/JSON Configuration Files
- Use for Kubernetes manifests, CI/CD pipelines, and configuration management
- Follow proper indentation and structure guidelines
- Include metadata and labels for proper organization
- Add validation and schema references where applicable
- Format: ` + "`yaml`" + ` or ` + "`json`" + `

Example YAML Pattern:
` + "```yaml" + `
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  namespace: production
  labels:
    app: web-service
    environment: production
data:
  database_url: "postgresql://db.example.com:5432/prod"
  cache_enabled: "true"
  log_level: "info"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-service
  namespace: production
spec:
  replicas: 3
  selector:
    matchLabels:
      app: web-service
  template:
    metadata:
      labels:
        app: web-service
    spec:
      containers:
      - name: web
        image: nginx:1.21
        ports:
        - containerPort: 80
` + "```" + `

`
}

// buildCodeTestingInstructions creates testing and integration instructions
func buildCodeTestingInstructions() string {
	return `### Integration and Testing Considerations
- Include unit tests for complex scripts
- Add integration testing steps
- Include deployment validation checks
- Add monitoring and alerting configurations
- Include rollback and recovery procedures

`
}

// buildCloudArchitecturePatterns creates cloud architecture pattern examples
func buildCloudArchitecturePatterns() string {
	return `

### Cloud Architecture Diagram Conventions

#### AWS Architecture Diagrams
- Use subgraphs for VPCs, Availability Zones, and service groupings
- Node naming: Use AWS service names (EC2, RDS, S3, Lambda, etc.)
- Include security groups, subnets, and load balancers
- Show data flow with labeled arrows

Example AWS Pattern:
` + "```" + `
graph TD
    subgraph "AWS Cloud"
        subgraph "VPC: 10.0.0.0/16"
            subgraph "Public Subnet"
                ALB[Application Load Balancer]
                NAT[NAT Gateway]
            end
            subgraph "Private Subnet"
                EC2[EC2 Instances]
                RDS[RDS Database]
            end
        end
        S3[S3 Buckets]
    end
    Users[Users] --> ALB
    ALB --> EC2
    EC2 --> RDS
    EC2 --> S3
` + "```" + `

#### Azure Architecture Diagrams
- Use subgraphs for Resource Groups, Virtual Networks, and subscriptions
- Node naming: Use Azure service names (VM, SQL Database, Storage Account, etc.)
- Include Azure-specific components (Application Gateway, Traffic Manager)
- Show resource relationships and dependencies

Example Azure Pattern:
` + "```" + `
graph TD
    subgraph "Azure Subscription"
        subgraph "Resource Group"
            subgraph "Virtual Network: 10.1.0.0/16"
                subgraph "Public Subnet"
                    AG[Application Gateway]
                    LB[Load Balancer]
                end
                subgraph "Private Subnet"
                    VM[Virtual Machines]
                    SQL[SQL Database]
                end
            end
            Storage[Storage Account]
        end
    end
    Users[Users] --> AG
    AG --> VM
    VM --> SQL
    VM --> Storage
` + "```" + `

#### Hybrid Cloud Architecture Diagrams
- Show connections between on-premises and cloud environments
- Include VPN or ExpressRoute connections
- Separate subgraphs for different environments
- Show data synchronization and backup flows

Example Hybrid Pattern:
` + "```" + `
graph TD
    subgraph "On-Premises"
        OnPremServers[Legacy Servers]
        OnPremDB[On-Prem Database]
        VPNGateway[VPN Gateway]
    end

    subgraph "AWS Cloud"
        subgraph "VPC"
            CloudServers[EC2 Instances]
            CloudDB[RDS Database]
            CloudVPN[VPN Connection]
        end
    end

    OnPremServers --> VPNGateway
    VPNGateway -.-> CloudVPN
    CloudVPN --> CloudServers
    OnPremDB -.-> CloudDB
    CloudServers --> CloudDB
` + "```" + `

`
}

// buildDiagramQualityGuidelines creates quality and formatting guidelines
func buildDiagramQualityGuidelines() string {
	return `### Diagram Quality Requirements
- Include 5-15 nodes for optimal clarity
- Use meaningful node labels (not generic terms)
- Group related components in subgraphs
- Show clear data flow direction with arrows
- Include security boundaries and access controls
- Use consistent naming conventions throughout

### Node Formatting Guidelines
- Use PascalCase for service names: EC2, RDS, S3
- Use descriptive labels: "Web Servers" instead of "Servers"
- Include capacity or scale indicators when relevant
- Use square brackets for services: [EC2 Instances]
- Use parentheses for external entities: (Users)
- Use curly braces for databases: {RDS Database}

### Arrow Types and Meanings
- Solid arrows (-->) for primary data flow
- Dashed arrows (-.->)  for secondary or backup connections
- Thick arrows (==>) for high-bandwidth connections
- Dotted arrows (...>) for occasional or batch data transfer

`
}

// buildDiagramFallbackInstructions creates fallback and format instructions
func buildDiagramFallbackInstructions() string {
	return `### Fallback Instructions
If the query is NOT about architecture, infrastructure, or technical implementation:
- Do NOT generate a diagram
- Focus on textual response with bullet points and structured information
- Only include diagrams if they genuinely add value to the architectural understanding

### Common Diagram Mistakes to Avoid
- Do NOT use "graph LR" (Left-Right) - always use "graph TD" (Top-Down)
- Do NOT create overcomplicated diagrams with too many nodes
- Do NOT use generic node names like "Server1", "Database1"
- Do NOT forget to enclose diagram code in proper markdown code blocks
- Do NOT include diagrams for non-architectural queries

### Code Block Format
Always format Mermaid diagrams exactly like this:

` + "```mermaid" + `
graph TD
    [Your diagram content here]
` + "```" + `

`
}

// validateCodeSecurity validates code snippets for security issues
func validateCodeSecurity(code, language string) bool {
	// Check for potential security issues
	if containsMaliciousPatterns(code) {
		return false
	}

	// Check for hardcoded secrets
	if containsHardcodedSecrets(code) {
		return false
	}

	// Language-specific security checks
	switch strings.ToLower(language) {
	case "bash", "sh", "shell":
		if containsUnsafeBashPatterns(code) {
			return false
		}
	case "powershell", "ps1":
		if containsUnsafePowerShellPatterns(code) {
			return false
		}
	case "terraform", "tf":
		if containsUnsafeTerraformPatterns(code) {
			return false
		}
	}

	return true
}

// containsMaliciousPatterns checks for general malicious patterns
func containsMaliciousPatterns(code string) bool {
	maliciousPatterns := []string{
		"rm -rf /",
		"format c:",
		"del /f /s /q",
		"shutdown -h now",
		"killall -9",
		":(){ :|:& };:", // Fork bomb
		"/dev/null 2>&1",
		"| bash",
		"| sh",
		"eval(",
		"exec(",
		"system(",
		"shell_exec(",
	}

	codeLines := strings.Split(strings.ToLower(code), "\n")
	for _, line := range codeLines {
		line = strings.TrimSpace(line)
		for _, pattern := range maliciousPatterns {
			if strings.Contains(line, pattern) {
				return true
			}
		}
	}
	return false
}

// containsHardcodedSecrets checks for hardcoded secrets
func containsHardcodedSecrets(code string) bool {
	secretPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)password\s*[=:]\s*["'][^"'\s]{8,}["']`),
		regexp.MustCompile(`(?i)api[_-]?key\s*[=:]\s*["'][^"'\s]{20,}["']`),
		regexp.MustCompile(`(?i)secret\s*[=:]\s*["'][^"'\s]{16,}["']`),
		regexp.MustCompile(`(?i)token\s*[=:]\s*["'][^"'\s]{20,}["']`),
		regexp.MustCompile(`(?i)aws_access_key_id\s*[=:]\s*["'][A-Z0-9]{20}["']`),
		regexp.MustCompile(`(?i)aws_secret_access_key\s*[=:]\s*["'][A-Za-z0-9/+=]{40}["']`),
		regexp.MustCompile(`sk-[a-zA-Z0-9]{48,}`), // OpenAI API key pattern
		regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`), // GitHub personal access token
	}

	// Check for environment variable patterns (these are safe)
	safePatterns := []*regexp.Regexp{
		regexp.MustCompile(`\$\{[^}]+\}`), // ${VAR} pattern
		regexp.MustCompile(`\$[A-Z_]+`),   // $VAR pattern
	}

	// First check if the code contains safe variable patterns
	for _, pattern := range safePatterns {
		if pattern.MatchString(code) {
			// If it contains variables, do more careful checking
			lines := strings.Split(code, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				// Skip lines that are clearly using variables
				if strings.Contains(line, "${") || strings.Contains(line, "$") {
					continue
				}
				// Check this line against secret patterns // pragma: allowlist secret
				for _, secretPattern := range secretPatterns { // pragma: allowlist secret
					if secretPattern.MatchString(line) {
						return true
					}
				}
			}
			return false
		}
	}

	// If no variables found, check normally
	for _, pattern := range secretPatterns {
		if pattern.MatchString(code) {
			return true
		}
	}
	return false
}

// containsUnsafeBashPatterns checks for unsafe bash patterns
func containsUnsafeBashPatterns(code string) bool {
	unsafePatterns := []string{
		"rm -rf $",
		"chmod 777",
		"chown root",
		"sudo rm",
		"dd if=/dev/zero",
		"mkfs.",
		"fdisk",
		"> /dev/",
		"eval $",
		"$($(",
	}

	codeLines := strings.Split(strings.ToLower(code), "\n")
	for _, line := range codeLines {
		line = strings.TrimSpace(line)
		for _, pattern := range unsafePatterns {
			if strings.Contains(line, pattern) {
				return true
			}
		}
	}
	return false
}

// containsUnsafePowerShellPatterns checks for unsafe PowerShell patterns
func containsUnsafePowerShellPatterns(code string) bool {
	unsafePatterns := []string{
		"remove-item -recurse -force",
		"format-volume",
		"clear-disk",
		"invoke-expression",
		"iex (",
		"downloadstring",
		"bypass",
		"unrestricted",
		"stop-computer",
		"restart-computer -force",
	}

	codeLines := strings.Split(strings.ToLower(code), "\n")
	for _, line := range codeLines {
		line = strings.TrimSpace(line)
		for _, pattern := range unsafePatterns {
			if strings.Contains(line, pattern) {
				return true
			}
		}
	}
	return false
}

// containsUnsafeTerraformPatterns checks for unsafe Terraform patterns
func containsUnsafeTerraformPatterns(code string) bool {
	// For demo/migration scenarios, we need to be more permissive
	// Only flag truly dangerous patterns, not common demo patterns
	unsafePatterns := []string{
		"rm -rf",
		"format c:",
		"del /f /s /q",
		"shutdown -h now",
		"killall -9",
		"| bash",
		"| sh",
		"eval(",
		"exec(",
		"system(",
		"shell_exec(",
		"destroy = true",
		// Note: "0.0.0.0/0" is valid in Terraform route tables and security groups
		// Only flag it in potentially dangerous contexts
	}

	codeLines := strings.Split(strings.ToLower(code), "\n")
	for _, line := range codeLines {
		line = strings.TrimSpace(line)
		for _, pattern := range unsafePatterns {
			if strings.Contains(line, pattern) {
				return true
			}
		}
	}

	// Check for actual malicious Terraform patterns (not demo patterns)
	// Only flag patterns that are clearly dangerous, not common demo configurations
	maliciousTerraformPatterns := []string{
		"data.external.shell",        // Executing shell commands
		"provisioner \"local-exec\"", // Local execution (context-dependent)
	}

	for _, line := range codeLines {
		line = strings.TrimSpace(line)
		for _, pattern := range maliciousTerraformPatterns {
			if strings.Contains(line, pattern) {
				return true
			}
		}
	}

	return false
}

// normalizeLanguage normalizes language identifiers to standard names
func normalizeLanguage(language string) string {
	langMap := map[string]string{
		"bash":          "bash",
		"sh":            "bash",
		"shell":         "bash",
		"zsh":           "bash",
		"terraform":     "terraform",
		"tf":            "terraform",
		"hcl":           "terraform",
		"hcl-terraform": "terraform",
		"terraform.tf":  "terraform",
		"powershell":    "powershell",
		"ps1":           "powershell",
		"pwsh":          "powershell",
		"yaml":          "yaml",
		"yml":           "yaml",
		"json":          "json",
		"javascript":    "javascript",
		"js":            "javascript",
		"python":        "python",
		"py":            "python",
		"go":            "go",
		"golang":        "go",
		"java":          "java",
		"c":             "c",
		"cpp":           "cpp",
		"c++":           "cpp",
		"csharp":        "csharp",
		"cs":            "csharp",
		"php":           "php",
		"ruby":          "ruby",
		"rb":            "ruby",
		"rust":          "rust",
		"rs":            "rust",
		"sql":           "sql",
		"dockerfile":    "dockerfile",
		"docker":        "dockerfile",
		"aws":           "bash", // AWS CLI is typically bash
		"azure":         "bash", // Azure CLI is typically bash
		"gcp":           "bash", // GCP CLI is typically bash
		"k8s":           "yaml", // Kubernetes manifests are YAML
		"kubernetes":    "yaml",
		"helm":          "yaml",
	}

	normalized := strings.ToLower(strings.TrimSpace(language))
	if mapped, exists := langMap[normalized]; exists {
		return mapped
	}
	return normalized
}

// formatWebResultWithURL formats a web result with URL validation and source attribution
func formatWebResultWithURL(index int, result string) string {
	url := extractURLFromWebResult(result)

	switch {
	case url != "" && isValidURL(url):
		return fmt.Sprintf("Web Result %d [%s]: %s\n\n", index, url, result)
	case url != "" && !isValidURL(url):
		return fmt.Sprintf("Web Result %d [Invalid URL]: %s\n\n", index, result)
	default:
		// When no URL is found, use simple format without brackets
		return fmt.Sprintf("Web Result %d: %s\n\n", index, result)
	}
}

// extractURLFromWebResult extracts the URL from a web result string
func extractURLFromWebResult(result string) string {
	// Web results are formatted as "Title: <title>\nSnippet: <snippet>\nURL: <url>"
	// or "Title: <title>\nURL: <url>" or "Snippet: <snippet>\nURL: <url>"
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "URL: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "URL: "))
		}
	}
	return ""
}

// ValidateSourceMetadata validates that all context items have valid source metadata
func ValidateSourceMetadata(contextItems []ContextItem) error {
	for i, item := range contextItems {
		if strings.TrimSpace(item.SourceID) == "" {
			return fmt.Errorf("context item %d is missing source ID", i)
		}
		if strings.TrimSpace(item.Content) == "" {
			return fmt.Errorf("context item %d is missing content", i)
		}
	}
	return nil
}

// DeduplicateSourcesByID removes duplicate context items based on SourceID
func DeduplicateSourcesByID(contextItems []ContextItem) []ContextItem {
	seen := make(map[string]bool)
	var deduplicated []ContextItem

	for _, item := range contextItems {
		if !seen[item.SourceID] {
			seen[item.SourceID] = true
			deduplicated = append(deduplicated, item)
		}
	}

	return deduplicated
}

// DeduplicateSourcesByContent removes duplicate context items with similar content
func DeduplicateSourcesByContent(contextItems []ContextItem, similarityThreshold float64) []ContextItem {
	if len(contextItems) <= 1 {
		return contextItems
	}

	var deduplicated []ContextItem
	deduplicated = append(deduplicated, contextItems[0]) // Always include first item

	for i := 1; i < len(contextItems); i++ {
		currentItem := contextItems[i]
		isSimilar := false

		// Check similarity with all previously added items
		for _, existingItem := range deduplicated {
			similarity := calculateContentSimilarity(currentItem.Content, existingItem.Content)
			if similarity >= similarityThreshold {
				isSimilar = true
				break
			}
		}

		if !isSimilar {
			deduplicated = append(deduplicated, currentItem)
		}
	}

	return deduplicated
}

// calculateContentSimilarity calculates a simple similarity score between two text strings
func calculateContentSimilarity(text1, text2 string) float64 {
	// Simple Jaccard similarity using word tokens
	words1 := strings.Fields(strings.ToLower(text1))
	words2 := strings.Fields(strings.ToLower(text2))

	if len(words1) == 0 && len(words2) == 0 {
		return 1.0
	}
	if len(words1) == 0 || len(words2) == 0 {
		return 0.0
	}

	// Create sets of words
	set1 := make(map[string]bool)
	set2 := make(map[string]bool)

	for _, word := range words1 {
		set1[word] = true
	}
	for _, word := range words2 {
		set2[word] = true
	}

	// Calculate intersection and union
	intersection := 0
	for word := range set1 {
		if set2[word] {
			intersection++
		}
	}

	union := len(set1) + len(set2) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// ValidateAndDeduplicateSources performs comprehensive source validation and deduplication
func ValidateAndDeduplicateSources(contextItems []ContextItem) ([]ContextItem, error) {
	// Step 1: Validate source metadata
	if err := ValidateSourceMetadata(contextItems); err != nil {
		return nil, fmt.Errorf("source validation failed: %w", err)
	}

	// Step 2: Remove exact duplicates by SourceID
	deduplicated := DeduplicateSourcesByID(contextItems)

	// Step 3: Remove similar content (configurable threshold)
	const contentSimilarityThreshold = 0.8
	finalItems := DeduplicateSourcesByContent(deduplicated, contentSimilarityThreshold)

	return finalItems, nil
}

// isValidURL performs basic URL validation for web search results
func isValidURL(url string) bool {
	if strings.TrimSpace(url) == "" {
		return false
	}

	// Check for basic URL structure
	url = strings.TrimSpace(url)
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return false
	}

	// Check for minimum URL length
	if len(url) < MinURLLength {
		return false
	}

	// Check for valid characters (basic check)
	for _, char := range url {
		if char < 32 || char > 126 {
			return false
		}
	}

	return true
}

// TrackWebSearchSources extracts and validates URLs from web search results
func TrackWebSearchSources(webResults []string) []string {
	var validURLs []string
	seenURLs := make(map[string]bool)

	for _, result := range webResults {
		url := extractURLFromWebResult(result)
		if url != "" && isValidURL(url) && !seenURLs[url] {
			validURLs = append(validURLs, url)
			seenURLs[url] = true
		}
	}

	return validURLs
}

// buildCodeSecurityRequirements creates security and quality requirements for code generation
func buildCodeSecurityRequirements() string {
	return `### Code Quality and Security Requirements
- NEVER include hardcoded secrets, API keys, passwords, or sensitive data
- Use environment variables, parameter stores, or secret management services for sensitive configuration
- Implement least-privilege access principles
- Include meaningful comments explaining complex logic
- Add error handling and validation for all inputs
- Use secure defaults and encryption configurations
- Include proper indentation and structure for readability
- Follow language-specific security best practices

### Security Best Practices
- Enable encryption in transit and at rest
- Implement proper authentication and authorization
- Use secure communication protocols (HTTPS, TLS)
- Apply principle of least privilege for access controls
- Regular security patching and updates
- Implement logging and monitoring for security events

### Code Block Formatting Requirements
- Use proper language identifiers for syntax highlighting
- Include descriptive comments within code blocks
- Follow consistent indentation and formatting standards
- Add line breaks for readability in complex configurations
- Use meaningful variable and resource names

### Conditional Code Generation Based on Platform
- Detect platform context from query (AWS, Azure, GCP, hybrid)
- Adapt code examples to the specific cloud provider
- Include platform-specific best practices and conventions
- Use appropriate tooling and services for each platform
- Provide cross-platform alternatives when applicable

### Error Handling Patterns
- Implement try-catch blocks for exception handling
- Add validation for input parameters and configurations
- Include graceful degradation for service failures
- Provide clear error messages and logging
- Implement retry logic with exponential backoff

### Documentation Requirements
- Include inline documentation for complex operations
- Add parameter descriptions and usage examples
- Include troubleshooting steps and common issues
- Document any prerequisites or dependencies
- Provide clear installation and configuration instructions

### Fallback Instructions for Non-Technical Queries
- For non-technical queries, focus on explanatory content
- Provide conceptual overviews instead of code implementations
- Include high-level architectural guidance
- Offer business-focused recommendations and considerations
- Suggest when technical implementation would be beneficial

`
}

// formatConversationHistory formats conversation history for inclusion in prompts with token-aware truncation

// formatConversationHistoryWithTokenLimit formats conversation history with a specific token limit
func formatConversationHistoryWithTokenLimit(conversationHistory []session.Message, maxTokens int) string {
	if len(conversationHistory) == 0 || maxTokens <= 0 {
		return ""
	}

	var builder strings.Builder
	currentTokens := 0

	// Filter and reverse to process most recent messages first
	filteredMessages := filterConversationForPrompt(conversationHistory)
	if len(filteredMessages) == 0 {
		return ""
	}

	// Process messages in reverse order (most recent first) to fit within token limit
	var includedMessages []session.Message
	truncationNotice := ""

	for i := len(filteredMessages) - 1; i >= 0; i-- {
		message := filteredMessages[i]

		// Format the message to estimate tokens
		roleDisplay := formatMessageRole(message.Role)
		timestamp := message.Timestamp.Format("15:04")
		formattedMessage := fmt.Sprintf("%s [%s]: %s\n\n", roleDisplay, timestamp, message.Content)

		messageTokens := EstimateTokens(formattedMessage)

		// Check if adding this message would exceed the limit
		if currentTokens+messageTokens > maxTokens {
			// Try to truncate the message content to fit
			remainingTokens := maxTokens - currentTokens - EstimateTokens(fmt.Sprintf("%s [%s]: \n\n", roleDisplay, timestamp))

			if remainingTokens > 100 { // Only include if we have reasonable space
				truncateMessageContentToTokens(message.Content, remainingTokens)
				includedMessages = append([]session.Message{message}, includedMessages...)
			}

			// Set truncation notice if we're excluding messages
			if i > 0 {
				truncationNotice = "...(earlier messages truncated to fit context window)...\n\n"
			}
			break
		}

		includedMessages = append([]session.Message{message}, includedMessages...)
		currentTokens += messageTokens
	}

	// Build the final conversation history
	if truncationNotice != "" {
		builder.WriteString(truncationNotice)
	}

	for _, message := range includedMessages {
		roleDisplay := formatMessageRole(message.Role)
		timestamp := message.Timestamp.Format("15:04")
		builder.WriteString(fmt.Sprintf("%s [%s]: %s\n\n", roleDisplay, timestamp, message.Content))
	}

	return builder.String()
}

// formatMessageRole formats a message role for display
func formatMessageRole(role session.MessageRole) string {
	switch role {
	case session.UserRole:
		return "User"
	case session.AssistantRole:
		return "Assistant"
	case session.SystemRole:
		return "System"
	default:
		return string(role)
	}
}

// truncateMessageContentToTokens truncates message content to fit within a token limit
func truncateMessageContentToTokens(content string, maxTokens int) string {
	if EstimateTokens(content) <= maxTokens {
		return content
	}

	// Calculate target character count
	targetChars := int(float64(maxTokens) * TokenEstimateRatio * TruncationSafetyRatio)
	runes := []rune(content)

	if len(runes) > targetChars {
		return string(runes[:targetChars]) + "...[truncated]"
	}

	return content
}

// filterConversationForPrompt filters conversation messages for prompt inclusion
func filterConversationForPrompt(messages []session.Message) []session.Message {
	var filtered []session.Message

	for _, message := range messages {
		// Include user and assistant messages, exclude system messages for brevity
		if message.Role == session.UserRole || message.Role == session.AssistantRole {
			// Truncate very long messages to prevent prompt overflow
			const maxMessageLength = 1500
			if len(message.Content) > maxMessageLength {
				truncatedMessage := message
				truncatedMessage.Content = message.Content[:maxMessageLength] + "...[truncated]"
				filtered = append(filtered, truncatedMessage)
			} else {
				filtered = append(filtered, message)
			}
		}
	}

	return filtered
}

// buildContextSourceInfo creates detailed context source information
func buildContextSourceInfo(contextItems []ContextItem, citedSources []string) []ContextSourceInfo {
	var contextSources []ContextSourceInfo
	citedSourceMap := make(map[string]bool)

	for _, source := range citedSources {
		citedSourceMap[source] = true
	}

	for i, item := range contextItems {
		preview := item.Content
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}

		sourceType := detectSourceType(item.SourceID)

		contextSource := ContextSourceInfo{
			SourceID:   item.SourceID,
			Title:      extractTitleFromSourceID(item.SourceID),
			Confidence: item.Score,
			Relevance:  calculateRelevanceScore(item),
			ChunkIndex: i,
			Preview:    preview,
			SourceType: sourceType,
			TokenCount: EstimateTokens(item.Content),
			Used:       citedSourceMap[item.SourceID],
		}

		contextSources = append(contextSources, contextSource)
	}

	return contextSources
}

// buildWebSourceInfo creates detailed web source information
func buildWebSourceInfo(webResults []string, citedSources []string) []WebSourceInfo {
	var webSources []WebSourceInfo
	citedSourceMap := make(map[string]bool)

	for _, source := range citedSources {
		citedSourceMap[source] = true
	}

	for _, result := range webResults {
		url := extractURLFromWebResult(result)
		if url == "" {
			continue
		}

		title, snippet := extractTitleAndSnippetFromWebResult(result)
		domain := extractDomainFromURL(url)

		webSource := WebSourceInfo{
			URL:        url,
			Title:      title,
			Snippet:    snippet,
			Confidence: 0.8, // Default confidence for web results
			Freshness:  "recent",
			Domain:     domain,
			Used:       citedSourceMap[url],
		}

		webSources = append(webSources, webSource)
	}

	return webSources
}

// detectSourceType determines the type of source based on its ID
func detectSourceType(sourceID string) string {
	sourceIDLower := strings.ToLower(sourceID)

	if strings.Contains(sourceIDLower, "runbook") {
		return "runbook"
	}
	if strings.Contains(sourceIDLower, "playbook") {
		return "playbook"
	}
	if strings.Contains(sourceIDLower, "sow") {
		return "sow"
	}
	if strings.Contains(sourceIDLower, "guide") {
		return "guide"
	}
	if strings.Contains(sourceIDLower, "policy") {
		return "policy"
	}

	return "internal_doc"
}

// extractTitleFromSourceID creates a human-readable title from a source ID
func extractTitleFromSourceID(sourceID string) string {
	// Remove file extension
	title := strings.TrimSuffix(sourceID, ".md")
	title = strings.TrimSuffix(title, ".pdf")
	title = strings.TrimSuffix(title, ".txt")

	// Replace hyphens and underscores with spaces
	title = strings.ReplaceAll(title, "-", " ")
	title = strings.ReplaceAll(title, "_", " ")

	// Capitalize first letter of each word
	words := strings.Fields(title)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(string(word[0])) + strings.ToLower(word[1:])
		}
	}

	return strings.Join(words, " ")
}

// calculateRelevanceScore calculates a relevance score based on context item properties
func calculateRelevanceScore(item ContextItem) float64 {
	// Base relevance is the confidence score
	relevance := item.Score

	// Boost relevance based on priority
	priorityBoost := float64(item.Priority) * 0.1
	relevance += priorityBoost

	// Ensure relevance is between 0 and 1
	if relevance > 1.0 {
		relevance = 1.0
	}
	if relevance < 0.0 {
		relevance = 0.0
	}

	return relevance
}

// extractTitleAndSnippetFromWebResult extracts title and snippet from web search result
func extractTitleAndSnippetFromWebResult(result string) (string, string) {
	lines := strings.Split(result, "\n")
	title := ""
	snippet := ""

	for _, line := range lines {
		if strings.HasPrefix(line, "Title: ") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "Title: "))
		} else if strings.HasPrefix(line, "Snippet: ") {
			snippet = strings.TrimSpace(strings.TrimPrefix(line, "Snippet: "))
		}
	}

	return title, snippet
}

// extractDomainFromURL extracts the domain from a URL
func extractDomainFromURL(url string) string {
	// Remove protocol
	domain := strings.TrimPrefix(url, "https://")
	domain = strings.TrimPrefix(domain, "http://")

	// Extract domain part before first slash
	if slashIndex := strings.Index(domain, "/"); slashIndex != -1 {
		domain = domain[:slashIndex]
	}

	// Remove www prefix
	domain = strings.TrimPrefix(domain, "www.")

	return domain
}

// BuildClarificationPrompt builds a prompt for generating clarification questions
func BuildClarificationPrompt(query string, contextItems []ContextItem, conversationHistory []session.Message) string {
	var prompt strings.Builder

	// System prompt for clarification
	prompt.WriteString(`You are an expert Solutions Architect assistant specializing in cloud technologies. Your role is to analyze user queries and determine if they need clarification before providing a comprehensive response.

When a query is ambiguous, incomplete, or lacks sufficient context, you should ask clarifying questions rather than making assumptions.

## Analysis Guidelines:

### Ambiguous Queries - Ask for clarification when:
- Query uses vague terms like "best", "good", "simple" without criteria
- Multiple valid interpretations exist
- Key requirements are not specified
- Scale or scope is unclear

### Incomplete Queries - Ask for clarification when:
- Missing critical context (source/target for migrations)
- No compliance or security requirements specified
- Timeline or budget constraints not mentioned
- Technical requirements incomplete

### Follow-up Questions - When user references:
- "That diagram" - ask which specific element they want modified
- "More details" - ask about which specific area needs elaboration
- "Alternative approach" - ask about specific constraints or preferences

## Response Format:
If clarification is needed, respond with:

**CLARIFICATION_NEEDED**

**Questions:**
[List 2-3 specific questions that would help provide a better response]

**Suggestions:**
[Provide 2-3 helpful suggestions for improving the query]

**Quick Options:**
[Provide 3-4 quick selection options if applicable]

If the query is clear and complete, respond with:

**PROCEED_WITH_SYNTHESIS**

`)

	// Add conversation history if available
	if len(conversationHistory) > 0 {
		prompt.WriteString("--- Previous Conversation Context ---\n")
		conversationContext := formatConversationHistoryWithTokenLimit(conversationHistory, 800)
		prompt.WriteString(conversationContext)
		prompt.WriteString("\n")
	}

	// Add current query
	prompt.WriteString(fmt.Sprintf("Current User Query: %s\n\n", query))

	// Add any available context
	if len(contextItems) > 0 {
		prompt.WriteString("--- Available Context ---\n")
		for i, item := range contextItems {
			if i >= 3 { // Limit context for clarification analysis
				break
			}
			prompt.WriteString(fmt.Sprintf("Context %d [%s]: %s\n\n", i+1, item.SourceID, item.Content))
		}
	}

	prompt.WriteString("Please analyze this query and determine if clarification is needed:")

	return prompt.String()
}

// BuildFollowupPrompt builds a prompt for handling follow-up questions with context resolution
func BuildFollowupPrompt(query string, contextItems []ContextItem, conversationHistory []session.Message, previousResponse string) string {
	var prompt strings.Builder

	// System prompt for follow-up handling
	prompt.WriteString(`You are an expert Solutions Architect assistant handling a follow-up question in an ongoing conversation.

## Follow-up Analysis:
- Identify what the user is referencing from the previous response
- Resolve references like "that diagram", "the security section", "mentioned earlier"
- Build upon the previous response while addressing the new request

## Context Resolution Guidelines:
- When user says "that diagram" - reference the specific architecture diagram from previous response
- When user says "more details" - identify which section needs elaboration
- When user asks "what about costs" - add cost analysis to the previous solution
- When user asks for "alternatives" - provide different approaches to the same problem

## Response Strategy:
- Acknowledge the reference to previous content
- Provide the requested information or modification
- Maintain consistency with the previous response
- Enhance rather than replace the previous solution

`)

	// Add conversation history
	if len(conversationHistory) > 0 {
		prompt.WriteString("--- Previous Conversation ---\n")
		conversationContext := formatConversationHistoryWithTokenLimit(conversationHistory, 1200)
		prompt.WriteString(conversationContext)
		prompt.WriteString("\n")
	}

	// Add previous response if provided
	if previousResponse != "" {
		prompt.WriteString("--- Previous Response to Reference ---\n")
		// Truncate previous response if too long
		if len(previousResponse) > 2000 {
			previousResponse = previousResponse[:2000] + "...(truncated)"
		}
		prompt.WriteString(previousResponse)
		prompt.WriteString("\n\n")
	}

	// Add current follow-up query
	prompt.WriteString(fmt.Sprintf("Follow-up Query: %s\n\n", query))

	// Add context items
	if len(contextItems) > 0 {
		prompt.WriteString("--- Additional Context ---\n")
		for i, item := range contextItems {
			if i >= 5 { // Limit context for follow-up
				break
			}
			prompt.WriteString(fmt.Sprintf("Context %d [%s]: %s\n\n", i+1, item.SourceID, item.Content))
		}
	}

	prompt.WriteString("Please provide a response that addresses the follow-up question while referencing and building upon the previous response:")

	return prompt.String()
}

// IsQueryClear analyzes if a query is clear enough to proceed without clarification
func IsQueryClear(query string) bool {
	queryLower := strings.ToLower(query)

	// Check for ambiguous terms
	ambiguousTerms := []string{"best", "good", "better", "simple", "easy", "quick", "basic", "general", "overview"}
	for _, term := range ambiguousTerms {
		if strings.Contains(queryLower, term) && !hasSpecificContext(queryLower) {
			return false
		}
	}

	// Check if query is too short or generic
	if len(strings.Fields(query)) < 5 {
		return false
	}

	// Check for overly generic queries
	genericPatterns := []string{
		"help me", "assist me", "how do i", "what should i",
		"migrate to cloud", "security plan", "architecture design",
	}

	for _, pattern := range genericPatterns {
		if strings.Contains(queryLower, pattern) && !hasSpecificContext(queryLower) {
			return false
		}
	}

	return true
}

// hasSpecificContext checks if the query contains specific contextual information
func hasSpecificContext(queryLower string) bool {
	specificTerms := []string{
		// Cloud providers
		"aws", "azure", "gcp", "google cloud",
		// Technologies
		"vmware", "hyper-v", "kubernetes", "docker", "terraform",
		// Compliance
		"hipaa", "gdpr", "sox", "pci", "iso 27001",
		// Specifics
		"production", "staging", "development", "gb", "tb", "users",
		"million", "thousand", "hours", "minutes", "days", "months",
	}

	for _, term := range specificTerms {
		if strings.Contains(queryLower, term) {
			return true
		}
	}

	return false
}

// DetectFollowupType detects the type of follow-up question
func DetectFollowupType(query string) string {
	queryLower := strings.ToLower(query)

	// Reference patterns
	if strings.Contains(queryLower, "that") || strings.Contains(queryLower, "this") ||
		strings.Contains(queryLower, "the above") || strings.Contains(queryLower, "mentioned") {
		return "reference"
	}

	// Expansion patterns
	if strings.Contains(queryLower, "more") || strings.Contains(queryLower, "detail") ||
		strings.Contains(queryLower, "expand") || strings.Contains(queryLower, "elaborate") {
		return "expansion"
	}

	// Alternative patterns
	if strings.Contains(queryLower, "different") || strings.Contains(queryLower, "alternative") ||
		strings.Contains(queryLower, "another") || strings.Contains(queryLower, "instead") {
		return "alternative"
	}

	// Modification patterns
	if strings.Contains(queryLower, "change") || strings.Contains(queryLower, "modify") ||
		strings.Contains(queryLower, "update") || strings.Contains(queryLower, "improve") {
		return "modification"
	}

	// Specific inquiry patterns
	if strings.Contains(queryLower, "cost") || strings.Contains(queryLower, "price") {
		return "cost_inquiry"
	}

	if strings.Contains(queryLower, "security") || strings.Contains(queryLower, "compliance") {
		return "security_inquiry"
	}

	if strings.Contains(queryLower, "diagram") || strings.Contains(queryLower, "architecture") {
		return "diagram_inquiry"
	}

	return "general_followup"
}

// validateCodeGeneration validates that migration queries generate expected code snippets
func validateCodeGeneration(query string, codeSnippets []CodeSnippet) {
	queryLower := strings.ToLower(query)

	// Check if this is a migration query that should generate code
	isMigrationQuery := strings.Contains(queryLower, "migration") ||
		strings.Contains(queryLower, "migrate") ||
		strings.Contains(queryLower, "lift-and-shift") ||
		strings.Contains(queryLower, "terraform") ||
		strings.Contains(queryLower, "infrastructure") ||
		strings.Contains(queryLower, "aws cli") ||
		strings.Contains(queryLower, "azure cli")

	if isMigrationQuery && len(codeSnippets) == 0 {
		log.Printf("WARNING: Migration query generated no code snippets. Query: %s", query)
		return
	}

	// Check for specific code types in migration queries
	if isMigrationQuery {
		hasInfrastructureCode := false
		hasConfigurationCode := false

		for _, snippet := range codeSnippets {
			switch snippet.Language {
			case "terraform", "tf", "hcl":
				hasInfrastructureCode = true
			case "bash", "shell", "powershell", "ps1":
				hasConfigurationCode = true
			case "yaml", "yml", "json":
				hasConfigurationCode = true
			}
		}

		if !hasInfrastructureCode && (strings.Contains(queryLower, "terraform") || strings.Contains(queryLower, "infrastructure")) {
			log.Printf("WARNING: Migration query requested infrastructure code but none was generated. Query: %s", query)
		}

		if !hasConfigurationCode && (strings.Contains(queryLower, "cli") || strings.Contains(queryLower, "script")) {
			log.Printf("WARNING: Migration query requested configuration/script code but none was generated. Query: %s", query)
		}
	}
}

// buildEnhancedCitationInstructions builds enhanced citation instructions for the prompt
func buildEnhancedCitationInstructions() string {
	return `
SOURCE CITATION REQUIREMENTS:
- When referencing information from internal documents, cite with [source_id] format
- When referencing information from web search results, cite with [URL] format
- Every factual claim should have a corresponding source citation
- Use the exact source identifiers provided in the context sections above

`
}
