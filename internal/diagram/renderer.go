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

// Package diagram provides functionality for rendering Mermaid diagrams as images
// using the mermaid.ink API service. It includes caching, error handling, and
// security validation for diagram rendering in Teams Adaptive Cards.
package diagram

import (
	"bytes"
	"context"
	"crypto/md5" // #nosec G501 - MD5 used only for cache key generation, not security
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	// DefaultMermaidInkURL is the default mermaid.ink API endpoint
	DefaultMermaidInkURL = "https://mermaid.ink/img"
	// DefaultTimeout is the default HTTP timeout for rendering requests
	DefaultTimeout = 30 * time.Second
	// DefaultCacheExpiry is the default cache expiry time for rendered diagrams
	DefaultCacheExpiry = 24 * time.Hour
	// MaxDiagramSize is the maximum allowed size for a Mermaid diagram
	MaxDiagramSize = 10 * 1024 // 10KB
	// MaxCacheSize is the maximum number of cached diagrams
	MaxCacheSize = 1000
)

// RendererConfig holds configuration for the diagram renderer
type RendererConfig struct {
	// MermaidInkURL is the mermaid.ink API endpoint
	MermaidInkURL string `mapstructure:"mermaid_ink_url"`
	// Timeout is the HTTP timeout for rendering requests
	Timeout time.Duration `mapstructure:"timeout"`
	// CacheExpiry is the cache expiry time for rendered diagrams
	CacheExpiry time.Duration `mapstructure:"cache_expiry"`
	// EnableCaching enables/disables diagram caching
	EnableCaching bool `mapstructure:"enable_caching"`
	// MaxDiagramSize is the maximum allowed size for a Mermaid diagram
	MaxDiagramSize int `mapstructure:"max_diagram_size"`
}

// DefaultRendererConfig returns default configuration for the diagram renderer
func DefaultRendererConfig() RendererConfig {
	return RendererConfig{
		MermaidInkURL:  DefaultMermaidInkURL,
		Timeout:        DefaultTimeout,
		CacheExpiry:    DefaultCacheExpiry,
		EnableCaching:  true,
		MaxDiagramSize: MaxDiagramSize,
	}
}

// CacheEntry represents a cached diagram entry
type CacheEntry struct {
	URL       string
	ExpiresAt time.Time
}

// Renderer handles diagram rendering using mermaid.ink API
type Renderer struct {
	config     RendererConfig
	httpClient *http.Client
	cache      map[string]CacheEntry
	cacheMutex sync.RWMutex
	logger     *zap.Logger
}

// NewRenderer creates a new diagram renderer with the given configuration
func NewRenderer(config RendererConfig, logger *zap.Logger) *Renderer {
	return &Renderer{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		cache:  make(map[string]CacheEntry),
		logger: logger,
	}
}

// RenderDiagram renders a Mermaid diagram to an image URL
func (r *Renderer) RenderDiagram(ctx context.Context, mermaidCode string) (string, error) {
	// Validate input
	if err := r.validateDiagramCode(mermaidCode); err != nil {
		return "", fmt.Errorf("invalid diagram code: %w", err)
	}

	// Check cache first
	if r.config.EnableCaching {
		if url, found := r.getCachedDiagram(mermaidCode); found {
			r.logger.Debug("Found cached diagram", zap.String("url", url))
			return url, nil
		}
	}

	// Render diagram
	url, err := r.renderDiagramViaAPI(ctx, mermaidCode)
	if err != nil {
		return "", fmt.Errorf("failed to render diagram: %w", err)
	}

	// Validate rendered URL
	if err := r.validateRenderedURL(url); err != nil {
		return "", fmt.Errorf("invalid rendered URL: %w", err)
	}

	// Cache the result
	if r.config.EnableCaching {
		r.cacheDiagram(mermaidCode, url)
	}

	r.logger.Info("Successfully rendered diagram", zap.String("url", url))
	return url, nil
}

// validateDiagramCode validates the Mermaid diagram code
func (r *Renderer) validateDiagramCode(code string) error {
	if strings.TrimSpace(code) == "" {
		return fmt.Errorf("diagram code cannot be empty")
	}

	if len(code) > r.config.MaxDiagramSize {
		return fmt.Errorf("diagram code too large: %d bytes (max: %d)", len(code), r.config.MaxDiagramSize)
	}

	// Basic Mermaid syntax validation
	if !strings.Contains(code, "graph TD") && !strings.Contains(code, "graph LR") {
		return fmt.Errorf("diagram code must contain valid Mermaid graph syntax")
	}

	// Check for potentially malicious content
	if containsMaliciousContent(code) {
		return fmt.Errorf("diagram code contains potentially malicious content")
	}

	return nil
}

// containsMaliciousContent checks for potentially malicious content in diagram code
func containsMaliciousContent(code string) bool {
	// Check for script injection attempts
	maliciousPatterns := []string{
		"<script",
		"javascript:",
		"onclick=",
		"onerror=",
		"onload=",
		"eval(",
		"setTimeout(",
		"setInterval(",
	}

	codeLower := strings.ToLower(code)
	for _, pattern := range maliciousPatterns {
		if strings.Contains(codeLower, pattern) {
			return true
		}
	}

	return false
}

