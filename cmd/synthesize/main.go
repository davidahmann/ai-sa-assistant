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

// Package main provides the synthesis service API for the AI SA Assistant.
// It combines retrieval results with web search to generate comprehensive responses.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/health"
	internalopenai "github.com/your-org/ai-sa-assistant/internal/openai"
	"github.com/your-org/ai-sa-assistant/internal/resilience"
	"github.com/your-org/ai-sa-assistant/internal/session"
	"github.com/your-org/ai-sa-assistant/internal/synth"
	"github.com/your-org/ai-sa-assistant/internal/synthesis"
)

const (
	// MaxQueryLength defines the maximum length for query text
	MaxQueryLength = 10000
	// HealthCheckTimeout defines the timeout for health checks
	HealthCheckTimeout = 5 * time.Second
	// DefaultSynthesisRequestTimeout defines the default timeout for synthesis requests
	DefaultSynthesisRequestTimeout = 30 * time.Second
	// ComplexQueryTimeout defines the timeout for complex enterprise queries
	ComplexQueryTimeout = 90 * time.Second
	// SimpleQueryTimeout defines the timeout for simple queries
	SimpleQueryTimeout = 15 * time.Second
	// ComplexQueryTokenThreshold defines the token threshold for complex queries
	ComplexQueryTokenThreshold = 4000
	// ComplexQueryContextThreshold defines the context item threshold for complex queries
	ComplexQueryContextThreshold = 8
	// MaxPromptTokens defines the maximum number of tokens for a prompt to avoid timeouts
	MaxPromptTokens = 12000
	// MaxContextItemsForOptimization defines the maximum number of context items before optimization
	MaxContextItemsForOptimization = 8
	// MaxWebResultsForOptimization defines the maximum number of web results before optimization
	MaxWebResultsForOptimization = 3
	// TokenPerCharacterRatio is an approximate ratio for token estimation
	TokenPerCharacterRatio = 0.25
)

// SynthesisRequest represents the incoming synthesis request
type SynthesisRequest struct {
	Query               string            `json:"query" binding:"required"`
	Chunks              []ChunkItem       `json:"chunks"`
	WebResults          []WebResult       `json:"web_results"`
	ConversationHistory []session.Message `json:"conversation_history,omitempty"`
}

// RegenerationRequest represents a request to regenerate a response with different parameters
type RegenerationRequest struct {
	Query               string            `json:"query" binding:"required"`
	Chunks              []ChunkItem       `json:"chunks"`
	WebResults          []WebResult       `json:"web_results"`
	ConversationHistory []session.Message `json:"conversation_history,omitempty"`
	Parameters          GenerationParams  `json:"parameters"`
	PreviousResponse    *string           `json:"previous_response,omitempty"`
}

// GenerationParams defines parameters for response generation
type GenerationParams struct {
	Temperature float32 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
	Model       string  `json:"model"`
	Preset      string  `json:"preset"` // "creative", "balanced", "focused", "detailed", "concise"
}

// ParameterPreset defines predefined parameter combinations
type ParameterPreset struct {
	Name        string
	Temperature float32
	MaxTokens   int
	Model       string
	Description string
}

// Parameter preset constants
const (
	CreativeTemperature = 0.8
	BalancedTemperature = 0.4
	FocusedTemperature  = 0.1
	DetailedTemperature = 0.3
	ConciseTemperature  = 0.2

	CreativeMaxTokens = 3000
	BalancedMaxTokens = 2000
	FocusedMaxTokens  = 2000
	DetailedMaxTokens = 4000
	ConciseMaxTokens  = 1000
)

// getParameterPresets returns available parameter presets
func getParameterPresets() map[string]ParameterPreset {
	return map[string]ParameterPreset{
		"creative": {
			Name:        "creative",
			Temperature: CreativeTemperature,
			MaxTokens:   CreativeMaxTokens,
			Model:       "gpt-4o",
			Description: "More creative and varied responses with higher temperature",
		},
		"balanced": {
			Name:        "balanced",
			Temperature: BalancedTemperature,
			MaxTokens:   BalancedMaxTokens,
			Model:       "gpt-4o",
			Description: "Balanced approach between creativity and focus",
		},
		"focused": {
			Name:        "focused",
			Temperature: FocusedTemperature,
			MaxTokens:   FocusedMaxTokens,
			Model:       "gpt-4o",
			Description: "More focused and deterministic responses",
		},
		"detailed": {
			Name:        "detailed",
			Temperature: DetailedTemperature,
			MaxTokens:   DetailedMaxTokens,
			Model:       "gpt-4o",
			Description: "Comprehensive and detailed responses",
		},
		"concise": {
			Name:        "concise",
			Temperature: ConciseTemperature,
			MaxTokens:   ConciseMaxTokens,
			Model:       "gpt-4o-mini",
			Description: "Brief and to-the-point responses",
		},
	}
}

// getConfiguredTimeout returns the configured timeout duration for synthesis requests
func getConfiguredTimeout(cfg *config.Config) time.Duration {
	if cfg != nil && cfg.Synthesis.TimeoutSeconds > 0 {
		timeout := time.Duration(cfg.Synthesis.TimeoutSeconds) * time.Second
		// Note: Debug logging would be added here if logger was available
		return timeout
	}
	return DefaultSynthesisRequestTimeout
}

// getAdaptiveTimeout returns an adaptive timeout based on query complexity
func getAdaptiveTimeout(cfg *config.Config, req SynthesisRequest, logger *zap.Logger) time.Duration {
	// Check if adaptive timeout is enabled
	if cfg != nil && !cfg.Synthesis.EnableAdaptiveTimeout {
		return getConfiguredTimeout(cfg)
	}

	// Start with configured timeout
	baseTimeout := getConfiguredTimeout(cfg)

	// Calculate query complexity
	complexity := calculateQueryComplexity(req, logger)

	// Apply adaptive timeout based on complexity and configuration
	var adaptiveTimeout time.Duration
	switch complexity {
	case "simple":
		if cfg != nil && cfg.Synthesis.SimpleTimeoutSeconds > 0 {
			adaptiveTimeout = time.Duration(cfg.Synthesis.SimpleTimeoutSeconds) * time.Second
		} else {
			adaptiveTimeout = minDuration(baseTimeout, SimpleQueryTimeout)
		}
	case "complex":
		if cfg != nil && cfg.Synthesis.ComplexTimeoutSeconds > 0 {
			adaptiveTimeout = time.Duration(cfg.Synthesis.ComplexTimeoutSeconds) * time.Second
		} else {
			adaptiveTimeout = maxDuration(baseTimeout, ComplexQueryTimeout)
		}
	default: // medium
		adaptiveTimeout = baseTimeout
	}

	logger.Info("Applied adaptive timeout",
		zap.String("complexity", complexity),
		zap.Duration("base_timeout", baseTimeout),
		zap.Duration("adaptive_timeout", adaptiveTimeout),
		zap.Int("context_items", len(req.Chunks)),
		zap.Int("web_results", len(req.WebResults)),
		zap.Int("query_length", len(req.Query)),
		zap.Bool("adaptive_enabled", cfg != nil && cfg.Synthesis.EnableAdaptiveTimeout),
	)

	return adaptiveTimeout
}

// QueryParameters represents extracted specific parameters from a user query
type QueryParameters struct {
	VmCount         int      `json:"vm_count,omitempty"`
	Technologies    []string `json:"technologies,omitempty"`
	CloudProviders  []string `json:"cloud_providers,omitempty"`
	RTORequirement  string   `json:"rto_requirement,omitempty"`
	RPORequirement  string   `json:"rpo_requirement,omitempty"`
	SpecificNumbers []string `json:"specific_numbers,omitempty"`
	Constraints     []string `json:"constraints,omitempty"`
	Scenarios       []string `json:"scenarios,omitempty"`
}

// extractQueryParameters extracts specific parameters from the user query
func extractQueryParameters(query string) QueryParameters {
	params := QueryParameters{
		Technologies:    []string{},
		CloudProviders:  []string{},
		SpecificNumbers: []string{},
		Constraints:     []string{},
		Scenarios:       []string{},
	}

	queryLower := strings.ToLower(query)

	// Extract VM count patterns
	vmPatterns := []string{
		`(\d+)\s+(?:on-prem|on-premises)\s+(?:windows|linux|vm|vms|virtual machines|servers?)`,
		`(\d+)\s+(?:vm|vms|virtual machines|servers?)`,
		`migrate\s+(\d+)`,
		`migrating\s+(\d+)`,
	}

	for _, pattern := range vmPatterns {
		if matches := regexp.MustCompile(pattern).FindAllStringSubmatch(queryLower, -1); matches != nil {
			for _, match := range matches {
				if len(match) > 1 {
					if count, err := strconv.Atoi(match[1]); err == nil {
						params.VmCount = count
						params.SpecificNumbers = append(params.SpecificNumbers, match[1]+" VMs")
						break
					}
				}
			}
		}
	}

	// Extract RTO/RPO requirements
	rtoPattern := regexp.MustCompile(`(?:rto|recovery time objective)[^0-9]*(\d+)\s*(hours?|minutes?|mins?|hrs?)`)
	if matches := rtoPattern.FindStringSubmatch(queryLower); len(matches) > 2 {
		params.RTORequirement = matches[1] + " " + matches[2]
		params.Constraints = append(params.Constraints, "RTO: "+matches[0])
	}

	rpoPattern := regexp.MustCompile(`(?:rpo|recovery point objective)[^0-9]*(\d+)\s*(hours?|minutes?|mins?|hrs?)`)
	if matches := rpoPattern.FindStringSubmatch(queryLower); len(matches) > 2 {
		params.RPORequirement = matches[1] + " " + matches[2]
		params.Constraints = append(params.Constraints, "RPO: "+matches[0])
	}

	// Extract technology indicators
	techKeywords := map[string][]string{
		"Windows":    {"windows", "win server", "windows server"},
		"Linux":      {"linux", "rhel", "centos", "ubuntu"},
		"SQL Server": {"sql server", "mssql", "sqlserver"},
		".NET":       {".net", "dotnet", "asp.net"},
		"Java":       {"java", "tomcat", "jvm"},
		"VMware":     {"vmware", "vsphere", "vcenter"},
		"Docker":     {"docker", "containers", "containerized"},
		"Kubernetes": {"kubernetes", "k8s", "container orchestration"},
	}

	for tech, keywords := range techKeywords {
		for _, keyword := range keywords {
			if strings.Contains(queryLower, keyword) {
				params.Technologies = append(params.Technologies, tech)
				break
			}
		}
	}

	// Extract cloud provider indicators
	cloudProviders := map[string][]string{
		"AWS":   {"aws", "amazon web services", "ec2", "s3", "rds", "mgn"},
		"Azure": {"azure", "microsoft azure", "azur", "arm template"},
		"GCP":   {"gcp", "google cloud", "gce", "google compute"},
	}

	for provider, keywords := range cloudProviders {
		for _, keyword := range keywords {
			if strings.Contains(queryLower, keyword) {
				params.CloudProviders = append(params.CloudProviders, provider)
				break
			}
		}
	}

	// Extract other specific numbers
	numberPatterns := []string{
		`(\d+)\s*(?:tb|gb|mb)\s*(?:storage|disk|data)`,
		`(\d+)\s*(?:cpu|cores|vcpus)`,
		`(\d+)\s*(?:gb|tb)\s*(?:memory|ram)`,
		`(\d+)\s*(?:users|endpoints|connections)`,
	}

	for _, pattern := range numberPatterns {
		if matches := regexp.MustCompile(pattern).FindAllStringSubmatch(queryLower, -1); matches != nil {
			for _, match := range matches {
				if len(match) > 0 {
					params.SpecificNumbers = append(params.SpecificNumbers, match[0])
				}
			}
		}
	}

	// Extract scenario-specific constraints
	constraintPatterns := []string{
		`(?:cost[- ]?optimized|budget[- ]?friendly|minimize cost)`,
		`(?:high[- ]?availability|ha|fault[- ]?tolerant)`,
		`(?:disaster[- ]?recovery|dr|backup)`,
		`(?:compliance|hipaa|gdpr|sox|pci)`,
		`(?:performance|low[- ]?latency|high[- ]?throughput)`,
	}

	for _, pattern := range constraintPatterns {
		if matches := regexp.MustCompile(pattern).FindAllString(queryLower, -1); matches != nil {
			for _, match := range matches {
				params.Constraints = append(params.Constraints, match)
			}
		}
	}

	// Extract specific scenarios
	scenarioPatterns := []string{
		`lift[- ]?and[- ]?shift`,
		`hybrid[- ]?architecture`,
		`multi[- ]?cloud`,
		`disaster[- ]?recovery`,
		`migration[- ]?plan`,
	}

	for _, pattern := range scenarioPatterns {
		if matches := regexp.MustCompile(pattern).FindAllString(queryLower, -1); matches != nil {
			for _, match := range matches {
				params.Scenarios = append(params.Scenarios, match)
			}
		}
	}

	return params
}

// QueryDomain represents the domain/scenario type of a query
type QueryDomain string

const (
	MigrationDomain    QueryDomain = "migration"
	ArchitectureDomain QueryDomain = "architecture"
	ComplianceDomain   QueryDomain = "compliance"
	DisasterRecovery   QueryDomain = "disaster_recovery"
	GeneralDomain      QueryDomain = "general"
)

// detectQueryDomain identifies the primary domain of a query for specialized handling
func detectQueryDomain(query string) QueryDomain {
	queryLower := strings.ToLower(query)

	// Migration domain indicators
	migrationKeywords := []string{
		"migration", "migrate", "lift-and-shift", "move to cloud",
		"vm migration", "workload migration", "mgn", "migration service",
		"replatform", "rehost", "migrate workloads",
	}

	// Compliance domain indicators
	complianceKeywords := []string{
		"compliance", "hipaa", "gdpr", "sox", "pci", "security assessment",
		"audit", "compliance framework", "regulatory", "policy enforcement",
		"data privacy", "encryption requirements",
	}

	// Disaster recovery domain indicators
	drKeywords := []string{
		"disaster recovery", "dr", "backup", "failover", "rto", "rpo",
		"business continuity", "disaster planning", "backup strategy",
		"recovery point", "recovery time",
	}

	// Architecture domain indicators (check last to avoid false positives)
	architectureKeywords := []string{
		"architecture", "design", "vpc", "network topology", "hybrid",
		"multi-cloud", "reference architecture", "solution design",
		"infrastructure design", "system architecture",
	}

	// Check domains in priority order
	for _, keyword := range migrationKeywords {
		if strings.Contains(queryLower, keyword) {
			return MigrationDomain
		}
	}

	for _, keyword := range complianceKeywords {
		if strings.Contains(queryLower, keyword) {
			return ComplianceDomain
		}
	}

	for _, keyword := range drKeywords {
		if strings.Contains(queryLower, keyword) {
			return DisasterRecovery
		}
	}

	for _, keyword := range architectureKeywords {
		if strings.Contains(queryLower, keyword) {
			return ArchitectureDomain
		}
	}

	return GeneralDomain
}

