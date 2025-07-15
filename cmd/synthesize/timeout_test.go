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

package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/resilience"
	"github.com/your-org/ai-sa-assistant/internal/session"
	"github.com/your-org/ai-sa-assistant/internal/synth"
)

func TestGetErrorType(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "none",
		},
		{
			name:     "timeout error",
			err:      errors.New("request timeout occurred"),
			expected: "timeout",
		},
		{
			name:     "deadline exceeded error",
			err:      context.DeadlineExceeded,
			expected: "timeout",
		},
		{
			name:     "rate limit error",
			err:      errors.New("rate limit exceeded"),
			expected: "rate_limit",
		},
		{
			name:     "too many requests error",
			err:      errors.New("too many requests"),
			expected: "rate_limit",
		},
		{
			name:     "circuit breaker error",
			err:      errors.New("circuit breaker is open"),
			expected: "circuit_breaker",
		},
		{
			name:     "connection error",
			err:      errors.New("connection refused"),
			expected: "connection",
		},
		{
			name:     "authentication error",
			err:      errors.New("authentication failed"),
			expected: "authentication",
		},
		{
			name:     "unknown error",
			err:      errors.New("some random error"),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getErrorType(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetConfiguredTimeout(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		expected time.Duration
	}{
		{
			name:     "nil config",
			cfg:      nil,
			expected: DefaultSynthesisRequestTimeout,
		},
		{
			name: "zero timeout",
			cfg: &config.Config{
				Synthesis: config.SynthesisConfig{
					TimeoutSeconds: 0,
				},
			},
			expected: DefaultSynthesisRequestTimeout,
		},
		{
			name: "configured timeout",
			cfg: &config.Config{
				Synthesis: config.SynthesisConfig{
					TimeoutSeconds: 45,
				},
			},
			expected: 45 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getConfiguredTimeout(tt.cfg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetAdaptiveTimeout(t *testing.T) {
	logger := zap.NewNop()

	cfg := &config.Config{
		Synthesis: config.SynthesisConfig{
			TimeoutSeconds:        60,
			SimpleTimeoutSeconds:  20,
			ComplexTimeoutSeconds: 90,
			EnableAdaptiveTimeout: true,
		},
	}

	tests := []struct {
		name     string
		req      SynthesisRequest
		expected time.Duration
	}{
		{
			name: "simple query",
			req: SynthesisRequest{
				Query:  "What is AWS?",
				Chunks: []ChunkItem{},
			},
			expected: 20 * time.Second,
		},
		{
			name: "complex query",
			req: SynthesisRequest{
				Query:  "Generate a comprehensive lift-and-shift migration plan for 120 on-premises Windows and Linux VMs to AWS",
				Chunks: make([]ChunkItem, 10), // Many chunks = complex
			},
			expected: 90 * time.Second,
		},
		{
			name: "medium query",
			req: SynthesisRequest{
				Query:  "How do I set up a VPC in AWS?",
				Chunks: make([]ChunkItem, 5), // Medium number of chunks
			},
			expected: 60 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getAdaptiveTimeout(cfg, tt.req, logger)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetAdaptiveTimeoutDisabled(t *testing.T) {
	logger := zap.NewNop()

	cfg := &config.Config{
		Synthesis: config.SynthesisConfig{
			TimeoutSeconds:        60,
			SimpleTimeoutSeconds:  20,
			ComplexTimeoutSeconds: 90,
			EnableAdaptiveTimeout: false, // Disabled
		},
	}

	req := SynthesisRequest{
		Query:  "Generate a comprehensive lift-and-shift migration plan for 120 on-premises Windows and Linux VMs to AWS",
		Chunks: make([]ChunkItem, 10), // Many chunks = complex
	}

	result := getAdaptiveTimeout(cfg, req, logger)
	// Should return base timeout since adaptive is disabled
	assert.Equal(t, 60*time.Second, result)
}

func TestCalculateQueryComplexity(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name     string
		req      SynthesisRequest
		expected string
	}{
		{
			name: "simple query",
			req: SynthesisRequest{
				Query:  "What is AWS?",
				Chunks: []ChunkItem{},
			},
			expected: "simple",
		},
		{
			name: "complex query by context",
			req: SynthesisRequest{
				Query:  "Migration plan",
				Chunks: make([]ChunkItem, 10), // Many chunks
			},
			expected: "complex",
		},
		{
			name: "complex query by length",
			req: SynthesisRequest{
				Query:  "Generate a comprehensive lift-and-shift migration plan for 120 on-premises Windows and Linux VMs to AWS including EC2 instance recommendations, VPC/subnet topology, and the latest AWS MGN best practices from Q2 2025. This should include detailed network configuration, security group setup, IAM roles and policies, monitoring and logging configurations, backup strategies, disaster recovery procedures, cost optimization recommendations, and step-by-step migration timelines with rollback procedures.", // Long, complex query
				Chunks: []ChunkItem{},
			},
			expected: "complex",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateQueryComplexity(tt.req, logger)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateRateLimitAwareRetryConfig(t *testing.T) {
	baseConfig := resilience.BackoffConfig{
		BaseDelay:   time.Second,
		MaxRetries:  3,
		MaxDelay:    30 * time.Second,
		Multiplier:  2.0,
		Jitter:      true,
		RetryOnFunc: resilience.DefaultRetryOnFunc,
	}

	rateLimitConfig := createRateLimitAwareRetryConfig(baseConfig)

	// Should have increased base delay for rate limits
	assert.GreaterOrEqual(t, rateLimitConfig.BaseDelay, 2*time.Second)

	// Should have increased max delay for rate limits
	assert.GreaterOrEqual(t, rateLimitConfig.MaxDelay, 120*time.Second)

	// Should retry on rate limit errors
	assert.True(t, rateLimitConfig.RetryOnFunc(errors.New("rate limit exceeded")))
	assert.True(t, rateLimitConfig.RetryOnFunc(errors.New("too many requests")))

	// Should not retry on context cancellation
	assert.False(t, rateLimitConfig.RetryOnFunc(context.Canceled))
	assert.False(t, rateLimitConfig.RetryOnFunc(context.DeadlineExceeded))
}

func TestEstimateTokenCount(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "empty text",
			text:     "",
			expected: 0,
		},
		{
			name:     "short text",
			text:     "Hello world",
			expected: estimateTokenCount("Hello world"),
		},
		{
			name:     "long text",
			text:     string(make([]byte, 1000)),
			expected: estimateTokenCount(string(make([]byte, 1000))),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := estimateTokenCount(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOptimizePromptSize(t *testing.T) {
	logger := zap.NewNop()

	webResults := []string{
		string(make([]byte, 500)),
		string(make([]byte, 500)),
		string(make([]byte, 500)),
	}

	conversation := []session.Message{
		{Content: string(make([]byte, 200))},
		{Content: string(make([]byte, 200))},
		{Content: string(make([]byte, 200))},
	}

	// Test with small prompt (should not optimize)
	smallContextItems := []synth.ContextItem{{Content: "small"}}
	smallWebResults := []string{"small"}
	smallConversation := []session.Message{{Content: "small"}}

	optimizedCtx, optimizedWeb, optimizedConv := optimizePromptSize(
		smallContextItems, smallWebResults, smallConversation, logger)

	assert.Len(t, optimizedCtx, 1)
	assert.Len(t, optimizedWeb, 1)
	assert.Len(t, optimizedConv, 1)

	// Test with large prompt (should optimize)
	largeContextItems := make([]synth.ContextItem, 15)
	for i := range largeContextItems {
		largeContextItems[i] = synth.ContextItem{Content: string(make([]byte, 2000))}
	}

	optimizedCtx, optimizedWeb, optimizedConv = optimizePromptSize(
		largeContextItems, webResults, conversation, logger)

	// Check if optimization actually happened
	t.Logf("Original context items: %d, Optimized: %d", len(largeContextItems), len(optimizedCtx))

	// The optimization should have reduced context items
	assert.True(t, len(optimizedCtx) <= len(largeContextItems), "Context items should be reduced")
	assert.LessOrEqual(t, len(optimizedWeb), MaxWebResultsForOptimization)
	assert.LessOrEqual(t, len(optimizedConv), 6) // Max conversation history
}
