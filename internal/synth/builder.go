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
	DefaultMaxTokens       = 6000
	DefaultMaxContextItems = 10
	DefaultMaxWebResults   = 5
	MinimalPromptLength    = 100
	TokenEstimateRatio     = 4
	TruncationSafetyRatio  = 0.9
	MinCodeMatchGroups     = 3
	MinSourceMatchGroups   = 2
	MinURLLength           = 10
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
	MainText     string        `json:"main_text"`
	DiagramCode  string        `json:"diagram_code"`
	DiagramURL   string        `json:"diagram_url,omitempty"`
	CodeSnippets []CodeSnippet `json:"code_snippets"`
	Sources      []string      `json:"sources"`
}

// CodeSnippet represents a code snippet with its language
type CodeSnippet struct {
	Language string `json:"language"`
	Code     string `json:"code"`
}

// BuildPrompt combines context into a comprehensive prompt for the LLM
func BuildPrompt(query string, contextItems []ContextItem, webResults []string) string {
	config := DefaultPromptConfig()
	config.QueryType = DetectQueryType(query)
	return BuildPromptWithConfig(query, contextItems, webResults, config)
}

// BuildPromptWithConversation combines context and conversation history into a comprehensive prompt for the LLM
func BuildPromptWithConversation(query string, contextItems []ContextItem, webResults []string, conversationHistory []session.Message) string {
	config := DefaultPromptConfig()
	config.QueryType = DetectQueryType(query)
	return BuildPromptWithConversationAndConfig(query, contextItems, webResults, conversationHistory, config)
}

// BuildPromptWithConfig combines context into a comprehensive prompt with configuration
func BuildPromptWithConfig(query string, contextItems []ContextItem, webResults []string, config PromptConfig) string {
	// Validate and deduplicate sources before processing
	validatedContext, err := ValidateAndDeduplicateSources(contextItems)
	if err != nil {
		// Log warning but continue with original context if validation fails
		validatedContext = contextItems
	}

	// Prioritize and limit context based on token constraints
	optimizedContext := PrioritizeContext(validatedContext, config.MaxContextItems)
	limitedWebResults := LimitWebResults(webResults, config.MaxWebResults)

	var prompt strings.Builder

	// System instructions based on query type
	systemPrompt := buildSystemPrompt(config.QueryType)
	prompt.WriteString(systemPrompt)

	// User query
	prompt.WriteString(fmt.Sprintf("User Query: %s\n\n", query))

	// Internal document context
	if len(optimizedContext) > 0 {
		prompt.WriteString("--- Internal Document Context ---\n")
		for i, item := range optimizedContext {
			prompt.WriteString(fmt.Sprintf("Context %d [%s]: %s\n\n", i+1, item.SourceID, item.Content))
		}
	}

	// Web search results with enhanced URL tracking
	if len(limitedWebResults) > 0 {
		prompt.WriteString("--- Live Web Search Results ---\n")
		for i, result := range limitedWebResults {
			formattedResult := formatWebResultWithURL(i+1, result)
			prompt.WriteString(formattedResult)
		}
	}

	prompt.WriteString("\nPlease provide your comprehensive response now:")

	// Ensure token limits are respected
	finalPrompt := prompt.String()
	if EstimateTokens(finalPrompt) > config.MaxTokens {
		finalPrompt = TruncateToTokenLimit(finalPrompt, config.MaxTokens)
	}

	return finalPrompt
}