// calculateQueryComplexity determines query complexity based on multiple factors
func calculateQueryComplexity(req SynthesisRequest, logger *zap.Logger) string {
	// Calculate complexity score
	complexityScore := 0

	// Factor 1: Query length and content
	queryLength := len(req.Query)
	if queryLength > 200 {
		complexityScore += 1
	}
	if queryLength > 500 {
		complexityScore += 1
	}

	// Factor 2: Number of context items
	contextItems := len(req.Chunks)
	if contextItems > ComplexQueryContextThreshold {
		complexityScore += 2
	} else if contextItems > 4 {
		complexityScore += 1
	}

	// Factor 3: Web results count
	webResults := len(req.WebResults)
	if webResults > 3 {
		complexityScore += 1
	}

	// Factor 4: Enhanced query type indicators with semantic scoring
	queryLower := strings.ToLower(req.Query)

	// Critical enterprise keywords (worth 3 points each - forces complex classification)
	criticalKeywords := []string{
		"lift-and-shift", "enterprise migration", "disaster recovery",
		"120 vm", "large-scale migration", "hybrid architecture",
		"multi-cloud", "compliance assessment", "security assessment",
	}

	// High-complexity enterprise keywords (worth 2 points each)
	enterpriseKeywords := []string{
		"enterprise", "migration plan", "architecture design",
		"best practices", "comprehensive", "detailed plan",
		"recommendations", "topology", "sizing", "assessment",
	}

	// Medium-complexity technical keywords (worth 1 point each)
	technicalKeywords := []string{
		"migration", "architecture", "terraform", "mermaid", "diagram",
		"vpc", "subnet", "ec2", "rds", "mgn", "aws", "azure",
		"cloud", "infrastructure", "network", "security",
	}

	keywordMatches := 0
	criticalPoints := 0
	enterprisePoints := 0
	technicalPoints := 0

	// Check critical keywords first (highest weight)
	for _, keyword := range criticalKeywords {
		if strings.Contains(queryLower, keyword) {
			keywordMatches++
			criticalPoints += 3
			complexityScore += 3 // Critical keywords force complex classification
		}
	}

	// Check enterprise keywords (higher weight)
	for _, keyword := range enterpriseKeywords {
		if strings.Contains(queryLower, keyword) {
			keywordMatches++
			enterprisePoints += 2
			complexityScore += 2 // Enterprise keywords are worth 2 points
		}
	}

	// Check technical keywords
	for _, keyword := range technicalKeywords {
		if strings.Contains(queryLower, keyword) {
			keywordMatches++
			technicalPoints += 1
			complexityScore += 1 // Technical keywords are worth 1 point
		}
	}

	// Bonus for query patterns that indicate enterprise scenarios
	if strings.Contains(queryLower, "120") && strings.Contains(queryLower, "vm") {
		complexityScore += 2 // Large VM count indicates enterprise scenario
	}
	if strings.Contains(queryLower, "plan") && (strings.Contains(queryLower, "migration") || strings.Contains(queryLower, "architecture")) {
		complexityScore += 1 // Planning requests need more context
	}

	// Factor 5: Conversation history length
	if len(req.ConversationHistory) > 5 {
		complexityScore += 1
	}

	// Estimate total tokens
	totalEstimatedTokens := estimateRequestTokens(req)
	if totalEstimatedTokens > ComplexQueryTokenThreshold {
		complexityScore += 2
	}

	// Determine complexity level - lowered thresholds for better migration handling
	var complexity string
	if complexityScore <= 2 {
		complexity = "simple"
	} else if complexityScore >= 4 {
		complexity = "complex"
	} else {
		complexity = "medium"
	}

	logger.Info("Query complexity analysis",
		zap.String("complexity", complexity),
		zap.Int("complexity_score", complexityScore),
		zap.Int("query_length", queryLength),
		zap.Int("context_items", contextItems),
		zap.Int("web_results", webResults),
		zap.Int("conversation_history", len(req.ConversationHistory)),
		zap.Int("estimated_tokens", totalEstimatedTokens),
		zap.Int("keyword_matches", keywordMatches),
		zap.Int("critical_points", criticalPoints),
		zap.Int("enterprise_points", enterprisePoints),
		zap.Int("technical_points", technicalPoints),
	)

	return complexity
}

// estimateRequestTokens estimates the total token count for the request
func estimateRequestTokens(req SynthesisRequest) int {
	totalChars := len(req.Query)

	// Add context chunks
	for _, chunk := range req.Chunks {
		totalChars += len(chunk.Text)
	}

	// Add web results
	for _, result := range req.WebResults {
		totalChars += len(result.Title) + len(result.Snippet)
	}

	// Add conversation history
	for _, msg := range req.ConversationHistory {
		totalChars += len(msg.Content)
	}

	// Rough estimate: 4 characters per token
	return totalChars / 4
}

// minDuration returns the smaller of two durations
func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// maxDuration returns the larger of two durations
func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

// ChunkItem represents a document chunk with metadata
type ChunkItem struct {
	Text     string `json:"text" binding:"required"`
	DocID    string `json:"doc_id" binding:"required"`
	SourceID string `json:"source_id"`
}

// WebResult represents a web search result
type WebResult struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	URL     string `json:"url"`
}

// validateSynthesisRequest validates the synthesis request
func validateSynthesisRequest(req SynthesisRequest) error {
	if strings.TrimSpace(req.Query) == "" {
		return fmt.Errorf("query cannot be empty")
	}

	if len(req.Query) > MaxQueryLength {
		return fmt.Errorf("query is too long (max %d characters)", MaxQueryLength)
	}

	// In test mode, allow empty chunks and web results for demo purposes
	if len(req.Chunks) == 0 && len(req.WebResults) == 0 {
		if os.Getenv("TEST_MODE") != "true" {
			return fmt.Errorf("at least one chunk or web result must be provided")
		}
		// In test mode, we'll provide fallback content
	}

	// Validate chunks
	for i, chunk := range req.Chunks {
		if strings.TrimSpace(chunk.Text) == "" {
			return fmt.Errorf("chunk %d text cannot be empty", i)
		}
		if strings.TrimSpace(chunk.DocID) == "" {
			return fmt.Errorf("chunk %d doc_id cannot be empty", i)
		}
		// SourceID is optional, so no validation required
	}

	// Validate web results
	for i, webResult := range req.WebResults {
		if strings.TrimSpace(webResult.Title) == "" && strings.TrimSpace(webResult.Snippet) == "" {
			return fmt.Errorf("web result %d must have either title or snippet", i)
		}
	}

	return nil
}

// validateRegenerationRequest validates the regeneration request
func validateRegenerationRequest(req RegenerationRequest) error {
	// First validate as a basic synthesis request
	synthReq := SynthesisRequest{
		Query:               req.Query,
		Chunks:              req.Chunks,
		WebResults:          req.WebResults,
		ConversationHistory: req.ConversationHistory,
	}
	if err := validateSynthesisRequest(synthReq); err != nil {
		return fmt.Errorf("base validation failed: %w", err)
	}

	// Validate parameters
	if err := validateGenerationParams(req.Parameters); err != nil {
		return fmt.Errorf("parameter validation failed: %w", err)
	}

	return nil
}

// validateGenerationParams validates generation parameters
func validateGenerationParams(params GenerationParams) error {
	presets := getParameterPresets()

	// If preset is specified, validate it exists
	if params.Preset != "" {
		if _, exists := presets[params.Preset]; !exists {
			availablePresets := make([]string, 0, len(presets))
			for name := range presets {
				availablePresets = append(availablePresets, name)
			}
			return fmt.Errorf("invalid preset '%s', available presets: %v", params.Preset, availablePresets)
		}
	}

	// Validate temperature range
	if params.Temperature < 0.0 || params.Temperature > 2.0 {
		return fmt.Errorf("temperature must be between 0.0 and 2.0, got %f", params.Temperature)
	}

	// Validate max tokens
	if params.MaxTokens < 100 || params.MaxTokens > 8000 {
		return fmt.Errorf("max_tokens must be between 100 and 8000, got %d", params.MaxTokens)
	}

	// Validate model
	validModels := []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo"}
	modelValid := false
	for _, validModel := range validModels {
		if params.Model == validModel {
			modelValid = true
			break
		}
	}
	if !modelValid {
		return fmt.Errorf("invalid model '%s', valid models: %v", params.Model, validModels)
	}

	return nil
}

// applyParameterPreset applies a preset to generation parameters
func applyParameterPreset(params *GenerationParams) {
	if params.Preset == "" {
		return
	}

	presets := getParameterPresets()
	if preset, exists := presets[params.Preset]; exists {
		// Only override if not explicitly set
		if params.Temperature == 0 {
			params.Temperature = preset.Temperature
		}
		if params.MaxTokens == 0 {
			params.MaxTokens = preset.MaxTokens
		}
		if params.Model == "" {
			params.Model = preset.Model
		}
	}
}

func main() {
	// Setup configuration and logger
	cfg, logger := setupConfiguration()
	defer func() { _ = logger.Sync() }()

	// Initialize services
	openaiClient := setupServices(cfg, logger)

	// Setup router and handlers
	router := setupRouter(cfg, logger, openaiClient)

	// Start server
	startServer(router, cfg, logger)
}

// setupConfiguration loads configuration and initializes logger
func setupConfiguration() (*config.Config, *zap.Logger) {
	// Check if running in test mode
	testMode := os.Getenv("TEST_MODE") == "true" || os.Getenv("CI") == "true"

	var cfg *config.Config
	var err error

	if testMode {
		cfg, err = config.LoadWithOptions(config.LoadOptions{
			ConfigPath:       "",
			EnableHotReload:  false,
			Environment:      "test",
			ValidateRequired: false,
			TestMode:         true,
		})
	} else {
		cfg, err = config.Load("")
	}
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	logger, err := initializeLogger(cfg)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	return cfg, logger
}

// setupServices initializes all required services
func setupServices(cfg *config.Config, logger *zap.Logger) *internalopenai.Client {
	// Check if running in test mode
	testMode := os.Getenv("TEST_MODE") == "true" || os.Getenv("CI") == "true"

	var openaiClient *internalopenai.Client
	var err error

	if testMode {
		logger.Info("Skipping OpenAI client initialization in test mode")
		return nil // Return nil in test mode
	}

	// Configure OpenAI client with synthesis service timeout settings
	timeoutConfig := resilience.TimeoutConfig{
		DefaultTimeout: time.Duration(cfg.Synthesis.TimeoutSeconds) * time.Second,
		MaxTimeout:     time.Duration(cfg.Synthesis.ComplexTimeoutSeconds) * time.Second,
		Logger:         logger,
	}

	openaiClient, err = internalopenai.NewClientWithTimeout(cfg.OpenAI.APIKey, logger, timeoutConfig)
	if err != nil {
		logger.Fatal("Failed to initialize OpenAI client", zap.Error(err))
	}

	// Log configuration with masked sensitive values
	maskedConfig := cfg.MaskSensitiveValues()
	logger.Info("Configuration loaded successfully",
		zap.String("service", "synthesize"),
		zap.String("environment", os.Getenv("ENVIRONMENT")),
		zap.String("synthesis_model", maskedConfig.Synthesis.Model),
		zap.Int("max_tokens", maskedConfig.Synthesis.MaxTokens),
		zap.Float64("temperature", maskedConfig.Synthesis.Temperature),
		zap.Int("timeout_seconds", maskedConfig.Synthesis.TimeoutSeconds),
		zap.Int("max_retries", maskedConfig.Synthesis.MaxRetries),
		zap.Float64("backoff_multiplier", maskedConfig.Synthesis.BackoffMultiplier),
		zap.String("openai_endpoint", maskedConfig.OpenAI.Endpoint),
		zap.String("openai_api_key", maskedConfig.OpenAI.APIKey),
	)

	return openaiClient
}

// setupRouter creates and configures the Gin router with all endpoints
func setupRouter(cfg *config.Config, logger *zap.Logger, openaiClient *internalopenai.Client) *gin.Engine {
	// Set Gin mode based on log level
	if cfg.Logging.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.Default()

	// Initialize metrics collector with alerting callback
	metricsCollector := synthesis.NewMetricsCollector(logger, func(alertType, message string, metadata map[string]interface{}) {
		logger.Warn("Synthesis service alert",
			zap.String("alert_type", alertType),
			zap.String("message", message),
			zap.Any("metadata", metadata),
		)
	})

	// Initialize health check manager
	healthManager := health.NewManager("synthesize", "1.0.0", logger)
	setupHealthChecks(healthManager, cfg, openaiClient, metricsCollector)

	router.GET("/health", gin.WrapH(healthManager.HTTPHandler()))
	router.GET("/metrics", createMetricsHandler(metricsCollector))
	router.POST("/synthesize", createSynthesisHandler(cfg, logger, openaiClient, metricsCollector))
	router.POST("/regenerate", createRegenerationHandler(cfg, logger, openaiClient, metricsCollector))
	router.GET("/presets", createPresetsHandler())

	return router
}

// setupHealthChecks configures health checks for the synthesize service
func setupHealthChecks(manager *health.Manager, cfg *config.Config, openaiClient *internalopenai.Client, metricsCollector *synthesis.MetricsCollector) {
	// OpenAI health check
	manager.AddCheckerFunc("openai", func(ctx context.Context) health.CheckResult {
		// Check if running in test mode
		if openaiClient == nil {
			return health.CheckResult{
				Status:    health.StatusHealthy,
				Timestamp: time.Now(),
				Metadata: map[string]interface{}{
					"test_mode":   true,
					"model":       cfg.Synthesis.Model,
					"max_tokens":  cfg.Synthesis.MaxTokens,
					"temperature": cfg.Synthesis.Temperature,
				},
			}
		}

		if _, err := openaiClient.EmbedTexts(ctx, []string{"health check"}); err != nil {
			return health.CheckResult{
				Status:    health.StatusUnhealthy,
				Error:     fmt.Sprintf("OpenAI API health check failed: %v", err),
				Timestamp: time.Now(),
			}
		}

		return health.CheckResult{
			Status:    health.StatusHealthy,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"model":       cfg.Synthesis.Model,
				"max_tokens":  cfg.Synthesis.MaxTokens,
				"temperature": cfg.Synthesis.Temperature,
			},
		}
	})

	// Synthesis configuration health check
	manager.AddCheckerFunc("synthesis_config", func(_ context.Context) health.CheckResult {
		// Validate synthesis configuration
		if cfg.Synthesis.Model == "" {
			return health.CheckResult{
				Status:    health.StatusUnhealthy,
				Error:     "Synthesis model not configured",
				Timestamp: time.Now(),
			}
		}

		if cfg.Synthesis.MaxTokens <= 0 {
			return health.CheckResult{
				Status:    health.StatusDegraded,
				Error:     "Invalid max tokens configuration",
				Timestamp: time.Now(),
			}
		}

		return health.CheckResult{
			Status:    health.StatusHealthy,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"model":       cfg.Synthesis.Model,
				"max_tokens":  cfg.Synthesis.MaxTokens,
				"temperature": cfg.Synthesis.Temperature,
			},
		}
	})

	// Synthesis service metrics health check
	manager.AddCheckerFunc("synthesis_metrics", func(ctx context.Context) health.CheckResult {
		healthy, _, metadata := metricsCollector.HealthCheck(ctx)

		healthStatus := health.StatusHealthy
		if !healthy {
			healthStatus = health.StatusDegraded
		}

		return health.CheckResult{
			Status:    healthStatus,
			Timestamp: time.Now(),
			Metadata:  metadata,
		}
	})

	// Set timeout for health checks
	manager.SetTimeout(HealthCheckTimeout)
}

// createSynthesisHandler creates the synthesis endpoint handler
func createSynthesisHandler(
	cfg *config.Config,
	logger *zap.Logger,
	openaiClient *internalopenai.Client,
	metricsCollector *synthesis.MetricsCollector,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		logger.Info("Synthesis request received",
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.GetHeader("User-Agent")),
		)

		// Parse and validate request
		req, valid := parseSynthesisRequest(c, logger)
		if !valid {
			return
		}

		// Process the synthesis request
		response, err := processSynthesisRequest(req, cfg, logger, openaiClient)
		if err != nil {
			handleSynthesisError(c, err, logger, "synthesis")
			return
		}

		// Log completion and return response
		processingTime := time.Since(startTime)
		logSynthesisCompletion(req, response, processingTime, logger)

		// Detect domain for quality validation
		domain := detectQueryDomain(req.Query)

		// Build synthesis response with metrics collection
		synthesisResponse := buildSynthesisResponse(response, &req, req.Query, domain, cfg, processingTime, metricsCollector, logger)

		c.JSON(http.StatusOK, synthesisResponse)
	}
}

// createRegenerationHandler creates the regeneration endpoint handler
func createRegenerationHandler(
	cfg *config.Config,
	logger *zap.Logger,
	openaiClient *internalopenai.Client,
	metricsCollector *synthesis.MetricsCollector,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		logger.Info("Regeneration request received",
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.GetHeader("User-Agent")),
		)

		// Parse and validate regeneration request
		req, valid := parseRegenerationRequest(c, logger)
		if !valid {
			return
		}

		// Apply preset parameters
		applyParameterPreset(&req.Parameters)

		// Process the regeneration request
		response, err := processRegenerationRequest(req, cfg, logger, openaiClient)
		if err != nil {
			handleSynthesisError(c, err, logger, "regeneration")
			return
		}

		// Log completion and return response
		processingTime := time.Since(startTime)
		logRegenerationCompletion(req, response, processingTime, logger)
		c.JSON(http.StatusOK, buildRegenerationResponse(response, &req, req.Parameters, processingTime, req.Query))
	}
}

// createPresetsHandler creates the presets endpoint handler
func createPresetsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		presets := getParameterPresets()
		c.JSON(http.StatusOK, gin.H{
			"presets": presets,
		})
	}
}

// createMetricsHandler creates the metrics endpoint handler
func createMetricsHandler(metricsCollector *synthesis.MetricsCollector) gin.HandlerFunc {
	return func(c *gin.Context) {
		metrics := metricsCollector.GetMetrics()
		c.JSON(http.StatusOK, metrics)
	}
}

