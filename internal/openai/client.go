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

// Package openai provides a client wrapper for OpenAI API interactions.
// It handles embedding generation, chat completions, and includes retry logic
// with exponential backoff for robust API communication.
package openai

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
	"go.uber.org/zap"

	"github.com/your-org/ai-sa-assistant/internal/resilience"
)

const (
	// EmbeddingModel defines the model to use for embeddings
	EmbeddingModel = "text-embedding-3-small"
	// ExpectedEmbeddingDimensions defines the expected embedding dimensions
	ExpectedEmbeddingDimensions = 1536
	// MaxRetries defines the maximum number of retry attempts
	MaxRetries = 3
	// BaseRetryDelay defines the base delay for exponential backoff
	BaseRetryDelay = time.Second
	// EmbeddingCostPer1KTokens defines the cost per 1K tokens for embeddings (in USD)
	EmbeddingCostPer1KTokens = 0.00002
	// ValidationTimeout defines the timeout for connection validation
	ValidationTimeout = 10 * time.Second
	// CostCalculationDivisor defines the divisor for cost calculation
	CostCalculationDivisor = 1000.0
	// QueryPreviewLength defines the length for query preview truncation
	QueryPreviewLength = 100
)

// Client wraps the go-openai client with enhanced functionality
type Client struct {
	client         *openai.Client
	logger         *zap.Logger
	model          string
	circuitBreaker *resilience.CircuitBreaker
	errorHandler   *resilience.ErrorHandler
	timeoutManager *resilience.TimeoutManager
}

// EmbeddingUsage tracks embedding API usage and costs
type EmbeddingUsage struct {
	TokensUsed     int
	RequestCount   int
	EstimatedCost  float64
	ProcessingTime time.Duration
}

// EmbeddingResponse represents the response from embedding operations
type EmbeddingResponse struct {
	Embeddings [][]float32
	Usage      EmbeddingUsage
}

// RetryableError represents an error that can be retried
type RetryableError struct {
	StatusCode int
	Message    string
	RetryAfter time.Duration
}

func (e *RetryableError) Error() string {
	return fmt.Sprintf("retryable error (status %d): %s", e.StatusCode, e.Message)
}

