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

// Package learning provides feedback-based learning and response improvement
package learning

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"go.uber.org/zap"
)

// FeedbackCategory represents different types of feedback
type FeedbackCategory int

const (
	// CategoryAccuracy represents feedback about response accuracy
	CategoryAccuracy FeedbackCategory = iota
	// CategoryCompleteness represents feedback about response completeness
	CategoryCompleteness
	// CategoryRelevance represents feedback about response relevance
	CategoryRelevance
	// CategoryOverall represents overall feedback
	CategoryOverall
)

// FeedbackType represents the sentiment of feedback
type FeedbackType int

const (
	// FeedbackNegative represents negative feedback
	FeedbackNegative FeedbackType = iota
	// FeedbackPositive represents positive feedback
	FeedbackPositive
	// FeedbackNeutral represents neutral feedback
	FeedbackNeutral
)

// DetailedFeedback represents enhanced feedback with categories and metadata
type DetailedFeedback struct {
	ID               string                   `json:"id"`
	Query            string                   `json:"query"`
	QueryType        string                   `json:"query_type"`
	Response         string                   `json:"response"`
	ResponseID       string                   `json:"response_id"`
	Categories       map[FeedbackCategory]int `json:"categories"`
	OverallFeedback  FeedbackType             `json:"overall_feedback"`
	Timestamp        time.Time                `json:"timestamp"`
	UserID           string                   `json:"user_id"`
	SessionID        string                   `json:"session_id"`
	ResponseTime     time.Duration            `json:"response_time"`
	SourcesUsed      []string                 `json:"sources_used"`
	RetrievalMetrics RetrievalMetrics         `json:"retrieval_metrics"`
	SynthesisMetrics SynthesisMetrics         `json:"synthesis_metrics"`
	Comments         string                   `json:"comments,omitempty"`
}

// RetrievalMetrics contains metrics about the retrieval process
type RetrievalMetrics struct {
	ChunksRetrieved    int     `json:"chunks_retrieved"`
	ConfidenceScore    float64 `json:"confidence_score"`
	FallbackTriggered  bool    `json:"fallback_triggered"`
	WebSearchTriggered bool    `json:"web_search_triggered"`
	AverageChunkScore  float64 `json:"average_chunk_score"`
}

// SynthesisMetrics contains metrics about the synthesis process
type SynthesisMetrics struct {
	TokensUsed       int     `json:"tokens_used"`
	Temperature      float64 `json:"temperature"`
	ModelUsed        string  `json:"model_used"`
	DiagramGenerated bool    `json:"diagram_generated"`
	CodeGenerated    bool    `json:"code_generated"`
}

// Insights contains insights derived from feedback analysis
type Insights struct {
	QueryPatterns        map[string]float64 `json:"query_patterns"`
	KnowledgeGaps        []KnowledgeGap     `json:"knowledge_gaps"`
	OptimalParameters    Parameters         `json:"optimal_parameters"`
	ResponseQualityTrend float64            `json:"response_quality_trend"`
	UpdatedAt            time.Time          `json:"updated_at"`
}

// KnowledgeGap represents areas where the system needs improvement
type KnowledgeGap struct {
	Topic            string   `json:"topic"`
	Severity         float64  `json:"severity"`
	NegativeFeedback int      `json:"negative_feedback_count"`
	SuggestedActions []string `json:"suggested_actions"`
}

// Parameters represents adaptive parameters for the system
type Parameters struct {
	RetrievalThreshold float64 `json:"retrieval_threshold"`
	FallbackThreshold  float64 `json:"fallback_threshold"`
	TemperatureAdjust  float64 `json:"temperature_adjust"`
	ChunkLimitAdjust   int     `json:"chunk_limit_adjust"`
	WebSearchThreshold float64 `json:"web_search_threshold"`
}

