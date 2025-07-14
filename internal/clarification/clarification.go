// Package clarification provides query analysis and clarification request generation
// for the AI-powered Solutions Architect assistant.
package clarification

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/your-org/ai-sa-assistant/internal/session"
)

// QueryAnalysis represents the analysis of a user query
type QueryAnalysis struct {
	Query                 string           `json:"query"`
	IsAmbiguous           bool             `json:"is_ambiguous"`
	IsIncomplete          bool             `json:"is_incomplete"`
	CompletenessScore     float64          `json:"completeness_score"`
	AmbiguityScore        float64          `json:"ambiguity_score"`
	RequiresClarification bool             `json:"requires_clarification"`
	DetectedIntents       []string         `json:"detected_intents"`
	MissingContext        []string         `json:"missing_context"`
	ClarificationNeeded   []Area           `json:"clarification_needed"`
	FollowupContext       *FollowupContext `json:"followup_context,omitempty"`
}

// Area represents a specific area that needs clarification
type Area struct {
	Area        string   `json:"area"`
	Questions   []string `json:"questions"`
	Suggestions []string `json:"suggestions"`
	Priority    string   `json:"priority"` // "high", "medium", "low"
}

// FollowupContext represents context for follow-up questions
type FollowupContext struct {
	ReferencesFound    []string `json:"references_found"`
	PreviousResponse   string   `json:"previous_response,omitempty"`
	ContextElements    []string `json:"context_elements"`
	ResolutionStrategy string   `json:"resolution_strategy"`
}

// Request represents a request for clarification
type Request struct {
	OriginalQuery   string           `json:"original_query"`
	Analysis        QueryAnalysis    `json:"analysis"`
	Questions       []Item           `json:"questions"`
	Suggestions     []string         `json:"suggestions"`
	QuickOptions    []QuickOption    `json:"quick_options"`
	GuidedTemplates []GuidedTemplate `json:"guided_templates"`
}

// Item represents a single clarification question
type Item struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Type     string   `json:"type"` // "choice", "text", "guided"
	Options  []string `json:"options,omitempty"`
	Category string   `json:"category"`
	Priority string   `json:"priority"`
	HelpText string   `json:"help_text,omitempty"`
	Examples []string `json:"examples,omitempty"`
}

// QuickOption represents a quick clarification option
type QuickOption struct {
	ID          string `json:"id"`
	Text        string `json:"text"`
	Category    string `json:"category"`
	QuerySuffix string `json:"query_suffix"`
}

// GuidedTemplate represents a guided question template
type GuidedTemplate struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Template    string   `json:"template"`
	Fields      []string `json:"fields"`
	Examples    []string `json:"examples"`
}

// Analyzer provides query analysis and clarification functionality
type Analyzer struct {
	ambiguityPatterns   []*regexp.Regexp
	incompletenessRules []incompletenessRule
	intentClassifiers   []intentClassifier
	clarificationRules  []clarificationRule
	followupDetectors   []followupDetector
}

// incompletenessRule defines a rule for detecting incomplete queries
type incompletenessRule struct {
	Pattern     *regexp.Regexp
	Weight      float64
	Description string
	Category    string
}

// intentClassifier defines a classifier for detecting query intent
type intentClassifier struct {
	Intent   string
	Patterns []*regexp.Regexp
	Keywords []string
	Weight   float64
}

// clarificationRule defines a rule for generating clarifications
type clarificationRule struct {
	Trigger     func(QueryAnalysis) bool
	Generator   func(QueryAnalysis) []Area
	Priority    string
	Description string
}

// followupDetector defines a detector for follow-up questions
type followupDetector struct {
	Pattern     *regexp.Regexp
	Type        string
	Resolver    func(string, []session.Message) *FollowupContext
	Description string
}

// NewAnalyzer creates a new query analyzer
func NewAnalyzer() *Analyzer {
	analyzer := &Analyzer{
		ambiguityPatterns:   buildAmbiguityPatterns(),
		incompletenessRules: buildIncompletenessRules(),
		intentClassifiers:   buildIntentClassifiers(),
		clarificationRules:  buildClarificationRules(),
		followupDetectors:   buildFollowupDetectors(),
	}
	return analyzer
}

