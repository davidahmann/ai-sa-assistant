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

// Package main implements the learning service for feedback-based improvements
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/learning"
	"go.uber.org/zap"
)

const (
	// DefaultAnalysisInterval is the default interval for running learning analysis
	DefaultAnalysisInterval = 1 * time.Hour
	// DefaultLookbackDays is the default number of days to look back for feedback
	DefaultLookbackDays = 30
)

// LearningService manages the offline learning pipeline
type LearningService struct {
	analytics *learning.Analytics
	config    *config.Config
	logger    *zap.Logger
	db        *sql.DB
}

// NewLearningService creates a new learning service
func NewLearningService(cfg *config.Config, logger *zap.Logger) (*LearningService, error) {
	// Open database connection
	db, err := sql.Open("sqlite3", cfg.Feedback.DBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create analytics instance
	analytics := learning.NewAnalytics(db, logger)

	// Initialize learning tables
	if err := analytics.InitializeLearningTables(); err != nil {
		return nil, fmt.Errorf("failed to initialize learning tables: %w", err)
	}

	return &LearningService{
		analytics: analytics,
		config:    cfg,
		logger:    logger,
		db:        db,
	}, nil
}

// Run starts the learning service
func (ls *LearningService) Run(ctx context.Context) error {
	ls.logger.Info("Starting learning service",
		zap.Duration("analysis_interval", DefaultAnalysisInterval),
		zap.Int("lookback_days", DefaultLookbackDays))

	// Run initial analysis
	if err := ls.runLearningAnalysis(); err != nil {
		ls.logger.Error("Initial learning analysis failed", zap.Error(err))
	}

	// Set up periodic analysis
	ticker := time.NewTicker(DefaultAnalysisInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			ls.logger.Info("Learning service stopping")
			return nil
		case <-ticker.C:
			if err := ls.runLearningAnalysis(); err != nil {
				ls.logger.Error("Learning analysis failed", zap.Error(err))
			}
		}
	}
}

// runLearningAnalysis performs the learning analysis
func (ls *LearningService) runLearningAnalysis() error {
	start := time.Now()
	ls.logger.Info("Starting learning analysis")

	// Analyze feedback patterns
	insights, err := ls.analytics.AnalyzeFeedbackPatterns(DefaultLookbackDays)
	if err != nil {
		return fmt.Errorf("failed to analyze feedback patterns: %w", err)
	}

	// Store insights
	if err := ls.analytics.StoreLearningInsights(insights); err != nil {
		return fmt.Errorf("failed to store learning insights: %w", err)
	}

	// Log analysis results
	duration := time.Since(start)
	ls.logger.Info("Learning analysis completed",
		zap.Duration("duration", duration),
		zap.Int("knowledge_gaps", len(insights.KnowledgeGaps)),
		zap.Float64("quality_trend", insights.ResponseQualityTrend),
		zap.Int("query_patterns", len(insights.QueryPatterns)))

	// Log knowledge gaps if any
	if len(insights.KnowledgeGaps) > 0 {
		ls.logger.Warn("Knowledge gaps identified",
			zap.Int("count", len(insights.KnowledgeGaps)))
		for _, gap := range insights.KnowledgeGaps {
			ls.logger.Info("Knowledge gap details",
				zap.String("topic", gap.Topic),
				zap.Float64("severity", gap.Severity),
				zap.Int("negative_feedback", gap.NegativeFeedback),
				zap.Strings("suggested_actions", gap.SuggestedActions))
		}
	}

	return nil
}

// Close closes the learning service
func (ls *LearningService) Close() error {
	if ls.db != nil {
		return ls.db.Close()
	}
	return nil
}

func main() {
	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer func() { _ = logger.Sync() }()

	// Create learning service
	service, err := NewLearningService(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to create learning service", zap.Error(err))
	}
	defer func() { _ = service.Close() }()

	// Set up context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start service in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- service.Run(ctx)
	}()

	// Wait for shutdown signal or error
	select {
	case <-sigChan:
		logger.Info("Received shutdown signal")
		cancel()
		<-errChan // Wait for service to stop
	case err := <-errChan:
		if err != nil {
			logger.Error("Service error", zap.Error(err))
		}
	}

	logger.Info("Learning service stopped")
}