// Analytics handles feedback analysis and learning
type Analytics struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewAnalytics creates a new analytics instance
func NewAnalytics(db *sql.DB, logger *zap.Logger) *Analytics {
	return &Analytics{
		db:     db,
		logger: logger,
	}
}

// AnalyzeFeedbackPatterns analyzes feedback patterns to generate insights
func (a *Analytics) AnalyzeFeedbackPatterns(days int) (*Insights, error) {
	// Get feedback from the last N days
	feedback, err := a.getRecentFeedback(days)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent feedback: %w", err)
	}

	if len(feedback) == 0 {
		return &Insights{
			QueryPatterns:        make(map[string]float64),
			KnowledgeGaps:        []KnowledgeGap{},
			OptimalParameters:    a.getDefaultParameters(),
			ResponseQualityTrend: 0.0,
			UpdatedAt:            time.Now(),
		}, nil
	}

	insights := &Insights{
		QueryPatterns:        a.analyzeQueryPatterns(feedback),
		KnowledgeGaps:        a.identifyKnowledgeGaps(feedback),
		OptimalParameters:    a.optimizeParameters(feedback),
		ResponseQualityTrend: a.calculateQualityTrend(feedback),
		UpdatedAt:            time.Now(),
	}

	return insights, nil
}

// getRecentFeedback retrieves feedback from the last N days
func (a *Analytics) getRecentFeedback(days int) ([]DetailedFeedback, error) {
	query := `
		SELECT id, query, feedback, timestamp, user_id, session_id
		FROM feedback
		WHERE timestamp >= datetime('now', '-' || ? || ' days')
		ORDER BY timestamp DESC
	`

	rows, err := a.db.Query(query, days)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent feedback: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			// Log error but don't return it as this is in a defer
			fmt.Printf("failed to close rows: %v\n", closeErr)
		}
	}()

	var feedback []DetailedFeedback
	for rows.Next() {
		var f DetailedFeedback
		var userID, sessionID sql.NullString
		var feedbackStr string

		err := rows.Scan(&f.ID, &f.Query, &feedbackStr, &f.Timestamp, &userID, &sessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to scan feedback: %w", err)
		}

		if userID.Valid {
			f.UserID = userID.String
		}
		if sessionID.Valid {
			f.SessionID = sessionID.String
		}

		// Parse feedback type
		f.OverallFeedback = parseFeedbackType(feedbackStr)
		f.QueryType = classifyQuery(f.Query)

		feedback = append(feedback, f)
	}

	return feedback, nil
}

// analyzeQueryPatterns identifies patterns in user queries
func (a *Analytics) analyzeQueryPatterns(feedback []DetailedFeedback) map[string]float64 {
	patterns := make(map[string]float64)
	positivePatterns := make(map[string]int)
	totalPatterns := make(map[string]int)

	for _, f := range feedback {
		queryType := f.QueryType
		totalPatterns[queryType]++

		if f.OverallFeedback == FeedbackPositive {
			positivePatterns[queryType]++
		}
	}

	for queryType, total := range totalPatterns {
		if total > 0 {
			patterns[queryType] = float64(positivePatterns[queryType]) / float64(total)
		}
	}

	return patterns
}

// identifyKnowledgeGaps finds areas with consistently poor feedback
func (a *Analytics) identifyKnowledgeGaps(feedback []DetailedFeedback) []KnowledgeGap {
	gaps := make(map[string]*KnowledgeGap)

	for _, f := range feedback {
		if f.OverallFeedback == FeedbackNegative {
			topic := f.QueryType
			if gap, exists := gaps[topic]; exists {
				gap.NegativeFeedback++
				gap.Severity = math.Min(1.0, gap.Severity+0.1)
			} else {
				gaps[topic] = &KnowledgeGap{
					Topic:            topic,
					Severity:         0.3,
					NegativeFeedback: 1,
					SuggestedActions: generateSuggestedActions(topic),
				}
			}
		}
	}

	var result []KnowledgeGap
	for _, gap := range gaps {
		if gap.NegativeFeedback >= 2 { // Only include significant gaps
			result = append(result, *gap)
		}
	}

	return result
}