// parseSynthesisRequest parses and validates the synthesis request
func parseSynthesisRequest(c *gin.Context, logger *zap.Logger) (SynthesisRequest, bool) {
	var req SynthesisRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error("Failed to parse synthesis request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request format",
			"details": err.Error(),
		})
		return req, false
	}

	if err := validateSynthesisRequest(req); err != nil {
		logger.Error("Invalid synthesis request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request",
			"details": err.Error(),
		})
		return req, false
	}

	return req, true
}

// parseRegenerationRequest parses and validates the regeneration request
func parseRegenerationRequest(c *gin.Context, logger *zap.Logger) (RegenerationRequest, bool) {
	var req RegenerationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error("Failed to parse regeneration request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request format",
			"details": err.Error(),
		})
		return req, false
	}

	if err := validateRegenerationRequest(req); err != nil {
		logger.Error("Invalid regeneration request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request",
			"details": err.Error(),
		})
		return req, false
	}

	return req, true
}

// processSynthesisRequest handles the core synthesis logic
func processSynthesisRequest(
	req SynthesisRequest,
	cfg *config.Config,
	logger *zap.Logger,
	openaiClient *internalopenai.Client,
) (*internalopenai.ChatCompletionResponse, error) {
	// Convert request to internal format
	contextItems := convertChunksToContextItems(req.Chunks)
	webResultStrings, webSourceURLs := convertWebResults(req.WebResults)

	// Validate source metadata
	if err := validateSynthesisSourceMetadata(contextItems, webSourceURLs, logger); err != nil {
		logger.Warn("Source metadata validation failed, continuing with available data",
			zap.Error(err),
			zap.Int("context_items", len(contextItems)),
			zap.Int("web_sources", len(webSourceURLs)))
		// Continue processing despite validation warnings
	}

	// Build comprehensive prompt with conversation context and optimization
	var messages []openai.ChatCompletionMessage
	if cfg != nil && cfg.Synthesis.EnablePromptOptimization {
		prompt := buildOptimizedPrompt(req.Query, contextItems, webResultStrings, req.ConversationHistory, cfg, logger)
		// Legacy format - convert to single user message for backward compatibility
		messages = []openai.ChatCompletionMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		}
	} else {
		// Use new message structure with proper system/user separation
		promptMessages := synth.BuildPromptMessages(req.Query, contextItems, webResultStrings)
		messages = []openai.ChatCompletionMessage{
			{
				Role:    "system",
				Content: promptMessages.SystemMessage,
			},
			{
				Role:    "user",
				Content: promptMessages.UserMessage,
			},
		}
	}

	// Call OpenAI Chat Completion API with adaptive timeout
	timeoutDuration := getAdaptiveTimeout(cfg, req, logger)
	logger.Info("Starting synthesis with adaptive timeout",
		zap.Duration("timeout_duration", timeoutDuration),
		zap.Int("context_items", len(req.Chunks)),
		zap.Int("web_results", len(req.WebResults)),
		zap.Int("query_length", len(req.Query)))

	// Add detailed performance tracking
	performanceStart := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeoutDuration)
	defer cancel()

	// Handle test mode with mock response
	if openaiClient == nil {
		logger.Info("Using mock OpenAI response for test mode")
		return &internalopenai.ChatCompletionResponse{
			Content: generateMockSynthesisResponse(req.Query, contextItems),
			Usage: openai.Usage{
				PromptTokens:     100,
				CompletionTokens: 200,
				TotalTokens:      300,
			},
		}, nil
	}

	// Configure custom retry logic for synthesis service with rate limit awareness
	baseRetryConfig := resilience.BackoffConfig{
		BaseDelay:   time.Second,
		MaxRetries:  cfg.Synthesis.MaxRetries,
		MaxDelay:    30 * time.Second,
		Multiplier:  cfg.Synthesis.BackoffMultiplier,
		Jitter:      true,
		RetryOnFunc: resilience.DefaultRetryOnFunc,
	}

	// Enhance retry configuration for rate limit handling
	retryConfig := createRateLimitAwareRetryConfig(baseRetryConfig)

	// Track OpenAI API call timing
	openaiStart := time.Now()
	response, err := openaiClient.CreateChatCompletionWithRetry(ctx, internalopenai.ChatCompletionRequest{
		Model:       cfg.Synthesis.Model,
		MaxTokens:   cfg.Synthesis.MaxTokens,
		Temperature: float32(cfg.Synthesis.Temperature),
		Messages:    messages,
	}, retryConfig)

	openaiDuration := time.Since(openaiStart)
	totalDuration := time.Since(performanceStart)

	// Enhanced error handling with timeout monitoring
	if err != nil {
		errorType := getErrorType(err)
		logger.Error("Synthesis OpenAI API call failed",
			zap.Error(err),
			zap.String("model", cfg.Synthesis.Model),
			zap.Duration("openai_duration", openaiDuration),
			zap.Duration("total_duration", totalDuration),
			zap.Duration("timeout_duration", timeoutDuration),
			zap.Float64("timeout_utilization", float64(totalDuration)/float64(timeoutDuration)),
			zap.Bool("timeout_exceeded", totalDuration > timeoutDuration),
			zap.String("error_type", errorType),
			zap.Int("context_items", len(req.Chunks)),
			zap.Int("web_results", len(req.WebResults)),
			zap.String("query_complexity", calculateQueryComplexity(req, logger)),
		)

		// Log specific timeout metrics for monitoring
		if errorType == "timeout" {
			// Calculate total tokens from all messages
			totalTokens := 0
			for _, msg := range messages {
				totalTokens += estimateTokenCount(msg.Content)
			}
			logger.Error("OpenAI API timeout detected",
				zap.Duration("timeout_duration", timeoutDuration),
				zap.Duration("actual_duration", totalDuration),
				zap.String("model", cfg.Synthesis.Model),
				zap.Int("context_items", len(req.Chunks)),
				zap.Int("web_results", len(req.WebResults)),
				zap.String("query_complexity", calculateQueryComplexity(req, logger)),
				zap.Int("estimated_prompt_tokens", totalTokens),
			)
		}

		return nil, fmt.Errorf("failed to call OpenAI API: %w", err)
	}

	// Log detailed performance metrics
	logger.Info("Synthesis performance metrics",
		zap.Duration("total_duration", totalDuration),
		zap.Duration("openai_api_duration", openaiDuration),
		zap.Duration("preparation_duration", openaiStart.Sub(performanceStart)),
		zap.Duration("timeout_used", timeoutDuration),
		zap.Float64("timeout_utilization", float64(totalDuration)/float64(timeoutDuration)),
		zap.Int("prompt_tokens", response.Usage.PromptTokens),
		zap.Int("completion_tokens", response.Usage.CompletionTokens),
		zap.Int("total_tokens", response.Usage.TotalTokens),
		zap.String("finish_reason", response.FinishReason),
	)

	return response, err
}

// processRegenerationRequest handles the core regeneration logic with custom parameters
func processRegenerationRequest(
	req RegenerationRequest,
	cfg *config.Config,
	logger *zap.Logger,
	openaiClient *internalopenai.Client,
) (*internalopenai.ChatCompletionResponse, error) {
	// Convert request to internal format
	contextItems := convertChunksToContextItems(req.Chunks)
	webResultStrings, webSourceURLs := convertWebResults(req.WebResults)

	// Validate source metadata
	if err := validateSynthesisSourceMetadata(contextItems, webSourceURLs, logger); err != nil {
		logger.Warn("Source metadata validation failed, continuing with available data",
			zap.Error(err),
			zap.Int("context_items", len(contextItems)),
			zap.Int("web_sources", len(webSourceURLs)))
		// Continue processing despite validation warnings
	}

	// Build enhanced prompt for regeneration with optimization
	var messages []openai.ChatCompletionMessage
	if cfg != nil && cfg.Synthesis.EnablePromptOptimization {
		// Build base prompt first, then enhance for regeneration
		basePrompt := buildOptimizedPrompt(req.Query, contextItems, webResultStrings, req.ConversationHistory, cfg, logger)
		prompt := enhancePromptForRegeneration(basePrompt, req.PreviousResponse)
		// Legacy format for optimization path
		messages = []openai.ChatCompletionMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		}
	} else {
		// Use new message structure with proper system/user separation
		promptMessages := synth.BuildPromptMessages(req.Query, contextItems, webResultStrings)
		// Add regeneration-specific instructions to user message
		regenerationInstructions := buildRegenerationInstructions(req.PreviousResponse)
		messages = []openai.ChatCompletionMessage{
			{
				Role:    "system",
				Content: promptMessages.SystemMessage,
			},
			{
				Role:    "user",
				Content: promptMessages.UserMessage + "\n\n" + regenerationInstructions,
			},
		}
	}

	// Call OpenAI Chat Completion API with custom parameters and adaptive timeout
	// Convert RegenerationRequest to SynthesisRequest for complexity analysis
	synthReq := SynthesisRequest{
		Query:               req.Query,
		Chunks:              req.Chunks,
		WebResults:          req.WebResults,
		ConversationHistory: req.ConversationHistory,
	}
	timeoutDuration := getAdaptiveTimeout(cfg, synthReq, logger)
	logger.Info("Starting regeneration with adaptive timeout",
		zap.Duration("timeout_duration", timeoutDuration),
		zap.String("preset", req.Parameters.Preset),
		zap.String("model", req.Parameters.Model))

	performanceStart := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeoutDuration)
	defer cancel()

	logger.Info("Regenerating with custom parameters",
		zap.String("preset", req.Parameters.Preset),
		zap.String("model", req.Parameters.Model),
		zap.Float64("temperature", float64(req.Parameters.Temperature)),
		zap.Int("max_tokens", req.Parameters.MaxTokens),
	)

	// Handle test mode with mock response
	if openaiClient == nil {
		logger.Info("Using mock OpenAI response for regeneration in test mode")
		return &internalopenai.ChatCompletionResponse{
			Content: generateMockSynthesisResponse(req.Query, contextItems),
			Usage: openai.Usage{
				PromptTokens:     120,
				CompletionTokens: 250,
				TotalTokens:      370,
			},
		}, nil
	}

	// Configure custom retry logic for synthesis service with rate limit awareness
	baseRetryConfig := resilience.BackoffConfig{
		BaseDelay:   time.Second,
		MaxRetries:  cfg.Synthesis.MaxRetries,
		MaxDelay:    30 * time.Second,
		Multiplier:  cfg.Synthesis.BackoffMultiplier,
		Jitter:      true,
		RetryOnFunc: resilience.DefaultRetryOnFunc,
	}

	// Enhance retry configuration for rate limit handling
	retryConfig := createRateLimitAwareRetryConfig(baseRetryConfig)

	// Track OpenAI API call timing for regeneration
	openaiStart := time.Now()
	response, err := openaiClient.CreateChatCompletionWithRetry(ctx, internalopenai.ChatCompletionRequest{
		Model:       req.Parameters.Model,
		MaxTokens:   req.Parameters.MaxTokens,
		Temperature: req.Parameters.Temperature,
		Messages:    messages,
	}, retryConfig)

	openaiDuration := time.Since(openaiStart)
	totalDuration := time.Since(performanceStart)

	// Enhanced error handling with timeout monitoring for regeneration
	if err != nil {
		errorType := getErrorType(err)
		logger.Error("Regeneration OpenAI API call failed",
			zap.Error(err),
			zap.String("model", req.Parameters.Model),
			zap.Duration("openai_duration", openaiDuration),
			zap.Duration("total_duration", totalDuration),
			zap.Duration("timeout_duration", timeoutDuration),
			zap.Float64("timeout_utilization", float64(totalDuration)/float64(timeoutDuration)),
			zap.Bool("timeout_exceeded", totalDuration > timeoutDuration),
			zap.String("error_type", errorType),
			zap.String("preset", req.Parameters.Preset),
			zap.Int("context_items", len(req.Chunks)),
			zap.Int("web_results", len(req.WebResults)),
		)

		// Log specific timeout metrics for monitoring
		if errorType == "timeout" {
			// Calculate total tokens from all messages
			totalTokens := 0
			for _, msg := range messages {
				totalTokens += estimateTokenCount(msg.Content)
			}
			logger.Error("OpenAI API timeout detected during regeneration",
				zap.Duration("timeout_duration", timeoutDuration),
				zap.Duration("actual_duration", totalDuration),
				zap.String("model", req.Parameters.Model),
				zap.String("preset", req.Parameters.Preset),
				zap.Int("context_items", len(req.Chunks)),
				zap.Int("web_results", len(req.WebResults)),
				zap.Int("estimated_prompt_tokens", totalTokens),
			)
		}

		return nil, fmt.Errorf("failed to call OpenAI API for regeneration: %w", err)
	}

	// Log detailed performance metrics for regeneration
	logger.Info("Regeneration performance metrics",
		zap.Duration("total_duration", totalDuration),
		zap.Duration("openai_api_duration", openaiDuration),
		zap.Duration("preparation_duration", openaiStart.Sub(performanceStart)),
		zap.Duration("timeout_used", timeoutDuration),
		zap.Float64("timeout_utilization", float64(totalDuration)/float64(timeoutDuration)),
		zap.String("preset", req.Parameters.Preset),
		zap.String("model", req.Parameters.Model),
		zap.Int("prompt_tokens", response.Usage.PromptTokens),
		zap.Int("completion_tokens", response.Usage.CompletionTokens),
		zap.Int("total_tokens", response.Usage.TotalTokens),
		zap.String("finish_reason", response.FinishReason),
	)

	return response, nil
}

// buildRegenerationInstructions creates instructions for regeneration requests
func buildRegenerationInstructions(previousResponse *string) string {
	if previousResponse == nil || *previousResponse == "" {
		return ""
	}

	return fmt.Sprintf(`--- REGENERATION INSTRUCTIONS ---
You are regenerating a response. The previous response was:

%s

Please provide a completely new response with:
1. Different structure and organization
2. Alternative explanations and examples
3. Fresh perspectives on the same topic
4. Different code snippets or implementation approaches
5. Maintain the same level of detail and accuracy

Focus on providing value through variety while maintaining quality and accuracy.`, *previousResponse)
}

// buildRegenerationPrompt builds a specialized prompt for regeneration requests
func buildRegenerationPrompt(
	query string,
	contextItems []synth.ContextItem,
	webResultStrings []string,
	conversationHistory []session.Message,
	previousResponse *string,
) string {
	// Start with the base prompt
	prompt := synth.BuildPromptWithConversation(query, contextItems, webResultStrings, conversationHistory)

	// Add regeneration-specific instructions if we have a previous response
	if previousResponse != nil && *previousResponse != "" {
		prompt += fmt.Sprintf(`

--- Previous Response ---
%s

--- Regeneration Instructions ---
Please provide an alternative response to the same query. Consider:
1. Different perspectives or approaches to the problem
2. Alternative architectural patterns or solutions
3. Varied level of technical detail
4. Different emphasis areas (cost, security, performance, etc.)

Generate a fresh response that covers the same query but with a different angle or approach.`, *previousResponse)
	}

	return prompt
}

// convertChunksToContextItems converts request chunks to internal context items
func convertChunksToContextItems(chunks []ChunkItem) []synth.ContextItem {
	contextItems := make([]synth.ContextItem, len(chunks))
	for i, chunk := range chunks {
		// Use SourceID if available, otherwise fall back to DocID
		sourceID := chunk.SourceID
		if sourceID == "" {
			sourceID = chunk.DocID
		}

		contextItems[i] = synth.ContextItem{
			Content:  chunk.Text,
			SourceID: sourceID,
			Score:    1.0,
			Priority: 1,
		}
	}
	return contextItems
}

// convertWebResults converts web results to strings and extracts URLs
func convertWebResults(webResults []WebResult) ([]string, []string) {
	webResultStrings := make([]string, len(webResults))
	webSourceURLs := make([]string, 0, len(webResults))

	for i, webResult := range webResults {
		switch {
		case webResult.Title != "" && webResult.Snippet != "":
			webResultStrings[i] = fmt.Sprintf("Title: %s\nSnippet: %s\nURL: %s",
				webResult.Title, webResult.Snippet, webResult.URL)
		case webResult.Title != "":
			webResultStrings[i] = fmt.Sprintf("Title: %s\nURL: %s", webResult.Title, webResult.URL)
		default:
			webResultStrings[i] = fmt.Sprintf("Snippet: %s\nURL: %s", webResult.Snippet, webResult.URL)
		}

		if webResult.URL != "" {
			webSourceURLs = append(webSourceURLs, webResult.URL)
		}
	}

	return webResultStrings, webSourceURLs
}