// AnalyzeQuery analyzes a query for ambiguity and completeness
func (a *Analyzer) AnalyzeQuery(_ context.Context, query string, conversationHistory []session.Message) (*QueryAnalysis, error) {
	analysis := &QueryAnalysis{
		Query:               query,
		DetectedIntents:     []string{},
		MissingContext:      []string{},
		ClarificationNeeded: []Area{},
	}

	// Check for follow-up context
	if len(conversationHistory) > 0 {
		analysis.FollowupContext = a.detectFollowupContext(query, conversationHistory)
	}

	// Analyze ambiguity
	const ambiguityThreshold = 0.5
	analysis.AmbiguityScore = a.calculateAmbiguityScore(query)
	analysis.IsAmbiguous = analysis.AmbiguityScore > ambiguityThreshold

	// Analyze completeness
	const completenessThreshold = 0.5
	analysis.CompletenessScore = a.calculateCompletenessScore(query)
	analysis.IsIncomplete = analysis.CompletenessScore < completenessThreshold

	// Detect intents
	analysis.DetectedIntents = a.detectIntents(query)

	// Identify missing context
	analysis.MissingContext = a.identifyMissingContext(query, analysis.DetectedIntents)

	// Generate clarification areas
	analysis.ClarificationNeeded = a.generateAreas(analysis)

	// Determine if clarification is required
	analysis.RequiresClarification = analysis.IsAmbiguous || analysis.IsIncomplete || len(analysis.ClarificationNeeded) > 0

	return analysis, nil
}

// GenerateClarificationRequest generates a clarification request
func (a *Analyzer) GenerateClarificationRequest(_ context.Context, analysis *QueryAnalysis) (*Request, error) {
	request := &Request{
		OriginalQuery:   analysis.Query,
		Analysis:        *analysis,
		Questions:       []Item{},
		Suggestions:     []string{},
		QuickOptions:    []QuickOption{},
		GuidedTemplates: []GuidedTemplate{},
	}

	// Generate clarification questions
	request.Questions = a.generateClarificationQuestions(analysis)

	// Generate suggestions
	request.Suggestions = a.generateSuggestions(analysis)

	// Generate quick options
	request.QuickOptions = a.generateQuickOptions(analysis)

	// Generate guided templates
	request.GuidedTemplates = a.generateGuidedTemplates(analysis)

	return request, nil
}

// calculateAmbiguityScore calculates how ambiguous a query is
func (a *Analyzer) calculateAmbiguityScore(query string) float64 {
	var score float64
	queryLower := strings.ToLower(query)

	// Check against ambiguity patterns
	for _, pattern := range a.ambiguityPatterns {
		if pattern.MatchString(queryLower) {
			score += 0.4
		}
	}

	// Check for vague terms
	vagueTerms := []string{"best", "good", "better", "simple", "easy", "quick", "basic", "general", "overview"}
	for _, term := range vagueTerms {
		if strings.Contains(queryLower, term) {
			score += 0.3
		}
	}

	// Check for lack of specificity
	const (
		minWordCountThreshold    = 3
		mediumWordCountThreshold = 5
		veryShortQueryPenalty    = 0.5
		shortQueryPenalty        = 0.2
		questionWordPenalty      = 0.3
	)
	wordCount := len(strings.Fields(query))
	if wordCount < minWordCountThreshold {
		score += veryShortQueryPenalty
	} else if wordCount < mediumWordCountThreshold {
		score += shortQueryPenalty
	}

	// Check for question words without specific context
	questionWords := []string{"help", "how", "what", "which", "when", "where", "why"}
	for _, word := range questionWords {
		if strings.Contains(queryLower, word) && !a.hasSpecificContext(queryLower) {
			score += questionWordPenalty
		}
	}

	const maxScore = 1.0
	return minFloat64(score, maxScore)
}

// calculateCompletenessScore calculates how complete a query is
func (a *Analyzer) calculateCompletenessScore(query string) float64 {
	score := 1.0
	queryLower := strings.ToLower(query)

	// Apply incompleteness rules
	for _, rule := range a.incompletenessRules {
		if rule.Pattern.MatchString(queryLower) {
			score -= rule.Weight
		}
	}

	// Adjust for query length and detail
	wordCount := len(strings.Fields(query))
	if wordCount < 3 {
		score -= 0.6
	} else if wordCount < 5 {
		score -= 0.4
	} else if wordCount < 7 {
		score -= 0.2
	}

	// Boost score if query has specific context
	if a.hasSpecificContext(queryLower) {
		score += 0.2
	}

	return minFloat64(maxFloat64(score, 0.0), 1.0)
}

