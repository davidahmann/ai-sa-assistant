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

// Package feedback provides enhanced feedback collection with detailed categories
package feedback

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/your-org/ai-sa-assistant/internal/learning"
	"go.uber.org/zap"
)

// DetailedFeedbackCollector extends the basic feedback logger with detailed collection
type DetailedFeedbackCollector struct {
	logger *Logger
	db     *sql.DB
	zaplog *zap.Logger
}

// NewDetailedFeedbackCollector creates a new detailed feedback collector
func NewDetailedFeedbackCollector(logger *Logger, db *sql.DB, zaplog *zap.Logger) *DetailedFeedbackCollector {
	return &DetailedFeedbackCollector{
		logger: logger,
		db:     db,
		zaplog: zaplog,
	}
}

// CollectDetailedFeedback collects detailed feedback with categories and metadata
func (dfc *DetailedFeedbackCollector) CollectDetailedFeedback(feedback *learning.DetailedFeedback) error {
	// Store in detailed feedback table
	if err := dfc.storeDetailedFeedback(feedback); err != nil {
		return fmt.Errorf("failed to store detailed feedback: %w", err)
	}

	// Also store in the basic feedback table for backward compatibility
	basicFeedback := convertToBasicFeedback(feedback)
	if err := dfc.logger.LogFeedbackWithContext(
		basicFeedback,
		fmt.Sprintf("%d", feedback.OverallFeedback),
		feedback.UserID,
		feedback.SessionID,
	); err != nil {
		dfc.zaplog.Warn("Failed to store basic feedback", zap.Error(err))
	}

	dfc.zaplog.Info("Detailed feedback collected",
		zap.String("id", feedback.ID),
		zap.String("query_type", feedback.QueryType),
		zap.String("user_id", feedback.UserID),
		zap.Any("overall_feedback", feedback.OverallFeedback))

	return nil
}