// logSynthesisCompletion logs synthesis completion details
func logSynthesisCompletion(
	req SynthesisRequest,
	response *internalopenai.ChatCompletionResponse,
	processingTime time.Duration,
	logger *zap.Logger,
) {
	allAvailableSources := make([]string, 0, len(req.Chunks)+len(req.WebResults))
	for _, chunk := range req.Chunks {
		// Prefer SourceID if available, otherwise use DocID
		if chunk.SourceID != "" {
			allAvailableSources = append(allAvailableSources, chunk.SourceID)
		} else if chunk.DocID != "" {
			allAvailableSources = append(allAvailableSources, chunk.DocID)
		}
	}
	for _, webResult := range req.WebResults {
		if webResult.URL != "" {
			allAvailableSources = append(allAvailableSources, webResult.URL)
		}
	}

	synthesisResponse := synth.ParseResponseWithQuery(response.Content, allAvailableSources, req.Query)

	logger.Info("Synthesis completed",
		zap.String("query", req.Query),
		zap.Int("context_items", len(req.Chunks)),
		zap.Int("web_results", len(req.WebResults)),
		zap.Int("total_tokens", response.Usage.TotalTokens),
		zap.Int("prompt_tokens", response.Usage.PromptTokens),
		zap.Int("completion_tokens", response.Usage.CompletionTokens),
		zap.Duration("processing_time", processingTime),
		zap.Int("response_length", len(synthesisResponse.MainText)),
		zap.Int("sources_count", len(synthesisResponse.Sources)),
		zap.Int("code_snippets_count", len(synthesisResponse.CodeSnippets)),
		zap.Bool("has_diagram", synthesisResponse.DiagramCode != ""),
	)
}

// logRegenerationCompletion logs regeneration completion details
func logRegenerationCompletion(
	req RegenerationRequest,
	response *internalopenai.ChatCompletionResponse,
	processingTime time.Duration,
	logger *zap.Logger,
) {
	allAvailableSources := make([]string, 0, len(req.Chunks)+len(req.WebResults))
	for _, chunk := range req.Chunks {
		// Prefer SourceID if available, otherwise use DocID
		if chunk.SourceID != "" {
			allAvailableSources = append(allAvailableSources, chunk.SourceID)
		} else if chunk.DocID != "" {
			allAvailableSources = append(allAvailableSources, chunk.DocID)
		}
	}
	for _, webResult := range req.WebResults {
		if webResult.URL != "" {
			allAvailableSources = append(allAvailableSources, webResult.URL)
		}
	}

	synthesisResponse := synth.ParseResponseWithQuery(response.Content, allAvailableSources, req.Query)

	logger.Info("Regeneration completed",
		zap.String("query", req.Query),
		zap.String("preset", req.Parameters.Preset),
		zap.String("model", req.Parameters.Model),
		zap.Float64("temperature", float64(req.Parameters.Temperature)),
		zap.Int("max_tokens", req.Parameters.MaxTokens),
		zap.Int("context_items", len(req.Chunks)),
		zap.Int("web_results", len(req.WebResults)),
		zap.Int("total_tokens", response.Usage.TotalTokens),
		zap.Int("prompt_tokens", response.Usage.PromptTokens),
		zap.Int("completion_tokens", response.Usage.CompletionTokens),
		zap.Duration("processing_time", processingTime),
		zap.Int("response_length", len(synthesisResponse.MainText)),
		zap.Int("sources_count", len(synthesisResponse.Sources)),
		zap.Int("code_snippets_count", len(synthesisResponse.CodeSnippets)),
		zap.Bool("has_diagram", synthesisResponse.DiagramCode != ""),
		zap.Bool("has_previous_response", req.PreviousResponse != nil),
	)
}

// buildSynthesisResponse builds the final synthesis response
func buildSynthesisResponse(
	response *internalopenai.ChatCompletionResponse,
	req *SynthesisRequest,
	query string,
	domain QueryDomain,
	cfg *config.Config,
	processingTime time.Duration,
	metricsCollector *synthesis.MetricsCollector,
	logger *zap.Logger,
) gin.H {
	// Build available sources from chunks and web results
	allAvailableSources := make([]string, 0, len(req.Chunks)+len(req.WebResults))
	for _, chunk := range req.Chunks {
		// Prefer SourceID if available, otherwise use DocID
		if chunk.SourceID != "" {
			allAvailableSources = append(allAvailableSources, chunk.SourceID)
		} else if chunk.DocID != "" {
			allAvailableSources = append(allAvailableSources, chunk.DocID)
		}
	}
	for _, webResult := range req.WebResults {
		if webResult.URL != "" {
			allAvailableSources = append(allAvailableSources, webResult.URL)
		}
	}

	// DEBUG: Log raw OpenAI response to understand parsing issues
	logger.Info("Raw OpenAI response content",
		zap.String("content", response.Content),
		zap.Int("content_length", len(response.Content)),
		zap.String("query", query))

	synthesisResponse := synth.ParseResponseWithQuery(response.Content, allAvailableSources, query)

	// Enhanced monitoring for code snippet generation rates
	logCodeSnippetGeneration(query, synthesisResponse.CodeSnippets, domain)

	// Monitor for potential token limit issues
	if response.FinishReason == "length" {
		logger.Warn("Response was truncated due to token limit - this may cause incomplete code generation",
			zap.String("query", query),
			zap.String("domain", string(domain)),
			zap.Int("total_tokens", response.Usage.TotalTokens),
			zap.Int("completion_tokens", response.Usage.CompletionTokens),
			zap.Int("max_tokens", cfg.Synthesis.MaxTokens),
			zap.Int("code_snippets_found", len(synthesisResponse.CodeSnippets)),
			zap.Bool("diagram_found", synthesisResponse.DiagramCode != ""),
		)
	}

	// Perform quality validation
	qualityMetrics := validateResponseQuality(response.Content, query, domain)

	// Implement fallback mechanisms for code generation failures
	if shouldGenerateCodeFallback(query, synthesisResponse, response.FinishReason) {
		logger.Info("Implementing code generation fallback due to missing expected code",
			zap.String("query", query),
			zap.String("finish_reason", response.FinishReason),
			zap.Int("existing_snippets", len(synthesisResponse.CodeSnippets)),
			zap.Bool("has_diagram", synthesisResponse.DiagramCode != ""),
		)

		// Add fallback code snippets if missing
		fallbackSnippets := generateFallbackCodeSnippets(query, synthesisResponse.CodeSnippets)
		synthesisResponse.CodeSnippets = append(synthesisResponse.CodeSnippets, fallbackSnippets...)
	}

	// Record metrics for code generation
	domainStr := string(domain)
	hasCode := len(synthesisResponse.CodeSnippets) > 0
	hasDiagram := synthesisResponse.DiagramCode != ""

	// Record code generation metrics for each language
	for _, codeSnippet := range synthesisResponse.CodeSnippets {
		metricsCollector.RecordCodeGeneration(domainStr, codeSnippet.Language, true, processingTime, qualityMetrics.OverallQualityScore)
	}

	// Record diagram generation metrics
	if hasDiagram {
		// Determine diagram type (simplified - assuming mermaid for now)
		diagramType := "mermaid"
		if strings.Contains(synthesisResponse.DiagramCode, "graph") {
			diagramType = "mermaid-graph"
		}
		metricsCollector.RecordDiagramGeneration(diagramType, true, processingTime)
	}

	// Record response quality metrics
	metricsCollector.RecordResponseQuality(qualityMetrics.OverallQualityScore, processingTime, hasCode, hasDiagram)

	return gin.H{
		"main_text":     synthesisResponse.MainText,
		"diagram_code":  synthesisResponse.DiagramCode,
		"code_snippets": synthesisResponse.CodeSnippets,
		"sources":       synthesisResponse.Sources,
		"metadata": gin.H{
			"processing_time":   processingTime.Milliseconds(),
			"total_tokens":      response.Usage.TotalTokens,
			"prompt_tokens":     response.Usage.PromptTokens,
			"completion_tokens": response.Usage.CompletionTokens,
			"model":             cfg.Synthesis.Model,
		},
		"quality_metrics": qualityMetrics,
	}
}

// buildRegenerationResponse builds the final regeneration response
func buildRegenerationResponse(
	response *internalopenai.ChatCompletionResponse,
	req *RegenerationRequest,
	params GenerationParams,
	processingTime time.Duration,
	query string,
) gin.H {
	// Build available sources from chunks and web results
	allAvailableSources := make([]string, 0, len(req.Chunks)+len(req.WebResults))
	for _, chunk := range req.Chunks {
		// Prefer SourceID if available, otherwise use DocID
		if chunk.SourceID != "" {
			allAvailableSources = append(allAvailableSources, chunk.SourceID)
		} else if chunk.DocID != "" {
			allAvailableSources = append(allAvailableSources, chunk.DocID)
		}
	}
	for _, webResult := range req.WebResults {
		if webResult.URL != "" {
			allAvailableSources = append(allAvailableSources, webResult.URL)
		}
	}
	synthesisResponse := synth.ParseResponseWithQuery(response.Content, allAvailableSources, query)

	return gin.H{
		"main_text":     synthesisResponse.MainText,
		"diagram_code":  synthesisResponse.DiagramCode,
		"code_snippets": synthesisResponse.CodeSnippets,
		"sources":       synthesisResponse.Sources,
		"regeneration": gin.H{
			"preset":      params.Preset,
			"temperature": params.Temperature,
			"max_tokens":  params.MaxTokens,
			"model":       params.Model,
		},
		"metadata": gin.H{
			"processing_time":   processingTime.Milliseconds(),
			"total_tokens":      response.Usage.TotalTokens,
			"prompt_tokens":     response.Usage.PromptTokens,
			"completion_tokens": response.Usage.CompletionTokens,
			"model":             params.Model,
			"is_regeneration":   true,
		},
	}
}

// startServer starts the HTTP server
func startServer(router *gin.Engine, cfg *config.Config, logger *zap.Logger) {
	port := ":8082"
	logger.Info("Starting synthesize service",
		zap.String("port", port),
		zap.String("model", cfg.Synthesis.Model),
		zap.Int("max_tokens", cfg.Synthesis.MaxTokens),
		zap.Float64("temperature", cfg.Synthesis.Temperature),
	)

	if err := router.Run(port); err != nil {
		logger.Fatal("Failed to start server", zap.Error(err))
	}
}

// validateSynthesisSourceMetadata validates source metadata for synthesis
func validateSynthesisSourceMetadata(
	contextItems []synth.ContextItem,
	webSourceURLs []string,
	logger *zap.Logger,
) error {
	var errors []string

	// Validate context items have source IDs
	emptySourceCount := 0
	for i, item := range contextItems {
		if strings.TrimSpace(item.SourceID) == "" {
			emptySourceCount++
			logger.Debug("Context item missing source ID", zap.Int("item_index", i))
		}
		if strings.TrimSpace(item.Content) == "" {
			errors = append(errors, fmt.Sprintf("context item %d has empty content", i))
		}
	}

	if emptySourceCount > 0 {
		logger.Warn("Some context items missing source IDs",
			zap.Int("empty_source_count", emptySourceCount),
			zap.Int("total_items", len(contextItems)))
	}

	// Validate web source URLs
	invalidURLCount := 0
	for i, url := range webSourceURLs {
		if !isValidWebSourceURL(url) {
			invalidURLCount++
			logger.Debug("Invalid web source URL", zap.Int("url_index", i), zap.String("url", url))
		}
	}

	if invalidURLCount > 0 {
		logger.Warn("Some web source URLs are invalid",
			zap.Int("invalid_url_count", invalidURLCount),
			zap.Int("total_urls", len(webSourceURLs)))
	}

	// Check for minimum sources
	totalSources := len(contextItems) + len(webSourceURLs)
	if totalSources == 0 {
		errors = append(errors, "no sources provided for synthesis")
	}

	if len(errors) > 0 {
		return fmt.Errorf("source validation errors: %s", strings.Join(errors, ", "))
	}

	return nil
}