// detectIntents detects the intents in a query
func (a *Analyzer) detectIntents(query string) []string {
	var intents []string
	queryLower := strings.ToLower(query)

	for _, classifier := range a.intentClassifiers {
		matched := false

		// Check patterns
		for _, pattern := range classifier.Patterns {
			if pattern.MatchString(queryLower) {
				matched = true
				break
			}
		}

		// Check keywords if no pattern matched
		if !matched {
			for _, keyword := range classifier.Keywords {
				if strings.Contains(queryLower, keyword) {
					matched = true
					break
				}
			}
		}

		if matched {
			intents = append(intents, classifier.Intent)
		}
	}

	return intents
}

// identifyMissingContext identifies what context is missing from the query
func (a *Analyzer) identifyMissingContext(query string, intents []string) []string {
	var missing []string
	queryLower := strings.ToLower(query)

	// Context requirements by intent
	contextRequirements := map[string][]string{
		"migration":         {"source_environment", "target_cloud", "workload_type", "timeline"},
		"security":          {"compliance_standards", "data_type", "environment", "threat_model"},
		"architecture":      {"scale", "requirements", "constraints", "environment"},
		"disaster_recovery": {"rto", "rpo", "criticality", "budget"},
		"cost_optimization": {"current_spend", "workloads", "optimization_goals"},
	}

	for _, intent := range intents {
		if requirements, exists := contextRequirements[intent]; exists {
			for _, requirement := range requirements {
				if !a.hasContext(queryLower, requirement) {
					missing = append(missing, requirement)
				}
			}
		}
	}

	return missing
}

// hasContext checks if the query contains context for a specific requirement
func (a *Analyzer) hasContext(query, requirement string) bool {
	contextPatterns := map[string][]string{
		"source_environment":   {"on-premises", "on-prem", "datacenter", "vmware", "hyper-v"},
		"target_cloud":         {"aws", "azure", "gcp", "google cloud", "cloud"},
		"workload_type":        {"database", "web", "application", "vm", "container"},
		"timeline":             {"months", "weeks", "days", "urgent", "asap", "deadline"},
		"compliance_standards": {"hipaa", "gdpr", "sox", "pci", "compliance"},
		"data_type":            {"pii", "sensitive", "confidential", "public", "personal"},
		"environment":          {"production", "staging", "development", "test", "prod"},
		"threat_model":         {"external", "internal", "insider", "threat", "risk"},
		"scale":                {"users", "concurrent", "load", "traffic", "scale"},
		"requirements":         {"performance", "availability", "reliability", "sla"},
		"constraints":          {"budget", "time", "resource", "limitation", "restrict"},
		"rto":                  {"rto", "recovery time", "downtime", "availability"},
		"rpo":                  {"rpo", "recovery point", "data loss", "backup"},
		"criticality":          {"critical", "important", "priority", "business impact"},
		"budget":               {"budget", "cost", "price", "spend", "dollar"},
		"current_spend":        {"current", "existing", "monthly", "annual", "spending"},
		"optimization_goals":   {"save", "reduce", "optimize", "efficiency", "cost"},
	}

	if patterns, exists := contextPatterns[requirement]; exists {
		for _, pattern := range patterns {
			if strings.Contains(query, pattern) {
				return true
			}
		}
	}

	return false
}