// NewClient creates a new OpenAI client with enhanced functionality
func NewClient(apiKey string, logger *zap.Logger) (*Client, error) {
	if apiKey == "" {
		return nil, resilience.NewBadRequestError("API key is required", nil)
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	// Validate API key format (basic check)
	if !strings.HasPrefix(apiKey, "sk-") {
		return nil, resilience.NewBadRequestError("invalid API key format", nil)
	}

	// Initialize resilience components
	cbConfig := resilience.DefaultCircuitBreakerConfig("openai")
	cbConfig.MaxFailures = resilience.DefaultMaxRetries
	cbConfig.ResetTimeout = resilience.DefaultResetTimeoutSeconds * time.Second
	circuitBreaker := resilience.NewCircuitBreaker(cbConfig, logger)

	errorHandler := resilience.NewErrorHandler(logger)
	timeoutManager := resilience.NewTimeoutManager(resilience.DefaultTimeoutConfig())

	client := &Client{
		client:         openai.NewClient(apiKey),
		logger:         logger,
		model:          EmbeddingModel,
		circuitBreaker: circuitBreaker,
		errorHandler:   errorHandler,
		timeoutManager: timeoutManager,
	}

	// Validate client connectivity
	if err := client.validateConnection(); err != nil {
		return nil, errorHandler.WrapError(err, "validating OpenAI connection")
	}

	client.logger.Info("OpenAI client initialized successfully",
		zap.String("model", EmbeddingModel),
		zap.Int("expected_dimensions", ExpectedEmbeddingDimensions),
		zap.Int("max_retries", MaxRetries),
	)

	return client, nil
}

// NewClientWithConfig creates a new OpenAI client with custom configuration (for testing)
// This function skips validation and is primarily intended for testing with mock servers
func NewClientWithConfig(config openai.ClientConfig, logger *zap.Logger) *Client {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Initialize resilience components
	cbConfig := resilience.DefaultCircuitBreakerConfig("openai")
	cbConfig.MaxFailures = resilience.DefaultMaxRetries
	cbConfig.ResetTimeout = resilience.DefaultResetTimeoutSeconds * time.Second
	circuitBreaker := resilience.NewCircuitBreaker(cbConfig, logger)

	errorHandler := resilience.NewErrorHandler(logger)
	timeoutManager := resilience.NewTimeoutManager(resilience.DefaultTimeoutConfig())

	client := &Client{
		client:         openai.NewClientWithConfig(config),
		logger:         logger,
		model:          EmbeddingModel,
		circuitBreaker: circuitBreaker,
		errorHandler:   errorHandler,
		timeoutManager: timeoutManager,
	}

	logger.Debug("OpenAI client initialized with custom config",
		zap.String("model", EmbeddingModel),
		zap.String("base_url", config.BaseURL),
	)

	return client
}

// validateConnection validates the OpenAI API connection
func (c *Client) validateConnection() error {
	ctx, cancel := context.WithTimeout(context.Background(), ValidationTimeout)
	defer cancel()

	// Test with a simple embedding request
	_, err := c.EmbedQuery(ctx, "test")
	if err != nil {
		return fmt.Errorf("connection validation failed: %w", err)
	}

	return nil
}

// EmbedTexts generates embeddings for multiple text chunks with batch processing
func (c *Client) EmbedTexts(ctx context.Context, texts []string) (*EmbeddingResponse, error) {
	if len(texts) == 0 {
		return &EmbeddingResponse{
			Embeddings: [][]float32{},
			Usage:      EmbeddingUsage{},
		}, nil
	}

	c.logger.Debug("Starting batch embedding generation",
		zap.Int("text_count", len(texts)),
		zap.String("model", c.model),
	)

	start := time.Now()
	var totalTokens int
	var totalRequests int

	// Process embeddings with retry logic
	embeddings, usage, err := c.createEmbeddingsWithRetry(ctx, texts)
	if err != nil {
		c.logger.Error("Failed to create embeddings",
			zap.Error(err),
			zap.Int("text_count", len(texts)),
		)
		return nil, fmt.Errorf("failed to create embeddings: %w", err)
	}

	// Validate embedding dimensions
	if err := c.validateEmbeddingDimensions(embeddings); err != nil {
		c.logger.Error("Invalid embedding dimensions",
			zap.Error(err),
			zap.Int("expected_dimensions", ExpectedEmbeddingDimensions),
		)
		return nil, fmt.Errorf("embedding validation failed: %w", err)
	}

	totalTokens += usage.PromptTokens
	totalRequests++

	processingTime := time.Since(start)
	estimatedCost := float64(totalTokens) / CostCalculationDivisor * EmbeddingCostPer1KTokens

	c.logger.Info("Batch embedding generation completed",
		zap.Int("text_count", len(texts)),
		zap.Int("tokens_used", totalTokens),
		zap.Int("requests_made", totalRequests),
		zap.Float64("estimated_cost_usd", estimatedCost),
		zap.Duration("processing_time", processingTime),
	)

	return &EmbeddingResponse{
		Embeddings: embeddings,
		Usage: EmbeddingUsage{
			TokensUsed:     totalTokens,
			RequestCount:   totalRequests,
			EstimatedCost:  estimatedCost,
			ProcessingTime: processingTime,
		},
	}, nil
}

// EmbedQuery generates an embedding for a single query text
func (c *Client) EmbedQuery(ctx context.Context, query string) ([]float32, error) {
	if query == "" {
		return nil, fmt.Errorf("query text cannot be empty")
	}

	c.logger.Debug("Starting query embedding generation",
		zap.String("query_preview", truncateText(query, QueryPreviewLength)),
		zap.String("model", c.model),
	)

	response, err := c.EmbedTexts(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	if len(response.Embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned for query")
	}

	c.logger.Debug("Query embedding generation completed",
		zap.Int("embedding_dimensions", len(response.Embeddings[0])),
		zap.Int("tokens_used", response.Usage.TokensUsed),
		zap.Float64("estimated_cost_usd", response.Usage.EstimatedCost),
	)

	return response.Embeddings[0], nil
}

// createEmbeddingsWithRetry creates embeddings with exponential backoff retry logic
func (c *Client) createEmbeddingsWithRetry(ctx context.Context, texts []string) ([][]float32, openai.Usage, error) {
	var embeddings [][]float32
	var usage openai.Usage

	// Use circuit breaker and exponential backoff
	err := c.circuitBreaker.Execute(ctx, func(ctx context.Context) error {
		return c.timeoutManager.Execute(ctx, func(ctx context.Context) error {
			return resilience.SimpleRetry(ctx, c.logger, func(ctx context.Context) error {
				var err error
				embeddings, usage, err = c.createEmbeddings(ctx, texts)
				return err
			})
		})
	})

	if err != nil {
		return nil, openai.Usage{}, c.errorHandler.WrapError(err, "creating embeddings")
	}

	return embeddings, usage, nil
}

// createEmbeddings creates embeddings using the OpenAI API
func (c *Client) createEmbeddings(ctx context.Context, texts []string) ([][]float32, openai.Usage, error) {
	req := openai.EmbeddingRequest{
		Input: texts,
		Model: openai.EmbeddingModel(c.model),
	}

	c.logger.Debug("Sending embedding request to OpenAI",
		zap.Int("input_count", len(texts)),
		zap.String("model", c.model),
	)

	resp, err := c.client.CreateEmbeddings(ctx, req)
	if err != nil {
		return nil, openai.Usage{}, c.handleAPIError(err)
	}

	if len(resp.Data) != len(texts) {
		return nil, openai.Usage{}, fmt.Errorf("unexpected response: got %d embeddings for %d texts",
			len(resp.Data), len(texts))
	}

	embeddings := make([][]float32, len(resp.Data))
	for i, embedding := range resp.Data {
		embeddings[i] = embedding.Embedding
	}

	c.logger.Debug("Embedding request completed successfully",
		zap.Int("embeddings_count", len(embeddings)),
		zap.Int("prompt_tokens", resp.Usage.PromptTokens),
		zap.Int("total_tokens", resp.Usage.TotalTokens),
	)

	return embeddings, resp.Usage, nil
}

// handleAPIError handles OpenAI API errors and determines if they are retryable
func (c *Client) handleAPIError(err error) error {
	if apiErr, ok := err.(*openai.APIError); ok {
		switch apiErr.HTTPStatusCode {
		case http.StatusUnauthorized:
			return resilience.NewUnauthorizedError("invalid API key or unauthorized access", err)
		case http.StatusTooManyRequests:
			return resilience.NewTooManyRequestsError("rate limit exceeded", err)
		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return resilience.NewServiceUnavailableError("OpenAI service temporarily unavailable", err)
		case http.StatusBadRequest:
			return resilience.NewBadRequestError(fmt.Sprintf("invalid request: %s", apiErr.Message), err)
		default:
			return resilience.NewInternalError(
				fmt.Sprintf("OpenAI API error (status %d): %s", apiErr.HTTPStatusCode, apiErr.Message),
				err,
			)
		}
	}

	return resilience.NewInternalError("OpenAI client error", err)
}

// validateEmbeddingDimensions validates that embeddings have the expected dimensions
func (c *Client) validateEmbeddingDimensions(embeddings [][]float32) error {
	for i, embedding := range embeddings {
		if len(embedding) != ExpectedEmbeddingDimensions {
			return fmt.Errorf("embedding %d has %d dimensions, expected %d", i, len(embedding), ExpectedEmbeddingDimensions)
		}
	}
	return nil
}

// truncateText truncates text to a maximum length for logging
func truncateText(text string, maxLength int) string {
	if len(text) <= maxLength {
		return text
	}
	return text[:maxLength] + "..."
}

// Legacy methods for backward compatibility

// GenerateEmbeddings generates embeddings for a slice of text chunks (legacy method)
func (c *Client) GenerateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	response, err := c.EmbedTexts(ctx, texts)
	if err != nil {
		return nil, err
	}
	return response.Embeddings, nil
}

// GenerateEmbedding generates a single embedding for a text (legacy method)
func (c *Client) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	return c.EmbedQuery(ctx, text)
}