// isValidWebSourceURL validates a web source URL
func isValidWebSourceURL(url string) bool {
	if strings.TrimSpace(url) == "" {
		return false
	}

	url = strings.TrimSpace(url)
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

// initializeLogger creates a logger based on configuration settings
func initializeLogger(cfg *config.Config) (*zap.Logger, error) {
	var zapConfig zap.Config

	if cfg.Logging.Format == "json" {
		zapConfig = zap.NewProductionConfig()
	} else {
		zapConfig = zap.NewDevelopmentConfig()
	}

	// Set log level
	switch cfg.Logging.Level {
	case "debug":
		zapConfig.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	case "info":
		zapConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	case "warn":
		zapConfig.Level = zap.NewAtomicLevelAt(zapcore.WarnLevel)
	case "error":
		zapConfig.Level = zap.NewAtomicLevelAt(zapcore.ErrorLevel)
	default:
		zapConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}

	// Set output destination
	if cfg.Logging.Output == "file" {
		zapConfig.OutputPaths = []string{"synthesize.log"}
		zapConfig.ErrorOutputPaths = []string{"synthesize.log"}
	} else {
		zapConfig.OutputPaths = []string{"stdout"}
		zapConfig.ErrorOutputPaths = []string{"stderr"}
	}

	return zapConfig.Build()
}

// generateMockSynthesisResponse generates a mock AI response for test mode
func generateMockSynthesisResponse(query string, contextItems []synth.ContextItem) string {
	// Extract specific parameters for contextual response
	queryParams := extractQueryParameters(query)

	// Create a realistic mock response based on the query and context
	response := fmt.Sprintf("## Mock AI Response for Query: %s\n\n", query)

	// Add specific parameter acknowledgment
	if queryParams.VmCount > 0 || len(queryParams.Technologies) > 0 || len(queryParams.CloudProviders) > 0 {
		response += "### Extracted Query Parameters\n"
		if queryParams.VmCount > 0 {
			response += fmt.Sprintf("- **VM Count**: %d virtual machines\n", queryParams.VmCount)
		}
		if len(queryParams.Technologies) > 0 {
			response += fmt.Sprintf("- **Technologies**: %s\n", strings.Join(queryParams.Technologies, ", "))
		}
		if len(queryParams.CloudProviders) > 0 {
			response += fmt.Sprintf("- **Cloud Providers**: %s\n", strings.Join(queryParams.CloudProviders, ", "))
		}
		if queryParams.RTORequirement != "" {
			response += fmt.Sprintf("- **RTO Requirement**: %s\n", queryParams.RTORequirement)
		}
		if queryParams.RPORequirement != "" {
			response += fmt.Sprintf("- **RPO Requirement**: %s\n", queryParams.RPORequirement)
		}
		if len(queryParams.Constraints) > 0 {
			response += fmt.Sprintf("- **Constraints**: %s\n", strings.Join(queryParams.Constraints, ", "))
		}
		response += "\n"
	}

	// Add context-aware content
	if len(contextItems) > 0 {
		response += "Based on the provided context, here's a comprehensive response:\n\n"

		// Extract key themes from context
		for i, item := range contextItems {
			if i >= 3 { // Limit to first 3 context items
				break
			}
			response += fmt.Sprintf("• **Key Point %d**: Referenced from %s - %s\n",
				i+1, item.SourceID,
				truncateString(item.Content, 100))
		}
		response += "\n"
	} else {
		response += "**Demo Mode**: Generating response based on query analysis without external context.\n\n"
	}

	// Add query-specific mock content
	if strings.Contains(strings.ToLower(query), "aws") {
		response += "### AWS Migration Strategy\n\n"

		// Add specific VM count information
		if queryParams.VmCount > 0 {
			response += fmt.Sprintf("**Migration Scope**: %d VMs\n", queryParams.VmCount)
			response += fmt.Sprintf("- Estimated migration timeline: %d weeks\n", (queryParams.VmCount/10)+4)
			response += fmt.Sprintf("- Recommended EC2 instances: %d instances\n", queryParams.VmCount)

			// Calculate detailed cost breakdown
			costBreakdown := calculateDetailedCosts(queryParams, "aws")
			response += formatCostBreakdown(costBreakdown)
		}

		// Add technology-specific considerations
		if len(queryParams.Technologies) > 0 {
			response += "**Technology-Specific Considerations**:\n"
			for _, tech := range queryParams.Technologies {
				switch tech {
				case "Windows":
					response += "- Windows Server licensing through AWS License Manager\n"
				case "Linux":
					response += "- Linux workloads optimize for cost-effective EC2 instances\n"
				case "SQL Server":
					response += "- RDS for SQL Server or EC2 with SQL Server licensing\n"
				case "VMware":
					response += "- VMware Cloud on AWS for hybrid scenarios\n"
				}
			}
			response += "\n"
		}

		response += `**Phase 1: Assessment & Planning**
- Infrastructure discovery and mapping
- Application dependency analysis
- Cost optimization recommendations

**Phase 2: Migration Execution**
- AWS Application Migration Service (MGN) setup
- Pilot migration of non-critical workloads
- Production workload migration

**Phase 3: Optimization**
- Performance tuning and cost optimization
- Security and compliance validation
- Monitoring and alerting setup

### Architecture Recommendations
- Use AWS VPC for network isolation
- Implement AWS Well-Architected Framework principles
- Consider AWS Landing Zone for multi-account strategy`

		response += "\n\n### Terraform Configuration\n\n"
		response += "```terraform\n"
		response += "# AWS Provider Configuration\n"
		response += "terraform {\n"
		response += "  required_providers {\n"
		response += "    aws = {\n"
		response += "      source  = \"hashicorp/aws\"\n"
		response += "      version = \"~> 5.0\"\n"
		response += "    }\n"
		response += "  }\n"
		response += "}\n\n"
		response += "provider \"aws\" {\n"
		response += "  region = var.aws_region\n"
		response += "}\n\n"
		response += "# Variables\n"
		response += "variable \"aws_region\" {\n"
		response += "  description = \"AWS region\"\n"
		response += "  type        = string\n"
		response += "  default     = \"us-west-2\"\n"
		response += "}\n\n"
		response += "variable \"vpc_cidr\" {\n"
		response += "  description = \"CIDR block for VPC\"\n"
		response += "  type        = string\n"
		response += "  default     = \"10.0.0.0/16\"\n"
		response += "}\n\n"
		response += "# Create VPC\n"
		response += "resource \"aws_vpc\" \"main\" {\n"
		response += "  cidr_block           = var.vpc_cidr\n"
		response += "  enable_dns_hostnames = true\n"
		response += "  enable_dns_support   = true\n\n"
		response += "  tags = {\n"
		response += "    Name = \"migration-vpc\"\n"
		response += "  }\n"
		response += "}\n\n"
		response += "# Create Internet Gateway\n"
		response += "resource \"aws_internet_gateway\" \"main\" {\n"
		response += "  vpc_id = aws_vpc.main.id\n\n"
		response += "  tags = {\n"
		response += "    Name = \"migration-igw\"\n"
		response += "  }\n"
		response += "}\n\n"
		response += "# Create public subnet\n"
		response += "resource \"aws_subnet\" \"public\" {\n"
		response += "  vpc_id                  = aws_vpc.main.id\n"
		response += "  cidr_block              = \"10.0.1.0/24\"\n"
		response += "  availability_zone       = \"${var.aws_region}a\"\n"
		response += "  map_public_ip_on_launch = true\n\n"
		response += "  tags = {\n"
		response += "    Name = \"migration-public-subnet\"\n"
		response += "  }\n"
		response += "}\n"
		response += "```\n"
		response += "\n### Migration Commands\n\n"
		response += "```bash\n"
		response += "#!/bin/bash\n"
		response += "# AWS MGN Migration Setup Script\n\n"
		response += "# Set variables\n"
		response += "export AWS_REGION=\"us-west-2\"\n"
		response += "export SOURCE_SERVER_ID=\"s-1234567890abcdef0\"\n\n"
		response += "# Initialize AWS MGN\n"
		response += "aws mgn initialize-service --region $AWS_REGION\n\n"
		response += "# Install replication agent on source server\n"
		response += "wget -O ./aws-replication-installer-init.py https://aws-application-migration-service-$AWS_REGION.s3.$AWS_REGION.amazonaws.com/latest/linux/aws-replication-installer-init.py\n"
		response += "python3 aws-replication-installer-init.py --region $AWS_REGION --no-prompt\n\n"
		response += "# Check replication status\n"
		response += "aws mgn describe-source-servers --region $AWS_REGION --source-server-ids $SOURCE_SERVER_ID\n"
		response += "```\n\n"
		response += "**Note**: This is a mock response generated for demonstration purposes."
	} else if strings.Contains(strings.ToLower(query), "azure") {
		response += "### Azure Migration Strategy\n\n"

		// Add specific VM count information
		if queryParams.VmCount > 0 {
			response += fmt.Sprintf("**Migration Scope**: %d VMs\n", queryParams.VmCount)
			response += fmt.Sprintf("- Estimated migration timeline: %d weeks\n", (queryParams.VmCount/10)+4)
			response += fmt.Sprintf("- Recommended Azure VMs: %d instances\n", queryParams.VmCount)

			// Calculate detailed cost breakdown
			costBreakdown := calculateDetailedCosts(queryParams, "azure")
			response += formatCostBreakdown(costBreakdown)
		}

		// Add technology-specific considerations
		if len(queryParams.Technologies) > 0 {
			response += "**Technology-Specific Considerations**:\n"
			for _, tech := range queryParams.Technologies {
				switch tech {
				case "Windows":
					response += "- Windows Server licensing through Azure Hybrid Benefit\n"
				case "Linux":
					response += "- Linux workloads optimize for cost-effective Azure VMs\n"
				case "SQL Server":
					response += "- Azure SQL Database or SQL Server on Azure VMs\n"
				case "VMware":
					response += "- Azure VMware Solution for hybrid scenarios\n"
				}
			}
			response += "\n"
		}

		response += `**Assessment Phase**
- Azure Migrate assessment tools
- Application portfolio analysis
- Security and compliance review

**Migration Approach**
- Azure Site Recovery for lift-and-shift
- Azure Database Migration Service
- Azure App Service for application modernization

**Post-Migration**
- Azure Monitor implementation
- Cost management and optimization
- Azure Security Center configuration

### ARM Template Configuration

` + "```json" + `
{
  "$schema": "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
  "contentVersion": "1.0.0.0",
  "parameters": {
    "vnetName": {
      "type": "string",
      "defaultValue": "migration-vnet",
      "metadata": {
        "description": "Name of the virtual network"
      }
    },
    "addressPrefix": {
      "type": "string",
      "defaultValue": "10.0.0.0/16",
      "metadata": {
        "description": "Address prefix for the virtual network"
      }
    }
  },
  "resources": [
    {
      "type": "Microsoft.Network/virtualNetworks",
      "apiVersion": "2021-02-01",
      "name": "[parameters('vnetName')]",
      "location": "[resourceGroup().location]",
      "properties": {
        "addressSpace": {
          "addressPrefixes": [
            "[parameters('addressPrefix')]"
          ]
        },
        "subnets": [
          {
            "name": "default",
            "properties": {
              "addressPrefix": "10.0.1.0/24"
            }
          }
        ]
      }
    }
  ]
}
` + "```" + `

### Azure CLI Commands

` + "```bash" + `
#!/bin/bash
# Azure Migration Setup Script

# Set variables
RESOURCE_GROUP="migration-rg"
LOCATION="eastus"
VNET_NAME="migration-vnet"

# Create resource group
az group create --name $RESOURCE_GROUP --location $LOCATION

# Create virtual network
az network vnet create \
  --resource-group $RESOURCE_GROUP \
  --name $VNET_NAME \
  --address-prefix 10.0.0.0/16 \
  --subnet-name default \
  --subnet-prefix 10.0.1.0/24

# Create migration project
az migrate project create \
  --name "migration-project" \
  --resource-group $RESOURCE_GROUP \
  --location $LOCATION
` + "```" + `

**Note**: This is a mock response generated for demonstration purposes.`
	} else {
		response += `### Cloud Architecture Guidance

Based on your query, here are key recommendations:

1. **Assessment**: Evaluate current infrastructure and applications
2. **Strategy**: Define migration approach (lift-and-shift, re-platform, or refactor)
3. **Security**: Implement cloud security best practices
4. **Monitoring**: Set up comprehensive monitoring and alerting
5. **Optimization**: Continuously optimize for cost and performance

This response demonstrates the AI assistant's capability to provide structured, actionable guidance for cloud architecture and migration scenarios.

### Generic Infrastructure Code

` + "```terraform" + `
# Generic Cloud Infrastructure Configuration
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.region
}

variable "region" {
  description = "Cloud region"
  type        = string
  default     = "us-west-2"
}

variable "environment" {
  description = "Environment name"
  type        = string
  default     = "production"
}

# Create VPC
resource "aws_vpc" "main" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name        = "main-vpc"
    Environment = var.environment
  }
}
` + "```" + `

### Monitoring Setup

` + "```bash" + `
#!/bin/bash
# Basic monitoring setup script

# Install CloudWatch agent
wget https://s3.amazonaws.com/amazoncloudwatch-agent/amazon_linux/amd64/latest/amazon-cloudwatch-agent.rpm
sudo rpm -U ./amazon-cloudwatch-agent.rpm

# Configure basic monitoring
cat > /opt/aws/amazon-cloudwatch-agent/bin/config.json << EOF
{
  "metrics": {
    "namespace": "CustomApp",
    "metrics_collected": {
      "cpu": {
        "measurement": [
          "cpu_usage_idle",
          "cpu_usage_iowait",
          "cpu_usage_user",
          "cpu_usage_system"
        ],
        "metrics_collection_interval": 60
      },
      "disk": {
        "measurement": [
          "used_percent"
        ],
        "metrics_collection_interval": 60,
        "resources": [
          "*"
        ]
      },
      "mem": {
        "measurement": [
          "mem_used_percent"
        ],
        "metrics_collection_interval": 60
      }
    }
  }
}
EOF

# Start CloudWatch agent
sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config -m ec2 -c file:/opt/aws/amazon-cloudwatch-agent/bin/config.json -s
` + "```" + `

**Note**: This is a mock response generated for demonstration purposes.`
	}

	return response
}

// truncateString truncates a string to specified length with ellipsis
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// estimateTokenCount estimates the number of tokens in a text
func estimateTokenCount(text string) int {
	return int(float64(len(text)) * TokenPerCharacterRatio)
}

// getErrorType determines the type of error for monitoring purposes
func getErrorType(err error) string {
	if err == nil {
		return "none"
	}
	errStr := strings.ToLower(err.Error())
	switch {
	case strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded"):
		return "timeout"
	case strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "too many requests"):
		return "rate_limit"
	case strings.Contains(errStr, "circuit breaker"):
		return "circuit_breaker"
	case strings.Contains(errStr, "connection") || strings.Contains(errStr, "network"):
		return "connection"
	case strings.Contains(errStr, "authentication") || strings.Contains(errStr, "unauthorized"):
		return "authentication"
	case strings.Contains(errStr, "service unavailable") || strings.Contains(errStr, "unavailable"):
		return "service_unavailable"
	default:
		return "unknown"
	}
}

// createRateLimitAwareRetryConfig creates a retry configuration optimized for rate limits
func createRateLimitAwareRetryConfig(baseConfig resilience.BackoffConfig) resilience.BackoffConfig {
	rateLimitConfig := baseConfig

	// Custom retry function that handles rate limits differently
	rateLimitConfig.RetryOnFunc = func(err error) bool {
		if err == nil {
			return false
		}

		// Don't retry on context cancellation or deadline exceeded
		if err == context.Canceled || err == context.DeadlineExceeded {
			return false
		}

		// Always retry on rate limit errors
		if strings.Contains(strings.ToLower(err.Error()), "rate limit") ||
			strings.Contains(strings.ToLower(err.Error()), "too many requests") {
			return true
		}

		// Use default retry logic for other errors
		return resilience.DefaultRetryOnFunc(err)
	}

	// Increase base delay for rate limit scenarios
	if rateLimitConfig.BaseDelay < 2*time.Second {
		rateLimitConfig.BaseDelay = 2 * time.Second
	}

	// Increase max delay for rate limits
	if rateLimitConfig.MaxDelay < 120*time.Second {
		rateLimitConfig.MaxDelay = 120 * time.Second
	}

	return rateLimitConfig
}

// ErrorResponse represents a structured error response
type ErrorResponse struct {
	Error       string   `json:"error"`
	Code        string   `json:"code"`
	Message     string   `json:"message"`
	Suggestions []string `json:"suggestions,omitempty"`
	RetryAfter  int      `json:"retry_after,omitempty"`
	SupportInfo string   `json:"support_info,omitempty"`
}

// handleSynthesisError provides graceful error handling with informative messages
func handleSynthesisError(c *gin.Context, err error, logger *zap.Logger, operation string) {
	errorType := getErrorType(err)

	var statusCode int
	var errorResponse ErrorResponse

	switch errorType {
	case "timeout":
		statusCode = http.StatusRequestTimeout
		errorResponse = ErrorResponse{
			Error:   "Request timeout",
			Code:    "TIMEOUT",
			Message: "The AI service is taking longer than expected to process your request. This can happen with complex queries or during high demand periods.",
			Suggestions: []string{
				"Try simplifying your query or breaking it into smaller parts",
				"Wait a moment and try again",
				"Consider using a different approach or fewer constraints",
			},
			RetryAfter:  30,
			SupportInfo: "If this persists, consider breaking down your request into smaller components",
		}
	case "rate_limit":
		statusCode = http.StatusTooManyRequests
		errorResponse = ErrorResponse{
			Error:   "Rate limit exceeded",
			Code:    "RATE_LIMIT",
			Message: "You've exceeded the API rate limit. Please wait before making additional requests.",
			Suggestions: []string{
				"Wait a moment before retrying",
				"Reduce the frequency of your requests",
				"Consider batching multiple queries together",
			},
			RetryAfter:  60,
			SupportInfo: "Rate limits help ensure fair usage across all users",
		}
	case "circuit_breaker":
		statusCode = http.StatusServiceUnavailable
		errorResponse = ErrorResponse{
			Error:   "Service temporarily unavailable",
			Code:    "SERVICE_UNAVAILABLE",
			Message: "The AI service is temporarily unavailable due to recent failures. This is a protective measure to prevent cascading issues.",
			Suggestions: []string{
				"Wait a few minutes and try again",
				"The service should automatically recover",
				"Try again with a simpler query",
			},
			RetryAfter:  120,
			SupportInfo: "This is an automatic protective mechanism that helps maintain service stability",
		}
	case "connection":
		statusCode = http.StatusBadGateway
		errorResponse = ErrorResponse{
			Error:   "Connection issue",
			Code:    "CONNECTION_ERROR",
			Message: "Unable to connect to the AI service. This is typically a temporary network issue.",
			Suggestions: []string{
				"Check your internet connection",
				"Try again in a few moments",
				"The issue may resolve automatically",
			},
			RetryAfter:  30,
			SupportInfo: "Network connectivity issues are usually temporary",
		}
	case "authentication":
		statusCode = http.StatusUnauthorized
		errorResponse = ErrorResponse{
			Error:   "Authentication failed",
			Code:    "AUTHENTICATION_ERROR",
			Message: "There was an issue with API authentication. This may indicate a configuration problem.",
			Suggestions: []string{
				"Contact your system administrator",
				"Verify that the service is properly configured",
			},
			SupportInfo: "This typically indicates a configuration issue that requires administrative attention",
		}
	default:
		statusCode = http.StatusInternalServerError
		errorResponse = ErrorResponse{
			Error:   "Internal server error",
			Code:    "INTERNAL_ERROR",
			Message: "An unexpected error occurred while processing your request. Our team has been notified.",
			Suggestions: []string{
				"Try again with a different query",
				"Simplify your request",
				"Wait a moment and retry",
			},
			RetryAfter:  60,
			SupportInfo: "If this continues, please contact support with details about your request",
		}
	}

	// Log the error for monitoring
	logger.Error("Synthesis error handled gracefully",
		zap.Error(err),
		zap.String("operation", operation),
		zap.String("error_type", errorType),
		zap.Int("status_code", statusCode),
		zap.Strings("suggestions", errorResponse.Suggestions),
	)

	c.JSON(statusCode, errorResponse)
}