// storeDetailedFeedback stores detailed feedback in the database
func (dfc *DetailedFeedbackCollector) storeDetailedFeedback(feedback *learning.DetailedFeedback) error {
	// Convert complex fields to JSON
	categoriesJSON, err := json.Marshal(feedback.Categories)
	if err != nil {
		return fmt.Errorf("failed to marshal categories: %w", err)
	}

	sourcesJSON, err := json.Marshal(feedback.SourcesUsed)
	if err != nil {
		return fmt.Errorf("failed to marshal sources: %w", err)
	}

	retrievalMetricsJSON, err := json.Marshal(feedback.RetrievalMetrics)
	if err != nil {
		return fmt.Errorf("failed to marshal retrieval metrics: %w", err)
	}

	synthesisMetricsJSON, err := json.Marshal(feedback.SynthesisMetrics)
	if err != nil {
		return fmt.Errorf("failed to marshal synthesis metrics: %w", err)
	}

	insertSQL := `
		INSERT OR REPLACE INTO detailed_feedback (
			id, query, query_type, response, response_id, categories,
			overall_feedback, timestamp, user_id, session_id,
			response_time, sources_used, retrieval_metrics,
			synthesis_metrics, comments
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = dfc.db.Exec(insertSQL,
		feedback.ID,
		feedback.Query,
		feedback.QueryType,
		feedback.Response,
		feedback.ResponseID,
		string(categoriesJSON),
		int(feedback.OverallFeedback),
		feedback.Timestamp,
		feedback.UserID,
		feedback.SessionID,
		int64(feedback.ResponseTime),
		string(sourcesJSON),
		string(retrievalMetricsJSON),
		string(synthesisMetricsJSON),
		feedback.Comments,
	)

	if err != nil {
		return fmt.Errorf("failed to insert detailed feedback: %w", err)
	}

	return nil
}

// GetDetailedFeedback retrieves detailed feedback from the database
func (dfc *DetailedFeedbackCollector) GetDetailedFeedback(limit int) ([]*learning.DetailedFeedback, error) {
	query := `
		SELECT id, query, query_type, response, response_id, categories,
			   overall_feedback, timestamp, user_id, session_id,
			   response_time, sources_used, retrieval_metrics,
			   synthesis_metrics, comments
		FROM detailed_feedback
		ORDER BY timestamp DESC
		LIMIT ?
	`

	rows, err := dfc.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query detailed feedback: %w", err)
	}
	defer rows.Close()

	var feedbacks []*learning.DetailedFeedback
	for rows.Next() {
		feedback := &learning.DetailedFeedback{}
		var categoriesJSON, sourcesJSON, retrievalMetricsJSON, synthesisMetricsJSON string
		var overallFeedback int
		var responseTime int64
		var userID, sessionID, response, responseID, comments sql.NullString

		err := rows.Scan(
			&feedback.ID,
			&feedback.Query,
			&feedback.QueryType,
			&response,
			&responseID,
			&categoriesJSON,
			&overallFeedback,
			&feedback.Timestamp,
			&userID,
			&sessionID,
			&responseTime,
			&sourcesJSON,
			&retrievalMetricsJSON,
			&synthesisMetricsJSON,
			&comments,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan detailed feedback: %w", err)
		}

		// Convert JSON fields back to structs
		if err := json.Unmarshal([]byte(categoriesJSON), &feedback.Categories); err != nil {
			dfc.zaplog.Warn("Failed to unmarshal categories", zap.Error(err))
			feedback.Categories = make(map[learning.FeedbackCategory]int)
		}

		if err := json.Unmarshal([]byte(sourcesJSON), &feedback.SourcesUsed); err != nil {
			dfc.zaplog.Warn("Failed to unmarshal sources", zap.Error(err))
			feedback.SourcesUsed = []string{}
		}

		if err := json.Unmarshal([]byte(retrievalMetricsJSON), &feedback.RetrievalMetrics); err != nil {
			dfc.zaplog.Warn("Failed to unmarshal retrieval metrics", zap.Error(err))
		}

		if err := json.Unmarshal([]byte(synthesisMetricsJSON), &feedback.SynthesisMetrics); err != nil {
			dfc.zaplog.Warn("Failed to unmarshal synthesis metrics", zap.Error(err))
		}

		// Set optional fields
		feedback.OverallFeedback = learning.FeedbackType(overallFeedback)
		feedback.ResponseTime = time.Duration(responseTime)

		if userID.Valid {
			feedback.UserID = userID.String
		}
		if sessionID.Valid {
			feedback.SessionID = sessionID.String
		}
		if response.Valid {
			feedback.Response = response.String
		}
		if responseID.Valid {
			feedback.ResponseID = responseID.String
		}
		if comments.Valid {
			feedback.Comments = comments.String
		}

		feedbacks = append(feedbacks, feedback)
	}

	return feedbacks, nil
}

// CreateDetailedFeedback creates a detailed feedback object with metadata
func (dfc *DetailedFeedbackCollector) CreateDetailedFeedback(
	query, queryType, response, responseID string,
	overallFeedback learning.FeedbackType,
	userID, sessionID string,
	responseTime time.Duration,
	sourcesUsed []string,
	retrievalMetrics learning.RetrievalMetrics,
	synthesisMetrics learning.SynthesisMetrics,
	comments string,
) *learning.DetailedFeedback {
	return &learning.DetailedFeedback{
		ID:               generateDetailedFeedbackID(),
		Query:            query,
		QueryType:        queryType,
		Response:         response,
		ResponseID:       responseID,
		Categories:       make(map[learning.FeedbackCategory]int),
		OverallFeedback:  overallFeedback,
		Timestamp:        time.Now(),
		UserID:           userID,
		SessionID:        sessionID,
		ResponseTime:     responseTime,
		SourcesUsed:      sourcesUsed,
		RetrievalMetrics: retrievalMetrics,
		SynthesisMetrics: synthesisMetrics,
		Comments:         comments,
	}
}

// convertToBasicFeedback converts detailed feedback to basic format
func convertToBasicFeedback(detailed *learning.DetailedFeedback) string {
	return detailed.Query
}

// generateDetailedFeedbackID generates a unique ID for detailed feedback
func generateDetailedFeedbackID() string {
	return fmt.Sprintf("detailed_feedback_%d", time.Now().UnixNano())
}

// GetFeedbackStats returns enhanced feedback statistics
func (dfc *DetailedFeedbackCollector) GetFeedbackStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Basic stats from original logger
	basicStats, err := dfc.logger.GetFeedbackStats()
	if err != nil {
		return nil, fmt.Errorf("failed to get basic stats: %w", err)
	}
	stats["basic"] = basicStats

	// Detailed stats
	detailedStats, err := dfc.getDetailedStats()
	if err != nil {
		return nil, fmt.Errorf("failed to get detailed stats: %w", err)
	}
	stats["detailed"] = detailedStats

	return stats, nil
}

// getDetailedStats returns detailed feedback statistics
func (dfc *DetailedFeedbackCollector) getDetailedStats() (map[string]interface{}, error) {
	query := `
		SELECT 
			query_type,
			overall_feedback,
			COUNT(*) as count,
			AVG(response_time) as avg_response_time
		FROM detailed_feedback
		WHERE timestamp >= datetime('now', '-30 days')
		GROUP BY query_type, overall_feedback
		ORDER BY query_type, overall_feedback
	`

	rows, err := dfc.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query detailed stats: %w", err)
	}
	defer rows.Close()

	stats := make(map[string]interface{})
	queryTypeStats := make(map[string]map[string]interface{})

	for rows.Next() {
		var queryType string
		var overallFeedback int
		var count int
		var avgResponseTime float64

		err := rows.Scan(&queryType, &overallFeedback, &count, &avgResponseTime)
		if err != nil {
			return nil, fmt.Errorf("failed to scan stats: %w", err)
		}

		if queryTypeStats[queryType] == nil {
			queryTypeStats[queryType] = make(map[string]interface{})
		}

		feedbackType := "unknown"
		switch learning.FeedbackType(overallFeedback) {
		case learning.FeedbackPositive:
			feedbackType = "positive"
		case learning.FeedbackNegative:
			feedbackType = "negative"
		case learning.FeedbackNeutral:
			feedbackType = "neutral"
		}

		queryTypeStats[queryType][feedbackType] = map[string]interface{}{
			"count":                count,
			"avg_response_time_ms": avgResponseTime,
		}
	}

	stats["by_query_type"] = queryTypeStats
	return stats, nil
}
