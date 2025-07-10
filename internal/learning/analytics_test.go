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

package learning

import (
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type TestingT interface {
	Helper()
	Errorf(format string, args ...interface{})
	FailNow()
	Cleanup(func())
}

func setupTestDB(t TestingT) *sql.DB {
	// Create temporary database
	tempFile, err := os.CreateTemp("", "test_learning_*.db")
	if err != nil {
		t.Errorf("Failed to create temp file: %v", err)
		t.FailNow()
	}
	tempFile.Close()

	db, err := sql.Open("sqlite3", tempFile.Name())
	if err != nil {
		t.Errorf("Failed to open database: %v", err)
		t.FailNow()
	}

	// Create test tables
	_, err = db.Exec(`
		CREATE TABLE feedback (
			id TEXT PRIMARY KEY,
			query TEXT NOT NULL,
			feedback TEXT NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			user_id TEXT,
			session_id TEXT
		)
	`)
	if err != nil {
		t.Errorf("Failed to create test table: %v", err)
		t.FailNow()
	}

	// Clean up function
	t.Cleanup(func() {
		db.Close()
		os.Remove(tempFile.Name())
	})

	return db
}

func TestNewAnalytics(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()

	analytics := NewAnalytics(db, logger)
	assert.NotNil(t, analytics)
	assert.Equal(t, db, analytics.db)
	assert.Equal(t, logger, analytics.logger)
}

func TestAnalyzeFeedbackPatterns_EmptyData(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	analytics := NewAnalytics(db, logger)

	err := analytics.InitializeLearningTables()
	require.NoError(t, err)

	insights, err := analytics.AnalyzeFeedbackPatterns(30)
	require.NoError(t, err)
	assert.NotNil(t, insights)
	assert.Empty(t, insights.QueryPatterns)
	assert.Empty(t, insights.KnowledgeGaps)
	assert.Equal(t, 0.0, insights.ResponseQualityTrend)
}

func TestAnalyzeFeedbackPatterns_WithData(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	analytics := NewAnalytics(db, logger)

	err := analytics.InitializeLearningTables()
	require.NoError(t, err)

	// Insert test feedback data
	testData := []struct {
		query    string
		feedback string
	}{
		{"How to migrate to AWS?", "positive"},
		{"AWS migration best practices", "positive"},
		{"Security compliance for HIPAA", "negative"},
		{"GDPR requirements", "negative"},
		{"Hybrid cloud setup", "positive"},
	}

	for i, data := range testData {
		_, err := db.Exec(`
			INSERT INTO feedback (id, query, feedback, timestamp)
			VALUES (?, ?, ?, ?)
		`, i, data.query, data.feedback, time.Now())
		require.NoError(t, err)
	}

	insights, err := analytics.AnalyzeFeedbackPatterns(30)
	require.NoError(t, err)
	assert.NotNil(t, insights)
	assert.NotEmpty(t, insights.QueryPatterns)
	assert.NotEmpty(t, insights.KnowledgeGaps)

	// Verify migration queries have positive feedback
	assert.True(t, insights.QueryPatterns["migration"] > 0.5)

	// Verify security queries have knowledge gaps
	hasSecurityGap := false
	for _, gap := range insights.KnowledgeGaps {
		if gap.Topic == "security" {
			hasSecurityGap = true
			break
		}
	}
	assert.True(t, hasSecurityGap)
}

func TestClassifyQuery(t *testing.T) {
	testCases := []struct {
		query    string
		expected string
	}{
		{"How to migrate to AWS?", "migration"},
		{"AWS lift and shift migration", "migration"},
		{"Security compliance requirements", "security"},
		{"HIPAA compliance checklist", "security"},
		{"Hybrid cloud architecture", "hybrid"},
		{"Connect on-premises to Azure", "hybrid"},
		{"Disaster recovery planning", "disaster-recovery"},
		{"DR strategy for critical workloads", "disaster-recovery"},
		{"Cost optimization strategies", "cost-optimization"},
		{"Pricing calculator help", "cost-optimization"},
		{"Random question", "general"},
	}

	for _, tc := range testCases {
		result := classifyQuery(tc.query)
		assert.Equal(t, tc.expected, result, "Query: %s", tc.query)
	}
}

func TestParseFeedbackType(t *testing.T) {
	testCases := []struct {
		feedback string
		expected FeedbackType
	}{
		{"positive", FeedbackPositive},
		{"👍", FeedbackPositive},
		{"good", FeedbackPositive},
		{"helpful", FeedbackPositive},
		{"negative", FeedbackNegative},
		{"👎", FeedbackNegative},
		{"bad", FeedbackNegative},
		{"unhelpful", FeedbackNegative},
		{"neutral", FeedbackNeutral},
		{"unknown", FeedbackNeutral},
	}

	for _, tc := range testCases {
		result := parseFeedbackType(tc.feedback)
		assert.Equal(t, tc.expected, result, "Feedback: %s", tc.feedback)
	}
}

func TestOptimizeParameters(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	analytics := NewAnalytics(db, logger)

	// Test with high satisfaction rate (90% positive)
	positiveFeedback := []DetailedFeedback{
		{OverallFeedback: FeedbackPositive},
		{OverallFeedback: FeedbackPositive},
		{OverallFeedback: FeedbackPositive},
		{OverallFeedback: FeedbackPositive},
		{OverallFeedback: FeedbackPositive},
		{OverallFeedback: FeedbackPositive},
		{OverallFeedback: FeedbackPositive},
		{OverallFeedback: FeedbackPositive},
		{OverallFeedback: FeedbackPositive},
		{OverallFeedback: FeedbackNegative},
	}

	params := analytics.optimizeParameters(positiveFeedback)
	defaultParams := analytics.getDefaultParameters()

	// High satisfaction should increase thresholds
	assert.Greater(t, params.RetrievalThreshold, defaultParams.RetrievalThreshold)
	assert.Greater(t, params.FallbackThreshold, defaultParams.FallbackThreshold)

	// Test with low satisfaction rate (20% positive)
	negativeFeedback := []DetailedFeedback{
		{OverallFeedback: FeedbackNegative},
		{OverallFeedback: FeedbackNegative},
		{OverallFeedback: FeedbackNegative},
		{OverallFeedback: FeedbackNegative},
		{OverallFeedback: FeedbackPositive},
	}

	params = analytics.optimizeParameters(negativeFeedback)

	// Low satisfaction should decrease thresholds
	assert.Less(t, params.RetrievalThreshold, defaultParams.RetrievalThreshold)
	assert.Less(t, params.FallbackThreshold, defaultParams.FallbackThreshold)
}

func TestCalculateQualityTrend(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	analytics := NewAnalytics(db, logger)

	// Test improving trend (newer feedback first)
	improvingFeedback := []DetailedFeedback{
		{OverallFeedback: FeedbackPositive}, // newer (first half)
		{OverallFeedback: FeedbackPositive},
		{OverallFeedback: FeedbackNegative}, // older (second half)
		{OverallFeedback: FeedbackNegative},
	}

	trend := analytics.calculateQualityTrend(improvingFeedback)
	assert.Greater(t, trend, 0.0, "Should show improving trend")

	// Test declining trend (newer feedback first)
	decliningFeedback := []DetailedFeedback{
		{OverallFeedback: FeedbackNegative}, // newer (first half)
		{OverallFeedback: FeedbackNegative},
		{OverallFeedback: FeedbackPositive}, // older (second half)
		{OverallFeedback: FeedbackPositive},
	}

	trend = analytics.calculateQualityTrend(decliningFeedback)
	assert.Less(t, trend, 0.0, "Should show declining trend")
}

func TestStoreLearningInsights(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	analytics := NewAnalytics(db, logger)

	err := analytics.InitializeLearningTables()
	require.NoError(t, err)

	insights := &LearningInsights{
		QueryPatterns: map[string]float64{
			"migration": 0.8,
			"security":  0.4,
		},
		KnowledgeGaps: []KnowledgeGap{
			{
				Topic:            "security",
				Severity:         0.7,
				NegativeFeedback: 5,
				SuggestedActions: []string{"Update security docs", "Add examples"},
			},
		},
		OptimalParameters:    analytics.getDefaultParameters(),
		ResponseQualityTrend: 0.15,
		UpdatedAt:            time.Now(),
	}

	err = analytics.StoreLearningInsights(insights)
	require.NoError(t, err)

	// Retrieve and verify
	retrieved, err := analytics.GetLearningInsights()
	require.NoError(t, err)
	assert.Equal(t, insights.QueryPatterns, retrieved.QueryPatterns)
	assert.Equal(t, len(insights.KnowledgeGaps), len(retrieved.KnowledgeGaps))
	assert.Equal(t, insights.ResponseQualityTrend, retrieved.ResponseQualityTrend)
}

func TestGetLearningInsights_NoData(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	analytics := NewAnalytics(db, logger)

	err := analytics.InitializeLearningTables()
	require.NoError(t, err)

	// Should return default insights when none exist
	insights, err := analytics.GetLearningInsights()
	require.NoError(t, err)
	assert.NotNil(t, insights)
	assert.Empty(t, insights.QueryPatterns)
	assert.Empty(t, insights.KnowledgeGaps)
	assert.Equal(t, 0.0, insights.ResponseQualityTrend)
}

func TestGenerateSuggestedActions(t *testing.T) {
	testCases := []struct {
		topic           string
		expectedActions int
	}{
		{"migration", 3},
		{"security", 3},
		{"hybrid", 3},
		{"disaster-recovery", 3},
		{"cost-optimization", 3},
		{"unknown-topic", 3}, // should return default actions
	}

	for _, tc := range testCases {
		actions := generateSuggestedActions(tc.topic)
		assert.Len(t, actions, tc.expectedActions, "Topic: %s", tc.topic)
		assert.NotEmpty(t, actions[0], "First action should not be empty")
	}
}

func TestIdentifyKnowledgeGaps(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	analytics := NewAnalytics(db, logger)

	feedback := []DetailedFeedback{
		{QueryType: "security", OverallFeedback: FeedbackNegative},
		{QueryType: "security", OverallFeedback: FeedbackNegative},
		{QueryType: "security", OverallFeedback: FeedbackNegative},
		{QueryType: "migration", OverallFeedback: FeedbackNegative}, // Only one negative
		{QueryType: "hybrid", OverallFeedback: FeedbackPositive},    // Positive feedback
	}

	gaps := analytics.identifyKnowledgeGaps(feedback)

	// Should only include security (≥2 negative feedback)
	assert.Len(t, gaps, 1)
	assert.Equal(t, "security", gaps[0].Topic)
	assert.Equal(t, 3, gaps[0].NegativeFeedback)
	assert.Greater(t, gaps[0].Severity, 0.0)
	assert.NotEmpty(t, gaps[0].SuggestedActions)
}

func BenchmarkAnalyzeFeedbackPatterns(b *testing.B) {
	db := setupTestDB(b)
	logger := zap.NewNop()
	analytics := NewAnalytics(db, logger)

	err := analytics.InitializeLearningTables()
	require.NoError(b, err)

	// Insert test data
	for i := 0; i < 1000; i++ {
		_, err := db.Exec(`
			INSERT INTO feedback (id, query, feedback, timestamp)
			VALUES (?, ?, ?, ?)
		`, i, "test query", "positive", time.Now())
		require.NoError(b, err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := analytics.AnalyzeFeedbackPatterns(30)
		require.NoError(b, err)
	}
}