// optimizeParameters adjusts system parameters based on feedback
func (a *Analytics) optimizeParameters(feedback []DetailedFeedback) Parameters {
	params := a.getDefaultParameters()

	// Analyze feedback to adjust parameters
	positiveCount := 0
	negativeCount := 0

	for _, f := range feedback {
		if f.OverallFeedback == FeedbackPositive {
			positiveCount++
		} else if f.OverallFeedback == FeedbackNegative {
			negativeCount++
		}
	}

	total := positiveCount + negativeCount
	if total == 0 {
		return params
	}

	satisfactionRate := float64(positiveCount) / float64(total)

	// Adjust parameters based on satisfaction rate
	if satisfactionRate < 0.6 {
		// Low satisfaction - be more conservative
		params.RetrievalThreshold = math.Max(0.3, params.RetrievalThreshold-0.05)
		params.FallbackThreshold = math.Max(0.2, params.FallbackThreshold-0.05)
		params.ChunkLimitAdjust = 2
	} else if satisfactionRate > 0.8 {
		// High satisfaction - be more aggressive
		params.RetrievalThreshold = math.Min(0.9, params.RetrievalThreshold+0.05)
		params.FallbackThreshold = math.Min(0.8, params.FallbackThreshold+0.05)
		params.ChunkLimitAdjust = -1
	}

	return params
}

// calculateQualityTrend calculates the trend in response quality
func (a *Analytics) calculateQualityTrend(feedback []DetailedFeedback) float64 {
	if len(feedback) < 2 {
		return 0.0
	}

	// Calculate quality scores for first and second half
	// Assuming feedback is ordered newest first, split into recent (first half) and older (second half)
	half := len(feedback) / 2
	recentHalf := feedback[:half] // newer feedback
	olderHalf := feedback[half:]  // older feedback

	recentScore := a.calculateAverageQuality(recentHalf)
	olderScore := a.calculateAverageQuality(olderHalf)

	return recentScore - olderScore
}

// calculateAverageQuality calculates average quality score for feedback
func (a *Analytics) calculateAverageQuality(feedback []DetailedFeedback) float64 {
	if len(feedback) == 0 {
		return 0.0
	}

	total := 0.0
	for _, f := range feedback {
		switch f.OverallFeedback {
		case FeedbackPositive:
			total += 1.0
		case FeedbackNegative:
			total += -1.0
		case FeedbackNeutral:
			total += 0.0
		}
	}

	return total / float64(len(feedback))
}

// getDefaultParameters returns default system parameters
func (a *Analytics) getDefaultParameters() Parameters {
	return Parameters{
		RetrievalThreshold: 0.7,
		FallbackThreshold:  0.5,
		TemperatureAdjust:  0.0,
		ChunkLimitAdjust:   0,
		WebSearchThreshold: 0.6,
	}
}

// parseFeedbackType converts feedback string to FeedbackType
func parseFeedbackType(feedback string) FeedbackType {
	switch strings.ToLower(feedback) {
	case "positive", "👍", "good", "helpful":
		return FeedbackPositive
	case "negative", "👎", "bad", "unhelpful":
		return FeedbackNegative
	default:
		return FeedbackNeutral
	}
}

// classifyQuery classifies query into a type
func classifyQuery(query string) string {
	queryLower := strings.ToLower(query)

	if strings.Contains(queryLower, "migrate") || strings.Contains(queryLower, "migration") {
		return "migration"
	}
	if strings.Contains(queryLower, "security") || strings.Contains(queryLower, "compliance") ||
		strings.Contains(queryLower, "gdpr") || strings.Contains(queryLower, "hipaa") ||
		strings.Contains(queryLower, "encryption") || strings.Contains(queryLower, "audit") {
		return "security"
	}
	if strings.Contains(queryLower, "hybrid") || strings.Contains(queryLower, "connect") {
		return "hybrid"
	}
	if strings.Contains(queryLower, "disaster") || strings.Contains(queryLower, "recovery") || strings.Contains(queryLower, "dr ") {
		return "disaster-recovery"
	}
	if strings.Contains(queryLower, "cost") || strings.Contains(queryLower, "pricing") {
		return "cost-optimization"
	}

	return "general"
}