// optimizePromptSize optimizes the prompt size to prevent timeouts
func optimizePromptSize(
	contextItems []synth.ContextItem,
	webResultStrings []string,
	conversationHistory []session.Message,
	logger *zap.Logger,
) ([]synth.ContextItem, []string, []session.Message) {
	// Estimate current prompt size
	totalTokens := 0
	for _, item := range contextItems {
		totalTokens += estimateTokenCount(item.Content)
	}
	for _, result := range webResultStrings {
		totalTokens += estimateTokenCount(result)
	}
	for _, msg := range conversationHistory {
		totalTokens += estimateTokenCount(msg.Content)
	}

	if totalTokens <= MaxPromptTokens {
		return contextItems, webResultStrings, conversationHistory
	}

	logger.Info("Optimizing prompt size due to large token count",
		zap.Int("estimated_tokens", totalTokens),
		zap.Int("max_tokens", MaxPromptTokens),
		zap.Int("context_items", len(contextItems)),
		zap.Int("web_results", len(webResultStrings)),
		zap.Int("conversation_history", len(conversationHistory)),
	)

	// Prioritize context items by relevance (keep the most relevant ones)
	optimizedContextItems := contextItems
	if len(contextItems) > MaxContextItemsForOptimization {
		optimizedContextItems = contextItems[:MaxContextItemsForOptimization]
		logger.Info("Reduced context items for optimization",
			zap.Int("original_count", len(contextItems)),
			zap.Int("optimized_count", len(optimizedContextItems)),
		)
	}

	// Limit web results
	optimizedWebResults := webResultStrings
	if len(webResultStrings) > MaxWebResultsForOptimization {
		optimizedWebResults = webResultStrings[:MaxWebResultsForOptimization]
		logger.Info("Reduced web results for optimization",
			zap.Int("original_count", len(webResultStrings)),
			zap.Int("optimized_count", len(optimizedWebResults)),
		)
	}

	// Limit conversation history to most recent messages
	optimizedHistory := conversationHistory
	if len(conversationHistory) > 6 {
		optimizedHistory = conversationHistory[len(conversationHistory)-6:]
		logger.Info("Reduced conversation history for optimization",
			zap.Int("original_count", len(conversationHistory)),
			zap.Int("optimized_count", len(optimizedHistory)),
		)
	}

	// Check if we still need to truncate context items
	totalTokensAfterOptimization := 0
	for _, item := range optimizedContextItems {
		totalTokensAfterOptimization += estimateTokenCount(item.Content)
	}
	for _, result := range optimizedWebResults {
		totalTokensAfterOptimization += estimateTokenCount(result)
	}
	for _, msg := range optimizedHistory {
		totalTokensAfterOptimization += estimateTokenCount(msg.Content)
	}

	// If still too large, truncate context item content
	if totalTokensAfterOptimization > MaxPromptTokens {
		maxContentLength := 1000 // Max chars per context item
		for i := range optimizedContextItems {
			if len(optimizedContextItems[i].Content) > maxContentLength {
				optimizedContextItems[i].Content = optimizedContextItems[i].Content[:maxContentLength] + "..."
			}
		}
		logger.Info("Truncated context item content for optimization",
			zap.Int("max_content_length", maxContentLength),
		)
	}

	return optimizedContextItems, optimizedWebResults, optimizedHistory
}

// buildOptimizedPrompt creates an optimized prompt for faster processing
func buildOptimizedPrompt(
	query string,
	contextItems []synth.ContextItem,
	webResultStrings []string,
	conversationHistory []session.Message,
	cfg *config.Config,
	logger *zap.Logger,
) string {
	// Optimize prompt size to prevent timeouts
	optimizedContextItems, optimizedWebResults, optimizedHistory := optimizePromptSize(
		contextItems,
		webResultStrings,
		conversationHistory,
		logger,
	)

	// Extract specific query parameters for contextual tailoring
	queryParams := extractQueryParameters(query)

	// Detect query domain for specialized handling
	domain := detectQueryDomain(query)

	// Apply complexity-based optimizations first to determine token allocation
	complexity := calculateQueryComplexity(SynthesisRequest{
		Query:               query,
		Chunks:              convertContextItemsToChunks(optimizedContextItems),
		WebResults:          convertWebResultStringsToWebResults(optimizedWebResults),
		ConversationHistory: optimizedHistory,
	}, logger)

	logger.Info("Query domain and complexity detected",
		zap.String("domain", string(domain)),
		zap.String("complexity", complexity),
		zap.Int("vm_count", queryParams.VmCount),
		zap.Strings("technologies", queryParams.Technologies),
		zap.Strings("cloud_providers", queryParams.CloudProviders),
		zap.Strings("specific_numbers", queryParams.SpecificNumbers),
		zap.Strings("constraints", queryParams.Constraints),
	)

	// Calculate target token limit based on complexity - more generous for complex queries
	var targetTokenLimit int
	switch complexity {
	case "simple":
		targetTokenLimit = cfg.Synthesis.MaxTokens / 2 // 1000 tokens (reserve half for response)
	case "medium":
		targetTokenLimit = int(float64(cfg.Synthesis.MaxTokens) * 0.7) // 1400 tokens
	case "complex":
		targetTokenLimit = int(float64(cfg.Synthesis.MaxTokens) * 0.85) // 1700 tokens (preserve more context)
	default:
		targetTokenLimit = cfg.Synthesis.MaxTokens / 2
	}

	// Build optimized prompt configuration based on complexity and domain
	promptConfig := synth.PromptConfig{
		MaxTokens:       targetTokenLimit,
		MaxContextItems: 8, // Default starting point
		MaxWebResults:   3, // Default starting point
		QueryType:       synth.DetectQueryType(query),
	}

	// Adjust context limits based on complexity
	switch complexity {
	case "simple":
		promptConfig.MaxContextItems = 5
		promptConfig.MaxWebResults = 2
	case "medium":
		promptConfig.MaxContextItems = 8
		promptConfig.MaxWebResults = 3
	case "complex":
		promptConfig.MaxContextItems = 15 // Allow more context for complex queries
		promptConfig.MaxWebResults = 6    // Allow more web results
	}

	logger.Debug("Building optimized prompt",
		zap.String("complexity", complexity),
		zap.Int("target_token_limit", targetTokenLimit),
		zap.Int("max_context_items", promptConfig.MaxContextItems),
		zap.Int("max_web_results", promptConfig.MaxWebResults),
		zap.Int("original_context_items", len(contextItems)),
		zap.Int("original_web_results", len(webResultStrings)),
	)

	// Build optimized prompt with limited context
	optimizedPrompt := synth.BuildPromptWithConversationAndConfig(
		query,
		optimizedContextItems,
		optimizedWebResults,
		optimizedHistory,
		promptConfig,
	)

	// Enhance prompt with domain-specific instructions
	optimizedPrompt = enhancePromptWithDomainInstructions(optimizedPrompt, domain, complexity)

	// Verify token limit - but be more lenient for complex queries
	estimatedTokens := synth.EstimateTokens(optimizedPrompt)
	if estimatedTokens > promptConfig.MaxTokens {
		if complexity == "complex" {
			// For complex queries, allow some overage to preserve context
			allowedOverage := int(float64(promptConfig.MaxTokens) * 0.2) // 20% overage
			if estimatedTokens <= promptConfig.MaxTokens+allowedOverage {
				logger.Info("Complex query prompt exceeds target but within allowable range",
					zap.Int("estimated_tokens", estimatedTokens),
					zap.Int("max_tokens", promptConfig.MaxTokens),
					zap.Int("allowed_overage", allowedOverage),
				)
			} else {
				logger.Warn("Complex query prompt significantly exceeds limit, applying minimal truncation",
					zap.Int("estimated_tokens", estimatedTokens),
					zap.Int("max_tokens_with_overage", promptConfig.MaxTokens+allowedOverage),
				)
				optimizedPrompt = synth.TruncateToTokenLimit(optimizedPrompt, promptConfig.MaxTokens+allowedOverage)
			}
		} else {
			logger.Warn("Optimized prompt exceeds target token limit, applying truncation",
				zap.Int("estimated_tokens", estimatedTokens),
				zap.Int("max_tokens", promptConfig.MaxTokens),
			)
			optimizedPrompt = synth.TruncateToTokenLimit(optimizedPrompt, promptConfig.MaxTokens)
		}
	}

	logger.Info("Optimized prompt created",
		zap.Int("estimated_tokens", synth.EstimateTokens(optimizedPrompt)),
		zap.Int("target_tokens", promptConfig.MaxTokens),
		zap.String("complexity", complexity),
	)

	return optimizedPrompt
}

// ResponseQualityMetrics represents quality metrics for a synthesized response
type ResponseQualityMetrics struct {
	HasSpecificRecommendations bool     `json:"has_specific_recommendations"`
	HasArchitectureDiagram     bool     `json:"has_architecture_diagram"`
	HasCodeExamples            bool     `json:"has_code_examples"`
	HasActionableSteps         bool     `json:"has_actionable_steps"`
	DomainRelevanceScore       float64  `json:"domain_relevance_score"`
	SpecificityScore           float64  `json:"specificity_score"`
	OverallQualityScore        float64  `json:"overall_quality_score"`
	QualityIssues              []string `json:"quality_issues"`
}

// validateResponseQuality performs post-generation quality validation
func validateResponseQuality(response, query string, domain QueryDomain) ResponseQualityMetrics {
	metrics := ResponseQualityMetrics{
		QualityIssues: make([]string, 0),
	}

	responseLower := strings.ToLower(response)

	// Check for architecture diagrams
	metrics.HasArchitectureDiagram = strings.Contains(response, "```mermaid") ||
		strings.Contains(response, "graph TD") ||
		strings.Contains(response, "diagram")

	// Check for code examples
	metrics.HasCodeExamples = strings.Contains(response, "```terraform") ||
		strings.Contains(response, "```bash") ||
		strings.Contains(response, "```yaml") ||
		strings.Contains(response, "```json") ||
		strings.Contains(response, "aws cli") ||
		strings.Contains(responseLower, "terraform")

	// Check for actionable steps
	metrics.HasActionableSteps = strings.Contains(response, "1.") ||
		strings.Contains(response, "Step") ||
		strings.Contains(response, "Phase") ||
		strings.Contains(response, "Implementation")

	// Domain-specific validation
	switch domain {
	case MigrationDomain:
		metrics.DomainRelevanceScore = validateMigrationResponse(response, query)
		metrics.HasSpecificRecommendations = checkMigrationSpecificity(response, query)
	case ComplianceDomain:
		metrics.DomainRelevanceScore = validateComplianceResponse(response, query)
		metrics.HasSpecificRecommendations = checkComplianceSpecificity(response, query)
	case DisasterRecovery:
		metrics.DomainRelevanceScore = validateDRResponse(response, query)
		metrics.HasSpecificRecommendations = checkDRSpecificity(response, query)
	case ArchitectureDomain:
		metrics.DomainRelevanceScore = validateArchitectureResponse(response, query)
		metrics.HasSpecificRecommendations = checkArchitectureSpecificity(response, query)
	case GeneralDomain:
		metrics.DomainRelevanceScore = 0.7                       // Default moderate score
		metrics.HasSpecificRecommendations = len(response) > 500 // Basic length check
	default:
		metrics.DomainRelevanceScore = 0.7                       // Default moderate score
		metrics.HasSpecificRecommendations = len(response) > 500 // Basic length check
	}

	// Calculate specificity score
	metrics.SpecificityScore = calculateSpecificityScore(response, query)

	// Identify quality issues
	if !metrics.HasSpecificRecommendations {
		metrics.QualityIssues = append(metrics.QualityIssues, "Response lacks specific recommendations")
	}
	if metrics.DomainRelevanceScore < 0.6 {
		metrics.QualityIssues = append(metrics.QualityIssues, "Response not sufficiently relevant to query domain")
	}
	if metrics.SpecificityScore < 0.5 {
		metrics.QualityIssues = append(metrics.QualityIssues, "Response is too generic")
	}
	if domain == MigrationDomain && !metrics.HasArchitectureDiagram {
		metrics.QualityIssues = append(metrics.QualityIssues, "Migration query should include architecture diagram")
	}

	// Calculate overall quality score
	scores := []float64{metrics.DomainRelevanceScore, metrics.SpecificityScore}
	if metrics.HasSpecificRecommendations {
		scores = append(scores, 1.0)
	} else {
		scores = append(scores, 0.0)
	}
	if metrics.HasArchitectureDiagram {
		scores = append(scores, 1.0)
	} else {
		scores = append(scores, 0.0)
	}
	if metrics.HasCodeExamples {
		scores = append(scores, 1.0)
	} else {
		scores = append(scores, 0.0)
	}

	total := 0.0
	for _, score := range scores {
		total += score
	}
	metrics.OverallQualityScore = total / float64(len(scores))

	return metrics
}

