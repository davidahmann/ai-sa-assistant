package openai

import (
	"context"
	"fmt"
	"time"

	"github.com/sashabaranov/go-openai"
)

// Client wraps the go-openai client with additional functionality
type Client struct {
	*openai.Client
	model string
}

// NewClient creates a new OpenAI client wrapper
func NewClient(apiKey, model string) *Client {
	return &Client{
		Client: openai.NewClient(apiKey),
		model:  model,
	}
}

// GenerateEmbeddings generates embeddings for a slice of text chunks
func (c *Client) GenerateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	req := openai.EmbeddingRequest{
		Input: texts,
		Model: openai.AdaEmbeddingV2,
	}

	resp, err := c.CreateEmbeddings(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create embeddings: %w", err)
	}

	embeddings := make([][]float32, len(resp.Data))
	for i, embedding := range resp.Data {
		embeddings[i] = embedding.Embedding
	}

	return embeddings, nil
}

// GenerateEmbedding generates a single embedding for a text
func (c *Client) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := c.GenerateEmbeddings(ctx, []string{text})
	if err != nil {
		return nil, err
	}

	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	return embeddings[0], nil
}

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

	// Implement exponential backoff for retries
	maxRetries := 3
	baseDelay := time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err := c.Client.CreateChatCompletion(ctx, openaiReq)
		if err != nil {
			// Check if it's a rate limit error
			if attempt < maxRetries-1 {
				delay := baseDelay * time.Duration(1<<uint(attempt))
				time.Sleep(delay)
				continue
			}
			return nil, fmt.Errorf("failed to create chat completion after %d attempts: %w", maxRetries, err)
		}

		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("no choices returned from OpenAI")
		}

		return &ChatCompletionResponse{
			Content:      resp.Choices[0].Message.Content,
			FinishReason: string(resp.Choices[0].FinishReason),
			Usage:        resp.Usage,
		}, nil
	}

	return nil, fmt.Errorf("exhausted all retry attempts")
}

// BuildSystemPrompt creates a system prompt for the assistant
func BuildSystemPrompt() string {
	return `You are an expert Cloud Solutions Architect assistant. Your role is to help Solutions Architects with pre-sales research and planning.

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