// Chat completion related structs and methods (preserved from existing implementation)

// ChatCompletionRequest represents a chat completion request
type ChatCompletionRequest struct {
	Messages    []openai.ChatCompletionMessage
	MaxTokens   int
	Temperature float32
	Model       string
}

// ChatCompletionResponse represents the response from a chat completion
type ChatCompletionResponse struct {
	Content      string
	FinishReason string
	Usage        openai.Usage
}

// CreateChatCompletion creates a chat completion with retry logic
func (c *Client) CreateChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// Use configured model if not specified
	if req.Model == "" {
		req.Model = c.model
	}

	openaiReq := openai.ChatCompletionRequest{
		Model:       req.Model,
		Messages:    req.Messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}

	c.logger.Debug("Creating chat completion",
		zap.String("model", req.Model),
		zap.Int("max_tokens", req.MaxTokens),
		zap.Float64("temperature", float64(req.Temperature)),
		zap.Int("message_count", len(req.Messages)),
	)

	var resp *ChatCompletionResponse

	// Use circuit breaker and exponential backoff
	err := c.circuitBreaker.Execute(ctx, func(ctx context.Context) error {
		return c.timeoutManager.Execute(ctx, func(ctx context.Context) error {
			return resilience.SimpleRetry(ctx, c.logger, func(ctx context.Context) error {
				openaiResp, err := c.client.CreateChatCompletion(ctx, openaiReq)
				if err != nil {
					return c.handleAPIError(err)
				}

				if len(openaiResp.Choices) == 0 {
					return resilience.NewInternalError("no choices returned from OpenAI", nil)
				}

				c.logger.Debug("Chat completion successful",
					zap.String("finish_reason", string(openaiResp.Choices[0].FinishReason)),
					zap.Int("prompt_tokens", openaiResp.Usage.PromptTokens),
					zap.Int("completion_tokens", openaiResp.Usage.CompletionTokens),
					zap.Int("total_tokens", openaiResp.Usage.TotalTokens),
				)

				resp = &ChatCompletionResponse{
					Content:      openaiResp.Choices[0].Message.Content,
					FinishReason: string(openaiResp.Choices[0].FinishReason),
					Usage:        openaiResp.Usage,
				}
				return nil
			})
		})
	})

	if err != nil {
		return nil, c.errorHandler.WrapError(err, "creating chat completion")
	}

	return resp, nil
}