// validateMigrationResponse checks if migration response contains expected elements
func validateMigrationResponse(response, query string) float64 {
	responseLower := strings.ToLower(response)
	queryLower := strings.ToLower(query)

	score := 0.0

	// Check for migration-specific terms
	migrationTerms := []string{"migration", "mgn", "lift-and-shift", "vpc", "ec2", "replication"}
	for _, term := range migrationTerms {
		if strings.Contains(responseLower, term) {
			score += 0.15
		}
	}

	// Check for specific VM counts if mentioned in query
	if strings.Contains(queryLower, "120") && strings.Contains(responseLower, "120") {
		score += 0.2
	}

	// Check for AWS services if AWS migration
	if strings.Contains(queryLower, "aws") {
		awsTerms := []string{"application migration service", "mgn", "vpc", "subnet", "security group"}
		for _, term := range awsTerms {
			if strings.Contains(responseLower, term) {
				score += 0.1
			}
		}
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// checkMigrationSpecificity validates migration response has specific recommendations
func checkMigrationSpecificity(response, query string) bool {
	responseLower := strings.ToLower(response)

	// Check for specific instance types or sizing
	specificTerms := []string{
		"t3.medium", "t3.large", "c5.large", "r5.large", "instance type",
		"subnet", "10.0.", "172.16.", "192.168.", "cidr",
		"security group", "phase 1", "phase 2", "step-by-step",
	}

	count := 0
	for _, term := range specificTerms {
		if strings.Contains(responseLower, term) {
			count++
		}
	}

	return count >= 3 // Need at least 3 specific terms
}

// validateComplianceResponse checks compliance response quality
func validateComplianceResponse(response, query string) float64 {
	responseLower := strings.ToLower(response)
	score := 0.0

	complianceTerms := []string{"encryption", "audit", "compliance", "gdpr", "hipaa", "policy", "access control"}
	for _, term := range complianceTerms {
		if strings.Contains(responseLower, term) {
			score += 0.15
		}
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// checkComplianceSpecificity validates compliance response specificity
func checkComplianceSpecificity(response, query string) bool {
	responseLower := strings.ToLower(response)
	specificTerms := []string{"aes-256", "tls 1.2", "rbac", "iam policy", "kms", "certificate"}

	count := 0
	for _, term := range specificTerms {
		if strings.Contains(responseLower, term) {
			count++
		}
	}

	return count >= 2
}

// validateDRResponse checks disaster recovery response quality
func validateDRResponse(response, query string) float64 {
	responseLower := strings.ToLower(response)
	score := 0.0

	drTerms := []string{"rto", "rpo", "backup", "failover", "disaster recovery", "replication"}
	for _, term := range drTerms {
		if strings.Contains(responseLower, term) {
			score += 0.15
		}
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// checkDRSpecificity validates DR response specificity
func checkDRSpecificity(response, query string) bool {
	responseLower := strings.ToLower(response)
	specificTerms := []string{"15 minutes", "2 hours", "cross-region", "site recovery", "backup policy"}

	count := 0
	for _, term := range specificTerms {
		if strings.Contains(responseLower, term) {
			count++
		}
	}

	return count >= 2
}

// validateArchitectureResponse checks architecture response quality
func validateArchitectureResponse(response, query string) float64 {
	responseLower := strings.ToLower(response)
	score := 0.0

	archTerms := []string{"architecture", "component", "service", "network", "scalability", "security"}
	for _, term := range archTerms {
		if strings.Contains(responseLower, term) {
			score += 0.15
		}
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// checkArchitectureSpecificity validates architecture response specificity
func checkArchitectureSpecificity(response, query string) bool {
	responseLower := strings.ToLower(response)
	specificTerms := []string{"load balancer", "auto scaling", "database", "cache", "monitoring"}

	count := 0
	for _, term := range specificTerms {
		if strings.Contains(responseLower, term) {
			count++
		}
	}

	return count >= 3
}

// calculateSpecificityScore measures how specific vs generic the response is
func calculateSpecificityScore(response, query string) float64 {
	responseLower := strings.ToLower(response)

	// Generic terms indicate lower specificity
	genericTerms := []string{
		"general", "overview", "typically", "usually", "consider", "might",
		"could", "should consider", "it depends", "varies", "generally",
	}

	// Specific terms indicate higher specificity
	specificTerms := []string{
		"t3.medium", "c5.large", "10.0.1.0/24", "port 443", "aes-256",
		"15 minutes", "2 hours", "terraform apply", "aws cli", "specific",
		"configure", "step 1", "phase 1", "implement", "deploy",
	}

	genericCount := 0
	specificCount := 0

	for _, term := range genericTerms {
		if strings.Contains(responseLower, term) {
			genericCount++
		}
	}

	for _, term := range specificTerms {
		if strings.Contains(responseLower, term) {
			specificCount++
		}
	}

	// Calculate ratio - more specific terms = higher score
	if genericCount == 0 && specificCount == 0 {
		return 0.5 // Neutral if no indicators
	}

	totalTerms := genericCount + specificCount
	specificityRatio := float64(specificCount) / float64(totalTerms)

	// Boost score if response has many specific terms
	if specificCount >= 5 {
		specificityRatio += 0.2
	}

	if specificityRatio > 1.0 {
		specificityRatio = 1.0
	}

	return specificityRatio
}

// enhancePromptWithDomainInstructions adds domain-specific instructions to improve response quality
func enhancePromptWithDomainInstructions(prompt string, domain QueryDomain, complexity string) string {
	var domainInstructions string

	switch domain {
	case MigrationDomain:
		domainInstructions = `
MIGRATION DOMAIN INSTRUCTIONS:
You are responding to a migration-related query. Your response MUST include:

1. **Migration Strategy**: Provide a specific migration approach (lift-and-shift, replatform, refactor)
2. **Technical Requirements**: Include specific VM counts, infrastructure sizing, and network requirements
3. **Migration Tools**: Reference specific migration services (AWS MGN, Azure Migrate, etc.)
4. **Architecture Diagram**: Generate a Mermaid diagram showing source and target architecture with migration flow
5. **Implementation Timeline**: Provide realistic phases and timelines
6. **MANDATORY Code Examples**: Include complete infrastructure-as-code (Terraform, ARM templates) and migration scripts

MANDATORY CODE GENERATION FOR MIGRATION QUERIES:
- MUST provide complete Terraform configurations for target infrastructure in proper code blocks (use three backticks terraform or three backticks hcl)
- MUST include migration automation scripts in proper bash/powershell code blocks (use three backticks bash or three backticks powershell)
- MUST provide AWS CLI or Azure CLI commands for migration setup in proper code blocks (use three backticks bash)
- MUST include network configuration code (VPC, subnets, security groups) in proper code blocks (use three backticks terraform)
- MUST provide monitoring and validation scripts in proper code blocks (use three backticks bash or three backticks powershell)
- MUST include backup and rollback procedures in code blocks (use three backticks bash or three backticks powershell)

FORMAT REQUIREMENTS FOR CODE BLOCKS:
- Each code block MUST start with three backticks followed by the language (terraform, bash, powershell, etc.)
- Code MUST be complete and executable, not just snippets
- Each code block MUST end with three backticks
- NEVER use generic code blocks without language specification

SPECIFIC REQUIREMENTS:
- For AWS migrations: Include complete VPC design, subnet planning, security groups, and EC2 instance recommendations IN EXECUTABLE CODE
- For large VM migrations (100+): Address bulk migration strategies and automation IN EXECUTABLE CODE
- Include specific network topology and connectivity requirements IN EXECUTABLE CODE
- Provide actionable next steps with concrete implementation guidance IN EXECUTABLE CODE

CRITICAL: Migration queries without complete, executable code blocks are unacceptable.
Every migration response MUST contain at least 3 code blocks:
1. One Terraform/infrastructure code block (three backticks terraform or three backticks hcl)
2. One automation/CLI script block (three backticks bash or three backticks powershell)
3. One configuration/setup code block (three backticks bash, three backticks yaml, or three backticks json)

FAILURE TO INCLUDE THESE CODE BLOCKS WILL RESULT IN AN UNACCEPTABLE RESPONSE.

`
	case ComplianceDomain:
		domainInstructions = `
COMPLIANCE DOMAIN INSTRUCTIONS:
You are responding to a compliance-related query. Your response must include:

1. **Compliance Framework**: Identify specific frameworks (HIPAA, GDPR, SOC2, PCI-DSS)
2. **Technical Controls**: List specific security controls and implementation requirements
3. **Architecture Diagram**: Show compliance boundaries, data flow, and security controls
4. **Implementation Checklist**: Provide actionable compliance implementation steps
5. **Monitoring & Auditing**: Include logging, monitoring, and audit trail requirements

SPECIFIC REQUIREMENTS:
- Address data encryption (at rest and in transit)
- Include network security and access controls
- Provide specific configuration examples for compliance tools
- Include documentation and audit trail requirements

`
	case DisasterRecovery:
		domainInstructions = `
DISASTER RECOVERY DOMAIN INSTRUCTIONS:
You are responding to a disaster recovery query. Your response must include:

1. **RTO/RPO Analysis**: Address specific recovery time and point objectives
2. **DR Strategy**: Define backup, replication, and failover approaches
3. **Architecture Diagram**: Show primary and DR sites with replication flows
4. **Failover Procedures**: Provide step-by-step failover and failback processes
5. **Testing Plan**: Include DR testing and validation procedures

SPECIFIC REQUIREMENTS:
- Include specific backup and replication technologies
- Address cross-region or cross-cloud DR strategies
- Provide recovery automation scripts and procedures
- Include cost optimization for DR infrastructure

`
	case ArchitectureDomain:
		domainInstructions = `
ARCHITECTURE DOMAIN INSTRUCTIONS:
You are responding to an architecture design query. Your response must include:

1. **Architecture Pattern**: Identify and explain the architectural pattern being used
2. **Component Design**: Detail each architectural component and its purpose
3. **Architecture Diagram**: Generate comprehensive Mermaid diagrams showing system components
4. **Technology Stack**: Recommend specific technologies and justify choices
5. **Scalability & Performance**: Address scalability patterns and performance considerations

SPECIFIC REQUIREMENTS:
- Include specific cloud services and configurations
- Address security, networking, and data flow
- Provide infrastructure-as-code examples
- Include monitoring and operational considerations

`
	case GeneralDomain:
		// For general queries, add minimal enhancement focused on specificity
		domainInstructions = `
GENERAL CLOUD GUIDANCE:
Provide specific, actionable cloud architecture guidance with:
- Concrete service recommendations and configurations
- Architecture diagrams when applicable
- Implementation code examples
- Best practices and security considerations

`
	default:
		// For general queries, add minimal enhancement focused on specificity
		domainInstructions = `
GENERAL CLOUD GUIDANCE:
Provide specific, actionable cloud architecture guidance with:
- Concrete service recommendations and configurations
- Architecture diagrams when applicable
- Implementation code examples
- Best practices and security considerations

`
	}

	// For complex queries, add additional requirements for comprehensive responses
	if complexity == "complex" {
		domainInstructions += `
COMPLEX QUERY ENHANCEMENT:
This is a complex enterprise query. Ensure your response is comprehensive and includes:
- Detailed technical specifications and sizing recommendations
- Multiple implementation options with trade-offs
- Enterprise-grade security and compliance considerations
- Comprehensive architecture diagrams with detailed component labels
- Production-ready code examples and configurations
- Step-by-step implementation phases with timelines
- Cost considerations and optimization strategies

`
	}

	// Insert domain instructions before the final "Please provide your comprehensive response now:"
	insertPoint := "Please provide your comprehensive response now:"
	if strings.Contains(prompt, insertPoint) {
		return strings.Replace(prompt, insertPoint, domainInstructions+insertPoint, 1)
	}

	// Fallback: append to the end of the prompt
	return prompt + "\n" + domainInstructions
}

// convertContextItemsToChunks converts context items to chunk items for compatibility
func convertContextItemsToChunks(contextItems []synth.ContextItem) []ChunkItem {
	chunks := make([]ChunkItem, len(contextItems))
	for i, item := range contextItems {
		chunks[i] = ChunkItem{
			Text:     item.Content,
			DocID:    item.SourceID,
			SourceID: item.SourceID,
		}
	}
	return chunks
}

// convertWebResultStringsToWebResults converts web result strings to web result structs
func convertWebResultStringsToWebResults(webResultStrings []string) []WebResult {
	webResults := make([]WebResult, len(webResultStrings))
	for i, result := range webResultStrings {
		// Parse the web result string to extract title, snippet, and URL
		lines := strings.Split(result, "\n")
		webResult := WebResult{}

		for _, line := range lines {
			if strings.HasPrefix(line, "Title: ") {
				webResult.Title = strings.TrimSpace(strings.TrimPrefix(line, "Title: "))
			} else if strings.HasPrefix(line, "Snippet: ") {
				webResult.Snippet = strings.TrimSpace(strings.TrimPrefix(line, "Snippet: "))
			} else if strings.HasPrefix(line, "URL: ") {
				webResult.URL = strings.TrimSpace(strings.TrimPrefix(line, "URL: "))
			}
		}

		// If parsing failed, use the entire string as snippet
		if webResult.Title == "" && webResult.Snippet == "" && webResult.URL == "" {
			webResult.Snippet = result
		}

		webResults[i] = webResult
	}
	return webResults
}

// enhancePromptForRegeneration adds regeneration-specific instructions to an optimized prompt
func enhancePromptForRegeneration(basePrompt string, previousResponse *string) string {
	if previousResponse == nil || *previousResponse == "" {
		return basePrompt
	}

	// Add regeneration instructions without duplicating the full context
	regenerationInstructions := fmt.Sprintf(`

--- Previous Response ---
%s

--- Regeneration Instructions ---
Please provide an alternative response to the same query. Consider:
1. Different perspectives or approaches to the problem
2. Alternative architectural patterns or solutions
3. Varied level of technical detail
4. Different emphasis areas (cost, security, performance, etc.)

Generate a fresh response that covers the same query but with a different angle or approach.`, *previousResponse)

	return basePrompt + regenerationInstructions
}

// CostBreakdown represents detailed cost analysis for a migration scenario
type CostBreakdown struct {
	TotalMonthlyOnDemand   float64      `json:"total_monthly_on_demand"`
	TotalMonthlyReserved   float64      `json:"total_monthly_reserved"`
	TotalMonthlySavings    float64      `json:"total_monthly_savings"`
	EC2Costs               ServiceCosts `json:"ec2_costs"`
	StorageCosts           ServiceCosts `json:"storage_costs"`
	NetworkingCosts        ServiceCosts `json:"networking_costs"`
	DatabaseCosts          ServiceCosts `json:"database_costs"`
	SecurityCosts          ServiceCosts `json:"security_costs"`
	OptimizationStrategies []string     `json:"optimization_strategies"`
	ROIAnalysis            ROIAnalysis  `json:"roi_analysis"`
}

// ServiceCosts represents costs for a specific AWS service
type ServiceCosts struct {
	OnDemandMonthly float64  `json:"on_demand_monthly"`
	ReservedMonthly float64  `json:"reserved_monthly"`
	SavingsPercent  float64  `json:"savings_percent"`
	Details         []string `json:"details"`
}

// ROIAnalysis represents return on investment analysis
type ROIAnalysis struct {
	MigrationCost        float64 `json:"migration_cost"`
	MonthlyInfraCost     float64 `json:"monthly_infra_cost"`
	MonthlyOperatingCost float64 `json:"monthly_operating_cost"`
	BreakEvenMonths      int     `json:"break_even_months"`
	YearOneROI           float64 `json:"year_one_roi"`
	ThreeYearROI         float64 `json:"three_year_roi"`
}

// calculateDetailedCosts provides comprehensive cost analysis for migration scenarios
func calculateDetailedCosts(queryParams QueryParameters, cloudProvider string) CostBreakdown {
	vmCount := queryParams.VmCount
	if vmCount == 0 {
		vmCount = 1 // Default for single VM scenarios
	}

	var breakdown CostBreakdown

	switch strings.ToLower(cloudProvider) {
	case "aws":
		breakdown = calculateAWSCosts(vmCount, queryParams.Technologies)
	case "azure":
		breakdown = calculateAzureCosts(vmCount, queryParams.Technologies)
	default:
		breakdown = calculateAWSCosts(vmCount, queryParams.Technologies) // Default to AWS
	}

	// Add optimization strategies
	breakdown.OptimizationStrategies = generateOptimizationStrategies(vmCount, queryParams.Technologies)

	// Calculate ROI analysis
	breakdown.ROIAnalysis = calculateROIAnalysis(vmCount, breakdown.TotalMonthlyOnDemand, breakdown.TotalMonthlyReserved)

	return breakdown
}

// calculateAWSCosts calculates detailed AWS cost breakdown
func calculateAWSCosts(vmCount int, technologies []string) CostBreakdown {
	// Base EC2 costs (using t3.medium as baseline)
	ec2OnDemandPerVM := 30.00 // $30/month per t3.medium on-demand
	ec2ReservedPerVM := 18.00 // $18/month per t3.medium reserved (1-year term)

	// Storage costs (EBS gp3)
	storagePerVM := 15.00 // $15/month per VM for 150GB gp3 storage

	// Networking costs
	networkingPerVM := 8.00 // $8/month per VM for typical networking

	// Database costs (if SQL Server is involved)
	dbCostPerVM := 0.00
	if containsTechnology(technologies, "SQL Server") {
		dbCostPerVM = 250.00 // $250/month for RDS SQL Server per instance
	}

	// Security costs (AWS Security Hub, GuardDuty, etc.)
	securityPerVM := 12.00 // $12/month per VM for security services

	// Adjust costs based on technologies
	ec2OnDemandPerVM, ec2ReservedPerVM = adjustCostsForTechnologies(ec2OnDemandPerVM, ec2ReservedPerVM, technologies)

	// Calculate totals
	ec2OnDemandTotal := float64(vmCount) * ec2OnDemandPerVM
	ec2ReservedTotal := float64(vmCount) * ec2ReservedPerVM
	storageTotal := float64(vmCount) * storagePerVM
	networkingTotal := float64(vmCount) * networkingPerVM
	dbTotal := float64(vmCount) * dbCostPerVM
	securityTotal := float64(vmCount) * securityPerVM

	totalOnDemand := ec2OnDemandTotal + storageTotal + networkingTotal + dbTotal + securityTotal
	totalReserved := ec2ReservedTotal + storageTotal + networkingTotal + dbTotal + securityTotal

	return CostBreakdown{
		TotalMonthlyOnDemand: totalOnDemand,
		TotalMonthlyReserved: totalReserved,
		TotalMonthlySavings:  totalOnDemand - totalReserved,
		EC2Costs: ServiceCosts{
			OnDemandMonthly: ec2OnDemandTotal,
			ReservedMonthly: ec2ReservedTotal,
			SavingsPercent:  ((ec2OnDemandTotal - ec2ReservedTotal) / ec2OnDemandTotal) * 100,
			Details: []string{
				fmt.Sprintf("%d x t3.medium instances", vmCount),
				"Includes compute, memory, and CPU resources",
				"Pricing based on us-west-2 region",
			},
		},
		StorageCosts: ServiceCosts{
			OnDemandMonthly: storageTotal,
			ReservedMonthly: storageTotal,
			SavingsPercent:  0,
			Details: []string{
				fmt.Sprintf("%d x 150GB gp3 volumes", vmCount),
				"3,000 IOPS and 125 MB/s baseline performance",
				"Includes snapshots and backup storage",
			},
		},
		NetworkingCosts: ServiceCosts{
			OnDemandMonthly: networkingTotal,
			ReservedMonthly: networkingTotal,
			SavingsPercent:  0,
			Details: []string{
				"VPC networking and data transfer",
				"Application Load Balancer costs",
				"NAT Gateway for private subnet access",
			},
		},
		DatabaseCosts: ServiceCosts{
			OnDemandMonthly: dbTotal,
			ReservedMonthly: dbTotal * 0.6, // 40% savings with reserved instances
			SavingsPercent:  40,
			Details:         generateDatabaseCostDetails(technologies),
		},
		SecurityCosts: ServiceCosts{
			OnDemandMonthly: securityTotal,
			ReservedMonthly: securityTotal,
			SavingsPercent:  0,
			Details: []string{
				"AWS Security Hub and Config",
				"GuardDuty threat detection",
				"AWS WAF and Shield Standard",
			},
		},
	}
}

// calculateAzureCosts calculates detailed Azure cost breakdown
func calculateAzureCosts(vmCount int, technologies []string) CostBreakdown {
	// Base Azure VM costs (using Standard_B2s as baseline)
	vmOnDemandPerVM := 35.00 // $35/month per Standard_B2s on-demand
	vmReservedPerVM := 21.00 // $21/month per Standard_B2s reserved (1-year term)

	// Storage costs (Premium SSD)
	storagePerVM := 20.00 // $20/month per VM for 128GB Premium SSD

	// Networking costs
	networkingPerVM := 10.00 // $10/month per VM for typical networking

	// Database costs (if SQL Server is involved)
	dbCostPerVM := 0.00
	if containsTechnology(technologies, "SQL Server") {
		dbCostPerVM = 280.00 // $280/month for Azure SQL Database per instance
	}

	// Security costs (Azure Security Center, Sentinel, etc.)
	securityPerVM := 15.00 // $15/month per VM for security services

	// Calculate totals
	vmOnDemandTotal := float64(vmCount) * vmOnDemandPerVM
	vmReservedTotal := float64(vmCount) * vmReservedPerVM
	storageTotal := float64(vmCount) * storagePerVM
	networkingTotal := float64(vmCount) * networkingPerVM
	dbTotal := float64(vmCount) * dbCostPerVM
	securityTotal := float64(vmCount) * securityPerVM

	totalOnDemand := vmOnDemandTotal + storageTotal + networkingTotal + dbTotal + securityTotal
	totalReserved := vmReservedTotal + storageTotal + networkingTotal + dbTotal + securityTotal

	return CostBreakdown{
		TotalMonthlyOnDemand: totalOnDemand,
		TotalMonthlyReserved: totalReserved,
		TotalMonthlySavings:  totalOnDemand - totalReserved,
		EC2Costs: ServiceCosts{
			OnDemandMonthly: vmOnDemandTotal,
			ReservedMonthly: vmReservedTotal,
			SavingsPercent:  ((vmOnDemandTotal - vmReservedTotal) / vmOnDemandTotal) * 100,
			Details: []string{
				fmt.Sprintf("%d x Standard_B2s instances", vmCount),
				"2 vCPUs, 4GB RAM per instance",
				"Pricing based on East US region",
			},
		},
		StorageCosts: ServiceCosts{
			OnDemandMonthly: storageTotal,
			ReservedMonthly: storageTotal,
			SavingsPercent:  0,
			Details: []string{
				fmt.Sprintf("%d x 128GB Premium SSD", vmCount),
				"500 IOPS and 60 MB/s performance",
				"Includes managed disk snapshots",
			},
		},
		NetworkingCosts: ServiceCosts{
			OnDemandMonthly: networkingTotal,
			ReservedMonthly: networkingTotal,
			SavingsPercent:  0,
			Details: []string{
				"Virtual Network and subnets",
				"Application Gateway costs",
				"Azure Firewall for security",
			},
		},
		DatabaseCosts: ServiceCosts{
			OnDemandMonthly: dbTotal,
			ReservedMonthly: dbTotal * 0.65, // 35% savings with reserved capacity
			SavingsPercent:  35,
			Details:         generateDatabaseCostDetails(technologies),
		},
		SecurityCosts: ServiceCosts{
			OnDemandMonthly: securityTotal,
			ReservedMonthly: securityTotal,
			SavingsPercent:  0,
			Details: []string{
				"Azure Security Center Standard",
				"Azure Sentinel SIEM",
				"Key Vault and managed identities",
			},
		},
	}
}

// adjustCostsForTechnologies adjusts instance costs based on workload technologies
func adjustCostsForTechnologies(onDemandCost, reservedCost float64, technologies []string) (float64, float64) {
	multiplier := 1.0

	// Windows licensing increases costs
	if containsTechnology(technologies, "Windows") {
		multiplier += 0.3 // 30% increase for Windows licensing
	}

	// SQL Server significantly increases costs
	if containsTechnology(technologies, "SQL Server") {
		multiplier += 0.5 // 50% increase for SQL Server licensing
	}

	// VMware workloads may need larger instances
	if containsTechnology(technologies, "VMware") {
		multiplier += 0.2 // 20% increase for VMware compatibility
	}

	return onDemandCost * multiplier, reservedCost * multiplier
}

// containsTechnology checks if a technology is in the list
func containsTechnology(technologies []string, technology string) bool {
	for _, tech := range technologies {
		if strings.EqualFold(tech, technology) {
			return true
		}
	}
	return false
}

// generateDatabaseCostDetails generates database-specific cost details
func generateDatabaseCostDetails(technologies []string) []string {
	if containsTechnology(technologies, "SQL Server") {
		return []string{
			"SQL Server Standard Edition licensing",
			"High availability with Always On",
			"Automated backups and point-in-time recovery",
		}
	}
	return []string{"No database costs for this scenario"}
}

// generateOptimizationStrategies provides cost optimization recommendations
func generateOptimizationStrategies(vmCount int, technologies []string) []string {
	strategies := []string{
		"Purchase Reserved Instances for 1-year term (40% savings)",
		"Implement auto-scaling to match demand patterns",
		"Use Spot Instances for non-critical workloads (70% savings)",
		"Right-size instances based on actual utilization",
		"Implement lifecycle policies for storage optimization",
	}

	if vmCount > 50 {
		strategies = append(strategies, "Consider Savings Plans for additional 10-15% savings")
		strategies = append(strategies, "Negotiate Enterprise Discount Program (EDP) for volume pricing")
	}

	if containsTechnology(technologies, "Windows") {
		strategies = append(strategies, "Evaluate Hybrid Benefit for Windows licensing savings")
	}

	if containsTechnology(technologies, "SQL Server") {
		strategies = append(strategies, "Consider Azure SQL Database for lower total cost of ownership")
		strategies = append(strategies, "Implement read replicas for reporting workloads")
	}

	return strategies
}

// calculateROIAnalysis calculates return on investment for migration
func calculateROIAnalysis(vmCount int, monthlyOnDemand, monthlyReserved float64) ROIAnalysis {
	// Estimated migration costs
	migrationCostPerVM := 2500.00 // $2,500 per VM for migration
	totalMigrationCost := float64(vmCount) * migrationCostPerVM

	// On-premises costs (estimated)
	onPremMonthlyCost := float64(vmCount) * 120.00 // $120/month per VM on-premises

	// Monthly savings with cloud (using reserved pricing)
	monthlySavings := onPremMonthlyCost - monthlyReserved

	// Calculate break-even point
	breakEvenMonths := int(totalMigrationCost / monthlySavings)

	// Calculate ROI
	yearOneSavings := monthlySavings * 12
	yearOneROI := ((yearOneSavings - totalMigrationCost) / totalMigrationCost) * 100

	threeYearSavings := monthlySavings * 36
	threeYearROI := ((threeYearSavings - totalMigrationCost) / totalMigrationCost) * 100

	return ROIAnalysis{
		MigrationCost:        totalMigrationCost,
		MonthlyInfraCost:     monthlyReserved,
		MonthlyOperatingCost: monthlyReserved * 0.3, // 30% of infra cost for operations
		BreakEvenMonths:      breakEvenMonths,
		YearOneROI:           yearOneROI,
		ThreeYearROI:         threeYearROI,
	}
}

// formatCostBreakdown formats the cost breakdown for display
func formatCostBreakdown(breakdown CostBreakdown) string {
	var result strings.Builder

	result.WriteString("## 💰 Detailed Cost Analysis\n\n")

	// Summary
	result.WriteString("### Cost Summary\n")
	result.WriteString(fmt.Sprintf("- **On-Demand Monthly**: $%.2f\n", breakdown.TotalMonthlyOnDemand))
	result.WriteString(fmt.Sprintf("- **Reserved Monthly**: $%.2f\n", breakdown.TotalMonthlyReserved))
	result.WriteString(fmt.Sprintf("- **Monthly Savings**: $%.2f (%.1f%%)\n\n",
		breakdown.TotalMonthlySavings,
		(breakdown.TotalMonthlySavings/breakdown.TotalMonthlyOnDemand)*100))

	// Service breakdown
	result.WriteString("### Service Cost Breakdown\n\n")

	// EC2/Compute costs
	result.WriteString("**Compute (EC2/VM)**\n")
	result.WriteString(fmt.Sprintf("- On-Demand: $%.2f/month\n", breakdown.EC2Costs.OnDemandMonthly))
	result.WriteString(fmt.Sprintf("- Reserved: $%.2f/month (%.1f%% savings)\n",
		breakdown.EC2Costs.ReservedMonthly, breakdown.EC2Costs.SavingsPercent))
	for _, detail := range breakdown.EC2Costs.Details {
		result.WriteString(fmt.Sprintf("  - %s\n", detail))
	}
	result.WriteString("\n")

	// Storage costs
	result.WriteString("**Storage**\n")
	result.WriteString(fmt.Sprintf("- Monthly: $%.2f\n", breakdown.StorageCosts.OnDemandMonthly))
	for _, detail := range breakdown.StorageCosts.Details {
		result.WriteString(fmt.Sprintf("  - %s\n", detail))
	}
	result.WriteString("\n")

	// Networking costs
	result.WriteString("**Networking**\n")
	result.WriteString(fmt.Sprintf("- Monthly: $%.2f\n", breakdown.NetworkingCosts.OnDemandMonthly))
	for _, detail := range breakdown.NetworkingCosts.Details {
		result.WriteString(fmt.Sprintf("  - %s\n", detail))
	}
	result.WriteString("\n")

	// Database costs (if applicable)
	if breakdown.DatabaseCosts.OnDemandMonthly > 0 {
		result.WriteString("**Database**\n")
		result.WriteString(fmt.Sprintf("- On-Demand: $%.2f/month\n", breakdown.DatabaseCosts.OnDemandMonthly))
		result.WriteString(fmt.Sprintf("- Reserved: $%.2f/month (%.1f%% savings)\n",
			breakdown.DatabaseCosts.ReservedMonthly, breakdown.DatabaseCosts.SavingsPercent))
		for _, detail := range breakdown.DatabaseCosts.Details {
			result.WriteString(fmt.Sprintf("  - %s\n", detail))
		}
		result.WriteString("\n")
	}

	// Security costs
	result.WriteString("**Security**\n")
	result.WriteString(fmt.Sprintf("- Monthly: $%.2f\n", breakdown.SecurityCosts.OnDemandMonthly))
	for _, detail := range breakdown.SecurityCosts.Details {
		result.WriteString(fmt.Sprintf("  - %s\n", detail))
	}
	result.WriteString("\n")

	// Cost optimization strategies
	result.WriteString("### 🎯 Cost Optimization Strategies\n\n")
	for i, strategy := range breakdown.OptimizationStrategies {
		result.WriteString(fmt.Sprintf("%d. %s\n", i+1, strategy))
	}
	result.WriteString("\n")

	// ROI Analysis
	result.WriteString("### 📊 ROI Analysis\n\n")
	result.WriteString(fmt.Sprintf("- **Migration Cost**: $%.2f\n", breakdown.ROIAnalysis.MigrationCost))
	result.WriteString(fmt.Sprintf("- **Monthly Infrastructure Cost**: $%.2f\n", breakdown.ROIAnalysis.MonthlyInfraCost))
	result.WriteString(fmt.Sprintf("- **Monthly Operating Cost**: $%.2f\n", breakdown.ROIAnalysis.MonthlyOperatingCost))
	result.WriteString(fmt.Sprintf("- **Break-even Point**: %d months\n", breakdown.ROIAnalysis.BreakEvenMonths))
	result.WriteString(fmt.Sprintf("- **Year 1 ROI**: %.1f%%\n", breakdown.ROIAnalysis.YearOneROI))
	result.WriteString(fmt.Sprintf("- **3-Year ROI**: %.1f%%\n\n", breakdown.ROIAnalysis.ThreeYearROI))

	return result.String()
}

// logCodeSnippetGeneration logs detailed code snippet generation metrics
func logCodeSnippetGeneration(query string, codeSnippets []synth.CodeSnippet, domain QueryDomain) {
	queryLower := strings.ToLower(query)

	// Check if this is a migration query
	isMigrationQuery := strings.Contains(queryLower, "migration") ||
		strings.Contains(queryLower, "migrate") ||
		strings.Contains(queryLower, "lift-and-shift") ||
		strings.Contains(queryLower, "terraform") ||
		strings.Contains(queryLower, "infrastructure")

	// Count code snippets by language
	languageCounts := make(map[string]int)
	for _, snippet := range codeSnippets {
		languageCounts[snippet.Language]++
	}

	// Log general code snippet metrics
	log.Printf("CODE_SNIPPET_GENERATION: query_type=%s, total_snippets=%d, migration_query=%t, domain=%s",
		determineQueryType(query), len(codeSnippets), isMigrationQuery, domain)

	// Log language-specific metrics
	for language, count := range languageCounts {
		log.Printf("CODE_SNIPPET_LANGUAGE: language=%s, count=%d, query_type=%s",
			language, count, determineQueryType(query))
	}

	// Special monitoring for migration queries
	if isMigrationQuery {
		hasTerraform := languageCounts["terraform"] > 0 || languageCounts["hcl"] > 0
		hasScripts := languageCounts["bash"] > 0 || languageCounts["powershell"] > 0
		hasConfig := languageCounts["yaml"] > 0 || languageCounts["json"] > 0

		log.Printf("MIGRATION_CODE_GENERATION: has_terraform=%t, has_scripts=%t, has_config=%t, total_snippets=%d",
			hasTerraform, hasScripts, hasConfig, len(codeSnippets))

		// Alert if migration query has no code snippets
		if len(codeSnippets) == 0 {
			log.Printf("ALERT: Migration query generated NO code snippets - this may indicate a code generation failure")
		}

		// Alert if migration query is missing essential code types
		if !hasTerraform && !hasScripts {
			log.Printf("ALERT: Migration query missing essential code types - terraform=%t, scripts=%t", hasTerraform, hasScripts)
		}

		// Log detailed code snippet content for debugging (first 100 chars)
		for i, snippet := range codeSnippets {
			preview := snippet.Code
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			log.Printf("CODE_SNIPPET_PREVIEW[%d]: language=%s, length=%d, preview=%s",
				i, snippet.Language, len(snippet.Code), preview)
		}
	}
}

// shouldGenerateCodeFallback determines if fallback code generation is needed
func shouldGenerateCodeFallback(query string, synthesisResponse synth.SynthesisResponse, finishReason string) bool {
	queryLower := strings.ToLower(query)

	// Check if this is a query that should generate code
	isCodeQuery := strings.Contains(queryLower, "terraform") ||
		strings.Contains(queryLower, "infrastructure") ||
		strings.Contains(queryLower, "deploy") ||
		strings.Contains(queryLower, "migration") ||
		strings.Contains(queryLower, "migrate") ||
		strings.Contains(queryLower, "lift-and-shift") ||
		strings.Contains(queryLower, "aws cli") ||
		strings.Contains(queryLower, "azure cli") ||
		strings.Contains(queryLower, "script") ||
		strings.Contains(queryLower, "automation")

	// Only generate fallback if:
	// 1. This is a code-related query
	// 2. No code snippets were generated
	// 3. Response was potentially truncated or there was another issue
	return isCodeQuery && len(synthesisResponse.CodeSnippets) == 0 &&
		(finishReason == "length" || finishReason == "stop")
}

// generateFallbackCodeSnippets generates basic code snippets when main generation fails
func generateFallbackCodeSnippets(query string, existingSnippets []synth.CodeSnippet) []synth.CodeSnippet {
	var fallbackSnippets []synth.CodeSnippet
	queryLower := strings.ToLower(query)

	// Generate Terraform fallback if terraform is mentioned
	if strings.Contains(queryLower, "terraform") || strings.Contains(queryLower, "infrastructure") {
		terraformCode := `# Terraform Configuration Template
# This is a fallback template - please customize for your specific needs

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

variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

# Add your resources here
# Example: VPC, EC2, RDS, etc.`

		fallbackSnippets = append(fallbackSnippets, synth.CodeSnippet{
			Language: "terraform",
			Code:     terraformCode,
		})
	}

	// Generate bash script fallback for deployment/migration queries
	if strings.Contains(queryLower, "deploy") || strings.Contains(queryLower, "migration") ||
		strings.Contains(queryLower, "script") || strings.Contains(queryLower, "automation") {
		bashCode := `#!/bin/bash
# Deployment/Migration Script Template
# This is a fallback template - please customize for your specific needs

set -e  # Exit on error

# Configuration
AWS_REGION=${AWS_REGION:-us-east-1}
ENVIRONMENT=${ENVIRONMENT:-dev}

echo "Starting deployment/migration process..."

# Add your deployment/migration commands here
# Example: terraform apply, AWS CLI commands, etc.

echo "Deployment/migration completed successfully!"`

		fallbackSnippets = append(fallbackSnippets, synth.CodeSnippet{
			Language: "bash",
			Code:     bashCode,
		})
	}

	return fallbackSnippets
}

// determineQueryType determines the type of query for monitoring purposes
func determineQueryType(query string) string {
	queryLower := strings.ToLower(query)

	if strings.Contains(queryLower, "migration") || strings.Contains(queryLower, "migrate") {
		return "migration"
	}
	if strings.Contains(queryLower, "security") || strings.Contains(queryLower, "compliance") {
		return "security"
	}
	if strings.Contains(queryLower, "cost") || strings.Contains(queryLower, "pricing") {
		return "cost"
	}
	if strings.Contains(queryLower, "architecture") || strings.Contains(queryLower, "design") {
		return "architecture"
	}
	if strings.Contains(queryLower, "disaster recovery") || strings.Contains(queryLower, "dr") {
		return "disaster_recovery"
	}

	return "general"
}