// BuildPromptWithConversationAndConfig combines context and conversation history into a comprehensive prompt with configuration
func BuildPromptWithConversationAndConfig(query string, contextItems []ContextItem, webResults []string, conversationHistory []session.Message, config PromptConfig) string {
	// Validate and deduplicate sources before processing
	validatedContext, err := ValidateAndDeduplicateSources(contextItems)
	if err != nil {
		// Log warning but continue with original context if validation fails
		validatedContext = contextItems
	}

	// Prioritize and limit context based on token constraints
	optimizedContext := PrioritizeContext(validatedContext, config.MaxContextItems)
	limitedWebResults := LimitWebResults(webResults, config.MaxWebResults)

	var prompt strings.Builder

	// System instructions based on query type
	systemPrompt := buildSystemPrompt(config.QueryType)
	prompt.WriteString(systemPrompt)

	// Conversation history (if available)
	if len(conversationHistory) > 0 {
		prompt.WriteString("--- Previous Conversation Context ---\n")
		conversationContext := formatConversationHistory(conversationHistory)
		prompt.WriteString(conversationContext)
		prompt.WriteString("\n")
	}

	// User query
	prompt.WriteString(fmt.Sprintf("Current User Query: %s\n\n", query))

	// Internal document context
	if len(optimizedContext) > 0 {
		prompt.WriteString("--- Internal Document Context ---\n")
		for i, item := range optimizedContext {
			prompt.WriteString(fmt.Sprintf("Context %d [%s]: %s\n\n", i+1, item.SourceID, item.Content))
		}
	}

	// Web search results with enhanced URL tracking
	if len(limitedWebResults) > 0 {
		prompt.WriteString("--- Live Web Search Results ---\n")
		for i, result := range limitedWebResults {
			formattedResult := formatWebResultWithURL(i+1, result)
			prompt.WriteString(formattedResult)
		}
	}

	// Add instruction about conversation continuity
	if len(conversationHistory) > 0 {
		prompt.WriteString("\nIMPORTANT: This is a continuation of an ongoing conversation. Please:\n")
		prompt.WriteString("- Reference previous context when relevant\n")
		prompt.WriteString("- Build upon earlier discussions\n")
		prompt.WriteString("- Maintain conversation continuity\n")
		prompt.WriteString("- Use phrases like 'As we discussed earlier' when appropriate\n\n")
	}

	prompt.WriteString("Please provide your comprehensive response now:")

	// Ensure token limits are respected
	finalPrompt := prompt.String()
	if EstimateTokens(finalPrompt) > config.MaxTokens {
		finalPrompt = TruncateToTokenLimit(finalPrompt, config.MaxTokens)
	}

	return finalPrompt
}

// ParseResponse parses the LLM response into structured components
func ParseResponse(response string) SynthesisResponse {
	return ParseResponseWithSources(response, []string{})
}