// BuildSystemPrompt creates a system prompt for the assistant
func BuildSystemPrompt() string {
	return `You are an expert Cloud Solutions Architect assistant. Your role is to help Solutions Architects ` +
		`with pre-sales research and planning.

Key responsibilities:
1. Synthesize information from internal playbooks and live web sources
2. Generate comprehensive, actionable plans for cloud migrations and architectures
3. Create high-level architecture diagrams using Mermaid.js syntax
4. Provide relevant code snippets (Terraform, AWS CLI, etc.) when applicable
5. Always cite your sources using [source_id] format

When responding:
- Be concise but comprehensive
- Focus on practical, actionable recommendations
- Include relevant technical details and best practices
- Generate architecture diagrams when appropriate
- Provide code examples to accelerate implementation

For diagrams, use Mermaid.js graph TD syntax and enclose in a mermaid code block.
For code snippets, use appropriate language identifiers (terraform, bash, yaml, etc.).`
}

// BuildUserPrompt creates a user prompt with context
func BuildUserPrompt(query string, contextChunks []string, webResults []string) string {
	prompt := fmt.Sprintf("User Query: %s\n\n", query)

	if len(contextChunks) > 0 {
		prompt += "--- Internal Document Context ---\n"
		for i, chunk := range contextChunks {
			prompt += fmt.Sprintf("Context %d: %s\n\n", i+1, chunk)
		}
	}

	if len(webResults) > 0 {
		prompt += "--- Live Web Search Results ---\n"
		for i, result := range webResults {
			prompt += fmt.Sprintf("Web Result %d: %s\n\n", i+1, result)
		}
	}

	prompt += `Please provide a comprehensive response that includes:
1. A detailed answer to the user's query
2. A high-level architecture diagram using Mermaid.js syntax (if applicable)
3. Relevant code snippets for implementation (if applicable)
4. Proper citations using [source_id] format

Remember to be specific, actionable, and professional in your response.`

	return prompt
}

// GetHealthCheck returns a health check function for this client
func (c *Client) GetHealthCheck() resilience.HealthCheckFunc {
	return func(ctx context.Context) resilience.HealthCheckResult {
		// Try a simple embedding request as a health check
		start := time.Now()
		_, err := c.EmbedQuery(ctx, "health check")
		duration := time.Since(start)

		if err != nil {
			return resilience.HealthCheckResult{
				Status:    resilience.HealthStatusUnhealthy,
				Message:   fmt.Sprintf("OpenAI API health check failed: %v", err),
				Timestamp: time.Now(),
				Duration:  duration,
			}
		}

		return resilience.HealthCheckResult{
			Status:    resilience.HealthStatusHealthy,
			Message:   "OpenAI API is healthy",
			Timestamp: time.Now(),
			Duration:  duration,
		}
	}
}

// GetCircuitBreakerStats returns circuit breaker statistics
func (c *Client) GetCircuitBreakerStats() resilience.CircuitBreakerStats {
	return c.circuitBreaker.GetStats()
}