// hasSpecificContext checks if query contains specific contextual information
func (a *Analyzer) hasSpecificContext(queryLower string) bool {
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

// generateAreas generates clarification areas based on analysis
func (a *Analyzer) generateAreas(analysis *QueryAnalysis) []Area {
	var areas []Area

	// Apply clarification rules
	for _, rule := range a.clarificationRules {
		if rule.Trigger(*analysis) {
			generatedAreas := rule.Generator(*analysis)
			areas = append(areas, generatedAreas...)
		}
	}

	return areas
}

// detectFollowupContext detects if this is a follow-up question
func (a *Analyzer) detectFollowupContext(query string, conversationHistory []session.Message) *FollowupContext {
	for _, detector := range a.followupDetectors {
		if detector.Pattern.MatchString(strings.ToLower(query)) {
			return detector.Resolver(query, conversationHistory)
		}
	}
	return nil
}

// generateClarificationQuestions generates clarification questions
func (a *Analyzer) generateClarificationQuestions(analysis *QueryAnalysis) []Item {
	var questions []Item
	id := 1

	for _, area := range analysis.ClarificationNeeded {
		for _, question := range area.Questions {
			item := Item{
				ID:       fmt.Sprintf("clarify_%d", id),
				Question: question,
				Type:     "choice",
				Category: area.Area,
				Priority: area.Priority,
			}

			// Add options for specific categories
			if area.Area == "cloud_provider" {
				item.Options = []string{"AWS", "Azure", "Google Cloud", "Multi-cloud", "Not sure"}
			} else if area.Area == "workload_type" {
				item.Options = []string{"Web applications", "Databases", "File storage", "Analytics", "Mixed workloads"}
			} else if area.Area == "environment" {
				item.Options = []string{"Production", "Development", "Staging", "All environments"}
			} else if area.Area == "compliance" {
				item.Options = []string{"HIPAA", "GDPR", "SOX", "PCI-DSS", "Other", "Not applicable"}
			}

			questions = append(questions, item)
			id++
		}
	}

	return questions
}

// generateSuggestions generates helpful suggestions
func (a *Analyzer) generateSuggestions(analysis *QueryAnalysis) []string {
	var suggestions []string

	if analysis.IsAmbiguous {
		suggestions = append(suggestions, "Try to be more specific about your requirements")
		suggestions = append(suggestions, "Include details about your current environment")
	}

	if analysis.IsIncomplete {
		suggestions = append(suggestions, "Provide more context about your use case")
		suggestions = append(suggestions, "Specify your goals and constraints")
	}

	for _, intent := range analysis.DetectedIntents {
		switch intent {
		case "migration":
			suggestions = append(suggestions, "Specify source and target environments")
			suggestions = append(suggestions, "Include timeline and scale requirements")
		case "security":
			suggestions = append(suggestions, "Mention compliance requirements")
			suggestions = append(suggestions, "Describe your security concerns")
		case "architecture":
			suggestions = append(suggestions, "Define performance and scale requirements")
			suggestions = append(suggestions, "Specify architectural constraints")
		}
	}

	return suggestions
}

// generateQuickOptions generates quick clarification options
func (a *Analyzer) generateQuickOptions(analysis *QueryAnalysis) []QuickOption {
	var options []QuickOption

	for _, intent := range analysis.DetectedIntents {
		switch intent {
		case "migration":
			options = append(options, QuickOption{
				ID:          "migration_aws",
				Text:        "AWS Migration",
				Category:    "cloud_provider",
				QuerySuffix: " to AWS",
			})
			options = append(options, QuickOption{
				ID:          "migration_azure",
				Text:        "Azure Migration",
				Category:    "cloud_provider",
				QuerySuffix: " to Azure",
			})
		case "security":
			options = append(options, QuickOption{
				ID:          "security_compliance",
				Text:        "With Compliance",
				Category:    "compliance",
				QuerySuffix: " with compliance requirements",
			})
			options = append(options, QuickOption{
				ID:          "security_assessment",
				Text:        "Security Assessment",
				Category:    "assessment",
				QuerySuffix: " and security assessment",
			})
		}
	}

	return options
}

// generateGuidedTemplates generates guided question templates
func (a *Analyzer) generateGuidedTemplates(analysis *QueryAnalysis) []GuidedTemplate {
	var templates []GuidedTemplate

	for _, intent := range analysis.DetectedIntents {
		switch intent {
		case "migration":
			templates = append(templates, GuidedTemplate{
				ID:          "migration_template",
				Name:        "Migration Planning",
				Description: "Get a comprehensive migration plan",
				Template:    "Help me migrate {workload_type} from {source_env} to {target_cloud} with {timeline} timeline",
				Fields:      []string{"workload_type", "source_env", "target_cloud", "timeline"},
				Examples:    []string{"Help me migrate web applications from on-premises to AWS with 6-month timeline"},
			})
		case "security":
			templates = append(templates, GuidedTemplate{
				ID:          "security_template",
				Name:        "Security Planning",
				Description: "Get a security assessment and plan",
				Template:    "Create a {security_type} plan for {environment} with {compliance} compliance",
				Fields:      []string{"security_type", "environment", "compliance"},
				Examples:    []string{"Create a data protection plan for production with HIPAA compliance"},
			})
		}
	}

	return templates
}

// Helper functions
func minFloat64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