// ParseResponseWithSources parses the LLM response into structured components with source validation
func ParseResponseWithSources(response string, availableSources []string) SynthesisResponse {
	result := SynthesisResponse{
		MainText:     response,
		CodeSnippets: []CodeSnippet{},
		Sources:      []string{},
	}

	// Extract diagram code
	if diagramCode := extractMermaidDiagram(response); diagramCode != "" {
		result.DiagramCode = diagramCode
		// Remove diagram from main text
		result.MainText = removeMermaidDiagram(response)
	}

	// Extract code snippets
	codeSnippets := extractCodeSnippets(response)
	result.CodeSnippets = codeSnippets
	// Remove code snippets from main text
	result.MainText = removeCodeSnippets(result.MainText)

	// Extract sources from citations
	citedSources := extractSources(response)

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

	// Regex to match code blocks with language identifiers
	codeRegex := regexp.MustCompile("```(\\w+)\\s*\\n([\\s\\S]*?)\\n```")
	matches := codeRegex.FindAllStringSubmatch(response, -1)

	for _, match := range matches {
		if len(match) >= MinCodeMatchGroups {
			language := match[1]
			code := strings.TrimSpace(match[2])

			// Skip mermaid blocks (handled separately)
			if language != "mermaid" && code != "" {
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

	return snippets
}

// removeCodeSnippets removes code blocks from text
func removeCodeSnippets(text string) string {
	codeRegex := regexp.MustCompile("```\\w*\\s*\\n[\\s\\S]*?\\n```")
	return codeRegex.ReplaceAllString(text, "")
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
	basePrompt := `You are an expert Cloud Solutions Architect assistant. ` +
		`Your role is to help Solutions Architects with pre-sales research and planning.

Your response must be structured and comprehensive. Please provide:

1. A detailed, actionable answer to the user's query
2. If applicable, generate a high-level architecture diagram using Mermaid.js graph TD syntax
3. If applicable, provide relevant code snippets for implementation
4. Always cite your sources using [source_id] format when referencing any information

SOURCE CITATION REQUIREMENTS:
- When referencing information from internal documents, cite with [source_id] format
- When referencing information from web search results, cite with [URL] format
- Place citations at the end of sentences or paragraphs where the information is used
- Every factual claim should have a corresponding source citation
- Use the exact source identifiers provided in the context sections above

Guidelines:
- Be specific and actionable in your recommendations
- Include technical details and best practices
- For diagrams: Use Mermaid.js graph TD syntax enclosed in a "mermaid" code block
- For code: Use appropriate language identifiers (terraform, bash, yaml, etc.)
- Citations: End sentences with [source_id] or [URL] when using information from any source
- Focus on practical implementation guidance

`

	diagramInstructions := buildDiagramInstructions(queryType)
	codeInstructions := buildCodeGenerationInstructions(queryType)

	switch queryType {
	case TechnicalQuery:
		technicalFocus := `TECHNICAL FOCUS: Emphasize technical implementation details, ` +
			`code examples, architectural patterns, and best practices. ` +
			`Provide specific configuration examples and troubleshooting guidance.

`
		return basePrompt + diagramInstructions + codeInstructions + technicalFocus
	case BusinessQuery:
		businessFocus := `BUSINESS FOCUS: Emphasize business value, cost considerations, ` +
			`ROI analysis, timeline estimates, and strategic implications. ` +
			`Include risk assessments and compliance considerations.

`
		return basePrompt + diagramInstructions + codeInstructions + businessFocus
	case GeneralQuery:
		return basePrompt + diagramInstructions + codeInstructions
	default:
		return basePrompt + diagramInstructions + codeInstructions
	}
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

	architectureKeywords := []string{
		// Core architecture terms
		"architecture", "design", "topology", "infrastructure", "deployment",
		"migration", "lift-and-shift", "lift and shift", "disaster recovery",
		"backup", "replication", "failover", "high availability", "scalability",

		// Cloud platforms
		"aws", "azure", "gcp", "google cloud", "cloud", "hybrid", "multi-cloud",
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
	return `#### Terraform (Infrastructure as Code)
- Use for cloud infrastructure provisioning
- Include provider configuration (AWS, Azure, GCP)
- Use meaningful resource names with proper naming conventions
- Include data sources for existing resources
- Add variable definitions and output values
- Format: ` + "`terraform`" + `

Example Pattern:
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
` + "```" + `

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
	unsafePatterns := []string{
		"destroy = true",
		"prevent_destroy = false",
		"skip_final_snapshot = true",
		"deletion_protection = false",
		"force_destroy = true",
		"0.0.0.0/0", // Overly permissive CIDR
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

// normalizeLanguage normalizes language identifiers to standard names
func normalizeLanguage(language string) string {
	langMap := map[string]string{
		"bash":       "bash",
		"sh":         "bash",
		"shell":      "bash",
		"zsh":        "bash",
		"terraform":  "terraform",
		"tf":         "terraform",
		"hcl":        "terraform",
		"powershell": "powershell",
		"ps1":        "powershell",
		"pwsh":       "powershell",
		"yaml":       "yaml",
		"yml":        "yaml",
		"json":       "json",
		"javascript": "javascript",
		"js":         "javascript",
		"python":     "python",
		"py":         "python",
		"go":         "go",
		"golang":     "go",
		"java":       "java",
		"c":          "c",
		"cpp":        "cpp",
		"c++":        "cpp",
		"csharp":     "csharp",
		"cs":         "csharp",
		"php":        "php",
		"ruby":       "ruby",
		"rb":         "ruby",
		"rust":       "rust",
		"rs":         "rust",
		"sql":        "sql",
		"dockerfile": "dockerfile",
		"docker":     "dockerfile",
		"aws":        "bash", // AWS CLI is typically bash
		"azure":      "bash", // Azure CLI is typically bash
		"gcp":        "bash", // GCP CLI is typically bash
		"k8s":        "yaml", // Kubernetes manifests are YAML
		"kubernetes": "yaml",
		"helm":       "yaml",
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

// formatConversationHistory formats conversation history for inclusion in prompts
func formatConversationHistory(conversationHistory []session.Message) string {
	if len(conversationHistory) == 0 {
		return ""
	}

	var builder strings.Builder

	// Filter and limit conversation history to prevent prompt overflow
	filteredMessages := filterConversationForPrompt(conversationHistory)
	const maxHistoryMessages = 10

	startIndex := 0
	if len(filteredMessages) > maxHistoryMessages {
		startIndex = len(filteredMessages) - maxHistoryMessages
		builder.WriteString("...(earlier messages truncated for brevity)...\n\n")
	}

	for i := startIndex; i < len(filteredMessages); i++ {
		message := filteredMessages[i]

		// Format role name
		var roleDisplay string
		switch message.Role {
		case session.UserRole:
			roleDisplay = "User"
		case session.AssistantRole:
			roleDisplay = "Assistant"
		case session.SystemRole:
			roleDisplay = "System"
		default:
			roleDisplay = string(message.Role)
		}

		// Format timestamp for context
		timestamp := message.Timestamp.Format("15:04")

		// Add the message
		builder.WriteString(fmt.Sprintf("%s [%s]: %s\n\n", roleDisplay, timestamp, message.Content))
	}

	return builder.String()
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