// renderDiagramViaAPI renders the diagram using the mermaid.ink API
func (r *Renderer) renderDiagramViaAPI(ctx context.Context, mermaidCode string) (string, error) {
	// Encode diagram code to base64
	encodedCode := base64.StdEncoding.EncodeToString([]byte(mermaidCode))

	// Construct API URL
	apiURL := fmt.Sprintf("%s/%s", r.config.MermaidInkURL, encodedCode)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("User-Agent", "AI-SA-Assistant/1.0")
	req.Header.Set("Accept", "image/svg+xml,image/png,image/*")

	// Make request
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Verify response content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("unexpected content type: %s", contentType)
	}

	// Return the URL (mermaid.ink returns the image directly, so we return the API URL)
	return apiURL, nil
}

// validateRenderedURL validates the rendered image URL
func (r *Renderer) validateRenderedURL(imageURL string) error {
	if imageURL == "" {
		return fmt.Errorf("rendered URL cannot be empty")
	}

	// Parse URL
	parsedURL, err := url.Parse(imageURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	// Check scheme
	if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		return fmt.Errorf("invalid URL scheme: %s", parsedURL.Scheme)
	}

	// Check host (only allow mermaid.ink domains)
	allowedHosts := []string{"mermaid.ink", "www.mermaid.ink"}
	hostAllowed := false
	for _, allowedHost := range allowedHosts {
		if parsedURL.Host == allowedHost {
			hostAllowed = true
			break
		}
	}
	if !hostAllowed {
		return fmt.Errorf("invalid host: %s", parsedURL.Host)
	}

	return nil
}

// getCachedDiagram retrieves a cached diagram URL if available and not expired
func (r *Renderer) getCachedDiagram(mermaidCode string) (string, bool) {
	r.cacheMutex.RLock()
	defer r.cacheMutex.RUnlock()

	key := r.generateCacheKey(mermaidCode)
	entry, exists := r.cache[key]
	if !exists {
		return "", false
	}

	// Check if expired
	if time.Now().After(entry.ExpiresAt) {
		// Remove expired entry
		delete(r.cache, key)
		return "", false
	}

	return entry.URL, true
}

// cacheDiagram caches a rendered diagram URL
func (r *Renderer) cacheDiagram(mermaidCode, url string) {
	r.cacheMutex.Lock()
	defer r.cacheMutex.Unlock()

	// Check cache size limit
	if len(r.cache) >= MaxCacheSize {
		// Remove oldest entries (simple LRU approximation)
		now := time.Now()
		for key, entry := range r.cache {
			if now.After(entry.ExpiresAt) {
				delete(r.cache, key)
			}
		}

		// If still at limit, remove arbitrary entries
		if len(r.cache) >= MaxCacheSize {
			count := 0
			for key := range r.cache {
				delete(r.cache, key)
				count++
				if count >= MaxCacheSize/4 { // Remove 25% of entries
					break
				}
			}
		}
	}

	key := r.generateCacheKey(mermaidCode)
	r.cache[key] = CacheEntry{
		URL:       url,
		ExpiresAt: time.Now().Add(r.config.CacheExpiry),
	}
}

// generateCacheKey generates a cache key for the given Mermaid code
func (r *Renderer) generateCacheKey(mermaidCode string) string {
	// Use MD5 hash of the code as the cache key (not for security, only for cache key generation)
	hasher := md5.New() // #nosec G401
	hasher.Write([]byte(mermaidCode))
	return hex.EncodeToString(hasher.Sum(nil))
}

// ClearCache clears all cached diagrams
func (r *Renderer) ClearCache() {
	r.cacheMutex.Lock()
	defer r.cacheMutex.Unlock()

	r.cache = make(map[string]CacheEntry)
	r.logger.Info("Diagram cache cleared")
}

// GetCacheStats returns statistics about the cache
func (r *Renderer) GetCacheStats() map[string]interface{} {
	r.cacheMutex.RLock()
	defer r.cacheMutex.RUnlock()

	expired := 0
	now := time.Now()
	for _, entry := range r.cache {
		if now.After(entry.ExpiresAt) {
			expired++
		}
	}

	return map[string]interface{}{
		"total_entries":   len(r.cache),
		"expired_entries": expired,
		"max_size":        MaxCacheSize,
		"expiry_duration": r.config.CacheExpiry.String(),
	}
}

// TestConnection tests the connection to the mermaid.ink API
func (r *Renderer) TestConnection(ctx context.Context) error {
	// Test with a simple diagram
	testDiagram := "graph TD\n    A[Test] --> B[Connection]"

	_, err := r.renderDiagramViaAPI(ctx, testDiagram)
	if err != nil {
		return fmt.Errorf("mermaid.ink API test failed: %w", err)
	}

	return nil
}

// RenderDiagramWithFallback renders a diagram with fallback options
func (r *Renderer) RenderDiagramWithFallback(ctx context.Context, mermaidCode string) (
	imageURL string, fallbackText string, err error) {
	// Try to render the diagram
	imageURL, err = r.RenderDiagram(ctx, mermaidCode)
	if err != nil {
		// Log the error but don't fail completely
		r.logger.Warn("Failed to render diagram, using fallback", zap.Error(err))

		// Create fallback text representation
		fallbackText = r.createFallbackText(mermaidCode)
		return "", fallbackText, nil
	}

	return imageURL, "", nil
}

// createFallbackText creates a text representation of the diagram for fallback
func (r *Renderer) createFallbackText(mermaidCode string) string {
	// Simple text representation of the diagram
	var buffer bytes.Buffer
	buffer.WriteString("**Architecture Diagram (Text Representation):**\n```\n")
	buffer.WriteString(mermaidCode)
	buffer.WriteString("\n```\n")
	buffer.WriteString("*Note: Diagram rendering is temporarily unavailable. Please see the text representation above.*")

	return buffer.String()
}