// generateSuggestedActions generates suggested actions for knowledge gaps
func generateSuggestedActions(topic string) []string {
	actions := map[string][]string{
		"migration": {
			"Add more migration case studies and best practices",
			"Include recent migration tooling updates",
			"Expand on migration troubleshooting guides",
		},
		"security": {
			"Update security compliance documentation",
			"Add security best practices examples",
			"Include latest security feature updates",
		},
		"hybrid": {
			"Expand hybrid connectivity patterns",
			"Add hybrid networking troubleshooting",
			"Include hybrid management best practices",
		},
		"disaster-recovery": {
			"Add more DR scenarios and solutions",
			"Include DR testing procedures",
			"Expand on RTO/RPO optimization",
		},
		"cost-optimization": {
			"Add cost optimization strategies",
			"Include pricing calculator examples",
			"Expand on cost monitoring best practices",
		},
	}

	if suggestions, exists := actions[topic]; exists {
		return suggestions
	}

	return []string{
		"Review and update documentation for this topic",
		"Add more examples and use cases",
		"Consider adding expert consultation",
	}
}

// StoreInsights stores learning insights in the database
func (a *Analytics) StoreInsights(insights *Insights) error {
	insightsJSON, err := json.Marshal(insights)
	if err != nil {
		return fmt.Errorf("failed to marshal insights: %w", err)
	}

	query := `
		INSERT OR REPLACE INTO learning_insights (id, insights, updated_at)
		VALUES ('latest', ?, ?)
	`

	_, err = a.db.Exec(query, string(insightsJSON), insights.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to store learning insights: %w", err)
	}

	a.logger.Info("Learning insights stored successfully")
	return nil
}

// GetInsights retrieves the latest learning insights
func (a *Analytics) GetInsights() (*Insights, error) {
	query := `
		SELECT insights, updated_at
		FROM learning_insights
		WHERE id = 'latest'
	`

	var insightsJSON string
	var updatedAt time.Time

	err := a.db.QueryRow(query).Scan(&insightsJSON, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			// Return default insights if none found
			return &Insights{
				QueryPatterns:        make(map[string]float64),
				KnowledgeGaps:        []KnowledgeGap{},
				OptimalParameters:    a.getDefaultParameters(),
				ResponseQualityTrend: 0.0,
				UpdatedAt:            time.Now(),
			}, nil
		}
		return nil, fmt.Errorf("failed to query learning insights: %w", err)
	}

	var insights Insights
	err = json.Unmarshal([]byte(insightsJSON), &insights)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal insights: %w", err)
	}

	return &insights, nil
}

// InitializeLearningTables creates the necessary database tables
func (a *Analytics) InitializeLearningTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS learning_insights (
			id TEXT PRIMARY KEY,
			insights TEXT NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS detailed_feedback (
			id TEXT PRIMARY KEY,
			query TEXT NOT NULL,
			query_type TEXT NOT NULL,
			response TEXT,
			response_id TEXT,
			categories TEXT,
			overall_feedback INTEGER,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			user_id TEXT,
			session_id TEXT,
			response_time INTEGER,
			sources_used TEXT,
			retrieval_metrics TEXT,
			synthesis_metrics TEXT,
			comments TEXT
		)`,
	}

	for _, query := range queries {
		_, err := a.db.Exec(query)
		if err != nil {
			return fmt.Errorf("failed to create learning table: %w", err)
		}
	}

	return nil
}
